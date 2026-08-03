package agentguard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const (
	maxArgLength      = 256
	maxResourceLength = 1024
)

var (
	bearerPattern         = regexp.MustCompile(`(?i)\b(bearer\s+)[^\s]+`)
	keyValueSecretPattern = regexp.MustCompile(`(?i)\b(password|passwd|token|api[_-]?key|secret|authorization)=([^&\s]+)`)
)

type BehaviorNormalizer struct {
	hostID     string
	hostBootID string
	tracker    *IdentityTracker
	sequence   uint64
}

func NewBehaviorNormalizer(hostID, hostBootID string, tracker *IdentityTracker) *BehaviorNormalizer {
	// host_boot_id is stable across Agent service restarts, so a counter that
	// restarts at zero collides with the database's host/boot/sequence unique
	// key and silently suppresses new behavior projections. Seed from wall time
	// and increment atomically for the lifetime of this process.
	return &BehaviorNormalizer{
		hostID: hostID, hostBootID: hostBootID, tracker: tracker,
		sequence: uint64(time.Now().UnixNano()),
	}
}

func (n *BehaviorNormalizer) Normalize(raw RawBehavior) (BehaviorEvent, bool) {
	if n == nil || n.tracker == nil {
		return BehaviorEvent{}, false
	}
	subject, ok := n.tracker.Attribute(raw.Process)
	if !ok || subject.Confidence == ConfidenceAmbiguous || subject.Confidence == ConfidenceUnattributed {
		return BehaviorEvent{}, false
	}
	if raw.OccurredAt.IsZero() {
		raw.OccurredAt = time.Now().UTC()
	}
	if raw.Outcome == "" {
		raw.Outcome = OutcomeUnknown
	}
	if raw.Source == "" {
		raw.Source = "ebpf"
	}
	if raw.Sensor == "" {
		raw.Sensor = "unknown"
	}
	if raw.Visibility == "" {
		raw.Visibility = "complete"
	}
	if raw.EventType == "" {
		raw.EventType = "agent_behavior"
	}
	raw.Decision = safeP2Decision(raw.Decision)
	if raw.Severity == "" {
		raw.Severity = "info"
	}
	eventID := raw.EventID
	if _, err := uuid.Parse(eventID); err != nil {
		eventID = uuid.NewString()
	}
	argv, argvTruncated := RedactArgv(raw.Argv)
	resource, resourceTruncated := redactResource(raw.Resource)
	isolation := redactMap(raw.Isolation)
	evidence := redactMap(raw.Evidence)
	instance, _ := n.tracker.Instance(subject.InstanceID)
	unit, _ := n.tracker.Unit(subject.UnitID)
	truncated := make([]string, 0, 3)
	if argvTruncated {
		truncated = append(truncated, "actor.argv")
	}
	if resourceTruncated {
		truncated = append(truncated, "resource.identity")
	}
	exe, exeTruncated := redactAndLimit(raw.Process.Exe, maxResourceLength)
	if exeTruncated {
		truncated = append(truncated, "actor.exe")
	}
	cwd, cwdTruncated := redactAndLimit(raw.Process.CWD, maxResourceLength)
	if cwdTruncated {
		truncated = append(truncated, "actor.cwd")
	}
	sort.Strings(truncated)

	return BehaviorEvent{
		Schema:              BehaviorSchemaV1,
		EventID:             eventID,
		EventType:           raw.EventType,
		HostID:              n.hostID,
		HostBootID:          n.hostBootID,
		AgentSequence:       atomic.AddUint64(&n.sequence, 1),
		InstanceID:          subject.InstanceID,
		ExecutionUnitID:     subject.UnitID,
		SessionID:           trustedSessionID(raw.SessionID, subject.SessionID),
		CorrelationID:       normalizedCorrelationID(raw.CorrelationID),
		ParentEventID:       normalizedOptionalUUID(raw.ParentEventID),
		AgentType:           instance.AgentType,
		ProfileKey:          instance.ProfileKey,
		ProfileVersion:      instance.ProfileVersion,
		OccurredAt:          raw.OccurredAt.UTC(),
		OccurredMonotonicNS: raw.OccurredMonotonicNS,
		Category:            raw.Category,
		Operation:           raw.Operation,
		Outcome:             raw.Outcome,
		Errno:               raw.Errno,
		Actor: Actor{
			PID: raw.Process.Identity.PID, StartTicks: raw.Process.Identity.StartTicks,
			PPID: raw.Process.PPID, Exe: exe, Argv: argv, CWD: cwd,
			UID: raw.Process.UID, GID: raw.Process.GID,
		},
		Resource:              resource,
		AttributionConfidence: subject.Confidence,
		Decision:              raw.Decision,
		Severity:              raw.Severity,
		RuleID:                raw.RuleID,
		Isolation:             isolation,
		Evidence:              evidence,
		Collection: CollectionEvidence{
			Source: raw.Source, Sensor: raw.Sensor, Visibility: raw.Visibility,
			TruncatedFields: truncated, LostEventsSinceLast: raw.LostEvents,
			AggregatedCount: 1, CoverageLevel: string(behaviorCoverage(unit.Coverage)),
			CoverageReasons: appendCoverageReasons(unit.Coverage, unit.IsolationActual),
			Completeness:    unit.Completeness,
		},
	}, true
}

func trustedSessionID(candidate, fallback string) string {
	if parsed, err := uuid.Parse(candidate); err == nil && parsed != uuid.Nil {
		return parsed.String()
	}
	return fallback
}

func normalizedCorrelationID(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "sha256:") && len(value) == len("sha256:")+sha256.Size*2 {
		if _, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:")); err == nil {
			return value
		}
	}
	return ""
}

func normalizedOptionalUUID(value string) string {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || parsed == uuid.Nil {
		return ""
	}
	return parsed.String()
}

func safeP2Decision(decision Decision) Decision {
	switch decision {
	case DecisionAudit, DecisionAlert, DecisionWouldDeny, DecisionEnforcementUnavailable,
		DecisionDeny, DecisionDenyAndFreeze:
		return decision
	default:
		return DecisionAudit
	}
}

func (e BehaviorEvent) MustJSON() string {
	data, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return string(data)
}

func RedactArgv(argv []string) ([]string, bool) {
	out := make([]string, 0, len(argv))
	truncated := false
	redactNext := false
	for _, argument := range argv {
		if redactNext {
			out = append(out, "[REDACTED]")
			redactNext = false
			continue
		}
		lower := strings.ToLower(argument)
		if isSecretFlag(lower) {
			out = append(out, argument)
			redactNext = true
			continue
		}
		redacted := RedactString(argument)
		if len(redacted) > maxArgLength {
			redacted = redacted[:maxArgLength]
			truncated = true
		}
		out = append(out, redacted)
	}
	return out, truncated
}

func isSecretFlag(value string) bool {
	value = strings.TrimLeft(value, "-")
	for _, key := range []string{
		"password", "passwd", "token", "api-key", "api_key", "apikey",
		"secret", "authorization", "access-token", "access_token",
	} {
		if value == key {
			return true
		}
	}
	return false
}

func RedactString(value string) string {
	// PostgreSQL jsonb rejects the JSON Unicode escape for NUL. Process title
	// rewrites can leave NUL padding in procfs/eBPF strings, so normalize it at
	// the collection boundary before the value can enter any behavior field.
	value = strings.ReplaceAll(strings.ToValidUTF8(value, "\uFFFD"), "\x00", " ")
	value = bearerPattern.ReplaceAllString(value, "${1}[REDACTED]")
	value = keyValueSecretPattern.ReplaceAllString(value, "${1}=[REDACTED]")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return value
	}
	if parsed.User != nil {
		parsed.User = url.User("[REDACTED]")
	}
	query := parsed.Query()
	for key := range query {
		if isSecretFlag(strings.ToLower(key)) {
			query.Set(key, "[REDACTED]")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func redactAndLimit(value string, limit int) (string, bool) {
	value = RedactString(value)
	if len(value) <= limit {
		return value, false
	}
	return value[:limit], true
}

func redactResource(resource Resource) (Resource, bool) {
	out := Resource{
		Type: resource.Type, Classification: resource.Classification,
	}
	out.Identity, _ = redactAndLimit(resource.Identity, maxResourceLength)
	truncated := len(RedactString(resource.Identity)) > maxResourceLength
	if len(resource.Attributes) > 0 {
		out.Attributes = redactMap(resource.Attributes)
	}
	return out, truncated
}

func redactMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		if isSensitiveAttributeKey(key) {
			out[key] = "[REDACTED]"
			continue
		}
		out[key] = redactAttributeValue(value, 0)
	}
	return out
}

func redactAttributeValue(value any, depth int) any {
	if depth >= 8 {
		return "[TRUNCATED]"
	}
	switch typed := value.(type) {
	case string:
		redacted, _ := redactAndLimit(typed, maxResourceLength)
		return redacted
	case []string:
		values, _ := RedactArgv(typed)
		return values
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactAttributeValue(item, depth+1))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSensitiveAttributeKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = redactAttributeValue(item, depth+1)
		}
		return out
	case IsolationState:
		return redactIsolationState(typed)
	case IsolationDiff:
		return redactIsolationDiff(typed, depth+1)
	default:
		return value
	}
}

func redactIsolationState(state IsolationState) IsolationState {
	state.CgroupPath, _ = redactAndLimit(state.CgroupPath, maxResourceLength)
	state.ContainerID, _ = redactAndLimit(state.ContainerID, 128)
	state.ContainerRuntime, _ = redactAndLimit(state.ContainerRuntime, 64)
	state.RootMount, _ = redactAndLimit(state.RootMount, 128)
	return state
}

func redactIsolationDiff(diff IsolationDiff, depth int) IsolationDiff {
	out := IsolationDiff{
		StateChanged: diff.StateChanged,
		Changes:      make(map[string]StateDifference, len(diff.Changes)),
		Unavailable:  append([]string(nil), diff.Unavailable...),
	}
	for key, change := range diff.Changes {
		out.Changes[key] = StateDifference{
			Before: redactAttributeValue(change.Before, depth+1),
			After:  redactAttributeValue(change.After, depth+1),
		}
	}
	return out
}

func isSensitiveAttributeKey(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	for _, marker := range []string{
		"password", "passwd", "token", "secret", "api_key", "apikey",
		"authorization", "cookie", "private_key",
	} {
		if key == marker || strings.HasSuffix(key, "_"+marker) {
			return true
		}
	}
	return false
}

func appendCoverageReasons(level CoverageLevel, state IsolationState) []string {
	out := coverageReasons(level)
	for _, unavailable := range unavailableIsolationDimensions(state) {
		out = append(out, unavailable)
	}
	sort.Strings(out)
	return out
}

type PathInput struct {
	RawPath       string
	CWD           string
	DirFDPath     string
	ContainerRoot string
}

type ResolvedPath struct {
	RawPath      string `json:"raw_path"`
	ResolvedPath string `json:"resolved_path"`
	HostPath     string `json:"host_path,omitempty"`
	Resolution   string `json:"resolution"`
	Confidence   string `json:"confidence"`
}

func ResolvePath(input PathInput) ResolvedPath {
	result := ResolvedPath{RawPath: RedactString(input.RawPath), Resolution: "unresolved", Confidence: "low"}
	var resolved string
	switch {
	case filepath.IsAbs(input.RawPath):
		resolved = filepath.Clean(input.RawPath)
		result.Resolution = "exact"
		result.Confidence = "high"
	case input.DirFDPath != "":
		resolved = filepath.Clean(filepath.Join(input.DirFDPath, input.RawPath))
		result.Resolution = "dirfd"
		result.Confidence = "high"
	case input.CWD != "":
		resolved = filepath.Clean(filepath.Join(input.CWD, input.RawPath))
		result.Resolution = "cwd"
		result.Confidence = "probable"
	}
	result.ResolvedPath = RedactString(resolved)
	if input.ContainerRoot != "" && resolved != "" {
		result.HostPath = RedactString(filepath.Join(input.ContainerRoot, strings.TrimPrefix(resolved, "/")))
	}
	return result
}

type aggregateEntry struct {
	event     BehaviorEvent
	firstSeen time.Time
	lastSeen  time.Time
	count     uint64
}

type Aggregator struct {
	window  time.Duration
	mu      sync.Mutex
	entries map[string]aggregateEntry
}

func NewAggregator(window time.Duration) *Aggregator {
	if window <= 0 {
		window = 2 * time.Second
	}
	return &Aggregator{window: window, entries: make(map[string]aggregateEntry)}
}

func (a *Aggregator) Add(event BehaviorEvent) []BehaviorEvent {
	if !isAggregatable(event) {
		return []BehaviorEvent{event}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	key := aggregateKey(event)
	entry, ok := a.entries[key]
	if !ok {
		a.entries[key] = aggregateEntry{event: event, firstSeen: event.OccurredAt, lastSeen: event.OccurredAt, count: 1}
		return nil
	}
	if event.OccurredAt.Sub(entry.firstSeen) >= a.window {
		delete(a.entries, key)
		entry.event.Collection.AggregatedCount = entry.count
		a.entries[key] = aggregateEntry{event: event, firstSeen: event.OccurredAt, lastSeen: event.OccurredAt, count: 1}
		return []BehaviorEvent{entry.event}
	}
	entry.lastSeen = event.OccurredAt
	entry.count++
	entry.event.OccurredAt = event.OccurredAt
	entry.event.Collection.LostEventsSinceLast += event.Collection.LostEventsSinceLast
	a.entries[key] = entry
	return nil
}

func (a *Aggregator) Flush(now time.Time) []BehaviorEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]BehaviorEvent, 0)
	for key, entry := range a.entries {
		if now.Sub(entry.lastSeen) < a.window {
			continue
		}
		entry.event.Collection.AggregatedCount = entry.count
		out = append(out, entry.event)
		delete(a.entries, key)
	}
	return out
}

func isAggregatable(event BehaviorEvent) bool {
	if event.Decision != DecisionAudit {
		return false
	}
	if event.Category == CategoryFile {
		return event.Operation == "read_observed" || event.Operation == "write_observed" || event.Operation == "open_intent"
	}
	if event.Category == CategoryNetwork {
		return event.Operation == "connect_failed"
	}
	return false
}

func aggregateKey(event BehaviorEvent) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d\x00%s\x00%s\x00%s",
		event.InstanceID, event.SessionID, event.ExecutionUnitID,
		event.Actor.PID, event.Actor.StartTicks, event.Category,
		event.Operation, event.Resource.Identity)
}

type EventPriority int

const (
	PriorityRepetitiveIO EventPriority = iota + 1
	PriorityProcessNetwork
	PriorityStateChange
	PriorityCriticalEvidence
)

type spoolItem struct {
	event    BehaviorEvent
	priority EventPriority
	order    uint64
}

type SpoolStats struct {
	Queued            int               `json:"queued"`
	DroppedByReason   map[string]uint64 `json:"dropped_by_reason"`
	DroppedByCategory map[string]uint64 `json:"dropped_by_category"`
}

type PrioritySpool struct {
	capacity        int
	mu              sync.Mutex
	next            uint64
	items           []spoolItem
	droppedReason   map[string]uint64
	droppedCategory map[string]uint64
}

func NewPrioritySpool(capacity int) *PrioritySpool {
	if capacity <= 0 {
		capacity = 1024
	}
	return &PrioritySpool{
		capacity:        capacity,
		droppedReason:   make(map[string]uint64),
		droppedCategory: make(map[string]uint64),
	}
}

func (s *PrioritySpool) Push(event BehaviorEvent, priority EventPriority) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	item := spoolItem{event: event, priority: priority, order: s.next}
	if len(s.items) < s.capacity {
		s.items = append(s.items, item)
		return true
	}
	lowest := 0
	for i := 1; i < len(s.items); i++ {
		if s.items[i].priority < s.items[lowest].priority ||
			(s.items[i].priority == s.items[lowest].priority && s.items[i].order < s.items[lowest].order) {
			lowest = i
		}
	}
	if s.items[lowest].priority >= priority {
		s.recordDropLocked(event, "queue_full")
		return false
	}
	evicted := s.items[lowest].event
	s.recordDropLocked(evicted, "evicted_lower_priority")
	s.items[lowest] = item
	return true
}

func (s *PrioritySpool) PopBatch(limit int) []BehaviorEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.items) {
		limit = len(s.items)
	}
	sort.SliceStable(s.items, func(i, j int) bool {
		if s.items[i].priority == s.items[j].priority {
			return s.items[i].order < s.items[j].order
		}
		return s.items[i].priority > s.items[j].priority
	})
	out := make([]BehaviorEvent, limit)
	for i := 0; i < limit; i++ {
		out[i] = s.items[i].event
	}
	s.items = append([]spoolItem(nil), s.items[limit:]...)
	return out
}

func (s *PrioritySpool) Stats() SpoolStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := SpoolStats{
		Queued:            len(s.items),
		DroppedByReason:   make(map[string]uint64, len(s.droppedReason)),
		DroppedByCategory: make(map[string]uint64, len(s.droppedCategory)),
	}
	for key, value := range s.droppedReason {
		stats.DroppedByReason[key] = value
	}
	for key, value := range s.droppedCategory {
		stats.DroppedByCategory[key] = value
	}
	return stats
}

func (s *PrioritySpool) recordDropLocked(event BehaviorEvent, reason string) {
	s.droppedReason[reason]++
	s.droppedCategory[string(event.Category)]++
}
