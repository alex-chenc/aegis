package repository

import (
	"context"
	"time"

	"api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AssistantToolPolicyRepository 工具策略仓库接口
type AssistantToolPolicyRepository interface {
	Upsert(ctx context.Context, policy *model.AssistantToolPolicy) error
	BatchUpsert(ctx context.Context, policies []model.AssistantToolPolicy) error
	FindByToolName(ctx context.Context, toolName string) (*model.AssistantToolPolicy, error)
	List(ctx context.Context, query ToolPolicyQuery) ([]model.AssistantToolPolicy, int64, error)
	UpdateWhitelist(ctx context.Context, toolName string, whitelisted bool, operator string) error
	BatchUpdateWhitelist(ctx context.Context, items []WhitelistUpdateItem, operator string) error
	ResetDefaultWhitelist(ctx context.Context, operator string) error
}

// ToolPolicyQuery 工具策略查询参数
type ToolPolicyQuery struct {
	Domain      string `json:"domain"`
	RiskLevel   string `json:"risk_level"`
	Whitelisted *bool  `json:"whitelisted"`
	Keyword     string `json:"keyword"`
	Page        int    `json:"page"`
	PageSize    int    `json:"page_size"`
}

// WhitelistUpdateItem 白名单更新项
type WhitelistUpdateItem struct {
	ToolName    string `json:"tool_name"`
	Whitelisted bool   `json:"whitelisted"`
}

type assistantToolPolicyRepo struct {
	db *gorm.DB
}

// NewAssistantToolPolicyRepository 创建工具策略仓库
func NewAssistantToolPolicyRepository(db *gorm.DB) AssistantToolPolicyRepository {
	return &assistantToolPolicyRepo{db: db}
}

func (r *assistantToolPolicyRepo) Upsert(ctx context.Context, policy *model.AssistantToolPolicy) error {
	if policy.ID == uuid.Nil {
		policy.ID = uuid.New()
	}

	return r.db.WithContext(ctx).
		Where("tool_name = ?", policy.ToolName).
		Assign(map[string]interface{}{
			"domain":              policy.Domain,
			"operation":           policy.Operation,
			"risk_level":          policy.RiskLevel,
			"description":         policy.Description,
			"args_summary":        policy.ArgsSummary,
			"default_whitelisted": policy.DefaultWhitelisted,
			"updated_at":          time.Now(),
		}).
		FirstOrCreate(policy).Error
}

func (r *assistantToolPolicyRepo) BatchUpsert(ctx context.Context, policies []model.AssistantToolPolicy) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, policy := range policies {
			if err := r.Upsert(ctx, &policy); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *assistantToolPolicyRepo) FindByToolName(ctx context.Context, toolName string) (*model.AssistantToolPolicy, error) {
	var policy model.AssistantToolPolicy
	err := r.db.WithContext(ctx).
		Where("tool_name = ?", toolName).
		First(&policy).Error
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

func (r *assistantToolPolicyRepo) List(ctx context.Context, query ToolPolicyQuery) ([]model.AssistantToolPolicy, int64, error) {
	var policies []model.AssistantToolPolicy
	var total int64

	tx := r.db.WithContext(ctx).Model(&model.AssistantToolPolicy{})

	if query.Domain != "" {
		tx = tx.Where("domain = ?", query.Domain)
	}
	if query.RiskLevel != "" {
		tx = tx.Where("risk_level = ?", query.RiskLevel)
	}
	if query.Whitelisted != nil {
		tx = tx.Where("whitelisted = ?", *query.Whitelisted)
	}
	if query.Keyword != "" {
		tx = tx.Where("tool_name ILIKE ? OR description ILIKE ?", "%"+query.Keyword+"%", "%"+query.Keyword+"%")
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
		Order("domain, tool_name").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&policies).Error

	return policies, total, err
}

func (r *assistantToolPolicyRepo) UpdateWhitelist(ctx context.Context, toolName string, whitelisted bool, operator string) error {
	return r.db.WithContext(ctx).
		Model(&model.AssistantToolPolicy{}).
		Where("tool_name = ?", toolName).
		Updates(map[string]interface{}{
			"whitelisted": whitelisted,
			"updated_by":  operator,
			"updated_at":  time.Now(),
		}).Error
}

func (r *assistantToolPolicyRepo) BatchUpdateWhitelist(ctx context.Context, items []WhitelistUpdateItem, operator string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			err := tx.Model(&model.AssistantToolPolicy{}).
				Where("tool_name = ?", item.ToolName).
				Updates(map[string]interface{}{
					"whitelisted": item.Whitelisted,
					"updated_by":  operator,
					"updated_at":  time.Now(),
				}).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *assistantToolPolicyRepo) ResetDefaultWhitelist(ctx context.Context, operator string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Reset whitelisted to default_whitelisted for all tools
		err := tx.Model(&model.AssistantToolPolicy{}).
			Where("1 = 1").
			Updates(map[string]interface{}{
				"whitelisted": gorm.Expr("default_whitelisted"),
				"updated_by":  operator,
				"updated_at":  time.Now(),
			}).Error
		return err
	})
}
