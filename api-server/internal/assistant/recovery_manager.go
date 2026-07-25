package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"api-server/internal/model"
	"api-server/internal/recovery"
	"api-server/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type RecoveryCreateRequest struct {
	SessionID     string
	RunID         string
	MessageID     string
	StepID        string
	ToolCallID    string
	ToolName      string
	OriginalQuery string
	OriginalArgs  map[string]interface{}
	Operator      string
	Error         error
}

type RecoveryDecisionRequest struct {
	ActionID string                 `json:"action_id"`
	Input    map[string]interface{} `json:"input,omitempty"`
}

type RecoveryDecisionResult struct {
	Recovery      *model.AssistantRecoveryRequest `json:"recovery"`
	Action        recovery.Action                 `json:"action"`
	Execution     map[string]interface{}          `json:"execution,omitempty"`
	ResumeRequest bool                            `json:"resume_request,omitempty"`
	RunHandle     *RunHandle                      `json:"run_handle,omitempty"`
}

type RecoveryActionExecutor interface {
	ExecuteRecoveryAction(
		ctx context.Context,
		request *model.AssistantRecoveryRequest,
		action recovery.Action,
		input map[string]interface{},
		operator string,
	) (map[string]interface{}, error)
}

// RecoveryManager persists and executes only actions declared by a typed
// backend recovery error. It never derives recovery behavior from error text.
type RecoveryManager struct {
	repo      repository.AssistantRecoveryRepository
	logger    *zap.Logger
	mu        sync.RWMutex
	executors map[string]RecoveryActionExecutor
}

func NewRecoveryManager(repo repository.AssistantRecoveryRepository, logger *zap.Logger) *RecoveryManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RecoveryManager{
		repo:      repo,
		logger:    logger,
		executors: make(map[string]RecoveryActionExecutor),
	}
}

func (m *RecoveryManager) RegisterExecutor(name string, executor RecoveryActionExecutor) {
	if m == nil || executor == nil || strings.TrimSpace(name) == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executors[strings.TrimSpace(name)] = executor
}

func (m *RecoveryManager) CreateFromError(ctx context.Context, input RecoveryCreateRequest) (*model.AssistantRecoveryRequest, bool, error) {
	if m == nil || m.repo == nil {
		return nil, false, nil
	}
	descriptor, ok := recovery.Describe(input.Error)
	if !ok {
		return nil, false, nil
	}
	if existing, err := m.repo.FindActiveByRunStepCode(
		ctx,
		input.RunID,
		input.StepID,
		input.ToolName,
		descriptor.Code,
	); err == nil {
		return existing, true, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, true, fmt.Errorf("find active recovery request: %w", err)
	}
	if existing, err := m.repo.FindPendingByToolCall(ctx, input.ToolCallID, descriptor.Code); err == nil {
		return existing, true, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, true, fmt.Errorf("find existing recovery request: %w", err)
	}

	request := &model.AssistantRecoveryRequest{
		ID:               uuid.New(),
		RecoveryID:       "recovery_" + uuid.NewString()[:8],
		SessionID:        input.SessionID,
		RunID:            input.RunID,
		MessageID:        input.MessageID,
		StepID:           input.StepID,
		ToolCallID:       input.ToolCallID,
		ToolName:         input.ToolName,
		Code:             descriptor.Code,
		Category:         string(descriptor.Category),
		RiskLevel:        descriptor.RiskLevel,
		Summary:          descriptor.Summary,
		Detail:           descriptor.Detail,
		OriginalQuery:    input.OriginalQuery,
		OriginalArgs:     mustMarshalJSON(redactRecoveryValue(input.OriginalArgs)),
		Context:          mustMarshalJSON(descriptor.Context),
		Actions:          mustMarshalJSON(descriptor.Actions),
		Status:           model.RecoveryStatusPending,
		DecisionInput:    datatypes.JSON([]byte("{}")),
		ResolutionResult: datatypes.JSON([]byte("{}")),
		RequestedBy:      input.Operator,
	}
	if err := m.repo.Create(ctx, request); err != nil {
		// A concurrent retry may win the active business-key insert between the
		// lookup above and Create. Return that durable request instead of
		// surfacing a duplicate-key failure or creating a second decision card.
		if existing, findErr := m.repo.FindActiveByRunStepCode(
			ctx,
			input.RunID,
			input.StepID,
			input.ToolName,
			descriptor.Code,
		); findErr == nil {
			return existing, true, nil
		}
		return nil, true, fmt.Errorf("create recovery request: %w", err)
	}
	m.logger.Info("assistant recovery request created",
		zap.String("recovery_id", request.RecoveryID),
		zap.String("session_id", request.SessionID),
		zap.String("run_id", request.RunID),
		zap.String("tool_name", request.ToolName),
		zap.String("error_code", request.Code),
		zap.Strings("action_ids", recoveryActionIDs(descriptor.Actions)),
	)
	return request, true, nil
}

func (m *RecoveryManager) Get(ctx context.Context, recoveryID string) (*model.AssistantRecoveryRequest, error) {
	if m == nil || m.repo == nil {
		return nil, fmt.Errorf("recovery manager unavailable")
	}
	return m.repo.FindByRecoveryID(ctx, recoveryID)
}

func (m *RecoveryManager) FindPendingByRun(ctx context.Context, runID string) (*model.AssistantRecoveryRequest, error) {
	if m == nil || m.repo == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return m.repo.FindPendingByRun(ctx, runID)
}

func (m *RecoveryManager) ListBySession(ctx context.Context, sessionID string, page, pageSize int) ([]model.AssistantRecoveryRequest, int64, error) {
	if m == nil || m.repo == nil {
		return nil, 0, fmt.Errorf("recovery manager unavailable")
	}
	return m.repo.ListBySession(ctx, sessionID, page, pageSize)
}

func (m *RecoveryManager) Decide(
	ctx context.Context,
	recoveryID string,
	decision RecoveryDecisionRequest,
	operator string,
) (*RecoveryDecisionResult, error) {
	request, err := m.Get(ctx, recoveryID)
	if err != nil {
		return nil, err
	}
	if request.Status != model.RecoveryStatusPending && request.Status != model.RecoveryStatusPaused {
		return nil, repository.ErrRecoveryAlreadyDecided
	}
	action, err := recoveryActionFromRequest(request, decision.ActionID)
	if err != nil {
		return nil, err
	}
	if action.InputRequired && strings.TrimSpace(recoveryStringValue(decision.Input["comment"])) == "" {
		return nil, fmt.Errorf("action %s requires a non-empty comment", action.ID)
	}
	inputJSON := mustMarshalJSON(decision.Input)
	if err := m.repo.BeginDecision(ctx, recoveryID, action.ID, operator, inputJSON); err != nil {
		return nil, err
	}
	request.Status = model.RecoveryStatusExecuting
	request.SelectedActionID = action.ID
	request.DecisionInput = inputJSON
	request.DecidedBy = operator

	m.logger.Info("assistant recovery decision selected",
		zap.String("recovery_id", request.RecoveryID),
		zap.String("session_id", request.SessionID),
		zap.String("run_id", request.RunID),
		zap.String("action_id", action.ID),
		zap.String("operator", operator),
	)

	status := model.RecoveryStatusResolved
	execution := map[string]interface{}{}
	switch action.ID {
	case "pause":
		status = model.RecoveryStatusPaused
		execution["message"] = "recovery request paused"
	case "cancel":
		status = model.RecoveryStatusCancelled
		execution["message"] = "recovery request cancelled"
	case "provide_other":
		execution["message"] = "operator guidance recorded for the linked assistant run"
	default:
		if action.Executor == recoveryResumeExecutor {
			execution["message"] = "assistant run will resume with the selected recovery context"
		} else {
			executor := m.executor(action.Executor)
			if executor == nil {
				err = fmt.Errorf("recovery action executor %q is unavailable", action.Executor)
			} else {
				execution, err = executor.ExecuteRecoveryAction(ctx, request, action, decision.Input, operator)
			}
		}
	}
	if err == nil && action.KeepsOpen {
		status = model.RecoveryStatusPaused
	}
	if err != nil {
		failureStatus := model.RecoveryStatusFailed
		if action.RetrySafe {
			failureStatus = model.RecoveryStatusPending
		}
		_ = m.repo.CompleteDecision(ctx, recoveryID, failureStatus, mustMarshalJSON(map[string]interface{}{
			"error": err.Error(),
		}), "")
		m.logger.Error("assistant recovery action failed",
			zap.String("recovery_id", request.RecoveryID),
			zap.String("session_id", request.SessionID),
			zap.String("action_id", action.ID),
			zap.Error(err),
		)
		return nil, err
	}
	if err := m.repo.CompleteDecision(ctx, recoveryID, status, mustMarshalJSON(execution), ""); err != nil {
		return nil, err
	}
	request, err = m.Get(ctx, recoveryID)
	if err != nil {
		return nil, err
	}
	m.logger.Info("assistant recovery decision completed",
		zap.String("recovery_id", request.RecoveryID),
		zap.String("session_id", request.SessionID),
		zap.String("action_id", action.ID),
		zap.String("status", status),
		zap.Bool("resume_requested", action.ResumesRun && status == model.RecoveryStatusResolved),
	)
	return &RecoveryDecisionResult{
		Recovery:      request,
		Action:        action,
		Execution:     execution,
		ResumeRequest: action.ResumesRun && status == model.RecoveryStatusResolved,
	}, nil
}

func (m *RecoveryManager) LinkResumeRun(ctx context.Context, recoveryID, resumeRunID string) error {
	if m == nil || m.repo == nil {
		return fmt.Errorf("recovery manager unavailable")
	}
	request, err := m.Get(ctx, recoveryID)
	if err != nil {
		return err
	}
	return m.repo.CompleteDecision(ctx, recoveryID, request.Status, request.ResolutionResult, resumeRunID)
}

func (m *RecoveryManager) executor(name string) RecoveryActionExecutor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.executors[name]
}

func recoveryActionFromRequest(request *model.AssistantRecoveryRequest, actionID string) (recovery.Action, error) {
	var actions []recovery.Action
	if request == nil || json.Unmarshal(request.Actions, &actions) != nil {
		return recovery.Action{}, fmt.Errorf("recovery actions are invalid")
	}
	for _, action := range actions {
		if action.ID == actionID {
			return action, nil
		}
	}
	return recovery.Action{}, fmt.Errorf("recovery action %q is not authorized for this request", actionID)
}

func recoveryActionIDs(actions []recovery.Action) []string {
	result := make([]string, 0, len(actions))
	for _, action := range actions {
		result = append(result, action.ID)
	}
	return result
}

func recoveryStringValue(value interface{}) string {
	text, _ := value.(string)
	return text
}

func redactRecoveryValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			if isSensitiveField(normalizedKey) ||
				strings.Contains(normalizedKey, "password") ||
				strings.Contains(normalizedKey, "token") ||
				strings.Contains(normalizedKey, "api_key") ||
				strings.Contains(normalizedKey, "secret") ||
				strings.Contains(normalizedKey, "credential") ||
				strings.Contains(normalizedKey, "private_key") ||
				strings.Contains(normalizedKey, "script_content") ||
				strings.Contains(normalizedKey, "source_code") {
				result[key] = "***"
				continue
			}
			result[key] = redactRecoveryValue(item)
		}
		return result
	case map[string]string:
		converted := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			converted[key] = item
		}
		return redactRecoveryValue(converted)
	case []interface{}:
		result := make([]interface{}, len(typed))
		for index, item := range typed {
			result[index] = redactRecoveryValue(item)
		}
		return result
	case string:
		const maxRecoveryArgumentLength = 2048
		runes := []rune(typed)
		if len(runes) > maxRecoveryArgumentLength {
			return string(runes[:maxRecoveryArgumentLength]) + "…"
		}
		return typed
	default:
		return value
	}
}
