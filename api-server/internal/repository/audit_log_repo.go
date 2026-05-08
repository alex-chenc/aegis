package repository

import (
	"fmt"

	"api-server/internal/model"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AuditLogRepo struct {
	db *gorm.DB
}

func NewAuditLogRepo(db *gorm.DB) *AuditLogRepo {
	return &AuditLogRepo{db: db}
}

func (r *AuditLogRepo) Create(log *model.ScriptAuditLog) error {
	if err := r.db.Create(log).Error; err != nil {
		logger.Error("failed to create audit log", zap.Error(err))
		return err
	}
	return nil
}

func (r *AuditLogRepo) FindByID(id uuid.UUID) (*model.ScriptAuditLog, error) {
	var log model.ScriptAuditLog
	if err := r.db.First(&log, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *AuditLogRepo) FindLatestByTaskID(taskID string) (*model.ScriptAuditLog, error) {
	var log model.ScriptAuditLog
	if err := r.db.Where("task_id = ?", taskID).Order("created_at DESC").First(&log).Error; err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *AuditLogRepo) List(scriptType, auditSource, passedStr string, page, pageSize int) ([]model.ScriptAuditLog, int64, error) {
	var logs []model.ScriptAuditLog
	var total int64

	query := r.db.Model(&model.ScriptAuditLog{})
	if scriptType != "" {
		query = query.Where("script_type = ?", scriptType)
	}
	if auditSource != "" {
		query = query.Where("audit_source = ?", auditSource)
	}
	if passedStr == "true" {
		query = query.Where("passed = true")
	} else if passedStr == "false" {
		query = query.Where("passed = false")
	}

	if err := query.Count(&total).Error; err != nil {
		logger.Error("failed to count audit logs", zap.Error(err))
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		logger.Error("failed to list audit logs", zap.Error(err))
		return nil, 0, err
	}

	return logs, total, nil
}

type AuditStats struct {
	TotalAudits      int64            `json:"total"`
	Passed           int64            `json:"passed"`
	Failed           int64            `json:"failed"`
	PassRate         float64          `json:"pass_rate"`
	BySource         map[string]int64 `json:"by_source"`
	ByType           map[string]int64 `json:"by_type"`
	RetryDistribution map[string]int64 `json:"retry_distribution"`
}

func (r *AuditLogRepo) GetStats() (*AuditStats, error) {
	stats := &AuditStats{
		BySource:          make(map[string]int64),
		ByType:            make(map[string]int64),
		RetryDistribution: make(map[string]int64),
	}

	if err := r.db.Model(&model.ScriptAuditLog{}).Count(&stats.TotalAudits).Error; err != nil {
		return nil, fmt.Errorf("failed to count total audits: %w", err)
	}
	if err := r.db.Model(&model.ScriptAuditLog{}).Where("passed = true").Count(&stats.Passed).Error; err != nil {
		return nil, fmt.Errorf("failed to count passed audits: %w", err)
	}
	stats.Failed = stats.TotalAudits - stats.Passed
	if stats.TotalAudits > 0 {
		stats.PassRate = float64(stats.Passed) / float64(stats.TotalAudits)
	}

	var sourceRows []struct {
		AuditSource string
		Count       int64
	}
	if err := r.db.Model(&model.ScriptAuditLog{}).Select("audit_source, count(*) as count").Group("audit_source").Scan(&sourceRows).Error; err != nil {
		return nil, fmt.Errorf("failed to group by source: %w", err)
	}
	for _, row := range sourceRows {
		stats.BySource[row.AuditSource] = row.Count
	}

	var typeRows []struct {
		ScriptType string
		Count      int64
	}
	if err := r.db.Model(&model.ScriptAuditLog{}).Select("script_type, count(*) as count").Group("script_type").Scan(&typeRows).Error; err != nil {
		return nil, fmt.Errorf("failed to group by type: %w", err)
	}
	for _, row := range typeRows {
		stats.ByType[row.ScriptType] = row.Count
	}

	// Retry distribution: count by attempt number for failed tasks
	var retryRows []struct {
		Attempt int
		Passed  bool
		Count   int64
	}
	if err := r.db.Model(&model.ScriptAuditLog{}).Select("attempt, passed, count(*) as count").Group("attempt, passed").Scan(&retryRows).Error; err != nil {
		return nil, fmt.Errorf("failed to group by attempt: %w", err)
	}
	for _, row := range retryRows {
		key := fmt.Sprintf("%d", row.Attempt)
		if !row.Passed {
			stats.RetryDistribution[key] += row.Count
		}
	}
	// Count final failures using JOIN instead of correlated subquery for better performance
	var finalFailCount int64
	if err := r.db.Raw(`SELECT COUNT(*) FROM script_audit_log s
		INNER JOIN (SELECT task_id, MAX(attempt) as max_attempt FROM script_audit_log GROUP BY task_id) la
		ON s.task_id = la.task_id AND s.attempt = la.max_attempt
		WHERE s.passed = false`).Scan(&finalFailCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count final failures: %w", err)
	}
	stats.RetryDistribution["failed"] = finalFailCount

	return stats, nil
}
