package repository

import (
	"encoding/json"
	"fmt"

	"api-server/internal/model"
	"gorm.io/gorm"
)

const agentGuardRuntimeSettingsKeyPrefix = "agent_guard.runtime."

type AgentGuardRuntimeSettingsRepository struct {
	system *SystemConfigRepo
}

func NewAgentGuardRuntimeSettingsRepository(system *SystemConfigRepo) *AgentGuardRuntimeSettingsRepository {
	return &AgentGuardRuntimeSettingsRepository{system: system}
}

func (r *AgentGuardRuntimeSettingsRepository) Get(hostID string) (*model.AgentGuardRuntimeSettings, error) {
	if r == nil || r.system == nil {
		return nil, fmt.Errorf("agent guard runtime settings repository unavailable")
	}
	config, err := r.system.GetByKey(agentGuardRuntimeSettingsKeyPrefix + hostID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			settings := model.DefaultAgentGuardRuntimeSettings(hostID)
			return &settings, nil
		}
		return nil, err
	}
	var settings model.AgentGuardRuntimeSettings
	if err := json.Unmarshal(config.ConfigValue, &settings); err != nil {
		return nil, fmt.Errorf("agent guard runtime settings decode failed: %w", err)
	}
	if settings.Schema == "" {
		settings.Schema = model.AgentGuardRuntimeSettingsSchema
	}
	settings.HostID = hostID
	return &settings, nil
}

func (r *AgentGuardRuntimeSettingsRepository) Upsert(settings *model.AgentGuardRuntimeSettings) error {
	if settings == nil || settings.HostID == "" {
		return fmt.Errorf("agent guard runtime settings invalid")
	}
	return r.system.Upsert(
		agentGuardRuntimeSettingsKeyPrefix+settings.HostID,
		settings,
		"Agent Guard 智能体运行时开关与 Hook 注入设置",
		"agent_guard",
	)
}
