package repository

import (
	"encoding/json"

	"api-server/internal/model"
	"api-server/pkg/logger"

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

func (r *SystemConfigRepo) Upsert(key string, value interface{}, description, category string) error {
	valueBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}

	var existing model.SystemConfig
	err = r.db.Where("config_key = ?", key).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		config := model.SystemConfig{
			ConfigKey:   key,
			ConfigValue: valueBytes,
			Description: description,
			Category:    category,
		}
		return r.db.Create(&config).Error
	}
	if err != nil {
		return err
	}

	existing.ConfigValue = valueBytes
	existing.Description = description
	return r.db.Save(&existing).Error
}

func (r *SystemConfigRepo) GetCommandAuditSettings() (*model.CommandAuditSettings, error) {
	config, err := r.GetByKey("command_audit.settings")
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			settings := model.DefaultCommandAuditSettings()
			defaultBytes, _ := json.Marshal(settings)
			defaultConfig := &model.SystemConfig{
				ConfigKey:   "command_audit.settings",
				ConfigValue: defaultBytes,
				Description: "命令审计全局配置",
				Category:    "command_audit",
			}
			if createErr := r.db.Create(defaultConfig).Error; createErr != nil {
				logger.Error("failed to create default audit settings", zap.Error(createErr))
				return &settings, nil
			}
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

func (r *SystemConfigRepo) UpdateCommandAuditSettings(settings *model.CommandAuditSettings) error {
	return r.Upsert("command_audit.settings", settings, "命令审计全局配置", "command_audit")
}
