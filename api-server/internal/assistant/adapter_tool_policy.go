package assistant

import (
	"context"

	"api-server/internal/model"

	agentruntime "github.com/alex-chenc/agent-runtime"
	"go.uber.org/zap"
)

// AssistantToolPolicy 适配 agent-runtime ToolPolicy 接口
// 将 RiskPolicy + ApprovalGate 的评估逻辑封装为 agentruntime.ToolPolicy
type AssistantToolPolicy struct {
	riskPolicy   *RiskPolicy
	approvalGate *ApprovalGate
	policySvc    *ToolPolicyService
	registry     *ToolRegistry
	sessionID    string
	operator     string
	logger       *zap.Logger
}

// AssistantToolPolicyConfig 适配器配置
type AssistantToolPolicyConfig struct {
	RiskPolicy   *RiskPolicy
	ApprovalGate *ApprovalGate
	PolicySvc    *ToolPolicyService
	Registry     *ToolRegistry
	SessionID    string
	Operator     string
	Logger       *zap.Logger
}

// NewAssistantToolPolicy 创建 ToolPolicy 适配器
func NewAssistantToolPolicy(cfg AssistantToolPolicyConfig) *AssistantToolPolicy {
	return &AssistantToolPolicy{
		riskPolicy:   cfg.RiskPolicy,
		approvalGate: cfg.ApprovalGate,
		policySvc:    cfg.PolicySvc,
		registry:     cfg.Registry,
		sessionID:    cfg.SessionID,
		operator:     cfg.Operator,
		logger:       cfg.Logger,
	}
}

// Evaluate 实现 agentruntime.ToolPolicy 接口
// agent-runtime 在每次工具调用前调用此方法，决定是否允许执行
func (p *AssistantToolPolicy) Evaluate(ctx context.Context, req agentruntime.ToolPolicyRequest) (agentruntime.ToolPolicyDecision, error) {
	// 1. 查找工具规格
	spec, ok := p.registry.Get(req.ToolName)
	if !ok {
		p.logger.Warn("tool not found in registry",
			zap.String("tool_name", req.ToolName),
		)
		return agentruntime.PolicyDeny, nil
	}

	// 2. 检查工具是否启用
	if !spec.Enabled {
		return agentruntime.PolicyDeny, nil
	}

	// 3. 获取审批模式
	approvalMode, _ := p.policySvc.GetApprovalMode(ctx)

	// 4. 使用 RiskPolicy 评估
	riskResult := p.riskPolicy.Evaluate(ctx, RiskEvaluateRequest{
		ToolName:      req.ToolName,
		ToolRiskLevel: string(spec.Risk),
		Mode:          approvalMode,
		Whitelisted:   spec.DefaultWhitelisted,
		Operator:      p.operator,
	})
	// 工具级 RequiresApproval 在非全权限模式下仍强制审批；
	// full_access 模式下用户已授予直接执行权限，跳过此覆盖。
	if spec.RequiresApproval && approvalMode != model.ApprovalModeFullAccess {
		riskResult.RequiresApproval = true
	}

	// 5. 映射到 agent-runtime 策略决策
	if riskResult.RequiresApproval {
		// 在 agent-runtime 层返回 RequireApproval
		// agent-runtime 会将此信息传递给 ToolGateway，由 ToolGateway 创建审批
		return agentruntime.PolicyRequireApproval, nil
	}
	if !riskResult.Allow {
		return agentruntime.PolicyDeny, nil
	}

	return agentruntime.PolicyAllow, nil
}
