package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"api-server/internal/mcpauthz"
	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type regressionEvaluator struct {
	decision mcpauthz.Decision
	err      error
	wait     bool
}

func (e regressionEvaluator) Evaluate(ctx context.Context, _ mcpauthz.Input) (mcpauthz.Decision, error) {
	if e.wait {
		<-ctx.Done()
		return mcpauthz.Decision{}, ctx.Err()
	}
	return e.decision, e.err
}

func allowRegressionDecision() mcpauthz.Decision {
	return mcpauthz.Decision{ContractVersion: mcpauthz.DecisionContractVersion, Allow: true, ReasonCode: mcpauthz.PolicyAllowed, DenyRuleIDs: []string{}, AuditRuleIDs: []string{}, PolicyRevision: "compatibility-v1"}
}

func denyRegressionDecision() mcpauthz.Decision {
	return mcpauthz.Decision{ContractVersion: mcpauthz.DecisionContractVersion, Allow: false, ReasonCode: mcpauthz.PolicyDenied, DenyRuleIDs: []string{"test.deny"}, AuditRuleIDs: []string{}, PolicyRevision: "compatibility-v1"}
}

func regressionRuntimeFixture(t *testing.T, withDecisionTable bool) (*MCPPlatformService, *httptest.Server, *MCPClientEndpointCreated, *int, *gorm.DB) {
	t.Helper()
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"content":[{"type":"text","text":"ok"}]}}`))
	}))
	svc, db := newMCPPlatformTestService(t)
	now := time.Now().UTC()
	serverID, revisionID := uuid.New(), uuid.New()
	serverURL := strings.Replace(upstream.URL, "127.0.0.1", "localhost", 1)
	if err := db.Create(&model.MCPServer{ID: serverID, ServerKey: "authz-regression", DisplayName: "Authz Regression", OwnerUserID: "admin", Environment: "test", Transport: model.MCPPlatformTransportStreamableHTTP, EndpointURL: serverURL, EndpointDisplay: serverURL, AuthType: model.MCPPlatformAuthNone, RiskTier: model.MCPPlatformRiskL1, LifecycleStatus: model.MCPPlatformServerPublished, ActiveRevisionID: &revisionID, ToolCount: 1, PublishedToolCount: 1, CreatedBy: "admin", UpdatedBy: "admin", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.MCPServerRevision{ID: revisionID, ServerID: serverID, RevisionNo: 1, ProtocolVersion: "2025-11-25", ToolsSnapshot: []byte(`[]`), Digest: "authz-digest", Status: model.MCPPlatformServerApproved, CreatedBy: "admin", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	tool := model.MCPToolRevision{ID: uuid.New(), ServerRevisionID: revisionID, UpstreamName: "health", Alias: "health", InputSchema: []byte(`{"type":"object","description":"Health probe arguments.","properties":{},"additionalProperties":false}`), OutputSchema: []byte(`{}`), RiskTier: model.MCPPlatformRiskL1, Status: "approved", CreatedAt: now}
	if err := db.Create(&tool).Error; err != nil {
		t.Fatal(err)
	}
	if withDecisionTable {
		// newMCPPlatformTestService provides the production authorization audit
		// table for normal regression cases.
	} else if err := db.Exec(`DROP TABLE mcp_authorization_decisions`).Error; err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateClientEndpoint(context.Background(), MCPClientEndpointCreateRequest{ClientKey: "authz-agent", DisplayName: "Authz Agent", ClientType: "service", ServerID: serverID}, "admin", "http://localhost:8084")
	if err != nil {
		t.Fatal(err)
	}
	return svc, upstream, created, &upstreamCalls, db
}

func TestMCPAuthorizationDenyDoesNotCallUpstream(t *testing.T) {
	svc, upstream, endpoint, calls, db := regressionRuntimeFixture(t, true)
	defer upstream.Close()
	svc.SetAuthorizationEvaluator(regressionEvaluator{decision: denyRegressionDecision()})
	_, err := svc.RuntimeCall(context.Background(), endpoint.Token, endpoint.ClientKey, "health", json.RawMessage(`{}`))
	if !errors.Is(err, ErrMCPPlatformAuthorizationDenied) {
		t.Fatalf("expected authorization deny, got %v", err)
	}
	if *calls != 0 {
		t.Fatalf("authorization deny reached upstream %d times", *calls)
	}
	var invocation model.MCPInvocation
	if err := db.Order("created_at DESC").First(&invocation).Error; err != nil {
		t.Fatalf("load denied invocation: %v", err)
	}
	if invocation.Status != "blocked" || invocation.PolicyDecision != "deny" {
		t.Fatalf("denied invocation audit state = status %q policy_decision %q, want blocked/deny", invocation.Status, invocation.PolicyDecision)
	}
	var audit model.MCPAuthorizationDecision
	if err := db.Where("invocation_id = ?", invocation.ID).First(&audit).Error; err != nil {
		t.Fatalf("load authorization decision: %v", err)
	}
	if audit.Outcome != "deny" {
		t.Fatalf("authorization decision outcome = %q, want deny", audit.Outcome)
	}
}

func TestMCPAuthorizationEvaluatorErrorAndTimeoutDoNotCallUpstream(t *testing.T) {
	for _, tc := range []struct {
		name      string
		evaluator regressionEvaluator
	}{
		{name: "error", evaluator: regressionEvaluator{err: errors.New("policy unavailable")}},
		{name: "timeout", evaluator: regressionEvaluator{wait: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, upstream, endpoint, calls, _ := regressionRuntimeFixture(t, true)
			defer upstream.Close()
			svc.SetAuthorizationTimeout(5 * time.Millisecond)
			svc.SetAuthorizationEvaluator(tc.evaluator)
			_, err := svc.RuntimeCall(context.Background(), endpoint.Token, endpoint.ClientKey, "health", json.RawMessage(`{}`))
			if !errors.Is(err, ErrMCPPlatformAuthorizationFailed) {
				t.Fatalf("expected evaluator failure, got %v", err)
			}
			if *calls != 0 {
				t.Fatalf("evaluator failure reached upstream %d times", *calls)
			}
		})
	}
}

func TestMCPAuthorizationAuditFailureDoesNotCallUpstream(t *testing.T) {
	svc, upstream, endpoint, calls, _ := regressionRuntimeFixture(t, false)
	defer upstream.Close()
	svc.SetAuthorizationEvaluator(regressionEvaluator{decision: allowRegressionDecision()})
	_, err := svc.RuntimeCall(context.Background(), endpoint.Token, endpoint.ClientKey, "health", json.RawMessage(`{}`))
	if !errors.Is(err, ErrMCPPlatformAuthorizationFailed) {
		t.Fatalf("expected audit failure, got %v", err)
	}
	if *calls != 0 {
		t.Fatalf("audit failure reached upstream %d times", *calls)
	}
}

func TestMCPAuthorizationAllowReachesUpstreamOnce(t *testing.T) {
	svc, upstream, endpoint, calls, _ := regressionRuntimeFixture(t, true)
	defer upstream.Close()
	svc.SetAuthorizationEvaluator(regressionEvaluator{decision: allowRegressionDecision()})
	result, err := svc.RuntimeCall(context.Background(), endpoint.Token, endpoint.ClientKey, "health", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result) == 0 || *calls != 1 {
		t.Fatalf("expected one successful upstream call, result=%#v calls=%d", result, *calls)
	}
}

func TestMCPRegoPublicationKeepsRuntimeToolsVisibleAndAuthorizesCall(t *testing.T) {
	svc, upstream, endpoint, calls, db := regressionRuntimeFixture(t, true)
	defer upstream.Close()
	createRegressionPolicyTables(t, db)

	const source = `package aegis.mcp.authz
import rego.v1

default allow := true
deny_rule_ids := []
reason_code := "POLICY_ALLOWED" if allow
decision := {
  "contract_version": "aegis.mcp.authz.decision.v1",
  "allow": allow,
  "reason_code": reason_code,
  "deny_rule_ids": deny_rule_ids,
  "audit_rule_ids": [],
  "policy_revision": data.aegis_config.meta.revision,
}`
	if _, err := svc.ConfigureAuthorizationPolicy(context.Background(), mcpauthz.Policy{Revision: "rego-runtime-r1", RegoSource: source}, "admin"); err != nil {
		t.Fatal(err)
	}

	tools, err := svc.RuntimeTools(context.Background(), endpoint.Token, endpoint.ClientKey)
	if err != nil || len(tools) != 1 || tools[0].Name != "health" {
		t.Fatalf("published Rego policy must keep grant tools visible, tools=%#v err=%v", tools, err)
	}
	if _, err := svc.RuntimeCall(context.Background(), endpoint.Token, endpoint.ClientKey, "health", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("published Rego policy must authorize the runtime call: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("expected exactly one upstream call, got %d", *calls)
	}
}

func createRegressionPolicyTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, statement := range []string{
		`CREATE TABLE mcp_policy_sets (id text primary key, policy_key text unique, display_name text, status text, created_by text, created_at datetime, updated_at datetime)`,
		`CREATE TABLE mcp_policy_versions (id text primary key, policy_set_id text, version integer, language_version text, source text, compiled_bundle_ref text, digest text, test_report text, status text, signature text, signing_key_id text, created_by text, created_at datetime)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestMCPAuthorizationPolicyActivationSurvivesEndpointAndRestart(t *testing.T) {
	svc, upstream, first, calls, db := regressionRuntimeFixture(t, true)
	defer upstream.Close()
	createRegressionPolicyTables(t, db)
	var releaseTool model.MCPCatalogReleaseTool
	if err := db.First(&releaseTool, "exposed_name = ?", "health").Error; err != nil {
		t.Fatal(err)
	}
	policy := mcpauthz.Policy{Revision: "strict-r1", Permissions: map[string]map[string]mcpauthz.Permission{
		first.ClientID.String(): {releaseTool.ID.String(): {CatalogReleaseID: releaseTool.ReleaseID.String(), ToolRevisionID: releaseTool.ToolRevisionID.String()}},
	}}
	if _, err := svc.ConfigureAuthorizationPolicy(context.Background(), policy, "admin"); err != nil {
		t.Fatal(err)
	}

	second, err := svc.CreateClientEndpoint(context.Background(), MCPClientEndpointCreateRequest{ClientKey: "strict-agent", DisplayName: "Strict Agent", ClientType: "service", ServerID: mustServerID(t, db)}, "admin", "http://localhost:8084")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RuntimeCall(context.Background(), second.Token, second.ClientKey, "health", json.RawMessage(`{}`)); !errors.Is(err, ErrMCPPlatformAuthorizationDenied) {
		t.Fatalf("new endpoint must not inherit compatibility permit under strict policy, got %v", err)
	}
	if *calls != 0 {
		t.Fatalf("strict policy denial reached upstream: %d", *calls)
	}

	restarted := NewMCPPlatformService(svc.repo, svc.logger)
	if err := restarted.LoadAuthorizationPolicy(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.RuntimeCall(context.Background(), second.Token, second.ClientKey, "health", json.RawMessage(`{}`)); !errors.Is(err, ErrMCPPlatformAuthorizationDenied) {
		t.Fatalf("restarted service must retain strict deny, got %v", err)
	}

	if _, err := restarted.ConfigureAuthorizationPolicy(context.Background(), mcpauthz.Policy{Revision: "bad", Permissions: map[string]map[string]mcpauthz.Permission{"x": {"y": {MaxLimit: -1}}}}, "admin"); err == nil {
		t.Fatal("invalid policy publication must fail")
	}
	if _, err := restarted.RuntimeCall(context.Background(), second.Token, second.ClientKey, "health", json.RawMessage(`{}`)); !errors.Is(err, ErrMCPPlatformAuthorizationDenied) {
		t.Fatalf("invalid publication must preserve active deny policy, got %v", err)
	}
}

func TestMCPAuthorizationFailedPublicationPreservesPreviousVersion(t *testing.T) {
	svc, upstream, endpoint, calls, db := regressionRuntimeFixture(t, true)
	defer upstream.Close()
	createRegressionPolicyTables(t, db)
	var releaseTool model.MCPCatalogReleaseTool
	if err := db.First(&releaseTool, "exposed_name = ?", "health").Error; err != nil {
		t.Fatal(err)
	}
	policy := mcpauthz.Policy{Revision: "stable-r1", Permissions: map[string]map[string]mcpauthz.Permission{
		endpoint.ClientID.String(): {releaseTool.ID.String(): {CatalogReleaseID: releaseTool.ReleaseID.String(), ToolRevisionID: releaseTool.ToolRevisionID.String()}},
	}}
	if _, err := svc.ConfigureAuthorizationPolicy(context.Background(), policy, "admin"); err != nil {
		t.Fatal(err)
	}
	var before model.MCPPolicyVersion
	if err := db.Where("version = ?", 1).First(&before).Error; err != nil {
		t.Fatal(err)
	}
	if before.Status != "active" || before.LanguageVersion == "" {
		t.Fatalf("expected first policy version active, got %#v", before)
	}
	if err := db.Exec(`CREATE TRIGGER fail_mcp_policy_version_insert BEFORE INSERT ON mcp_policy_versions BEGIN SELECT RAISE(ABORT, 'forced publication failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	failedPolicy := policy
	failedPolicy.Revision = "should-not-activate"
	if _, err := svc.ConfigureAuthorizationPolicy(context.Background(), failedPolicy, "admin"); err == nil {
		t.Fatal("expected policy publication failure")
	}
	var after model.MCPPolicyVersion
	if err := db.Where("version = ?", 1).First(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after.Status != "active" || after.ID != before.ID {
		t.Fatalf("failed publication changed previous active version: before=%#v after=%#v", before, after)
	}
	if err := db.Where("version = ?", 2).First(&model.MCPPolicyVersion{}).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("failed publication left a new policy version: %v", err)
	}
	if _, err := svc.RuntimeCall(context.Background(), endpoint.Token, endpoint.ClientKey, "health", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("previous active policy stopped allowing after failed publication: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("expected exactly one upstream call under preserved policy, got %d", *calls)
	}
}

func TestMCPAuthorizationPolicyStatusReturnsEmptyAndActiveProjection(t *testing.T) {
	svc, _, _, _, db := regressionRuntimeFixture(t, true)
	createRegressionPolicyTables(t, db)
	status, err := svc.GetAuthorizationPolicyStatus(context.Background())
	if err != nil || status.Status != "empty" || status.Policy != nil {
		t.Fatalf("expected safe empty status, status=%#v err=%v", status, err)
	}
	policy := mcpauthz.Policy{Revision: "ui-r1", Permissions: map[string]map[string]mcpauthz.Permission{}}
	if _, err := svc.ConfigureAuthorizationPolicy(context.Background(), policy, "admin"); err != nil {
		t.Fatal(err)
	}
	status, err = svc.GetAuthorizationPolicyStatus(context.Background())
	if err != nil || status.Status != "active" || status.Version != 1 || status.Policy == nil || status.Policy.Revision != policy.Revision {
		t.Fatalf("expected active controlled projection, status=%#v err=%v", status, err)
	}
}

func mustServerID(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	var server model.MCPServer
	if err := db.First(&server, "server_key = ?", "authz-regression").Error; err != nil {
		t.Fatal(err)
	}
	return server.ID
}
