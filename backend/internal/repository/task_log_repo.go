package repository

import (
	"strings"
	"time"

	"aegis-system/internal/model"
	"aegis-system/pkg/logger"

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

func (r *TaskLogRepository) UpdateForRedispatch(id uuid.UUID, scriptContent string, scriptVersion int) error {
	now := time.Now()
	result := r.db.Model(&model.TaskLog{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"script_content": scriptContent,
			"script_version": scriptVersion,
			"status":         "PENDING",
			"stdout":         nil,
			"stderr":         nil,
			"exit_code":      nil,
			"started_at":     now,
			"finished_at":    nil,
		})

	if result.Error != nil {
		logger.Error("failed to update task for redispatch",
			zap.Error(result.Error),
			zap.String("id", id.String()),
		)
		return result.Error
	}

	logger.Info("task updated for redispatch",
		zap.String("id", id.String()),
		zap.Int("script_version", scriptVersion),
	)
	return nil
}

func (r *TaskLogRepository) UpdateStatus(id uuid.UUID, status string) error {
	result := r.db.Model(&model.TaskLog{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"started_at": time.Now(),
		})

	if result.Error != nil {
		logger.Error("failed to update task status",
			zap.Error(result.Error),
			zap.String("id", id.String()),
		)
		return result.Error
	}
	return nil
}

type TaskGroupSummary struct {
	TaskGroupID  uuid.UUID  `gorm:"column:task_group_id" json:"task_group_id"`
	TaskCount    int        `gorm:"column:task_count" json:"task_count"`
	TaskType     string     `gorm:"column:task_type" json:"task_type"`
	HasCheck     int        `gorm:"column:has_check" json:"has_check"`
	HasFix       int        `gorm:"column:has_fix" json:"has_fix"`
	Status       string     `gorm:"column:status" json:"status"`
	SuccessCount int        `gorm:"column:success_count" json:"success_count"`
	FailedCount  int        `gorm:"column:failed_count" json:"failed_count"`
	PendingCount int        `gorm:"column:pending_count" json:"pending_count"`
	RunningCount int        `gorm:"column:running_count" json:"running_count"`
	TimeoutCount int        `gorm:"column:timeout_count" json:"timeout_count"`
	CreatedAt    time.Time  `gorm:"column:created_at" json:"created_at"`
	FinishedAt   *time.Time `gorm:"column:finished_at" json:"finished_at"`
}

type ListTaskGroupsParams struct {
	Page      int
	PageSize  int
	Status    string
	TaskType  string
	StartTime *time.Time
	EndTime   *time.Time
	Search    string
}

func (r *TaskLogRepository) ListTaskGroups(params ListTaskGroupsParams) ([]TaskGroupSummary, error) {
	offset := (params.Page - 1) * params.PageSize
	if offset < 0 {
		offset = 0
	}

	query := r.db.Table("task_logs").
		Select(`
			task_group_id,
			COUNT(*) as task_count,
			MAX(task_type) as task_type,
			COALESCE(SUM(CASE WHEN task_type = 'check' OR task_type = 'CHECK' THEN 1 ELSE 0 END), 0) as has_check,
			COALESCE(SUM(CASE WHEN task_type = 'fix' OR task_type = 'FIX' THEN 1 ELSE 0 END), 0) as has_fix,
			CASE 
				WHEN SUM(CASE WHEN status = 'SUCCESS' THEN 1 ELSE 0 END) = COUNT(*) THEN 'success'
				WHEN SUM(CASE WHEN status = 'FAILED' THEN 1 ELSE 0 END) + SUM(CASE WHEN status = 'TIMEOUT' THEN 1 ELSE 0 END) = COUNT(*) THEN 'failed'
				WHEN SUM(CASE WHEN status = 'RUNNING' THEN 1 ELSE 0 END) > 0 THEN 'running'
				WHEN SUM(CASE WHEN status = 'PENDING' THEN 1 ELSE 0 END) = COUNT(*) THEN 'pending'
				ELSE 'partial'
			END as status,
			SUM(CASE WHEN status = 'SUCCESS' THEN 1 ELSE 0 END) as success_count,
			SUM(CASE WHEN status = 'FAILED' THEN 1 ELSE 0 END) as failed_count,
			SUM(CASE WHEN status = 'PENDING' THEN 1 ELSE 0 END) as pending_count,
			SUM(CASE WHEN status = 'RUNNING' THEN 1 ELSE 0 END) as running_count,
			SUM(CASE WHEN status = 'TIMEOUT' THEN 1 ELSE 0 END) as timeout_count,
			MIN(created_at) as created_at,
			MAX(finished_at) as finished_at
		`).
		Group("task_group_id")

	if params.Status != "" {
		query = query.Having(`
			CASE 
				WHEN SUM(CASE WHEN status = 'SUCCESS' THEN 1 ELSE 0 END) = COUNT(*) THEN 'success'
				WHEN SUM(CASE WHEN status = 'FAILED' THEN 1 ELSE 0 END) + SUM(CASE WHEN status = 'TIMEOUT' THEN 1 ELSE 0 END) = COUNT(*) THEN 'failed'
				WHEN SUM(CASE WHEN status = 'RUNNING' THEN 1 ELSE 0 END) > 0 THEN 'running'
				WHEN SUM(CASE WHEN status = 'PENDING' THEN 1 ELSE 0 END) = COUNT(*) THEN 'pending'
				ELSE 'partial'
			END = ?
		`, params.Status)
	}

	if params.TaskType != "" {
		taskTypes := strings.Split(params.TaskType, ",")
		for i, t := range taskTypes {
			taskTypes[i] = strings.ToUpper(strings.TrimSpace(t))
		}
		query = query.Where("UPPER(task_type) IN ?", taskTypes)
	}

	if params.StartTime != nil {
		query = query.Where("created_at >= ?", params.StartTime)
	}

	if params.EndTime != nil {
		query = query.Where("created_at <= ?", params.EndTime)
	}

	if params.Search != "" {
		search := "%" + params.Search + "%"
		query = query.Where("task_group_id IN (SELECT DISTINCT task_group_id FROM task_logs tl JOIN aegis_rules ar ON tl.rule_id = ar.id WHERE ar.title ILIKE ?)", search)
	}

	var summaries []TaskGroupSummary
	result := query.Order("created_at DESC").Limit(params.PageSize).Offset(offset).Scan(&summaries)
	if result.Error != nil {
		logger.Error("failed to list task groups",
			zap.Error(result.Error),
			zap.Int("page", params.Page),
			zap.Int("page_size", params.PageSize),
		)
		return nil, result.Error
	}

	return summaries, nil
}

func (r *TaskLogRepository) CountTaskGroups(params ListTaskGroupsParams) (int64, error) {
	subQuery := r.db.Table("task_logs").
		Select(`task_group_id,
			CASE 
				WHEN SUM(CASE WHEN status = 'SUCCESS' THEN 1 ELSE 0 END) = COUNT(*) THEN 'success'
				WHEN SUM(CASE WHEN status = 'FAILED' THEN 1 ELSE 0 END) + SUM(CASE WHEN status = 'TIMEOUT' THEN 1 ELSE 0 END) = COUNT(*) THEN 'failed'
				WHEN SUM(CASE WHEN status = 'RUNNING' THEN 1 ELSE 0 END) > 0 THEN 'running'
				WHEN SUM(CASE WHEN status = 'PENDING' THEN 1 ELSE 0 END) = COUNT(*) THEN 'pending'
				ELSE 'partial'
			END as status
		`).
		Group("task_group_id")

	if params.TaskType != "" {
		taskTypes := strings.Split(params.TaskType, ",")
		for i, t := range taskTypes {
			taskTypes[i] = strings.ToUpper(strings.TrimSpace(t))
		}
		subQuery = subQuery.Where("UPPER(task_type) IN ?", taskTypes)
	}

	if params.StartTime != nil {
		subQuery = subQuery.Where("created_at >= ?", params.StartTime)
	}

	if params.EndTime != nil {
		subQuery = subQuery.Where("created_at <= ?", params.EndTime)
	}

	if params.Search != "" {
		search := "%" + params.Search + "%"
		subQuery = subQuery.Where("task_group_id IN (SELECT DISTINCT task_group_id FROM task_logs tl JOIN aegis_rules ar ON tl.rule_id = ar.id WHERE ar.title ILIKE ?)", search)
	}

	var count int64
	result := r.db.Table("(?) as sub", subQuery).Count(&count)
	if result.Error != nil {
		logger.Error("failed to count task groups", zap.Error(result.Error))
		return 0, result.Error
	}

	return count, nil
}

func (r *TaskLogRepository) FindRunningTasks() ([]model.TaskLog, error) {
	var tasks []model.TaskLog
	result := r.db.Where("status IN ?", []string{"RUNNING", "PENDING"}).Find(&tasks)
	if result.Error != nil {
		logger.Error("failed to find running tasks", zap.Error(result.Error))
		return nil, result.Error
	}
	return tasks, nil
}

func (r *TaskLogRepository) Delete(id uuid.UUID) error {
	result := r.db.Delete(&model.TaskLog{}, "id = ?", id)
	if result.Error != nil {
		logger.Error("failed to delete task", zap.Error(result.Error), zap.String("id", id.String()))
		return result.Error
	}
	logger.Info("task deleted", zap.String("id", id.String()))
	return nil
}

func (r *TaskLogRepository) DeleteByGroupID(groupID uuid.UUID) (int64, error) {
	result := r.db.Where("task_group_id = ?", groupID).Delete(&model.TaskLog{})
	if result.Error != nil {
		logger.Error("failed to delete task group", zap.Error(result.Error), zap.String("group_id", groupID.String()))
		return 0, result.Error
	}
	logger.Info("task group deleted", zap.String("group_id", groupID.String()), zap.Int64("count", result.RowsAffected))
	return result.RowsAffected, nil
}

func (r *TaskLogRepository) FindTimedOutTasks(timeout time.Duration) ([]model.TaskLog, error) {
	var tasks []model.TaskLog
	cutoff := time.Now().Add(-timeout)
	// 检查 started_at 或 created_at（如果 started_at 为 NULL）
	result := r.db.Where("status IN ? AND COALESCE(started_at, created_at) < ?", []string{"RUNNING", "PENDING"}, cutoff).
		Find(&tasks)
	if result.Error != nil {
		logger.Error("failed to find timed out tasks", zap.Error(result.Error))
		return nil, result.Error
	}
	return tasks, nil
}

func (r *TaskLogRepository) MarkAsTimedOut(taskIDs []uuid.UUID) (int64, error) {
	if len(taskIDs) == 0 {
		return 0, nil
	}
	now := time.Now()
	result := r.db.Model(&model.TaskLog{}).
		Where("id IN ?", taskIDs).
		Updates(map[string]interface{}{
			"status":      "TIMEOUT",
			"finished_at": now,
		})
	if result.Error != nil {
		logger.Error("failed to mark tasks as timed out", zap.Error(result.Error))
		return 0, result.Error
	}
	logger.Info("tasks marked as timed out", zap.Int64("count", result.RowsAffected))
	return result.RowsAffected, nil
}

func (r *TaskLogRepository) BatchDeleteByGroupIDs(groupIDs []uuid.UUID) (int64, int64, error) {
	if len(groupIDs) == 0 {
		return 0, 0, nil
	}

	var totalTasksInGroups int64
	r.db.Model(&model.TaskLog{}).Where("task_group_id IN ?", groupIDs).Count(&totalTasksInGroups)

	result := r.db.Where("task_group_id IN ? AND status IN ?", groupIDs, []string{"SUCCESS", "FAILED", "TIMEOUT"}).
		Delete(&model.TaskLog{})

	if result.Error != nil {
		logger.Error("failed to batch delete task groups",
			zap.Error(result.Error),
			zap.Int("requested_count", len(groupIDs)),
		)
		return 0, 0, result.Error
	}

	deletedCount := result.RowsAffected
	skippedCount := totalTasksInGroups - deletedCount

	logger.Info("batch delete task groups completed",
		zap.Int64("deleted", deletedCount),
		zap.Int64("skipped", skippedCount),
	)

	return deletedCount, skippedCount, nil
}
