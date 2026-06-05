package assistant

import (
	"context"

	"github.com/alex-chenc/aegis/api-server/internal/model"
	"github.com/alex-chenc/aegis/api-server/internal/repository"
	"go.uber.org/zap"
)

// ApprovalGate 审批网关
type ApprovalGate struct {
	approvalRepo repository.AssistantApprovalRepository
	toolCallRepo repository.AssistantToolCallRepository
	logger       *zap.Logger
}

// ApprovalGateDeps 审批网关依赖
type ApprovalGateDeps struct {
	ApprovalRepo repository.AssistantApprovalRepository
	ToolCallRepo repository.AssistantToolCallRepository
	Logger       *zap.Logger
}

// ApprovalEvaluateRequest 审批评估请求
type ApprovalEvaluateRequest struct {
	Operator  string
	SessionID string
	ToolName  string
	Args      map[string]interface{}
}

// RiskDecision 风险决策
type RiskDecision struct {
	Allow            bool
	RequiresApproval bool
	Mode             string
	RiskLevel        string
	Reason           string
	ImpactSummary    string
}

// CreateApprovalRequest 创建审批请求
type CreateApprovalRequest struct {
	ToolCallID string
	SessionID  string
	ToolName   string
	RiskLevel  string
	Title      string
	Args       map[string]interface{}
	Operator   string
}

// ApprovalExecutionResult 审批执行结果
type ApprovalExecutionResult struct {
	Approval   *model.AssistantApproval
	ToolResult *ToolExecutionResult
}

// NewApprovalGate 创建审批网关
func NewApprovalGate(deps ApprovalGateDeps) *ApprovalGate {
	return &ApprovalGate{
		approvalRepo: deps.ApprovalRepo,
		toolCallRepo: deps.ToolCallRepo,
		logger:       deps.Logger,
	}
}

// Evaluate 评估审批需求
func (g *ApprovalGate) Evaluate(ctx context.Context, req ApprovalEvaluateRequest) (*RiskDecision, error) {
	// TODO: Implement full evaluation logic with ToolPolicyService
	return &RiskDecision{
		Allow:            true,
		RequiresApproval: false,
		Mode:             "whitelist",
		RiskLevel:        "readonly",
		Reason:           "default allow",
	}, nil
}

// CreateApproval 创建审批
func (g *ApprovalGate) CreateApproval(ctx context.Context, req CreateApprovalRequest) (*model.AssistantApproval, error) {
	approval := &model.AssistantApproval{
		ApprovalID:  "appr_" + req.ToolCallID,
		SessionID:   req.SessionID,
		ToolCallID:  req.ToolCallID,
		ToolName:    req.ToolName,
		RiskLevel:   req.RiskLevel,
		Title:       req.Title,
		Status:      model.ApprovalStatusPending,
		RequestedBy: req.Operator,
	}

	if err := g.approvalRepo.Create(ctx, approval); err != nil {
		return nil, err
	}

	return approval, nil
}

// Approve 批准
func (g *ApprovalGate) Approve(ctx context.Context, approvalID string, operator string, comment string) (*ApprovalExecutionResult, error) {
	approval, err := g.approvalRepo.FindByApprovalID(ctx, approvalID)
	if err != nil {
		return nil, err
	}

	if approval.Status != model.ApprovalStatusPending {
		return nil, ErrApprovalNotPending
	}

	if err := g.approvalRepo.MarkApproved(ctx, approvalID, operator, comment); err != nil {
		return nil, err
	}

	// TODO: Execute the approved tool call

	return &ApprovalExecutionResult{
		Approval: approval,
	}, nil
}

// Reject 拒绝
func (g *ApprovalGate) Reject(ctx context.Context, approvalID string, operator string, comment string) (*model.AssistantApproval, error) {
	approval, err := g.approvalRepo.FindByApprovalID(ctx, approvalID)
	if err != nil {
		return nil, err
	}

	if approval.Status != model.ApprovalStatusPending {
		return nil, ErrApprovalNotPending
	}

	if err := g.approvalRepo.MarkRejected(ctx, approvalID, operator, comment); err != nil {
		return nil, err
	}

	return approval, nil
}

// Errors
var (
	ErrApprovalNotPending = &ApprovalError{Message: "approval is not pending"}
)

type ApprovalError struct {
	Message string
}

func (e *ApprovalError) Error() string {
	return e.Message
}
