package repository

import (
	"encoding/json"

	"server/internal/model"
	"server/pkg/logger"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type SystemConfigRepo struct {
	db *gorm.DB
}

func NewSystemConfigRepo(db *gorm.DB) *SystemConfigRepo {
	return &SystemConfigRepo{db: db}
}

func (r *SystemConfigRepo) GetByKey(key string) (*model.SystemConfig, error) {
	var config model.SystemConfig
	if err := r.db.Where("config_key = ?", key).First(&config).Error; err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *SystemConfigRepo) GetCommandAuditSettings() (*model.CommandAuditSettings, error) {
	config, err := r.GetByKey("command_audit.settings")
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			settings := model.DefaultCommandAuditSettings()
			return &settings, nil
		}
		logger.Error("failed to get command audit settings", zap.Error(err))
		return nil, err
	}

	var settings model.CommandAuditSettings
	if err := json.Unmarshal(config.ConfigValue, &settings); err != nil {
		logger.Error("failed to unmarshal audit settings", zap.Error(err))
		defaultSettings := model.DefaultCommandAuditSettings()
		return &defaultSettings, nil
	}
	return &settings, nil
}
