package assistant

import (
	"encoding/json"

	"gorm.io/datatypes"
)

func extractPlanFromEvents(events []AssistantEvent) datatypes.JSON {
	var plan map[string]interface{}

	for _, event := range events {
		switch event.Type {
		case EventPlan:
			plan = normalizePlanEventPayload(event.Payload)
		case EventStepStarted:
			applyStepStatusToPlan(plan, event.Payload, "running")
		case EventStepCompleted:
			applyStepStatusToPlan(plan, event.Payload, "completed")
		case EventStepFailed:
			applyStepStatusToPlan(plan, event.Payload, "failed")
		case EventStepRetrying:
			applyStepStatusToPlan(plan, event.Payload, "retrying")
		}
	}

	if len(plan) == 0 {
		return nil
	}
	b, err := json.Marshal(plan)
	if err != nil {
		return nil
	}
	return datatypes.JSON(b)
}

func normalizePlanEventPayload(payload interface{}) map[string]interface{} {
	if payload == nil {
		return nil
	}
	var b []byte
	switch v := payload.(type) {
	case datatypes.JSON:
		b = v
	case json.RawMessage:
		b = v
	case []byte:
		b = v
	default:
		var err error
		b, err = json.Marshal(v)
		if err != nil {
			return nil
		}
	}

	var plan map[string]interface{}
	if err := json.Unmarshal(b, &plan); err != nil {
		return nil
	}
	return plan
}

func applyStepStatusToPlan(plan map[string]interface{}, payload interface{}, defaultStatus string) {
	if len(plan) == 0 {
		return
	}
	eventPayload := toMap(payload)
	stepID, _ := eventPayload["step_id"].(string)
	if stepID == "" {
		return
	}
	steps, _ := plan["steps"].([]interface{})
	if len(steps) == 0 {
		return
	}

	status, _ := eventPayload["status"].(string)
	if status == "" {
		status = defaultStatus
	}
	resultSummary, _ := eventPayload["result_summary"].(string)
	title, _ := eventPayload["title"].(string)

	for _, item := range steps {
		step, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		currentID, _ := step["step_id"].(string)
		if currentID == "" {
			currentID, _ = step["id"].(string)
		}
		if currentID != stepID {
			continue
		}
		if status != "" {
			step["status"] = status
		}
		if resultSummary != "" {
			step["result_summary"] = resultSummary
		}
		if title != "" {
			step["title"] = title
		}
		return
	}
}
