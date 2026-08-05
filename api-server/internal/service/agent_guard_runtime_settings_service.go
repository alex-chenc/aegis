package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"api-server/internal/model"
	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const AgentGuardRuntimeSettingsConfigType = "agent_guard_runtime_settings"

var ErrAgentGuardRuntimeSettingsInvalid = errors.New("agent_guard_runtime_settings_invalid")

type agentGuardRuntimeSettingsStore interface {
	Get(string) (*model.AgentGuardRuntimeSettings, error)
	Upsert(*model.AgentGuardRuntimeSettings) error
}

type agentGuardRuntimeSettingsDispatcher interface {
	SyncAgentConfig(context.Context, string, []*pb.AgentConfig) (int32, error)
}

type AgentGuardRuntimeSettingsService struct {
	store      agentGuardRuntimeSettingsStore
	dispatcher agentGuardRuntimeSettingsDispatcher
	logger     *zap.Logger
	now        func() time.Time
}

func NewAgentGuardRuntimeSettingsService(
	store agentGuardRuntimeSettingsStore,
	dispatcher agentGuardRuntimeSettingsDispatcher,
	logger *zap.Logger,
) *AgentGuardRuntimeSettingsService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgentGuardRuntimeSettingsService{
		store: store, dispatcher: dispatcher, logger: logger, now: time.Now,
	}
}

func (s *AgentGuardRuntimeSettingsService) Get(ctx context.Context, hostID uuid.UUID) (*model.AgentGuardRuntimeSettings, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("agent guard runtime settings unavailable")
	}
	settings, err := s.store.Get(hostID.String())
	if err != nil {
		return nil, err
	}
	if settings.DispatchStatus == "pending_reconnect" && s.dispatcher != nil {
		affected, dispatchErr := s.dispatchRuntimeSettings(ctx, settings)
		if dispatchErr == nil && affected > 0 {
			settings.DispatchStatus = "dispatched"
			settings.DispatchErrorCode = ""
			for index := range settings.Injections {
				if settings.Injections[index].Enabled {
					settings.Injections[index].Status = "dispatched"
				}
			}
			settings.UpdatedAt = s.now().UTC()
			if err := s.store.Upsert(settings); err != nil {
				s.logger.Warn("agent_guard_runtime_settings_reconcile_status_save_failed",
					zap.String("host_id", hostID.String()), zap.Error(err))
			}
		} else if dispatchErr != nil {
			s.logger.Debug("agent_guard_runtime_settings_reconcile_pending",
				zap.String("host_id", hostID.String()), zap.Error(dispatchErr))
		}
	}
	return settings, nil
}

func (s *AgentGuardRuntimeSettingsService) Update(
	ctx context.Context,
	requested model.AgentGuardRuntimeSettings,
	operator string,
) (*model.AgentGuardRuntimeSettings, error) {
	if s == nil || s.store == nil || s.dispatcher == nil {
		return nil, fmt.Errorf("agent guard runtime settings unavailable")
	}
	hostID, err := uuid.Parse(strings.TrimSpace(requested.HostID))
	if err != nil {
		return nil, ErrAgentGuardRuntimeSettingsInvalid
	}
	settings := model.DefaultAgentGuardRuntimeSettings(hostID.String())
	settings.Version = s.now().UTC().UnixMilli()
	settings.ToolAdapterEnabled = requested.ToolAdapterEnabled
	settings.SessionHookEnabled = requested.SessionHookEnabled
	settings.BehaviorPolicyEnabled = requested.BehaviorPolicyEnabled
	settings.EscapePolicyEnabled = requested.EscapePolicyEnabled
	settings.Injections = normalizeRuntimeInjections(requested.Injections, settings.Injections)
	settings.UpdatedAt = s.now().UTC()
	settings.DispatchStatus = "pending"

	if err := s.store.Upsert(&settings); err != nil {
		return nil, fmt.Errorf("agent guard runtime settings save failed: %w", err)
	}
	affected, dispatchErr := s.dispatchRuntimeSettings(ctx, &settings)
	if dispatchErr != nil {
		settings.DispatchStatus = "failed"
		settings.DispatchErrorCode = "agent_guard_runtime_settings_dispatch_failed"
		_ = s.store.Upsert(&settings)
		s.logger.Warn("agent_guard_runtime_settings_dispatch_failed",
			zap.String("host_id", hostID.String()), zap.String("operator", operator),
			zap.String("error_code", settings.DispatchErrorCode), zap.Error(dispatchErr))
		return &settings, dispatchErr
	}
	if affected == 0 {
		settings.DispatchStatus = "pending_reconnect"
	} else {
		settings.DispatchStatus = "dispatched"
		for index := range settings.Injections {
			if settings.Injections[index].Enabled {
				settings.Injections[index].Status = "dispatched"
			}
		}
	}
	settings.DispatchErrorCode = ""
	if err := s.store.Upsert(&settings); err != nil {
		return nil, fmt.Errorf("agent guard runtime settings status save failed: %w", err)
	}
	s.logger.Info("agent_guard_runtime_settings_dispatched",
		zap.String("host_id", hostID.String()), zap.String("operator", operator),
		zap.Int32("affected_agents", affected), zap.String("dispatch_status", settings.DispatchStatus),
		zap.Bool("tool_adapter_enabled", settings.ToolAdapterEnabled),
		zap.Bool("session_hook_enabled", settings.SessionHookEnabled))
	return &settings, nil
}

func (s *AgentGuardRuntimeSettingsService) dispatchRuntimeSettings(
	ctx context.Context,
	settings *model.AgentGuardRuntimeSettings,
) (int32, error) {
	payload, err := marshalRuntimeSettingsPayload(settings)
	if err != nil {
		return 0, fmt.Errorf("agent guard runtime settings encode failed: %w", err)
	}
	return s.dispatcher.SyncAgentConfig(ctx, settings.HostID, []*pb.AgentConfig{{
		ConfigType: AgentGuardRuntimeSettingsConfigType,
		ConfigJson: string(payload),
	}})
}

func marshalRuntimeSettingsPayload(settings *model.AgentGuardRuntimeSettings) ([]byte, error) {
	if settings == nil {
		return nil, ErrAgentGuardRuntimeSettingsInvalid
	}
	return json.Marshal(struct {
		Schema                string `json:"schema"`
		Version               int64  `json:"version"`
		HostID                string `json:"host_id"`
		ToolAdapterEnabled    bool   `json:"tool_adapter_enabled"`
		SessionHookEnabled    bool   `json:"session_hook_enabled"`
		BehaviorPolicyEnabled bool   `json:"behavior_policy_enabled"`
		EscapePolicyEnabled   bool   `json:"escape_policy_enabled"`
		Injections            []struct {
			AgentType       string `json:"agent_type"`
			Enabled         bool   `json:"enabled"`
			BehaviorEnabled bool   `json:"behavior_enabled"`
			EscapeEnabled   bool   `json:"escape_enabled"`
		} `json:"injections"`
	}{
		Schema: settings.Schema, Version: settings.Version, HostID: settings.HostID,
		ToolAdapterEnabled:    settings.ToolAdapterEnabled,
		SessionHookEnabled:    settings.SessionHookEnabled,
		BehaviorPolicyEnabled: settings.BehaviorPolicyEnabled,
		EscapePolicyEnabled:   settings.EscapePolicyEnabled,
		Injections:            runtimeInjectionPayload(settings.Injections),
	})
}

func runtimeInjectionPayload(injections []model.AgentGuardHookInjection) []struct {
	AgentType       string `json:"agent_type"`
	Enabled         bool   `json:"enabled"`
	BehaviorEnabled bool   `json:"behavior_enabled"`
	EscapeEnabled   bool   `json:"escape_enabled"`
} {
	payload := make([]struct {
		AgentType       string `json:"agent_type"`
		Enabled         bool   `json:"enabled"`
		BehaviorEnabled bool   `json:"behavior_enabled"`
		EscapeEnabled   bool   `json:"escape_enabled"`
	}, 0, len(injections))
	for _, injection := range injections {
		payload = append(payload, struct {
			AgentType       string `json:"agent_type"`
			Enabled         bool   `json:"enabled"`
			BehaviorEnabled bool   `json:"behavior_enabled"`
			EscapeEnabled   bool   `json:"escape_enabled"`
		}{AgentType: injection.AgentType, Enabled: injection.Enabled, BehaviorEnabled: injection.BehaviorEnabled, EscapeEnabled: injection.EscapeEnabled})
	}
	return payload
}

func normalizeRuntimeInjections(requested, defaults []model.AgentGuardHookInjection) []model.AgentGuardHookInjection {
	allowed := make(map[string]bool, len(model.AgentGuardHookAgentTypes))
	for _, agentType := range model.AgentGuardHookAgentTypes {
		allowed[agentType] = true
	}
	byType := make(map[string]model.AgentGuardHookInjection, len(requested))
	for _, injection := range requested {
		agentType := strings.TrimSpace(injection.AgentType)
		if !allowed[agentType] {
			continue
		}
		behaviorEnabled := injection.BehaviorEnabled
		escapeEnabled := injection.EscapeEnabled
		if injection.Enabled && !behaviorEnabled && !escapeEnabled {
			behaviorEnabled = true
		}
		byType[agentType] = model.AgentGuardHookInjection{AgentType: agentType, Enabled: injection.Enabled || behaviorEnabled || escapeEnabled, BehaviorEnabled: behaviorEnabled, EscapeEnabled: escapeEnabled}
	}
	result := make([]model.AgentGuardHookInjection, 0, len(model.AgentGuardHookAgentTypes))
	for _, fallback := range defaults {
		injection, ok := byType[fallback.AgentType]
		if !ok {
			injection = fallback
		}
		if injection.Enabled {
			injection.Status = "pending"
		} else {
			injection.Status = "disabled"
		}
		injection.ErrorCode = ""
		result = append(result, injection)
	}
	return result
}
