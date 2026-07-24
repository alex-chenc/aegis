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
// authoritative runtime plan. Each step allows exactly one mapped tool. The
// ReAct tool_name field is only a wire-format copy of this binding; it is not a
// model tool-election input.
func runtimePlanFromToolExecutionPlan(plan *ToolExecutionPlan) *agentruntime.Plan {
	if plan == nil || len(plan.Steps) == 0 {
		return nil
	}
	steps := make([]agentruntime.PlanStep, 0, len(plan.Steps))
	for index, source := range plan.Steps {
		if strings.TrimSpace(source.ToolName) == "" {
			continue
		}
		dependencies := []string(nil)
		if len(steps) > 0 {
			dependencies = []string{steps[len(steps)-1].StepID}
		}
		objective := strings.TrimSpace(source.Reason)
		if objective == "" {
			objective = "Execute " + source.ToolName + " and return the actual result."
		}
		if source.Condition != "" && !strings.Contains(objective, source.Condition) {
			objective += " Execution condition: " + source.Condition
		}
		expected := strings.Join(source.Postconditions, "; ")
		if expected == "" {
			expected = "Return actual tool evidence, or explicitly explain a skip using prior evidence."
		}
		steps = append(steps, agentruntime.PlanStep{
			StepID:         source.StepID,
			Title:          runtimePlanStepTitle(source, index),
			Objective:      objective,
			ExpectedOutput: expected,
			SuggestedTools: []string{source.ToolName},
			AllowedTools:   []string{source.ToolName},
			ToolArgs:       cloneToolPlanArgs(source.Args),
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

func runtimePlanStepTitle(source ToolPlanStep, index int) string {
	title := strings.TrimSpace(source.ToolName)
	if scriptType, ok := source.Args["script_type"].(string); ok && strings.TrimSpace(scriptType) != "" {
		title += " (" + strings.ToLower(strings.TrimSpace(scriptType)) + ")"
	}
	stepLabel := strings.TrimSpace(source.StepID)
	if stepLabel == "" {
		stepLabel = fmt.Sprintf("step_%02d", index+1)
	}
	return fmt.Sprintf("%s [%s]", title, stepLabel)
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
