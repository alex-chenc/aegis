package pipeline

import (
	"encoding/json"
	"testing"

	"dc/internal/model"

	"github.com/google/uuid"
)

func TestNormalizeEscapeFindingKeepsWouldDenyAsAuditOnly(t *testing.T) {
	hostID := uuid.New()
	eventID := uuid.NewString()
	instanceID := uuid.NewString()
	unitID := uuid.NewString()
	sessionID := uuid.NewString()
	runtimeEvent := &model.RuntimeEvent{
		EventID:   eventID,
		HostID:    hostID,
		EventType: "agent_sandbox_violation",
		EventData: `{"schema":"aegis.agent_behavior.v1","event_id":"` + eventID +
			`","host_id":"` + hostID.String() + `","instance_id":"` + instanceID +
			`","execution_unit_id":"` + unitID + `","session_id":"` + sessionID +
			`","occurred_at":"2026-07-30T10:00:00Z","decision":"would_deny","severity":"critical",` +
			`"rule_id":"access_outside_workspace","operation":"file_access_violation",` +
			`"isolation":{"baseline":{},"actual":{},"diff":{"state_changed":false}},` +
			`"evidence":{"rule":"access_outside_workspace","operation":"file_access","baseline":{},` +
			`"actual":{},"diff":{"state_changed":false},"state_changed":false,` +
			`"permission":{"class":"restricted","complete":true,"permission_mode":"default","network_access":false},` +
			`"hook_pid_matched":true,"hook_event_id":"` + uuid.NewString() + `","tool_call_id":"call-1",` +
			`"evidence_event_ids":["` + eventID + `","` + uuid.NewString() + `"]}}`,
	}
	finding, err := NormalizeAgentGuardEscapeFinding(runtimeEvent)
	if err != nil {
		t.Fatalf("NormalizeAgentGuardEscapeFinding: %v", err)
	}
	if finding.RecommendedAction != "alert" || finding.Verdict != "suspicious" ||
		finding.EvidenceSourceTable != "agent_guard_events" {
		t.Fatalf("finding = %#v", finding)
	}
	var sources []string
	if err := json.Unmarshal(finding.DecisionSources, &sources); err != nil ||
		len(sources) != 2 || sources[0] != "escape_permission_rule" || sources[1] != "ebpf" {
		t.Fatalf("decision_sources=%s err=%v", finding.DecisionSources, err)
	}
}

func TestNormalizeEscapeFindingRejectsUnknownOrActiveDecision(t *testing.T) {
	for _, test := range []struct {
		name      string
		eventType string
		decision  string
		rule      string
	}{
		{name: "unknown rule", eventType: "agent_sandbox_violation", decision: "audit", rule: "arbitrary"},
		{name: "active deny", eventType: "agent_sandbox_violation", decision: "deny", rule: "access_outside_workspace"},
		{name: "unsupported type", eventType: "agent_guard_health", decision: "audit", rule: "access_outside_workspace"},
	} {
		t.Run(test.name, func(t *testing.T) {
			hostID := uuid.New()
			eventID := uuid.NewString()
			runtimeEvent := &model.RuntimeEvent{
				EventID: eventID, HostID: hostID, EventType: test.eventType,
				EventData: `{"schema":"aegis.agent_behavior.v1","event_id":"` + eventID +
					`","host_id":"` + hostID.String() + `","instance_id":"` + uuid.NewString() +
					`","execution_unit_id":"` + uuid.NewString() + `","session_id":"` + uuid.NewString() +
					`","occurred_at":"2026-07-30T10:00:00Z","decision":"` + test.decision +
					`","rule_id":"` + test.rule + `","operation":"setns_violation",` +
					`"isolation":{"baseline":{},"actual":{},"diff":{}},` +
					`"evidence":{"rule":"` + test.rule + `","operation":"setns",` +
					`"baseline":{},"actual":{},"diff":{},"state_changed":false}}`,
			}
			if _, err := NormalizeAgentGuardEscapeFinding(runtimeEvent); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestNormalizeEscapeFindingSuppressesIncompleteHookProcessEvidence(t *testing.T) {
	hostID := uuid.New()
	eventID := uuid.NewString()
	runtimeEvent := &model.RuntimeEvent{
		EventID: eventID, HostID: hostID, EventType: "agent_sandbox_violation",
		EventData: `{"schema":"aegis.agent_behavior.v1","event_id":"` + eventID +
			`","host_id":"` + hostID.String() + `","instance_id":"` + uuid.NewString() +
			`","execution_unit_id":"` + uuid.NewString() + `","session_id":"` + uuid.NewString() +
			`","occurred_at":"2026-07-30T10:00:00Z","decision":"alert","severity":"high",` +
			`"rule_id":"access_outside_workspace","operation":"open_read_violation",` +
			`"evidence":{"rule":"access_outside_workspace","operation":"open_read",` +
			`"permission":{"class":"restricted","complete":true},"classification":"confirmed_escape"}}`,
	}
	finding, err := NormalizeAgentGuardEscapeFinding(runtimeEvent)
	if err != nil {
		t.Fatalf("NormalizeAgentGuardEscapeFinding: %v", err)
	}
	if finding != nil {
		t.Fatal("incomplete Hook/process evidence must not create a finding")
	}
}
