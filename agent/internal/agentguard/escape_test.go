package agentguard

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEscapeDetectorEmitsPermissionBoundaryRuleWithoutProcEvidence(t *testing.T) {
	attempt := GuardAttempt{
		EventID:    "attempt-1",
		Operation:  "setns",
		Target:     "mnt:[999]",
		ReturnCode: -1,
	}
	violation, ok := DetectEscapeAttempt(attempt)
	if !ok {
		t.Fatal("external setns was not detected")
	}
	if violation.Rule != EscapeRuleProcessBoundary ||
		violation.Decision != DecisionWouldDeny ||
		violation.StateChanged || len(violation.Baseline.NamespaceInodes) != 0 || len(violation.Diff.Changes) != 0 {
		t.Fatalf("unexpected violation: %#v", violation)
	}
	if len(violation.EvidenceEventIDs) != 1 || violation.EvidenceEventIDs[0] != "attempt-1" {
		t.Fatalf("missing evidence: %#v", violation)
	}
	if violation.Decision == Decision("deny") || violation.Decision == Decision("deny_and_freeze") {
		t.Fatal("P2 detector emitted an active decision")
	}
}

func TestEscapeDetectorCoversPermissionBoundarySignals(t *testing.T) {
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
			name: "bpf load",
			in:   GuardAttempt{Operation: "bpf", Category: CategoryKernel},
			rule: EscapeRuleProcessBoundary,
		},
		{
			name: "module load",
			in:   GuardAttempt{Operation: "finit_module", Category: CategoryKernel},
			rule: EscapeRuleProcessBoundary,
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

	got, ok := DetectEscapeAttempt(GuardAttempt{
		Operation: "capset", Category: CategoryIdentity,
	})
	if !ok || got.Rule != EscapeRuleProcessBoundary || got.StateChanged {
		t.Fatalf("capability escalation not detected: %#v %v", got, ok)
	}
}

func TestEscapeDetectorIgnoresCgroupDriftWhenEvidenceIsUnavailable(t *testing.T) {
	baseline := newIsolationState()
	baseline.CgroupPath = "/user.slice/agent.scope"
	baseline.Availability["cgroup"] = EvidenceAvailability{Available: true}

	actual := baseline
	actual.CgroupPath = ""
	actual.Availability = cloneAvailability(baseline.Availability)
	actual.Availability["cgroup"] = EvidenceAvailability{
		Available: false,
		Reason:    "proc_cgroup_read_failed",
	}

	if violation, ok := DetectEscapeAttempt(GuardAttempt{
		Operation: "open_read",
		Target:    "/etc/ld.so.cache",
		Baseline:  baseline,
		Actual:    actual,
	}); ok {
		t.Fatalf("unavailable cgroup evidence must not create an escape violation: %#v", violation)
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
