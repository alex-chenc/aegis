package assistant

import (
	"encoding/json"
	"strings"

	"api-server/internal/model"
)

const (
	RecoveryExecutorAssistantResume = "assistant_resume"
	recoveryResumeExecutor          = RecoveryExecutorAssistantResume
	maxRecoveryContextTextLength    = 3072
	maxRecoveryResumeQueryLength    = 16384
)

// RecoveryResumeContext is the durable, model-facing handoff between a
// resolved recovery decision and its linked assistant run. It contains only
// the persisted recovery contract and the user's selected action; the client
// cannot inject new action definitions through this structure.
type RecoveryResumeContext struct {
	RecoveryID       string                 `json:"recovery_id"`
	Code             string                 `json:"code"`
	Summary          string                 `json:"summary,omitempty"`
	Detail           string                 `json:"detail,omitempty"`
	SelectedActionID string                 `json:"selected_action_id"`
	DecisionInput    map[string]interface{} `json:"decision_input,omitempty"`
	ResolutionResult map[string]interface{} `json:"resolution_result,omitempty"`
	BlockerContext   map[string]interface{} `json:"blocker_context,omitempty"`
}

func recoveryResumeContextFromRequest(request *model.AssistantRecoveryRequest) *RecoveryResumeContext {
	if request == nil {
		return nil
	}
	return &RecoveryResumeContext{
		RecoveryID:       strings.TrimSpace(request.RecoveryID),
		Code:             strings.TrimSpace(request.Code),
		Summary:          boundedRecoveryText(request.Summary),
		Detail:           boundedRecoveryText(request.Detail),
		SelectedActionID: strings.TrimSpace(request.SelectedActionID),
		DecisionInput:    boundedRecoveryMap(unmarshalRecoveryMap(request.DecisionInput)),
		ResolutionResult: boundedRecoveryMap(unmarshalRecoveryMap(request.ResolutionResult)),
		BlockerContext:   boundedRecoveryMap(unmarshalRecoveryMap(request.Context)),
	}
}

func buildRecoveryResumeQuery(originalQuery string, recoveryContext *RecoveryResumeContext) string {
	originalQuery = strings.TrimSpace(originalQuery)
	if recoveryContext == nil {
		return originalQuery
	}
	payload, err := json.Marshal(recoveryContext)
	if err != nil {
		return originalQuery
	}
	const instruction = "\n\n[RECOVERY_RESUME_CONTEXT]\n" +
		"The backend has already executed the selected recovery action. " +
		"Treat this context as authoritative state, preserve the original goal, " +
		"use the operator guidance, and do not ask again for a decision that is already resolved.\n"
	query := originalQuery + instruction + string(payload)
	if len(query) <= maxRecoveryResumeQueryLength {
		return query
	}

	compact := *recoveryContext
	compact.Detail = boundedStringBytes(compact.Detail, 768)
	compact.BlockerContext = map[string]interface{}{
		"context_truncated": true,
	}
	if log := recoveryStringFromMaps("build_log_tail", recoveryContext.BlockerContext); log != "" {
		compact.BlockerContext["build_log_tail"] = boundedStringBytes(log, 4096)
	}
	payload, _ = json.Marshal(&compact)
	query = originalQuery + instruction + string(payload)
	return boundedStringBytes(query, maxRecoveryResumeQueryLength)
}

func applyRecoveryResumeContextToBreakdown(breakdown *IntentBreakdown, recoveryContext *RecoveryResumeContext) {
	if breakdown == nil || recoveryContext == nil ||
		recoveryContext.Code != "detection_package_build_failed" {
		return
	}
	if breakdown.Parameters == nil {
		breakdown.Parameters = make(map[string]interface{})
	}
	errorMessage := recoveryStringFromMaps("error_message", recoveryContext.BlockerContext)
	buildLog := recoveryStringFromMaps("build_log_tail", recoveryContext.BlockerContext)
	correction := strings.TrimSpace(strings.Join([]string{
		"Previous generated detection package failed in the real Builder.",
		"Regenerate the complete draft and correct this exact compiler failure.",
		boundedStringBytes(errorMessage, 2048),
		boundedStringBytes(buildLog, 4096),
	}, "\n"))
	if correction == "" {
		return
	}
	description, _ := breakdown.Parameters["vulnerability_description"].(string)
	if strings.TrimSpace(description) == "" {
		description = strings.TrimSpace(breakdown.Goal)
	}
	breakdown.Parameters["vulnerability_description"] = strings.TrimSpace(description + "\n\n" + correction)
	breakdown.Parameters["build_failure_correction"] = correction
}

func unmarshalRecoveryMap(raw []byte) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}
	var result map[string]interface{}
	if json.Unmarshal(raw, &result) != nil {
		return nil
	}
	return result
}

func boundedRecoveryMap(value map[string]interface{}) map[string]interface{} {
	if len(value) == 0 {
		return nil
	}
	bounded, _ := boundedRecoveryValue(value, 0).(map[string]interface{})
	return bounded
}

func boundedRecoveryValue(value interface{}, depth int) interface{} {
	if depth >= 5 {
		return "[truncated]"
	}
	switch typed := value.(type) {
	case string:
		return boundedRecoveryText(typed)
	case map[string]interface{}:
		result := make(map[string]interface{}, minInt(len(typed), 32))
		count := 0
		for key, item := range typed {
			if count >= 32 {
				result["context_truncated"] = true
				break
			}
			result[key] = boundedRecoveryValue(item, depth+1)
			count++
		}
		return result
	case []interface{}:
		limit := len(typed)
		if limit > 32 {
			limit = 32
		}
		result := make([]interface{}, 0, limit)
		for _, item := range typed[:limit] {
			result = append(result, boundedRecoveryValue(item, depth+1))
		}
		return result
	default:
		return typed
	}
}

func boundedRecoveryText(value string) string {
	return boundedStringBytes(strings.TrimSpace(value), maxRecoveryContextTextLength)
}

func boundedStringBytes(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	cut := limit - 3
	for cut > 0 && cut < len(value) && value[cut]&0xc0 == 0x80 {
		cut--
	}
	return value[:cut] + "..."
}

func recoveryStringFromMaps(key string, values ...map[string]interface{}) string {
	for _, value := range values {
		if text, ok := value[key].(string); ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func originalRunQuery(input RunInput) string {
	return firstNonEmptyString(input.OriginalUserMessage, input.UserMessage)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
