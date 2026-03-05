package repository

import (
	"baseline-system/internal/model"
	"baseline-system/pkg/logger"

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
