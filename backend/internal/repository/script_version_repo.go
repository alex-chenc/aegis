package repository

import (
	"aegis-system/internal/model"
	"aegis-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ScriptVersionRepository struct {
	db *gorm.DB
}

func NewScriptVersionRepository(db *gorm.DB) *ScriptVersionRepository {
	return &ScriptVersionRepository{db: db}
}

func (r *ScriptVersionRepository) Create(version *model.ScriptVersion) error {
	result := r.db.Create(version)
	if result.Error != nil {
		logger.Error("failed to create script version",
			zap.Error(result.Error),
			zap.String("rule_id", version.RuleID.String()),
			zap.String("script_type", version.ScriptType),
		)
		return result.Error
	}

	logger.Info("script version created",
		zap.String("id", version.ID.String()),
		zap.String("rule_id", version.RuleID.String()),
		zap.String("script_type", version.ScriptType),
		zap.Int("version", version.Version),
	)
	return nil
}

func (r *ScriptVersionRepository) FindByRuleAndType(ruleID uuid.UUID, scriptType string) ([]model.ScriptVersion, error) {
	var versions []model.ScriptVersion
	result := r.db.Where("rule_id = ? AND script_type = ?", ruleID, scriptType).
		Order("version DESC").
		Find(&versions)

	if result.Error != nil {
		logger.Error("failed to find script versions",
			zap.Error(result.Error),
			zap.String("rule_id", ruleID.String()),
			zap.String("script_type", scriptType),
		)
		return nil, result.Error
	}

	logger.Debug("script versions found",
		zap.Int("count", len(versions)),
		zap.String("rule_id", ruleID.String()),
	)
	return versions, nil
}

func (r *ScriptVersionRepository) SetCurrentVersion(versionID uuid.UUID) error {
	var sv model.ScriptVersion
	if err := r.db.First(&sv, "id = ?", versionID).Error; err != nil {
		logger.Error("failed to find script version", zap.Error(err), zap.String("id", versionID.String()))
		return err
	}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ScriptVersion{}).
			Where("rule_id = ? AND script_type = ?", sv.RuleID, sv.ScriptType).
			Update("is_current", false).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.ScriptVersion{}).
			Where("id = ?", versionID).
			Update("is_current", true).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		logger.Error("failed to set current version", zap.Error(err), zap.String("id", versionID.String()))
		return err
	}

	logger.Info("current script version set",
		zap.String("id", versionID.String()),
		zap.String("rule_id", sv.RuleID.String()),
	)
	return nil
}
