package mcpauthz

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func validInput() Input {
	return Input{ContractVersion: InputContractVersion, Operation: "tools/call", Request: NowRequest("a", "d", time.Unix(1, 0)), Principal: Principal{ClientID: "client", CredentialID: "credential"}, Grant: Grant{ID: "grant", Version: 1, ResourceScope: map[string]any{}}, Tool: Tool{CatalogReleaseID: "release", ReleaseToolID: "release-tool", ToolRevisionID: "tool", ServerRevisionID: "server", ExposedName: "read", UpstreamName: "read", InputSchemaDigest: "sha256:schema", RiskTier: "l1"}, Arguments: map[string]any{}, Snapshot: Snapshot{DeploymentGeneration: 1, PolicyRevision: "policy-1"}}
}

func TestDecodeDecisionRejectsAmbiguity(t *testing.T) {
	base := `{"contract_version":"aegis.mcp.authz.decision.v1","allow":true,"reason_code":"POLICY_ALLOWED","deny_rule_ids":[],"audit_rule_ids":[],"policy_revision":"p"}`
	for _, raw := range []string{
		base + ` {}`,
		`{"contract_version":"aegis.mcp.authz.decision.v1","allow":true,"reason_code":"POLICY_ALLOWED","deny_rule_ids":[],"deny_rule_ids":[],"audit_rule_ids":[],"policy_revision":"p"}`,
		`{"contract_version":"aegis.mcp.authz.decision.v1","allow":"true","reason_code":"POLICY_ALLOWED","deny_rule_ids":[],"audit_rule_ids":[],"policy_revision":"p"}`,
	} {
		if _, err := DecodeDecision([]byte(raw)); err == nil {
			t.Fatalf("expected malformed decision to fail: %s", raw)
		}
	}
}

func TestPreparedPolicyDefaultDenyAndPermit(t *testing.T) {
	policy := Policy{Revision: "p", Permissions: map[string]map[string]Permission{"client": {"release-tool": {CatalogReleaseID: "release", ToolRevisionID: "tool"}}}}
	prepared, err := Prepare(context.Background(), policy)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	input := validInput()
	input.Snapshot.PolicyRevision = "p"
	decision, err := prepared.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allow || decision.ReasonCode != PolicyAllowed {
		t.Fatalf("expected permit, got %#v", decision)
	}
	input.Principal.ClientID = "new-client"
	decision, err = prepared.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allow || len(decision.DenyRuleIDs) == 0 {
		t.Fatalf("expected default deny, got %#v", decision)
	}
}

func TestPreparedPolicyRejectsUnverifiedUserRequirement(t *testing.T) {
	policy := Policy{Revision: "p", Permissions: map[string]map[string]Permission{"client": {"release-tool": {CatalogReleaseID: "release", ToolRevisionID: "tool", RequireUser: true, UserIDs: []string{"u"}}}}}
	prepared, err := Prepare(context.Background(), policy)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	input := validInput()
	input.Snapshot.PolicyRevision = "p"
	decision, err := prepared.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allow {
		t.Fatal("unverified user must not satisfy a user permission")
	}
}

func TestDecodeArgumentsKeepsSchemaSemantics(t *testing.T) {
	object, canonical, err := DecodeArguments([]byte(`{"limit":2}`), []byte(`{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":3}},"required":["limit"],"additionalProperties":false}`))
	if err != nil || object["limit"].(json.Number) != json.Number("2") || string(canonical) != `{"limit":2}` {
		t.Fatalf("unexpected arguments object=%#v canonical=%s err=%v", object, canonical, err)
	}
	for _, raw := range []string{`{"limit":0}`, `{"limit":"2"}`, `{"limit":2,"extra":true}`, `{"limit":2,"limit":3}`, `{"limit":2} {}`} {
		if _, _, err := DecodeArguments([]byte(raw), []byte(`{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":3}},"required":["limit"],"additionalProperties":false}`)); err == nil {
			t.Fatalf("expected arguments to fail: %s", raw)
		}
	}
	if _, _, err := DecodeArguments([]byte(`{"value":9007199254740993}`), []byte(`{"type":"object","properties":{"value":{"type":"integer","maximum":9007199254740992}},"additionalProperties":false}`)); err == nil {
		t.Fatal("large integer above maximum must not be rounded through float64")
	}
}

func TestPreparedSnapshotCancellationDoesNotAllow(t *testing.T) {
	prepared, err := Prepare(context.Background(), Policy{Revision: "p", Permissions: map[string]map[string]Permission{}})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	input := validInput()
	input.Snapshot.PolicyRevision = "p"
	_, err = prepared.Evaluate(ctx, input)
	if err == nil || !errors.Is(err, ErrEvaluationCanceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestCompatibilityEvaluatorRequiresExplicitReleaseToolBinding(t *testing.T) {
	evaluator := NewCompatibilityEvaluator()
	input := validInput()
	decision, err := evaluator.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allow || len(decision.DenyRuleIDs) == 0 {
		t.Fatalf("unbound compatibility call must deny: %#v", decision)
	}
	evaluator.Allow(input.Principal.ClientID, input.Tool.ReleaseToolID)
	decision, err = evaluator.Evaluate(context.Background(), input)
	if err != nil || !decision.Allow {
		t.Fatalf("explicit compatibility permit should allow: %#v err=%v", decision, err)
	}
}
