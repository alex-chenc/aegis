package agentguard

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestManagerCreatesNamespaceUnitBaselineAndReportsDrift(t *testing.T) {
	controller := confirmedProcess(5100, 100, "/opt/codex/bin/codex", "codex")
	worker := ProcessSnapshot{
		Identity: ProcessIdentity{PID: 5110, StartTicks: 110},
		PPID:     controller.Identity.PID,
		Exe:      "/opt/codex/bin/codex-linux-sandbox",
		Argv:     []string{"codex-linux-sandbox"},
	}
	controllerState := completeIsolationState(100, "/user.slice/codex", "0x0000000000000001")
	workerState := completeIsolationState(200, "/user.slice/codex/worker", "0x0000000000000001")
	scanner := &fakeScanner{
		processes: map[uint32]ProcessSnapshot{
			controller.Identity.PID: controller,
			worker.Identity.PID:     worker,
		},
		isolation: map[uint32]IsolationState{
			controller.Identity.PID: controllerState,
			worker.Identity.PID:     workerState,
		},
	}
	reporter := &captureReporter{}
	manager := NewManager(ManagerConfig{
		Enabled: true, BehaviorMonitorEnabled: true, HostID: "host-1",
		StateDir: t.TempDir(), ReconcileInterval: time.Hour,
		FlushInterval: time.Hour, SpoolCapacity: 32,
	}, scanner, reporter)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	manager.flush()

	var namespaceUnitID string
	for _, unit := range manager.Tracker().Units() {
		if unit.Type != IsolationLinuxNamespace {
			continue
		}
		namespaceUnitID = unit.UnitID
		if unit.IsolationBaseline.NamespaceInodes["mnt"] != 200 ||
			unit.IsolationActual.Fingerprint() != unit.IsolationBaseline.Fingerprint() ||
			unit.Completeness != "complete" {
			t.Fatalf("namespace baseline invalid: %#v", unit)
		}
	}
	if namespaceUnitID == "" {
		t.Fatal("Codex namespace worker did not become an execution unit")
	}

	changed := workerState
	changed.NamespaceInodes = map[string]uint64{"mnt": 999, "pid": 201}
	changed.CgroupPath = "/host.slice"
	changed.Availability = cloneAvailability(workerState.Availability)
	scanner.isolation[worker.Identity.PID] = changed
	if err := manager.reconcileOnce(); err != nil {
		t.Fatal(err)
	}
	manager.flush()

	var lifecycleHasBaseline, driftFound bool
	for _, event := range reporter.snapshot() {
		var body map[string]any
		_ = json.Unmarshal([]byte(event.EventDataJson), &body)
		if event.EventType == "agent_execution_unit_started" &&
			body["execution_unit_id"] == namespaceUnitID {
			_, lifecycleHasBaseline = body["isolation_baseline"]
		}
		if event.EventType == "agent_isolation_drift" {
			driftFound = true
			if body["decision"] == "deny" || body["decision"] == "deny_and_freeze" {
				t.Fatalf("P2 drift actively enforced: %s", event.EventDataJson)
			}
			evidence, ok := body["evidence"].(map[string]any)
			if !ok || evidence["baseline"] == nil || evidence["actual"] == nil || evidence["diff"] == nil {
				t.Fatalf("drift evidence incomplete: %s", event.EventDataJson)
			}
		}
	}
	if !lifecycleHasBaseline || !driftFound {
		t.Fatalf("baseline/drift missing: lifecycle=%v drift=%v", lifecycleHasBaseline, driftFound)
	}
}

func TestManagerMapsIdentityKernelIsolationAttemptsAndEscapeEvidence(t *testing.T) {
	controller := confirmedProcess(6100, 100, "/opt/codex/bin/codex", "codex")
	child := ProcessSnapshot{
		Identity: ProcessIdentity{PID: 6110, StartTicks: 110},
		PPID:     controller.Identity.PID, Exe: "/usr/bin/bash", Argv: []string{"bash"},
	}
	state := completeIsolationState(300, "/sandbox", "0x0000000000000001")
	scanner := &fakeScanner{
		processes: map[uint32]ProcessSnapshot{
			controller.Identity.PID: controller, child.Identity.PID: child,
		},
		isolation: map[uint32]IsolationState{
			controller.Identity.PID: state, child.Identity.PID: state,
		},
	}
	reporter := &captureReporter{}
	manager := NewManager(ManagerConfig{
		Enabled: true, BehaviorMonitorEnabled: true, HostID: "host-1",
		StateDir: t.TempDir(), ReconcileInterval: time.Hour,
		FlushInterval: time.Hour, SpoolCapacity: 32,
	}, scanner, reporter)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()

	for _, eventMap := range []map[string]any{
		{
			"event_id": "identity-attempt", "event_type": "agent_guard_syscall",
			"pid": int(child.Identity.PID), "security_category": "identity",
			"security_operation": "setuid", "security_target": "argument:0",
			"return_code": int64(-1),
		},
		{
			"event_id": "kernel-attempt", "event_type": "agent_guard_syscall",
			"pid": int(child.Identity.PID), "security_category": "kernel",
			"security_operation": "bpf", "security_target": "argument:5",
			"return_code": int64(-1),
		},
		{
			"event_id": "isolation-attempt", "event_type": "agent_guard_syscall",
			"pid": int(child.Identity.PID), "security_category": "isolation",
			"security_operation": "setns", "security_target": "mnt:[999]",
			"return_code": int64(-1),
		},
	} {
		if !manager.ObserveEventMap(eventMap) {
			t.Fatalf("semantic event rejected: %#v", eventMap)
		}
	}
	manager.flush()

	categories := map[string]bool{}
	violations := 0
	for _, event := range reporter.snapshot() {
		var body map[string]any
		if json.Unmarshal([]byte(event.EventDataJson), &body) != nil {
			continue
		}
		if event.EventType == "agent_behavior" {
			if category, _ := body["category"].(string); category != "" {
				categories[category] = true
			}
		}
		if event.EventType == "agent_sandbox_violation" {
			violations++
			if body["decision"] != "would_deny" {
				t.Fatalf("escape decision=%v, want would_deny", body["decision"])
			}
		}
	}
	for _, category := range []string{"identity", "kernel", "isolation"} {
		if !categories[category] {
			t.Fatalf("missing %s behavior: %#v", category, categories)
		}
	}
	if violations < 2 {
		t.Fatalf("expected kernel and namespace violations, got %d", violations)
	}
}

func completeIsolationState(mountNS uint64, cgroupPath, effective string) IsolationState {
	state := newIsolationState()
	state.NamespaceInodes = map[string]uint64{"mnt": mountNS, "pid": mountNS + 1}
	state.CgroupPath = cgroupPath
	state.CgroupVersion = 2
	state.MountInfoDigest = "sha256:mount"
	state.MountCount = 1
	state.Capabilities = CapabilityState{
		Visible: true, Inheritable: "0x0000000000000000",
		Permitted: effective, Effective: effective,
		Bounding: "0x000001ffffffffff", Ambient: "0x0000000000000000",
	}
	noNewPrivileges := true
	seccomp := 2
	state.NoNewPrivileges = &noNewPrivileges
	state.SeccompMode = &seccomp
	for _, key := range isolationDimensions {
		state.Availability[key] = EvidenceAvailability{Available: true}
	}
	return state
}
