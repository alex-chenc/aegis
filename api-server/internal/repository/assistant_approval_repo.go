package repository

import (
	"context"
	"time"

	"github.com/alex-chenc/aegis/api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AssistantApprovalRepository 审批仓库接口
type AssistantApprovalRepository interface {
	Create(ctx context.Context, approval *model.AssistantApproval) error
	FindByApprovalID(ctx context.Context, approvalID string) (*model.AssistantApproval, error)
	ListBySession(ctx context.Context, sessionID string, page, pageSize int) ([]model.AssistantApproval, int64, error)
	ListPending(ctx context.Context, sessionID string) ([]model.AssistantApproval, error)
	MarkApproved(ctx context.Context, approvalID string, operator string, comment string) error
	MarkRejected(ctx context.Context, approvalID string, operator string, comment string) error
	MarkExecuted(ctx context.Context, approvalID string) error
	MarkFailed(ctx context.Context, approvalID string, errMsg string) error
	MarkExpired(ctx context.Context, approvalID string) error
}

type assistantApprovalRepo struct {
	db *gorm.DB
}

// NewAssistantApprovalRepository 创建审批仓库
func NewAssistantApprovalRepository(db *gorm.DB) AssistantApprovalRepository {
	return &assistantApprovalRepo{db: db}
}

func (r *assistantApprovalRepo) Create(ctx context.Context, approval *model.AssistantApproval) error {
	if approval.ID == uuid.Nil {
		approval.ID = uuid.New()
	}
	if approval.ApprovalID == "" {
		approval.ApprovalID = "appr_" + uuid.New().String()[:8]
	}
	return r.db.WithContext(ctx).Create(approval).Error
}

func (r *assistantApprovalRepo) FindByApprovalID(ctx context.Context, approvalID string) (*model.AssistantApproval, error) {
	var approval model.AssistantApproval
	err := r.db.WithContext(ctx).
		Where("approval_id = ?", approvalID).
		First(&approval).Error
	if err != nil {
		return nil, err
	}
	return &approval, nil
}

func (r *assistantApprovalRepo) ListBySession(ctx context.Context, sessionID string, page, pageSize int) ([]model.AssistantApproval, int64, error) {
	var approvals []model.AssistantApproval
	var total int64

	tx := r.db.WithContext(ctx).
		Model(&model.AssistantApproval{}).
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
		Find(&approvals).Error

	return approvals, total, err
}

func (r *assistantApprovalRepo) ListPending(ctx context.Context, sessionID string) ([]model.AssistantApproval, error) {
	var approvals []model.AssistantApproval
	err := r.db.WithContext(ctx).
		Where("session_id = ? AND status = ?", sessionID, model.ApprovalStatusPending).
		Order("created_at ASC").
		Find(&approvals).Error
	return approvals, err
}

func (r *assistantApprovalRepo) MarkApproved(ctx context.Context, approvalID string, operator string, comment string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&model.AssistantApproval{}).
		Where("approval_id = ? AND status = ?", approvalID, model.ApprovalStatusPending).
		Updates(map[string]interface{}{
			"status":         model.ApprovalStatusApproved,
			"reviewed_by":    operator,
			"review_comment": comment,
			"reviewed_at":    now,
		}).Error
}

func (r *assistantApprovalRepo) MarkRejected(ctx context.Context, approvalID string, operator string, comment string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&model.AssistantApproval{}).
		Where("approval_id = ? AND status = ?", approvalID, model.ApprovalStatusPending).
		Updates(map[string]interface{}{
			"status":         model.ApprovalStatusRejected,
			"reviewed_by":    operator,
			"review_comment": comment,
			"reviewed_at":    now,
		}).Error
}

func (r *assistantApprovalRepo) MarkExecuted(ctx context.Context, approvalID string) error {
	return r.db.WithContext(ctx).
		Model(&model.AssistantApproval{}).
		Where("approval_id = ?", approvalID).
		Update("status", model.ApprovalStatusExecuted).Error
}

func (r *assistantApprovalRepo) MarkFailed(ctx context.Context, approvalID string, errMsg string) error {
	return r.db.WithContext(ctx).
		Model(&model.AssistantApproval{}).
		Where("approval_id = ?", approvalID).
		Updates(map[string]interface{}{
			"status":         model.ApprovalStatusFailed,
			"review_comment": errMsg,
		}).Error
}

func (r *assistantApprovalRepo) MarkExpired(ctx context.Context, approvalID string) error {
	return r.db.WithContext(ctx).
		Model(&model.AssistantApproval{}).
		Where("approval_id = ? AND status = ?", approvalID, model.ApprovalStatusPending).
		Update("status", model.ApprovalStatusExpired).Error
}
