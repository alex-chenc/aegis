package assistant

import (
	"context"
	"fmt"
	"strings"

	agentruntime "github.com/alex-chenc/agent-runtime"
	"go.uber.org/zap"
)

// CompiledPlanValidator runs after the workflow compiler and before
// RuntimeFactory.Build. It catches deterministic plan errors—bad argument
// types, missing previous_step producers, unclosed async completion contracts—
// so they surface as a single clear failure instead of a repeated identical
// runtime tool-call error.
type CompiledPlanValidator struct {
	registry *ToolRegistry
	logger   *zap.Logger
}

func NewCompiledPlanValidator(registry *ToolRegistry, logger *zap.Logger) *CompiledPlanValidator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CompiledPlanValidator{registry: registry, logger: logger}
}

// Validate checks the compiled plan against schema, dependency, and completion
// contract rules. It returns a *CompilePlanError when a deterministic violation
// is found so the caller can surface a structured, localized failure.
func (v *CompiledPlanValidator) Validate(plan *ToolExecutionPlan, descriptors []agentruntime.ToolDescriptor) error {
	if plan == nil || len(plan.Steps) == 0 {
		return nil
	}
	descriptorByName := make(map[string]agentruntime.ToolDescriptor, len(descriptors))
	for _, descriptor := range descriptors {
		descriptorByName[descriptor.Name] = descriptor
	}
	mappedToolSet := make(map[string]bool, len(plan.Steps))
	for _, step := range plan.Steps {
		mappedToolSet[step.ToolName] = true
	}

	// 1. Argument schema validation + typed binding checks.
	for _, step := range plan.Steps {
		tool, ok := v.registry.Get(step.ToolName)
		if !ok || tool == nil {
			continue
		}
		if err := v.validateStepArgs(step, tool); err != nil {
			v.logger.Warn("assistant compiled plan rejected",
				zap.String("workflow_id", workflowIDFromPlan(plan)),
				zap.String("step_id", step.StepID),
				zap.String("tool_name", step.ToolName),
				zap.String("validation_stage", "arguments"),
				zap.String("error_code", "compiled_plan_invalid"),
				zap.Error(err),
			)
			return err
		}
	}

	// 2. Required previous_step arguments must have a reachable producer.
	for index, step := range plan.Steps {
		if err := v.validatePreviousStepProducers(step, plan.Steps[:index], step.ToolName); err != nil {
			v.logger.Warn("assistant compiled plan rejected",
				zap.String("workflow_id", workflowIDFromPlan(plan)),
				zap.String("step_id", step.StepID),
				zap.String("tool_name", step.ToolName),
				zap.String("validation_stage", "dependencies"),
				zap.String("error_code", "compiled_plan_invalid"),
				zap.Error(err),
			)
			return err
		}
	}

	// 3. Async primary must declare a mapped completion capability.
	for _, step := range plan.Steps {
		tool, ok := v.registry.Get(step.ToolName)
		if !ok || tool == nil {
			continue
		}
		if tool.ExecutionContract.Mode != ToolExecutionAsynchronous {
			continue
		}
		completionCapability := strings.TrimSpace(tool.ExecutionContract.CompletionCapability)
		if completionCapability == "" {
			continue
		}
		if !v.hasMappedCompletionTool(completionCapability, mappedToolSet) {
			err := newCompilePlanError("completion_contract", step.ToolName, "completion_capability",
				fmt.Sprintf("asynchronous tool %s declares completion capability %q but no mapped tool provides it", step.ToolName, completionCapability))
			v.logger.Warn("assistant compiled plan rejected",
				zap.String("workflow_id", workflowIDFromPlan(plan)),
				zap.String("step_id", step.StepID),
				zap.String("tool_name", step.ToolName),
				zap.String("validation_stage", "completion_contract"),
				zap.String("error_code", "compiled_plan_invalid"),
				zap.Error(err),
			)
			return err
		}
	}

	return nil
}

// validateStepArgs checks that compiled arguments satisfy the tool's JSON
// Schema and that argument values respect their declared ArgValueKind. Required
// arguments bound from previous_step are exempt from the presence check because
// they are injected at runtime by the ToolGateway.
func (v *CompiledPlanValidator) validateStepArgs(step ToolPlanStep, tool *ToolSpec) error {
	schema := normalizeRuntimeArgsSchema(tool.ArgsSchema)
	// Build the set of required args that are satisfied by a previous_step
	// binding so they are not flagged as missing.
	previousStepArgs := make(map[string]bool)
	for argName, source := range step.ArgSources {
		if strings.EqualFold(strings.TrimSpace(source.SourceType), "previous_step") {
			previousStepArgs[argName] = true
		}
	}
	// Validate only the present compiled arguments.
	if err := ValidateToolArgs(schema, step.Args); err != nil {
		// If the error is about a missing required arg that is bound from
		// previous_step, it is expected—skip it.
		if isMissingRequiredForPreviousStep(err, previousStepArgs) {
			return nil
		}
		return newCompilePlanError("arguments", step.ToolName, "args", err.Error())
	}
	if tool.Preflight != nil {
		if err := tool.Preflight(context.Background(), step.Args); err != nil {
			return newCompilePlanError("arguments", step.ToolName, "args", err.Error())
		}
	}
	return nil
}

// validatePreviousStepProducers ensures that a required previous_step argument
// has at least one prior plan step that can produce it.
func (v *CompiledPlanValidator) validatePreviousStepProducers(step ToolPlanStep, priorSteps []ToolPlanStep, toolName string) error {
	for argName, source := range step.ArgSources {
		if !strings.EqualFold(strings.TrimSpace(source.SourceType), "previous_step") {
			continue
		}
		hasProducer := false
		for _, prior := range priorSteps {
			tool, ok := v.registry.Get(prior.ToolName)
			if !ok || tool == nil {
				continue
			}
			if resultContractProducesArgument(tool.ResultContract, argName, source.SourceRef) {
				hasProducer = true
				break
			}
		}
		if !hasProducer {
			return newCompilePlanError("dependencies", step.ToolName, argName,
				fmt.Sprintf("step %s requires previous_step argument %q but no prior step declares a matching result field", step.StepID, argName))
		}
	}
	return nil
}

func resultContractProducesArgument(contract ToolResultContract, argName, sourceRef string) bool {
	candidates := make([]string, 0, len(contract.OperationRefFields)+len(contract.ArtifactRefFields)+len(contract.SideEffectRefFields)+len(contract.FactBindings))
	candidates = append(candidates, contract.OperationRefFields...)
	candidates = append(candidates, contract.ArtifactRefFields...)
	candidates = append(candidates, contract.SideEffectRefFields...)
	for _, binding := range contract.FactBindings {
		candidates = append(candidates, binding.IDField)
	}
	argBase := resultReferenceBase(argName)
	sourceBase := resultReferenceBase(sourceRef)
	for _, candidate := range candidates {
		candidateBase := resultReferenceBase(candidate)
		if candidateBase == "" {
			continue
		}
		if candidateBase == argBase || (sourceBase != "" && candidateBase == sourceBase) {
			return true
		}
	}
	return false
}

func resultReferenceBase(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if index := strings.LastIndex(value, "."); index >= 0 {
		value = value[index+1:]
	}
	value = strings.TrimSuffix(value, "_ids")
	value = strings.TrimSuffix(value, "_id")
	return strings.TrimSpace(value)
}

// hasMappedCompletionTool checks whether any mapped plan tool provides the
// declared completion capability.
func (v *CompiledPlanValidator) hasMappedCompletionTool(capability string, mappedToolSet map[string]bool) bool {
	capability = strings.ToLower(strings.TrimSpace(capability))
	for _, tool := range v.registry.List() {
		if tool == nil || !tool.Enabled {
			continue
		}
		if !mappedToolSet[tool.Name] {
			continue
		}
		if strings.EqualFold(strings.ToLower(strings.TrimSpace(tool.Capability)), capability) {
			return true
		}
	}
	return false
}

func workflowIDFromPlan(plan *ToolExecutionPlan) string {
	if plan == nil {
		return ""
	}
	// The plan does not carry workflow_ids directly; derive from step capabilities.
	for _, step := range plan.Steps {
		switch step.Capability {
		case "trigger_asset_collection":
			return assetInventoryWorkflowID
		case "start_vulnerability_scan":
			return vulnerabilityAssessmentWorkflowID
		case "generate_vulnerability_script", "execute_vulnerability_host_scripts":
			return vulnerabilityRemediationWorkflowID
		}
	}
	return ""
}

// isMissingRequiredForPreviousStep returns true when the validation error is
// about a missing required property that is expected to be injected from a
// prior step's outcome.
func isMissingRequiredForPreviousStep(err error, previousStepArgs map[string]bool) bool {
	if err == nil || len(previousStepArgs) == 0 {
		return false
	}
	msg := err.Error()
	// ValidateToolArgs produces messages like "$.task_id is required".
	for argName := range previousStepArgs {
		if strings.Contains(msg, argName+" is required") {
			return true
		}
	}
	return false
}
