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

// ApprovalGate 审批网关
type ApprovalGate struct {
	approvalRepo  repository.AssistantApprovalRepository
	toolCallRepo  repository.AssistantToolCallRepository
	policyService *ToolPolicyService
	riskPolicy    *RiskPolicy
	logger        *zap.Logger
}

// ApprovalGateDeps 审批网关依赖
type ApprovalGateDeps struct {
	ApprovalRepo  repository.AssistantApprovalRepository
	ToolCallRepo  repository.AssistantToolCallRepository
	PolicyService *ToolPolicyService
	RiskPolicy    *RiskPolicy
	Logger        *zap.Logger
}

// NewApprovalGate 创建审批网关
func NewApprovalGate(deps ApprovalGateDeps) *ApprovalGate {
	return &ApprovalGate{
		approvalRepo:  deps.ApprovalRepo,
		toolCallRepo:  deps.ToolCallRepo,
		policyService: deps.PolicyService,
		riskPolicy:    deps.RiskPolicy,
		logger:        deps.Logger,
	}
}

// ApprovalEvaluateRequest 审批评估请求
type ApprovalEvaluateRequest struct {
	Operator  string `json:"operator"`
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	Args      map[string]interface{} `json:"args"`
}

// RiskDecision 风险决策
type RiskDecision struct {
	Allow            bool   `json:"allow"`
	RequiresApproval bool   `json:"requires_approval"`
	Mode             string `json:"mode"`
	RiskLevel        string `json:"risk_level"`
	Reason           string `json:"reason"`
}

// CreateApprovalRequest 创建审批请求
type CreateApprovalRequest struct {
	ToolCallID string                 `json:"tool_call_id"`
	SessionID  string                 `json:"session_id"`
	ToolName   string                 `json:"tool_name"`
	RiskLevel  string                 `json:"risk_level"`
	Title      string                 `json:"title"`
	Args       map[string]interface{} `json:"args"`
	Operator   string                 `json:"operator"`
}

// ApprovalExecutionResult 审批执行结果
type ApprovalExecutionResult struct {
	Approval   *model.AssistantApproval `json:"approval"`
	ToolResult *ToolExecutionResult     `json:"tool_result,omitempty"`
}

// Evaluate 评估审批需求
func (g *ApprovalGate) Evaluate(ctx context.Context, req ApprovalEvaluateRequest) (*RiskDecision, error) {
	// Get approval mode
	mode, err := g.policyService.GetApprovalMode(ctx)
	if err != nil {
		mode = "whitelist"
	}

	// Check whitelist status
	isWhitelisted, _ := g.policyService.IsToolWhitelisted(ctx, req.ToolName)

	// Get tool risk level from policy
	toolPolicy, err := g.policyService.policyRepo.FindByToolName(ctx, req.ToolName)
	riskLevel := "readonly"
	if err == nil {
		riskLevel = toolPolicy.RiskLevel
	}

	// Evaluate risk
	result := g.riskPolicy.Evaluate(ctx, RiskEvaluateRequest{
		ToolName:      req.ToolName,
		ToolRiskLevel: riskLevel,
		Mode:          mode,
		Whitelisted:   isWhitelisted,
		Operator:      req.Operator,
	})

	return &RiskDecision{
		Allow:            result.Allow,
		RequiresApproval: result.RequiresApproval,
		Mode:             result.Mode,
		RiskLevel:        result.RiskLevel,
		Reason:           fmt.Sprintf("mode=%s, whitelisted=%v, risk=%s", mode, isWhitelisted, riskLevel),
	}, nil
}

// CreateApproval 创建审批
func (g *ApprovalGate) CreateApproval(ctx context.Context, req CreateApprovalRequest) (*model.AssistantApproval, error) {
	approvalID := "appr_" + uuid.New().String()[:8]

	// Build params preview (exclude sensitive data)
	paramsPreview := make(map[string]interface{})
	for k, v := range req.Args {
		// Mask sensitive fields
		if isSensitiveField(k) {
			paramsPreview[k] = "***"
		} else {
			paramsPreview[k] = v
		}
	}

	// Set expiration (30 minutes)
	expiresAt := time.Now().Add(30 * time.Minute)

	approval := &model.AssistantApproval{
		ID:            uuid.New(),
		ApprovalID:    approvalID,
		SessionID:     req.SessionID,
		ToolCallID:    req.ToolCallID,
		ToolName:      req.ToolName,
		RiskLevel:     req.RiskLevel,
		Title:         req.Title,
		ParamsPreview: mustMarshalJSON(paramsPreview),
		Status:        model.ApprovalStatusPending,
		RequestedBy:   req.Operator,
		ExpiresAt:     &expiresAt,
	}

	if err := g.approvalRepo.Create(ctx, approval); err != nil {
		return nil, fmt.Errorf("failed to create approval: %w", err)
	}

	g.logger.Info("approval created",
		zap.String("approval_id", approvalID),
		zap.String("tool_name", req.ToolName),
		zap.String("risk_level", req.RiskLevel),
	)

	return approval, nil
}

// Approve 批准
func (g *ApprovalGate) Approve(ctx context.Context, approvalID string, operator string, comment string) (*ApprovalExecutionResult, error) {
	approval, err := g.approvalRepo.FindByApprovalID(ctx, approvalID)
	if err != nil {
		return nil, ErrApprovalNotFound
	}

	if approval.Status != model.ApprovalStatusPending {
		return nil, ErrApprovalNotPending
	}

	// Check expiration
	if approval.ExpiresAt != nil && approval.ExpiresAt.Before(time.Now()) {
		_ = g.approvalRepo.MarkExpired(ctx, approvalID)
		return nil, ErrApprovalExpired
	}

	// Mark approved
	if err := g.approvalRepo.MarkApproved(ctx, approvalID, operator, comment); err != nil {
		return nil, fmt.Errorf("failed to mark approval: %w", err)
	}

	// Reload approval with updated status
	approval, _ = g.approvalRepo.FindByApprovalID(ctx, approvalID)

	g.logger.Info("approval approved",
		zap.String("approval_id", approvalID),
		zap.String("operator", operator),
	)

	return &ApprovalExecutionResult{
		Approval: approval,
	}, nil
}

// Reject 拒绝
func (g *ApprovalGate) Reject(ctx context.Context, approvalID string, operator string, comment string) (*model.AssistantApproval, error) {
	approval, err := g.approvalRepo.FindByApprovalID(ctx, approvalID)
	if err != nil {
		return nil, ErrApprovalNotFound
	}

	if approval.Status != model.ApprovalStatusPending {
		return nil, ErrApprovalNotPending
	}

	if err := g.approvalRepo.MarkRejected(ctx, approvalID, operator, comment); err != nil {
		return nil, fmt.Errorf("failed to reject approval: %w", err)
	}

	// Mark tool call as rejected
	_ = g.toolCallRepo.MarkRejected(ctx, approval.ToolCallID, comment)

	approval, _ = g.approvalRepo.FindByApprovalID(ctx, approvalID)

	g.logger.Info("approval rejected",
		zap.String("approval_id", approvalID),
		zap.String("operator", operator),
	)

	return approval, nil
}

// MarkExecuted 标记审批已执行
func (g *ApprovalGate) MarkExecuted(ctx context.Context, approvalID string) error {
	return g.approvalRepo.MarkExecuted(ctx, approvalID)
}

// GetApproval 获取审批详情
func (g *ApprovalGate) GetApproval(ctx context.Context, approvalID string) (*model.AssistantApproval, error) {
	return g.approvalRepo.FindByApprovalID(ctx, approvalID)
}

// MarkFailed 标记审批执行失败
func (g *ApprovalGate) MarkFailed(ctx context.Context, approvalID string, errMsg string) error {
	return g.approvalRepo.MarkFailed(ctx, approvalID, errMsg)
}

func isSensitiveField(key string) bool {
	sensitiveFields := []string{"password", "token", "secret", "api_key", "credential", "private_key"}
	for _, f := range sensitiveFields {
		if key == f {
			return true
		}
	}
	return false
}

// Errors
var (
	ErrApprovalNotFound  = &ApprovalError{Message: "approval not found"}
	ErrApprovalNotPending = &ApprovalError{Message: "approval is not pending"}
	ErrApprovalExpired   = &ApprovalError{Message: "approval has expired"}
)

type ApprovalError struct {
	Message string
}

func (e *ApprovalError) Error() string {
	return e.Message
}
