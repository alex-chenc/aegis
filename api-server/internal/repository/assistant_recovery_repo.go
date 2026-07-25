package repository

import (
	"context"
	"errors"
	"time"

	"api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrRecoveryAlreadyDecided = errors.New("assistant recovery request is no longer pending")

type AssistantRecoveryRepository interface {
	Create(ctx context.Context, request *model.AssistantRecoveryRequest) error
	FindByRecoveryID(ctx context.Context, recoveryID string) (*model.AssistantRecoveryRequest, error)
	FindPendingByToolCall(ctx context.Context, toolCallID, code string) (*model.AssistantRecoveryRequest, error)
	FindActiveByRunStepCode(ctx context.Context, runID, stepID, toolName, code string) (*model.AssistantRecoveryRequest, error)
	FindPendingByRun(ctx context.Context, runID string) (*model.AssistantRecoveryRequest, error)
	ListBySession(ctx context.Context, sessionID string, page, pageSize int) ([]model.AssistantRecoveryRequest, int64, error)
	BeginDecision(ctx context.Context, recoveryID, actionID, operator string, input datatypes.JSON) error
	CompleteDecision(ctx context.Context, recoveryID, status string, result datatypes.JSON, resumeRunID string) error
}

type assistantRecoveryRepo struct {
	db *gorm.DB
}

func NewAssistantRecoveryRepository(db *gorm.DB) AssistantRecoveryRepository {
	return &assistantRecoveryRepo{db: db}
}

func (r *assistantRecoveryRepo) Create(ctx context.Context, request *model.AssistantRecoveryRequest) error {
	if request.ID == uuid.Nil {
		request.ID = uuid.New()
	}
	if request.RecoveryID == "" {
		request.RecoveryID = "recovery_" + uuid.NewString()[:8]
	}
	return r.db.WithContext(ctx).Create(request).Error
}

func (r *assistantRecoveryRepo) FindByRecoveryID(ctx context.Context, recoveryID string) (*model.AssistantRecoveryRequest, error) {
	var request model.AssistantRecoveryRequest
	err := r.db.WithContext(ctx).Where("recovery_id = ?", recoveryID).First(&request).Error
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func (r *assistantRecoveryRepo) FindPendingByToolCall(ctx context.Context, toolCallID, code string) (*model.AssistantRecoveryRequest, error) {
	var request model.AssistantRecoveryRequest
	err := r.db.WithContext(ctx).
		Where("tool_call_id = ? AND code = ? AND status IN ?", toolCallID, code, []string{
			model.RecoveryStatusPending,
			model.RecoveryStatusExecuting,
			model.RecoveryStatusPaused,
		}).
		Order("created_at DESC").
		First(&request).Error
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func (r *assistantRecoveryRepo) FindActiveByRunStepCode(
	ctx context.Context,
	runID, stepID, toolName, code string,
) (*model.AssistantRecoveryRequest, error) {
	normalizedStepID := stepID
	if normalizedStepID == "" {
		normalizedStepID = toolName
	}
	var request model.AssistantRecoveryRequest
	err := r.db.WithContext(ctx).
		Where(`
			run_id = ?
			AND COALESCE(NULLIF(step_id, ''), tool_name) = ?
			AND tool_name = ?
			AND code = ?
			AND status IN ?`,
			runID,
			normalizedStepID,
			toolName,
			code,
			[]string{
				model.RecoveryStatusPending,
				model.RecoveryStatusExecuting,
				model.RecoveryStatusPaused,
			},
		).
		Order("created_at ASC").
		First(&request).Error
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func (r *assistantRecoveryRepo) FindPendingByRun(ctx context.Context, runID string) (*model.AssistantRecoveryRequest, error) {
	var request model.AssistantRecoveryRequest
	err := r.db.WithContext(ctx).
		Where("run_id = ? AND status = ?", runID, model.RecoveryStatusPending).
		Order("created_at ASC").
		First(&request).Error
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func (r *assistantRecoveryRepo) ListBySession(ctx context.Context, sessionID string, page, pageSize int) ([]model.AssistantRecoveryRequest, int64, error) {
	var requests []model.AssistantRecoveryRequest
	var total int64
	query := r.db.WithContext(ctx).Model(&model.AssistantRecoveryRequest{}).Where("session_id = ?", sessionID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&requests).Error
	return requests, total, err
}

func (r *assistantRecoveryRepo) BeginDecision(ctx context.Context, recoveryID, actionID, operator string, input datatypes.JSON) error {
	now := time.Now()
	result := r.db.WithContext(ctx).
		Model(&model.AssistantRecoveryRequest{}).
		Where("recovery_id = ? AND status IN ?", recoveryID, []string{
			model.RecoveryStatusPending,
			model.RecoveryStatusPaused,
		}).
		Updates(map[string]interface{}{
			"status":             model.RecoveryStatusExecuting,
			"selected_action_id": actionID,
			"decision_input":     input,
			"decided_by":         operator,
			"decided_at":         now,
			"updated_at":         now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrRecoveryAlreadyDecided
	}
	return nil
}

func (r *assistantRecoveryRepo) CompleteDecision(ctx context.Context, recoveryID, status string, result datatypes.JSON, resumeRunID string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"status":            status,
		"resolution_result": result,
		"updated_at":        now,
	}
	if resumeRunID != "" {
		updates["resume_run_id"] = resumeRunID
	}
	if status == model.RecoveryStatusResolved {
		updates["resolved_at"] = now
	}
	return r.db.WithContext(ctx).
		Model(&model.AssistantRecoveryRequest{}).
		Where("recovery_id = ?", recoveryID).
		Updates(updates).Error
}
