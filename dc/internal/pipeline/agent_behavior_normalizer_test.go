package pipeline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNormalizeAgentBehaviorRedactsSecretsAndTracksCollectionGaps(t *testing.T) {
	hostID := uuid.New()
	eventID := uuid.New().String()
	instanceID := uuid.NewString()
	unitID := uuid.NewString()
	sessionID := uuid.NewString()
	raw := `{
		"schema":"aegis.agent_behavior.v1",
		"event_id":"` + eventID + `",
		"host_id":"` + hostID.String() + `",
		"host_boot_id":"boot-1",
		"agent_sequence":42,
		"instance_id":"` + instanceID + `",
		"execution_unit_id":"` + unitID + `",
		"session_id":"` + sessionID + `",
		"occurred_at":"2026-07-30T10:00:00.123456Z",
		"occurred_monotonic_ns":99182700123,
		"category":"network",
		"operation":"connect",
		"outcome":"success",
		"attribution_confidence":"confirmed",
		"actor":{
			"pid":1234,
			"ppid":1200,
			"start_ticks":"987654",
			"name":"curl",
			"exe":"/usr/bin/curl",
			"argv":["curl","--authorization","Bearer secret-token","https://user:password@example.com/api?token=abc"],
			"cwd":"/workspace"
		},
		"resource":{
			"type":"network",
			"identity":"https://user:password@example.com/api?token=abc",
			"classification":"external_network",
			"attributes":{"authorization":"Bearer another-secret"}
		},
		"evidence":{"note":"Authorization: Bearer evidence-secret"},
		"collection":{
			"source":"ebpf",
			"sensor":"tcp_connect",
			"visibility":"complete",
			"truncated_fields":["actor.argv"],
			"lost_events_since_last":3,
			"aggregated_count":5
		}
	}`

	got, err := NormalizeAgentBehavior(eventID, hostID, raw)
	if err != nil {
		t.Fatalf("NormalizeAgentBehavior: %v", err)
	}
	if got.CommandVisibility != "partial" {
		t.Fatalf("command visibility = %q, want partial", got.CommandVisibility)
	}
	if got.LostEventsSinceLast != 3 || got.AggregatedCount != 5 || !got.HasTruncatedFields {
		t.Fatalf("completeness counters lost=%d aggregate=%d truncated=%t",
			got.LostEventsSinceLast, got.AggregatedCount, got.HasTruncatedFields)
	}
	serialized, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}
	text := string(serialized)
	for _, secret := range []string{"secret-token", "password@example.com", "token=abc", "another-secret", "evidence-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("projection leaked secret %q: %s", secret, text)
		}
	}
	for _, marker := range []string{
		redactedValue,
		`"lost_events_since_last":3`,
		`"aggregated_count":5`,
		`"coverage_level":"monitor_only"`,
		`"p1_monitor_only"`,
	} {
		if !strings.Contains(text, marker) {
			t.Fatalf("projection missing %q: %s", marker, text)
		}
	}
}

func TestNormalizeAgentBehaviorRejectsInvalidContract(t *testing.T) {
	hostID := uuid.New()
	eventID := uuid.New().String()
	tests := []struct {
		name string
		raw  string
	}{
		{name: "invalid json", raw: `{`},
		{name: "unsupported schema", raw: `{"schema":"aegis.agent_behavior.v2"}`},
		{name: "missing sequence", raw: `{"schema":"aegis.agent_behavior.v1","event_id":"` + eventID + `","host_id":"` + hostID.String() + `","host_boot_id":"boot","occurred_at":"2026-07-30T10:00:00Z","category":"process","operation":"exec","outcome":"success"}`},
		{name: "host mismatch", raw: `{"schema":"aegis.agent_behavior.v1","event_id":"` + eventID + `","host_id":"` + uuid.NewString() + `","host_boot_id":"boot","agent_sequence":1,"occurred_at":"2026-07-30T10:00:00Z","occurred_monotonic_ns":1,"category":"process","operation":"exec","outcome":"success","collection":{"source":"ebpf","sensor":"execve","lost_events_since_last":0}}`},
		{name: "missing monotonic clock", raw: `{"schema":"aegis.agent_behavior.v1","event_id":"` + eventID + `","host_id":"` + hostID.String() + `","host_boot_id":"boot","agent_sequence":1,"occurred_at":"2026-07-30T10:00:00Z","category":"process","operation":"exec","outcome":"success","collection":{"source":"ebpf","sensor":"execve","lost_events_since_last":0}}`},
		{name: "missing collection counter", raw: `{"schema":"aegis.agent_behavior.v1","event_id":"` + eventID + `","host_id":"` + hostID.String() + `","host_boot_id":"boot","agent_sequence":1,"occurred_at":"2026-07-30T10:00:00Z","occurred_monotonic_ns":1,"category":"process","operation":"exec","outcome":"success","collection":{"source":"ebpf","sensor":"execve"}}`},
		{name: "unknown coverage", raw: `{"schema":"aegis.agent_behavior.v1","event_id":"` + eventID + `","host_id":"` + hostID.String() + `","host_boot_id":"boot","agent_sequence":1,"occurred_at":"2026-07-30T10:00:00Z","occurred_monotonic_ns":1,"category":"process","operation":"exec","outcome":"success","collection":{"source":"ebpf","sensor":"execve","lost_events_since_last":0,"coverage_level":"magic"}}`},
		{name: "active deny in monitor only", raw: `{"schema":"aegis.agent_behavior.v1","event_id":"` + eventID + `","host_id":"` + hostID.String() + `","host_boot_id":"boot","agent_sequence":1,"occurred_at":"2026-07-30T10:00:00Z","occurred_monotonic_ns":1,"category":"process","operation":"exec","outcome":"denied","decision":"deny","collection":{"source":"ebpf","sensor":"execve","lost_events_since_last":0}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NormalizeAgentBehavior(eventID, hostID, test.raw); err == nil {
				t.Fatal("expected contract error")
			}
		})
	}
}

func TestNormalizeAgentBehaviorCanonicalizesEarlyP1FailureOutcome(t *testing.T) {
	hostID := uuid.New()
	eventID := uuid.NewString()
	raw := `{"schema":"aegis.agent_behavior.v1","event_id":"` + eventID + `","host_id":"` + hostID.String() + `","host_boot_id":"boot","agent_sequence":1,"instance_id":"` + uuid.NewString() + `","execution_unit_id":"` + uuid.NewString() + `","session_id":"` + uuid.NewString() + `","occurred_at":"2026-07-30T10:00:00Z","occurred_monotonic_ns":1,"category":"process","operation":"exec","outcome":"failed","actor":{"pid":1,"start_ticks":1},"attribution_confidence":"confirmed","collection":{"source":"ebpf","sensor":"execve","lost_events_since_last":0}}`
	got, err := NormalizeAgentBehavior(eventID, hostID, raw)
	if err != nil {
		t.Fatalf("NormalizeAgentBehavior: %v", err)
	}
	if got.Outcome != "failure" {
		t.Fatalf("outcome = %q, want failure", got.Outcome)
	}
}
