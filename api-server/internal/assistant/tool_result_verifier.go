package assistant

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// ToolResultVerifyResult holds the outcome of a postcondition verification.
type ToolResultVerifyResult struct {
	Passed      bool                   `json:"passed"`
	Violations  []PostconditionViolation `json:"violations,omitempty"`
	Evidence    map[string]interface{}  `json:"evidence,omitempty"`
	Reason      string                  `json:"reason,omitempty"`
}

// PostconditionViolation describes a single postcondition that was not met.
type PostconditionViolation struct {
	Postcondition string `json:"postcondition"`
	Reason        string `json:"reason"`
}

// ToolResultVerifier checks tool execution results against postconditions
// defined in the tool's contract.
type ToolResultVerifier struct {
	logger *zap.Logger
}

// NewToolResultVerifier creates a new verifier.
func NewToolResultVerifier(logger *zap.Logger) *ToolResultVerifier {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ToolResultVerifier{logger: logger}
}

// Verify checks whether the tool execution result satisfies the postconditions
// declared in the plan step.
func (v *ToolResultVerifier) Verify(ctx context.Context, step ToolPlanStep, result ToolExecutionResult) *ToolResultVerifyResult {
	_ = ctx
	if len(step.Postconditions) == 0 {
		return &ToolResultVerifyResult{Passed: true}
	}

	violations := make([]PostconditionViolation, 0)
	evidence := make(map[string]interface{})

	for _, postcondition := range step.Postconditions {
		passed, reason := v.checkPostcondition(postcondition, step, result)
		if !passed {
			violations = append(violations, PostconditionViolation{
				Postcondition: postcondition,
				Reason:        reason,
			})
		}
		evidence[postcondition] = passed
	}

	verifyResult := &ToolResultVerifyResult{
		Passed:     len(violations) == 0,
		Violations: violations,
		Evidence:   evidence,
	}

	if !verifyResult.Passed {
		verifyResult.Reason = fmt.Sprintf("tool %s postcondition check failed: %d violation(s)", step.ToolName, len(violations))
		v.logger.Warn("tool result postcondition failed",
			zap.String("tool_name", step.ToolName),
			zap.String("step_id", step.StepID),
			zap.Int("violations", len(violations)),
		)
	}
	return verifyResult
}

// checkPostcondition evaluates a single postcondition against the tool result.
func (v *ToolResultVerifier) checkPostcondition(postcondition string, step ToolPlanStep, result ToolExecutionResult) (bool, string) {
	if !result.Success {
		return false, fmt.Sprintf("tool execution failed: %s", result.Error)
	}

	switch postcondition {
	case "task_id_created":
		return checkResultHasField(result, "task_id")
	case "asset_collection_task_id_present":
		return checkResultHasField(result, "task_id")
	case "block_action_created":
		if ok, _ := checkResultHasField(result, "action_id"); ok {
			return true, ""
		}
		return checkResultHasField(result, "block_id")
	case "block_action_result_present":
		if result.Data != nil {
			return true, ""
		}
		return false, "result data is nil"
	case "script_generated":
		if result.Data != nil {
			return true, ""
		}
		return false, "no script data in result"
	default:
		// 未知后置条件默认通过，不阻断执行
		v.logger.Debug("unknown postcondition, defaulting to passed",
			zap.String("postcondition", postcondition),
			zap.String("tool_name", step.ToolName),
		)
		return true, ""
	}
}

// checkResultHasField checks if the tool result data contains a non-empty field.
func checkResultHasField(result ToolExecutionResult, fieldName string) (bool, string) {
	if result.Data == nil {
		return false, fmt.Sprintf("result data is nil, expected field %q", fieldName)
	}
	dataMap, ok := result.Data.(map[string]interface{})
	if !ok {
		return false, fmt.Sprintf("result data is not a map, expected field %q", fieldName)
	}
	value, exists := dataMap[fieldName]
	if !exists {
		return false, fmt.Sprintf("result missing required field %q", fieldName)
	}
	if value == nil {
		return false, fmt.Sprintf("result field %q is nil", fieldName)
	}
	if s, ok := value.(string); ok && s == "" {
		return false, fmt.Sprintf("result field %q is empty string", fieldName)
	}
	return true, ""
}
