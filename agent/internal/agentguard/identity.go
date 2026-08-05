package agentguard

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type processLabel struct {
	Identity ProcessIdentity
	Process  ProcessSnapshot
	Subject  GuardSubject
}

// ProcessExitResult is the last attributed process snapshot plus any owning
// lifecycle entities that transitioned because the process exited.
type ProcessExitResult struct {
	Process         ProcessSnapshot
	Subject         GuardSubject
	Instance        RuntimeInstance
	Session         BehaviorSession
	Unit            ExecutionUnit
	InstanceStopped bool
}

type IdentityTracker struct {
	hostID        string
	profiles      *ProfileRegistry
	mu            sync.RWMutex
	instances     map[string]RuntimeInstance
	sessions      map[string]BehaviorSession
	units         map[string]ExecutionUnit
	processLabels map[uint32]processLabel
	cgroupLabels  map[string]GuardSubject
}

func NewIdentityTracker(hostID string, profiles *ProfileRegistry) *IdentityTracker {
	if profiles == nil {
		profiles = NewBuiltinProfileRegistry()
	}
	return &IdentityTracker{
		hostID:        hostID,
		profiles:      profiles,
		instances:     make(map[string]RuntimeInstance),
		sessions:      make(map[string]BehaviorSession),
		units:         make(map[string]ExecutionUnit),
		processLabels: make(map[uint32]processLabel),
		cgroupLabels:  make(map[string]GuardSubject),
	}
}

func (t *IdentityTracker) ObserveController(process ProcessSnapshot) (RuntimeInstance, bool) {
	if !process.Identity.Valid() {
		return RuntimeInstance{}, false
	}
	match := t.profiles.MatchController(process)
	if match.Profile == nil || match.Confidence != ConfidenceConfirmed {
		return RuntimeInstance{}, false
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	return t.observeControllerLocked(process, match, "")
}

func (t *IdentityTracker) observeControllerLocked(process ProcessSnapshot, match ProfileMatch, launchedBy string) (RuntimeInstance, bool) {
	instanceID := stableID("instance", t.hostID, process.Identity.PID, process.Identity.StartTicks)
	if existing, ok := t.instances[instanceID]; ok {
		existing.LastSeenAt = time.Now().UTC()
		existing.Status = "running"
		t.instances[instanceID] = existing
		t.processLabels[process.Identity.PID] = processLabel{
			Identity: process.Identity,
			Process:  process,
			Subject:  defaultControllerSubject(instanceID, match.Confidence),
		}
		return existing, true
	}

	now := time.Now().UTC()
	coverage := CoverageMonitorOnly
	if match.Profile.SandboxFamily == IsolationLocalProcessTree {
		coverage = CoverageNoIsolation
	}
	instance := RuntimeInstance{
		InstanceID:         instanceID,
		HostID:             t.hostID,
		ProfileKey:         match.Profile.ProfileKey,
		ProfileVersion:     match.Profile.ProfileVersion,
		AgentType:          match.Profile.AgentType,
		DisplayName:        match.Profile.DisplayName,
		Controller:         process.Identity,
		ControllerExe:      process.Exe,
		RunUID:             process.UID,
		Confidence:         match.Confidence,
		Status:             "running",
		Coverage:           coverage,
		LaunchedByInstance: launchedBy,
		FirstSeenAt:        now,
		LastSeenAt:         now,
	}
	// Runtime discovery is not a session boundary. Until a signed Codex
	// SessionStart hook arrives, keep only the instance/process identity and do
	// not fabricate an activity_window session for behavior collection.
	subject := defaultControllerSubject(instanceID, match.Confidence)
	t.instances[instanceID] = instance
	t.processLabels[process.Identity.PID] = processLabel{Identity: process.Identity, Process: process, Subject: subject}
	return instance, true
}

func defaultControllerSubject(instanceID string, confidence Confidence) GuardSubject {
	return GuardSubject{
		InstanceID: instanceID,
		Confidence: confidence,
	}
}

func (t *IdentityTracker) OnFork(parent ProcessIdentity, child ProcessSnapshot) bool {
	if !parent.Valid() || !child.Identity.Valid() {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	label, ok := t.processLabels[parent.PID]
	if !ok || label.Identity.StartTicks != parent.StartTicks {
		return false
	}
	// Fork and exec are consumed from separate eBPF maps, so a short-lived
	// controller's exec can be attributed before its fork event is delivered.
	// Preserve any attribution already established for the same process epoch;
	// in particular, never replace a controller's own subject with its parent's.
	if current, exists := t.processLabels[child.Identity.PID]; exists && current.Identity == child.Identity {
		return true
	}
	t.processLabels[child.Identity.PID] = processLabel{Identity: child.Identity, Process: child, Subject: label.Subject}
	t.touchLocked(label.Subject, time.Now().UTC())
	return true
}

func (t *IdentityTracker) OnExec(process ProcessSnapshot) GuardSubject {
	t.mu.Lock()
	defer t.mu.Unlock()
	if current, ok := t.processLabels[process.Identity.PID]; ok && current.Identity.StartTicks == process.Identity.StartTicks {
		// A forked Codex process can remain executable as "codex" briefly before
		// execve installs the actual tool binary. It is still a descendant of the
		// existing controller/session and must not become another runtime instance.
		current.Process = process
		t.processLabels[process.Identity.PID] = current
		t.touchLocked(current.Subject, time.Now().UTC())
		return current.Subject
	}
	// sched_process_exec may be consumed before the matching fork event. PPID
	// from procfs is sufficient to inherit an already confirmed controller or
	// Hook session and prevents the transient forked `codex` image from being
	// registered as a second runtime instance.
	if parent, ok := t.processLabels[process.PPID]; ok && parent.Identity.PID == process.PPID {
		t.processLabels[process.Identity.PID] = processLabel{
			Identity: process.Identity, Process: process, Subject: parent.Subject,
		}
		t.touchLocked(parent.Subject, time.Now().UTC())
		return parent.Subject
	}
	match := t.profiles.MatchController(process)
	if match.Profile != nil && match.Confidence == ConfidenceConfirmed {
		instance, _ := t.observeControllerLocked(process, match, "")
		return t.processLabels[instance.Controller.PID].Subject
	}
	return GuardSubject{Confidence: ConfidenceUnattributed}
}

func (t *IdentityTracker) OnExit(identity ProcessIdentity) {
	t.mu.RLock()
	label, ok := t.processLabels[identity.PID]
	t.mu.RUnlock()
	if !ok || label.Identity.StartTicks != identity.StartTicks {
		return
	}
	_, _ = t.ExitPID(identity.PID, time.Now().UTC())
}

// ExitPID completes attribution without reading /proc. sched_process_exit is
// delivered after the process may already be unreadable, so the tracker keeps
// the last trusted snapshot captured at fork/exec/reconcile time.
func (t *IdentityTracker) ExitPID(pid uint32, observedAt time.Time) (ProcessExitResult, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	label, ok := t.processLabels[pid]
	if !ok {
		return ProcessExitResult{}, false
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	delete(t.processLabels, pid)
	result := ProcessExitResult{Process: label.Process, Subject: label.Subject}
	if result.Process.Identity != label.Identity {
		result.Process.Identity = label.Identity
	}
	if instance, exists := t.instances[label.Subject.InstanceID]; exists && instance.Controller == label.Identity {
		instance.Status = "stopped"
		instance.LastSeenAt = observedAt
		t.instances[instance.InstanceID] = instance
		result.Instance = instance
		result.InstanceStopped = true
		if session, exists := t.sessions[label.Subject.SessionID]; exists {
			session.Status = "ended"
			session.LastSeenAt = observedAt
			t.sessions[session.SessionID] = session
			result.Session = session
		}
		if unit, exists := t.units[label.Subject.UnitID]; exists {
			unit.Status = "stopped"
			unit.LastSeenAt = observedAt
			t.units[unit.UnitID] = unit
			result.Unit = unit
		}
	}
	return result, true
}

// ExitController closes a controller lifecycle even when its process label was
// lost due to cross-map event reordering. Reconciliation calls this with the
// full PID epoch, so a reused PID cannot stop a newer instance.
func (t *IdentityTracker) ExitController(identity ProcessIdentity, observedAt time.Time) (ProcessExitResult, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	instanceID := stableID("instance", t.hostID, identity.PID, identity.StartTicks)
	instance, ok := t.instances[instanceID]
	if !ok || instance.Controller != identity || instance.Status != "running" {
		return ProcessExitResult{}, false
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	process := ProcessSnapshot{
		Identity: identity,
		Exe:      instance.ControllerExe,
		UID:      instance.RunUID,
	}
	if label, exists := t.processLabels[identity.PID]; exists && label.Identity == identity {
		process = label.Process
	}
	subject := defaultControllerSubject(instanceID, instance.Confidence)
	result := ProcessExitResult{Process: process, Subject: subject, InstanceStopped: true}
	if label, exists := t.processLabels[identity.PID]; exists && label.Identity == identity {
		subject = label.Subject
		result.Subject = subject
		delete(t.processLabels, identity.PID)
	} else {
		// The controller can disappear between procfs reconciliation passes. In
		// that case recover the active hook session from its pinned unit so a
		// dead Codex root cannot leave the real session active forever.
		for _, candidate := range t.sessions {
			if candidate.InstanceID != instanceID || candidate.Status != "active" || !isTrustedSession(candidate) {
				continue
			}
			for _, unit := range t.units {
				if unit.InstanceID == instanceID && unit.SessionID == candidate.SessionID && unit.RootProcess == identity {
					subject = GuardSubject{
						InstanceID: instanceID, SessionID: candidate.SessionID,
						UnitID: unit.UnitID, Confidence: ConfidenceConfirmed,
					}
					result.Subject = subject
					break
				}
			}
			if result.Subject.SessionID != "" {
				break
			}
		}
	}
	instance.Status = "stopped"
	instance.LastSeenAt = observedAt
	t.instances[instanceID] = instance
	result.Instance = instance
	if session, exists := t.sessions[subject.SessionID]; exists {
		session.Status = "ended"
		session.LastSeenAt = observedAt
		t.sessions[subject.SessionID] = session
		result.Session = session
	}
	if unit, exists := t.units[subject.UnitID]; exists {
		unit.Status = "stopped"
		unit.LastSeenAt = observedAt
		t.units[subject.UnitID] = unit
		result.Unit = unit
	}
	if subject.SessionID != "" {
		for pid, label := range t.processLabels {
			if label.Subject.SessionID == subject.SessionID {
				delete(t.processLabels, pid)
			}
		}
	}
	return result, true
}

func (t *IdentityTracker) LookupProcess(identity ProcessIdentity) (GuardSubject, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	label, ok := t.processLabels[identity.PID]
	if !ok || label.Identity.StartTicks != identity.StartTicks {
		return GuardSubject{}, false
	}
	return label.Subject, true
}

func (t *IdentityTracker) ProcessByPID(pid uint32) (ProcessSnapshot, GuardSubject, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	label, ok := t.processLabels[pid]
	if !ok {
		return ProcessSnapshot{}, GuardSubject{}, false
	}
	process := label.Process
	if process.Identity != label.Identity {
		process.Identity = label.Identity
	}
	return process, label.Subject, true
}

func (t *IdentityTracker) RefreshProcess(process ProcessSnapshot, observedAt time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	label, ok := t.processLabels[process.Identity.PID]
	if !ok || label.Identity.StartTicks != process.Identity.StartTicks {
		return false
	}
	label.Process = process
	t.processLabels[process.Identity.PID] = label
	t.touchLocked(label.Subject, observedAt)
	return true
}

func (t *IdentityTracker) Attribute(process ProcessSnapshot) (GuardSubject, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if process.CgroupPath != "" {
		if subject, ok := t.cgroupLabels[normalizeCgroupPath(process.CgroupPath)]; ok {
			return subject, true
		}
		if info, ok := ParseContainerCgroup(process.CgroupPath); ok {
			if subject, ok := t.cgroupLabels["container:"+info.ContainerID]; ok {
				return subject, true
			}
		}
	}
	label, ok := t.processLabels[process.Identity.PID]
	if !ok || label.Identity.StartTicks != process.Identity.StartTicks {
		return GuardSubject{}, false
	}
	return label.Subject, true
}

func (t *IdentityTracker) AttachContainer(instanceID string, info ContainerCgroup) (ExecutionUnit, error) {
	if info.ContainerID == "" || info.Path == "" {
		return ExecutionUnit{}, fmt.Errorf("container cgroup identity is incomplete")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	instance, ok := t.instances[instanceID]
	if !ok {
		return ExecutionUnit{}, fmt.Errorf("instance %s not found", instanceID)
	}
	now := time.Now().UTC()
	sessionID := t.activeTrustedSessionIDLocked(instanceID)
	unitID := stableID("unit", instanceID, info.ContainerID)
	unit := ExecutionUnit{
		UnitID: unitID, InstanceID: instanceID, SessionID: sessionID,
		Type: IsolationOCIContainer, RootProcess: ProcessIdentity{},
		CgroupPath: info.Path, ContainerID: info.ContainerID, ContainerRuntime: info.Runtime,
		Coverage: CoverageMonitorOnly, Status: "observed", FirstSeenAt: now, LastSeenAt: now,
	}
	subject := GuardSubject{
		InstanceID: instance.InstanceID, SessionID: sessionID,
		UnitID: unitID, Confidence: ConfidenceConfirmed,
	}
	t.units[unitID] = unit
	t.cgroupLabels[normalizeCgroupPath(info.Path)] = subject
	t.cgroupLabels["container:"+info.ContainerID] = subject
	return unit, nil
}

func (t *IdentityTracker) AttachNamespace(instanceID string, process ProcessSnapshot, state IsolationState) (ExecutionUnit, error) {
	if !process.Identity.Valid() || len(state.NamespaceInodes) == 0 {
		return ExecutionUnit{}, fmt.Errorf("namespace identity is incomplete")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	instance, ok := t.instances[instanceID]
	if !ok {
		return ExecutionUnit{}, fmt.Errorf("instance %s not found", instanceID)
	}
	fingerprint := namespaceFingerprint(state)
	sessionID := t.activeTrustedSessionIDLocked(instanceID)
	unitID := stableID("unit", instanceID, "namespace", fingerprint)
	now := time.Now().UTC()
	unit, exists := t.units[unitID]
	if !exists {
		unit = ExecutionUnit{
			UnitID: unitID, InstanceID: instanceID, SessionID: sessionID,
			Type: IsolationLinuxNamespace, RootProcess: process.Identity,
			CgroupPath: state.CgroupPath, ContainerID: state.ContainerID,
			ContainerRuntime: state.ContainerRuntime, Coverage: CoverageMonitorOnly,
			IsolationBaseline: state, IsolationActual: state,
			IsolationDiff: IsolationDiff{Changes: map[string]StateDifference{}, Unavailable: unavailableIsolationDimensions(state)},
			Completeness:  state.Completeness(), Status: "observed",
			FirstSeenAt: now, LastSeenAt: now,
		}
		t.units[unitID] = unit
	} else if unit.SessionID != sessionID {
		// A namespace can be discovered before SessionStart. Once a real hook
		// session is active, bind the existing unit to that session instead of
		// inventing an execution_unit session that could leak unattributed data.
		unit.SessionID = sessionID
		unit.LastSeenAt = now
		t.units[unitID] = unit
	}
	subject := GuardSubject{
		InstanceID: instance.InstanceID, SessionID: sessionID,
		UnitID: unitID, Confidence: ConfidenceConfirmed,
	}
	t.processLabels[process.Identity.PID] = processLabel{
		Identity: process.Identity, Process: process, Subject: subject,
	}
	return unit, nil
}

// activeTrustedSessionIDLocked returns the real external-session-backed
// session currently owning an instance. Runtime/isolation discovery may run
// before the lifecycle hook, so it must never manufacture a session ID of its
// own. An empty result deliberately leaves the unit unable to pass the
// behavior-event trust gate until SessionStart arrives.
func (t *IdentityTracker) activeTrustedSessionIDLocked(instanceID string) string {
	var selected BehaviorSession
	for _, session := range t.sessions {
		if session.InstanceID != instanceID || session.Status != "active" || !isTrustedSession(session) {
			continue
		}
		if selected.SessionID == "" || session.LastSeenAt.After(selected.LastSeenAt) {
			selected = session
		}
	}
	return selected.SessionID
}

func (t *IdentityTracker) AssignProcessToUnit(process ProcessSnapshot, unitID string) bool {
	if !process.Identity.Valid() {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	unit, ok := t.units[unitID]
	if !ok {
		return false
	}
	current, ok := t.processLabels[process.Identity.PID]
	if !ok || current.Identity.StartTicks != process.Identity.StartTicks ||
		current.Subject.InstanceID != unit.InstanceID {
		return false
	}
	current.Subject.UnitID = unit.UnitID
	current.Subject.SessionID = unit.SessionID
	current.Process = process
	t.processLabels[process.Identity.PID] = current
	t.touchLocked(current.Subject, time.Now().UTC())
	return true
}

func (t *IdentityTracker) UpdateUnitIsolation(
	unitID string,
	actual IsolationState,
	capabilities GuardCapabilities,
) (ExecutionUnit, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	unit, ok := t.units[unitID]
	if !ok {
		return ExecutionUnit{}, false
	}
	hadBaseline := !unit.IsolationBaseline.CapturedAt.IsZero()
	if !hadBaseline {
		unit.IsolationBaseline = actual
	}
	unit.IsolationActual = actual
	unit.IsolationDiff = DiffIsolationState(unit.IsolationBaseline, actual)
	unit.Capabilities = capabilities
	unit.Completeness = actual.Completeness()
	unit.CgroupPath = actual.CgroupPath
	if unit.ContainerID == "" {
		unit.ContainerID = actual.ContainerID
	}
	if unit.ContainerRuntime == "" {
		unit.ContainerRuntime = actual.ContainerRuntime
	}
	if unit.Type != IsolationLocalProcessTree {
		if unit.Completeness == "complete" {
			unit.Coverage = CoverageMonitorOnly
		} else {
			unit.Coverage = CoverageDegraded
		}
	}
	unit.LastSeenAt = time.Now().UTC()
	t.units[unitID] = unit
	return unit, hadBaseline
}

func namespaceFingerprint(state IsolationState) string {
	var builder strings.Builder
	keys := make([]string, 0, len(state.NamespaceInodes))
	for key := range state.NamespaceInodes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&builder, "%s=%d;", key, state.NamespaceInodes[key])
	}
	return stableID("namespace", builder.String())
}

func (t *IdentityTracker) Instances() []RuntimeInstance {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]RuntimeInstance, 0, len(t.instances))
	for _, instance := range t.instances {
		out = append(out, instance)
	}
	return out
}

func (t *IdentityTracker) Instance(instanceID string) (RuntimeInstance, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	instance, ok := t.instances[instanceID]
	return instance, ok
}

func (t *IdentityTracker) Unit(unitID string) (ExecutionUnit, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	unit, ok := t.units[unitID]
	return unit, ok
}

// ProcessesForUnit returns only locally attributed PID/start_ticks identities.
// Callers must still re-read procfs before a destructive action.
func (t *IdentityTracker) ProcessesForUnit(unitID string) []ProcessIdentity {
	t.mu.RLock()
	defer t.mu.RUnlock()
	identities := make([]ProcessIdentity, 0)
	for _, label := range t.processLabels {
		if label.Subject.UnitID == unitID {
			identities = append(identities, label.Identity)
		}
	}
	return uniqueIdentities(identities)
}

// ProcessesForInstance returns only identities attributed to this instance in
// the local registry. It never accepts or expands a remote PID/path selector.
func (t *IdentityTracker) ProcessesForInstance(instanceID string) []ProcessIdentity {
	t.mu.RLock()
	defer t.mu.RUnlock()
	identities := make([]ProcessIdentity, 0)
	for _, label := range t.processLabels {
		if label.Subject.InstanceID == instanceID {
			identities = append(identities, label.Identity)
		}
	}
	return uniqueIdentities(identities)
}

// KernelSubjects returns only confirmed, locally attributed PID identities.
// Ambiguous/candidate labels are intentionally absent from enforcement maps.
func (t *IdentityTracker) KernelSubjects(policy CompiledKernelPolicy) []KernelSubject {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]KernelSubject, 0, len(t.processLabels))
	for pid, label := range t.processLabels {
		if label.Subject.Confidence != ConfidenceConfirmed {
			continue
		}
		instance, ok := t.instances[label.Subject.InstanceID]
		if !ok || instance.Confidence != ConfidenceConfirmed {
			continue
		}
		policySlot, ok := policy.PolicySlotFor(instance)
		if !ok {
			continue
		}
		result = append(result, KernelSubject{
			PID: pid, InstanceSlot: stableKernelSlot("instance", label.Subject.InstanceID),
			UnitSlot:   stableKernelSlot("unit", label.Subject.UnitID),
			PolicySlot: policySlot, ProcessEpoch: label.Identity.StartTicks,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PID < result[j].PID })
	return result
}

func (t *IdentityTracker) Sessions() []BehaviorSession {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]BehaviorSession, 0, len(t.sessions))
	for _, session := range t.sessions {
		out = append(out, session)
	}
	return out
}

func (t *IdentityTracker) ObserveTrustedSession(
	subject GuardSubject,
	source, correlationHash, externalSessionID string,
) (BehaviorSession, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now().UTC()
	session, exists := t.sessions[subject.SessionID]
	if !exists || !isTrustedSession(session) || session.Status != "active" ||
		session.ExternalSessionID != externalSessionID || externalSessionID == "" {
		return BehaviorSession{}, false
	}
	changed := session.Source != source || session.Confidence != ConfidenceConfirmed ||
		session.CorrelationTokenHash != correlationHash || session.ExternalSessionID != externalSessionID
	session.Source = source
	session.Confidence = ConfidenceConfirmed
	session.CorrelationTokenHash = correlationHash
	session.ExternalSessionID = externalSessionID
	session.LastSeenAt = now
	t.sessions[subject.SessionID] = session
	return session, changed
}

// StartTrustedSession creates a real product session from an authenticated
// lifecycle hook. The first process observed by that hook becomes the root of
// a dedicated local process tree; later fork/exec attribution inherits this
// session and unit from the root label.
func (t *IdentityTracker) StartTrustedSession(
	process ProcessSnapshot,
	subject GuardSubject,
	source, externalSessionID string,
	observedAt time.Time,
) (BehaviorSession, ExecutionUnit, bool, error) {
	if !process.Identity.Valid() || !validExternalSessionID(externalSessionID) || externalSessionID == "" ||
		!trustedLifecycleSource(source) {
		return BehaviorSession{}, ExecutionUnit{}, false, fmt.Errorf("trusted session identity is incomplete")
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	instance, ok := t.instances[subject.InstanceID]
	if !ok || instance.Confidence != ConfidenceConfirmed || instance.Status != "running" {
		return BehaviorSession{}, ExecutionUnit{}, false, fmt.Errorf("trusted session runtime instance is unavailable")
	}
	if label, exists := t.processLabels[process.Identity.PID]; exists &&
		(label.Identity != process.Identity || label.Subject.InstanceID != instance.InstanceID) {
		return BehaviorSession{}, ExecutionUnit{}, false, fmt.Errorf("trusted session root attribution conflicts")
	}
	if process.Identity != instance.Controller {
		label, exists := t.processLabels[process.Identity.PID]
		if !exists || label.Identity != process.Identity || label.Subject.InstanceID != instance.InstanceID {
			return BehaviorSession{}, ExecutionUnit{}, false, fmt.Errorf("trusted session root is outside runtime instance")
		}
	}

	sessionID := stableID("session", instance.InstanceID, source, externalSessionID)
	unitID := stableID("unit", sessionID, string(IsolationLocalProcessTree))
	session, exists := t.sessions[sessionID]
	changed := !exists || session.Status != "active"
	if !exists {
		session = BehaviorSession{
			SessionID: sessionID, InstanceID: instance.InstanceID,
			ExternalSessionID: externalSessionID, Source: source,
			Confidence: ConfidenceConfirmed, Status: "active",
			FirstSeenAt: observedAt, LastSeenAt: observedAt,
		}
	} else {
		session.ExternalSessionID = externalSessionID
		session.Source = source
		session.Confidence = ConfidenceConfirmed
		session.Status = "active"
		session.LastSeenAt = observedAt
	}
	unit, unitExists := t.units[unitID]
	if !unitExists || unit.Status != "observed" || unit.RootProcess != process.Identity {
		changed = true
	}
	if !unitExists {
		unit = ExecutionUnit{
			UnitID: unitID, InstanceID: instance.InstanceID, SessionID: sessionID,
			Type: IsolationLocalProcessTree, RootProcess: process.Identity,
			Coverage: instance.Coverage, Status: "observed",
			FirstSeenAt: observedAt, LastSeenAt: observedAt,
		}
	} else {
		unit.RootProcess = process.Identity
		unit.Status = "observed"
		unit.LastSeenAt = observedAt
	}
	rootSubject := GuardSubject{
		InstanceID: instance.InstanceID, SessionID: sessionID, UnitID: unitID,
		Confidence: ConfidenceConfirmed,
	}
	t.sessions[sessionID] = session
	t.units[unitID] = unit
	// The Hook root may already have descendants that were discovered before
	// SessionStart. Rebind the complete currently attributed runtime tree so
	// the whole session uses one real session and execution-unit identity.
	for pid, label := range t.processLabels {
		if label.Subject.InstanceID != instance.InstanceID {
			continue
		}
		label.Subject = rootSubject
		t.processLabels[pid] = label
	}
	t.processLabels[process.Identity.PID] = processLabel{
		Identity: process.Identity, Process: process, Subject: rootSubject,
	}
	return session, unit, changed, nil
}

// EndTrustedSession closes only the matching product session and execution
// unit. A shared Codex controller remains a running runtime instance and is
// restored to its default activity attribution for future sessions.
func (t *IdentityTracker) EndTrustedSession(
	source, externalSessionID string,
	root ProcessIdentity,
	observedAt time.Time,
) (BehaviorSession, ExecutionUnit, bool, error) {
	if !root.Valid() || !validExternalSessionID(externalSessionID) || externalSessionID == "" ||
		!trustedLifecycleSource(source) {
		return BehaviorSession{}, ExecutionUnit{}, false, fmt.Errorf("trusted session identity is incomplete")
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	var session BehaviorSession
	for _, candidate := range t.sessions {
		if candidate.Source == source && candidate.ExternalSessionID == externalSessionID {
			candidateUnitID := stableID("unit", candidate.SessionID, string(IsolationLocalProcessTree))
			if candidateUnit, ok := t.units[candidateUnitID]; ok && candidateUnit.RootProcess == root {
				session = candidate
				break
			}
		}
	}
	if session.SessionID == "" {
		return BehaviorSession{}, ExecutionUnit{}, false, fmt.Errorf("trusted session was not found")
	}
	unitID := stableID("unit", session.SessionID, string(IsolationLocalProcessTree))
	unit, ok := t.units[unitID]
	if !ok {
		return BehaviorSession{}, ExecutionUnit{}, false, fmt.Errorf("trusted session unit was not found")
	}
	if session.Status == "ended" && unit.Status == "stopped" {
		return session, unit, false, nil
	}
	session.Status = "ended"
	session.LastSeenAt = observedAt
	unit.Status = "stopped"
	unit.LastSeenAt = observedAt
	t.sessions[session.SessionID] = session
	t.units[unit.UnitID] = unit

	instance := t.instances[session.InstanceID]
	for pid, label := range t.processLabels {
		if label.Subject.SessionID != session.SessionID {
			continue
		}
		if label.Identity == instance.Controller {
			label.Subject = defaultControllerSubject(instance.InstanceID, instance.Confidence)
			t.processLabels[pid] = label
			continue
		}
		delete(t.processLabels, pid)
	}
	return session, unit, true, nil
}

func trustedLifecycleSource(source string) bool {
	switch source {
	case ToolSourceAgentOfficial, ToolSourceAdapterHook, ToolSourceAegisWrapper:
		return true
	default:
		return false
	}
}

func isTrustedSession(session BehaviorSession) bool {
	return trustedLifecycleSource(session.Source) &&
		session.Confidence == ConfidenceConfirmed && session.ExternalSessionID != ""
}

// TrustedSession is the fail-closed gate used by behavior normalization. A
// process must point at an active, hook-confirmed session and its matching
// execution unit before any event can enter the runtime stream.
func (t *IdentityTracker) TrustedSession(subject GuardSubject) (BehaviorSession, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if subject.SessionID == "" || subject.UnitID == "" {
		return BehaviorSession{}, false
	}
	session, ok := t.sessions[subject.SessionID]
	if !ok || session.Status != "active" || !isTrustedSession(session) {
		return BehaviorSession{}, false
	}
	unit, ok := t.units[subject.UnitID]
	if !ok || unit.SessionID != subject.SessionID || unit.Status != "observed" {
		return BehaviorSession{}, false
	}
	return session, true
}

func (t *IdentityTracker) Units() []ExecutionUnit {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]ExecutionUnit, 0, len(t.units))
	for _, unit := range t.units {
		out = append(out, unit)
	}
	return out
}

func (t *IdentityTracker) Session(sessionID string) (BehaviorSession, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	session, ok := t.sessions[sessionID]
	return session, ok
}

func (t *IdentityTracker) touchLocked(subject GuardSubject, now time.Time) {
	if instance, ok := t.instances[subject.InstanceID]; ok {
		instance.LastSeenAt = now
		t.instances[subject.InstanceID] = instance
	}
	if session, ok := t.sessions[subject.SessionID]; ok {
		session.LastSeenAt = now
		t.sessions[subject.SessionID] = session
	}
	if unit, ok := t.units[subject.UnitID]; ok {
		unit.LastSeenAt = now
		t.units[subject.UnitID] = unit
	}
}

var containerIDPattern = regexp.MustCompile(`(?i)(?:^|/|(?:docker|cri-containerd|crio|libpod)-)([a-f0-9]{64})(?:\.scope)?(?:$|/)`)

func ParseContainerCgroup(raw string) (ContainerCgroup, bool) {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		path := line
		version := 0
		if len(parts) == 3 {
			path = parts[2]
			if parts[0] == "0" && parts[1] == "" {
				version = 2
			} else {
				version = 1
			}
		}
		match := containerIDPattern.FindStringSubmatch(path)
		if len(match) != 2 {
			continue
		}
		runtime := "containerd"
		lower := strings.ToLower(path)
		switch {
		case strings.Contains(lower, "libpod-"):
			runtime = "podman"
		case strings.Contains(lower, "docker"):
			runtime = "docker"
		case strings.Contains(lower, "crio-"):
			runtime = "cri-o"
		}
		return ContainerCgroup{
			Version: version, Runtime: runtime,
			ContainerID: strings.ToLower(match[1]), Path: normalizeCgroupPath(path),
		}, true
	}
	return ContainerCgroup{}, false
}

func normalizeCgroupPath(path string) string {
	parts := strings.SplitN(strings.TrimSpace(path), ":", 3)
	if len(parts) == 3 {
		path = parts[2]
	}
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimSuffix(path, "/")
}

func stableID(parts ...any) string {
	var builder strings.Builder
	for _, part := range parts {
		fmt.Fprint(&builder, part, "\x00")
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(builder.String())).String()
}

type ReconcileStats struct {
	ControllersDiscovered uint64
	ProcessLabelsRepaired uint64
	ExpiredLabelsRemoved  uint64
	ContainersAttached    uint64
	Exits                 []ProcessExitResult
}

type Reconciler struct {
	tracker       *IdentityTracker
	missingPasses map[ProcessIdentity]uint8
}

func NewReconciler(tracker *IdentityTracker) *Reconciler {
	return &Reconciler{tracker: tracker, missingPasses: make(map[ProcessIdentity]uint8)}
}

func (r *Reconciler) Reconcile(processes []ProcessSnapshot) ReconcileStats {
	var stats ReconcileStats
	observedAt := time.Now().UTC()
	live := make(map[uint32]ProcessIdentity, len(processes))
	byPID := make(map[uint32]ProcessSnapshot, len(processes))
	for _, process := range processes {
		live[process.Identity.PID] = process.Identity
		byPID[process.Identity.PID] = process
	}

	// Expire stale labels before discovering controllers. Otherwise a reused PID
	// can overwrite the old label and leave the previous instance running forever.
	expired := make([]uint32, 0)
	r.tracker.mu.RLock()
	for pid, label := range r.tracker.processLabels {
		identity, ok := live[pid]
		if ok && identity.StartTicks == label.Identity.StartTicks {
			delete(r.missingPasses, label.Identity)
			continue
		}
		// A different start_ticks is definitive PID reuse. A missing PID must
		// survive two complete scans before the fallback declares exit, avoiding
		// a terminal stop on one transient /proc read gap.
		if ok {
			expired = append(expired, pid)
			delete(r.missingPasses, label.Identity)
			continue
		}
		r.missingPasses[label.Identity]++
		if r.missingPasses[label.Identity] >= 2 {
			expired = append(expired, pid)
			delete(r.missingPasses, label.Identity)
		}
	}
	r.tracker.mu.RUnlock()
	for _, pid := range expired {
		if exit, ok := r.tracker.ExitPID(pid, observedAt); ok {
			stats.Exits = append(stats.Exits, exit)
			stats.ExpiredLabelsRemoved++
		}
	}

	// A late fork used to overwrite an already confirmed controller label with
	// its parent's subject. Even after that label exited, the instance could be
	// left running without any label for the normal expiry loop to inspect.
	// Reconcile running controller epochs independently as a final safety net.
	orphanedControllers := make([]ProcessIdentity, 0)
	r.tracker.mu.RLock()
	for _, instance := range r.tracker.instances {
		if instance.Status != "running" || !instance.Controller.Valid() {
			continue
		}
		liveIdentity, liveNow := live[instance.Controller.PID]
		if liveNow && liveIdentity == instance.Controller {
			delete(r.missingPasses, instance.Controller)
			continue
		}
		if label, exists := r.tracker.processLabels[instance.Controller.PID]; exists && label.Identity == instance.Controller {
			continue
		}
		if liveNow {
			orphanedControllers = append(orphanedControllers, instance.Controller)
			delete(r.missingPasses, instance.Controller)
			continue
		}
		r.missingPasses[instance.Controller]++
		if r.missingPasses[instance.Controller] >= 2 {
			orphanedControllers = append(orphanedControllers, instance.Controller)
			delete(r.missingPasses, instance.Controller)
		}
	}
	r.tracker.mu.RUnlock()
	for _, identity := range orphanedControllers {
		if exit, ok := r.tracker.ExitController(identity, observedAt); ok {
			stats.Exits = append(stats.Exits, exit)
			stats.ExpiredLabelsRemoved++
		}
	}

	// Repair descendants from already known parents before controller discovery.
	// A forked controller binary otherwise appears briefly as a second Codex
	// controller before it execs the requested shell/tool process.
	repairDescendants := func() {
		for pass := 0; pass < 4; pass++ {
			repaired := false
			for _, process := range processes {
				if r.tracker.RefreshProcess(process, observedAt) {
					continue
				}
				parent, ok := byPID[process.PPID]
				if !ok {
					continue
				}
				if r.tracker.OnFork(parent.Identity, process) {
					stats.ProcessLabelsRepaired++
					repaired = true
				}
			}
			if !repaired {
				break
			}
		}
	}
	repairDescendants()
	for _, process := range processes {
		if r.tracker.RefreshProcess(process, observedAt) {
			continue
		}
		if _, ok := r.tracker.ObserveController(process); ok {
			stats.ControllersDiscovered++
		}
	}
	// Newly discovered top-level controllers can have descendants in the same
	// procfs snapshot; attach those after discovery as well.
	repairDescendants()

	return stats
}
