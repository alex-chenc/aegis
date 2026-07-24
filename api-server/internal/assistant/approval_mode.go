package assistant

import (
	"context"
	"strings"

	"api-server/internal/model"
)

func normalizeAssistantApprovalMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case model.ApprovalModeRequestApproval:
		return model.ApprovalModeRequestApproval
	case model.ApprovalModeFullAccess:
		return model.ApprovalModeFullAccess
	case model.ApprovalModeWhitelist:
		return model.ApprovalModeWhitelist
	default:
		return model.ApprovalModeWhitelist
	}
}

func (o *Orchestrator) snapshotApprovalMode(ctx context.Context) string {
	if o == nil || o.approvalGate == nil || o.approvalGate.policyService == nil {
		return model.ApprovalModeWhitelist
	}
	mode, err := o.approvalGate.policyService.GetApprovalMode(ctx)
	if err != nil {
		return model.ApprovalModeWhitelist
	}
	return normalizeAssistantApprovalMode(mode)
}

// applyEffectiveApprovalMode makes the authorization artifact describe the
// actual decision for this run. Tool contracts still retain their default
// approval requirement in the registry.
func applyEffectiveApprovalMode(plan *ToolExecutionPlan, mode string) {
	if plan == nil || normalizeAssistantApprovalMode(mode) != model.ApprovalModeFullAccess {
		return
	}
	for index := range plan.Steps {
		plan.Steps[index].RequiresApproval = false
	}
	for index := range plan.DecisionRecords {
		if plan.DecisionRecords[index].Decision != toolDecisionAccepted {
			continue
		}
		plan.DecisionRecords[index].ApprovalState = "not_required"
		if plan.DecisionRecords[index].Evidence == nil {
			plan.DecisionRecords[index].Evidence = make(map[string]interface{})
		}
		plan.DecisionRecords[index].Evidence["approval_mode"] = model.ApprovalModeFullAccess
	}
}

func approvalModePromptRule(mode string) string {
	switch normalizeAssistantApprovalMode(mode) {
	case model.ApprovalModeFullAccess:
		return "The current run uses full_access. Authorized tools execute without interactive approval; do not ask for or wait for approval. RBAC, tool enablement, explicit write intent, target validation, and argument validation still apply."
	case model.ApprovalModeRequestApproval:
		return "The current run uses request_approval. Every tool execution requires interactive approval before dispatch."
	default:
		return "The current run uses whitelist mode. Non-whitelisted and high-risk operations require interactive approval before dispatch."
	}
}
