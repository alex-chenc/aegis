package assistant

import (
	"context"

	"api-server/internal/model"
	"api-server/internal/repository"
)

// RiskPolicy 风险策略（对齐设计文档 8.2 节）
type RiskPolicy struct {
	systemConfig *repository.SystemConfigRepo
}

// RiskPolicyDeps 风险策略依赖
type RiskPolicyDeps struct {
	SystemConfig *repository.SystemConfigRepo
}

// NewRiskPolicy 创建风险策略
func NewRiskPolicy(deps RiskPolicyDeps) *RiskPolicy {
	return &RiskPolicy{
		systemConfig: deps.SystemConfig,
	}
}

// Evaluate 评估工具风险
func (p *RiskPolicy) Evaluate(ctx context.Context, req RiskEvaluateRequest) RiskEvaluateResult {
	// full_access（全权限）模式下用户明确授予所有工具直接执行的权限，
	// 工具级别的 RequiresApproval 标记不应再强制审批。应用工具级覆盖前先短路。
	if req.Mode == model.ApprovalModeFullAccess {
		return RiskEvaluateResult{
			Allow:            true,
			RequiresApproval: false,
			Mode:             req.Mode,
			RiskLevel:        effectiveRiskLevel(req.ToolRiskLevel),
		}
	}

	riskLevel := effectiveRiskLevel(req.ToolRiskLevel)

	// Determine if approval is required based on mode.
	// full_access is handled by the early return above.
	requiresApproval := false

	switch req.Mode {
	case "request_approval":
		// All tools require approval
		requiresApproval = true
	case "whitelist":
		// Whitelisted tools skip approval
		if !req.Whitelisted {
			requiresApproval = true
		}
	default:
		// Default to whitelist mode
		if !req.Whitelisted {
			requiresApproval = true
		}
	}

	// High-risk tools always require approval under whitelist/request_approval
	// modes. full_access is handled by the early return above and bypasses
	// approval entirely.
	if riskLevel == "critical" || riskLevel == "high" {
		requiresApproval = true
	}

	return RiskEvaluateResult{
		Allow:            true,
		RequiresApproval: requiresApproval,
		Mode:             req.Mode,
		RiskLevel:        riskLevel,
	}
}

// effectiveRiskLevel 规范化工具风险等级，空值默认为 readonly。
func effectiveRiskLevel(level string) string {
	if level == "" {
		return "readonly"
	}
	return level
}

// RiskEvaluateRequest 风险评估请求
type RiskEvaluateRequest struct {
	ToolName      string `json:"tool_name"`
	ToolRiskLevel string `json:"tool_risk_level"`
	Mode          string `json:"mode"`        // request_approval, whitelist, full_access
	Whitelisted   bool   `json:"whitelisted"` // whether tool is in whitelist
	Operator      string `json:"operator"`
}

// RiskEvaluateResult 风险评估结果
type RiskEvaluateResult struct {
	Allow            bool   `json:"allow"`
	RequiresApproval bool   `json:"requires_approval"`
	Mode             string `json:"mode"`
	RiskLevel        string `json:"risk_level"`
}
