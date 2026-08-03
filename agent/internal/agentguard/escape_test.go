package agentguard

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEscapeDetectorEmitsOnlyMonitorDecisionsWithStateEvidence(t *testing.T) {
	baseline := newIsolationState()
	baseline.NamespaceInodes = map[string]uint64{"mnt": 100, "pid": 101}
	baseline.Availability["namespaces"] = EvidenceAvailability{Available: true}
	actual := baseline
	actual.NamespaceInodes = map[string]uint64{"mnt": 100, "pid": 101}
	actual.Availability = cloneAvailability(baseline.Availability)

	attempt := GuardAttempt{
		EventID:    "attempt-1",
		Operation:  "setns",
		Target:     "mnt:[999]",
		ReturnCode: -1,
		Baseline:   baseline,
		Actual:     actual,
	}
	violation, ok := DetectEscapeAttempt(attempt)
	if !ok {
		t.Fatal("external setns was not detected")
	}
	if violation.Rule != EscapeRuleJoinExternalNamespace ||
		violation.Decision != DecisionWouldDeny ||
		violation.StateChanged {
		t.Fatalf("unexpected violation: %#v", violation)
	}
	if violation.Baseline.NamespaceInodes["mnt"] != 100 ||
		violation.Actual.NamespaceInodes["mnt"] != 100 ||
		violation.EvidenceEventIDs[0] != "attempt-1" {
		t.Fatalf("missing evidence: %#v", violation)
	}
	if violation.Decision == Decision("deny") || violation.Decision == Decision("deny_and_freeze") {
		t.Fatal("P2 detector emitted an active decision")
	}
}

func TestEscapeDetectorCoversRuntimeProcCgroupKernelAndCapabilitySignals(t *testing.T) {
	tests := []struct {
		name string
		in   GuardAttempt
		rule string
	}{
		{
			name: "runtime socket",
			in:   GuardAttempt{Operation: "open_write", Category: CategoryFile, Target: "/run/docker.sock"},
			rule: EscapeRuleAccessRuntimeSocket,
		},
		{
			name: "host proc root",
			in:   GuardAttempt{Operation: "open", Category: CategoryFile, Target: "/proc/1/root/etc/shadow"},
			rule: EscapeRuleAccessHostProcRoot,
		},
		{
			name: "cgroup write",
			in:   GuardAttempt{Operation: "write", Category: CategoryFile, Target: "/sys/fs/cgroup/cgroup.procs"},
			rule: EscapeRuleWriteCgroupFS,
		},
		{
			name: "bpf load",
			in:   GuardAttempt{Operation: "bpf", Category: CategoryKernel},
			rule: EscapeRuleLoadBPFOrModule,
		},
		{
			name: "module load",
			in:   GuardAttempt{Operation: "finit_module", Category: CategoryKernel},
			rule: EscapeRuleLoadBPFOrModule,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := DetectEscapeAttempt(tt.in)
			if !ok || got.Rule != tt.rule || got.Decision != DecisionWouldDeny {
				t.Fatalf("DetectEscapeAttempt()=(%#v,%v), want %s would_deny", got, ok, tt.rule)
			}
		})
	}

	before := newIsolationState()
	before.Capabilities = CapabilityState{Visible: true, Effective: "0x0000000000000001"}
	before.Availability["capabilities"] = EvidenceAvailability{Available: true}
	after := before
	after.Capabilities.Effective = "0x0000000000000003"
	after.Availability = cloneAvailability(before.Availability)
	got, ok := DetectEscapeAttempt(GuardAttempt{
		Operation: "capset", Category: CategoryIdentity, Baseline: before, Actual: after,
	})
	if !ok || got.Rule != EscapeRuleCapabilityEscalation || !got.StateChanged {
		t.Fatalf("capability escalation not detected: %#v %v", got, ok)
	}
}

func TestNormalizerRedactsNestedIsolationEvidence(t *testing.T) {
	tracker, child := newAttributedTracker(t)
	normalizer := NewBehaviorNormalizer("host-1", "boot-1", tracker)
	event, ok := normalizer.Normalize(RawBehavior{
		OccurredAt: time.Now(), Category: CategoryIsolation, Operation: "setns_attempt",
		Outcome: OutcomeFailed, Process: child,
		Resource: Resource{Type: "namespace", Identity: "/proc/1/ns/mnt"},
		Decision: DecisionWouldDeny, Severity: "high",
		Isolation: map[string]any{
			"actual": map[string]any{"authorization": "Bearer test-secret-value"},
		},
		Evidence: map[string]any{
			"target": "https://alice:password@example.invalid/ns?token=test-secret-value",
		},
	})
	if !ok {
		t.Fatal("expected attributed event")
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "test-secret-value") || strings.Contains(string(data), "alice:password") {
		t.Fatalf("nested evidence leaked secret: %s", data)
	}
	if event.Decision != DecisionWouldDeny || event.Severity != "high" {
		t.Fatalf("safe P2 decision was not preserved: %#v", event)
	}
}
