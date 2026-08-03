package agentguard

import (
	"testing"
	"time"
)

func confirmedProcess(pid uint32, ticks uint64, exe string, args ...string) ProcessSnapshot {
	return ProcessSnapshot{
		Identity:       ProcessIdentity{PID: pid, StartTicks: ticks},
		Exe:            exe,
		Argv:           args,
		ConfigEvidence: []string{".codex"},
		UID:            1000,
	}
}

func TestTrackerForkExecExitAndPIDReuse(t *testing.T) {
	tracker := NewIdentityTracker("host-1", NewBuiltinProfileRegistry())
	controller := confirmedProcess(4100, 100, "/opt/codex/bin/codex", "codex")
	instance, ok := tracker.ObserveController(controller)
	if !ok {
		t.Fatal("expected confirmed controller")
	}

	bash := ProcessSnapshot{
		Identity: ProcessIdentity{PID: 4110, StartTicks: 110},
		PPID:     controller.Identity.PID,
		Exe:      "/usr/bin/bash",
		Argv:     []string{"bash"},
	}
	if !tracker.OnFork(controller.Identity, bash) {
		t.Fatal("expected fork label propagation")
	}
	python := ProcessSnapshot{
		Identity: ProcessIdentity{PID: 4120, StartTicks: 120},
		PPID:     bash.Identity.PID,
		Exe:      "/usr/bin/python3",
		Argv:     []string{"python3", "job.py"},
	}
	if !tracker.OnFork(bash.Identity, python) {
		t.Fatal("expected nested fork propagation")
	}
	subject, ok := tracker.LookupProcess(python.Identity)
	if !ok || subject.InstanceID != instance.InstanceID {
		t.Fatalf("nested child not attributed to controller: %#v", subject)
	}

	tracker.OnExit(python.Identity)
	reused := ProcessIdentity{PID: python.Identity.PID, StartTicks: 999}
	if _, ok := tracker.LookupProcess(reused); ok {
		t.Fatal("PID reuse inherited stale label")
	}
	if tracker.OnFork(ProcessIdentity{PID: 9999, StartTicks: 1}, ProcessSnapshot{Identity: reused}) {
		t.Fatal("unattributed process must not be claimed")
	}
}

func TestTrackerCgroupAttributionDoesNotDependOnPPID(t *testing.T) {
	tracker := NewIdentityTracker("host-1", NewBuiltinProfileRegistry())
	controller := ProcessSnapshot{
		Identity:       ProcessIdentity{PID: 5200, StartTicks: 200},
		Exe:            "/usr/bin/openclaw",
		Argv:           []string{"openclaw", "gateway"},
		ConfigEvidence: []string{".openclaw"},
	}
	instance, ok := tracker.ObserveController(controller)
	if !ok {
		t.Fatal("expected openclaw controller")
	}
	containerID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	info, ok := ParseContainerCgroup("0::/system.slice/docker-" + containerID + ".scope")
	if !ok || info.ContainerID != containerID || info.Runtime != "docker" || info.Version != 2 {
		t.Fatalf("unexpected cgroup parse: %#v", info)
	}
	unit, err := tracker.AttachContainer(instance.InstanceID, info)
	if err != nil {
		t.Fatalf("attach container: %v", err)
	}

	shimChild := ProcessSnapshot{
		Identity:   ProcessIdentity{PID: 9000, StartTicks: 900},
		PPID:       8999,
		Exe:        "/usr/bin/python3",
		CgroupPath: info.Path,
	}
	subject, ok := tracker.Attribute(shimChild)
	if !ok || subject.UnitID != unit.UnitID || subject.InstanceID != instance.InstanceID {
		t.Fatalf("container member must use cgroup attribution: %#v", subject)
	}
}

func TestParseContainerCgroupVariants(t *testing.T) {
	id := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	for name, value := range map[string]string{
		"v1":         "11:memory:/docker/" + id,
		"containerd": "0::/kubepods.slice/cri-containerd-" + id + ".scope",
		"podman":     "0::/user.slice/libpod-" + id + ".scope",
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := ParseContainerCgroup(value)
			if !ok || got.ContainerID != id {
				t.Fatalf("failed to parse %s: %#v", name, got)
			}
		})
	}
}

func TestReconcilerRepairsMissedForkWithoutClaimingUnrelatedProcess(t *testing.T) {
	tracker := NewIdentityTracker("host-1", NewBuiltinProfileRegistry())
	controller := confirmedProcess(4100, 100, "/opt/codex/bin/codex", "codex")
	instance, _ := tracker.ObserveController(controller)
	child := ProcessSnapshot{
		Identity: ProcessIdentity{PID: 4110, StartTicks: 110},
		PPID:     controller.Identity.PID,
		Exe:      "/usr/bin/bash",
	}
	ordinary := ProcessSnapshot{
		Identity: ProcessIdentity{PID: 8000, StartTicks: 800},
		PPID:     1,
		Exe:      "/usr/bin/bash",
	}
	stats := NewReconciler(tracker).Reconcile([]ProcessSnapshot{controller, child, ordinary})
	if stats.ProcessLabelsRepaired != 1 {
		t.Fatalf("expected one repair, got %#v", stats)
	}
	subject, ok := tracker.LookupProcess(child.Identity)
	if !ok || subject.InstanceID != instance.InstanceID {
		t.Fatal("missed fork not repaired")
	}
	if _, ok := tracker.LookupProcess(ordinary.Identity); ok {
		t.Fatal("ordinary shell was incorrectly attributed")
	}
}

func TestTrackerExitStopsControllerSessionAndUnitButChildExitDoesNot(t *testing.T) {
	tracker := NewIdentityTracker("host-1", NewBuiltinProfileRegistry())
	controller := confirmedProcess(4100, 100, "/opt/codex/bin/codex", "codex")
	controller.PPID = 4000
	instance, ok := tracker.ObserveController(controller)
	if !ok {
		t.Fatal("expected controller")
	}
	child := ProcessSnapshot{
		Identity: ProcessIdentity{PID: 4110, StartTicks: 110},
		PPID:     controller.Identity.PID, Exe: "/usr/bin/bash",
	}
	if !tracker.OnFork(controller.Identity, child) {
		t.Fatal("expected child attribution")
	}

	childExit, ok := tracker.ExitPID(child.Identity.PID, time.Now().UTC())
	if !ok || childExit.Process.Identity != child.Identity || childExit.InstanceStopped {
		t.Fatalf("child exit changed controller lifecycle: %#v", childExit)
	}
	if current, _ := tracker.Instance(instance.InstanceID); current.Status != "running" {
		t.Fatalf("child exit stopped instance: %#v", current)
	}

	controllerExit, ok := tracker.ExitPID(controller.Identity.PID, time.Now().UTC())
	if !ok || !controllerExit.InstanceStopped || controllerExit.Process.PPID != 4000 {
		t.Fatalf("controller exit not preserved: %#v", controllerExit)
	}
	if current, _ := tracker.Instance(instance.InstanceID); current.Status != "stopped" {
		t.Fatalf("instance status=%q", current.Status)
	}
	for _, session := range tracker.Sessions() {
		if session.InstanceID == instance.InstanceID && session.Status != "ended" {
			t.Fatalf("session status=%q", session.Status)
		}
	}
	for _, unit := range tracker.Units() {
		if unit.InstanceID == instance.InstanceID && unit.Status != "stopped" {
			t.Fatalf("unit status=%q", unit.Status)
		}
	}
}

func TestLateForkDoesNotOverwriteControllerAttribution(t *testing.T) {
	tracker := NewIdentityTracker("host-1", NewBuiltinProfileRegistry())
	parent := confirmedProcess(4100, 100, "/opt/codex/bin/codex", "codex")
	parentInstance, ok := tracker.ObserveController(parent)
	if !ok {
		t.Fatal("expected parent controller")
	}
	child := confirmedProcess(4200, 200, "/opt/codex/bin/codex", "codex")
	child.PPID = parent.Identity.PID
	childSubject := tracker.OnExec(child)
	if childSubject.InstanceID == "" || childSubject.InstanceID == parentInstance.InstanceID {
		t.Fatalf("expected child controller instance, got %#v", childSubject)
	}

	// Fork and exec are read from different eBPF maps. A delayed fork event must
	// not replace the stronger controller attribution already established by exec.
	if !tracker.OnFork(parent.Identity, child) {
		t.Fatal("expected delayed fork to be accepted without changing attribution")
	}
	exit, ok := tracker.ExitPID(child.Identity.PID, time.Now().UTC())
	if !ok || !exit.InstanceStopped || exit.Instance.InstanceID != childSubject.InstanceID {
		t.Fatalf("late fork stole controller attribution: %#v", exit)
	}
	parentAfter, ok := tracker.Instance(parentInstance.InstanceID)
	if !ok || parentAfter.Status != "running" {
		t.Fatalf("child exit changed parent lifecycle: %#v", parentAfter)
	}
}

func TestReconcilerTurnsMissingControllerIntoStoppedLifecycle(t *testing.T) {
	tracker := NewIdentityTracker("host-1", NewBuiltinProfileRegistry())
	controller := confirmedProcess(4100, 100, "/opt/codex/bin/codex", "codex")
	instance, _ := tracker.ObserveController(controller)

	reconciler := NewReconciler(tracker)
	if first := reconciler.Reconcile(nil); first.ExpiredLabelsRemoved != 0 {
		t.Fatalf("single transient scan stopped controller: %#v", first)
	}
	stats := reconciler.Reconcile(nil)
	if stats.ExpiredLabelsRemoved != 1 || len(stats.Exits) != 1 || !stats.Exits[0].InstanceStopped {
		t.Fatalf("missing process did not produce lifecycle exit: %#v", stats)
	}
	if current, _ := tracker.Instance(instance.InstanceID); current.Status != "stopped" {
		t.Fatalf("reconcile instance status=%q", current.Status)
	}
}

func TestReconcilerStopsControllerWhoseProcessLabelWasLost(t *testing.T) {
	tracker := NewIdentityTracker("host-1", NewBuiltinProfileRegistry())
	controller := confirmedProcess(4100, 100, "/opt/codex/bin/codex", "codex")
	instance, _ := tracker.ObserveController(controller)

	tracker.mu.Lock()
	delete(tracker.processLabels, controller.Identity.PID)
	tracker.mu.Unlock()

	reconciler := NewReconciler(tracker)
	if first := reconciler.Reconcile(nil); len(first.Exits) != 0 {
		t.Fatalf("single missing scan stopped orphaned controller: %#v", first)
	}
	second := reconciler.Reconcile(nil)
	if len(second.Exits) != 1 || !second.Exits[0].InstanceStopped {
		t.Fatalf("orphaned controller did not stop: %#v", second)
	}
	if current, _ := tracker.Instance(instance.InstanceID); current.Status != "stopped" {
		t.Fatalf("orphaned instance status=%q", current.Status)
	}
}

func TestReconcilerStopsOldInstanceBeforeAttributingReusedControllerPID(t *testing.T) {
	tracker := NewIdentityTracker("host-1", NewBuiltinProfileRegistry())
	oldController := confirmedProcess(4100, 100, "/opt/codex/bin/codex", "codex")
	oldInstance, _ := tracker.ObserveController(oldController)
	newController := confirmedProcess(4100, 900, "/opt/codex/bin/codex", "codex")

	stats := NewReconciler(tracker).Reconcile([]ProcessSnapshot{newController})
	if stats.ExpiredLabelsRemoved != 1 || stats.ControllersDiscovered != 1 ||
		len(stats.Exits) != 1 || !stats.Exits[0].InstanceStopped {
		t.Fatalf("PID reuse lifecycle=%#v", stats)
	}
	if current, _ := tracker.Instance(oldInstance.InstanceID); current.Status != "stopped" {
		t.Fatalf("old instance status=%q", current.Status)
	}
	newSubject, ok := tracker.LookupProcess(newController.Identity)
	if !ok || newSubject.InstanceID == oldInstance.InstanceID {
		t.Fatalf("new controller reused old attribution: %#v", newSubject)
	}
}
