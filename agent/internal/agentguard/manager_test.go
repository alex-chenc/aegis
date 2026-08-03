package agentguard

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	pb "aegis-agent/pkg/api/v1"
)

func TestBehaviorRuntimeEventSanitizesInvalidUTF8AtProtoBoundary(t *testing.T) {
	invalid := string([]byte{'x', 0xff, 'y'})
	event := behaviorRuntimeEvent(BehaviorEvent{
		EventID:    invalid,
		HostID:     invalid,
		EventType:  invalid,
		OccurredAt: time.Now(),
		Actor: Actor{
			PID:        10,
			StartTicks: 20,
			PPID:       1,
			Exe:        invalid,
			Argv:       []string{invalid},
		},
		Resource: Resource{Type: "file", Identity: invalid},
		Category: CategoryFile,
		Severity: invalid,
	})
	for name, value := range map[string]string{
		"event_id":        event.EventId,
		"host_id":         event.HostId,
		"event_type":      event.EventType,
		"process_name":    event.ProcessName,
		"command_line":    event.CommandLine,
		"file_path":       event.FilePath,
		"severity":        event.Severity,
		"event_data_json": event.EventDataJson,
	} {
		if !utf8.ValidString(value) {
			t.Fatalf("%s remains invalid UTF-8: %q", name, value)
		}
	}
}

func TestQueuePendingLifecyclesIncludesNewExecControllerDependencies(t *testing.T) {
	manager := NewManager(ManagerConfig{
		Enabled: true, BehaviorMonitorEnabled: true, HostID: "host-1",
		StateDir: t.TempDir(), ReconcileInterval: time.Hour, FlushInterval: time.Hour,
	}, &fakeScanner{processes: map[uint32]ProcessSnapshot{}}, &captureReporter{})
	controller := confirmedProcess(4200, 200, "/opt/codex/bin/codex", "codex")
	manager.tracker.OnExec(controller)
	manager.queuePendingLifecycles()

	manager.statusMu.Lock()
	events := append([]*pb.RuntimeEvent(nil), manager.pendingStatus...)
	manager.statusMu.Unlock()
	if len(events) < 3 {
		t.Fatalf("expected instance, unit, and session lifecycle events, got %d", len(events))
	}
	want := []string{"agent_instance_started", "agent_execution_unit_started", "agent_behavior_session_started"}
	for index, eventType := range want {
		if events[index].EventType != eventType {
			t.Fatalf("lifecycle[%d] = %q, want %q", index, events[index].EventType, eventType)
		}
	}
}

type fakeScanner struct {
	processes map[uint32]ProcessSnapshot
	isolation map[uint32]IsolationState
}

func (s *fakeScanner) Scan() ([]ProcessSnapshot, error) {
	out := make([]ProcessSnapshot, 0, len(s.processes))
	for _, process := range s.processes {
		out = append(out, process)
	}
	return out, nil
}

func (s *fakeScanner) ReadPID(pid uint32) (ProcessSnapshot, error) {
	process, ok := s.processes[pid]
	if !ok {
		return ProcessSnapshot{}, context.Canceled
	}
	return process, nil
}

func (s *fakeScanner) BootID() string { return "boot-1" }

func (s *fakeScanner) ReadIsolation(pid uint32) (IsolationState, error) {
	state, ok := s.isolation[pid]
	if !ok {
		return IsolationState{}, context.Canceled
	}
	return state, nil
}

type captureReporter struct {
	mu     sync.Mutex
	events []*pb.RuntimeEvent
}

func (r *captureReporter) ReportEvents(events []*pb.RuntimeEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, events...)
	return nil
}

func (r *captureReporter) snapshot() []*pb.RuntimeEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*pb.RuntimeEvent(nil), r.events...)
}

func TestManagerRestoresAttributionEmitsRedactedMonitorOnlyEventsAndBundleStatus(t *testing.T) {
	controller := confirmedProcess(4100, 100, "/opt/codex/bin/codex", "codex")
	child := ProcessSnapshot{
		Identity: ProcessIdentity{PID: 4110, StartTicks: 110},
		PPID:     controller.Identity.PID,
		Exe:      "/usr/bin/bash",
		Argv:     []string{"bash"},
	}
	scanner := &fakeScanner{processes: map[uint32]ProcessSnapshot{
		controller.Identity.PID: controller,
		child.Identity.PID:      child,
	}}
	reporter := &captureReporter{}
	manager := NewManager(ManagerConfig{
		Enabled: true, BehaviorMonitorEnabled: true, HostID: "host-1",
		StateDir: t.TempDir(), ReconcileInterval: time.Hour,
		FlushInterval: 5 * time.Millisecond, SpoolCapacity: 8,
	}, scanner, reporter)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}

	if !manager.ObserveRaw(RawBehavior{
		OccurredAt: time.Now(), Category: CategoryProcess, Operation: "exec",
		Outcome: OutcomeSuccess, Process: child,
		Argv:     []string{"curl", "--token", "test-secret-value"},
		Resource: Resource{Type: "process", Identity: "/usr/bin/curl"},
	}) {
		t.Fatal("expected reconciled child behavior to be accepted")
	}
	bundle := validBundle(t, 1)
	payload, _ := json.Marshal(bundle)
	if err := manager.ApplyBundle(string(payload)); err != nil {
		t.Fatalf("apply bundle: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	manager.Stop()

	var behavior, applied, received bool
	var instanceIndex, unitIndex, sessionIndex, behaviorIndex = -1, -1, -1, -1
	for index, event := range reporter.snapshot() {
		switch event.EventType {
		case "agent_instance_started":
			instanceIndex = index
		case "agent_execution_unit_started":
			unitIndex = index
		case "agent_behavior_session_started":
			sessionIndex = index
		}
		if event.EventType == "agent_behavior" {
			behavior = true
			behaviorIndex = index
			if containsSecret(event.EventDataJson, event.CommandLine) {
				t.Fatalf("secret leaked in reported behavior: %#v", event)
			}
		}
		if event.EventType == "agent_guard_config_status" && event.EventDataJson != "" {
			var body map[string]any
			if json.Unmarshal([]byte(event.EventDataJson), &body) == nil {
				if body["bundle_version"] == float64(0) {
					t.Fatalf("unprojectable config status emitted: %s", event.EventDataJson)
				}
				if body["status"] == "applied" {
					applied = true
				}
				if body["status"] == "received" && body["bundle_version"] == float64(1) && body["digest"] == bundle.Digest {
					received = true
				}
			}
		}
	}
	if !behavior || !applied || !received {
		t.Fatalf("missing behavior/config status: behavior=%v received=%v applied=%v events=%d", behavior, received, applied, len(reporter.snapshot()))
	}
	if instanceIndex < 0 || unitIndex <= instanceIndex || sessionIndex <= unitIndex || behaviorIndex <= sessionIndex {
		t.Fatalf("lifecycle ordering mismatch: instance=%d unit=%d session=%d behavior=%d", instanceIndex, unitIndex, sessionIndex, behaviorIndex)
	}
}

func containsSecret(values ...string) bool {
	for _, value := range values {
		if value == "test-secret-value" {
			return true
		}
		for i := 0; i+17 <= len(value); i++ {
			if value[i:i+17] == "test-secret-value" {
				return true
			}
		}
	}
	return false
}

func TestParseProcStatUsesStartTicksNotWallClock(t *testing.T) {
	data := []byte("123 (worker with ) paren) S 42 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 987654 0 0")
	ppid, ticks, err := parseProcStat(data)
	if err != nil {
		t.Fatal(err)
	}
	if ppid != 42 || ticks != 987654 {
		t.Fatalf("unexpected process identity: ppid=%d ticks=%d", ppid, ticks)
	}
}
