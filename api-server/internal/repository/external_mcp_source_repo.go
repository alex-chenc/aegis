package repository

import (
	"context"
	"time"

	"github.com/alex-chenc/aegis/api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ExternalMCPSourceRepository 外接 MCP 数据源仓库接口
type ExternalMCPSourceRepository interface {
	Create(ctx context.Context, source *model.ExternalMCPSource) error
	FindBySourceID(ctx context.Context, sourceID string) (*model.ExternalMCPSource, error)
	List(ctx context.Context, query MCPSourceQuery) ([]model.ExternalMCPSource, int64, error)
	Update(ctx context.Context, source *model.ExternalMCPSource) error
	Delete(ctx context.Context, sourceID string) error
	UpdateTestStatus(ctx context.Context, sourceID string, status string, errMsg string) error
	UpdateSchemaCache(ctx context.Context, sourceID string, schema interface{}) error
}

// MCPSourceQuery MCP 数据源查询参数
type MCPSourceQuery struct {
	SourceType string `json:"source_type"`
	Enabled    *bool  `json:"enabled"`
	Keyword    string `json:"keyword"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
}

type externalMCPSourceRepo struct {
	db *gorm.DB
}

// NewExternalMCPSourceRepository 创建外接 MCP 数据源仓库
func NewExternalMCPSourceRepository(db *gorm.DB) ExternalMCPSourceRepository {
	return &externalMCPSourceRepo{db: db}
}

func (r *externalMCPSourceRepo) Create(ctx context.Context, source *model.ExternalMCPSource) error {
	if source.ID == uuid.Nil {
		source.ID = uuid.New()
	}
	if source.SourceID == "" {
		source.SourceID = "mcp_" + uuid.New().String()[:8]
	}
	return r.db.WithContext(ctx).Create(source).Error
}

func (r *externalMCPSourceRepo) FindBySourceID(ctx context.Context, sourceID string) (*model.ExternalMCPSource, error) {
	var source model.ExternalMCPSource
	err := r.db.WithContext(ctx).
		Where("source_id = ?", sourceID).
		First(&source).Error
	if err != nil {
		return nil, err
	}
	return &source, nil
}

func (r *externalMCPSourceRepo) List(ctx context.Context, query MCPSourceQuery) ([]model.ExternalMCPSource, int64, error) {
	var sources []model.ExternalMCPSource
	var total int64

	tx := r.db.WithContext(ctx).Model(&model.ExternalMCPSource{})

	if query.SourceType != "" {
		tx = tx.Where("source_type = ?", query.SourceType)
	}
	if query.Enabled != nil {
		tx = tx.Where("enabled = ?", *query.Enabled)
	}
	if query.Keyword != "" {
		tx = tx.Where("name ILIKE ? OR description ILIKE ?", "%"+query.Keyword+"%", "%"+query.Keyword+"%")
	}

	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	err := tx.
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&sources).Error

	return sources, total, err
}

func (r *externalMCPSourceRepo) Update(ctx context.Context, source *model.ExternalMCPSource) error {
	source.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(source).Error
}

func (r *externalMCPSourceRepo) Delete(ctx context.Context, sourceID string) error {
	return r.db.WithContext(ctx).
		Where("source_id = ?", sourceID).
		Delete(&model.ExternalMCPSource{}).Error
}

func (r *externalMCPSourceRepo) UpdateTestStatus(ctx context.Context, sourceID string, status string, errMsg string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&model.ExternalMCPSource{}).
		Where("source_id = ?", sourceID).
		Updates(map[string]interface{}{
			"last_test_status": status,
			"last_test_error":  errMsg,
			"last_test_at":     now,
			"updated_at":       now,
		}).Error
}

func (r *externalMCPSourceRepo) UpdateSchemaCache(ctx context.Context, sourceID string, schema interface{}) error {
	return r.db.WithContext(ctx).
		Model(&model.ExternalMCPSource{}).
		Where("source_id = ?", sourceID).
		Updates(map[string]interface{}{
			"schema_cache": schema,
			"updated_at":   time.Now(),
		}).Error
}
