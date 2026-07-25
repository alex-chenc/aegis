package assistant

import (
	"context"
	"fmt"
	"strings"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const defaultToolExecutionTimeout = 30 * time.Second

// ToolDispatcher 工具调度器
type ToolDispatcher struct {
	registry      *ToolRegistry
	approvalGate  *ApprovalGate
	toolCallRepo  repository.AssistantToolCallRepository
	sessionRepo   repository.AssistantSessionRepository
	policyService *ToolPolicyService
	filters       *ToolInvocationFilterChain
	recovery      *RecoveryManager
	logger        *zap.Logger
}

func (d *ToolDispatcher) SetRecoveryManager(manager *RecoveryManager) {
	if d != nil {
		d.recovery = manager
	}
}

// NewToolDispatcher 创建工具调度器
func NewToolDispatcher(
	registry *ToolRegistry,
	approvalGate *ApprovalGate,
	toolCallRepo repository.AssistantToolCallRepository,
	sessionRepo repository.AssistantSessionRepository,
	policyService *ToolPolicyService,
	logger *zap.Logger,
) *ToolDispatcher {
	return &ToolDispatcher{
		registry:      registry,
		approvalGate:  approvalGate,
		toolCallRepo:  toolCallRepo,
		sessionRepo:   sessionRepo,
		policyService: policyService,
		filters:       NewToolInvocationFilterChain(registry, logger),
		logger:        logger,
	}
}

// DispatchRequest 调度请求
type DispatchRequest struct {
	SessionID    string                 `json:"session_id"`
	MessageID    string                 `json:"message_id"`
	RunID        string                 `json:"run_id"`
	StepID       string                 `json:"step_id,omitempty"`
	CallID       string                 `json:"call_id,omitempty"`
	ToolName     string                 `json:"tool_name"`
	Args         map[string]interface{} `json:"args"`
	Operator     string                 `json:"operator"`
	UserQuery    string                 `json:"-"`
	ApprovalMode string                 `json:"approval_mode,omitempty"`
	Approved     bool                   `json:"approved"` // true if already approved via approval gate
	// Polling marks an internal Runtime retry of an already-authorized,
	// read-only, idempotent status call. It updates the original logical tool
	// call instead of creating another user-visible/audit row.
	Polling bool `json:"polling,omitempty"`
}

// DispatchResult 调度结果
type DispatchResult struct {
	CallID           string                          `json:"call_id"`
	ToolName         string                          `json:"tool_name"`
	Success          bool                            `json:"success"`
	Data             interface{}                     `json:"data,omitempty"`
	Error            string                          `json:"error,omitempty"`
	DurationMs       int64                           `json:"duration_ms"`
	ApprovalRequired bool                            `json:"approval_required,omitempty"`
	ApprovalID       string                          `json:"approval_id,omitempty"`
	Approval         *model.AssistantApproval        `json:"approval,omitempty"`
	Blocked          bool                            `json:"blocked,omitempty"`
	Recovery         *model.AssistantRecoveryRequest `json:"recovery,omitempty"`
}

// Dispatch 调度工具执行
func (d *ToolDispatcher) Dispatch(ctx context.Context, req DispatchRequest) (*DispatchResult, error) {
	// Get tool spec
	tool, ok := d.registry.Get(req.ToolName)
	if !ok {
		return nil, fmt.Errorf("tool %s not found", req.ToolName)
	}
	if !tool.ExposurePolicy.DirectCallable {
		return nil, fmt.Errorf("tool %s is internal and cannot be called directly by the model", req.ToolName)
	}
	if req.Polling && (tool.Risk != ToolRiskReadonly || !tool.Idempotent) {
		return nil, fmt.Errorf("tool %s is not eligible for automatic asynchronous polling", req.ToolName)
	}

	phase := ToolInvocationPhaseDispatch
	if req.Approved {
		phase = ToolInvocationPhaseApprovalResume
	}
	prepared, err := d.PrepareInvocation(ctx, ToolInvocationFilterRequest{
		SessionID: req.SessionID,
		MessageID: req.MessageID,
		RunID:     req.RunID,
		StepID:    req.StepID,
		Phase:     phase,
		ToolName:  req.ToolName,
		Args:      req.Args,
	})
	if err != nil {
		return nil, fmt.Errorf("tool %s pre-invocation rejected: %w", req.ToolName, err)
	}
	req.Args = prepared.Args

	callID := req.CallID
	if callID == "" {
		callID = "call_" + uuid.New().String()[:8]
	}
	if !req.Polling {
		// A normal model-selected call creates one durable logical record.
		// Internal polling updates that same record until it reaches a terminal
		// business outcome.
		toolCall := &model.AssistantToolCall{
			ID:        uuid.New(),
			SessionID: req.SessionID,
			RunID:     req.RunID,
			MessageID: req.MessageID,
			CallID:    callID,
			ToolName:  req.ToolName,
			Domain:    string(tool.Domain),
			RiskLevel: string(tool.Risk),
			Status:    model.ToolCallStatusRunning,
			Args:      mustMarshalJSON(req.Args),
		}
		if err := d.toolCallRepo.Create(ctx, toolCall); err != nil {
			d.logger.Error("failed to create tool call record", zap.Error(err))
		}
	}

	if req.Polling {
		return d.executeTool(ctx, callID, tool, req)
	}

	// Check if already approved
	if req.Approved {
		return d.executeTool(ctx, callID, tool, req)
	}

	// Evaluate risk
	isWhitelisted, _ := d.policyService.IsToolWhitelisted(ctx, req.ToolName)
	mode := normalizeAssistantApprovalMode(req.ApprovalMode)
	if strings.TrimSpace(req.ApprovalMode) == "" {
		configuredMode, _ := d.policyService.GetApprovalMode(ctx)
		mode = normalizeAssistantApprovalMode(configuredMode)
	}

	riskResult := d.approvalGate.riskPolicy.Evaluate(ctx, RiskEvaluateRequest{
		ToolName:      req.ToolName,
		ToolRiskLevel: string(tool.Risk),
		Mode:          mode,
		Whitelisted:   isWhitelisted,
		Operator:      req.Operator,
	})
	// 工具级 RequiresApproval 在非全权限模式下仍强制审批；
	// full_access 模式下用户已授予直接执行权限，跳过此覆盖。
	if tool.RequiresApproval && mode != model.ApprovalModeFullAccess {
		riskResult.RequiresApproval = true
	}
	if mode == model.ApprovalModeFullAccess && (tool.RequiresApproval || tool.Risk == ToolRiskHigh || tool.Risk == ToolRiskCritical) {
		d.logger.Info("assistant tool executing under full access",
			zap.String("session_id", req.SessionID),
			zap.String("run_id", req.RunID),
			zap.String("tool_name", req.ToolName),
			zap.String("risk_level", string(tool.Risk)),
			zap.String("approval_mode", mode),
		)
	}

	if riskResult.RequiresApproval {
		// Create approval request
		approval, err := d.approvalGate.CreateApproval(ctx, CreateApprovalRequest{
			ToolCallID: callID,
			SessionID:  req.SessionID,
			ToolName:   req.ToolName,
			RiskLevel:  string(tool.Risk),
			Title:      fmt.Sprintf("审批: %s", tool.Description),
			Args:       req.Args,
			Operator:   req.Operator,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create approval: %w", err)
		}

		// Mark tool call as requiring approval
		_ = d.toolCallRepo.MarkApprovalRequired(ctx, callID, approval.ApprovalID)
		_ = d.sessionRepo.IncrementApprovalCount(ctx, req.SessionID)
		_ = d.sessionRepo.UpdateStatus(ctx, req.SessionID, model.SessionStatusWaitingApproval)

		return &DispatchResult{
			CallID:           callID,
			ToolName:         req.ToolName,
			ApprovalRequired: true,
			ApprovalID:       approval.ApprovalID,
			Approval:         approval,
		}, nil
	}

	// Execute directly
	return d.executeTool(ctx, callID, tool, req)
}

// PrepareInvocation runs the shared, side-effect-free pre-invocation boundary.
// Callers must use the returned arguments and must not mutate them afterward.
func (d *ToolDispatcher) PrepareInvocation(ctx context.Context, req ToolInvocationFilterRequest) (ToolInvocationFilterResult, error) {
	if d == nil || d.filters == nil {
		return ToolInvocationFilterResult{}, fmt.Errorf("tool invocation filters are not configured")
	}
	return d.filters.Prepare(ctx, req)
}

func (d *ToolDispatcher) executeTool(ctx context.Context, callID string, tool *ToolSpec, req DispatchRequest) (*DispatchResult, error) {
	// Apply the tool-owned timeout contract while preserving the historical
	// default for tools that do not declare a custom execution budget.
	executionTimeout := defaultToolExecutionTimeout
	if tool != nil && tool.DefaultTimeout > 0 {
		executionTimeout = tool.DefaultTimeout
	}
	toolCtx, cancel := context.WithTimeout(ctx, executionTimeout)
	defer cancel()

	start := time.Now()
	type toolResult struct {
		result *ToolExecutionResult
		err    error
	}
	resultCh := make(chan toolResult, 1)
	toolCtx = WithToolInvocationContext(toolCtx, ToolInvocationContext{
		SessionID: req.SessionID,
		MessageID: req.MessageID,
		RunID:     req.RunID,
		CallID:    callID,
		Operator:  req.Operator,
		Approved:  req.Approved,
	})

	go func() {
		res, err := d.registry.Execute(toolCtx, req.ToolName, req.Args)
		resultCh <- toolResult{result: res, err: err}
	}()

	select {
	case tr := <-resultCh:
		duration := time.Since(start).Milliseconds()

		if tr.err != nil {
			_ = d.toolCallRepo.MarkFailed(ctx, callID, tr.err.Error(), duration)
			return &DispatchResult{
				CallID:     callID,
				ToolName:   req.ToolName,
				Success:    false,
				Error:      tr.err.Error(),
				DurationMs: duration,
			}, nil
		}

		if tr.result.Success {
			_ = d.toolCallRepo.MarkSuccess(ctx, callID, tr.result.Data, duration)
			outcome := normalizeToolOutcome(tool, tr.result.Data)
			if err := d.toolCallRepo.MarkOutcome(
				ctx,
				callID,
				string(outcome.OperationStatus),
				outcome.Terminal,
				outcome,
			); err != nil {
				d.logger.Warn("failed to persist assistant tool business outcome",
					zap.String("call_id", callID),
					zap.String("tool_name", tool.Name),
					zap.String("operation_status", string(outcome.OperationStatus)),
					zap.Bool("terminal", outcome.Terminal),
					zap.Error(err),
				)
			}
			if !req.Polling {
				_ = d.sessionRepo.IncrementToolCallCount(ctx, req.SessionID)
			}
		} else {
			var recoveryRequest *model.AssistantRecoveryRequest
			var recoverable bool
			if d.recovery != nil && tr.result.Cause != nil {
				var createErr error
				recoveryRequest, recoverable, createErr = d.recovery.CreateFromError(ctx, RecoveryCreateRequest{
					SessionID:     req.SessionID,
					RunID:         req.RunID,
					MessageID:     req.MessageID,
					StepID:        req.StepID,
					ToolCallID:    callID,
					ToolName:      req.ToolName,
					OriginalQuery: req.UserQuery,
					OriginalArgs:  req.Args,
					Operator:      req.Operator,
					Error:         tr.result.Cause,
				})
				if createErr != nil {
					d.logger.Error("failed to persist assistant recovery request",
						zap.String("session_id", req.SessionID),
						zap.String("run_id", req.RunID),
						zap.String("call_id", callID),
						zap.String("tool_name", req.ToolName),
						zap.Error(createErr),
					)
					recoverable = false
				}
			}
			if recoverable && recoveryRequest != nil {
				_ = d.toolCallRepo.MarkFailed(ctx, callID, recoveryRequest.Summary, duration)
				_ = d.toolCallRepo.UpdateStatus(ctx, callID, model.ToolCallStatusBlocked)
				return &DispatchResult{
					CallID:     callID,
					ToolName:   req.ToolName,
					Success:    false,
					Error:      recoveryRequest.Summary,
					DurationMs: duration,
					Blocked:    true,
					Recovery:   recoveryRequest,
				}, nil
			}
			_ = d.toolCallRepo.MarkFailed(ctx, callID, tr.result.Error, duration)
		}

		return &DispatchResult{
			CallID:     callID,
			ToolName:   req.ToolName,
			Success:    tr.result.Success,
			Data:       tr.result.Data,
			Error:      tr.result.Error,
			DurationMs: duration,
		}, nil

	case <-toolCtx.Done():
		duration := time.Since(start).Milliseconds()
		timeoutErr := fmt.Sprintf("tool %s execution timeout after %s", req.ToolName, executionTimeout)
		d.logger.Warn("tool execution timeout",
			zap.String("tool", req.ToolName),
			zap.String("call_id", callID),
			zap.Int64("duration_ms", duration),
			zap.Int64("timeout_ms", executionTimeout.Milliseconds()),
		)
		_ = d.toolCallRepo.MarkFailed(ctx, callID, timeoutErr, duration)
		return &DispatchResult{
			CallID:     callID,
			ToolName:   req.ToolName,
			Success:    false,
			Error:      timeoutErr,
			DurationMs: duration,
		}, nil
	}
}

// ExecuteApprovedTool 执行已批准的工具
func (d *ToolDispatcher) ExecuteApprovedTool(ctx context.Context, approvalID string, operator string) (*DispatchResult, error) {
	// Find approval
	approval, err := d.approvalGate.approvalRepo.FindByApprovalID(ctx, approvalID)
	if err != nil {
		return nil, fmt.Errorf("approval not found: %w", err)
	}

	// Find tool call
	toolCall, err := d.toolCallRepo.FindByCallID(ctx, approval.ToolCallID)
	if err != nil {
		return nil, fmt.Errorf("tool call not found: %w", err)
	}

	// Get tool spec
	tool, ok := d.registry.Get(approval.ToolName)
	if !ok {
		return nil, fmt.Errorf("tool %s not found", approval.ToolName)
	}

	// Parse args
	var args map[string]interface{}
	if toolCall.Args != nil {
		args = unmarshalJSON(toolCall.Args)
	}

	// Execute
	req := DispatchRequest{
		SessionID: approval.SessionID,
		MessageID: toolCall.MessageID,
		RunID:     toolCall.RunID,
		ToolName:  approval.ToolName,
		Args:      args,
		Operator:  operator,
		Approved:  true,
	}

	prepared, err := d.PrepareInvocation(ctx, ToolInvocationFilterRequest{
		SessionID: req.SessionID,
		MessageID: req.MessageID,
		RunID:     req.RunID,
		Phase:     ToolInvocationPhaseApprovalResume,
		ToolName:  req.ToolName,
		Args:      req.Args,
	})
	if err != nil {
		return nil, fmt.Errorf("approved tool %s failed pre-invocation validation: %w", req.ToolName, err)
	}
	req.Args = prepared.Args
	return d.executeTool(ctx, approval.ToolCallID, tool, req)
}
