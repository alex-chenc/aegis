package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dc/internal/model"

	"github.com/google/uuid"
)

type agentGuardEscapeEnvelope struct {
	Schema          string         `json:"schema"`
	EventID         string         `json:"event_id"`
	EventType       string         `json:"event_type"`
	HostID          string         `json:"host_id"`
	InstanceID      string         `json:"instance_id"`
	SessionID       string         `json:"session_id"`
	ExecutionUnitID string         `json:"execution_unit_id"`
	Agent           map[string]any `json:"agent"`
	ExecutionUnit   map[string]any `json:"execution_unit"`
	OccurredAt      string         `json:"occurred_at"`
	Decision        string         `json:"decision"`
	Severity        string         `json:"severity"`
	RuleID          string         `json:"rule_id"`
	Operation       string         `json:"operation"`
	Violation       struct {
		Rule             string         `json:"rule"`
		Operation        string         `json:"operation"`
		Baseline         map[string]any `json:"baseline"`
		Actual           map[string]any `json:"actual"`
		Diff             map[string]any `json:"diff"`
		StateChanged     bool           `json:"state_changed"`
		ReturnCode       *int64         `json:"return_code"`
		Decision         string         `json:"decision"`
		Severity         string         `json:"severity"`
		EvidenceEventIDs []string       `json:"evidence_event_ids"`
	} `json:"violation"`
	Isolation        map[string]any `json:"isolation"`
	Evidence         map[string]any `json:"evidence"`
	EvidenceEventIDs []string       `json:"evidence_event_ids"`
}

var allowedEscapeRules = map[string]struct{}{
	"join_external_namespace":         {},
	"leave_expected_cgroup":           {},
	"access_container_runtime_socket": {},
	"access_host_proc_root":           {},
	"write_cgroupfs":                  {},
	"mount_host_sensitive_path":       {},
	"ptrace_external_process":         {},
	"ptrace_aegis_agent":              {},
	"load_bpf_or_module":              {},
	"capability_escalation":           {},
	"credential_or_capability_gain":   {},
	"isolation_baseline_drift":        {},
}

func NormalizeAgentGuardEscapeFinding(event *model.RuntimeEvent) (*model.AgentSecurityFinding, error) {
	if event == nil || !allowedValue(event.EventType, "agent_sandbox_violation", "agent_isolation_drift") ||
		len(event.EventData) > maxAgentGuardEvent {
		return nil, fmt.Errorf("%w: escape event type or size", ErrAgentBehaviorInvalidContract)
	}
	var envelope agentGuardEscapeEnvelope
	if err := json.Unmarshal([]byte(event.EventData), &envelope); err != nil ||
		envelope.Schema != agentBehaviorSchema {
		return nil, fmt.Errorf("%w: escape schema", ErrAgentBehaviorInvalidContract)
	}
	if envelope.EventType != "" && envelope.EventType != event.EventType {
		return nil, fmt.Errorf("%w: escape event type mismatch", ErrAgentBehaviorInvalidContract)
	}
	if envelope.EventID != "" && envelope.EventID != event.EventID {
		return nil, fmt.Errorf("%w: escape event_id mismatch", ErrAgentBehaviorInvalidContract)
	}
	if envelope.HostID != "" {
		hostID, parseErr := uuid.Parse(envelope.HostID)
		if parseErr != nil || hostID != event.HostID {
			return nil, fmt.Errorf("%w: escape host_id mismatch", ErrAgentBehaviorInvalidContract)
		}
	}
	instanceID, unitID, sessionID, err := escapeScope(envelope)
	if err != nil {
		return nil, err
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, envelope.OccurredAt)
	if err != nil {
		return nil, fmt.Errorf("%w: escape occurred_at", ErrAgentBehaviorInvalidContract)
	}
	rule := firstString(
		strings.TrimSpace(envelope.Violation.Rule),
		strings.TrimSpace(envelope.RuleID),
		stringValueAny(envelope.Evidence["rule"]),
	)
	if event.EventType == "agent_isolation_drift" && rule == "" {
		rule = "isolation_baseline_drift"
	}
	if _, exists := allowedEscapeRules[rule]; !exists {
		return nil, fmt.Errorf("%w: escape rule", ErrAgentBehaviorInvalidContract)
	}
	operation := firstString(
		strings.TrimSpace(envelope.Violation.Operation),
		stringValueAny(envelope.Evidence["operation"]),
		strings.TrimSuffix(envelope.Operation, "_violation"),
	)
	if operation == "" || len(operation) > 64 {
		return nil, fmt.Errorf("%w: escape operation", ErrAgentBehaviorInvalidContract)
	}
	decision := firstString(envelope.Violation.Decision, envelope.Decision)
	if decision == "" {
		decision = "audit"
	}
	if !allowedValue(decision, "audit", "would_deny", "enforcement_unavailable") {
		return nil, ErrAgentBehaviorActiveDecision
	}
	severity := firstString(envelope.Violation.Severity, envelope.Severity)
	if severity == "" {
		severity = escapeSeverity(rule, event.EventType)
	}
	if !allowedValue(severity, "medium", "high", "critical") {
		return nil, fmt.Errorf("%w: escape severity", ErrAgentBehaviorInvalidContract)
	}
	evidenceIDs := append([]string{event.EventID}, envelope.EvidenceEventIDs...)
	evidenceIDs = append(evidenceIDs, envelope.Violation.EvidenceEventIDs...)
	evidenceIDs = append(evidenceIDs, stringSliceAny(envelope.Evidence["evidence_event_ids"])...)
	evidenceIDs = uniqueSortedStrings(evidenceIDs)
	for _, evidenceID := range evidenceIDs {
		if _, parseErr := uuid.Parse(evidenceID); parseErr != nil {
			return nil, fmt.Errorf("%w: escape evidence ID", ErrAgentBehaviorInvalidContract)
		}
	}
	if !validIsolationEvidence(envelope) {
		return nil, fmt.Errorf("%w: escape isolation evidence", ErrAgentBehaviorInvalidContract)
	}
	stateChanged := envelope.Violation.StateChanged || boolValue(envelope.Evidence["state_changed"])
	findingKey := "escape:v1:" + event.EventType + ":" + event.EventID
	verdict := "suspicious"
	confidence := 0.90
	if decision == "enforcement_unavailable" {
		verdict, confidence = "inconclusive", 0.62
	}
	ruleHit := map[string]any{
		"rule_key":      rule,
		"event_id":      event.EventID,
		"operation":     truncateLimit(operation, 64),
		"state_changed": stateChanged,
		"decision":      decision,
	}
	return &model.AgentSecurityFinding{
		ID:                  stableFindingID(findingKey),
		FindingKey:          findingKey,
		HostID:              event.HostID,
		InstanceID:          instanceID,
		SessionID:           sessionID,
		ExecutionUnitID:     unitID,
		Title:               escapeTitle(event.EventType, rule),
		Severity:            severity,
		Verdict:             verdict,
		Confidence:          confidence,
		Status:              "open",
		DecisionSources:     mustJSON([]string{"agent_guard_rule", decision}, []string{}),
		RuleHits:            mustJSON([]map[string]any{ruleHit}, []map[string]any{}),
		EvidenceEventIDs:    mustJSON(evidenceIDs, []string{}),
		EvidenceGraph:       evidenceGraph(evidenceIDs, nil, rule),
		AttackStages:        mustJSON([]string{"privilege_escalation", "defense_evasion"}, []string{}),
		Summary:             "Agent Guard observed a sandbox violation or isolation drift signal.",
		RecommendedAction:   "alert",
		FirstObservedAt:     occurredAt,
		LastObservedAt:      occurredAt,
		EvidenceSourceTable: "agent_guard_events",
	}, nil
}

func escapeScope(envelope agentGuardEscapeEnvelope) (*uuid.UUID, *uuid.UUID, *uuid.UUID, error) {
	instanceRaw := firstString(
		envelope.InstanceID,
		stringValueAny(envelope.Agent["instance_id"]),
		stringValueAny(envelope.ExecutionUnit["instance_id"]),
	)
	unitRaw := firstString(
		envelope.ExecutionUnitID,
		stringValueAny(envelope.ExecutionUnit["execution_unit_id"]),
		stringValueAny(envelope.ExecutionUnit["id"]),
	)
	sessionRaw := firstString(
		envelope.SessionID,
		stringValueAny(envelope.Agent["session_id"]),
		stringValueAny(envelope.ExecutionUnit["session_id"]),
	)
	instanceID, err := optionalUUID(instanceRaw)
	if err != nil || instanceID == nil {
		return nil, nil, nil, fmt.Errorf("%w: escape instance_id", ErrAgentBehaviorInvalidContract)
	}
	unitID, err := optionalUUID(unitRaw)
	if err != nil || unitID == nil {
		return nil, nil, nil, fmt.Errorf("%w: escape execution_unit_id", ErrAgentBehaviorInvalidContract)
	}
	sessionID, err := optionalUUID(sessionRaw)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: escape session_id", ErrAgentBehaviorInvalidContract)
	}
	return instanceID, unitID, sessionID, nil
}

func validIsolationEvidence(envelope agentGuardEscapeEnvelope) bool {
	if envelope.Isolation == nil || envelope.Evidence == nil {
		return false
	}
	for _, key := range []string{"baseline", "actual", "diff"} {
		if _, ok := envelope.Isolation[key].(map[string]any); ok {
			continue
		}
		if _, ok := envelope.Evidence[key].(map[string]any); !ok {
			return false
		}
	}
	return true
}

func stringSliceAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func escapeSeverity(rule, eventType string) string {
	if eventType == "agent_isolation_drift" {
		return "high"
	}
	switch rule {
	case "join_external_namespace", "access_container_runtime_socket",
		"mount_host_sensitive_path", "capability_escalation", "credential_or_capability_gain":
		return "critical"
	default:
		return "high"
	}
}

func escapeTitle(eventType, rule string) string {
	if eventType == "agent_isolation_drift" {
		return "Agent isolation drift: " + rule
	}
	return "Agent sandbox violation: " + rule
}
