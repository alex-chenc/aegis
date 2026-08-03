package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"dc/internal/model"

	"github.com/google/uuid"
)

const (
	agentBehaviorSchema = "aegis.agent_behavior.v1"
	redactedValue       = "[REDACTED]"
	maxEvidenceString   = 2048
	maxArgvItems        = 128
	maxAgentGuardEvent  = 1 << 20
)

var (
	ErrAgentBehaviorInvalidSchema   = errors.New("invalid agent behavior schema")
	ErrAgentBehaviorInvalidContract = errors.New("invalid agent behavior contract")
	ErrAgentBehaviorActiveDecision  = errors.New("active Agent Guard decision is disabled in monitor-only")
)

type behaviorEnvelope struct {
	Schema                string             `json:"schema"`
	EventID               string             `json:"event_id"`
	HostID                string             `json:"host_id"`
	HostBootID            string             `json:"host_boot_id"`
	AgentSequence         *int64             `json:"agent_sequence"`
	InstanceID            string             `json:"instance_id"`
	SessionID             string             `json:"session_id"`
	ExecutionUnitID       string             `json:"execution_unit_id"`
	PolicyID              string             `json:"policy_id"`
	PolicyVersion         *int64             `json:"policy_version"`
	RuleID                string             `json:"rule_id"`
	CorrelationID         string             `json:"correlation_id"`
	ParentEventID         string             `json:"parent_event_id"`
	AgentType             string             `json:"agent_type"`
	ProfileKey            string             `json:"profile_key"`
	ProfileVersion        *int64             `json:"profile_version"`
	OccurredAt            string             `json:"occurred_at"`
	OccurredMonotonicNS   *int64             `json:"occurred_monotonic_ns"`
	Category              string             `json:"category"`
	Operation             string             `json:"operation"`
	Outcome               string             `json:"outcome"`
	Errno                 *int               `json:"errno"`
	Decision              string             `json:"decision"`
	Severity              string             `json:"severity"`
	Actor                 behaviorActor      `json:"actor"`
	ProcessChain          []map[string]any   `json:"process_chain"`
	Resource              behaviorResource   `json:"resource"`
	Isolation             map[string]any     `json:"isolation"`
	Collection            behaviorCollection `json:"collection"`
	Evidence              map[string]any     `json:"evidence"`
	AttributionConfidence string             `json:"attribution_confidence"`
}

type behaviorActor struct {
	PID        int             `json:"pid"`
	PPID       int             `json:"ppid"`
	StartTicks json.RawMessage `json:"start_ticks"`
	Name       string          `json:"name"`
	Exe        string          `json:"exe"`
	Argv       []string        `json:"argv"`
	CWD        string          `json:"cwd"`
	Visibility string          `json:"visibility"`
}

type behaviorResource struct {
	Type           string         `json:"type"`
	Identity       string         `json:"identity"`
	Classification string         `json:"classification"`
	Attributes     map[string]any `json:"attributes"`
}

type behaviorCollection struct {
	Source                string   `json:"source"`
	Sensor                string   `json:"sensor"`
	Visibility            string   `json:"visibility"`
	TruncatedFields       []string `json:"truncated_fields"`
	LostEventsSinceLast   *int64   `json:"lost_events_since_last"`
	AggregatedCount       int64    `json:"aggregated_count,omitempty"`
	AggregateWindowMS     int64    `json:"aggregate_window_ms,omitempty"`
	CoverageLevel         string   `json:"coverage_level,omitempty"`
	CoverageReasons       []string `json:"coverage_reasons,omitempty"`
	AttributionConfidence string   `json:"attribution_confidence,omitempty"`
}

func NormalizeAgentBehavior(topLevelEventID string, topLevelHostID uuid.UUID, raw string) (*model.AgentBehaviorEvent, error) {
	if len(raw) > maxAgentGuardEvent {
		return nil, fmt.Errorf("%w: event too large", ErrAgentBehaviorInvalidContract)
	}
	var envelope behaviorEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON", ErrAgentBehaviorInvalidContract)
	}
	if envelope.Schema != agentBehaviorSchema {
		return nil, fmt.Errorf("%w: %q", ErrAgentBehaviorInvalidSchema, envelope.Schema)
	}
	if envelope.EventID == "" || envelope.EventID != topLevelEventID {
		return nil, fmt.Errorf("%w: event_id mismatch", ErrAgentBehaviorInvalidContract)
	}
	if _, err := uuid.Parse(envelope.EventID); err != nil {
		return nil, fmt.Errorf("%w: event_id", ErrAgentBehaviorInvalidContract)
	}
	eventHostID, err := uuid.Parse(envelope.HostID)
	if err != nil || eventHostID != topLevelHostID {
		return nil, fmt.Errorf("%w: host_id mismatch", ErrAgentBehaviorInvalidContract)
	}
	if envelope.HostBootID == "" || len(envelope.HostBootID) > 100 ||
		envelope.AgentSequence == nil || *envelope.AgentSequence < 0 {
		return nil, fmt.Errorf("%w: host boot sequence", ErrAgentBehaviorInvalidContract)
	}
	if envelope.OccurredMonotonicNS == nil || *envelope.OccurredMonotonicNS < 0 {
		return nil, fmt.Errorf("%w: occurred_monotonic_ns", ErrAgentBehaviorInvalidContract)
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, envelope.OccurredAt)
	if err != nil {
		return nil, fmt.Errorf("%w: occurred_at", ErrAgentBehaviorInvalidContract)
	}
	if !allowedValue(envelope.Category, "process", "file", "network", "identity", "persistence", "isolation", "kernel", "ipc", "tool", "control") ||
		envelope.Operation == "" || len(envelope.Operation) > 64 {
		return nil, fmt.Errorf("%w: category or operation", ErrAgentBehaviorInvalidContract)
	}
	if envelope.Outcome == "failed" {
		// Early P1 agents emitted "failed" while the 029 schema and V6.2
		// query contract use "failure".
		envelope.Outcome = "failure"
	}
	if !allowedValue(envelope.Outcome, "success", "failure", "denied", "unknown") {
		return nil, fmt.Errorf("%w: outcome", ErrAgentBehaviorInvalidContract)
	}
	decision := strings.TrimSpace(envelope.Decision)
	if decision == "" {
		decision = "audit"
	}
	if decision == "deny" || decision == "deny_and_freeze" || decision == "alert" {
		return nil, ErrAgentBehaviorActiveDecision
	}
	if !allowedValue(decision, "allow", "audit", "would_deny", "enforcement_unavailable") {
		return nil, fmt.Errorf("%w: decision", ErrAgentBehaviorInvalidContract)
	}
	severity := strings.TrimSpace(envelope.Severity)
	if severity == "" {
		severity = "info"
	}
	if !allowedValue(severity, "info", "low", "medium", "high", "critical") {
		return nil, fmt.Errorf("%w: severity", ErrAgentBehaviorInvalidContract)
	}

	instanceID, err := optionalUUID(envelope.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("%w: instance_id", ErrAgentBehaviorInvalidContract)
	}
	sessionID, err := optionalUUID(envelope.SessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: session_id", ErrAgentBehaviorInvalidContract)
	}
	unitID, err := optionalUUID(envelope.ExecutionUnitID)
	if err != nil {
		return nil, fmt.Errorf("%w: execution_unit_id", ErrAgentBehaviorInvalidContract)
	}
	policyID, err := optionalUUID(envelope.PolicyID)
	if err != nil {
		return nil, fmt.Errorf("%w: policy_id", ErrAgentBehaviorInvalidContract)
	}
	if instanceID == nil || sessionID == nil || unitID == nil ||
		!allowedValue(envelope.AttributionConfidence, "candidate", "probable", "confirmed") {
		return nil, fmt.Errorf("%w: attribution identity", ErrAgentBehaviorInvalidContract)
	}
	processStartTicks := normalizeNumber(envelope.Actor.StartTicks)
	if envelope.Actor.PID <= 0 || envelope.Actor.PPID < 0 ||
		processStartTicks == "" || processStartTicks == "0" {
		return nil, fmt.Errorf("%w: actor process identity", ErrAgentBehaviorInvalidContract)
	}

	visibility := envelope.Actor.Visibility
	if visibility == "" {
		visibility = envelope.Collection.Visibility
	}
	if visibility == "" {
		visibility = "complete"
	}
	if !allowedValue(visibility, "complete", "partial", "unobservable") {
		return nil, fmt.Errorf("%w: visibility", ErrAgentBehaviorInvalidContract)
	}
	if envelope.Collection.Source == "" || envelope.Collection.Sensor == "" ||
		envelope.Collection.LostEventsSinceLast == nil {
		return nil, fmt.Errorf("%w: collection", ErrAgentBehaviorInvalidContract)
	}
	if *envelope.Collection.LostEventsSinceLast > 0 || len(envelope.Collection.TruncatedFields) > 0 ||
		envelope.Collection.AggregatedCount > 1 {
		visibility = "partial"
	}
	if *envelope.Collection.LostEventsSinceLast < 0 || envelope.Collection.AggregatedCount < 0 {
		return nil, fmt.Errorf("%w: collection counters", ErrAgentBehaviorInvalidContract)
	}
	if envelope.Collection.AggregatedCount == 0 {
		envelope.Collection.AggregatedCount = 1
	}
	if envelope.Collection.TruncatedFields == nil {
		envelope.Collection.TruncatedFields = []string{}
	}
	if envelope.Collection.CoverageReasons == nil {
		envelope.Collection.CoverageReasons = []string{}
	}
	envelope.Collection.Visibility = visibility
	envelope.Collection.AttributionConfidence = envelope.AttributionConfidence
	switch envelope.Collection.CoverageLevel {
	case "":
		envelope.Collection.CoverageLevel = "monitor_only"
		envelope.Collection.CoverageReasons = appendUnique(envelope.Collection.CoverageReasons, "p1_monitor_only")
	case "full_enforcement", "behavior_monitor_escape_enforce", "monitor_only", "no_isolation", "remote_unobservable", "degraded":
	default:
		return nil, fmt.Errorf("%w: collection coverage", ErrAgentBehaviorInvalidContract)
	}

	commandArgv := redactArgv(envelope.Actor.Argv)
	if envelope.ProcessChain == nil {
		envelope.ProcessChain = []map[string]any{}
	}
	if envelope.Resource.Attributes == nil {
		envelope.Resource.Attributes = map[string]any{}
	}
	if envelope.Isolation == nil {
		envelope.Isolation = map[string]any{}
	}
	if envelope.Evidence == nil {
		envelope.Evidence = map[string]any{}
	}
	if err := normalizeAgentToolSemantics(&envelope, occurredAt); err != nil {
		return nil, err
	}
	resourceIdentity := redactText(envelope.Resource.Identity)
	resourceHash := ""
	if resourceIdentity != "" {
		sum := sha256.Sum256([]byte(resourceIdentity))
		resourceHash = hex.EncodeToString(sum[:])
	}
	resource := map[string]any{
		"type":           truncate(envelope.Resource.Type),
		"identity":       resourceIdentity,
		"classification": truncate(envelope.Resource.Classification),
		"attributes":     sanitizeValue(envelope.Resource.Attributes, ""),
	}

	return &model.AgentBehaviorEvent{
		RawEventID:             envelope.EventID,
		HostID:                 topLevelHostID,
		HostBootID:             envelope.HostBootID,
		AgentSequence:          *envelope.AgentSequence,
		InstanceID:             instanceID,
		SessionID:              sessionID,
		ExecutionUnitID:        unitID,
		PolicyID:               policyID,
		PolicyVersion:          envelope.PolicyVersion,
		RuleID:                 truncateLimit(envelope.RuleID, 100),
		SchemaVersion:          envelope.Schema,
		CorrelationID:          truncateLimit(envelope.CorrelationID, 100),
		ParentEventID:          truncateLimit(envelope.ParentEventID, 100),
		AgentType:              truncateLimit(envelope.AgentType, 64),
		ProfileKey:             truncateLimit(envelope.ProfileKey, 128),
		ProfileVersion:         envelope.ProfileVersion,
		Category:               envelope.Category,
		Operation:              envelope.Operation,
		Outcome:                envelope.Outcome,
		Errno:                  envelope.Errno,
		Decision:               decision,
		Severity:               severity,
		PID:                    envelope.Actor.PID,
		PPID:                   envelope.Actor.PPID,
		ProcessStartTicks:      processStartTicks,
		ProcessName:            truncateLimit(envelope.Actor.Name, 255),
		ProcessExe:             redactText(envelope.Actor.Exe),
		CommandArgv:            mustJSON(commandArgv, []string{}),
		CommandCWD:             redactText(envelope.Actor.CWD),
		CommandVisibility:      visibility,
		ProcessChain:           mustJSON(sanitizeValue(envelope.ProcessChain, ""), []any{}),
		ResourceType:           truncateLimit(envelope.Resource.Type, 32),
		ResourceIdentity:       resourceIdentity,
		ResourceIdentityHash:   resourceHash,
		ResourceClassification: truncateLimit(envelope.Resource.Classification, 64),
		Resource:               mustJSON(resource, map[string]any{}),
		Isolation:              mustJSON(sanitizeValue(envelope.Isolation, ""), map[string]any{}),
		Collection:             mustJSON(envelope.Collection, map[string]any{}),
		Evidence:               mustJSON(sanitizeValue(envelope.Evidence, ""), map[string]any{}),
		OccurredAt:             occurredAt,
		OccurredMonotonicNS:    envelope.OccurredMonotonicNS,
		ReceivedAt:             time.Now().UTC(),
		AggregatedCount:        envelope.Collection.AggregatedCount,
		LostEventsSinceLast:    *envelope.Collection.LostEventsSinceLast,
		HasTruncatedFields:     len(envelope.Collection.TruncatedFields) > 0,
		Completeness:           mustJSON(envelope.Collection, map[string]any{}),
	}, nil
}

// SanitizeAgentGuardEventData produces valid JSON for raw storage without
// retaining credential-like fields. Invalid JSON follows the existing DC
// normalization contract and becomes an empty object.
func SanitizeAgentGuardEventData(raw string) string {
	if len(raw) > maxAgentGuardEvent {
		return "{}"
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "{}"
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return "{}"
	}
	data, err := json.Marshal(sanitizeValue(value, ""))
	if err != nil {
		return "{}"
	}
	return string(data)
}

func RedactSummary(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return strings.Join(redactArgv(strings.Fields(value)), " ")
}

func optionalUUID(value string) (*uuid.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func allowedValue(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func redactArgv(values []string) []string {
	if len(values) > maxArgvItems {
		values = values[:maxArgvItems]
	}
	result := make([]string, len(values))
	redactNext := false
	for index, value := range values {
		if redactNext {
			result[index] = redactedValue
			redactNext = false
			continue
		}
		lower := strings.ToLower(value)
		if isSensitiveKey(strings.TrimLeft(lower, "-")) {
			result[index] = truncate(value)
			redactNext = true
			continue
		}
		result[index] = redactText(value)
	}
	return result
}

func redactText(value string) string {
	value = truncate(value)
	lower := strings.ToLower(value)
	if index := strings.Index(lower, "bearer "); index >= 0 {
		start := index + len("bearer ")
		end := strings.IndexAny(value[start:], " \t\r\n,;")
		if end < 0 {
			end = len(value) - start
		}
		value = value[:start] + redactedValue + value[start+end:]
		lower = strings.ToLower(value)
	}
	for _, marker := range []string{"token=", "password=", "passwd=", "secret=", "api_key=", "apikey=", "authorization=", "cookie="} {
		if index := strings.Index(lower, marker); index >= 0 {
			end := strings.IndexAny(value[index+len(marker):], "& ,;")
			if end < 0 {
				end = len(value) - index - len(marker)
			}
			start := index + len(marker)
			value = value[:start] + redactedValue + value[start+end:]
			lower = strings.ToLower(value)
		}
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		if parsed.User != nil {
			parsed.User = url.User(redactedValue)
		}
		query := parsed.Query()
		for key := range query {
			if isSensitiveKey(key) {
				query.Set(key, redactedValue)
			}
		}
		parsed.RawQuery = query.Encode()
		value = parsed.String()
	}
	return truncate(value)
}

func sanitizeValue(value any, key string) any {
	if isSensitiveKey(key) {
		return redactedValue
	}
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for childKey, child := range typed {
			result[childKey] = sanitizeValue(child, childKey)
		}
		return result
	case []any:
		if len(typed) > maxArgvItems {
			typed = typed[:maxArgvItems]
		}
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = sanitizeValue(child, key)
		}
		return result
	case []map[string]any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = sanitizeValue(child, key)
		}
		return result
	case string:
		normalizedKey := strings.ToLower(key)
		if strings.Contains(normalizedKey, "argv") || strings.Contains(normalizedKey, "cmdline") ||
			strings.Contains(normalizedKey, "command") {
			return RedactSummary(typed)
		}
		return redactText(typed)
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimLeft(key, "-"), "-", "_"))
	for _, marker := range []string{
		"token", "password", "passwd", "secret", "api_key", "apikey", "authorization",
		"cookie", "credential", "private_key", "stdin", "stdout", "stderr", "environment",
		"env", "file_content", "network_payload", "payload", "raw_output",
	} {
		if normalized == marker || strings.HasSuffix(normalized, "_"+marker) {
			return true
		}
	}
	return false
}

func truncate(value string) string {
	return truncateLimit(value, maxEvidenceString)
}

func truncateLimit(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func normalizeNumber(value json.RawMessage) string {
	raw := strings.TrimSpace(string(value))
	if raw == "" || raw == "null" {
		return ""
	}
	if strings.HasPrefix(raw, `"`) {
		var text string
		if json.Unmarshal(value, &text) == nil {
			return truncate(text)
		}
		return ""
	}
	if _, err := strconv.ParseUint(raw, 10, 64); err != nil {
		return ""
	}
	return raw
}

func mustJSON(value any, fallback any) json.RawMessage {
	data, err := json.Marshal(value)
	if err == nil {
		return data
	}
	data, _ = json.Marshal(fallback)
	return data
}

func appendUnique(values []string, value string) []string {
	for _, candidate := range values {
		if candidate == value {
			return values
		}
	}
	return append(values, value)
}
