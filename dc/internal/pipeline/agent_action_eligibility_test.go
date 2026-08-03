package pipeline

import (
	"encoding/json"
	"testing"
	"time"

	"dc/internal/model"

	"github.com/google/uuid"
)

func TestEvaluateAgentGuardActionEligibilityRequiresEverySafetyGate(t *testing.T) {
	unitID := uuid.New()
	base := AgentActionEligibilityInput{
		RequestedAction:       "freeze_execution_unit",
		AttributionConfidence: "confirmed",
		ExecutionUnitID:       &unitID,
		CoverageLevel:         "full_enforcement",
		FindingVerdict:        "malicious",
		RuleEvidence:          true,
		NonToolEvidence:       true,
		EvidenceResolved:      true,
		EvidenceVisibility:    "complete",
		PublishedPolicy:       true,
		PolicyAuthorized:      true,
		DecisionSources:       []string{"agent_guard_rule", "event_correlation"},
		BehaviorDecisions:     []string{"audit"},
		Flags: AgentActionFeatureFlags{
			ActionEnabled: true, FreezeEnabled: true, PublishEnabled: true,
		},
	}
	if result := EvaluateAgentGuardActionEligibility(base); !result.Eligible || !result.Dispatchable {
		t.Fatalf("eligible result=%#v", result)
	}

	tests := []struct {
		name   string
		mutate func(*AgentActionEligibilityInput)
		reason string
	}{
		{name: "flags off", mutate: func(in *AgentActionEligibilityInput) { in.Flags.ActionEnabled = false }, reason: "action_disabled"},
		{name: "ambiguous attribution", mutate: func(in *AgentActionEligibilityInput) { in.AttributionConfidence = "probable" }, reason: "attribution_not_confirmed"},
		{name: "no unit", mutate: func(in *AgentActionEligibilityInput) { in.ExecutionUnitID = nil }, reason: "execution_unit_unresolved"},
		{name: "remote unobservable", mutate: func(in *AgentActionEligibilityInput) { in.CoverageLevel = "remote_unobservable" }, reason: "coverage_not_enforceable"},
		{name: "unresolved finding", mutate: func(in *AgentActionEligibilityInput) { in.FindingVerdict = "inconclusive" }, reason: "finding_unresolved"},
		{name: "AI only", mutate: func(in *AgentActionEligibilityInput) { in.DecisionSources = []string{"ai_analysis"} }, reason: "ai_only_action_forbidden"},
		{name: "tool semantics only", mutate: func(in *AgentActionEligibilityInput) { in.NonToolEvidence = false }, reason: "tool_semantics_only"},
		{name: "no rule evidence", mutate: func(in *AgentActionEligibilityInput) { in.RuleEvidence = false }, reason: "rule_evidence_missing"},
		{name: "missing event", mutate: func(in *AgentActionEligibilityInput) { in.EvidenceResolved = false }, reason: "evidence_unresolved"},
		{name: "partial evidence", mutate: func(in *AgentActionEligibilityInput) { in.EvidenceVisibility = "partial" }, reason: "evidence_incomplete"},
		{name: "unpublished policy", mutate: func(in *AgentActionEligibilityInput) { in.PublishedPolicy = false }, reason: "published_policy_required"},
		{name: "policy denies action", mutate: func(in *AgentActionEligibilityInput) { in.PolicyAuthorized = false }, reason: "policy_action_not_authorized"},
		{name: "would deny", mutate: func(in *AgentActionEligibilityInput) { in.BehaviorDecisions = []string{"would_deny"} }, reason: "non_executed_decision"},
		{name: "freeze flag off", mutate: func(in *AgentActionEligibilityInput) { in.Flags.FreezeEnabled = false }, reason: "freeze_disabled"},
		{name: "publisher off", mutate: func(in *AgentActionEligibilityInput) { in.Flags.PublishEnabled = false }, reason: "action_transport_disabled"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := base
			input.DecisionSources = append([]string(nil), base.DecisionSources...)
			input.BehaviorDecisions = append([]string(nil), base.BehaviorDecisions...)
			test.mutate(&input)
			result := EvaluateAgentGuardActionEligibility(input)
			if result.Eligible || result.Dispatchable || result.ReasonCode != test.reason {
				t.Fatalf("result=%#v want reason %s", result, test.reason)
			}
		})
	}
}

func TestDCDenyEligibilityNeverPretendsAsyncExecution(t *testing.T) {
	unitID := uuid.New()
	result := EvaluateAgentGuardActionEligibility(AgentActionEligibilityInput{
		RequestedAction: "deny", AttributionConfidence: "confirmed", ExecutionUnitID: &unitID,
		CoverageLevel: "full_enforcement", FindingVerdict: "malicious", RuleEvidence: true, NonToolEvidence: true,
		EvidenceResolved: true, EvidenceVisibility: "complete", PublishedPolicy: true,
		PolicyAuthorized: true, DecisionSources: []string{"agent_guard_rule"},
		BehaviorDecisions: []string{"audit"},
		Flags:             AgentActionFeatureFlags{ActionEnabled: true, DenyEnabled: true, PublishEnabled: true},
	})
	if !result.Eligible || result.Dispatchable || result.ReasonCode != "dc_async_deny_not_dispatchable" {
		t.Fatalf("deny result=%#v", result)
	}
}

func TestBuildAgentGuardActionCandidateUsesStableUnitScopedCommand(t *testing.T) {
	unitID, instanceID := uuid.New(), uuid.New()
	findingID := uuid.New()
	finding := &model.AgentSecurityFinding{
		ID: findingID, FindingKey: "correlation:v1:AGB-DOWNLOAD-EXEC-001:anchor",
		HostID: uuid.New(), InstanceID: &instanceID, ExecutionUnitID: &unitID,
		Severity: "critical", FirstObservedAt: time.Now().UTC(),
	}
	first := BuildAgentGuardActionCandidate(finding, "freeze_execution_unit", 300)
	second := BuildAgentGuardActionCandidate(finding, "freeze_execution_unit", 300)
	if first == nil || first.ID != second.ID || first.CommandID != second.CommandID ||
		first.CommandID != "AG-GUARD-"+first.ID.String() ||
		first.ExecutionUnitID == nil || *first.ExecutionUnitID != unitID {
		t.Fatalf("candidate is not stable/unit scoped: %#v %#v", first, second)
	}
	var result map[string]any
	if json.Unmarshal(first.Result, &result) != nil || result["target"] != unitID.String() {
		t.Fatalf("candidate result=%s", first.Result)
	}
}
