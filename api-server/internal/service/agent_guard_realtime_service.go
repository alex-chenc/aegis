package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"api-server/internal/model"

	"go.uber.org/zap"
)

type agentGuardWSBroadcaster interface {
	Broadcast(WSMessage)
}

type agentGuardActionStatusUpdater interface {
	ApplyReportedStatus(context.Context, AgentGuardActionStatusReport) (*model.AgentGuardAction, error)
}

type AgentGuardRealtimeHandler struct {
	broadcaster agentGuardWSBroadcaster
	actions     agentGuardActionStatusUpdater
	logger      *zap.Logger
}

func NewAgentGuardRealtimeHandler(
	broadcaster agentGuardWSBroadcaster,
	logger *zap.Logger,
) *AgentGuardRealtimeHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgentGuardRealtimeHandler{broadcaster: broadcaster, logger: logger}
}

func (h *AgentGuardRealtimeHandler) SetActionStatusUpdater(actions agentGuardActionStatusUpdater) {
	h.actions = actions
}

func (h *AgentGuardRealtimeHandler) HandleKafkaMessage(
	ctx context.Context,
	_ []byte,
	value []byte,
) error {
	var envelope struct {
		EventID       string          `json:"event_id"`
		HostID        string          `json:"host_id"`
		EventType     string          `json:"event_type"`
		Severity      string          `json:"severity"`
		EventData     json.RawMessage `json:"event_data"`
		EventDataJSON json.RawMessage `json:"event_data_json"`
	}
	if err := json.Unmarshal(value, &envelope); err != nil {
		return fmt.Errorf("decode agent guard websocket envelope: %w", err)
	}
	messageType := agentGuardWebSocketMessageType(envelope.EventType)
	if messageType == "" {
		return nil
	}

	rawData := envelope.EventData
	if len(rawData) == 0 {
		rawData = envelope.EventDataJSON
	}
	var payloadText string
	if len(rawData) > 0 && rawData[0] == '"' {
		if err := json.Unmarshal(rawData, &payloadText); err != nil {
			return fmt.Errorf("decode agent guard websocket event data: %w", err)
		}
	} else {
		payloadText = string(rawData)
	}
	var detail struct {
		InstanceID      string `json:"instance_id"`
		SessionID       string `json:"session_id"`
		ExecutionUnitID string `json:"execution_unit_id"`
		Category        string `json:"category"`
		Operation       string `json:"operation"`
		Status          string `json:"status"`
		BundleVersion   int64  `json:"bundle_version"`
		CoverageLevel   string `json:"coverage_level"`
		ActionID        string `json:"action_id"`
		CommandID       string `json:"command_id"`
		Action          string `json:"action"`
	}
	if strings.TrimSpace(payloadText) != "" {
		if err := json.Unmarshal([]byte(payloadText), &detail); err != nil {
			h.logger.Debug("agent_guard_websocket_summary_parse_failed",
				zap.String("event_id", envelope.EventID),
				zap.String("event_type", envelope.EventType),
				zap.Error(err),
			)
		}
	}
	if envelope.EventType == "agent_guard_action_status" {
		if h.actions == nil {
			return fmt.Errorf("agent guard action status updater is unavailable")
		}
		var report AgentGuardActionStatusReport
		if err := json.Unmarshal([]byte(payloadText), &report); err != nil {
			return fmt.Errorf("decode agent guard action status: %w", err)
		}
		report.HostID = envelope.HostID
		updated, err := h.actions.ApplyReportedStatus(ctx, report)
		if err != nil {
			h.logger.Warn("agent_guard_action_status_rejected",
				zap.String("event_id", envelope.EventID),
				zap.String("host_id", envelope.HostID),
				zap.String("command_id", detail.CommandID),
				zap.String("action", detail.Action),
				zap.String("status", detail.Status),
				zap.Error(err),
			)
			return err
		}
		detail.ActionID = updated.ID.String()
		detail.CommandID = updated.CommandID
		detail.Action = updated.Action
		detail.Status = updated.Status
	}
	if h.broadcaster == nil {
		return nil
	}
	h.broadcaster.Broadcast(WSMessage{
		Type: messageType,
		Data: map[string]any{
			"event_id":          envelope.EventID,
			"host_id":           envelope.HostID,
			"instance_id":       detail.InstanceID,
			"session_id":        detail.SessionID,
			"execution_unit_id": detail.ExecutionUnitID,
			"category":          detail.Category,
			"operation":         detail.Operation,
			"action_id":         detail.ActionID,
			"command_id":        detail.CommandID,
			"action":            detail.Action,
			"status":            detail.Status,
			"bundle_version":    detail.BundleVersion,
			"coverage_level":    detail.CoverageLevel,
			"severity":          envelope.Severity,
		},
	})
	return nil
}

func agentGuardWebSocketMessageType(eventType string) string {
	switch eventType {
	case "agent_instance_started", "agent_instance_updated", "agent_instance_stopped",
		"agent_behavior_session_started", "agent_behavior_session_updated", "agent_behavior_session_stopped",
		"agent_execution_unit_started", "agent_execution_unit_updated", "agent_execution_unit_stopped",
		"agent_guard_health":
		return "agent_guard.instance_updated"
	case "agent_behavior", "agent_sandbox_violation", "agent_isolation_drift":
		return "agent_guard.behavior_created"
	case "agent_guard_config_status":
		return "agent_guard.delivery_updated"
	case "agent_guard_action_status":
		return "agent_guard.action_updated"
	default:
		return ""
	}
}
