package agentguard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

const (
	ActionFreezeExecutionUnit = "freeze_execution_unit"
	ActionResumeExecutionUnit = "resume_execution_unit"
	ActionHoldExecutionUnit   = "hold_execution_unit"
	ActionKillExecutionUnit   = "kill_execution_unit"
	ActionKillAgentInstance   = "kill_agent_instance"

	ActionStatusSuccess = "success"
	ActionStatusFailed  = "failed"

	ActionMethodCgroupV2      = "cgroup_v2_freezer"
	ActionMethodPIDFDFallback = "pidfd_sigstop_fallback"
	ActionMethodPIDFDKill     = "pidfd_sigkill"
)

type SignalDelivery struct {
	Method   string
	Degraded bool
}

type ProcessSignaler interface {
	Signal(identity ProcessIdentity, signal unix.Signal) (SignalDelivery, error)
}

type ActionFileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte) error
}

type ScheduledCall interface {
	Cancel()
}

type ActionScheduler interface {
	AfterFunc(delay time.Duration, callback func()) ScheduledCall
}

type ActionResult struct {
	ActionID        string `json:"action_id"`
	CommandID       string `json:"command_id,omitempty"`
	Action          string `json:"action"`
	InstanceID      string `json:"instance_id,omitempty"`
	ExecutionUnitID string `json:"execution_unit_id,omitempty"`
	Status          string `json:"status"`
	Method          string `json:"method,omitempty"`
	Degraded        bool   `json:"degraded"`
	ErrorCode       string `json:"error_code,omitempty"`
	AutoResume      bool   `json:"auto_resume,omitempty"`
	Executed        bool   `json:"executed"`
	StateChanged    bool   `json:"state_changed"`
}

type osActionFS struct{}

func (osActionFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (osActionFS) WriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0)
}

type realScheduledCall struct{ timer *time.Timer }

func (c realScheduledCall) Cancel() {
	if c.timer != nil {
		c.timer.Stop()
	}
}

type realActionScheduler struct{}

func (realActionScheduler) AfterFunc(delay time.Duration, callback func()) ScheduledCall {
	return realScheduledCall{timer: time.AfterFunc(delay, callback)}
}

type pidfdProcessSignaler struct {
	scanner        ProcessScanner
	pidfdSupported bool
}

func (s *pidfdProcessSignaler) Signal(identity ProcessIdentity, signal unix.Signal) (SignalDelivery, error) {
	if !identity.Valid() {
		return SignalDelivery{}, errors.New("agent_guard_process_identity_invalid")
	}
	if err := s.verify(identity); err != nil {
		return SignalDelivery{}, err
	}
	if s.pidfdSupported {
		fd, err := unix.PidfdOpen(int(identity.PID), 0)
		if err == nil {
			defer unix.Close(fd)
			if err := s.verify(identity); err != nil {
				return SignalDelivery{}, err
			}
			if err := unix.PidfdSendSignal(fd, signal, nil, 0); err != nil {
				return SignalDelivery{}, fmt.Errorf("agent_guard_pidfd_signal_failed: %w", err)
			}
			return SignalDelivery{Method: "pidfd"}, nil
		}
		if !errors.Is(err, unix.ENOSYS) && !errors.Is(err, unix.EINVAL) {
			return SignalDelivery{}, fmt.Errorf("agent_guard_pidfd_open_failed: %w", err)
		}
	}
	// Compatibility fallback is deliberately marked degraded. The identity is
	// checked immediately before kill(2), so a recycled PID is never accepted.
	if err := s.verify(identity); err != nil {
		return SignalDelivery{}, err
	}
	if err := unix.Kill(int(identity.PID), signal); err != nil {
		return SignalDelivery{}, fmt.Errorf("agent_guard_verified_pid_signal_failed: %w", err)
	}
	return SignalDelivery{Method: "verified_pid_signal", Degraded: true}, nil
}

func (s *pidfdProcessSignaler) verify(identity ProcessIdentity) error {
	process, err := s.scanner.ReadPID(identity.PID)
	if err != nil || process.Identity != identity {
		return errors.New("agent_guard_process_identity_stale")
	}
	return nil
}

type unitActionState struct {
	status     string
	method     string
	identities []ProcessIdentity
	timer      ScheduledCall
}

type ActionExecutor struct {
	mu                 sync.Mutex
	tracker            *IdentityTracker
	scanner            ProcessScanner
	fs                 ActionFileSystem
	signaler           ProcessSignaler
	scheduler          ActionScheduler
	cgroupRoot         string
	selfPID            uint32
	parentPID          uint32
	freezeTimeout      time.Duration
	freezeEnabled      bool
	enforcementEnabled bool
	cgroupFreeze       bool
	states             map[string]*unitActionState
	report             func(ActionResult)
}

func newActionExecutor(cfg ManagerConfig, tracker *IdentityTracker, scanner ProcessScanner, capabilities GuardCapabilities) *ActionExecutor {
	actionFS := cfg.ActionFS
	if actionFS == nil {
		actionFS = osActionFS{}
	}
	scheduler := cfg.ActionScheduler
	if scheduler == nil {
		scheduler = realActionScheduler{}
	}
	signaler := cfg.ProcessSignaler
	if signaler == nil {
		signaler = &pidfdProcessSignaler{scanner: scanner, pidfdSupported: capabilities.Pidfd}
	}
	cgroupRoot := cfg.CgroupRoot
	if cgroupRoot == "" {
		cgroupRoot = "/sys/fs/cgroup"
	}
	timeout := cfg.FreezeTimeout
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	selfPID := cfg.SelfPID
	if selfPID == 0 {
		selfPID = uint32(os.Getpid())
	}
	parentPID := cfg.ParentPID
	if parentPID == 0 {
		parentPID = uint32(os.Getppid())
	}
	return &ActionExecutor{
		tracker: tracker, scanner: scanner, fs: actionFS, signaler: signaler,
		scheduler: scheduler, cgroupRoot: filepath.Clean(cgroupRoot),
		selfPID: selfPID, parentPID: parentPID, freezeTimeout: timeout,
		freezeEnabled: cfg.FreezeEnabled, enforcementEnabled: cfg.EnforcementEnabled,
		cgroupFreeze: capabilities.CgroupVersion == 2 && capabilities.CgroupFreeze,
		states:       make(map[string]*unitActionState),
	}
}

func IsAgentGuardAction(action string) bool {
	switch strings.TrimSpace(action) {
	case ActionFreezeExecutionUnit, ActionResumeExecutionUnit, ActionHoldExecutionUnit,
		ActionKillExecutionUnit, ActionKillAgentInstance:
		return true
	default:
		return false
	}
}

func (e *ActionExecutor) SetFreezeTimeout(timeout time.Duration) {
	if timeout < 30*time.Second || timeout > 900*time.Second {
		return
	}
	e.mu.Lock()
	e.freezeTimeout = timeout
	e.mu.Unlock()
}

func (e *ActionExecutor) Execute(_ context.Context, commandID, action, target string) (ActionResult, error) {
	action = strings.TrimSpace(action)
	target = strings.TrimSpace(target)
	result := ActionResult{CommandID: commandID, Action: action, Status: ActionStatusFailed}
	parsed, err := uuid.Parse(target)
	if err != nil || parsed == uuid.Nil || parsed.String() != strings.ToLower(target) {
		result.ErrorCode = "agent_guard_action_target_invalid"
		return result, errors.New(result.ErrorCode)
	}
	if !IsAgentGuardAction(action) {
		result.ErrorCode = "agent_guard_action_unsupported"
		return result, errors.New(result.ErrorCode)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	var executeErr error
	switch action {
	case ActionFreezeExecutionUnit:
		result, executeErr = e.freezeLocked(commandID, target)
	case ActionResumeExecutionUnit:
		result, executeErr = e.resumeLocked(commandID, target, false)
	case ActionHoldExecutionUnit:
		result, executeErr = e.holdLocked(commandID, target)
	case ActionKillExecutionUnit:
		result, executeErr = e.killUnitLocked(commandID, target)
	case ActionKillAgentInstance:
		result, executeErr = e.killInstanceLocked(commandID, target)
	}
	return result, executeErr
}

func (e *ActionExecutor) freezeLocked(commandID, unitID string) (ActionResult, error) {
	result := ActionResult{CommandID: commandID, Action: ActionFreezeExecutionUnit, ExecutionUnitID: unitID, Status: ActionStatusFailed}
	if !e.freezeEnabled {
		return actionFailure(result, "agent_guard_freeze_disabled")
	}
	if !e.enforcementEnabled {
		return actionFailure(result, "agent_guard_enforcement_disabled")
	}
	unit, ok := e.tracker.Unit(unitID)
	if !ok {
		return actionFailure(result, "agent_guard_execution_unit_not_found")
	}
	result.InstanceID = unit.InstanceID
	if state := e.states[unitID]; state != nil {
		switch state.status {
		case "frozen", "held":
			result.Status, result.Method = ActionStatusSuccess, state.method
			result.Degraded = state.method == ActionMethodPIDFDFallback
			return result, nil
		case "freezing", "resuming", "killing":
			return actionFailure(result, "agent_guard_action_state_conflict")
		}
	}
	identities := e.tracker.ProcessesForUnit(unitID)
	if err := e.validateTargets(identities); err != nil {
		return actionFailure(result, err.Error())
	}

	state := &unitActionState{status: "freezing"}
	e.states[unitID] = state
	if e.cgroupFreeze && unit.CgroupPath != "" {
		if err := e.verifyCgroupMembership(unit, identities); err == nil {
			if err := e.setCgroupFrozen(unit, true); err == nil {
				state.status, state.method = "frozen", ActionMethodCgroupV2
				result.Status, result.Method = ActionStatusSuccess, ActionMethodCgroupV2
				result.Executed, result.StateChanged = true, true
				e.scheduleAutoResumeLocked(unitID, state)
				return result, nil
			}
			// A write may have succeeded even when confirmation failed. Best-effort
			// rollback precedes the process-level fallback.
			_ = e.setCgroupFrozen(unit, false)
		}
	}
	if len(identities) == 0 {
		delete(e.states, unitID)
		return actionFailure(result, "agent_guard_execution_unit_empty")
	}
	if err := e.signalIdentities(identities, unix.SIGSTOP); err != nil {
		_ = e.signalIdentities(identities, unix.SIGCONT)
		delete(e.states, unitID)
		return actionFailure(result, "agent_guard_freeze_fallback_failed")
	}
	// Reconcile once after SIGSTOP to cover descendants created during the
	// first pass, then stop only newly attributed identities.
	e.refreshAttribution()
	refreshed := e.tracker.ProcessesForUnit(unitID)
	additional := subtractIdentities(refreshed, identities)
	if err := e.validateTargets(additional); err != nil {
		_ = e.signalIdentities(identities, unix.SIGCONT)
		delete(e.states, unitID)
		return actionFailure(result, err.Error())
	}
	if err := e.signalIdentities(additional, unix.SIGSTOP); err != nil {
		_ = e.signalIdentities(append(identities, additional...), unix.SIGCONT)
		delete(e.states, unitID)
		return actionFailure(result, "agent_guard_freeze_rescan_failed")
	}
	state.status, state.method = "frozen", ActionMethodPIDFDFallback
	state.identities = append(identities, additional...)
	result.Status, result.Method, result.Degraded = ActionStatusSuccess, ActionMethodPIDFDFallback, true
	result.Executed, result.StateChanged = true, true
	e.scheduleAutoResumeLocked(unitID, state)
	return result, nil
}

func (e *ActionExecutor) resumeLocked(commandID, unitID string, auto bool) (ActionResult, error) {
	action := ActionResumeExecutionUnit
	if auto {
		action = "auto_resume"
	}
	result := ActionResult{CommandID: commandID, Action: action, ExecutionUnitID: unitID, Status: ActionStatusFailed, AutoResume: auto}
	unit, ok := e.tracker.Unit(unitID)
	if !ok {
		return actionFailure(result, "agent_guard_execution_unit_not_found")
	}
	result.InstanceID = unit.InstanceID
	state := e.states[unitID]
	if state == nil || state.status == "active" {
		result.Status, result.Method = ActionStatusSuccess, "already_active"
		return result, nil
	}
	if state.status == "resuming" || state.status == "freezing" || state.status == "killing" {
		return actionFailure(result, "agent_guard_action_state_conflict")
	}
	if state.timer != nil {
		state.timer.Cancel()
		state.timer = nil
	}
	state.status = "resuming"
	var err error
	switch state.method {
	case ActionMethodCgroupV2:
		err = e.setCgroupFrozen(unit, false)
	case ActionMethodPIDFDFallback:
		err = e.signalIdentities(state.identities, unix.SIGCONT)
	default:
		err = errors.New("agent_guard_freeze_state_invalid")
	}
	if err != nil {
		state.status = "frozen"
		return actionFailure(result, "agent_guard_resume_failed")
	}
	result.Status, result.Method = ActionStatusSuccess, state.method
	result.Degraded = state.method == ActionMethodPIDFDFallback
	result.Executed, result.StateChanged = true, true
	delete(e.states, unitID)
	return result, nil
}

func (e *ActionExecutor) holdLocked(commandID, unitID string) (ActionResult, error) {
	result := ActionResult{CommandID: commandID, Action: ActionHoldExecutionUnit, ExecutionUnitID: unitID, Status: ActionStatusFailed}
	if !e.freezeEnabled {
		return actionFailure(result, "agent_guard_freeze_disabled")
	}
	if !e.enforcementEnabled {
		return actionFailure(result, "agent_guard_enforcement_disabled")
	}
	unit, ok := e.tracker.Unit(unitID)
	if !ok {
		return actionFailure(result, "agent_guard_execution_unit_not_found")
	}
	result.InstanceID = unit.InstanceID
	state := e.states[unitID]
	if state == nil {
		freezeResult, err := e.freezeLocked(commandID, unitID)
		if err != nil {
			result.InstanceID = freezeResult.InstanceID
			return actionFailure(result, freezeResult.ErrorCode)
		}
		state = e.states[unitID]
		if state == nil || state.status != "frozen" {
			return actionFailure(result, "agent_guard_action_state_conflict")
		}
	}
	if state.status != "frozen" && state.status != "held" {
		return actionFailure(result, "agent_guard_action_state_conflict")
	}
	if state.timer != nil {
		state.timer.Cancel()
		state.timer = nil
	}
	state.status = "held"
	result.Status, result.Method = ActionStatusSuccess, state.method
	result.Degraded = state.method == ActionMethodPIDFDFallback
	result.Executed, result.StateChanged = true, true
	return result, nil
}

func (e *ActionExecutor) killUnitLocked(commandID, unitID string) (ActionResult, error) {
	result := ActionResult{CommandID: commandID, Action: ActionKillExecutionUnit, ExecutionUnitID: unitID, Status: ActionStatusFailed}
	if !e.enforcementEnabled {
		return actionFailure(result, "agent_guard_enforcement_disabled")
	}
	if !e.freezeEnabled {
		return actionFailure(result, "agent_guard_freeze_disabled")
	}
	unit, ok := e.tracker.Unit(unitID)
	if !ok {
		return actionFailure(result, "agent_guard_execution_unit_not_found")
	}
	result.InstanceID = unit.InstanceID
	identities := e.tracker.ProcessesForUnit(unitID)
	if len(identities) == 0 {
		result.Status, result.Method = ActionStatusSuccess, "already_stopped"
		return result, nil
	}
	if err := e.validateTargets(identities); err != nil {
		return actionFailure(result, err.Error())
	}
	if state := e.states[unitID]; state != nil && state.timer != nil {
		state.timer.Cancel()
	}
	if err := e.signalIdentities(identities, unix.SIGKILL); err != nil {
		return actionFailure(result, "agent_guard_kill_failed")
	}
	e.states[unitID] = &unitActionState{status: "killed", method: ActionMethodPIDFDKill, identities: identities}
	result.Status, result.Method = ActionStatusSuccess, ActionMethodPIDFDKill
	result.Executed, result.StateChanged = true, true
	return result, nil
}

func (e *ActionExecutor) killInstanceLocked(commandID, instanceID string) (ActionResult, error) {
	result := ActionResult{CommandID: commandID, Action: ActionKillAgentInstance, InstanceID: instanceID, Status: ActionStatusFailed}
	if !e.enforcementEnabled {
		return actionFailure(result, "agent_guard_enforcement_disabled")
	}
	if !e.freezeEnabled {
		return actionFailure(result, "agent_guard_freeze_disabled")
	}
	if _, ok := e.tracker.Instance(instanceID); !ok {
		return actionFailure(result, "agent_guard_instance_not_found")
	}
	identities := e.tracker.ProcessesForInstance(instanceID)
	if len(identities) == 0 {
		result.Status, result.Method = ActionStatusSuccess, "already_stopped"
		return result, nil
	}
	if err := e.validateTargets(identities); err != nil {
		return actionFailure(result, err.Error())
	}
	if err := e.signalIdentities(identities, unix.SIGKILL); err != nil {
		return actionFailure(result, "agent_guard_kill_failed")
	}
	for _, unit := range e.tracker.Units() {
		if unit.InstanceID != instanceID {
			continue
		}
		if state := e.states[unit.UnitID]; state != nil && state.timer != nil {
			state.timer.Cancel()
		}
		e.states[unit.UnitID] = &unitActionState{status: "killed", method: ActionMethodPIDFDKill}
	}
	result.Status, result.Method = ActionStatusSuccess, ActionMethodPIDFDKill
	result.Executed, result.StateChanged = true, true
	return result, nil
}

func actionFailure(result ActionResult, code string) (ActionResult, error) {
	result.Status = ActionStatusFailed
	result.ErrorCode = code
	return result, errors.New(code)
}

func (e *ActionExecutor) validateTargets(identities []ProcessIdentity) error {
	for _, identity := range identities {
		if identity.PID == 1 || identity.PID == e.selfPID || identity.PID == e.parentPID {
			return errors.New("agent_guard_protected_target")
		}
		process, err := e.scanner.ReadPID(identity.PID)
		if err != nil || process.Identity != identity {
			return errors.New("agent_guard_process_identity_stale")
		}
		base := strings.ToLower(filepath.Base(process.Exe))
		if process.Exe == "" || protectedProcessBasename(base) {
			return errors.New("agent_guard_protected_target")
		}
	}
	return nil
}

func protectedProcessBasename(base string) bool {
	switch base {
	case "systemd", "init", "kthreadd", "dockerd", "containerd", "containerd-shim",
		"kubelet", "aegis-agent", "aegis-server", "aegis-api-server", "aegis-dc":
		return true
	default:
		return false
	}
}

func (e *ActionExecutor) signalIdentities(identities []ProcessIdentity, signal unix.Signal) error {
	for _, identity := range uniqueIdentities(identities) {
		if _, err := e.signaler.Signal(identity, signal); err != nil {
			return err
		}
	}
	return nil
}

func (e *ActionExecutor) setCgroupFrozen(unit ExecutionUnit, frozen bool) error {
	path, err := e.localCgroupPath(unit.CgroupPath)
	if err != nil {
		return err
	}
	freezePath := filepath.Join(path, "cgroup.freeze")
	eventsPath := filepath.Join(path, "cgroup.events")
	value := "0"
	if frozen {
		value = "1"
	}
	if err := e.fs.WriteFile(freezePath, []byte(value)); err != nil {
		return fmt.Errorf("agent_guard_cgroup_freeze_write_failed: %w", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, readErr := e.fs.ReadFile(eventsPath)
		if readErr == nil && cgroupFrozenState(data) == frozen {
			return nil
		}
		if time.Now().After(deadline) {
			if !frozen {
				// A failed confirmation must not be silently treated as resumed.
				return errors.New("agent_guard_cgroup_resume_unconfirmed")
			}
			return errors.New("agent_guard_cgroup_freeze_unconfirmed")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (e *ActionExecutor) verifyCgroupMembership(unit ExecutionUnit, identities []ProcessIdentity) error {
	path, err := e.localCgroupPath(unit.CgroupPath)
	if err != nil {
		return err
	}
	data, err := e.fs.ReadFile(filepath.Join(path, "cgroup.procs"))
	if err != nil {
		return errors.New("agent_guard_cgroup_membership_unavailable")
	}
	pids, err := parseCgroupProcs(data)
	if err != nil || len(pids) == 0 {
		return errors.New("agent_guard_cgroup_membership_invalid")
	}
	allowed := make(map[uint32]ProcessIdentity, len(identities))
	for _, identity := range identities {
		allowed[identity.PID] = identity
	}
	for _, pid := range pids {
		identity, ok := allowed[pid]
		if !ok {
			return errors.New("agent_guard_cgroup_contains_foreign_process")
		}
		process, readErr := e.scanner.ReadPID(pid)
		if readErr != nil || process.Identity != identity {
			return errors.New("agent_guard_process_identity_stale")
		}
	}
	return nil
}

func (e *ActionExecutor) localCgroupPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "/" || strings.ContainsRune(raw, '\x00') {
		return "", errors.New("agent_guard_cgroup_target_invalid")
	}
	for _, segment := range strings.Split(strings.ReplaceAll(raw, "\\", "/"), "/") {
		if segment == ".." {
			return "", errors.New("agent_guard_cgroup_target_invalid")
		}
	}
	rel := strings.TrimPrefix(filepath.Clean("/"+raw), "/")
	resolved := filepath.Join(e.cgroupRoot, rel)
	check, err := filepath.Rel(e.cgroupRoot, resolved)
	if err != nil || check == "." || check == ".." || strings.HasPrefix(check, ".."+string(filepath.Separator)) {
		return "", errors.New("agent_guard_cgroup_target_invalid")
	}
	return resolved, nil
}

func cgroupFrozenState(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "frozen" {
			return fields[1] == "1"
		}
	}
	return false
}

func (e *ActionExecutor) scheduleAutoResumeLocked(unitID string, state *unitActionState) {
	timeout := e.freezeTimeout
	state.timer = e.scheduler.AfterFunc(timeout, func() {
		e.mu.Lock()
		current := e.states[unitID]
		if current != state || current.status != "frozen" {
			e.mu.Unlock()
			return
		}
		actionUUID := uuid.NewString()
		result, err := e.resumeLocked("AG-GUARD-"+actionUUID, unitID, true)
		result.ActionID = actionUUID
		report := e.report
		e.mu.Unlock()
		if err != nil {
			result.Status = ActionStatusFailed
		}
		if report != nil {
			report(result)
		}
	})
}

func (e *ActionExecutor) refreshAttribution() {
	processes, err := e.scanner.Scan()
	if err == nil {
		NewReconciler(e.tracker).Reconcile(processes)
	}
}

func uniqueIdentities(values []ProcessIdentity) []ProcessIdentity {
	seen := make(map[ProcessIdentity]struct{}, len(values))
	out := make([]ProcessIdentity, 0, len(values))
	for _, value := range values {
		if !value.Valid() {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PID == out[j].PID {
			return out[i].StartTicks < out[j].StartTicks
		}
		return out[i].PID < out[j].PID
	})
	return out
}

func subtractIdentities(values, existing []ProcessIdentity) []ProcessIdentity {
	seen := make(map[ProcessIdentity]struct{}, len(existing))
	for _, value := range existing {
		seen[value] = struct{}{}
	}
	var out []ProcessIdentity
	for _, value := range values {
		if _, ok := seen[value]; !ok {
			out = append(out, value)
		}
	}
	return uniqueIdentities(out)
}

// parseCgroupProcs is kept small and strict for future cgroup membership
// confirmation without ever accepting a server-provided PID list.
func parseCgroupProcs(data []byte) ([]uint32, error) {
	var pids []uint32
	for _, field := range strings.Fields(string(data)) {
		value, err := strconv.ParseUint(field, 10, 32)
		if err != nil || value == 0 {
			return nil, errors.New("agent_guard_cgroup_procs_invalid")
		}
		pids = append(pids, uint32(value))
	}
	return pids, nil
}
