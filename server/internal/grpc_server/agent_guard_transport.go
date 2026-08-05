package grpc_server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"server/internal/queue"
	pb "server/pkg/api/v1"
	"server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const (
	agentGuardBundleConfigType          = "agent_guard_bundle"
	agentGuardBundleSchema              = "aegis.agent_guard.bundle.v1"
	agentGuardRuntimeSettingsConfigType = "agent_guard_runtime_settings"
	agentGuardRuntimeSettingsSchema     = "aegis.agent_guard.runtime_settings.v1"
	agentGuardRuntimeSettingsKeyPrefix  = "agent_guard.runtime."
)

type agentGuardRuntimeHookInjection struct {
	AgentType       string `json:"agent_type"`
	Enabled         bool   `json:"enabled"`
	BehaviorEnabled bool   `json:"behavior_enabled"`
	EscapeEnabled   bool   `json:"escape_enabled"`
}

type agentGuardRuntimeSettingsPayload struct {
	Schema                string                           `json:"schema"`
	Version               int64                            `json:"version"`
	HostID                string                           `json:"host_id"`
	ToolAdapterEnabled    bool                             `json:"tool_adapter_enabled"`
	SessionHookEnabled    bool                             `json:"session_hook_enabled"`
	BehaviorPolicyEnabled bool                             `json:"behavior_policy_enabled"`
	EscapePolicyEnabled   bool                             `json:"escape_policy_enabled"`
	Injections            []agentGuardRuntimeHookInjection `json:"injections"`
}

var (
	errAgentGuardBundleInvalid  = errors.New("invalid agent guard bundle envelope")
	errAgentGuardBundleStale    = errors.New("stale agent guard bundle")
	errAgentGuardBundleConflict = errors.New("conflicting agent guard bundle version")
)

type agentGuardEventEnvelope struct {
	Schema        string          `json:"schema"`
	EventID       string          `json:"event_id"`
	HostID        string          `json:"host_id"`
	HostBootID    string          `json:"host_boot_id"`
	AgentSequence json.RawMessage `json:"agent_sequence"`
	InstanceID    string          `json:"instance_id"`
	Agent         struct {
		InstanceID string `json:"instance_id"`
	} `json:"agent"`
}

type agentGuardBundleEnvelope struct {
	Schema        string `json:"schema"`
	BundleVersion int64  `json:"bundle_version"`
	HostID        string `json:"host_id"`
	Digest        string `json:"digest"`
}

type agentGuardBundleSnapshot struct {
	Config      *pb.ConfigSync
	Version     int64
	Digest      string
	Schema      string
	PayloadHash [sha256.Size]byte
	CachedAt    time.Time
}

func normalizeRuntimeEvent(hostID string, event *pb.RuntimeEvent) (*pb.RuntimeEvent, queue.SecurityEventMetadata) {
	if event == nil {
		event = &pb.RuntimeEvent{}
	}
	normalized := proto.Clone(event).(*pb.RuntimeEvent)
	envelope, _ := decodeAgentGuardEventEnvelope(normalized.EventDataJson)

	if normalized.EventId == "" ||
		(isAgentGuardFamilyEvent(normalized.EventType) && !isUUID(normalized.EventId)) {
		if _, err := uuid.Parse(envelope.EventID); err == nil {
			normalized.EventId = envelope.EventID
		} else {
			normalized.EventId = uuid.NewString()
		}
	}
	normalized.HostId = hostID

	instanceID := envelope.InstanceID
	if instanceID == "" {
		instanceID = envelope.Agent.InstanceID
	}
	if !isUUID(instanceID) {
		instanceID = ""
	}
	hostBootID := envelope.HostBootID
	if !isUUID(hostBootID) {
		hostBootID = ""
	}
	partitionKey := hostID
	if instanceID != "" {
		partitionKey += ":" + instanceID
	}

	return normalized, queue.SecurityEventMetadata{
		PartitionKey:  partitionKey,
		EventID:       safeRoutingToken(normalized.EventId, 128),
		HostID:        hostID,
		InstanceID:    instanceID,
		HostBootID:    hostBootID,
		AgentSequence: positiveIntegerString(envelope.AgentSequence),
		EventType:     safeRoutingToken(normalized.EventType, 64),
		Schema:        agentGuardSchemaToken(envelope.Schema),
	}
}

func decodeAgentGuardEventEnvelope(payload string) (agentGuardEventEnvelope, error) {
	var envelope agentGuardEventEnvelope
	if strings.TrimSpace(payload) == "" {
		return envelope, nil
	}
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil {
		return agentGuardEventEnvelope{}, err
	}
	return envelope, nil
}

func jsonScalarString(value json.RawMessage) string {
	value = bytes.TrimSpace(value)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return ""
	}
	if value[0] == '"' {
		var text string
		if json.Unmarshal(value, &text) == nil {
			return text
		}
		return ""
	}
	return string(value)
}

func positiveIntegerString(value json.RawMessage) string {
	text := jsonScalarString(value)
	if text == "" {
		return ""
	}
	number, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return ""
	}
	return strconv.FormatUint(number, 10)
}

func isUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func safeRoutingToken(value string, maxLength int) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxLength {
		return ""
	}
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		case strings.ContainsRune("._:-", character):
		default:
			return ""
		}
	}
	return value
}

func agentGuardSchemaToken(value string) string {
	switch value {
	case "aegis.agent_behavior.v1", "aegis.agent_guard.v1":
		return value
	default:
		return ""
	}
}

func isAgentGuardFamilyEvent(eventType string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(eventType)), "agent_")
}

func shouldCreateLegacyAlert(event *pb.RuntimeEvent) bool {
	return event != nil &&
		event.MatchedRuleId != "" &&
		!isAgentGuardFamilyEvent(event.EventType)
}

func parseAgentGuardBundle(
	hostID uuid.UUID,
	config *pb.ConfigSync,
) (agentGuardBundleSnapshot, error) {
	if config == nil ||
		config.ConfigType != agentGuardBundleConfigType ||
		config.Action != "full_sync" ||
		strings.TrimSpace(config.Payload) == "" {
		return agentGuardBundleSnapshot{}, errAgentGuardBundleInvalid
	}
	var envelope agentGuardBundleEnvelope
	if err := json.Unmarshal([]byte(config.Payload), &envelope); err != nil {
		return agentGuardBundleSnapshot{}, fmt.Errorf("%w: %v", errAgentGuardBundleInvalid, err)
	}
	if envelope.Schema != agentGuardBundleSchema ||
		envelope.BundleVersion < 1 ||
		!isSHA256Digest(envelope.Digest) {
		return agentGuardBundleSnapshot{}, errAgentGuardBundleInvalid
	}
	if envelope.HostID != "" && envelope.HostID != hostID.String() {
		return agentGuardBundleSnapshot{}, fmt.Errorf(
			"%w: bundle host %s does not match target host",
			errAgentGuardBundleInvalid,
			envelope.HostID,
		)
	}
	return agentGuardBundleSnapshot{
		Config: &pb.ConfigSync{
			ConfigType: config.ConfigType,
			Action:     config.Action,
			Payload:    config.Payload,
		},
		Version:     envelope.BundleVersion,
		Digest:      envelope.Digest,
		Schema:      envelope.Schema,
		PayloadHash: sha256.Sum256([]byte(config.Payload)),
		CachedAt:    time.Now().UTC(),
	}, nil
}

func isSHA256Digest(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func (s *GRPCServer) cacheAgentGuardBundle(
	hostID uuid.UUID,
	config *pb.ConfigSync,
) (agentGuardBundleSnapshot, error) {
	incoming, err := parseAgentGuardBundle(hostID, config)
	if err != nil {
		return agentGuardBundleSnapshot{}, err
	}
	for {
		value, loaded := s.agentGuardBundles.LoadOrStore(hostID, incoming)
		if !loaded {
			return incoming, nil
		}
		current := value.(agentGuardBundleSnapshot)
		switch {
		case incoming.Version < current.Version:
			return current, fmt.Errorf(
				"%w: current=%d incoming=%d",
				errAgentGuardBundleStale,
				current.Version,
				incoming.Version,
			)
		case incoming.Version == current.Version &&
			(incoming.Digest != current.Digest || incoming.PayloadHash != current.PayloadHash):
			return current, fmt.Errorf(
				"%w: version=%d",
				errAgentGuardBundleConflict,
				incoming.Version,
			)
		}
		if s.agentGuardBundles.CompareAndSwap(hostID, current, incoming) {
			return incoming, nil
		}
	}
}

func (s *GRPCServer) loadAgentGuardBundle(
	hostID uuid.UUID,
) (agentGuardBundleSnapshot, bool) {
	value, ok := s.agentGuardBundles.Load(hostID)
	if !ok {
		return agentGuardBundleSnapshot{}, false
	}
	snapshot := value.(agentGuardBundleSnapshot)
	snapshot.Config = proto.Clone(snapshot.Config).(*pb.ConfigSync)
	return snapshot, true
}

func (s *GRPCServer) dispatchAgentConfig(
	ctx context.Context,
	connection *AgentConnection,
	config *pb.ConfigSync,
) error {
	if connection == nil || config == nil {
		return errors.New("agent config dispatch target unavailable")
	}
	if connection.Stream != nil {
		return connection.Stream.Send(&pb.CommandRequest{
			Request: &pb.CommandRequest_ConfigSync{ConfigSync: config},
		})
	}
	if connection.CallbackClient == nil {
		return errors.New("agent has no stream or callback config channel")
	}
	response, err := connection.CallbackClient.SyncConfig(ctx, &pb.ConfigSyncRequest{
		Configs: []*pb.ConfigSync{config},
	})
	if err != nil {
		return err
	}
	if response == nil || !response.Success {
		if response != nil && response.Message != "" {
			return errors.New(response.Message)
		}
		return errors.New("agent rejected config")
	}
	if applied, present := response.Applied[config.ConfigType]; present && !applied {
		return fmt.Errorf("agent did not apply config type %s", config.ConfigType)
	}
	return nil
}

func (s *GRPCServer) dispatchCachedAgentGuardBundle(
	ctx context.Context,
	hostID uuid.UUID,
	connection *AgentConnection,
) (bool, error) {
	snapshot, ok := s.loadAgentGuardBundle(hostID)
	if !ok {
		return false, nil
	}
	if err := s.dispatchAgentConfig(ctx, connection, snapshot.Config); err != nil {
		return false, err
	}
	return true, nil
}

func agentGuardBundleErrorCode(err error) string {
	switch {
	case errors.Is(err, errAgentGuardBundleStale):
		return "agent_guard_bundle_stale"
	case errors.Is(err, errAgentGuardBundleConflict):
		return "agent_guard_bundle_version_conflict"
	default:
		return "agent_guard_bundle_invalid"
	}
}

func agentConfigChannel(connection *AgentConnection) string {
	if connection != nil && connection.Stream != nil {
		return "stream"
	}
	if connection != nil && connection.CallbackClient != nil {
		return "callback"
	}
	return "unavailable"
}

func cloneConfigList(configs []*pb.ConfigSync) []*pb.ConfigSync {
	cloned := make([]*pb.ConfigSync, 0, len(configs))
	for _, config := range configs {
		if config != nil {
			cloned = append(cloned, proto.Clone(config).(*pb.ConfigSync))
		}
	}
	return cloned
}

func (s *GRPCServer) configsForAgent(hostID uuid.UUID) []*pb.ConfigSync {
	configs := cloneConfigList(s.buildAllConfigs())
	if snapshot, ok := s.loadAgentGuardBundle(hostID); ok {
		configs = append(configs, snapshot.Config)
	}
	if config, ok := s.loadAgentGuardRuntimeSettings(hostID); ok {
		configs = append(configs, config)
	}
	return configs
}

func (s *GRPCServer) cacheAgentGuardRuntimeSettings(hostID uuid.UUID, config *pb.ConfigSync) error {
	if config == nil || config.ConfigType != agentGuardRuntimeSettingsConfigType ||
		config.Action != "full_sync" || strings.TrimSpace(config.Payload) == "" {
		return errors.New("invalid agent guard runtime settings")
	}
	var envelope struct {
		Schema  string `json:"schema"`
		Version int64  `json:"version"`
		HostID  string `json:"host_id"`
	}
	if err := json.Unmarshal([]byte(config.Payload), &envelope); err != nil ||
		envelope.Schema != agentGuardRuntimeSettingsSchema || envelope.Version < 1 || envelope.HostID != hostID.String() {
		return errors.New("invalid agent guard runtime settings")
	}
	normalizedPayload, err := normalizeAgentGuardRuntimeSettingsPayload(config.Payload, hostID)
	if err != nil {
		return err
	}
	cached := proto.Clone(config).(*pb.ConfigSync)
	cached.Payload = normalizedPayload
	s.agentGuardRuntimeSettings.Store(hostID, cached)
	return nil
}

func (s *GRPCServer) loadAgentGuardRuntimeSettings(hostID uuid.UUID) (*pb.ConfigSync, bool) {
	value, ok := s.agentGuardRuntimeSettings.Load(hostID)
	if ok {
		return proto.Clone(value.(*pb.ConfigSync)).(*pb.ConfigSync), true
	}
	if s.systemConfigRepo == nil {
		return nil, false
	}
	stored, err := s.systemConfigRepo.GetByKey(agentGuardRuntimeSettingsKeyPrefix + hostID.String())
	if err != nil {
		return nil, false
	}
	normalizedPayload, err := normalizeAgentGuardRuntimeSettingsPayload(string(stored.ConfigValue), hostID)
	if err != nil {
		logger.Warn("agent_guard_runtime_settings_persisted_config_invalid",
			zap.String("host_id", hostID.String()),
			zap.String("error_code", "agent_guard_runtime_settings_invalid"))
		return nil, false
	}
	config := &pb.ConfigSync{
		ConfigType: agentGuardRuntimeSettingsConfigType,
		Action:     "full_sync",
		Payload:    normalizedPayload,
	}
	s.agentGuardRuntimeSettings.Store(hostID, config)
	return proto.Clone(config).(*pb.ConfigSync), true
}

func normalizeAgentGuardRuntimeSettingsPayload(payload string, hostID uuid.UUID) (string, error) {
	var settings agentGuardRuntimeSettingsPayload
	if err := json.Unmarshal([]byte(payload), &settings); err != nil ||
		settings.Schema != agentGuardRuntimeSettingsSchema || settings.Version < 1 ||
		settings.HostID != hostID.String() {
		return "", errors.New("invalid agent guard runtime settings")
	}
	seen := make(map[string]struct{}, len(settings.Injections))
	for _, injection := range settings.Injections {
		if _, ok := seen[injection.AgentType]; ok {
			return "", errors.New("duplicate agent guard runtime hook agent type")
		}
		seen[injection.AgentType] = struct{}{}
	}
	normalized, err := json.Marshal(settings)
	if err != nil {
		return "", err
	}
	return string(normalized), nil
}
