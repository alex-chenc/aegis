package mcpauthz

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPreparedPolicyRejectsMismatchedBindingsAndRiskDenyWins(t *testing.T) {
	prepared, err := Prepare(context.Background(), Policy{
		Revision:         "r1",
		BlockRiskAtLeast: "l4",
		Permissions: map[string]map[string]Permission{
			"client": {
				"release-tool": {
					CatalogReleaseID: "release", ToolRevisionID: "tool",
					ProjectIDs: []string{"project-a"}, MaxLimit: 10,
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()

	input := validInput()
	input.Snapshot.PolicyRevision = "r1"
	input.Arguments = map[string]any{"project_id": "project-a", "limit": int64(2)}
	input.Grant.ResourceScope = map[string]any{"project_ids": []any{"project-a"}}
	input.Tool.RiskTier = "l4"
	decision, err := prepared.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allow || decision.ReasonCode != PolicyDenied || len(decision.DenyRuleIDs) < 1 {
		t.Fatalf("risk deny must win over matching permit: %#v", decision)
	}

	input.Tool.RiskTier = "l1"
	input.Arguments = map[string]any{"project_id": "project-b", "limit": int64(2)}
	decision, err = prepared.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allow || len(decision.DenyRuleIDs) == 0 {
		t.Fatalf("project scope mismatch must default deny: %#v", decision)
	}
}

func TestPreparedPolicyRequiresVerifiedUserAndLimitBounds(t *testing.T) {
	prepared, err := Prepare(context.Background(), Policy{
		Revision: "r1",
		Permissions: map[string]map[string]Permission{
			"client": {"release-tool": {
				CatalogReleaseID: "release", ToolRevisionID: "tool",
				RequireUser: true, UserIDs: []string{"u1"}, MaxLimit: 3,
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()

	input := validInput()
	input.Snapshot.PolicyRevision = "r1"
	input.Arguments = map[string]any{"limit": int64(3)}
	for name, user := range map[string]User{
		"unverified": {Verified: false, ID: "u1"},
		"wrong-user": {Verified: true, ID: "u2"},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := input
			candidate.Principal.User = user
			decision, err := prepared.Evaluate(context.Background(), candidate)
			if err != nil {
				t.Fatal(err)
			}
			if decision.Allow {
				t.Fatalf("user requirement bypassed: %#v", decision)
			}
		})
	}

	input.Principal.User = User{Verified: true, ID: "u1"}
	for _, limit := range []any{int64(0), int64(4), "3"} {
		input.Arguments = map[string]any{"limit": limit}
		decision, err := prepared.Evaluate(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		if decision.Allow {
			t.Fatalf("invalid limit %v bypassed policy: %#v", limit, decision)
		}
	}
}

type blockingEvaluator struct{}

func (blockingEvaluator) Evaluate(ctx context.Context, _ Input) (Decision, error) {
	<-ctx.Done()
	return Decision{}, ctx.Err()
}

func TestEvaluateWithTimeoutReturnsCancellation(t *testing.T) {
	start := time.Now()
	_, err := EvaluateWithTimeout(context.Background(), blockingEvaluator{}, validInput(), 5*time.Millisecond)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected evaluator cancellation, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timeout was not bounded: %s", elapsed)
	}
}

func TestPreparedSnapshotRejectsClosedOrStaleRevision(t *testing.T) {
	prepared, err := Prepare(context.Background(), Policy{Revision: "r1", Permissions: map[string]map[string]Permission{}})
	if err != nil {
		t.Fatal(err)
	}
	input := validInput()
	input.Snapshot.PolicyRevision = "r2"
	if _, err := prepared.Evaluate(context.Background(), input); !errors.Is(err, ErrPolicyUnavailable) {
		t.Fatalf("stale policy revision must fail closed, got %v", err)
	}
	prepared.Close()
	input.Snapshot.PolicyRevision = "r1"
	if _, err := prepared.Evaluate(context.Background(), input); !errors.Is(err, ErrPolicyUnavailable) {
		t.Fatalf("closed snapshot must fail closed, got %v", err)
	}
}

func TestPrepareRegoCompilesAndEvaluatesDecisionContract(t *testing.T) {
	source := `package aegis.mcp.authz
import rego.v1
default allow := false
allow if input.principal.client_id == "client"
reason_code := "POLICY_ALLOWED" if allow
reason_code := "POLICY_DENIED" if not allow
deny_rule_ids := [] if allow
deny_rule_ids := ["NO_MATCHING_PERMIT"] if not allow
decision := {"contract_version": "aegis.mcp.authz.decision.v1", "allow": allow, "reason_code": reason_code, "deny_rule_ids": deny_rule_ids, "audit_rule_ids": [], "policy_revision": data.aegis_config.meta.revision}`
	prepared, err := Prepare(context.Background(), Policy{Revision: "rego-r1", RegoSource: source})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	input := validInput()
	input.Snapshot.PolicyRevision = "rego-r1"
	decision, err := prepared.Evaluate(context.Background(), input)
	if err != nil || !decision.Allow || decision.PolicyRevision != "rego-r1" {
		t.Fatalf("rego decision = %#v, err=%v", decision, err)
	}
	if !prepared.AllowsTool("unknown-client", "unknown-tool") {
		t.Fatal("rego source snapshots must defer visibility to call-time evaluation")
	}
}

func TestPrepareRegoRejectsForbiddenBuiltinAndMalformedContract(t *testing.T) {
	for name, source := range map[string]string{
		"forbidden builtin": `package aegis.mcp.authz
import rego.v1
decision := http.send({"method": "GET", "url": "http://example.test"})`,
		"missing decision fields": `package aegis.mcp.authz
import rego.v1
decision := {"allow": true}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Prepare(context.Background(), Policy{Revision: "rego-r1", RegoSource: source}); err == nil {
				t.Fatal("invalid Rego policy was accepted")
			}
		})
	}
}
