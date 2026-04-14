package repository

import (
	"api-server/internal/model"

	"gorm.io/gorm"
)

type AIRuleConfigRepository struct {
	db *gorm.DB
}

func NewAIRuleConfigRepository(db *gorm.DB) *AIRuleConfigRepository {
	return &AIRuleConfigRepository{db: db}
}

// GetConfig 获取默认配置（按名称获取）
func (r *AIRuleConfigRepository) GetConfig(name string) (*model.AIConfig, error) {
	var config model.AIConfig
	err := r.db.Where("name = ?", name).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// GetDefaultConfig 获取默认AI规则配置，如果不存在则创建
func (r *AIRuleConfigRepository) GetDefaultConfig() (*model.AIConfig, error) {
	config, err := r.GetConfig("default")
	if err == nil {
		return config, nil
	}

	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	// 创建默认配置
	defaultConfig := &model.AIConfig{
		Name:                      "default",
		Description:               "AI规则生成默认配置",
		Enabled:                   false,
		Mode:                      "suggest",
		Triggers:                  `["high_frequency", "new_mitre", "critical"]`,
		Thresholds:                `{"high_frequency_count":5,"high_frequency_hours":24}`,
		Conservatism:              0.5,
		RequireApproval:           true,
		AutoActivateAfterApproval: false,
		ActivationDelayHours:     24,
		NotifyOnGeneration:        true,
		NotifyOnApproval:         true,
		NotificationTargets:      `[]`,
	}

	if err := r.db.Create(defaultConfig).Error; err != nil {
		// 可能是并发创建导致的唯一冲突，再次尝试获取
		if err.Error() == "ERROR: duplicate key (SQLSTATE 23505)" || err.Error() == "pq: duplicate key" {
			return r.GetConfig("default")
		}
		return nil, err
	}

	return defaultConfig, nil
}

// UpdateConfig 更新配置
func (r *AIRuleConfigRepository) UpdateConfig(config *model.AIConfig) error {
	return r.db.Save(config).Error
}

// UpdateByName 根据名称更新配置
func (r *AIRuleConfigRepository) UpdateByName(name string, updates map[string]interface{}) error {
	return r.db.Model(&model.AIConfig{}).Where("name = ?", name).Updates(updates).Error
}

// IncrementGeneratedCount 增加生成规则计数
func (r *AIRuleConfigRepository) IncrementGeneratedCount(name string) error {
	return r.db.Model(&model.AIConfig{}).Where("name = ?", name).
		Update("rules_generated_count", gorm.Expr("rules_generated_count + 1")).Error
}

// IncrementApprovedCount 增加审核通过计数
func (r *AIRuleConfigRepository) IncrementApprovedCount(name string) error {
	return r.db.Model(&model.AIConfig{}).Where("name = ?", name).
		Update("rules_approved_count", gorm.Expr("rules_approved_count + 1")).Error
}

// List 获取所有配置
func (r *AIRuleConfigRepository) List() ([]model.AIConfig, error) {
	var configs []model.AIConfig
	err := r.db.Order("created_at DESC").Find(&configs).Error
	return configs, err
}
