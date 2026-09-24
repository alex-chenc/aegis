package mcpauthz

import (
	"context"
	"testing"
	"time"
)

func securityFixture(phase string) SecurityInput {
	started := phase == "post"
	status := "not_started"
	if started {
		status = "succeeded"
	}
	return SecurityInput{
		ContractVersion: SecurityInputContractVersion, Phase: phase, Operation: "tools/call",
		Request: SecurityRequest{InvocationID: "invocation", DecisionID: "decision", ReceivedAtUnixMS: time.Now().UnixMilli()},
		Tool:    SecurityTool{ExposedName: "read", RiskTier: "l1"}, Arguments: map[string]any{},
		Upstream: SecurityUpstream{Started: started, Status: status}, PayloadDigest: "sha256:test",
		Snapshot: SecuritySnapshot{DeploymentGeneration: 1, PolicyRevision: SecurityPolicyRevision},
	}
}

func TestDefaultSecurityPolicyUsesRegoForPreAndPost(t *testing.T) {
	evaluator, err := PrepareSecurity(context.Background(), SecurityPolicyRevision, DefaultSecurityPolicy)
	if err != nil {
		t.Fatal(err)
	}
	defer evaluator.Close()

	pre := securityFixture("pre")
	pre.Tool.RiskTier = "l4"
	decision, err := evaluator.EvaluateSecurity(context.Background(), pre)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != "deny" || len(decision.RuleIDs) != 1 || decision.RuleIDs[0] != "mcp.security.risk.l4.v1" {
		t.Fatalf("unexpected pre decision: %#v", decision)
	}

	post := securityFixture("post")
	post.Result = map[string]any{"access_token": "redacted-by-test"}
	decision, err = evaluator.EvaluateSecurity(context.Background(), post)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != "deny" || len(decision.RuleIDs) != 1 || decision.RuleIDs[0] != "mcp.security.output.sensitive-key.v1" {
		t.Fatalf("unexpected post decision: %#v", decision)
	}
}

func TestSecurityInputRejectsPhaseAndUpstreamMismatch(t *testing.T) {
	input := securityFixture("pre")
	input.Upstream.Started = true
	if err := input.Validate(); err == nil {
		t.Fatal("expected pre input with started upstream to fail closed")
	}
	input = securityFixture("post")
	input.Upstream.Started = false
	if err := input.Validate(); err == nil {
		t.Fatal("expected post input without started upstream to fail closed")
	}
	input = securityFixture("pre")
	input.Result = map[string]any{"unexpected": true}
	if err := input.Validate(); err == nil {
		t.Fatal("expected pre input with a result to fail closed")
	}
	input = securityFixture("post")
	input.Upstream.Status = "not_started"
	if err := input.Validate(); err == nil {
		t.Fatal("expected post input with invalid upstream status to fail closed")
	}
	input = securityFixture("pre")
	input.PayloadDigest = ""
	if err := input.Validate(); err == nil {
		t.Fatal("expected missing payload digest to fail closed")
	}
	input.PayloadDigest = "digest-without-algorithm"
	if err := input.Validate(); err == nil {
		t.Fatal("expected payload digest without algorithm prefix to fail closed")
	}
}

func TestDecodeSecurityDecisionRejectsUnknownAction(t *testing.T) {
	_, err := DecodeSecurityDecision([]byte(`{"contract_version":"aegis.mcp.security.decision.v2","phase":"pre","action":"permit","reason_code":"POLICY_ALLOWED","rule_ids":[],"audit_rule_ids":[],"severity":"low","evidence_refs":[],"policy_revision":"mcp-security-rego-v1"}`), SecurityPolicyRevision)
	if err == nil {
		t.Fatal("expected unknown action to be rejected")
	}
}

func TestDecodeSecurityDecisionRejectsInconsistentAuditFields(t *testing.T) {
	cases := []string{
		`{"contract_version":"aegis.mcp.security.decision.v2","phase":"pre","action":"allow","reason_code":"POLICY_ALLOWED","rule_ids":["rule"],"audit_rule_ids":[],"severity":"low","evidence_refs":[],"policy_revision":"mcp-security-rego-v1"}`,
		`{"contract_version":"aegis.mcp.security.decision.v2","phase":"pre","action":"deny","reason_code":"POLICY_ALLOWED","rule_ids":["rule"],"audit_rule_ids":[],"severity":"critical","evidence_refs":[],"policy_revision":"mcp-security-rego-v1"}`,
		`{"contract_version":"aegis.mcp.security.decision.v2","phase":"post","action":"audit","reason_code":"AUDIT_ONLY","rule_ids":[],"audit_rule_ids":["rule"],"severity":"low","evidence_refs":[{"stage":"pre","path":"$","digest":"sha256:test"}],"policy_revision":"mcp-security-rego-v1"}`,
	}
	for _, raw := range cases {
		if _, err := DecodeSecurityDecision([]byte(raw), SecurityPolicyRevision); err == nil {
			t.Fatalf("expected inconsistent decision to be rejected: %s", raw)
		}
	}
}

func TestSecurityEvaluatorRejectsStalePolicyRevision(t *testing.T) {
	evaluator, err := PrepareSecurity(context.Background(), SecurityPolicyRevision, DefaultSecurityPolicy)
	if err != nil {
		t.Fatal(err)
	}
	defer evaluator.Close()
	input := securityFixture("pre")
	input.Snapshot.PolicyRevision = "stale"
	if _, err := evaluator.EvaluateSecurity(context.Background(), input); err == nil {
		t.Fatal("expected stale policy revision to fail closed")
	}
}
