package assistant

import "time"

const assistantRuntimeEventsMetadataKey = "assistant_runtime_events"

type assistantRuntimeDisplayEvent struct {
	Type      string                 `json:"type"`
	RunID     string                 `json:"run_id,omitempty"`
	MessageID string                 `json:"message_id,omitempty"`
	Timestamp string                 `json:"timestamp,omitempty"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
}

func compactRuntimeDisplayEvents(events []AssistantEvent, runID, messageID string) []assistantRuntimeDisplayEvent {
	result := make([]assistantRuntimeDisplayEvent, 0, len(events))
	for _, event := range events {
		if runID != "" && event.RunID != "" && event.RunID != runID {
			continue
		}
		if messageID != "" && event.MessageID != "" && event.MessageID != messageID {
			continue
		}

		payload, ok := compactRuntimeDisplayPayload(event)
		if !ok {
			continue
		}
		result = append(result, assistantRuntimeDisplayEvent{
			Type:      event.Type,
			RunID:     event.RunID,
			MessageID: event.MessageID,
			Timestamp: formatRuntimeEventTime(event.Timestamp),
			Payload:   payload,
		})
	}
	return result
}

func compactRuntimeDisplayPayload(event AssistantEvent) (map[string]interface{}, bool) {
	payload := toMap(event.Payload)
	switch event.Type {
	case EventThinking:
		content, _ := payload["content"].(string)
		if content == "" {
			content, _ = payload["message"].(string)
		}
		if content == "" {
			return nil, false
		}
		result := map[string]interface{}{"content": content}
		if callID, _ := payload["call_id"].(string); callID != "" {
			result["call_id"] = callID
		}
		if toolName, _ := payload["tool_name"].(string); toolName != "" {
			result["tool_name"] = toolName
		}
		return result, true
	case EventToolCall:
		callID, _ := payload["call_id"].(string)
		toolName, _ := payload["tool_name"].(string)
		if callID == "" && toolName == "" {
			return nil, false
		}
		return map[string]interface{}{
			"call_id":   callID,
			"tool_name": toolName,
		}, true
	case EventToolResult:
		callID, _ := payload["call_id"].(string)
		if callID == "" {
			return nil, false
		}
		result := map[string]interface{}{
			"call_id": callID,
			"result":  payload["result"],
		}
		for _, key := range []string{
			"transport_status",
			"operation_status",
			"terminal",
			"outcome_message",
			"capability",
			"outcome",
		} {
			if value, exists := payload[key]; exists {
				result[key] = value
			}
		}
		return result, true
	case EventToolError:
		callID, _ := payload["call_id"].(string)
		if callID == "" {
			return nil, false
		}
		result := map[string]interface{}{
			"call_id": callID,
			"error":   payload["error"],
		}
		for _, key := range []string{"tool_name", "validation_stage"} {
			if value, exists := payload[key]; exists {
				result[key] = value
			}
		}
		return result, true
	case EventPlan:
		plan := normalizePlanEventPayload(event.Payload)
		if len(plan) == 0 {
			return nil, false
		}
		return plan, true
	case EventStepStarted, EventStepCompleted, EventStepFailed, EventStepRetrying:
		result := make(map[string]interface{})
		for _, key := range []string{"step_id", "id", "title", "status", "result_summary", "summary", "error"} {
			value, exists := payload[key]
			if !exists || value == nil {
				continue
			}
			if str, ok := value.(string); ok && str == "" {
				continue
			}
			result[key] = value
		}
		if _, ok := result["step_id"]; !ok {
			if id, ok := result["id"]; ok {
				result["step_id"] = id
			}
		}
		if len(result) == 0 {
			return nil, false
		}
		return result, true
	default:
		return nil, false
	}
}

func formatRuntimeEventTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339Nano)
}
