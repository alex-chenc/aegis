package pipeline

import (
	"encoding/json"
	"testing"
	"time"

	"dc/internal/model"

	"github.com/google/uuid"
)

func TestBuiltinRuleManifestIsStable(t *testing.T) {
	want := map[string]string{
		"AGB-BUILTIN-001": "sha256:e9a7f8b0dda7c742557bbc1a0551ea4caeb0329973ec1c24f7751b4cd2902a82",
		"AGB-BUILTIN-002": "sha256:5852cf43c0be2ddc21e83c8c12fb898ac2aae47bc0d7bff2a5246d4d2436e613",
		"AGB-BUILTIN-003": "sha256:b066e0b452fb7749f9afb49e8a6b918de1285ccef912f50680795a4bc110e03e",
		"AGB-BUILTIN-004": "sha256:43e4e365124e4d895a27f8267e8ab424f0482f121794a812aea946231520e130",
		"AGB-BUILTIN-005": "sha256:63ce19628fc8285ded19f9609ec93770341c14e24e47680e95aff3cec4d775f1",
	}
	manifest := BuiltinRuleManifest()
	if len(manifest) != len(want) {
		t.Fatalf("manifest size = %d, want %d", len(manifest), len(want))
	}
	for _, rule := range manifest {
		if rule.Version != 1 || rule.Digest != want[rule.Key] || rule.ID == uuid.Nil ||
			len(rule.DefaultParameters) == 0 || len(rule.RequiredEvidence) == 0 ||
			len(rule.AllowConditions) == 0 {
			t.Fatalf("unstable rule manifest: %#v", rule)
		}
		delete(want, rule.Key)
	}
	if len(want) != 0 {
		t.Fatalf("missing rules: %v", want)
	}
	manifest[0].DefaultParameters["resource_groups"] = []string{"mutated"}
	if len(BuiltinRuleManifest()[0].DefaultParameters["resource_groups"].([]string)) == 1 {
		t.Fatal("caller mutated the immutable built-in manifest")
	}
}

func TestBuiltinRulesMatchRealEvidenceAndRespectNegativeConditions(t *testing.T) {
	base := testBehaviorEvent()
	tests := []struct {
		name     string
		mutate   func(*model.AgentBehaviorEvent)
		options  RuleEvaluationOptions
		wantRule string
	}{
		{
			name: "sensitive directory",
			mutate: func(event *model.AgentBehaviorEvent) {
				event.Category, event.Operation, event.Outcome = "file", "read_observed", "success"
				setResource(event, "/etc/shadow", map[string]any{"resolved_path": "/etc/shadow"})
			},
			wantRule: "AGB-BUILTIN-001",
		},
		{
			name: "untrusted exception field cannot suppress rule",
			mutate: func(event *model.AgentBehaviorEvent) {
				event.Category, event.Operation, event.Outcome = "file", "read_observed", "success"
				setResource(event, "/etc/shadow", map[string]any{
					"resolved_path": "/etc/shadow", "policy_exception": true,
				})
			},
			wantRule: "AGB-BUILTIN-001",
		},
		{
			name: "verified policy exception suppresses rule",
			mutate: func(event *model.AgentBehaviorEvent) {
				event.Category, event.Operation, event.Outcome = "file", "read_observed", "success"
				setResource(event, "/etc/shadow", map[string]any{"resolved_path": "/etc/shadow"})
			},
			options: RuleEvaluationOptions{PolicyExceptionEventIDs: []string{"event-1"}},
		},
		{
			name: "external network",
			mutate: func(event *model.AgentBehaviorEvent) {
				event.Category, event.Operation, event.Outcome = "network", "connect", "success"
				setResource(event, "8.8.8.8:443", map[string]any{
					"destination_ip": "8.8.8.8", "destination_port": 443,
					"direction": "outbound", "protocol": "tcp",
				})
			},
			wantRule: "AGB-BUILTIN-002",
		},
		{
			name: "trusted network exception",
			mutate: func(event *model.AgentBehaviorEvent) {
				event.Category, event.Operation, event.Outcome = "network", "connect", "success"
				setResource(event, "10.1.2.3:443", map[string]any{
					"destination_ip": "10.1.2.3", "destination_port": 443,
					"direction": "outbound", "protocol": "tcp",
				})
			},
		},
		{
			name: "file create",
			mutate: func(event *model.AgentBehaviorEvent) {
				event.Category, event.Operation, event.Outcome = "file", "create", "success"
				setResource(event, "/tmp/tool.sh", map[string]any{
					"resolved_path": "/tmp/tool.sh", "inode_created": true, "executable": true,
				})
			},
			wantRule: "AGB-BUILTIN-003",
		},
		{
			name: "failed create is not generated file",
			mutate: func(event *model.AgentBehaviorEvent) {
				event.Category, event.Operation, event.Outcome = "file", "create", "failure"
				setResource(event, "/tmp/tool.sh", map[string]any{
					"resolved_path": "/tmp/tool.sh", "inode_created": false,
				})
			},
		},
		{
			name: "sensitive command exact basename",
			mutate: func(event *model.AgentBehaviorEvent) {
				event.Category, event.Operation, event.Outcome = "process", "exec", "success"
				event.ProcessExe = "/usr/bin/sudo"
				event.CommandArgv = json.RawMessage(`["sudo","id"]`)
			},
			wantRule: "AGB-BUILTIN-004",
		},
		{
			name: "command substring is not executable match",
			mutate: func(event *model.AgentBehaviorEvent) {
				event.Category, event.Operation, event.Outcome = "process", "exec", "success"
				event.ProcessExe = "/usr/bin/printf"
				event.CommandArgv = json.RawMessage(`["printf","sudo-report.txt"]`)
			},
		},
		{
			name: "confirmed host privilege gain",
			mutate: func(event *model.AgentBehaviorEvent) {
				event.Category, event.Operation, event.Outcome = "identity", "setuid", "success"
				setResource(event, "uid", map[string]any{
					"euid_before": 1000, "euid_after": 0, "user_namespace": "host",
				})
			},
			wantRule: "AGB-BUILTIN-005",
		},
		{
			name: "container namespace root exception",
			mutate: func(event *model.AgentBehaviorEvent) {
				event.Category, event.Operation, event.Outcome = "identity", "setuid", "success"
				setResource(event, "uid", map[string]any{
					"euid_before": 1000, "euid_after": 0, "user_namespace": "container",
				})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := *base
			test.mutate(&event)
			event = *ClassifyAgentBehavior(&event, test.options)
			hits := EvaluateBuiltinRules(&event, test.options)
			if test.wantRule == "" {
				if len(hits) != 0 {
					t.Fatalf("unexpected hits: %#v", hits)
				}
				return
			}
			if !containsRuleHit(hits, test.wantRule) {
				t.Fatalf("hits = %#v, want %s", hits, test.wantRule)
			}
		})
	}
}

func testBehaviorEvent() *model.AgentBehaviorEvent {
	instanceID, unitID, sessionID := uuid.New(), uuid.New(), uuid.New()
	return &model.AgentBehaviorEvent{
		RawEventID: "event-1", HostID: uuid.New(), HostBootID: "boot", AgentSequence: 1,
		InstanceID: &instanceID, ExecutionUnitID: &unitID, SessionID: &sessionID,
		Category: "process", Operation: "exec", Outcome: "success", Decision: "audit",
		PID: 10, PPID: 1, ProcessStartTicks: "99", CommandArgv: json.RawMessage(`[]`),
		Resource:   json.RawMessage(`{"type":"process","attributes":{}}`),
		Collection: json.RawMessage(`{"visibility":"complete","lost_events_since_last":0,"aggregated_count":1}`),
		OccurredAt: time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC),
	}
}

func setResource(event *model.AgentBehaviorEvent, identity string, attributes map[string]any) {
	event.ResourceIdentity = identity
	data, _ := json.Marshal(map[string]any{
		"type": event.Category, "identity": identity, "attributes": attributes,
	})
	event.Resource = data
}

func containsRuleHit(hits []AgentRuleHit, key string) bool {
	for _, hit := range hits {
		if hit.RuleKey == key {
			return true
		}
	}
	return false
}
