package repository

import (
	"context"

	"api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ExternalMCPQueryLogRepository 外接 MCP 查询日志仓库接口
type ExternalMCPQueryLogRepository interface {
	Create(ctx context.Context, log *model.ExternalMCPQueryLog) error
	ListBySession(ctx context.Context, sessionID string, page, pageSize int) ([]model.ExternalMCPQueryLog, int64, error)
	ListBySource(ctx context.Context, sourceID string, page, pageSize int) ([]model.ExternalMCPQueryLog, int64, error)
}

type externalMCPQueryLogRepo struct {
	db *gorm.DB
}

// NewExternalMCPQueryLogRepository 创建外接 MCP 查询日志仓库
func NewExternalMCPQueryLogRepository(db *gorm.DB) ExternalMCPQueryLogRepository {
	return &externalMCPQueryLogRepo{db: db}
}

func (r *externalMCPQueryLogRepo) Create(ctx context.Context, log *model.ExternalMCPQueryLog) error {
	if log.ID == uuid.Nil {
		log.ID = uuid.New()
	}
	if log.QueryID == "" {
		log.QueryID = "mcpq_" + uuid.New().String()[:8]
	}
	return r.db.WithContext(ctx).Create(log).Error
}

func (r *externalMCPQueryLogRepo) ListBySession(ctx context.Context, sessionID string, page, pageSize int) ([]model.ExternalMCPQueryLog, int64, error) {
	var logs []model.ExternalMCPQueryLog
	var total int64

	tx := r.db.WithContext(ctx).
		Model(&model.ExternalMCPQueryLog{}).
		Where("session_id = ?", sessionID)

	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	err := tx.
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&logs).Error

	return logs, total, err
}

func (r *externalMCPQueryLogRepo) ListBySource(ctx context.Context, sourceID string, page, pageSize int) ([]model.ExternalMCPQueryLog, int64, error) {
	var logs []model.ExternalMCPQueryLog
	var total int64

	tx := r.db.WithContext(ctx).
		Model(&model.ExternalMCPQueryLog{}).
		Where("source_id = ?", sourceID)

	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	err := tx.
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&logs).Error

	return logs, total, err
}
