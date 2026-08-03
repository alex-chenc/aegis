package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dc/internal/model"

	"github.com/google/uuid"
)

const agentGuardSchema = "aegis.agent_guard.v1"

type instanceStateEnvelope struct {
	Schema               string          `json:"schema"`
	InstanceID           string          `json:"instance_id"`
	AssetID              string          `json:"asset_id"`
	ProfileKey           string          `json:"profile_key"`
	ProfileVersion       *int64          `json:"profile_version"`
	AgentType            string          `json:"agent_type"`
	DisplayName          string          `json:"display_name"`
	ControllerPID        *int            `json:"controller_pid"`
	ControllerStartTicks json.RawMessage `json:"controller_start_ticks"`
	ControllerExe        string          `json:"controller_exe"`
	RunUID               *int            `json:"run_uid"`
	DetectionConfidence  string          `json:"detection_confidence"`
	Status               string          `json:"status"`
	CoverageLevel        string          `json:"coverage_level"`
	CoverageReasons      []string        `json:"coverage_reasons"`
	FirstSeenAt          string          `json:"first_seen_at"`
	LastSeenAt           string          `json:"last_seen_at"`
}

type unitStateEnvelope struct {
	Schema            string          `json:"schema"`
	ExecutionUnitID   string          `json:"execution_unit_id"`
	InstanceID        string          `json:"instance_id"`
	UnitType          string          `json:"unit_type"`
	Fingerprint       string          `json:"fingerprint"`
	RootPID           *int            `json:"root_pid"`
	RootStartTicks    json.RawMessage `json:"root_start_ticks"`
	CgroupPath        string          `json:"cgroup_path"`
	ContainerID       string          `json:"container_id"`
	ContainerRuntime  string          `json:"container_runtime"`
	RemoteBackend     string          `json:"remote_backend"`
	RemoteExecutionID string          `json:"remote_execution_id"`
	RemoteHostRef     string          `json:"remote_host_ref"`
	CoverageLevel     string          `json:"coverage_level"`
	CoverageReasons   []string        `json:"coverage_reasons"`
	IsolationBaseline map[string]any  `json:"isolation_baseline"`
	IsolationActual   map[string]any  `json:"isolation_actual"`
	IsolationDiff     map[string]any  `json:"isolation_diff"`
	Capabilities      map[string]any  `json:"capabilities"`
	Completeness      any             `json:"completeness"`
	Status            string          `json:"status"`
	FirstSeenAt       string          `json:"first_seen_at"`
	LastSeenAt        string          `json:"last_seen_at"`
}

type sessionStateEnvelope struct {
	Schema               string         `json:"schema"`
	SessionID            string         `json:"session_id"`
	InstanceID           string         `json:"instance_id"`
	ExecutionUnitID      string         `json:"execution_unit_id"`
	ExternalSessionID    string         `json:"external_session_id"`
	Source               string         `json:"source"`
	Confidence           string         `json:"confidence"`
	CorrelationTokenHash string         `json:"correlation_token_hash"`
	Status               string         `json:"status"`
	StartedAt            string         `json:"started_at"`
	LastSeenAt           string         `json:"last_seen_at"`
	Completeness         map[string]any `json:"completeness"`
}

type deliveryStateEnvelope struct {
	Schema        string `json:"schema"`
	Status        string `json:"status"`
	BundleVersion *int64 `json:"bundle_version"`
	Digest        string `json:"digest"`
	ErrorCode     string `json:"error_code"`
	OccurredAt    string `json:"occurred_at"`
}

func NormalizeAgentGuardState(eventType string, hostID uuid.UUID, raw string) (*model.AgentGuardStateProjection, error) {
	if len(raw) > maxAgentGuardEvent {
		return nil, fmt.Errorf("%w: event too large", ErrAgentBehaviorInvalidContract)
	}
	switch eventType {
	case "agent_guard_config_status":
		return normalizeDeliveryState(eventType, hostID, raw)
	case "agent_guard_action_status":
		return normalizeAgentGuardActionStatus(eventType, hostID, raw)
	case "agent_instance_started", "agent_instance_updated", "agent_instance_stopped":
		return normalizeInstanceState(eventType, hostID, raw)
	case "agent_execution_unit_started", "agent_execution_unit_updated", "agent_execution_unit_stopped":
		return normalizeUnitState(eventType, hostID, raw)
	case "agent_behavior_session_started", "agent_behavior_session_updated", "agent_behavior_session_stopped":
		return normalizeSessionState(eventType, hostID, raw)
	default:
		return nil, fmt.Errorf("%w: unsupported state event", ErrAgentBehaviorInvalidContract)
	}
}

func normalizeDeliveryState(eventType string, hostID uuid.UUID, raw string) (*model.AgentGuardStateProjection, error) {
	var envelope deliveryStateEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || envelope.Schema != agentGuardSchema {
		return nil, fmt.Errorf("%w: delivery schema", ErrAgentBehaviorInvalidContract)
	}
	if envelope.BundleVersion == nil || *envelope.BundleVersion < 1 || len(envelope.Digest) > 80 ||
		len(envelope.ErrorCode) > 100 {
		return nil, fmt.Errorf("%w: delivery identity", ErrAgentBehaviorInvalidContract)
	}
	status := envelope.Status
	switch status {
	case "received", "validating":
		status = "received"
	case "applied":
		if envelope.Digest == "" {
			return nil, fmt.Errorf("%w: applied delivery digest", ErrAgentBehaviorInvalidContract)
		}
	case "rejected", "failed":
		status = "failed"
	case "degraded", "stale", "unsupported_agent_version":
	default:
		return nil, fmt.Errorf("%w: delivery status", ErrAgentBehaviorInvalidContract)
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, envelope.OccurredAt)
	if err != nil {
		return nil, fmt.Errorf("%w: delivery occurred_at", ErrAgentBehaviorInvalidContract)
	}
	return &model.AgentGuardStateProjection{
		EventType: eventType,
		Delivery: &model.AgentGuardDeliveryStatus{
			HostID:         hostID,
			BundleVersion:  *envelope.BundleVersion,
			BundleDigest:   envelope.Digest,
			Status:         status,
			ErrorCode:      envelope.ErrorCode,
			LastReportedAt: occurredAt,
		},
	}, nil
}

func normalizeInstanceState(eventType string, hostID uuid.UUID, raw string) (*model.AgentGuardStateProjection, error) {
	var envelope instanceStateEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || envelope.Schema != agentGuardSchema {
		return nil, fmt.Errorf("%w: instance schema", ErrAgentBehaviorInvalidContract)
	}
	id, err := uuid.Parse(envelope.InstanceID)
	if err != nil || envelope.ProfileVersion == nil || *envelope.ProfileVersion < 1 ||
		envelope.ControllerPID == nil || *envelope.ControllerPID <= 0 {
		return nil, fmt.Errorf("%w: instance identity", ErrAgentBehaviorInvalidContract)
	}
	startTicks := normalizeNumber(envelope.ControllerStartTicks)
	if startTicks == "" || startTicks == "0" ||
		strings.TrimSpace(envelope.ProfileKey) == "" || len(envelope.ProfileKey) > 128 ||
		strings.TrimSpace(envelope.AgentType) == "" || len(envelope.AgentType) > 64 {
		return nil, fmt.Errorf("%w: instance controller or profile", ErrAgentBehaviorInvalidContract)
	}
	if !allowedValue(envelope.DetectionConfidence, "candidate", "probable", "confirmed") ||
		!allowedValue(envelope.Status, "running", "stale", "stopped", "unknown") {
		return nil, fmt.Errorf("%w: instance status", ErrAgentBehaviorInvalidContract)
	}
	coverage, reasons, err := normalizeP1Coverage(envelope.CoverageLevel, envelope.CoverageReasons)
	if err != nil {
		return nil, err
	}
	firstSeen, lastSeen, err := parseStateTimes(envelope.FirstSeenAt, envelope.LastSeenAt)
	if err != nil {
		return nil, err
	}
	var stoppedAt *time.Time
	if eventType == "agent_instance_stopped" {
		if envelope.Status != "stopped" {
			return nil, fmt.Errorf("%w: instance stopped status", ErrAgentBehaviorInvalidContract)
		}
		stoppedAt = &lastSeen
	}
	var assetID *uuid.UUID
	if strings.TrimSpace(envelope.AssetID) != "" {
		parsed, parseErr := uuid.Parse(envelope.AssetID)
		if parseErr != nil {
			return nil, fmt.Errorf("%w: instance asset identity", ErrAgentBehaviorInvalidContract)
		}
		assetID = &parsed
	}
	instance := &model.AgentRuntimeInstance{
		ID:                   id,
		HostID:               hostID,
		AssetID:              assetID,
		ProfileKey:           truncate(envelope.ProfileKey),
		ProfileVersion:       *envelope.ProfileVersion,
		AgentType:            truncate(envelope.AgentType),
		DisplayName:          truncateLimit(envelope.DisplayName, 255),
		ControllerPID:        *envelope.ControllerPID,
		ControllerStartTicks: startTicks,
		ControllerExe:        redactText(envelope.ControllerExe),
		RunUID:               envelope.RunUID,
		DetectionConfidence:  envelope.DetectionConfidence,
		Status:               envelope.Status,
		CoverageLevel:        coverage,
		CoverageReasons:      mustJSON(reasons, []string{}),
		Metadata:             mustJSON(map[string]any{}, map[string]any{}),
		FirstSeenAt:          firstSeen,
		LastSeenAt:           lastSeen,
		StoppedAt:            stoppedAt,
	}
	return &model.AgentGuardStateProjection{EventType: eventType, ObjectID: id, Instance: instance}, nil
}

func normalizeUnitState(eventType string, hostID uuid.UUID, raw string) (*model.AgentGuardStateProjection, error) {
	var envelope unitStateEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || envelope.Schema != agentGuardSchema {
		return nil, fmt.Errorf("%w: execution unit schema", ErrAgentBehaviorInvalidContract)
	}
	id, err := uuid.Parse(envelope.ExecutionUnitID)
	if err != nil {
		return nil, fmt.Errorf("%w: execution_unit_id", ErrAgentBehaviorInvalidContract)
	}
	instanceID, err := uuid.Parse(envelope.InstanceID)
	if err != nil || !allowedValue(envelope.UnitType,
		"local_process_tree", "linux_namespace", "oci_container", "remote_sandbox", "whole_process_container") ||
		strings.TrimSpace(envelope.Fingerprint) == "" || len(envelope.Fingerprint) > 160 ||
		len(envelope.ContainerID) > 128 || len(envelope.ContainerRuntime) > 64 {
		return nil, fmt.Errorf("%w: execution unit identity", ErrAgentBehaviorInvalidContract)
	}
	if len(envelope.RemoteBackend) > 64 || len(envelope.RemoteExecutionID) > 255 || len(envelope.RemoteHostRef) > 255 {
		return nil, fmt.Errorf("%w: execution unit remote identity", ErrAgentBehaviorInvalidContract)
	}
	rootStartTicks := normalizeNumber(envelope.RootStartTicks)
	if envelope.RootPID != nil && (*envelope.RootPID <= 0 || rootStartTicks == "" || rootStartTicks == "0") {
		return nil, fmt.Errorf("%w: execution unit root", ErrAgentBehaviorInvalidContract)
	}
	var rootStartTicksValue *string
	if rootStartTicks != "" {
		rootStartTicksValue = &rootStartTicks
	}
	if !allowedValue(envelope.Status,
		"observed", "healthy", "violating", "freezing", "frozen", "resuming", "stopped", "stale", "unobservable", "degraded") {
		return nil, fmt.Errorf("%w: execution unit status", ErrAgentBehaviorInvalidContract)
	}
	coverage, reasons, err := normalizeP1Coverage(envelope.CoverageLevel, envelope.CoverageReasons)
	if err != nil {
		return nil, err
	}
	if envelope.UnitType == "remote_sandbox" {
		if strings.TrimSpace(envelope.RemoteBackend) == "" || strings.TrimSpace(envelope.RemoteExecutionID) == "" {
			return nil, fmt.Errorf("%w: remote sandbox reference", ErrAgentBehaviorInvalidContract)
		}
		coverage = "remote_unobservable"
		reasons = appendUnique(reasons, "remote_sensor_unverified")
	} else if envelope.RemoteBackend != "" || envelope.RemoteExecutionID != "" || envelope.RemoteHostRef != "" {
		return nil, fmt.Errorf("%w: remote metadata on local unit", ErrAgentBehaviorInvalidContract)
	}
	firstSeen, lastSeen, err := parseStateTimes(envelope.FirstSeenAt, envelope.LastSeenAt)
	if err != nil {
		return nil, err
	}
	var stoppedAt *time.Time
	if eventType == "agent_execution_unit_stopped" {
		if envelope.Status != "stopped" {
			return nil, fmt.Errorf("%w: execution unit stopped status", ErrAgentBehaviorInvalidContract)
		}
		stoppedAt = &lastSeen
	}
	if envelope.IsolationBaseline == nil {
		envelope.IsolationBaseline = map[string]any{}
	}
	if envelope.IsolationActual == nil {
		envelope.IsolationActual = map[string]any{}
	}
	if envelope.IsolationDiff == nil {
		envelope.IsolationDiff = map[string]any{}
	}
	if envelope.Capabilities != nil {
		envelope.IsolationActual["capabilities"] = envelope.Capabilities
	}
	if envelope.Completeness != nil {
		envelope.IsolationActual["completeness"] = envelope.Completeness
	}
	unit := &model.AgentExecutionUnit{
		ID:                id,
		HostID:            hostID,
		InstanceID:        instanceID,
		UnitType:          envelope.UnitType,
		Fingerprint:       truncateLimit(envelope.Fingerprint, 160),
		RootPID:           envelope.RootPID,
		RootStartTicks:    rootStartTicksValue,
		CgroupPath:        redactText(envelope.CgroupPath),
		ContainerID:       truncateLimit(envelope.ContainerID, 128),
		ContainerRuntime:  truncateLimit(envelope.ContainerRuntime, 64),
		RemoteBackend:     truncateLimit(envelope.RemoteBackend, 64),
		RemoteExecutionID: redactText(envelope.RemoteExecutionID),
		RemoteHostRef:     redactText(envelope.RemoteHostRef),
		CoverageLevel:     coverage,
		CoverageReasons:   mustJSON(reasons, []string{}),
		IsolationBaseline: mustJSON(sanitizeValue(envelope.IsolationBaseline, ""), map[string]any{}),
		IsolationActual:   mustJSON(sanitizeValue(envelope.IsolationActual, ""), map[string]any{}),
		IsolationDiff:     mustJSON(sanitizeValue(envelope.IsolationDiff, ""), map[string]any{}),
		Status:            envelope.Status,
		FirstSeenAt:       firstSeen,
		LastSeenAt:        lastSeen,
		StoppedAt:         stoppedAt,
	}
	return &model.AgentGuardStateProjection{EventType: eventType, ObjectID: id, Unit: unit}, nil
}

func normalizeSessionState(eventType string, hostID uuid.UUID, raw string) (*model.AgentGuardStateProjection, error) {
	var envelope sessionStateEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || envelope.Schema != agentGuardSchema {
		return nil, fmt.Errorf("%w: behavior session schema", ErrAgentBehaviorInvalidContract)
	}
	id, err := uuid.Parse(envelope.SessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: session_id", ErrAgentBehaviorInvalidContract)
	}
	instanceID, err := uuid.Parse(envelope.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("%w: session instance_id", ErrAgentBehaviorInvalidContract)
	}
	unitID, err := uuid.Parse(envelope.ExecutionUnitID)
	if err != nil {
		return nil, fmt.Errorf("%w: session execution_unit_id", ErrAgentBehaviorInvalidContract)
	}
	if !allowedValue(envelope.Source, "agent_official", "adapter_hook", "aegis_wrapper", "execution_unit", "activity_window") ||
		!allowedValue(envelope.Confidence, "confirmed", "probable", "inferred") ||
		!allowedValue(envelope.Status, "active", "ended", "stale") {
		return nil, fmt.Errorf("%w: session classification", ErrAgentBehaviorInvalidContract)
	}
	if (envelope.Source == "execution_unit" || envelope.Source == "activity_window") && envelope.Confidence != "inferred" {
		return nil, fmt.Errorf("%w: inferred session source claimed trust", ErrAgentBehaviorInvalidContract)
	}
	if (envelope.Source == "agent_official" || envelope.Source == "adapter_hook" || envelope.Source == "aegis_wrapper") &&
		envelope.Confidence != "confirmed" {
		return nil, fmt.Errorf("%w: trusted session source without confirmed attribution", ErrAgentBehaviorInvalidContract)
	}
	if envelope.CorrelationTokenHash != "" && !isSHA256Reference(envelope.CorrelationTokenHash) {
		return nil, fmt.Errorf("%w: correlation token hash", ErrAgentBehaviorInvalidContract)
	}
	if envelope.Source == "aegis_wrapper" && envelope.CorrelationTokenHash == "" {
		return nil, fmt.Errorf("%w: wrapper correlation hash", ErrAgentBehaviorInvalidContract)
	}
	if len(envelope.ExternalSessionID) > 255 {
		return nil, fmt.Errorf("%w: external session identity", ErrAgentBehaviorInvalidContract)
	}
	startedAt, lastSeen, err := parseStateTimes(envelope.StartedAt, envelope.LastSeenAt)
	if err != nil {
		return nil, err
	}
	if envelope.Completeness == nil {
		envelope.Completeness = map[string]any{}
	}
	var endedAt *time.Time
	if eventType == "agent_behavior_session_stopped" {
		if envelope.Status != "ended" {
			return nil, fmt.Errorf("%w: session stopped status", ErrAgentBehaviorInvalidContract)
		}
		endedAt = &lastSeen
	}
	var externalSessionID, correlationTokenHash *string
	if envelope.ExternalSessionID != "" {
		value := redactText(envelope.ExternalSessionID)
		externalSessionID = &value
	}
	if envelope.CorrelationTokenHash != "" {
		value := envelope.CorrelationTokenHash
		correlationTokenHash = &value
	}
	session := &model.AgentBehaviorSession{
		ID:                   id,
		HostID:               hostID,
		InstanceID:           instanceID,
		ExecutionUnitID:      &unitID,
		ExternalSessionID:    externalSessionID,
		Source:               envelope.Source,
		Confidence:           envelope.Confidence,
		CorrelationTokenHash: correlationTokenHash,
		Status:               envelope.Status,
		Completeness:         mustJSON(sanitizeValue(envelope.Completeness, ""), map[string]any{}),
		StartedAt:            startedAt,
		LastSeenAt:           lastSeen,
		EndedAt:              endedAt,
	}
	return &model.AgentGuardStateProjection{EventType: eventType, ObjectID: id, Session: session}, nil
}

func normalizeP1Coverage(level string, reasons []string) (string, []string, error) {
	if reasons == nil {
		reasons = []string{}
	}
	if len(reasons) > 32 {
		return "", nil, fmt.Errorf("%w: coverage reasons", ErrAgentBehaviorInvalidContract)
	}
	for index := range reasons {
		reasons[index] = redactText(truncateLimit(reasons[index], 128))
	}
	switch level {
	case "":
		return "monitor_only", appendUnique(reasons, "p1_monitor_only"), nil
	case "full_enforcement", "behavior_monitor_escape_enforce", "monitor_only", "no_isolation", "remote_unobservable", "unsupported_profile", "degraded":
		return level, reasons, nil
	default:
		return "", nil, fmt.Errorf("%w: coverage_level", ErrAgentBehaviorInvalidContract)
	}
}

func parseStateTimes(firstValue, lastValue string) (time.Time, time.Time, error) {
	first, err := time.Parse(time.RFC3339Nano, firstValue)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: first_seen_at", ErrAgentBehaviorInvalidContract)
	}
	last, err := time.Parse(time.RFC3339Nano, lastValue)
	if err != nil || last.Before(first) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: last_seen_at", ErrAgentBehaviorInvalidContract)
	}
	return first, last, nil
}
