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
	Subject  GuardSubject
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
	sessionID := stableID("session", instanceID, "activity")
	unitID := stableID("unit", instanceID, string(IsolationLocalProcessTree))
	session := BehaviorSession{
		SessionID: sessionID, InstanceID: instanceID, Source: "activity_window",
		Confidence: ConfidenceInferred, Status: "active", FirstSeenAt: now, LastSeenAt: now,
	}
	unit := ExecutionUnit{
		UnitID: unitID, InstanceID: instanceID, SessionID: sessionID,
		Type: IsolationLocalProcessTree, RootProcess: process.Identity,
		Coverage: coverage, Status: "observed", FirstSeenAt: now, LastSeenAt: now,
	}
	subject := GuardSubject{
		InstanceID: instanceID, SessionID: sessionID, UnitID: unitID,
		Confidence: match.Confidence,
	}
	t.instances[instanceID] = instance
	t.sessions[sessionID] = session
	t.units[unitID] = unit
	t.processLabels[process.Identity.PID] = processLabel{Identity: process.Identity, Subject: subject}
	return instance, true
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
	t.processLabels[child.Identity.PID] = processLabel{Identity: child.Identity, Subject: label.Subject}
	t.touchLocked(label.Subject, time.Now().UTC())
	return true
}

func (t *IdentityTracker) OnExec(process ProcessSnapshot) GuardSubject {
	t.mu.Lock()
	defer t.mu.Unlock()
	var launchedBy string
	if current, ok := t.processLabels[process.Identity.PID]; ok && current.Identity.StartTicks == process.Identity.StartTicks {
		launchedBy = current.Subject.InstanceID
	}
	match := t.profiles.MatchController(process)
	if match.Profile != nil && match.Confidence == ConfidenceConfirmed {
		instance, _ := t.observeControllerLocked(process, match, launchedBy)
		return t.processLabels[instance.Controller.PID].Subject
	}
	if current, ok := t.processLabels[process.Identity.PID]; ok && current.Identity.StartTicks == process.Identity.StartTicks {
		t.touchLocked(current.Subject, time.Now().UTC())
		return current.Subject
	}
	return GuardSubject{Confidence: ConfidenceUnattributed}
}

func (t *IdentityTracker) OnExit(identity ProcessIdentity) {
	t.mu.Lock()
	defer t.mu.Unlock()
	label, ok := t.processLabels[identity.PID]
	if !ok || label.Identity.StartTicks != identity.StartTicks {
		return
	}
	delete(t.processLabels, identity.PID)
	if instance, ok := t.instances[label.Subject.InstanceID]; ok && instance.Controller == identity {
		instance.Status = "stopped"
		instance.LastSeenAt = time.Now().UTC()
		t.instances[instance.InstanceID] = instance
	}
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
	sessionID := stableID("session", instanceID, "container", info.ContainerID)
	unitID := stableID("unit", instanceID, info.ContainerID)
	session := BehaviorSession{
		SessionID: sessionID, InstanceID: instanceID, Source: "execution_unit",
		Confidence: ConfidenceConfirmed, Status: "active", FirstSeenAt: now, LastSeenAt: now,
	}
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
	t.sessions[sessionID] = session
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
	sessionID := stableID("session", instanceID, "namespace", fingerprint)
	unitID := stableID("unit", instanceID, "namespace", fingerprint)
	now := time.Now().UTC()
	unit, exists := t.units[unitID]
	if !exists {
		session := BehaviorSession{
			SessionID: sessionID, InstanceID: instanceID, Source: "execution_unit",
			Confidence: ConfidenceConfirmed, Status: "active",
			FirstSeenAt: now, LastSeenAt: now,
		}
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
		t.sessions[sessionID] = session
		t.units[unitID] = unit
	}
	subject := GuardSubject{
		InstanceID: instance.InstanceID, SessionID: sessionID,
		UnitID: unitID, Confidence: ConfidenceConfirmed,
	}
	t.processLabels[process.Identity.PID] = processLabel{
		Identity: process.Identity, Subject: subject,
	}
	return unit, nil
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
	source, correlationHash string,
) (BehaviorSession, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now().UTC()
	session, exists := t.sessions[subject.SessionID]
	if !exists {
		return BehaviorSession{}, false
	}
	changed := session.Source != source || session.Confidence != ConfidenceConfirmed ||
		session.CorrelationTokenHash != correlationHash
	session.Source = source
	session.Confidence = ConfidenceConfirmed
	session.CorrelationTokenHash = correlationHash
	session.LastSeenAt = now
	t.sessions[subject.SessionID] = session
	return session, changed
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
}

type Reconciler struct {
	tracker *IdentityTracker
}

func NewReconciler(tracker *IdentityTracker) *Reconciler {
	return &Reconciler{tracker: tracker}
}

func (r *Reconciler) Reconcile(processes []ProcessSnapshot) ReconcileStats {
	var stats ReconcileStats
	live := make(map[uint32]ProcessIdentity, len(processes))
	byPID := make(map[uint32]ProcessSnapshot, len(processes))
	for _, process := range processes {
		live[process.Identity.PID] = process.Identity
		byPID[process.Identity.PID] = process
		if _, exists := r.tracker.LookupProcess(process.Identity); exists {
			continue
		}
		if _, ok := r.tracker.ObserveController(process); ok {
			stats.ControllersDiscovered++
		}
	}
	for pass := 0; pass < 4; pass++ {
		repaired := false
		for _, process := range processes {
			if _, exists := r.tracker.LookupProcess(process.Identity); exists {
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

	r.tracker.mu.Lock()
	for pid, label := range r.tracker.processLabels {
		identity, ok := live[pid]
		if !ok || identity.StartTicks != label.Identity.StartTicks {
			delete(r.tracker.processLabels, pid)
			stats.ExpiredLabelsRemoved++
		}
	}
	r.tracker.mu.Unlock()
	return stats
}
