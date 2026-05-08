package repository

import (
	"api-server/internal/model"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type CommandAuditRuleRepo struct {
	db *gorm.DB
}

func NewCommandAuditRuleRepo(db *gorm.DB) *CommandAuditRuleRepo {
	return &CommandAuditRuleRepo{db: db}
}

func (r *CommandAuditRuleRepo) List(category, severity, matchType, status, search string, page, pageSize int) ([]model.CommandAuditRule, int64, error) {
	var rules []model.CommandAuditRule
	var total int64

	query := r.db.Model(&model.CommandAuditRule{})
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if severity != "" {
		query = query.Where("severity = ?", severity)
	}
	if matchType != "" {
		query = query.Where("match_type = ?", matchType)
	}
	if status == "enabled" {
		query = query.Where("is_enabled = true")
	} else if status == "disabled" {
		query = query.Where("is_enabled = false")
	}
	if search != "" {
		query = query.Where("name ILIKE ? OR pattern ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		logger.Error("failed to count command audit rules", zap.Error(err))
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("is_preset DESC, created_at DESC").Offset(offset).Limit(pageSize).Find(&rules).Error; err != nil {
		logger.Error("failed to list command audit rules", zap.Error(err))
		return nil, 0, err
	}

	return rules, total, nil
}

func (r *CommandAuditRuleRepo) FindByID(id uuid.UUID) (*model.CommandAuditRule, error) {
	var rule model.CommandAuditRule
	if err := r.db.First(&rule, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *CommandAuditRuleRepo) Create(rule *model.CommandAuditRule) error {
	if err := r.db.Create(rule).Error; err != nil {
		logger.Error("failed to create command audit rule", zap.Error(err), zap.String("name", rule.Name))
		return err
	}
	return nil
}

func (r *CommandAuditRuleRepo) Update(rule *model.CommandAuditRule) error {
	if err := r.db.Save(rule).Error; err != nil {
		logger.Error("failed to update command audit rule", zap.Error(err), zap.String("id", rule.ID.String()))
		return err
	}
	return nil
}

func (r *CommandAuditRuleRepo) Delete(id uuid.UUID) error {
	result := r.db.Delete(&model.CommandAuditRule{}, "id = ? AND is_preset = false", id)
	if result.Error != nil {
		logger.Error("failed to delete command audit rule", zap.Error(result.Error), zap.String("id", id.String()))
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *CommandAuditRuleRepo) Toggle(id uuid.UUID) (*model.CommandAuditRule, error) {
	var rule model.CommandAuditRule
	if err := r.db.First(&rule, "id = ?", id).Error; err != nil {
		return nil, err
	}
	rule.IsEnabled = !rule.IsEnabled
	if err := r.db.Save(&rule).Error; err != nil {
		logger.Error("failed to toggle command audit rule", zap.Error(err), zap.String("id", id.String()))
		return nil, err
	}
	return &rule, nil
}

func (r *CommandAuditRuleRepo) FindAllEnabled() ([]model.CommandAuditRule, error) {
	var rules []model.CommandAuditRule
	if err := r.db.Where("is_enabled = true").Find(&rules).Error; err != nil {
		logger.Error("failed to find enabled rules", zap.Error(err))
		return nil, err
	}
	return rules, nil
}

func (r *CommandAuditRuleRepo) CreateInBatches(rules []model.CommandAuditRule, batchSize int) error {
	if len(rules) == 0 {
		return nil
	}
	if err := r.db.CreateInBatches(rules, batchSize).Error; err != nil {
		logger.Error("failed to batch create rules", zap.Error(err), zap.Int("count", len(rules)))
		return err
	}
	return nil
}
