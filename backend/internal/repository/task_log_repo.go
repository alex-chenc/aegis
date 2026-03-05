package repository

import (
	"time"

	"baseline-system/internal/model"
	"baseline-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type TaskLogRepository struct {
	db *gorm.DB
}

func NewTaskLogRepository(db *gorm.DB) *TaskLogRepository {
	return &TaskLogRepository{db: db}
}

func (r *TaskLogRepository) Create(log *model.TaskLog) error {
	result := r.db.Create(log)
	if result.Error != nil {
		logger.Error("failed to create task log",
			zap.Error(result.Error),
			zap.String("task_group_id", log.TaskGroupID.String()),
		)
		return result.Error
	}

	logger.Info("task log created",
		zap.String("id", log.ID.String()),
		zap.String("task_group_id", log.TaskGroupID.String()),
		zap.String("task_type", log.TaskType),
	)
	return nil
}

func (r *TaskLogRepository) UpdateResult(id uuid.UUID, stdout, stderr *string, exitCode *int, status string, finishedAt time.Time) error {
	result := r.db.Model(&model.TaskLog{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"stdout":      stdout,
			"stderr":      stderr,
			"exit_code":   exitCode,
			"status":      status,
			"finished_at": finishedAt,
		})

	if result.Error != nil {
		logger.Error("failed to update task result",
			zap.Error(result.Error),
			zap.String("id", id.String()),
		)
		return result.Error
	}

	logger.Info("task result updated",
		zap.String("id", id.String()),
		zap.String("status", status),
	)
	return nil
}

func (r *TaskLogRepository) FindByGroupID(groupID uuid.UUID) ([]model.TaskLog, error) {
	var logs []model.TaskLog
	result := r.db.Where("task_group_id = ?", groupID).Order("created_at").Find(&logs)
	if result.Error != nil {
		logger.Error("failed to find task logs by group_id",
			zap.Error(result.Error),
			zap.String("group_id", groupID.String()),
		)
		return nil, result.Error
	}

	logger.Debug("task logs found", zap.Int("count", len(logs)), zap.String("group_id", groupID.String()))
	return logs, nil
}

func (r *TaskLogRepository) FindByID(id uuid.UUID) (*model.TaskLog, error) {
	var log model.TaskLog
	result := r.db.First(&log, "id = ?", id)
	if result.Error != nil {
		logger.Error("failed to find task log by id", zap.Error(result.Error), zap.String("id", id.String()))
		return nil, result.Error
	}
	return &log, nil
}
