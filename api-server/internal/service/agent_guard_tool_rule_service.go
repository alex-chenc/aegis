package service

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

const (
	agentBehaviorSchemaV1        = "aegis.agent_behavior.v1"
	agentToolRuleMatchConfidence = 0.95
)

type AgentGuardToolFindingWriter interface {
	UpsertToolFinding(context.Context, *model.AgentSecurityFinding) error
}

type AgentGuardToolRuleCatalog interface {
	GetRule(context.Context, string, int64) (*model.AgentBehaviorRuleDefinition, error)
}

// AgentGuardToolRuleHandler owns rule matching for trusted upper-layer tool
// events. It deliberately does not consume process/file/network eBPF events.
type AgentGuardToolRuleHandler struct {
	writer      AgentGuardToolFindingWriter
	catalog     AgentGuardToolRuleCatalog
	broadcaster agentGuardWSBroadcaster
	logger      *zap.Logger
}

func NewAgentGuardToolRuleHandler(
	writer AgentGuardToolFindingWriter,
	catalog AgentGuardToolRuleCatalog,
	broadcaster agentGuardWSBroadcaster,
	logger *zap.Logger,
) *AgentGuardToolRuleHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgentGuardToolRuleHandler{
		writer: writer, catalog: catalog, broadcaster: broadcaster, logger: logger,
	}
}

type agentGuardToolKafkaEnvelope struct {
	EventID       string          `json:"event_id"`
	HostID        string          `json:"host_id"`
	EventType     string          `json:"event_type"`
	EventData     json.RawMessage `json:"event_data"`
	EventDataJSON json.RawMessage `json:"event_data_json"`
}

type agentGuardToolBehaviorEnvelope struct {
	Schema          string                 `json:"schema"`
	EventID         string                 `json:"event_id"`
	HostID          string                 `json:"host_id"`
	HostBootID      string                 `json:"host_boot_id"`
	AgentSequence   int64                  `json:"agent_sequence"`
	InstanceID      string                 `json:"instance_id"`
	SessionID       string                 `json:"session_id"`
	ExecutionUnitID string                 `json:"execution_unit_id"`
	AgentType       string                 `json:"agent_type"`
	ProfileKey      string                 `json:"profile_key"`
	OccurredAt      time.Time              `json:"occurred_at"`
	Category        string                 `json:"category"`
	Operation       string                 `json:"operation"`
	Outcome         string                 `json:"outcome"`
	Decision        string                 `json:"decision"`
	Severity        string                 `json:"severity"`
	Actor           agentGuardToolActor    `json:"actor"`
	Resource        agentGuardToolResource `json:"resource"`
	Collection      map[string]any         `json:"collection"`
	Evidence        map[string]any         `json:"evidence"`
	CorrelationID   string                 `json:"correlation_id"`
}

type agentGuardToolActor struct {
	PID        int             `json:"pid"`
	PPID       int             `json:"ppid"`
	StartTicks json.RawMessage `json:"start_ticks"`
	Name       string          `json:"name"`
	Exe        string          `json:"exe"`
	Argv       []string        `json:"argv"`
	CWD        string          `json:"cwd"`
}

type agentGuardToolResource struct {
	Type           string         `json:"type"`
	Identity       string         `json:"identity"`
	Classification string         `json:"classification"`
	Attributes     map[string]any `json:"attributes"`
}

type agentGuardToolMatch struct {
	Definition     model.AgentBehaviorRuleDefinition
	Classification string
}

func (h *AgentGuardToolRuleHandler) HandleKafkaMessage(
	ctx context.Context,
	_ []byte,
	value []byte,
) error {
	var envelope agentGuardToolKafkaEnvelope
	if err := json.Unmarshal(value, &envelope); err != nil {
		return fmt.Errorf("decode agent guard tool event envelope: %w", err)
	}
	if envelope.EventType != "agent_behavior" {
		return nil
	}
	raw, err := decodeAgentGuardToolPayload(envelope.EventDataJSON, envelope.EventData)
	if err != nil {
		return err
	}
	var event agentGuardToolBehaviorEnvelope
	if err := json.Unmarshal(raw, &event); err != nil {
		return fmt.Errorf("decode agent guard tool behavior: %w", err)
	}
	if event.Schema != agentBehaviorSchemaV1 || event.Category != "tool" ||
		(event.Operation != "tool_call_started" && event.Operation != "tool_call_completed" && event.Operation != "tool_call_failed") {
		return nil
	}
	if envelope.EventID == "" || envelope.EventID != event.EventID || event.HostID == "" || envelope.HostID != event.HostID {
		return fmt.Errorf("agent guard tool event identity mismatch")
	}
	hostID, err := uuid.Parse(event.HostID)
	if err != nil {
		return fmt.Errorf("agent guard tool event host id: %w", err)
	}
	instanceID, err := uuid.Parse(event.InstanceID)
	if err != nil {
		return fmt.Errorf("agent guard tool event instance id: %w", err)
	}
	sessionID, err := uuid.Parse(event.SessionID)
	if err != nil {
		return fmt.Errorf("agent guard tool event session id: %w", err)
	}
	unitID, err := uuid.Parse(event.ExecutionUnitID)
	if err != nil {
		return fmt.Errorf("agent guard tool event execution unit id: %w", err)
	}

	matched, ok := matchAgentGuardToolCommand(event.Resource)
	if !ok {
		h.logger.Debug("agent_guard_tool_rule_not_matched",
			zap.String("event_id", event.EventID), zap.String("host_id", event.HostID),
			zap.String("tool_name", event.Resource.Identity),
		)
		return nil
	}
	if h.writer == nil {
		return fmt.Errorf("agent guard tool finding writer is unavailable")
	}
	definition := matched.Definition
	if h.catalog != nil {
		if stored, catalogErr := h.catalog.GetRule(ctx, definition.RuleKey, definition.RuleVersion); catalogErr == nil && stored != nil {
			definition = *stored
		} else if catalogErr != nil {
			h.logger.Warn("agent_guard_tool_rule_catalog_fallback",
				zap.String("event_id", event.EventID), zap.String("rule_key", definition.RuleKey),
				zap.Error(catalogErr),
			)
		}
	}
	finding := buildAgentGuardToolFinding(hostID, instanceID, sessionID, unitID, event, definition, matched.Classification)
	if err := h.writer.UpsertToolFinding(ctx, finding); err != nil {
		return err
	}
	h.logger.Info("agent_guard_tool_rule_matched",
		zap.String("event_id", event.EventID), zap.String("host_id", event.HostID),
		zap.String("session_id", event.SessionID), zap.String("rule_key", definition.RuleKey),
		zap.String("rule_name", definition.Name), zap.String("classification", matched.Classification),
		zap.String("rule_owner", "api-server"),
	)
	if h.broadcaster != nil {
		h.broadcaster.Broadcast(WSMessage{
			Type: "agent_guard.finding_updated",
			Data: map[string]any{
				"finding_id": finding.ID.String(), "host_id": event.HostID,
				"session_id": event.SessionID, "execution_unit_id": event.ExecutionUnitID,
				"severity": finding.Severity, "created": true,
			},
		})
	}
	return nil
}

func decodeAgentGuardToolPayload(values ...json.RawMessage) ([]byte, error) {
	for _, value := range values {
		value = []byte(strings.TrimSpace(string(value)))
		if len(value) == 0 || string(value) == "null" {
			continue
		}
		if value[0] == '"' {
			var payload string
			if err := json.Unmarshal(value, &payload); err != nil {
				return nil, fmt.Errorf("decode agent guard tool event data string: %w", err)
			}
			return []byte(payload), nil
		}
		return value, nil
	}
	return nil, fmt.Errorf("agent guard tool event data is empty")
}

func matchAgentGuardToolCommand(resource agentGuardToolResource) (agentGuardToolMatch, bool) {
	command := ""
	if resource.Attributes != nil {
		if value, ok := resource.Attributes["command"].(string); ok {
			command = strings.TrimSpace(value)
		}
		if command == "" {
			if input, ok := resource.Attributes["tool_input"].(map[string]any); ok {
				for _, key := range []string{"command", "cmdline", "command_line", "script"} {
					if value, ok := input[key].(string); ok && strings.TrimSpace(value) != "" {
						command = strings.TrimSpace(value)
						break
					}
				}
			}
		}
	}
	if command == "" {
		return agentGuardToolMatch{}, false
	}
	for _, rawToken := range strings.Fields(command) {
		token := strings.Trim(rawToken, "\"'`;,()[]{}")
		executable := filepath.Base(token)
		if classification := classifyAgentGuardToolExecutable(executable); classification != "" {
			definition := agentGuardSensitiveCommandDefinition()
			return agentGuardToolMatch{Definition: definition, Classification: classification}, true
		}
	}
	return agentGuardToolMatch{}, false
}

func classifyAgentGuardToolExecutable(executable string) string {
	switch executable {
	case "curl", "wget", "nc", "ncat", "socat", "ssh", "scp":
		return "network_transfer"
	case "sudo", "su", "pkexec":
		return "privilege"
	case "chmod", "chown", "setfacl", "setcap":
		return "permission_change"
	case "nsenter", "unshare", "mount", "umount", "chroot":
		return "namespace_mount"
	case "useradd", "usermod", "crontab", "systemctl":
		return "account_persistence"
	case "rm", "dd", "shred":
		return "destructive"
	case "auditctl", "iptables", "nft", "ufw":
		return "security_control"
	default:
		if strings.HasPrefix(executable, "mkfs") {
			return "destructive"
		}
		return ""
	}
}

func agentGuardSensitiveCommandDefinition() model.AgentBehaviorRuleDefinition {
	for _, definition := range model.BuiltinAgentBehaviorRuleManifest() {
		if definition.RuleKey == model.AgentGuardRuleKeySensitiveCommand {
			return definition
		}
	}
	return model.AgentBehaviorRuleDefinition{}
}

func buildAgentGuardToolFinding(
	hostID, instanceID, sessionID, unitID uuid.UUID,
	event agentGuardToolBehaviorEnvelope,
	definition model.AgentBehaviorRuleDefinition,
	classification string,
) *model.AgentSecurityFinding {
	findingKey := "tool-command:v1:" + definition.RuleKey + ":" + toolCallIdentity(event)
	findingID := uuid.NewSHA1(uuid.Nil, []byte(findingKey))
	occurredAt := event.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	decision := event.Decision
	if decision == "" {
		decision = "alert"
	}
	severity := definition.DefaultSeverity
	if severity == "" {
		severity = "medium"
	}
	outcome := event.Outcome
	if outcome == "" {
		outcome = "unknown"
	}
	hit := map[string]any{
		"rule_id":                 definition.ID.String(),
		"rule_key":                definition.RuleKey,
		"rule_version":            definition.RuleVersion,
		"rule_digest":             definition.Digest,
		"rule_name":               definition.Name,
		"event_id":                event.EventID,
		"tool_call_id":            toolCallID(event.Resource.Attributes),
		"severity":                severity,
		"decision":                decision,
		"confidence":              agentToolRuleMatchConfidence,
		"match_kind":              "tool_command_line",
		"attack_stage":            "execution",
		"outcome":                 outcome,
		"resource_classification": classification,
	}
	return &model.AgentSecurityFinding{
		ID:              findingID,
		FindingKey:      findingKey,
		HostID:          hostID,
		InstanceID:      &instanceID,
		SessionID:       &sessionID,
		ExecutionUnitID: &unitID,
		Title:           definition.Name,
		Severity:        severity,
		Verdict:         "suspicious",
		Confidence:      agentToolRuleMatchConfidence,
		Status:          "open",
		// Keep the public decision-source contract (`rule|ai|combined`). The
		// owner/source detail is carried in EvidenceGraph, which is returned by
		// the finding detail endpoint.
		DecisionSources:  datatypes.JSON(`["rule"]`),
		RuleHits:         mustAgentGuardToolJSON([]any{hit}),
		EvidenceEventIDs: mustAgentGuardToolJSON([]string{event.EventID}),
		EvidenceGraph: mustAgentGuardToolJSON(map[string]any{
			"rule_owner": "api-server", "source": "agent_hook_tool_event",
			"event_ids": []string{event.EventID}, "correlation_status": toolCorrelationStatus(event.Resource.Attributes),
		}),
		AttackStages:      datatypes.JSON(`["execution"]`),
		Summary:           "上层工具命令命中安全规则，命中事实来自智能体 Hook 工具事件。",
		RecommendedAction: definition.RecommendedAction,
		FirstObservedAt:   occurredAt,
		LastObservedAt:    occurredAt,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
}

func toolCallIdentity(event agentGuardToolBehaviorEnvelope) string {
	if callID := toolCallID(event.Resource.Attributes); callID != "" {
		return event.SessionID + ":" + callID
	}
	return event.EventID
}

func toolCallID(attributes map[string]any) string {
	if attributes != nil {
		if value, ok := attributes["tool_call_id"].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func toolCorrelationStatus(attributes map[string]any) string {
	if attributes != nil {
		if value, ok := attributes["correlation_status"].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "unmatched"
}

func mustAgentGuardToolJSON(value any) datatypes.JSON {
	data, err := json.Marshal(value)
	if err != nil {
		return datatypes.JSON(`{}`)
	}
	return datatypes.JSON(data)
}
