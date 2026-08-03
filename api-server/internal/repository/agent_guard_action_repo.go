package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAgentGuardActionStateConflict = errors.New("agent guard action state conflict")

type AgentGuardActionRepository struct {
	db *gorm.DB
}

func NewAgentGuardActionRepository(db *gorm.DB) *AgentGuardActionRepository {
	return &AgentGuardActionRepository{db: db}
}

func (r *AgentGuardActionRepository) ResolveExecutionUnit(
	ctx context.Context,
	id uuid.UUID,
) (*model.AgentExecutionUnit, *model.AgentRuntimeInstance, *model.AgentGuardPolicyDelivery, error) {
	var unit model.AgentExecutionUnit
	if err := r.db.WithContext(ctx).First(&unit, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, ErrAgentGuardExecutionUnitNotFound
		}
		return nil, nil, nil, fmt.Errorf("resolve agent guard execution unit: %w", err)
	}
	instance, delivery, err := r.resolveInstanceAndDelivery(ctx, unit.InstanceID)
	if err != nil {
		return nil, nil, nil, err
	}
	if instance.HostID != unit.HostID {
		return nil, nil, nil, ErrAgentGuardActionStateConflict
	}
	return &unit, instance, delivery, nil
}

func (r *AgentGuardActionRepository) ResolveInstance(
	ctx context.Context,
	id uuid.UUID,
) (*model.AgentRuntimeInstance, *model.AgentGuardPolicyDelivery, error) {
	return r.resolveInstanceAndDelivery(ctx, id)
}

func (r *AgentGuardActionRepository) resolveInstanceAndDelivery(
	ctx context.Context,
	id uuid.UUID,
) (*model.AgentRuntimeInstance, *model.AgentGuardPolicyDelivery, error) {
	var instance model.AgentRuntimeInstance
	if err := r.db.WithContext(ctx).First(&instance, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrAgentGuardInstanceNotFound
		}
		return nil, nil, fmt.Errorf("resolve agent guard instance: %w", err)
	}
	var delivery model.AgentGuardPolicyDelivery
	err := r.db.WithContext(ctx).
		Where("host_id = ?", instance.HostID).
		Order("bundle_version DESC").
		First(&delivery).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &instance, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("resolve agent guard capability snapshot: %w", err)
	}
	return &instance, &delivery, nil
}

func (r *AgentGuardActionRepository) FindActiveFreeze(
	ctx context.Context,
	unitID uuid.UUID,
) (*model.AgentGuardAction, error) {
	var action model.AgentGuardAction
	err := r.db.WithContext(ctx).
		Where(
			"execution_unit_id = ? AND action IN ? AND status IN ?",
			unitID,
			[]string{model.AgentGuardActionFreezeExecutionUnit, model.AgentGuardActionHoldExecutionUnit},
			[]string{
				model.AgentGuardActionStatusPending,
				model.AgentGuardActionStatusDispatching,
				model.AgentGuardActionStatusRunning,
			},
		).
		Order("requested_at DESC").
		First(&action).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAgentGuardActionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find active agent guard freeze: %w", err)
	}
	return &action, nil
}

func (r *AgentGuardActionRepository) CreateOrGetActiveFreeze(
	ctx context.Context,
	action *model.AgentGuardAction,
) (*model.AgentGuardAction, bool, error) {
	var stored *model.AgentGuardAction
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var unit model.AgentExecutionUnit
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&unit, "id = ?", action.ExecutionUnitID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAgentGuardExecutionUnitNotFound
			}
			return fmt.Errorf("lock agent guard freeze target: %w", err)
		}
		var existing model.AgentGuardAction
		err := tx.Where(
			"execution_unit_id = ? AND action IN ? AND status IN ?",
			action.ExecutionUnitID,
			[]string{model.AgentGuardActionFreezeExecutionUnit, model.AgentGuardActionHoldExecutionUnit},
			[]string{
				model.AgentGuardActionStatusPending,
				model.AgentGuardActionStatusDispatching,
				model.AgentGuardActionStatusRunning,
			},
		).Order("requested_at DESC").First(&existing).Error
		if err == nil {
			stored = &existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("check active agent guard freeze: %w", err)
		}
		if err := tx.Create(action).Error; err != nil {
			return fmt.Errorf("create agent guard freeze action: %w", err)
		}
		copy := *action
		stored = &copy
		created = true
		return nil
	})
	return stored, created, err
}

func (r *AgentGuardActionRepository) Create(
	ctx context.Context,
	action *model.AgentGuardAction,
) error {
	if err := r.db.WithContext(ctx).Create(action).Error; err != nil {
		return fmt.Errorf("create agent guard action: %w", err)
	}
	return nil
}

func (r *AgentGuardActionRepository) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (*model.AgentGuardAction, error) {
	var action model.AgentGuardAction
	if err := r.db.WithContext(ctx).First(&action, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentGuardActionNotFound
		}
		return nil, fmt.Errorf("get agent guard action: %w", err)
	}
	return &action, nil
}

func (r *AgentGuardActionRepository) GetByCommandID(
	ctx context.Context,
	commandID string,
) (*model.AgentGuardAction, error) {
	var action model.AgentGuardAction
	if err := r.db.WithContext(ctx).First(&action, "command_id = ?", commandID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentGuardActionNotFound
		}
		return nil, fmt.Errorf("get agent guard action by command: %w", err)
	}
	return &action, nil
}

func (r *AgentGuardActionRepository) Transition(
	ctx context.Context,
	id uuid.UUID,
	nextStatus string,
	result datatypes.JSON,
	errorCode string,
	errorMessage string,
	at time.Time,
) (*model.AgentGuardAction, error) {
	var action model.AgentGuardAction
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&action, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAgentGuardActionNotFound
			}
			return fmt.Errorf("lock agent guard action: %w", err)
		}
		if action.Status == nextStatus {
			return nil
		}
		if !agentGuardActionTransitionAllowed(action.Status, nextStatus) {
			return ErrAgentGuardActionStateConflict
		}
		action.Status = nextStatus
		action.ErrorCode = truncateAgentGuardActionField(errorCode, 100)
		action.ErrorMessage = truncateAgentGuardActionField(errorMessage, 1000)
		if len(result) > 0 {
			if len(result) > 16*1024 {
				return fmt.Errorf("agent guard action result exceeds limit")
			}
			action.Result = append(datatypes.JSON(nil), result...)
		}
		if nextStatus == model.AgentGuardActionStatusDispatching && action.DispatchedAt == nil {
			action.DispatchedAt = &at
		}
		if agentGuardActionTerminal(nextStatus) {
			action.CompletedAt = &at
		}
		action.UpdatedAt = at
		if err := tx.Save(&action).Error; err != nil {
			return fmt.Errorf("transition agent guard action: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &action, nil
}

func agentGuardActionTransitionAllowed(current, next string) bool {
	if agentGuardActionTerminal(current) {
		return false
	}
	switch current {
	case model.AgentGuardActionStatusPending:
		return next == model.AgentGuardActionStatusDispatching ||
			next == model.AgentGuardActionStatusRunning ||
			agentGuardActionTerminal(next)
	case model.AgentGuardActionStatusDispatching:
		return next == model.AgentGuardActionStatusRunning || agentGuardActionTerminal(next)
	case model.AgentGuardActionStatusRunning:
		return agentGuardActionTerminal(next)
	default:
		return false
	}
}

func agentGuardActionTerminal(status string) bool {
	switch status {
	case model.AgentGuardActionStatusSuccess,
		model.AgentGuardActionStatusFailed,
		model.AgentGuardActionStatusExpired,
		model.AgentGuardActionStatusCancelled:
		return true
	default:
		return false
	}
}

func truncateAgentGuardActionField(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
