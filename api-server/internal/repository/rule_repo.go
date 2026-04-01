package repository

import (
	"api-server/internal/model"
	"api-server/pkg/logger"

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

func (r *RuleRepository) BatchCreate(rules []*model.AegisRule) error {
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

func (r *RuleRepository) FindByTemplateID(templateID uuid.UUID) ([]model.AegisRule, error) {
	var rules []model.AegisRule
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

func (r *RuleRepository) FindByID(id uuid.UUID) (*model.AegisRule, error) {
	var rule model.AegisRule
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

	result := r.db.Model(&model.AegisRule{}).Where("id = ?", ruleID).Updates(updates)
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
	result := r.db.Model(&model.AegisRule{}).
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

func (r *RuleRepository) UpdateScriptStatusByType(ruleID uuid.UUID, scriptType, status string) error {
	updates := map[string]interface{}{
		"script_status": status,
	}

	if scriptType == "CHECK" {
		updates["check_script_status"] = status
	} else {
		updates["fix_script_status"] = status
	}

	result := r.db.Model(&model.AegisRule{}).
		Where("id = ?", ruleID).
		Updates(updates)

	if result.Error != nil {
		logger.Error("failed to update script status by type",
			zap.Error(result.Error),
			zap.String("rule_id", ruleID.String()),
			zap.String("script_type", scriptType),
			zap.String("status", status),
		)
		return result.Error
	}

	logger.Debug("script status by type updated",
		zap.String("rule_id", ruleID.String()),
		zap.String("script_type", scriptType),
		zap.String("status", status),
	)
	return nil
}

func (r *RuleRepository) UpdateScriptStatusWithError(ruleID uuid.UUID, scriptType, status, errorMsg string) error {
	updates := map[string]interface{}{
		"script_status": status,
	}

	if scriptType == "CHECK" {
		updates["check_script_status"] = status
		if errorMsg != "" {
			updates["check_script_error"] = errorMsg
		}
	} else {
		updates["fix_script_status"] = status
		if errorMsg != "" {
			updates["fix_script_error"] = errorMsg
		}
	}

	result := r.db.Model(&model.AegisRule{}).
		Where("id = ?", ruleID).
		Updates(updates)

	if result.Error != nil {
		logger.Error("failed to update script status with error",
			zap.Error(result.Error),
			zap.String("rule_id", ruleID.String()),
			zap.String("script_type", scriptType),
			zap.String("status", status),
		)
		return result.Error
	}

	logger.Debug("script status with error updated",
		zap.String("rule_id", ruleID.String()),
		zap.String("script_type", scriptType),
		zap.String("status", status),
	)
	return nil
}

// HasTasks 检查规则是否有关联任务
func (r *RuleRepository) HasTasks(ruleID uuid.UUID) (bool, int64, error) {
	var count int64
	result := r.db.Table("task_logs").Where("rule_id = ?", ruleID).Count(&count)
	if result.Error != nil {
		logger.Error("failed to count tasks for rule",
			zap.Error(result.Error),
			zap.String("rule_id", ruleID.String()),
		)
		return false, 0, result.Error
	}
	return count > 0, count, nil
}

// Delete 删除规则（仅当无关联任务时可删除）
func (r *RuleRepository) Delete(ruleID uuid.UUID) error {
	// 先删除关联的 script_versions
	if err := r.db.Where("rule_id = ?", ruleID).Delete(&model.ScriptVersion{}).Error; err != nil {
		logger.Error("failed to delete script versions",
			zap.Error(err),
			zap.String("rule_id", ruleID.String()),
		)
		return err
	}

	// 删除关联的 self_healing_logs（通过 task_logs 关联的）
	// 先获取关联的 task_logs IDs
	var taskLogIDs []uuid.UUID
	if err := r.db.Model(&model.TaskLog{}).Where("rule_id = ?", ruleID).Pluck("id", &taskLogIDs).Error; err != nil {
		logger.Error("failed to get task log ids",
			zap.Error(err),
			zap.String("rule_id", ruleID.String()),
		)
		return err
	}

	// 删除关联的 self_healing_logs
	if len(taskLogIDs) > 0 {
		if err := r.db.Where("original_task_id IN ?", taskLogIDs).Delete(&model.HealingLog{}).Error; err != nil {
			logger.Error("failed to delete healing logs",
				zap.Error(err),
				zap.String("rule_id", ruleID.String()),
			)
			return err
		}
	}

	// 删除关联的 task_logs
	if err := r.db.Where("rule_id = ?", ruleID).Delete(&model.TaskLog{}).Error; err != nil {
		logger.Error("failed to delete task logs",
			zap.Error(err),
			zap.String("rule_id", ruleID.String()),
		)
		return err
	}

	// 再删除规则
	result := r.db.Delete(&model.AegisRule{}, "id = ?", ruleID)
	if result.Error != nil {
		logger.Error("failed to delete rule",
			zap.Error(result.Error),
			zap.String("rule_id", ruleID.String()),
		)
		return result.Error
	}

	logger.Info("rule deleted", zap.String("rule_id", ruleID.String()))
	return nil
}

// UpdateScriptContent 直接更新脚本内容（用于用户手动编辑保存）
func (r *RuleRepository) UpdateScriptContent(ruleID uuid.UUID, scriptType, scriptContent string) error {
	updates := map[string]interface{}{}

	if scriptType == "CHECK" {
		updates["generated_check_script"] = scriptContent
	} else {
		updates["generated_fix_script"] = scriptContent
	}

	result := r.db.Model(&model.AegisRule{}).Where("id = ?", ruleID).Updates(updates)
	if result.Error != nil {
		logger.Error("failed to update rule script content",
			zap.Error(result.Error),
			zap.String("rule_id", ruleID.String()),
			zap.String("script_type", scriptType),
		)
		return result.Error
	}

	logger.Info("rule script content updated",
		zap.String("rule_id", ruleID.String()),
		zap.String("script_type", scriptType),
	)
	return nil
}

// FindByTemplateIDAndTitles 根据模板ID和规则标题列表查询已存在的规则
// 返回已存在规则的标题集合，用于去重
func (r *RuleRepository) FindByTemplateIDAndTitles(templateID uuid.UUID, titles []string) (map[string]bool, error) {
	if len(titles) == 0 {
		return make(map[string]bool), nil
	}

	var rules []model.AegisRule
	result := r.db.Where("template_id = ? AND title IN ?", templateID, titles).
		Select("title").
		Find(&rules)

	if result.Error != nil {
		logger.Error("failed to find existing rules",
			zap.Error(result.Error),
			zap.String("template_id", templateID.String()))
		return nil, result.Error
	}

	existingTitles := make(map[string]bool, len(rules))
	for _, rule := range rules {
		existingTitles[rule.Title] = true
	}

	logger.Debug("found existing rules",
		zap.Int("count", len(existingTitles)),
		zap.String("template_id", templateID.String()))

	return existingTitles, nil
}
