package pipeline

import (
	"strings"

	"dc/internal/model"

	"github.com/google/uuid"
)

type AgentActionFeatureFlags struct {
	ActionEnabled  bool
	DenyEnabled    bool
	FreezeEnabled  bool
	PublishEnabled bool
}

type AgentActionEligibilityInput struct {
	RequestedAction       string
	AttributionConfidence string
	ExecutionUnitID       *uuid.UUID
	CoverageLevel         string
	FindingVerdict        string
	RuleEvidence          bool
	NonToolEvidence       bool
	EvidenceResolved      bool
	EvidenceVisibility    string
	PublishedPolicy       bool
	PolicyAuthorized      bool
	DecisionSources       []string
	BehaviorDecisions     []string
	FreezeTimeoutSeconds  int
	Flags                 AgentActionFeatureFlags
}

type AgentActionEligibilityResult struct {
	Eligible     bool
	Dispatchable bool
	ReasonCode   string
}

func EvaluateAgentGuardActionEligibility(input AgentActionEligibilityInput) AgentActionEligibilityResult {
	reject := func(reason string) AgentActionEligibilityResult {
		return AgentActionEligibilityResult{ReasonCode: reason}
	}
	if !input.Flags.ActionEnabled {
		return reject("action_disabled")
	}
	if input.AttributionConfidence != "confirmed" {
		return reject("attribution_not_confirmed")
	}
	if input.ExecutionUnitID == nil || *input.ExecutionUnitID == uuid.Nil {
		return reject("execution_unit_unresolved")
	}
	if input.CoverageLevel != "full_enforcement" &&
		input.CoverageLevel != "behavior_monitor_escape_enforce" {
		return reject("coverage_not_enforceable")
	}
	if input.FindingVerdict != "malicious" {
		return reject("finding_unresolved")
	}
	if containsStringValue(input.DecisionSources, "ai_analysis") &&
		!containsStringValue(input.DecisionSources, "agent_guard_rule") &&
		!containsStringValue(input.DecisionSources, "event_correlation") {
		return reject("ai_only_action_forbidden")
	}
	if !input.NonToolEvidence {
		return reject("tool_semantics_only")
	}
	if !input.RuleEvidence ||
		(!containsStringValue(input.DecisionSources, "agent_guard_rule") &&
			!containsStringValue(input.DecisionSources, "event_correlation")) {
		return reject("rule_evidence_missing")
	}
	if !input.EvidenceResolved {
		return reject("evidence_unresolved")
	}
	if input.EvidenceVisibility != "complete" {
		return reject("evidence_incomplete")
	}
	if !input.PublishedPolicy {
		return reject("published_policy_required")
	}
	if !input.PolicyAuthorized {
		return reject("policy_action_not_authorized")
	}
	for _, decision := range input.BehaviorDecisions {
		if decision == "would_deny" || decision == "enforcement_unavailable" {
			return reject("non_executed_decision")
		}
	}
	switch input.RequestedAction {
	case "freeze_execution_unit":
		if !input.Flags.FreezeEnabled {
			return reject("freeze_disabled")
		}
		if !input.Flags.PublishEnabled {
			return reject("action_transport_disabled")
		}
		return AgentActionEligibilityResult{Eligible: true, Dispatchable: true}
	case "deny":
		if !input.Flags.DenyEnabled {
			return reject("deny_disabled")
		}
		return AgentActionEligibilityResult{
			Eligible: true, Dispatchable: false, ReasonCode: "dc_async_deny_not_dispatchable",
		}
	default:
		return reject("unsupported_action")
	}
}

func BuildAgentGuardActionCandidate(
	finding *model.AgentSecurityFinding,
	action string,
	freezeTimeoutSeconds int,
) *model.AgentGuardAction {
	if finding == nil || finding.ID == uuid.Nil || finding.ExecutionUnitID == nil ||
		finding.InstanceID == nil || strings.TrimSpace(action) == "" {
		return nil
	}
	key := "agent-action:v1:" + finding.ID.String() + ":" + action
	actionID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(key))
	timeout := freezeTimeoutSeconds
	requestedAt := finding.LastObservedAt
	if requestedAt.IsZero() {
		requestedAt = finding.FirstObservedAt
	}
	return &model.AgentGuardAction{
		ID: actionID, CommandID: "AG-GUARD-" + actionID.String(), TriggerFindingID: &finding.ID,
		HostID: finding.HostID, InstanceID: finding.InstanceID,
		ExecutionUnitID: finding.ExecutionUnitID, Action: action,
		Source: "correlation_policy", Status: "pending",
		Reason:               "published correlation policy authorized deterministic Agent Guard action",
		FreezeTimeoutSeconds: &timeout,
		Result: mustJSON(map[string]any{
			"target": finding.ExecutionUnitID.String(), "dispatch_state": "candidate",
		}, map[string]any{}),
		RequestedAt: requestedAt,
	}
}
