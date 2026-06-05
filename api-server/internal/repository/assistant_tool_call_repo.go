package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/alex-chenc/aegis/api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// AssistantToolCallRepository 工具调用仓库接口
type AssistantToolCallRepository interface {
	Create(ctx context.Context, call *model.AssistantToolCall) error
	FindByCallID(ctx context.Context, callID string) (*model.AssistantToolCall, error)
	ListBySession(ctx context.Context, sessionID string, page, pageSize int) ([]model.AssistantToolCall, int64, error)
	MarkSuccess(ctx context.Context, callID string, result interface{}, durationMs int64) error
	MarkFailed(ctx context.Context, callID string, errMsg string, durationMs int64) error
	MarkApprovalRequired(ctx context.Context, callID string, approvalID string) error
	MarkRejected(ctx context.Context, callID string, comment string) error
	UpdateStatus(ctx context.Context, callID string, status string) error
}

type assistantToolCallRepo struct {
	db *gorm.DB
}

// NewAssistantToolCallRepository 创建工具调用仓库
func NewAssistantToolCallRepository(db *gorm.DB) AssistantToolCallRepository {
	return &assistantToolCallRepo{db: db}
}

func (r *assistantToolCallRepo) Create(ctx context.Context, call *model.AssistantToolCall) error {
	if call.ID == uuid.Nil {
		call.ID = uuid.New()
	}
	if call.CallID == "" {
		call.CallID = "call_" + uuid.New().String()[:8]
	}
	return r.db.WithContext(ctx).Create(call).Error
}

func (r *assistantToolCallRepo) FindByCallID(ctx context.Context, callID string) (*model.AssistantToolCall, error) {
	var call model.AssistantToolCall
	err := r.db.WithContext(ctx).
		Where("call_id = ?", callID).
		First(&call).Error
	if err != nil {
		return nil, err
	}
	return &call, nil
}

func (r *assistantToolCallRepo) ListBySession(ctx context.Context, sessionID string, page, pageSize int) ([]model.AssistantToolCall, int64, error) {
	var calls []model.AssistantToolCall
	var total int64

	tx := r.db.WithContext(ctx).
		Model(&model.AssistantToolCall{}).
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
		Find(&calls).Error

	return calls, total, err
}

func (r *assistantToolCallRepo) MarkSuccess(ctx context.Context, callID string, result interface{}, durationMs int64) error {
	return r.db.WithContext(ctx).
		Model(&model.AssistantToolCall{}).
		Where("call_id = ?", callID).
		Updates(map[string]interface{}{
			"status":      model.ToolCallStatusSuccess,
			"result":      datatypes.JSON(mustMarshal(result)),
			"duration_ms": durationMs,
			"updated_at":  time.Now(),
		}).Error
}

func (r *assistantToolCallRepo) MarkFailed(ctx context.Context, callID string, errMsg string, durationMs int64) error {
	return r.db.WithContext(ctx).
		Model(&model.AssistantToolCall{}).
		Where("call_id = ?", callID).
		Updates(map[string]interface{}{
			"status":        model.ToolCallStatusFailed,
			"error_message": errMsg,
			"duration_ms":   durationMs,
			"updated_at":    time.Now(),
		}).Error
}

func (r *assistantToolCallRepo) MarkApprovalRequired(ctx context.Context, callID string, approvalID string) error {
	return r.db.WithContext(ctx).
		Model(&model.AssistantToolCall{}).
		Where("call_id = ?", callID).
		Updates(map[string]interface{}{
			"status":     model.ToolCallStatusApprovalRequired,
			"updated_at": time.Now(),
		}).Error
}

func (r *assistantToolCallRepo) MarkRejected(ctx context.Context, callID string, comment string) error {
	return r.db.WithContext(ctx).
		Model(&model.AssistantToolCall{}).
		Where("call_id = ?", callID).
		Updates(map[string]interface{}{
			"status":        model.ToolCallStatusRejected,
			"error_message": comment,
			"updated_at":    time.Now(),
		}).Error
}

func (r *assistantToolCallRepo) UpdateStatus(ctx context.Context, callID string, status string) error {
	return r.db.WithContext(ctx).
		Model(&model.AssistantToolCall{}).
		Where("call_id = ?", callID).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		}).Error
}

func mustMarshal(v interface{}) []byte {
	if v == nil {
		return []byte("{}")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}
