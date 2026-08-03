package agentguard

import "testing"

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
