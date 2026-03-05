package repository

import (
	"time"

	"baseline-system/internal/model"
	"baseline-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type HealingLogRepository struct {
	db *gorm.DB
}

func NewHealingLogRepository(db *gorm.DB) *HealingLogRepository {
	return &HealingLogRepository{db: db}
}

func (r *HealingLogRepository) Create(log *model.HealingLog) error {
	result := r.db.Create(log)
	if result.Error != nil {
		logger.Error("failed to create healing log",
			zap.Error(result.Error),
			zap.String("original_task_id", log.OriginalTaskID.String()),
		)
		return result.Error
	}

	logger.Info("healing log created",
		zap.String("id", log.ID.String()),
		zap.String("original_task_id", log.OriginalTaskID.String()),
		zap.String("script_type", log.ScriptType),
	)
	return nil
}

func (r *HealingLogRepository) Update(log *model.HealingLog) error {
	result := r.db.Model(&model.HealingLog{}).
		Where("id = ?", log.ID).
		Updates(map[string]interface{}{
			"total_attempts":          log.TotalAttempts,
			"status":                  log.Status,
			"final_script_version_id": log.FinalScriptVersionID,
			"attempts_detail":         log.AttemptsDetail,
			"finished_at":             log.FinishedAt,
		})

	if result.Error != nil {
		logger.Error("failed to update healing log",
			zap.Error(result.Error),
			zap.String("id", log.ID.String()),
		)
		return result.Error
	}

	logger.Info("healing log updated",
		zap.String("id", log.ID.String()),
		zap.String("status", log.Status),
		zap.Int("total_attempts", log.TotalAttempts),
	)
	return nil
}

func (r *HealingLogRepository) FindByID(id uuid.UUID) (*model.HealingLog, error) {
	var log model.HealingLog
	result := r.db.First(&log, "id = ?", id)
	if result.Error != nil {
		logger.Error("failed to find healing log by id", zap.Error(result.Error), zap.String("id", id.String()))
		return nil, result.Error
	}
	return &log, nil
}

func (r *HealingLogRepository) FindByOriginalTaskID(taskID uuid.UUID) (*model.HealingLog, error) {
	var log model.HealingLog
	result := r.db.Where("original_task_id = ?", taskID).First(&log)
	if result.Error != nil {
		logger.Error("failed to find healing log by original_task_id",
			zap.Error(result.Error),
			zap.String("task_id", taskID.String()),
		)
		return nil, result.Error
	}
	return &log, nil
}

func (r *HealingLogRepository) IncrementAttempts(id uuid.UUID) error {
	result := r.db.Model(&model.HealingLog{}).
		Where("id = ?", id).
		Update("total_attempts", gorm.Expr("total_attempts + 1"))

	if result.Error != nil {
		logger.Error("failed to increment attempts", zap.Error(result.Error), zap.String("id", id.String()))
		return result.Error
	}

	logger.Debug("healing attempts incremented", zap.String("id", id.String()))
	return nil
}

func (r *HealingLogRepository) MarkCompleted(id uuid.UUID, scriptVersionID uuid.UUID) error {
	now := time.Now()
	result := r.db.Model(&model.HealingLog{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":                  "healed",
			"final_script_version_id": scriptVersionID,
			"finished_at":             now,
		})

	if result.Error != nil {
		logger.Error("failed to mark healing completed", zap.Error(result.Error), zap.String("id", id.String()))
		return result.Error
	}

	logger.Info("healing marked as completed",
		zap.String("id", id.String()),
		zap.String("script_version_id", scriptVersionID.String()),
	)
	return nil
}

func (r *HealingLogRepository) MarkFailed(id uuid.UUID) error {
	now := time.Now()
	result := r.db.Model(&model.HealingLog{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":      "failed",
			"finished_at": now,
		})

	if result.Error != nil {
		logger.Error("failed to mark healing failed", zap.Error(result.Error), zap.String("id", id.String()))
		return result.Error
	}

	logger.Warn("healing marked as failed", zap.String("id", id.String()))
	return nil
}
