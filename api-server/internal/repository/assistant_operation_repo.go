package repository

import (
	"context"
	"encoding/json"
	"time"

	"api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// AssistantOperationRepository stores durable workflow state for high-level
// tools whose backend work spans multiple model tool calls.
type AssistantOperationRepository struct {
	db *gorm.DB
}

func NewAssistantOperationRepository(db *gorm.DB) *AssistantOperationRepository {
	return &AssistantOperationRepository{db: db}
}

func (r *AssistantOperationRepository) Create(ctx context.Context, operation *model.AssistantOperation) error {
	if operation.ID == uuid.Nil {
		operation.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(operation).Error
}

func (r *AssistantOperationRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.AssistantOperation, error) {
	var operation model.AssistantOperation
	if err := r.db.WithContext(ctx).First(&operation, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &operation, nil
}

func (r *AssistantOperationRepository) FindByIdempotencyKey(ctx context.Context, sessionID, workflowID, key string) (*model.AssistantOperation, bool, error) {
	var operation model.AssistantOperation
	err := r.db.WithContext(ctx).
		Where("session_id = ? AND workflow_id = ? AND idempotency_key = ?", sessionID, workflowID, key).
		Order("created_at DESC").
		First(&operation).Error
	if err == gorm.ErrRecordNotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &operation, true, nil
}

func (r *AssistantOperationRepository) ListNonTerminal(ctx context.Context, operationType string, limit int) ([]model.AssistantOperation, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var operations []model.AssistantOperation
	err := r.db.WithContext(ctx).
		Where("type = ? AND terminal = ?", operationType, false).
		Order("updated_at ASC").
		Limit(limit).
		Find(&operations).Error
	return operations, err
}

func (r *AssistantOperationRepository) Transition(ctx context.Context, id uuid.UUID, from []string, to string, result interface{}) (bool, error) {
	updates := map[string]interface{}{
		"status":        to,
		"current_stage": operationStage(result),
		"updated_at":    time.Now(),
	}
	if result != nil {
		updates["result"] = datatypes.JSON(marshalOperationJSON(result))
		counts, references, violations := operationLedgerProjection(result)
		updates["counts"] = datatypes.JSON(marshalOperationJSON(counts))
		updates["domain_references"] = datatypes.JSON(marshalOperationJSON(references))
		updates["violations"] = datatypes.JSON(marshalOperationJSON(violations))
	}
	tx := r.db.WithContext(ctx).Model(&model.AssistantOperation{}).
		Where("id = ? AND status IN ?", id, from).
		Updates(updates)
	return tx.RowsAffected == 1, tx.Error
}

func (r *AssistantOperationRepository) Update(ctx context.Context, id uuid.UUID, status string, result interface{}, errorCode, errorMessage string, terminal bool) error {
	updates := map[string]interface{}{
		"status":        status,
		"current_stage": operationStage(result),
		"terminal":      terminal,
		"result":        datatypes.JSON(marshalOperationJSON(result)),
		"error_code":    errorCode,
		"error_message": errorMessage,
		"updated_at":    time.Now(),
	}
	counts, references, violations := operationLedgerProjection(result)
	updates["counts"] = datatypes.JSON(marshalOperationJSON(counts))
	updates["domain_references"] = datatypes.JSON(marshalOperationJSON(references))
	updates["violations"] = datatypes.JSON(marshalOperationJSON(violations))
	if terminal {
		now := time.Now()
		updates["finished_at"] = &now
	}
	return r.db.WithContext(ctx).Model(&model.AssistantOperation{}).Where("id = ?", id).Updates(updates).Error
}

func operationStage(value interface{}) string {
	if object, ok := value.(map[string]interface{}); ok {
		if stage, ok := object["stage"].(string); ok {
			return stage
		}
	}
	return ""
}

func operationLedgerProjection(value interface{}) (map[string]interface{}, map[string]interface{}, []map[string]interface{}) {
	counts := make(map[string]interface{})
	references := make(map[string]interface{})
	violations := make([]map[string]interface{}, 0)
	object, ok := value.(map[string]interface{})
	if !ok {
		return counts, references, violations
	}
	if nested, exists := object["counts"]; exists {
		counts["task_status"] = nested
	}
	for _, key := range []string{"rule_count", "host_count", "expected_count", "created_count", "noncompliant_count"} {
		if item, exists := object[key]; exists {
			counts[key] = item
		}
	}
	for _, key := range []string{"task_group_id", "template_id"} {
		if item, exists := object[key]; exists {
			references[key] = item
		}
	}
	if code, exists := object["error_code"].(string); exists && code != "" {
		violations = append(violations, map[string]interface{}{
			"code":    code,
			"message": object["error_message"],
		})
	}
	return counts, references, violations
}

func marshalOperationJSON(value interface{}) []byte {
	if value == nil {
		return []byte("{}")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return encoded
}
