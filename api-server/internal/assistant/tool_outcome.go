package assistant

import (
	"encoding/json"
	"fmt"
	"strings"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

// normalizeToolOutcome converts arbitrary tool data into a domain-neutral
// business outcome using the tool's declarative result contract.
func normalizeToolOutcome(tool *ToolSpec, data interface{}) *agentruntime.ToolOutcome {
	outcome := &agentruntime.ToolOutcome{
		OperationStatus: agentruntime.OperationSucceeded,
		Terminal:        true,
	}
	if tool == nil {
		return outcome
	}
	contract := tool.ResultContract
	capability := BuildToolUseContract(tool).Capability
	outcome.Capability = capability
	payload := resultMap(data)

	if contract.AcceptedOnSuccess || tool.ExecutionContract.Mode == ToolExecutionAsynchronous {
		outcome.OperationStatus = agentruntime.OperationAccepted
		outcome.Terminal = false
	}
	if contract.OperationStatusField != "" {
		state := strings.ToLower(strings.TrimSpace(resultStringAtPath(payload, contract.OperationStatusField)))
		switch {
		case state == "":
			// A status endpoint without the declared state field is not valid
			// terminal evidence. Treat the malformed business result as failed
			// instead of inheriting the synchronous-success default.
			outcome.OperationStatus = agentruntime.OperationFailed
			outcome.Terminal = true
		case stringInFold(state, contract.SuccessValues):
			outcome.OperationStatus = agentruntime.OperationSucceeded
			outcome.Terminal = true
		case stringInFold(state, contract.FailureValues):
			outcome.OperationStatus = agentruntime.OperationFailed
			outcome.Terminal = true
		case stringInFold(state, contract.PendingValues):
			outcome.OperationStatus = agentruntime.OperationRunning
			outcome.Terminal = false
		default:
			outcome.OperationStatus = agentruntime.OperationRunning
			outcome.Terminal = false
		}
	}

	outcome.OperationRef = evidenceRef(capability, contract.OperationRefFields, payload)
	if outcome.Terminal && outcome.OperationStatus == agentruntime.OperationSucceeded && len(contract.ArtifactRefFields) > 0 {
		if resultStringAtPath(payload, contract.ArtifactRefFields[0]) == "" {
			outcome.OperationStatus = agentruntime.OperationFailed
		} else if ref := evidenceRef("artifact", contract.ArtifactRefFields, payload); len(ref) > 1 {
			outcome.Artifacts = append(outcome.Artifacts, stringMapToAny(ref))
		}
	}
	if outcome.Terminal && outcome.OperationStatus == agentruntime.OperationSucceeded && len(contract.SideEffectRefFields) > 0 {
		if resultStringAtPath(payload, contract.SideEffectRefFields[0]) == "" {
			outcome.OperationStatus = agentruntime.OperationFailed
		} else if ref := evidenceRef("side_effect", contract.SideEffectRefFields, payload); len(ref) > 1 {
			outcome.SideEffects = append(outcome.SideEffects, stringMapToAny(ref))
		}
	}
	outcome.Facts = extractToolFacts(contract.FactBindings, payload)

	if outcome.Terminal && (outcome.OperationStatus == agentruntime.OperationSucceeded || outcome.OperationStatus == agentruntime.OperationSkipped) {
		outcome.SatisfiesCapabilities = dedupeStrings(append([]string{capability}, contract.SatisfiesCapabilities...))
	}
	outcome.Message = toolOutcomeMessage(outcome)
	return outcome
}

func resultMap(data interface{}) map[string]interface{} {
	// Always normalize through JSON so typed slices and structs become
	// []interface{} / map[string]interface{}. Result contracts must behave the
	// same for handler-native values and values reloaded from persisted JSON.
	encoded, err := json.Marshal(data)
	if err != nil {
		if value, ok := data.(map[string]interface{}); ok {
			return value
		}
		return map[string]interface{}{}
	}
	var value map[string]interface{}
	if err := json.Unmarshal(encoded, &value); err != nil {
		return map[string]interface{}{}
	}
	return value
}

func resultValueAtPath(payload map[string]interface{}, path string) interface{} {
	var current interface{} = payload
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current = object[part]
	}
	return current
}

func resultStringAtPath(payload map[string]interface{}, path string) string {
	value := resultValueAtPath(payload, path)
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func stringInFold(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

func evidenceRef(kind string, fields []string, payload map[string]interface{}) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	ref := map[string]string{"type": kind}
	for _, field := range fields {
		if value := strings.TrimSpace(resultStringAtPath(payload, field)); value != "" && value != "<nil>" {
			key := field
			if index := strings.LastIndex(field, "."); index >= 0 {
				key = field[index+1:]
			}
			ref[key] = value
		}
	}
	return ref
}

func stringMapToAny(value map[string]string) map[string]interface{} {
	result := make(map[string]interface{}, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func extractToolFacts(bindings []ToolFactBinding, payload map[string]interface{}) []map[string]interface{} {
	var facts []map[string]interface{}
	for _, binding := range bindings {
		items, _ := resultValueAtPath(payload, binding.ItemsField).([]interface{})
		for _, raw := range items {
			item, _ := raw.(map[string]interface{})
			if item == nil {
				continue
			}
			state := resultStringAtPath(item, binding.StateField)
			if binding.StateValue != "" && !strings.EqualFold(state, binding.StateValue) {
				continue
			}
			fact := map[string]interface{}{"kind": binding.Kind}
			if binding.IDField != "" {
				if id := resultStringAtPath(item, binding.IDField); id != "" {
					fact["id"] = id
				}
			}
			if binding.StateField != "" {
				fact["state"] = state
			}
			facts = append(facts, fact)
		}
	}
	return facts
}

func toolOutcomeMessage(outcome *agentruntime.ToolOutcome) string {
	if outcome == nil {
		return ""
	}
	switch outcome.OperationStatus {
	case agentruntime.OperationAccepted:
		return "The tool request was accepted but has not reached a terminal state."
	case agentruntime.OperationRunning:
		return "The business operation is still running."
	case agentruntime.OperationFailed:
		return "The business operation reached a terminal failure."
	case agentruntime.OperationSkipped:
		return "The business operation was intentionally skipped with terminal evidence."
	default:
		return "The business operation reached a terminal success."
	}
}
