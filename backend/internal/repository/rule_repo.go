package repository

import (
	"baseline-system/internal/model"
	"baseline-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type RuleRepository struct {
	db *gorm.DB
}

func NewRuleRepository(db *gorm.DB) *RuleRepository {
	return &RuleRepository{db: db}
}

func (r *RuleRepository) BatchCreate(rules []*model.BaselineRule) error {
	if len(rules) == 0 {
		return nil
	}

	result := r.db.CreateInBatches(rules, 100)
	if result.Error != nil {
		logger.Error("failed to batch create rules", zap.Error(result.Error), zap.Int("count", len(rules)))
		return result.Error
	}

	logger.Info("rules batch created successfully",
		zap.Int("count", len(rules)),
		zap.String("template_id", rules[0].TemplateID.String()),
	)
	return nil
}

func (r *RuleRepository) FindByTemplateID(templateID uuid.UUID) ([]model.BaselineRule, error) {
	var rules []model.BaselineRule
	result := r.db.Where("template_id = ?", templateID).Order("created_at").Find(&rules)
	if result.Error != nil {
		logger.Error("failed to find rules by template_id",
			zap.Error(result.Error),
			zap.String("template_id", templateID.String()),
		)
		return nil, result.Error
	}

	logger.Debug("rules found", zap.Int("count", len(rules)), zap.String("template_id", templateID.String()))
	return rules, nil
}

func (r *RuleRepository) FindByID(id uuid.UUID) (*model.BaselineRule, error) {
	var rule model.BaselineRule
	result := r.db.First(&rule, "id = ?", id)
	if result.Error != nil {
		logger.Error("failed to find rule by id", zap.Error(result.Error), zap.String("id", id.String()))
		return nil, result.Error
	}
	return &rule, nil
}

func (r *RuleRepository) UpdateScript(ruleID uuid.UUID, scriptType, scriptContent string, version int) error {
	updates := map[string]interface{}{
		"script_status": "ready",
	}

	if scriptType == "CHECK" {
		updates["generated_check_script"] = scriptContent
		updates["check_script_version"] = version
	} else {
		updates["generated_fix_script"] = scriptContent
		updates["fix_script_version"] = version
	}

	result := r.db.Model(&model.BaselineRule{}).Where("id = ?", ruleID).Updates(updates)
	if result.Error != nil {
		logger.Error("failed to update rule script",
			zap.Error(result.Error),
			zap.String("rule_id", ruleID.String()),
			zap.String("script_type", scriptType),
		)
		return result.Error
	}

	logger.Info("rule script updated",
		zap.String("rule_id", ruleID.String()),
		zap.String("script_type", scriptType),
		zap.Int("version", version),
	)
	return nil
}

func (r *RuleRepository) UpdateScriptStatus(ruleID uuid.UUID, status string) error {
	result := r.db.Model(&model.BaselineRule{}).
		Where("id = ?", ruleID).
		Update("script_status", status)

	if result.Error != nil {
		logger.Error("failed to update script status",
			zap.Error(result.Error),
			zap.String("rule_id", ruleID.String()),
			zap.String("status", status),
		)
		return result.Error
	}

	logger.Debug("script status updated", zap.String("rule_id", ruleID.String()), zap.String("status", status))
	return nil
}
