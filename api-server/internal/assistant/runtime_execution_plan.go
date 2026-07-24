package assistant

import (
	"fmt"
	"strings"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

// TOOL ELECTION SECURITY INVARIANT:
// Every tool-enabled Assistant run must execute the immutable plan produced by
// capability Mapping. agent-runtime may execute a bound step, but it must never
// create a second model-authored tool plan or elect a free tool_name.
//
// Do not weaken this to a workflow-specific check. New Assistant tools become
// executable only through capability Mapping and authorization hard gates.
func runtimeInitialPlanForAssistant(plan *ToolExecutionPlan) *agentruntime.Plan {
	if !usesMappingBoundExecutionPlan(plan) {
		return nil
	}
	return runtimePlanFromToolExecutionPlan(plan)
}

func runtimeInitialPlanForAssistantWithDescriptors(plan *ToolExecutionPlan, descriptors []agentruntime.ToolDescriptor) *agentruntime.Plan {
	if !usesMappingBoundExecutionPlan(plan) {
		return nil
	}
	return runtimePlanFromToolExecutionPlanWithDescriptors(plan, descriptors)
}

func mappingBoundExecutionPlanForAssistant(plan *ToolExecutionPlan) *ToolExecutionPlan {
	if !usesMappingBoundExecutionPlan(plan) {
		return nil
	}
	return plan
}

func usesMappingBoundExecutionPlan(plan *ToolExecutionPlan) bool {
	if plan == nil || len(plan.Steps) == 0 {
		return false
	}
	for _, step := range plan.Steps {
		if strings.TrimSpace(step.StepID) == "" || strings.TrimSpace(step.ToolName) == "" {
			return false
		}
	}
	return true
}

func usesDeterministicBaselineWorkflow(plan *ToolExecutionPlan) bool {
	if plan == nil {
		return false
	}
	for _, step := range plan.Steps {
		if step.ToolName != "Baseline.Compliance.Run" {
			continue
		}
		templateSelector, hasTemplate := step.Args["template_selector"].(string)
		_, hasTargetScope := step.Args["target_scope"].(string)
		_, hasSelectors := step.Args["host_selectors"]
		return hasTemplate && strings.TrimSpace(templateSelector) != "" && (hasTargetScope || hasSelectors)
	}
	return false
}

// runtimePlanFromToolExecutionPlan converts the backend Mapping result into the
// authoritative runtime plan. Each business step allows exactly one mapped
// primary tool plus, when declared, that primary's already-mapped completion
// tool. The model cannot add, replace, or reorder tools after Mapping.
func runtimePlanFromToolExecutionPlan(plan *ToolExecutionPlan) *agentruntime.Plan {
	return runtimePlanFromToolExecutionPlanWithDescriptors(plan, nil)
}

// runtimePlanFromToolExecutionPlanWithDescriptors compiles the Mapping artifact
// into executable business steps. A registered completion tool is attached to
// its mapped asynchronous producer step instead of becoming an unreachable
// downstream step: agent-runtime requires terminal evidence before completing
// a tool-backed step, so the completion tool must remain inside that same
// backend-authorized boundary.
func runtimePlanFromToolExecutionPlanWithDescriptors(plan *ToolExecutionPlan, descriptors []agentruntime.ToolDescriptor) *agentruntime.Plan {
	if plan == nil || len(plan.Steps) == 0 {
		return nil
	}
	stepBindings := buildRuntimeStepToolBindings(plan, descriptors)
	attachedCompletionTools := attachedRuntimeCompletionTools(plan, descriptors)
	steps := make([]agentruntime.PlanStep, 0, len(plan.Steps))
	titleCounts := make(map[string]int)
	for _, source := range plan.Steps {
		if strings.TrimSpace(source.ToolName) == "" {
			continue
		}
		if attachedCompletionTools[source.ToolName] {
			continue
		}
		dependencies := []string(nil)
		if len(steps) > 0 {
			dependencies = []string{steps[len(steps)-1].StepID}
		}
		allowedTools := append([]string{}, stepBindings[source.StepID]...)
		if len(allowedTools) == 0 {
			allowedTools = []string{source.ToolName}
		}
		title, objective, expected := runtimeBusinessStepContract(source, allowedTools)
		if source.Condition != "" && !strings.Contains(objective, source.Condition) {
			objective += " Execution condition: " + source.Condition
		}
		titleCounts[title]++
		if titleCounts[title] > 1 {
			title = fmt.Sprintf("%s (%d)", title, titleCounts[title])
		}
		toolArgs := cloneToolPlanArgs(source.Args)
		if len(allowedTools) > 1 {
			// Bound arguments differ between the producer and its completion
			// tool. The Aegis gateway applies the immutable per-tool Mapping
			// arguments and previous-step references after the model copies the
			// currently allowed tool name.
			toolArgs = nil
		}
		steps = append(steps, agentruntime.PlanStep{
			StepID:         source.StepID,
			Title:          title,
			Objective:      objective,
			ExpectedOutput: expected,
			SuggestedTools: []string{source.ToolName},
			AllowedTools:   allowedTools,
			ToolArgs:       toolArgs,
			Dependencies:   dependencies,
			CreatedBy:      "aegis_tool_decision",
			RiskLevel:      runtimeRiskLevel(source.Risk),
		})
	}
	if len(steps) == 0 {
		return nil
	}
	return &agentruntime.Plan{
		PlanID:    plan.DecisionTraceID,
		Version:   1,
		Goal:      plan.Goal,
		NeedsPlan: true,
		EstSteps:  len(steps),
		Steps:     steps,
	}
}

func buildRuntimeStepToolBindings(plan *ToolExecutionPlan, descriptors []agentruntime.ToolDescriptor) map[string][]string {
	if plan == nil {
		return nil
	}
	mappedTools := make(map[string]bool, len(plan.Steps))
	for _, step := range plan.Steps {
		mappedTools[step.ToolName] = true
	}
	descriptorByName := make(map[string]agentruntime.ToolDescriptor, len(descriptors))
	for _, descriptor := range descriptors {
		descriptorByName[descriptor.Name] = descriptor
	}
	bindings := make(map[string][]string, len(plan.Steps))
	for _, step := range plan.Steps {
		allowed := []string{step.ToolName}
		seen := map[string]bool{step.ToolName: true}
		for _, completionTool := range descriptorByName[step.ToolName].CompletionTools {
			if !mappedTools[completionTool] || seen[completionTool] {
				continue
			}
			seen[completionTool] = true
			allowed = append(allowed, completionTool)
		}
		bindings[step.StepID] = allowed
	}
	return bindings
}

func attachedRuntimeCompletionTools(plan *ToolExecutionPlan, descriptors []agentruntime.ToolDescriptor) map[string]bool {
	result := make(map[string]bool)
	if plan == nil {
		return result
	}
	mappedTools := make(map[string]bool, len(plan.Steps))
	for _, step := range plan.Steps {
		mappedTools[step.ToolName] = true
	}
	for _, descriptor := range descriptors {
		if !mappedTools[descriptor.Name] {
			continue
		}
		for _, completionTool := range descriptor.CompletionTools {
			if mappedTools[completionTool] {
				result[completionTool] = true
			}
		}
	}
	return result
}

func runtimeBusinessStepContract(source ToolPlanStep, allowedTools []string) (string, string, string) {
	switch source.Capability {
	case "resolve_hosts":
		return "Resolve target hosts",
			"Resolve only the requested host scope and verify that every selected target is currently online. Do not start scans, remediation, or later plan actions in this step.",
			"Return terminal Host.Resolve evidence with resolved host IDs and coverage. Once that terminal evidence exists, immediately complete the current step."
	case "run_baseline_compliance":
		return "Run and verify baseline compliance",
			"Start the mapped baseline compliance operation with the backend-bound scope, template, and remediation policy. Then use only the mapped Operation.Get completion tool with the real returned operation_id until the operation reaches a terminal state.",
			"Return terminal operation evidence with coverage, task references, and remediation results. Complete this step only after the mapped completion tool reports a terminal outcome."
	case "trigger_asset_collection":
		return "Refresh and verify asset inventory",
			"Start the mapped asset collection operation with the backend-bound scope. Then use only the mapped Asset.Collection.Get completion tool with the real returned task_id until the task reaches a terminal state (completed, failed, or cancelled).",
			"Return terminal asset collection evidence with task_id, host coverage (total/success/failed), and terminal status. Complete this step only after the mapped completion tool reports a terminal outcome."
	case "get_operation_status":
		return "Monitor operation status",
			"Query only the mapped operation reference from prior tool evidence until the operation reaches a terminal state.",
			"Return terminal operation status and its verified side-effect references, then immediately complete the current step."
	}

	title := humanizeCapability(source.Capability)
	if title == "" {
		title = "Execute mapped capability"
	}
	objective := "Execute only the capability bound to the current Mapping step. Do not attempt tools assigned to later steps."
	expected := "Return the actual terminal evidence for the current mapped capability, then immediately complete the current step."
	if len(allowedTools) > 1 {
		objective = "Execute the mapped primary capability, then use only its mapped completion tool with the real operation reference until terminal."
		expected = "Return terminal evidence from the mapped completion tool, then immediately complete the current step."
	}
	return title, objective, expected
}

func humanizeCapability(capability string) string {
	words := strings.Fields(strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(strings.TrimSpace(capability)))
	for index := range words {
		if words[index] == "" {
			continue
		}
		words[index] = strings.ToUpper(words[index][:1]) + words[index][1:]
	}
	return strings.Join(words, " ")
}

func runtimePlanStepTitle(source ToolPlanStep, _ int) string {
	title, _, _ := runtimeBusinessStepContract(source, []string{source.ToolName})
	if scriptType, ok := source.Args["script_type"].(string); ok && strings.TrimSpace(scriptType) != "" {
		title += " (" + strings.ToLower(strings.TrimSpace(scriptType)) + ")"
	}
	return title
}

func cloneToolPlanArgs(args map[string]interface{}) map[string]interface{} {
	if len(args) == 0 {
		return nil
	}
	result := make(map[string]interface{}, len(args))
	for key, value := range args {
		result[key] = value
	}
	return result
}

func runtimeRiskLevel(risk string) agentruntime.RiskLevel {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case string(ToolRiskReadonly):
		return agentruntime.RiskReadOnly
	case string(ToolRiskLow):
		return agentruntime.RiskLow
	case string(ToolRiskCritical):
		return agentruntime.RiskDangerous
	default:
		return agentruntime.RiskHigh
	}
}

// normalizeBlockedMappingPlanSteps closes an agent-runtime status gap for
// caller-provided plans. The runtime correctly refuses to execute a step whose
// dependency failed, but currently leaves that step pending when the loop
// finishes. Aegis converts only those transitively blocked steps to skipped so
// persisted plans never imply that more tool calls are still scheduled.
func normalizeBlockedMappingPlanSteps(result *agentruntime.TaskResult) []agentruntime.PlanStep {
	if result == nil || result.FinalPlan == nil || len(result.FinalPlan.Steps) == 0 {
		return nil
	}
	statusByID := make(map[string]agentruntime.StepStatus, len(result.FinalPlan.Steps))
	for _, step := range result.FinalPlan.Steps {
		statusByID[step.StepID] = step.Status
	}

	blockedIDs := make(map[string]bool)
	skipped := make([]agentruntime.PlanStep, 0)
	for changed := true; changed; {
		changed = false
		for index := range result.FinalPlan.Steps {
			step := &result.FinalPlan.Steps[index]
			switch step.Status {
			case agentruntime.StepPending, agentruntime.StepRetrying, agentruntime.StepWaitingTool:
			default:
				continue
			}
			blocked := false
			for _, dependencyID := range step.Dependencies {
				if statusByID[dependencyID] == agentruntime.StepFailed || blockedIDs[dependencyID] {
					blocked = true
					break
				}
			}
			if !blocked {
				continue
			}
			step.Status = agentruntime.StepSkipped
			statusByID[step.StepID] = agentruntime.StepSkipped
			blockedIDs[step.StepID] = true
			skipped = append(skipped, *step)
			changed = true
		}
	}
	return skipped
}
