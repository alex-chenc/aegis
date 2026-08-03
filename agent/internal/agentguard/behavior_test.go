package agentguard

import (
	"strings"
	"testing"
	"time"
)

func newAttributedTracker(t *testing.T) (*IdentityTracker, ProcessSnapshot) {
	t.Helper()
	tracker := NewIdentityTracker("host-1", NewBuiltinProfileRegistry())
	controller := confirmedProcess(4100, 100, "/opt/codex/bin/codex", "codex")
	_, ok := tracker.ObserveController(controller)
	if !ok {
		t.Fatal("controller not observed")
	}
	child := ProcessSnapshot{
		Identity: ProcessIdentity{PID: 4110, StartTicks: 110},
		PPID:     controller.Identity.PID,
		Exe:      "/usr/bin/bash",
		Argv:     []string{"bash"},
	}
	tracker.OnFork(controller.Identity, child)
	return tracker, child
}

func TestNormalizerRedactsSecretsAndRejectsUnattributed(t *testing.T) {
	tracker, child := newAttributedTracker(t)
	normalizer := NewBehaviorNormalizer("host-1", "boot-1", tracker)
	raw := RawBehavior{
		OccurredAt: time.Now(),
		Category:   CategoryProcess,
		Operation:  "exec",
		Outcome:    OutcomeSuccess,
		Process:    child,
		Argv: []string{
			"curl",
			"--token", "test-secret-value",
			"https://alice:password@example.invalid/x?api_key=test-secret-value",
		},
		Resource: Resource{Type: "process", Identity: "/usr/bin/curl"},
	}
	event, ok := normalizer.Normalize(raw)
	if !ok {
		t.Fatal("expected attributed event")
	}
	encoded := event.MustJSON()
	for _, secret := range []string{"test-secret-value", "alice:password"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("secret leaked into event: %s", encoded)
		}
	}
	if event.Decision != DecisionAudit || event.Collection.Visibility == "" {
		t.Fatalf("P1 must emit explicit monitor-only evidence: %#v", event)
	}

	raw.Process = ProcessSnapshot{Identity: ProcessIdentity{PID: 9000, StartTicks: 900}, Exe: "/usr/bin/bash"}
	if _, ok := normalizer.Normalize(raw); ok {
		t.Fatal("unattributed process entered Agent behavior stream")
	}
}

func TestResolvePathUsesCwdDirFDAndContainerRoot(t *testing.T) {
	got := ResolvePath(PathInput{RawPath: "../tmp/file.txt", CWD: "/work/sub"})
	if got.ResolvedPath != "/work/tmp/file.txt" || got.Resolution != "cwd" {
		t.Fatalf("unexpected cwd resolution: %#v", got)
	}
	got = ResolvePath(PathInput{RawPath: "config.json", DirFDPath: "/sandbox/etc", ContainerRoot: "/var/lib/containers/root"})
	if got.ResolvedPath != "/sandbox/etc/config.json" || got.HostPath != "/var/lib/containers/root/sandbox/etc/config.json" || got.Resolution != "dirfd" {
		t.Fatalf("unexpected dirfd/container resolution: %#v", got)
	}
}

func TestNormalizerSupportsP1SecuritySemanticDomainsInAuditMode(t *testing.T) {
	tracker, child := newAttributedTracker(t)
	normalizer := NewBehaviorNormalizer("host-1", "boot-1", tracker)
	for _, category := range []Category{
		CategoryProcess, CategoryFile, CategoryNetwork, CategoryIdentity,
		CategoryKernel, CategoryIsolation, CategoryIPC, CategoryControl,
	} {
		event, ok := normalizer.Normalize(RawBehavior{
			OccurredAt: time.Now(), Category: category, Operation: "attempt",
			Outcome: OutcomeUnknown, Process: child,
			Resource: Resource{Type: string(category), Identity: "test"},
			Source:   "test", Sensor: "semantic_sensor",
		})
		if !ok || event.Category != category || event.Decision != DecisionAudit {
			t.Fatalf("semantic category %q not normalized in audit mode: %#v", category, event)
		}
	}
}

func TestAggregatorAndPrioritySpoolExposeDrops(t *testing.T) {
	now := time.Now()
	event := BehaviorEvent{
		Schema:          BehaviorSchemaV1,
		EventID:         "e1",
		InstanceID:      "i1",
		ExecutionUnitID: "u1",
		SessionID:       "s1",
		OccurredAt:      now,
		Category:        CategoryFile,
		Operation:       "read_observed",
		Outcome:         OutcomeSuccess,
		Actor:           Actor{PID: 10, StartTicks: 20},
		Resource:        Resource{Type: "file", Identity: "/tmp/a"},
		Decision:        DecisionAudit,
	}
	agg := NewAggregator(2 * time.Second)
	if flushed := agg.Add(event); len(flushed) != 0 {
		t.Fatal("first event should remain in aggregation window")
	}
	event.EventID = "e2"
	event.OccurredAt = now.Add(time.Second)
	if flushed := agg.Add(event); len(flushed) != 0 {
		t.Fatal("second event should aggregate")
	}
	flushed := agg.Flush(now.Add(3 * time.Second))
	if len(flushed) != 1 || flushed[0].Collection.AggregatedCount != 2 {
		t.Fatalf("unexpected aggregate: %#v", flushed)
	}

	spool := NewPrioritySpool(2)
	spool.Push(event, PriorityRepetitiveIO)
	event.EventID = "state"
	event.Operation = "exit"
	spool.Push(event, PriorityStateChange)
	event.EventID = "critical"
	event.Operation = "isolation_drift"
	if !spool.Push(event, PriorityCriticalEvidence) {
		t.Fatal("critical event must be retained")
	}
	stats := spool.Stats()
	if stats.DroppedByReason["evicted_lower_priority"] != 1 {
		t.Fatalf("expected visible eviction count: %#v", stats)
	}
}
