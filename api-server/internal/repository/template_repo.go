package repository

import (
	"api-server/internal/model"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type TemplateRepository struct {
	db *gorm.DB
}

func NewTemplateRepository(db *gorm.DB) *TemplateRepository {
	return &TemplateRepository{db: db}
}

func (r *TemplateRepository) Create(template *model.Template) error {
	result := r.db.Create(template)
	if result.Error != nil {
		logger.Error("failed to create template", zap.Error(result.Error), zap.String("name", template.Name))
		return result.Error
	}

	logger.Info("template created successfully",
		zap.String("id", template.ID.String()),
		zap.String("name", template.Name),
		zap.String("file_type", template.FileType),
	)
	return nil
}

func (r *TemplateRepository) FindAll(page, pageSize int) ([]model.Template, error) {
	var templates []model.Template
	offset := (page - 1) * pageSize

	result := r.db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&templates)
	if result.Error != nil {
		logger.Error("failed to find templates", zap.Error(result.Error))
		return nil, result.Error
	}

	logger.Debug("templates found", zap.Int("count", len(templates)), zap.Int("page", page))
	return templates, nil
}

func (r *TemplateRepository) FindByID(id uuid.UUID) (*model.Template, error) {
	var template model.Template
	result := r.db.First(&template, "id = ?", id)
	if result.Error != nil {
		logger.Error("failed to find template by id", zap.Error(result.Error), zap.String("id", id.String()))
		return nil, result.Error
	}
	return &template, nil
}

func (r *TemplateRepository) UpdateStatus(id uuid.UUID, status string, errorMessage *string, ruleCount int) error {
	result := r.db.Model(&model.Template{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        status,
			"error_message": errorMessage,
			"rule_count":    ruleCount,
		})

	if result.Error != nil {
		logger.Error("failed to update template status",
			zap.Error(result.Error),
			zap.String("id", id.String()),
			zap.String("status", status),
		)
		return result.Error
	}

	logger.Info("template status updated",
		zap.String("id", id.String()),
		zap.String("status", status),
		zap.Int("rule_count", ruleCount),
	)
	return nil
}

func (r *TemplateRepository) Delete(id uuid.UUID) error {
	result := r.db.Delete(&model.Template{}, "id = ?", id)
	if result.Error != nil {
		logger.Error("failed to delete template", zap.Error(result.Error), zap.String("id", id.String()))
		return result.Error
	}

	logger.Info("template deleted", zap.String("id", id.String()))
	return nil
}

func (r *TemplateRepository) FindByName(name string) (*model.Template, error) {
	var template model.Template
	result := r.db.Where("name = ?", name).Order("created_at DESC").First(&template)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			logger.Info("no existing template found with name", zap.String("name", name))
			return nil, nil
		}
		logger.Error("failed to find template by name", zap.Error(result.Error), zap.String("name", name))
		return nil, result.Error
	}
	logger.Info("found existing template by name", zap.String("id", template.ID.String()), zap.String("name", name))
	return &template, nil
}

func (r *TemplateRepository) ResetStatus(id uuid.UUID) error {
	result := r.db.Model(&model.Template{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        "parsing",
			"error_message": nil,
			"rule_count":    0,
		})

	if result.Error != nil {
		logger.Error("failed to reset template status",
			zap.Error(result.Error),
			zap.String("id", id.String()),
		)
		return result.Error
	}

	logger.Info("template status reset", zap.String("id", id.String()))
	return nil
}

func (r *TemplateRepository) ExistsByMD5(md5 string) (bool, *model.Template, error) {
	var template model.Template
	result := r.db.Where("file_md5 = ?", md5).First(&template)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return false, nil, nil
		}
		logger.Error("failed to check md5", zap.Error(result.Error), zap.String("md5", md5))
		return false, nil, result.Error
	}
	return true, &template, nil
}

func (r *TemplateRepository) CountByName(name string) (int64, error) {
	var count int64
	result := r.db.Model(&model.Template{}).Where("name = ?", name).Count(&count)
	if result.Error != nil {
		logger.Error("failed to count templates by name", zap.Error(result.Error), zap.String("name", name))
		return 0, result.Error
	}
	return count, nil
}

func (r *TemplateRepository) DeleteWithRules(id uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var ruleIDs []uuid.UUID
		if err := tx.Model(&model.AegisRule{}).Where("template_id = ?", id).Pluck("id", &ruleIDs).Error; err != nil {
			return err
		}

		if len(ruleIDs) > 0 {
			var taskLogIDs []uuid.UUID
			if err := tx.Model(&model.TaskLog{}).Where("rule_id IN ?", ruleIDs).Pluck("id", &taskLogIDs).Error; err != nil {
				return err
			}

			if len(taskLogIDs) > 0 {
				// 先清除 task_logs 的 healing_id 外键引用，避免删除 self_healing_logs 时违反约束
				if err := tx.Model(&model.TaskLog{}).Where("id IN ?", taskLogIDs).Update("healing_id", nil).Error; err != nil {
					return err
				}
				if err := tx.Where("original_task_id IN ?", taskLogIDs).Delete(&model.HealingLog{}).Error; err != nil {
					return err
				}
			}

			// 清除剩余 healing_id 引用（rule_id 维度）
			if err := tx.Model(&model.TaskLog{}).Where("rule_id IN ? AND healing_id IS NOT NULL", ruleIDs).Update("healing_id", nil).Error; err != nil {
				return err
			}

			if err := tx.Where("rule_id IN ?", ruleIDs).Delete(&model.TaskLog{}).Error; err != nil {
				return err
			}

			if err := tx.Where("rule_id IN ?", ruleIDs).Delete(&model.HealingLog{}).Error; err != nil {
				return err
			}

			if err := tx.Where("rule_id IN ?", ruleIDs).Delete(&model.ScriptVersion{}).Error; err != nil {
				return err
			}
		}

		if err := tx.Where("template_id = ?", id).Delete(&model.AegisRule{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.Template{}, "id = ?", id).Error; err != nil {
			return err
		}

		logger.Info("template and related data deleted", zap.String("template_id", id.String()), zap.Int("rules", len(ruleIDs)))
		return nil
	})
}
