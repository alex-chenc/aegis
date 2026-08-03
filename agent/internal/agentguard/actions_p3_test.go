package agentguard

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	pb "aegis-agent/pkg/api/v1"

	"golang.org/x/sys/unix"
)

type fakeActionFS struct {
	mu     sync.Mutex
	files  map[string]string
	writes []string
}

func (f *fakeActionFS) ReadFile(path string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	value, ok := f.files[path]
	if !ok {
		return nil, errors.New("not found")
	}
	return []byte(value), nil
}

func (f *fakeActionFS) WriteFile(path string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.files[path]; !ok {
		return errors.New("not found")
	}
	value := strings.TrimSpace(string(data))
	f.files[path] = value
	f.writes = append(f.writes, path+"="+value)
	if strings.HasSuffix(path, "/cgroup.freeze") {
		eventsPath := strings.TrimSuffix(path, "cgroup.freeze") + "cgroup.events"
		f.files[eventsPath] = "populated 1\nfrozen " + value + "\n"
	}
	return nil
}

type fakeProcessSignaler struct {
	mu       sync.Mutex
	signals  []fakeSignal
	degraded bool
	failPID  uint32
}

type fakeSignal struct {
	identity ProcessIdentity
	signal   unix.Signal
}

func (f *fakeProcessSignaler) Signal(identity ProcessIdentity, signal unix.Signal) (SignalDelivery, error) {
	if identity.PID == f.failPID {
		return SignalDelivery{}, errors.New("signal failed")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signals = append(f.signals, fakeSignal{identity: identity, signal: signal})
	return SignalDelivery{Method: "fake_pidfd", Degraded: f.degraded}, nil
}

type fakeScheduledCall struct {
	mu       sync.Mutex
	callback func()
	canceled bool
}

func (c *fakeScheduledCall) Cancel() {
	c.mu.Lock()
	c.canceled = true
	c.mu.Unlock()
}

func (c *fakeScheduledCall) fire() {
	c.mu.Lock()
	callback, canceled := c.callback, c.canceled
	c.mu.Unlock()
	if !canceled && callback != nil {
		callback()
	}
}

type fakeActionScheduler struct {
	mu    sync.Mutex
	calls []*fakeScheduledCall
}

func (s *fakeActionScheduler) AfterFunc(_ time.Duration, callback func()) ScheduledCall {
	call := &fakeScheduledCall{callback: callback}
	s.mu.Lock()
	s.calls = append(s.calls, call)
	s.mu.Unlock()
	return call
}

func newP3ActionManager(t *testing.T, cgroup bool) (*Manager, string, string, *fakeActionFS, *fakeProcessSignaler, *fakeActionScheduler) {
	t.Helper()
	controller := confirmedProcess(5100, 100, "/opt/codex/bin/codex", "codex")
	child := ProcessSnapshot{
		Identity: ProcessIdentity{PID: 5110, StartTicks: 110},
		PPID:     controller.Identity.PID, Exe: "/usr/bin/bash", Argv: []string{"bash"},
		CgroupPath: "/aegis/unit-a",
	}
	scanner := &fakeScanner{processes: map[uint32]ProcessSnapshot{
		controller.Identity.PID: controller,
		child.Identity.PID:      child,
	}}
	actionFS := &fakeActionFS{files: map[string]string{}}
	if cgroup {
		actionFS.files["/sys/fs/cgroup/aegis/unit-a/cgroup.freeze"] = "0"
		actionFS.files["/sys/fs/cgroup/aegis/unit-a/cgroup.events"] = "populated 1\nfrozen 0\n"
		actionFS.files["/sys/fs/cgroup/aegis/unit-a/cgroup.procs"] = "5100\n5110\n"
	}
	signaler := &fakeProcessSignaler{}
	scheduler := &fakeActionScheduler{}
	capabilities := &GuardCapabilities{CgroupVersion: 2, CgroupFreeze: cgroup, Pidfd: true, BPFLSM: true}
	manager := NewManager(ManagerConfig{
		Enabled: true, BehaviorMonitorEnabled: true, EnforcementEnabled: true, FreezeEnabled: true,
		HostID: "host-p3", StateDir: t.TempDir(), Capabilities: capabilities,
		ActionFS: actionFS, ProcessSignaler: signaler, ActionScheduler: scheduler,
		CgroupRoot: "/sys/fs/cgroup", SelfPID: 9000, ParentPID: 9001,
		FreezeTimeout: time.Minute,
	}, scanner, &captureReporter{})
	manager.reconciler.Reconcile([]ProcessSnapshot{controller, child})
	instances := manager.tracker.Instances()
	if len(instances) != 1 {
		t.Fatalf("expected one instance, got %d", len(instances))
	}
	units := manager.tracker.Units()
	if len(units) != 1 {
		t.Fatalf("expected one unit, got %d", len(units))
	}
	unit := units[0]
	unit.CgroupPath = "/aegis/unit-a"
	manager.tracker.mu.Lock()
	manager.tracker.units[unit.UnitID] = unit
	manager.tracker.mu.Unlock()
	return manager, instances[0].InstanceID, unit.UnitID, actionFS, signaler, scheduler
}

func TestAgentGuardActionRejectsNonUUIDAndNeverTrustsRemoteProcessTargets(t *testing.T) {
	manager, _, _, actionFS, signaler, _ := newP3ActionManager(t, true)
	for _, target := range []string{"*", "5110", "/sys/fs/cgroup/aegis/unit-a", `{"execution_unit_id":"x"}`, "host-p3"} {
		_, err := manager.ExecuteAgentGuardAction(context.Background(), "cmd", ActionFreezeExecutionUnit, target, "test")
		if err == nil || !strings.Contains(err.Error(), "target_invalid") {
			t.Fatalf("target %q should be rejected as invalid UUID, got %v", target, err)
		}
	}
	if len(actionFS.writes) != 0 || len(signaler.signals) != 0 {
		t.Fatalf("invalid remote target caused side effects: writes=%v signals=%v", actionFS.writes, signaler.signals)
	}
}

func TestAgentGuardFreezeUsesCgroupV2ConfirmsStateIsIdempotentAndAutoResumes(t *testing.T) {
	manager, _, unitID, actionFS, signaler, scheduler := newP3ActionManager(t, true)
	result, err := manager.ExecuteAgentGuardAction(context.Background(), "cmd-1", ActionFreezeExecutionUnit, unitID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ActionStatusSuccess || result.Method != ActionMethodCgroupV2 {
		t.Fatalf("unexpected freeze result: %+v", result)
	}
	if len(actionFS.writes) != 1 || !strings.HasSuffix(actionFS.writes[0], "cgroup.freeze=1") {
		t.Fatalf("expected one cgroup freeze write, got %v", actionFS.writes)
	}
	if len(signaler.signals) != 0 {
		t.Fatalf("cgroup freeze must not signal processes: %v", signaler.signals)
	}
	if _, err := manager.ExecuteAgentGuardAction(context.Background(), "cmd-2", ActionFreezeExecutionUnit, unitID, "retry"); err != nil {
		t.Fatal(err)
	}
	if len(actionFS.writes) != 1 {
		t.Fatalf("idempotent freeze wrote again: %v", actionFS.writes)
	}
	if len(scheduler.calls) != 1 {
		t.Fatalf("expected one expiry timer, got %d", len(scheduler.calls))
	}
	scheduler.calls[0].fire()
	if len(actionFS.writes) != 2 || !strings.HasSuffix(actionFS.writes[1], "cgroup.freeze=0") {
		t.Fatalf("auto resume did not clear cgroup freeze: %v", actionFS.writes)
	}
	manager.statusMu.Lock()
	statuses := append([]*pb.RuntimeEvent(nil), manager.pendingStatus...)
	manager.statusMu.Unlock()
	var auto map[string]any
	if err := json.Unmarshal([]byte(statuses[len(statuses)-1].EventDataJson), &auto); err != nil {
		t.Fatal(err)
	}
	commandID, _ := auto["command_id"].(string)
	if auto["action"] != "auto_resume" || auto["auto_resume"] != true ||
		auto["executed"] != true || auto["state_changed"] != true ||
		!strings.HasPrefix(commandID, "AG-GUARD-") || auto["action_id"] == "" {
		t.Fatalf("auto resume must be an independent evidenced action: %v", auto)
	}
}

func TestAgentGuardFreezeFallbackUsesSavedStartTicksAndHoldCancelsAutoResume(t *testing.T) {
	manager, _, unitID, _, signaler, scheduler := newP3ActionManager(t, false)
	result, err := manager.ExecuteAgentGuardAction(context.Background(), "cmd-1", ActionFreezeExecutionUnit, unitID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != ActionMethodPIDFDFallback || !result.Degraded {
		t.Fatalf("fallback must be explicit and degraded: %+v", result)
	}
	if len(signaler.signals) != 2 {
		t.Fatalf("expected controller and child SIGSTOP, got %+v", signaler.signals)
	}
	for _, delivery := range signaler.signals {
		if delivery.signal != unix.SIGSTOP || !delivery.identity.Valid() {
			t.Fatalf("fallback did not preserve pid/start_ticks: %+v", delivery)
		}
	}
	if _, err := manager.ExecuteAgentGuardAction(context.Background(), "cmd-2", ActionHoldExecutionUnit, unitID, "investigation"); err != nil {
		t.Fatal(err)
	}
	scheduler.calls[0].fire()
	if len(signaler.signals) != 2 {
		t.Fatalf("held unit auto-resumed: %+v", signaler.signals)
	}
	if _, err := manager.ExecuteAgentGuardAction(context.Background(), "cmd-3", ActionResumeExecutionUnit, unitID, "done"); err != nil {
		t.Fatal(err)
	}
	if len(signaler.signals) != 4 || signaler.signals[2].signal != unix.SIGCONT || signaler.signals[3].signal != unix.SIGCONT {
		t.Fatalf("resume did not target saved identities: %+v", signaler.signals)
	}
}

func TestAgentGuardHoldAtomicallyFreezesActiveUnitWithoutAutoResume(t *testing.T) {
	manager, _, unitID, actionFS, signaler, scheduler := newP3ActionManager(t, true)
	result, err := manager.ExecuteAgentGuardAction(
		context.Background(), "cmd-hold", ActionHoldExecutionUnit, unitID, "investigation",
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != ActionHoldExecutionUnit || result.Status != ActionStatusSuccess ||
		result.Method != ActionMethodCgroupV2 || !result.Executed || !result.StateChanged {
		t.Fatalf("active hold did not atomically freeze: %+v", result)
	}
	if len(actionFS.writes) != 1 || len(signaler.signals) != 0 {
		t.Fatalf("active hold used unexpected mechanism: writes=%v signals=%v", actionFS.writes, signaler.signals)
	}
	if len(scheduler.calls) != 1 || !scheduler.calls[0].canceled {
		t.Fatalf("hold must cancel freeze auto-resume timer: %+v", scheduler.calls)
	}
	scheduler.calls[0].fire()
	if len(actionFS.writes) != 1 {
		t.Fatalf("held unit auto-resumed: %v", actionFS.writes)
	}
}

func TestAgentGuardActionsAreHardGatedWhenFlagsOff(t *testing.T) {
	manager, _, unitID, actionFS, signaler, _ := newP3ActionManager(t, true)
	manager.actions.freezeEnabled = false
	manager.actions.enforcementEnabled = false
	for _, action := range []string{ActionFreezeExecutionUnit, ActionHoldExecutionUnit, ActionKillExecutionUnit} {
		if _, err := manager.ExecuteAgentGuardAction(context.Background(), "cmd", action, unitID, "test"); err == nil || !strings.Contains(err.Error(), "disabled") {
			t.Fatalf("%s should be disabled, got %v", action, err)
		}
	}
	if len(actionFS.writes) != 0 || len(signaler.signals) != 0 {
		t.Fatalf("disabled action caused side effects: writes=%v signals=%v", actionFS.writes, signaler.signals)
	}
}

func TestAgentGuardFreezeAndKillRequireBothLocalSafetyFlags(t *testing.T) {
	manager, _, unitID, actionFS, signaler, _ := newP3ActionManager(t, true)
	manager.actions.enforcementEnabled = false
	if _, err := manager.ExecuteAgentGuardAction(context.Background(), "cmd", ActionFreezeExecutionUnit, unitID, "test"); err == nil || !strings.Contains(err.Error(), "enforcement_disabled") {
		t.Fatalf("freeze should require enforcement flag, got %v", err)
	}
	manager.actions.enforcementEnabled = true
	manager.actions.freezeEnabled = false
	if _, err := manager.ExecuteAgentGuardAction(context.Background(), "cmd", ActionKillExecutionUnit, unitID, "test"); err == nil || !strings.Contains(err.Error(), "freeze_disabled") {
		t.Fatalf("kill should require freeze flag, got %v", err)
	}
	if len(actionFS.writes) != 0 || len(signaler.signals) != 0 {
		t.Fatalf("single-flag action caused side effects: writes=%v signals=%v", actionFS.writes, signaler.signals)
	}
}

func TestAgentGuardProtectedTargetRejectsWholeActionBeforeSignal(t *testing.T) {
	manager, _, unitID, _, signaler, _ := newP3ActionManager(t, false)
	manager.actions.selfPID = 5110
	if _, err := manager.ExecuteAgentGuardAction(context.Background(), "cmd", ActionKillExecutionUnit, unitID, "test"); err == nil || !strings.Contains(err.Error(), "protected_target") {
		t.Fatalf("expected protected target rejection, got %v", err)
	}
	if len(signaler.signals) != 0 {
		t.Fatalf("partial destructive action occurred: %+v", signaler.signals)
	}
}

func TestAgentGuardKillInstanceResolvesOnlyLocalRegistryIdentities(t *testing.T) {
	manager, instanceID, _, _, signaler, _ := newP3ActionManager(t, false)
	result, err := manager.ExecuteAgentGuardAction(context.Background(), "cmd", ActionKillAgentInstance, instanceID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ActionStatusSuccess || len(signaler.signals) != 2 {
		t.Fatalf("unexpected kill result/signals: %+v %+v", result, signaler.signals)
	}
	for _, delivery := range signaler.signals {
		if delivery.signal != unix.SIGKILL || !delivery.identity.Valid() {
			t.Fatalf("kill did not use local pid/start_ticks identity: %+v", delivery)
		}
	}
}

func TestAgentGuardActionStatusCarriesStableIDsAndExecutionEvidence(t *testing.T) {
	manager, _, unitID, _, _, _ := newP3ActionManager(t, true)
	actionUUID := "768c5eb3-ea89-4bed-95cc-36212e87c58a"
	commandID := "AG-GUARD-" + actionUUID
	if _, err := manager.ExecuteAgentGuardAction(context.Background(), commandID, ActionFreezeExecutionUnit, unitID, "test"); err != nil {
		t.Fatal(err)
	}
	manager.statusMu.Lock()
	events := append([]*pb.RuntimeEvent(nil), manager.pendingStatus...)
	manager.statusMu.Unlock()
	if len(events) == 0 {
		t.Fatal("missing action status")
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(events[len(events)-1].EventDataJson), &body); err != nil {
		t.Fatal(err)
	}
	if body["action_id"] != actionUUID || body["command_id"] != commandID || body["executed"] != true || body["state_changed"] != true {
		t.Fatalf("action status evidence incomplete: %v", body)
	}
}
