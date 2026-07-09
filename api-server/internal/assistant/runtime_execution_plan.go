package assistant

import (
	"fmt"
	"strings"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

// runtimeInitialPlanForAssistant deliberately ignores the authorization
// artifact. In pure-agent mode agent-runtime is the single planner; Aegis only
// constrains the tool set and never supplies a business execution plan.
func runtimeInitialPlanForAssistant(_ *ToolExecutionPlan) *agentruntime.Plan {
	return nil
}

// runtimePlanFromToolExecutionPlan converts the backend-authorized tool plan
// into agent-runtime's authoritative initial plan. Each step is constrained to
// one exact tool while still allowing ReAct to decide whether a conditional
// step should be skipped after reading prior observations.
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
