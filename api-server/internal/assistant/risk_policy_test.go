package assistant

import (
	"context"
	"testing"

	"api-server/internal/model"
)

// newTestRiskPolicy builds a RiskPolicy without external dependencies.
func newTestRiskPolicy() *RiskPolicy {
	return NewRiskPolicy(RiskPolicyDeps{})
}

func TestRiskPolicy_FullAccessBypassesApprovalForHighRiskTool(t *testing.T) {
	policy := newTestRiskPolicy()

	// full_access must allow a high-risk tool to execute directly without approval.
	result := policy.Evaluate(context.Background(), RiskEvaluateRequest{
		ToolName:      "Baseline.Compliance.Run",
		ToolRiskLevel: string(ToolRiskHigh),
		Mode:          model.ApprovalModeFullAccess,
		Whitelisted:   false,
	})

	if !result.Allow {
		t.Fatalf("full_access should allow execution, got Allow=false")
	}
	if result.RequiresApproval {
		t.Fatalf("full_access should not require approval for high-risk tool, got RequiresApproval=true")
	}
}

func TestRiskPolicy_FullAccessBypassesApprovalForCriticalRiskTool(t *testing.T) {
	policy := newTestRiskPolicy()

	result := policy.Evaluate(context.Background(), RiskEvaluateRequest{
		ToolName:      "Critical.Tool",
		ToolRiskLevel: string(ToolRiskCritical),
		Mode:          model.ApprovalModeFullAccess,
		Whitelisted:   false,
	})

	if !result.Allow {
		t.Fatalf("full_access should allow critical-risk tool, got Allow=false")
	}
	if result.RequiresApproval {
		t.Fatalf("full_access should not require approval for critical-risk tool, got RequiresApproval=true")
	}
}

func TestRiskPolicy_WhitelistModeRequiresApprovalForHighRiskTool(t *testing.T) {
	policy := newTestRiskPolicy()

	result := policy.Evaluate(context.Background(), RiskEvaluateRequest{
		ToolName:      "Baseline.Compliance.Run",
		ToolRiskLevel: string(ToolRiskHigh),
		Mode:          model.ApprovalModeWhitelist,
		Whitelisted:   false,
	})

	if !result.Allow {
		t.Fatalf("whitelist mode should still allow (pending approval), got Allow=false")
	}
	if !result.RequiresApproval {
		t.Fatalf("whitelist mode should require approval for non-whitelisted high-risk tool")
	}
}

func TestRiskPolicy_WhitelistModeSkipsApprovalForWhitelistedReadonly(t *testing.T) {
	policy := newTestRiskPolicy()

	result := policy.Evaluate(context.Background(), RiskEvaluateRequest{
		ToolName:      "Host.List",
		ToolRiskLevel: string(ToolRiskReadonly),
		Mode:          model.ApprovalModeWhitelist,
		Whitelisted:   true,
	})

	if !result.Allow {
		t.Fatalf("whitelist mode should allow whitelisted tool, got Allow=false")
	}
	if result.RequiresApproval {
		t.Fatalf("whitelist mode should not require approval for whitelisted readonly tool")
	}
}

func TestRiskPolicy_RequestApprovalModeRequiresApprovalForAllTools(t *testing.T) {
	policy := newTestRiskPolicy()

	// Even a whitelisted readonly tool requires approval under request_approval.
	result := policy.Evaluate(context.Background(), RiskEvaluateRequest{
		ToolName:      "Host.List",
		ToolRiskLevel: string(ToolRiskReadonly),
		Mode:          model.ApprovalModeRequestApproval,
		Whitelisted:   true,
	})

	if !result.RequiresApproval {
		t.Fatalf("request_approval mode should require approval for all tools")
	}
}

func TestRiskPolicy_DefaultsToWhitelistForUnknownMode(t *testing.T) {
	policy := newTestRiskPolicy()

	result := policy.Evaluate(context.Background(), RiskEvaluateRequest{
		ToolName:      "Host.List",
		ToolRiskLevel: string(ToolRiskReadonly),
		Mode:          "unknown_mode",
		Whitelisted:   false,
	})

	if !result.RequiresApproval {
		t.Fatalf("unknown mode should default to whitelist behavior and require approval for non-whitelisted tool")
	}
}
