package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dc/internal/model"

	"github.com/google/uuid"
)

type agentGuardActionStatusEnvelope struct {
	Schema          string         `json:"schema"`
	ActionID        string         `json:"action_id"`
	CommandID       string         `json:"command_id"`
	InstanceID      string         `json:"instance_id"`
	ExecutionUnitID string         `json:"execution_unit_id"`
	Action          string         `json:"action"`
	Status          string         `json:"status"`
	Result          map[string]any `json:"result"`
	Method          string         `json:"method"`
	Degraded        bool           `json:"degraded"`
	AutoResume      bool           `json:"auto_resume"`
	Executed        bool           `json:"executed"`
	StateChanged    bool           `json:"state_changed"`
	ErrorCode       string         `json:"error_code"`
	ErrorMessage    string         `json:"error_message"`
	OccurredAt      string         `json:"occurred_at"`
}

func normalizeAgentGuardActionStatus(
	eventType string,
	hostID uuid.UUID,
	raw string,
) (*model.AgentGuardStateProjection, error) {
	var envelope agentGuardActionStatusEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || envelope.Schema != agentGuardSchema {
		return nil, fmt.Errorf("%w: action status schema", ErrAgentBehaviorInvalidContract)
	}
	actionID, err := uuid.Parse(envelope.ActionID)
	if err != nil || strings.TrimSpace(envelope.CommandID) == "" || len(envelope.CommandID) > 100 {
		return nil, fmt.Errorf("%w: action status identity", ErrAgentBehaviorInvalidContract)
	}
	commandActionID, err := actionIDFromCommand(envelope.CommandID)
	if err != nil || commandActionID != actionID {
		return nil, fmt.Errorf("%w: action command identity", ErrAgentBehaviorInvalidContract)
	}
	if !allowedValue(envelope.Action,
		"deny", "freeze_execution_unit", "resume_execution_unit", "hold_execution_unit",
		"kill_execution_unit", "kill_agent_instance", "auto_resume") ||
		!allowedValue(envelope.Status, "pending", "dispatching", "running", "success", "failed", "expired", "cancelled") {
		return nil, fmt.Errorf("%w: action or status", ErrAgentBehaviorInvalidContract)
	}
	instanceID, err := optionalUUID(envelope.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("%w: action instance_id", ErrAgentBehaviorInvalidContract)
	}
	unitID, err := optionalUUID(envelope.ExecutionUnitID)
	if err != nil {
		return nil, fmt.Errorf("%w: action execution_unit_id", ErrAgentBehaviorInvalidContract)
	}
	switch envelope.Action {
	case "kill_agent_instance":
		if instanceID == nil {
			return nil, fmt.Errorf("%w: instance action target", ErrAgentBehaviorInvalidContract)
		}
	case "auto_resume":
		if instanceID == nil || unitID == nil || envelope.Status != "success" {
			return nil, fmt.Errorf("%w: timeout auto-resume target or status", ErrAgentBehaviorInvalidContract)
		}
	default:
		if unitID == nil {
			return nil, fmt.Errorf("%w: unit action target", ErrAgentBehaviorInvalidContract)
		}
	}
	if envelope.Result == nil {
		envelope.Result = map[string]any{}
	}
	// Agent action evidence is emitted as bounded top-level scalar fields. Fold
	// it into result for immutable storage while retaining the wire contract.
	envelope.Result["method"] = truncateLimit(envelope.Method, 64)
	envelope.Result["degraded"] = envelope.Degraded
	envelope.Result["auto_resume"] = envelope.AutoResume
	envelope.Result["executed"] = envelope.Executed
	envelope.Result["state_changed"] = envelope.StateChanged
	if envelope.Status == "success" &&
		!envelope.StateChanged && !envelope.Executed {
		return nil, fmt.Errorf("%w: action success lacks state evidence", ErrAgentBehaviorInvalidContract)
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, envelope.OccurredAt)
	if err != nil {
		return nil, fmt.Errorf("%w: action occurred_at", ErrAgentBehaviorInvalidContract)
	}
	if len(envelope.ErrorCode) > 100 || len(envelope.ErrorMessage) > maxEvidenceString {
		return nil, fmt.Errorf("%w: action error detail", ErrAgentBehaviorInvalidContract)
	}
	action := &model.AgentGuardAction{
		ID: actionID, CommandID: envelope.CommandID, HostID: hostID,
		InstanceID: instanceID, ExecutionUnitID: unitID, Action: envelope.Action,
		Status: envelope.Status, Result: mustJSON(sanitizeValue(envelope.Result, ""), map[string]any{}),
		ErrorCode: truncateLimit(envelope.ErrorCode, 100), ErrorMessage: redactText(envelope.ErrorMessage),
		UpdatedAt: occurredAt, RequestedAt: occurredAt,
	}
	if envelope.Action == "auto_resume" {
		action.Source = "timeout"
		action.Reason = "automatic freeze timeout elapsed"
	}
	if allowedValue(envelope.Status, "success", "failed", "expired", "cancelled") {
		action.CompletedAt = &occurredAt
	}
	return &model.AgentGuardStateProjection{
		EventType: eventType, ObjectID: actionID, Action: action,
	}, nil
}
