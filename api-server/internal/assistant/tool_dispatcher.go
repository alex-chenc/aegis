package assistant

import (
	"context"
	"fmt"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ToolDispatcher 工具调度器
type ToolDispatcher struct {
	registry      *ToolRegistry
	approvalGate  *ApprovalGate
	toolCallRepo  repository.AssistantToolCallRepository
	sessionRepo   repository.AssistantSessionRepository
	policyService *ToolPolicyService
	logger        *zap.Logger
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
		logger:        logger,
	}
}

// DispatchRequest 调度请求
type DispatchRequest struct {
	SessionID string                 `json:"session_id"`
	MessageID string                 `json:"message_id"`
	RunID     string                 `json:"run_id"`
	ToolName  string                 `json:"tool_name"`
	Args      map[string]interface{} `json:"args"`
	Operator  string                 `json:"operator"`
	Approved  bool                   `json:"approved"` // true if already approved via approval gate
}

// DispatchResult 调度结果
type DispatchResult struct {
	CallID          string                 `json:"call_id"`
	ToolName        string                 `json:"tool_name"`
	Success         bool                   `json:"success"`
	Data            interface{}            `json:"data,omitempty"`
	Error           string                 `json:"error,omitempty"`
	DurationMs      int64                  `json:"duration_ms"`
	ApprovalRequired bool                 `json:"approval_required,omitempty"`
	ApprovalID      string                 `json:"approval_id,omitempty"`
}

// Dispatch 调度工具执行
func (d *ToolDispatcher) Dispatch(ctx context.Context, req DispatchRequest) (*DispatchResult, error) {
	// Get tool spec
	tool, ok := d.registry.Get(req.ToolName)
	if !ok {
		return nil, fmt.Errorf("tool %s not found", req.ToolName)
	}

	// Create tool call record
	callID := "call_" + uuid.New().String()[:8]
	toolCall := &model.AssistantToolCall{
		ID:        uuid.New(),
		SessionID: req.SessionID,
		MessageID: req.MessageID,
		CallID:    callID,
		ToolName:  req.ToolName,
		Domain:    tool.Domain,
		RiskLevel: tool.RiskLevel,
		Status:    model.ToolCallStatusRunning,
		Args:      mustMarshalJSON(req.Args),
	}
	if err := d.toolCallRepo.Create(ctx, toolCall); err != nil {
		d.logger.Error("failed to create tool call record", zap.Error(err))
	}

	// Check if already approved
	if req.Approved {
		return d.executeTool(ctx, callID, tool, req)
	}

	// Evaluate risk
	isWhitelisted, _ := d.policyService.IsToolWhitelisted(ctx, req.ToolName)
	mode, _ := d.policyService.GetApprovalMode(ctx)

	riskResult := d.approvalGate.riskPolicy.Evaluate(ctx, RiskEvaluateRequest{
		ToolName:      req.ToolName,
		ToolRiskLevel: tool.RiskLevel,
		Mode:          mode,
		Whitelisted:   isWhitelisted,
		Operator:      req.Operator,
	})

	if riskResult.RequiresApproval {
		// Create approval request
		approval, err := d.approvalGate.CreateApproval(ctx, CreateApprovalRequest{
			ToolCallID: callID,
			SessionID:  req.SessionID,
			ToolName:   req.ToolName,
			RiskLevel:  tool.RiskLevel,
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

		return &DispatchResult{
			CallID:           callID,
			ToolName:         req.ToolName,
			ApprovalRequired: true,
			ApprovalID:       approval.ApprovalID,
		}, nil
	}

	// Execute directly
	return d.executeTool(ctx, callID, tool, req)
}

func (d *ToolDispatcher) executeTool(ctx context.Context, callID string, tool *ToolSpec, req DispatchRequest) (*DispatchResult, error) {
	start := time.Now()
	result, err := d.registry.Execute(ctx, req.ToolName, req.Args)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		_ = d.toolCallRepo.MarkFailed(ctx, callID, err.Error(), duration)
		return &DispatchResult{
			CallID:     callID,
			ToolName:   req.ToolName,
			Success:    false,
			Error:      err.Error(),
			DurationMs: duration,
		}, nil
	}

	if result.Success {
		_ = d.toolCallRepo.MarkSuccess(ctx, callID, result.Data, duration)
		_ = d.sessionRepo.IncrementToolCallCount(ctx, req.SessionID)
	} else {
		_ = d.toolCallRepo.MarkFailed(ctx, callID, result.Error, duration)
	}

	return &DispatchResult{
		CallID:     callID,
		ToolName:   req.ToolName,
		Success:    result.Success,
		Data:       result.Data,
		Error:      result.Error,
		DurationMs: duration,
	}, nil
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
		ToolName:  approval.ToolName,
		Args:      args,
		Operator:  operator,
		Approved:  true,
	}

	return d.executeTool(ctx, approval.ToolCallID, tool, req)
}
