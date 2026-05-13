package repository

import (
	"context"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AgentExecutionRepository struct {
	db *gorm.DB
}

func NewAgentExecutionRepository(db *gorm.DB) *AgentExecutionRepository {
	return &AgentExecutionRepository{db: db}
}

// ==================== Create Methods ====================

func (r *AgentExecutionRepository) CreateExecution(exec *model.AgentExecution) error {
	return r.db.Create(exec).Error
}

func (r *AgentExecutionRepository) CreateStepExecution(step *model.AgentStepExecution) error {
	return r.db.Create(step).Error
}

func (r *AgentExecutionRepository) CreateReflection(refl *model.AgentReflection) error {
	return r.db.Create(refl).Error
}

func (r *AgentExecutionRepository) CreateAudit(audit *model.AgentAudit) error {
	return r.db.Create(audit).Error
}

func (r *AgentExecutionRepository) CreateCorrection(corr *model.AgentCorrection) error {
	return r.db.Create(corr).Error
}

func (r *AgentExecutionRepository) CreateToolCall(tc *model.AgentToolCallRecord) error {
	return r.db.Create(tc).Error
}

func (r *AgentExecutionRepository) CreateModelError(err *model.AgentModelError) error {
	return r.db.Create(err).Error
}

// ==================== Query Methods ====================

func (r *AgentExecutionRepository) FindByTaskID(taskID string) (*model.AgentExecution, error) {
	var exec model.AgentExecution
	if err := r.db.Where("task_id = ?", taskID).First(&exec).Error; err != nil {
		return nil, err
	}
	return &exec, nil
}

func (r *AgentExecutionRepository) FindBySessionID(sessionID string) (*model.AgentExecution, error) {
	var exec model.AgentExecution
	if err := r.db.Where("session_id = ?", sessionID).Order("created_at DESC").First(&exec).Error; err != nil {
		return nil, err
	}
	return &exec, nil
}

func (r *AgentExecutionRepository) FindStepsByExecutionID(execID uuid.UUID) ([]*model.AgentStepExecution, error) {
	var steps []*model.AgentStepExecution
	if err := r.db.Where("execution_id = ?", execID).Order("created_at ASC").Find(&steps).Error; err != nil {
		return nil, err
	}
	return steps, nil
}

func (r *AgentExecutionRepository) FindReflectionsByExecutionID(execID uuid.UUID) ([]*model.AgentReflection, error) {
	var reflections []*model.AgentReflection
	if err := r.db.Where("execution_id = ?", execID).Order("created_at ASC").Find(&reflections).Error; err != nil {
		return nil, err
	}
	return reflections, nil
}

func (r *AgentExecutionRepository) FindAuditsByExecutionID(execID uuid.UUID) ([]*model.AgentAudit, error) {
	var audits []*model.AgentAudit
	if err := r.db.Where("execution_id = ?", execID).Order("created_at ASC").Find(&audits).Error; err != nil {
		return nil, err
	}
	return audits, nil
}

func (r *AgentExecutionRepository) FindCorrectionsByExecutionID(execID uuid.UUID) ([]*model.AgentCorrection, error) {
	var corrections []*model.AgentCorrection
	if err := r.db.Where("execution_id = ?", execID).Order("created_at ASC").Find(&corrections).Error; err != nil {
		return nil, err
	}
	return corrections, nil
}

func (r *AgentExecutionRepository) FindToolCallsByExecutionID(execID uuid.UUID) ([]*model.AgentToolCallRecord, error) {
	var toolCalls []*model.AgentToolCallRecord
	if err := r.db.Where("execution_id = ?", execID).Order("created_at ASC").Find(&toolCalls).Error; err != nil {
		return nil, err
	}
	return toolCalls, nil
}

func (r *AgentExecutionRepository) FindModelErrorsByExecutionID(execID uuid.UUID) ([]*model.AgentModelError, error) {
	var modelErrors []*model.AgentModelError
	if err := r.db.Where("execution_id = ?", execID).Order("created_at ASC").Find(&modelErrors).Error; err != nil {
		return nil, err
	}
	return modelErrors, nil
}

// ==================== RAG Query Methods ====================

// FindFailedReflections gets recent failed reflections for experience learning.
// Conditions: recoverable=true, within last 7 days, ordered by created_at DESC.
func (r *AgentExecutionRepository) FindFailedReflections(ctx context.Context, limit int) ([]*model.AgentReflection, error) {
	var reflections []*model.AgentReflection
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	if err := r.db.WithContext(ctx).
		Where("recoverable = ? AND created_at >= ?", true, sevenDaysAgo).
		Order("created_at DESC").
		Limit(limit).
		Find(&reflections).Error; err != nil {
		return nil, err
	}
	return reflections, nil
}

// FindSuccessfulSummaries gets recent successful analysis summaries for similar case retrieval.
// Conditions: status=completed, ordered by created_at DESC.
func (r *AgentExecutionRepository) FindSuccessfulSummaries(ctx context.Context, limit int) ([]*model.AgentExecution, error) {
	var executions []*model.AgentExecution
	if err := r.db.WithContext(ctx).
		Where("status = ?", "completed").
		Order("created_at DESC").
		Limit(limit).
		Find(&executions).Error; err != nil {
		return nil, err
	}
	return executions, nil
}
