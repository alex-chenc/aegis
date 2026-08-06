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
	"access_outside_workspace":        {},
	"network_boundary_violation":      {},
	"access_container_runtime_socket": {},
	"process_boundary_operation":      {},
	"approval_boundary_violation":     {},
	"protected_path_write":            {},
	"unsandboxed_execution":           {},
	"host_execution_bypass":           {},
}

func NormalizeAgentGuardEscapeFinding(event *model.RuntimeEvent) (*model.AgentSecurityFinding, error) {
	if event == nil || event.EventType != "agent_sandbox_violation" ||
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
	if !allowedValue(decision, "audit", "alert", "would_deny", "enforcement_unavailable") {
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
	permissionMap, _ := envelope.Evidence["permission"].(map[string]any)
	permissionClass := strings.ToLower(strings.TrimSpace(stringValueAny(permissionMap["class"])))
	if permissionClass == "full_access" {
		return nil, nil
	}
	if permissionClass != "restricted" || permissionMap["complete"] != true {
		// Unknown and legacy sessions are not safe escape scopes. Do not create
		// a finding until a complete restricted-session snapshot is present.
		return nil, nil
	}
	boundary := strings.ToLower(strings.TrimSpace(stringValueAny(permissionMap["boundary"])))
	if boundary == "no_isolation" || boundary == "remote_unobservable" {
		return nil, nil
	}
	// DC never turns a rule-shaped signal into a finding by itself. The Agent
	// must prove the trusted Hook-to-process link and provide at least the
	// Hook event plus the eBPF event in the evidence chain.
	if !boolValue(envelope.Evidence["hook_pid_matched"]) ||
		stringValueAny(envelope.Evidence["hook_event_id"]) == "" ||
		stringValueAny(envelope.Evidence["tool_call_id"]) == "" || len(evidenceIDs) < 2 {
		return nil, nil
	}
	classification := firstString(
		stringValueAny(envelope.Evidence["classification"]),
		stringValueAny(envelope.Evidence["escape_classification"]),
	)
	if classification == "" && envelope.Decision == "would_deny" {
		// Accept pre-refactor events as policy attempts for replay/backfill only.
		classification = "policy_violation_attempt"
	}
	switch classification {
	case "not_applicable", "authorized_boundary_expansion", "evidence_insufficient":
		return nil, nil
	case "", "policy_violation_attempt", "confirmed_escape":
	default:
		return nil, fmt.Errorf("%w: escape classification", ErrAgentBehaviorInvalidContract)
	}
	stateChanged := envelope.Violation.StateChanged || boolValue(envelope.Evidence["state_changed"])
	findingKey := "escape:v2:" + sessionID.String() + ":" + rule + ":" + event.EventID
	verdict := "suspicious"
	confidence := 0.90
	if classification == "confirmed_escape" {
		verdict, confidence = "malicious", 0.98
	}
	if decision == "enforcement_unavailable" {
		verdict, confidence = "inconclusive", 0.62
	}
	ruleHit := map[string]any{
		"rule_key":       rule,
		"event_id":       event.EventID,
		"operation":      truncateLimit(operation, 64),
		"state_changed":  stateChanged,
		"decision":       decision,
		"classification": classification,
	}
	return &model.AgentSecurityFinding{
		ID:                  stableFindingID(findingKey),
		FindingKey:          findingKey,
		HostID:              event.HostID,
		InstanceID:          instanceID,
		SessionID:           sessionID,
		ExecutionUnitID:     unitID,
		Title:               escapeTitle(event.EventType, rule, stringValueAny(permissionMap["agent_type"])),
		Severity:            severity,
		Verdict:             verdict,
		Confidence:          confidence,
		Status:              "open",
		DecisionSources:     mustJSON([]string{"escape_permission_rule", "ebpf"}, []string{}),
		RuleHits:            mustJSON([]map[string]any{ruleHit}, []map[string]any{}),
		EvidenceEventIDs:    mustJSON(evidenceIDs, []string{}),
		EvidenceGraph:       evidenceGraph(evidenceIDs, nil, rule),
		AttackStages:        mustJSON([]string{"privilege_escalation", "defense_evasion"}, []string{}),
		Summary:             escapeSummary(stringValueAny(permissionMap["agent_type"]), rule),
		RecommendedAction:   map[string]string{"confirmed_escape": "contain", "policy_violation_attempt": "alert"}[classification],
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
	switch rule {
	case "access_container_runtime_socket", "process_boundary_operation", "host_execution_bypass":
		return "critical"
	case "access_outside_workspace", "network_boundary_violation", "approval_boundary_violation", "protected_path_write":
		return "medium"
	default:
		return "high"
	}
}

func escapeTitle(eventType, rule, agentType string) string {
	name := escapeRuleDisplayName(rule)
	if name == "" {
		name = rule
	}
	if agentType != "" {
		return agentDisplayName(agentType) + "逃逸：" + name
	}
	return "智能体逃逸：" + name
}

func escapeRuleDisplayName(rule string) string {
	return map[string]string{
		"access_outside_workspace":        "访问工作区外路径",
		"network_boundary_violation":      "越过网络访问边界",
		"access_container_runtime_socket": "访问容器运行时接口",
		"process_boundary_operation":      "执行进程边界操作",
		"approval_boundary_violation":     "绕过操作确认边界",
		"protected_path_write":            "写入受保护路径",
		"unsandboxed_execution":           "未受沙箱约束的执行",
		"host_execution_bypass":           "绕过 OpenClaw 沙箱执行",
	}[rule]
}

func agentDisplayName(agentType string) string {
	switch strings.ToLower(strings.TrimSpace(agentType)) {
	case "claude", "claude-code":
		return "Claude Code"
	case "openclaw":
		return "OpenClaw"
	case "hermes":
		return "Hermes"
	case "zcode":
		return "Zcode"
	case "codex":
		return "Codex"
	default:
		return "智能体"
	}
}

func escapeSummary(agentType, rule string) string {
	name := escapeRuleDisplayName(rule)
	if name == "" {
		name = rule
	}
	return agentDisplayName(agentType) + " 会话触发了“" + name + "”规则，且 Hook、进程 PID/start_ticks 与 eBPF 实际执行结果已完成关联。"
}
