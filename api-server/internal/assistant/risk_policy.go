package assistant

import (
	"context"

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
	// Determine risk level from tool spec
	riskLevel := req.ToolRiskLevel
	if riskLevel == "" {
		riskLevel = "readonly"
	}

	// Determine if approval is required based on mode
	requiresApproval := false
	allow := true

	switch req.Mode {
	case "request_approval":
		// All tools require approval
		requiresApproval = true
	case "whitelist":
		// Whitelisted tools skip approval
		if !req.Whitelisted {
			requiresApproval = true
		}
	case "full_access":
		// All tools execute directly
		requiresApproval = false
	default:
		// Default to whitelist mode
		if !req.Whitelisted {
			requiresApproval = true
		}
	}

	// High-risk tools always require approval regardless of mode
	if riskLevel == "critical" || riskLevel == "high" {
		if req.Mode == "full_access" {
			// full_access still allows, but marks for audit
			allow = true
		} else {
			requiresApproval = true
		}
	}

	return RiskEvaluateResult{
		Allow:            allow,
		RequiresApproval: requiresApproval,
		Mode:             req.Mode,
		RiskLevel:        riskLevel,
	}
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
