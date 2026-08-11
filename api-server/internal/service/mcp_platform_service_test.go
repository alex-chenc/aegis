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

	"api-server/internal/model"
	"api-server/internal/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newMCPPlatformTestService(t *testing.T) (*MCPPlatformService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:mcp-platform-"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE mcp_servers (id text primary key, server_key text, display_name text, owner_team_id text, owner_user_id text, environment text, transport text, endpoint_url text, endpoint_display text, credential_ref text, auth_type text, protocol_version text, risk_tier text, lifecycle_status text, active_revision_id text, tool_count integer, published_tool_count integer, last_health_status text, last_error_code text, last_error_message text, last_synced_at datetime, created_by text, updated_by text, created_at datetime, updated_at datetime)`,
		`CREATE TABLE mcp_onboarding_jobs (id text primary key, idempotency_key text unique, server_id text, display_name text, endpoint_url text, endpoint_display text, credential_ref text, auth_type text, owner_team_id text, owner_user_id text, environment text, target_catalog_id text, publish_policy text, status text, step text, attempt integer, error_code text, error_message text, revision_id text, created_by text, created_at datetime, updated_at datetime, completed_at datetime)`,
		`CREATE TABLE mcp_server_revisions (id text primary key, server_id text, revision_no integer, protocol_version text, capabilities text, tools_snapshot text, digest text, status text, discovery_error text, created_by text, created_at datetime)`,
		`CREATE TABLE mcp_tool_revisions (id text primary key, server_revision_id text, upstream_name text, alias text, title text, description text, input_schema text, output_schema text, verified_metadata text, risk_tier text, status text, created_at datetime)`,
		`CREATE TABLE mcp_catalogs (id text primary key, catalog_key text, display_name text, status text, created_by text, created_at datetime, updated_at datetime)`,
		`CREATE TABLE mcp_catalog_releases (id text primary key, catalog_id text, server_revision_id text, release_no integer, manifest text, manifest_digest text, status text, created_by text, created_at datetime, published_at datetime)`,
		`CREATE TABLE mcp_catalog_release_tools (id text primary key, release_id text, tool_revision_id text, exposed_name text, title text, description text, input_schema text, output_schema text, approval_mode text, rate_limit text, resource text, status text, display_order integer, created_at datetime)`,
		`CREATE TABLE mcp_clients (id text primary key, client_key text, display_name text, client_type text, status text, created_by text, created_at datetime, updated_at datetime)`,
		`CREATE TABLE mcp_client_grants (id text primary key, client_id text, catalog_id text, tool_allowlist text, resource_scope text, status text, expires_at datetime, created_by text, created_at datetime, updated_at datetime)`,
		`CREATE TABLE mcp_client_credentials (id text primary key, client_id text, token_prefix text, token_hash text, status text, expires_at datetime, last_used_at datetime, created_by text, created_at datetime, updated_at datetime)`,
		`CREATE TABLE mcp_approval_requests (id text primary key, approval_type text, subject_type text, subject_id text, requested_by text, status text, request_digest text, reason text, decision_reason text, decided_by text, created_at datetime, decided_at datetime)`,
		`CREATE TABLE mcp_invocations (id text primary key, client_id text, catalog_release_id text, tool_revision_id text, user_id text, tool_alias text, status text, policy_decision text, rule_status text, ai_status text, request_digest text, result_digest text, created_at datetime, completed_at datetime)`,
		`CREATE TABLE mcp_security_verdicts (id text primary key, invocation_id text, deterministic_severity text, ai_verdict text, overall_risk text, evidence text, updated_at datetime)`,
		`CREATE TABLE mcp_rule_definitions (id text primary key, rule_key text, version integer, name text, phase text, severity text, definition text, digest text, enabled boolean, created_at datetime)`,
		`CREATE TABLE mcp_rule_hits (id text primary key, invocation_id text, rule_definition_id text, severity text, phase text, evidence text, created_at datetime)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := NewMCPPlatformService(repository.NewMCPPlatformRepository(db), zap.NewNop())
	svc.client = &http.Client{Timeout: 2 * time.Second}
	return svc, db
}

func seedMCPSecurityRules(t *testing.T, db *gorm.DB) {
	t.Helper()
	now := time.Now().UTC()
	for _, rule := range []model.MCPRuleDefinition{
		{ID: uuid.New(), RuleKey: "block_l4_tool_call", Version: 1, Name: "Block L4", Phase: "pre", Severity: "critical", Definition: []byte(`{"matcher":"tool_risk_at_least","threshold":"l4","action":"block"}`), Digest: "l4", Enabled: true, CreatedAt: now},
		{ID: uuid.New(), RuleKey: "block_sensitive_output_keys", Version: 1, Name: "Sensitive output", Phase: "post", Severity: "critical", Definition: []byte(`{"matcher":"sensitive_output_keys","keys":["token","password"],"action":"block"}`), Digest: "sensitive", Enabled: true, CreatedAt: now},
		{ID: uuid.New(), RuleKey: "audit_upstream_failure", Version: 1, Name: "Upstream failure", Phase: "post", Severity: "medium", Definition: []byte(`{"matcher":"call_failed","action":"audit"}`), Digest: "failed", Enabled: true, CreatedAt: now},
		{ID: uuid.New(), RuleKey: "block_injection_input", Version: 1, Name: "Injection input", Phase: "pre", Severity: "critical", Definition: []byte(`{"matcher":"input_patterns","patterns":["../"," union select "],"action":"block"}`), Digest: "injection", Enabled: true, CreatedAt: now},
	} {
		if err := db.Create(&rule).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestMCPServerListHidesRetiredByDefault(t *testing.T) {
	svc, db := newMCPPlatformTestService(t)
	now := time.Now().UTC()
	activeID, retiredID := uuid.New(), uuid.New()
	for _, server := range []model.MCPServer{
		{ID: activeID, ServerKey: "active", DisplayName: "Active MCP", OwnerUserID: "admin", Environment: "dev", Transport: model.MCPPlatformTransportStreamableHTTP, EndpointURL: "https://active.example/mcp", EndpointDisplay: "https://active.example/mcp", AuthType: model.MCPPlatformAuthNone, RiskTier: model.MCPPlatformRiskL1, LifecycleStatus: model.MCPPlatformServerPublished, CreatedBy: "admin", UpdatedBy: "admin", CreatedAt: now, UpdatedAt: now},
		{ID: retiredID, ServerKey: "retired", DisplayName: "Retired MCP", OwnerUserID: "admin", Environment: "dev", Transport: model.MCPPlatformTransportStreamableHTTP, EndpointURL: "https://retired.example/mcp", EndpointDisplay: "https://retired.example/mcp", AuthType: model.MCPPlatformAuthNone, RiskTier: model.MCPPlatformRiskL1, LifecycleStatus: model.MCPPlatformServerRetired, CreatedBy: "admin", UpdatedBy: "admin", CreatedAt: now.Add(-time.Minute), UpdatedAt: now},
	} {
		if err := db.Create(&server).Error; err != nil {
			t.Fatal(err)
		}
	}

	items, total, err := svc.ListServers(context.Background(), repository.MCPServerQuery{Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != activeID {
		t.Fatalf("expected default server list to hide retired records, items=%#v total=%d err=%v", items, total, err)
	}

	items, total, err = svc.ListServers(context.Background(), repository.MCPServerQuery{Status: model.MCPPlatformServerRetired, Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != retiredID {
		t.Fatalf("expected explicit retired filter to remain available for audit, items=%#v total=%d err=%v", items, total, err)
	}
}

func TestMCPToolListHidesToolsForRetiredServer(t *testing.T) {
	svc, db := newMCPPlatformTestService(t)
	now := time.Now().UTC()
	activeServerID, retiredServerID := uuid.New(), uuid.New()
	activeRevisionID, retiredRevisionID := uuid.New(), uuid.New()
	for _, server := range []model.MCPServer{
		{ID: activeServerID, ServerKey: "active-tools", DisplayName: "Active Tools MCP", OwnerUserID: "admin", Environment: "dev", Transport: model.MCPPlatformTransportStreamableHTTP, EndpointURL: "https://active.example/mcp", EndpointDisplay: "https://active.example/mcp", AuthType: model.MCPPlatformAuthNone, RiskTier: model.MCPPlatformRiskL1, LifecycleStatus: model.MCPPlatformServerPublished, CreatedBy: "admin", UpdatedBy: "admin", CreatedAt: now, UpdatedAt: now},
		{ID: retiredServerID, ServerKey: "retired-tools", DisplayName: "Retired Tools MCP", OwnerUserID: "admin", Environment: "dev", Transport: model.MCPPlatformTransportStreamableHTTP, EndpointURL: "https://retired.example/mcp", EndpointDisplay: "https://retired.example/mcp", AuthType: model.MCPPlatformAuthNone, RiskTier: model.MCPPlatformRiskL1, LifecycleStatus: model.MCPPlatformServerRetired, CreatedBy: "admin", UpdatedBy: "admin", CreatedAt: now, UpdatedAt: now},
	} {
		if err := db.Create(&server).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, revision := range []model.MCPServerRevision{
		{ID: activeRevisionID, ServerID: activeServerID, RevisionNo: 1, ProtocolVersion: "2025-11-25", ToolsSnapshot: []byte(`[]`), Digest: "active-tools", Status: model.MCPPlatformServerApproved, CreatedBy: "admin", CreatedAt: now},
		{ID: retiredRevisionID, ServerID: retiredServerID, RevisionNo: 1, ProtocolVersion: "2025-11-25", ToolsSnapshot: []byte(`[]`), Digest: "retired-tools", Status: model.MCPPlatformServerApproved, CreatedBy: "admin", CreatedAt: now},
	} {
		if err := db.Create(&revision).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, tool := range []model.MCPToolRevision{
		{ID: uuid.New(), ServerRevisionID: activeRevisionID, UpstreamName: "active_tool", Alias: "active_tool", InputSchema: []byte(`{"type":"object"}`), OutputSchema: []byte(`{}`), RiskTier: model.MCPPlatformRiskL1, Status: "approved", CreatedAt: now},
		{ID: uuid.New(), ServerRevisionID: retiredRevisionID, UpstreamName: "retired_tool", Alias: "retired_tool", InputSchema: []byte(`{"type":"object"}`), OutputSchema: []byte(`{}`), RiskTier: model.MCPPlatformRiskL1, Status: "approved", CreatedAt: now},
	} {
		if err := db.Create(&tool).Error; err != nil {
			t.Fatal(err)
		}
	}

	items, total, err := svc.ListTools(context.Background(), nil, 1, 10)
	if err != nil || total != 1 || len(items) != 1 || items[0].Alias != "active_tool" {
		t.Fatalf("expected default tool list to hide retired service tools, items=%#v total=%d err=%v", items, total, err)
	}
	items, total, err = svc.ListTools(context.Background(), &retiredRevisionID, 1, 10)
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("expected retired service tools to stay hidden for explicit revision query, items=%#v total=%d err=%v", items, total, err)
	}
}

func TestMCPClientEndpointBindsOneServerAndFiltersTools(t *testing.T) {
	svc, db := newMCPPlatformTestService(t)
	now := time.Now().UTC()
	serverID, revisionID := uuid.New(), uuid.New()
	if err := db.Create(&model.MCPServer{ID: serverID, ServerKey: "local", DisplayName: "Aegis Local MCP", OwnerUserID: "admin", Environment: "dev", Transport: model.MCPPlatformTransportStreamableHTTP, EndpointURL: "http://127.0.0.1:1/mcp", EndpointDisplay: "http://127.0.0.1:1/mcp", AuthType: model.MCPPlatformAuthNone, RiskTier: model.MCPPlatformRiskL2, LifecycleStatus: model.MCPPlatformServerPublished, ActiveRevisionID: &revisionID, ToolCount: 2, PublishedToolCount: 2, CreatedBy: "admin", UpdatedBy: "admin", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.MCPServerRevision{ID: revisionID, ServerID: serverID, RevisionNo: 1, ProtocolVersion: "2025-11-25", ToolsSnapshot: []byte(`[]`), Digest: "digest", Status: model.MCPPlatformServerApproved, CreatedBy: "admin", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	for _, tool := range []model.MCPToolRevision{
		{ID: uuid.New(), ServerRevisionID: revisionID, UpstreamName: "health", Alias: "get_aegis_health", Title: "Health", Description: "Health check", InputSchema: []byte(`{"type":"object"}`), OutputSchema: []byte(`{}`), RiskTier: model.MCPPlatformRiskL1, Status: "approved", CreatedAt: now},
		{ID: uuid.New(), ServerRevisionID: revisionID, UpstreamName: "hosts", Alias: "list_hosts", Title: "Hosts", Description: "List hosts", InputSchema: []byte(`{"type":"object"}`), OutputSchema: []byte(`{}`), RiskTier: model.MCPPlatformRiskL2, Status: "approved", CreatedAt: now},
	} {
		if err := db.Create(&tool).Error; err != nil {
			t.Fatal(err)
		}
	}
	created, err := svc.CreateClientEndpoint(context.Background(), MCPClientEndpointCreateRequest{ClientKey: "codex-aegis", DisplayName: "Codex", ClientType: "service", ServerID: serverID}, "admin", "http://localhost:8084")
	if err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || !strings.HasSuffix(created.Endpoint, "/mcp/v1/clients/codex-aegis") || len(created.Tools) != 2 {
		t.Fatalf("unexpected endpoint response: %#v", created)
	}
	tools, err := svc.RuntimeTools(context.Background(), created.Token, "codex-aegis")
	if err != nil || len(tools) != 2 {
		t.Fatalf("expected every published service tool to be enabled on creation, tools=%#v err=%v", tools, err)
	}
	if _, err := svc.RuntimeTools(context.Background(), created.Token, "another-client"); !errors.Is(err, ErrMCPPlatformClientEndpointDenied) {
		t.Fatalf("expected endpoint identity mismatch to be denied, got %v", err)
	}
	if _, err := svc.UpdateClientEndpointTools(context.Background(), created.GrantID, []string{"list_hosts"}, "admin", "http://localhost:8084"); err != nil {
		t.Fatal(err)
	}
	tools, err = svc.RuntimeTools(context.Background(), created.Token, "codex-aegis")
	if err != nil || len(tools) != 1 || tools[0].Name != "list_hosts" {
		t.Fatalf("expected Client grant to filter runtime tools, tools=%#v err=%v", tools, err)
	}
	if _, err := svc.RuntimeCall(context.Background(), created.Token, "codex-aegis", "get_aegis_health", json.RawMessage(`{}`)); !errors.Is(err, ErrMCPPlatformToolNotAllowed) {
		t.Fatalf("expected a disabled tool call to fail before upstream dispatch, got %v", err)
	}

	var listHosts model.MCPToolRevision
	if err := db.Where("server_revision_id = ? AND alias = ?", revisionID, "list_hosts").First(&listHosts).Error; err != nil {
		t.Fatal(err)
	}
	invocation := &model.MCPInvocation{ID: uuid.New(), ClientID: &created.ClientID, ToolRevisionID: &listHosts.ID, UserID: created.ClientKey, ToolAlias: listHosts.Alias, Status: "succeeded", PolicyDecision: "allow", CreatedAt: now}
	if err := db.Create(invocation).Error; err != nil {
		t.Fatal(err)
	}
	auditItems, total, err := svc.ListInvocations(context.Background(), 1, 100)
	if err != nil || total != 1 || len(auditItems) != 1 {
		t.Fatalf("expected one enriched invocation, items=%#v total=%d err=%v", auditItems, total, err)
	}
	if auditItems[0].ServerID != serverID || auditItems[0].ServerName != "Aegis Local MCP" || auditItems[0].ClientKey != "codex-aegis" || !auditItems[0].ToolEnabled {
		t.Fatalf("expected service, Client and grant state on audit item: %#v", auditItems[0])
	}
	disabled, err := svc.DisableInvocationTool(context.Background(), invocation.ID, "admin")
	if err != nil || !disabled.Disabled || !disabled.Changed {
		t.Fatalf("expected audit action to disable the Client tool: %#v err=%v", disabled, err)
	}
	if _, err := svc.RuntimeCall(context.Background(), created.Token, "codex-aegis", "list_hosts", json.RawMessage(`{}`)); !errors.Is(err, ErrMCPPlatformToolNotAllowed) {
		t.Fatalf("expected audit-disabled tool call to fail before upstream dispatch, got %v", err)
	}
	disabled, err = svc.DisableInvocationTool(context.Background(), invocation.ID, "admin")
	if err != nil || !disabled.Disabled || disabled.Changed {
		t.Fatalf("expected repeated disable to be idempotent: %#v err=%v", disabled, err)
	}
}

func TestMCPDeletesRevokeAuthorizationAndRetireServer(t *testing.T) {
	svc, db := newMCPPlatformTestService(t)
	now := time.Now().UTC()
	serverID, revisionID := uuid.New(), uuid.New()
	if err := db.Create(&model.MCPServer{ID: serverID, ServerKey: "retire-me", DisplayName: "Retire Me", OwnerUserID: "admin", Environment: "dev", Transport: model.MCPPlatformTransportStreamableHTTP, EndpointURL: "http://127.0.0.1:1/mcp", EndpointDisplay: "http://127.0.0.1:1/mcp", AuthType: model.MCPPlatformAuthNone, RiskTier: model.MCPPlatformRiskL2, LifecycleStatus: model.MCPPlatformServerPublished, ActiveRevisionID: &revisionID, CreatedBy: "admin", UpdatedBy: "admin", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.MCPServerRevision{ID: revisionID, ServerID: serverID, RevisionNo: 1, ProtocolVersion: "2025-11-25", ToolsSnapshot: []byte(`[]`), Digest: "retire-digest", Status: model.MCPPlatformServerApproved, CreatedBy: "admin", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	first, err := svc.CreateClientEndpoint(context.Background(), MCPClientEndpointCreateRequest{ClientKey: "revoke-me", DisplayName: "Revoke Me", ClientType: "service", ServerID: serverID}, "admin", "http://localhost:8084")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RevokeClientEndpoint(context.Background(), first.ClientID, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RuntimeTools(context.Background(), first.Token, first.ClientKey); !errors.Is(err, ErrMCPPlatformClientEndpointDenied) {
		t.Fatalf("expected revoked Client token to be denied, got %v", err)
	}
	var revokedClient model.MCPClient
	if err := db.First(&revokedClient, "id = ?", first.ClientID).Error; err != nil || revokedClient.Status != "revoked" {
		t.Fatalf("expected Client to be revoked, client=%#v err=%v", revokedClient, err)
	}
	second, err := svc.CreateClientEndpoint(context.Background(), MCPClientEndpointCreateRequest{ClientKey: "retire-bound", DisplayName: "Retire Bound", ClientType: "service", ServerID: serverID}, "admin", "http://localhost:8084")
	if err != nil {
		t.Fatal(err)
	}
	retired, err := svc.RetireServer(context.Background(), serverID, "admin")
	if err != nil || retired.LifecycleStatus != model.MCPPlatformServerRetired {
		t.Fatalf("expected server to be retired, server=%#v err=%v", retired, err)
	}
	if _, err := svc.RuntimeTools(context.Background(), second.Token, second.ClientKey); !errors.Is(err, ErrMCPPlatformClientEndpointDenied) {
		t.Fatalf("expected Client bound to retired server to be denied, got %v", err)
	}
	var revokedGrant model.MCPClientGrant
	if err := db.First(&revokedGrant, "id = ?", second.GrantID).Error; err != nil || revokedGrant.Status != "revoked" {
		t.Fatalf("expected server retirement to revoke grant, grant=%#v err=%v", revokedGrant, err)
	}
}

func TestMCPRuntimeCreatesSafeVerdictAndBlocksSensitiveResult(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"content":[{"type":"text","text":"ok"}]}}`))
	}))
	defer upstream.Close()

	svc, db := newMCPPlatformTestService(t)
	seedMCPSecurityRules(t, db)
	now := time.Now().UTC()
	serverID, revisionID := uuid.New(), uuid.New()
	serverURL := strings.Replace(upstream.URL, "127.0.0.1", "localhost", 1)
	if err := db.Create(&model.MCPServer{ID: serverID, ServerKey: "secure-local", DisplayName: "Secure MCP", OwnerUserID: "admin", Environment: "dev", Transport: model.MCPPlatformTransportStreamableHTTP, EndpointURL: serverURL, EndpointDisplay: serverURL, AuthType: model.MCPPlatformAuthNone, RiskTier: model.MCPPlatformRiskL2, LifecycleStatus: model.MCPPlatformServerPublished, ActiveRevisionID: &revisionID, ToolCount: 1, PublishedToolCount: 1, CreatedBy: "admin", UpdatedBy: "admin", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.MCPServerRevision{ID: revisionID, ServerID: serverID, RevisionNo: 1, ProtocolVersion: "2025-11-25", ToolsSnapshot: []byte(`[]`), Digest: "digest", Status: model.MCPPlatformServerApproved, CreatedBy: "admin", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	tool := model.MCPToolRevision{ID: uuid.New(), ServerRevisionID: revisionID, UpstreamName: "safe", Alias: "safe_tool", Title: "Safe", InputSchema: []byte(`{"type":"object"}`), OutputSchema: []byte(`{}`), RiskTier: model.MCPPlatformRiskL1, Status: "approved", CreatedAt: now}
	if err := db.Create(&tool).Error; err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateClientEndpoint(context.Background(), MCPClientEndpointCreateRequest{ClientKey: "secure-agent", DisplayName: "Secure Agent", ClientType: "service", ServerID: serverID}, "admin", "http://localhost:8084")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RuntimeCall(context.Background(), created.Token, created.ClientKey, tool.Alias, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	var verdict model.MCPSecurityVerdict
	if err := db.Order("updated_at DESC").First(&verdict).Error; err != nil {
		t.Fatal(err)
	}
	if verdict.DeterministicSeverity != "low" || verdict.OverallRisk != "low" || verdict.AIVerdict != "not_run" {
		t.Fatalf("unexpected safe verdict: %#v", verdict)
	}

	upstream.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"access_token":"must-not-reach-client"}}`))
	})
	if _, err := svc.RuntimeCall(context.Background(), created.Token, created.ClientKey, tool.Alias, json.RawMessage(`{}`)); !errors.Is(err, ErrMCPPlatformSecurityBlocked) {
		t.Fatalf("expected sensitive upstream result to be blocked, got %v", err)
	}
	if upstreamCalls != 2 {
		t.Fatalf("expected exactly two upstream calls, got %d", upstreamCalls)
	}
	verdict = model.MCPSecurityVerdict{}
	if err := db.Order("updated_at DESC").First(&verdict).Error; err != nil {
		t.Fatal(err)
	}
	if verdict.OverallRisk != "critical" {
		t.Fatalf("expected critical verdict for sensitive result: %#v", verdict)
	}
	securityItems, securityTotal, err := svc.ListSecurityVerdicts(context.Background(), 1, 10)
	if err != nil || securityTotal != 2 || len(securityItems) != 2 {
		t.Fatalf("expected two enriched security verdicts, items=%#v total=%d err=%v", securityItems, securityTotal, err)
	}
	matchedSensitiveRule := false
	for _, item := range securityItems {
		if item.InvocationID == verdict.InvocationID {
			for _, ruleName := range item.MatchedRules {
				if ruleName == "Sensitive output" {
					matchedSensitiveRule = true
				}
			}
		}
	}
	if !matchedSensitiveRule {
		t.Fatalf("expected verdict to expose matched rule name: %#v", securityItems)
	}
	var hitCount int64
	if err := db.Model(&model.MCPRuleHit{}).Count(&hitCount).Error; err != nil || hitCount != 1 {
		t.Fatalf("expected one rule hit, count=%d err=%v", hitCount, err)
	}
}

func TestMCPRuntimeBlocksL4BeforeUpstreamAndRulesCanBeDisabled(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{}}`))
	}))
	defer upstream.Close()
	svc, db := newMCPPlatformTestService(t)
	seedMCPSecurityRules(t, db)
	now := time.Now().UTC()
	serverID, revisionID := uuid.New(), uuid.New()
	serverURL := strings.Replace(upstream.URL, "127.0.0.1", "localhost", 1)
	if err := db.Create(&model.MCPServer{ID: serverID, ServerKey: "danger", DisplayName: "Danger MCP", OwnerUserID: "admin", Environment: "dev", Transport: model.MCPPlatformTransportStreamableHTTP, EndpointURL: serverURL, EndpointDisplay: serverURL, AuthType: model.MCPPlatformAuthNone, RiskTier: model.MCPPlatformRiskL4, LifecycleStatus: model.MCPPlatformServerPublished, ActiveRevisionID: &revisionID, ToolCount: 1, PublishedToolCount: 1, CreatedBy: "admin", UpdatedBy: "admin", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.MCPServerRevision{ID: revisionID, ServerID: serverID, RevisionNo: 1, ProtocolVersion: "2025-11-25", ToolsSnapshot: []byte(`[]`), Digest: "digest", Status: model.MCPPlatformServerApproved, CreatedBy: "admin", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	tool := model.MCPToolRevision{ID: uuid.New(), ServerRevisionID: revisionID, UpstreamName: "danger", Alias: "danger_tool", InputSchema: []byte(`{"type":"object"}`), OutputSchema: []byte(`{}`), RiskTier: model.MCPPlatformRiskL4, Status: "approved", CreatedAt: now}
	if err := db.Create(&tool).Error; err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateClientEndpoint(context.Background(), MCPClientEndpointCreateRequest{ClientKey: "danger-agent", DisplayName: "Danger Agent", ClientType: "service", ServerID: serverID}, "admin", "http://localhost:8084")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RuntimeCall(context.Background(), created.Token, created.ClientKey, tool.Alias, json.RawMessage(`{}`)); !errors.Is(err, ErrMCPPlatformSecurityBlocked) {
		t.Fatalf("expected L4 call to be blocked, got %v", err)
	}
	if upstreamCalls != 0 {
		t.Fatalf("pre-call security rule reached upstream %d times", upstreamCalls)
	}
	rules, total, err := svc.ListSecurityRules(context.Background(), 1, 10)
	if err != nil || total != 4 || len(rules) != 4 {
		t.Fatalf("unexpected rules page items=%#v total=%d err=%v", rules, total, err)
	}
	var l4Rule model.MCPRuleDefinition
	if err := db.Where("rule_key = ?", "block_l4_tool_call").First(&l4Rule).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetSecurityRuleEnabled(context.Background(), l4Rule.ID, false, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RuntimeCall(context.Background(), created.Token, created.ClientKey, tool.Alias, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("disabled rule should no longer block: %v", err)
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected one upstream call after rule disable, got %d", upstreamCalls)
	}
}

func TestMCPRuntimeBlocksInjectionInputBeforeUpstream(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{}}`))
	}))
	defer upstream.Close()
	svc, db := newMCPPlatformTestService(t)
	seedMCPSecurityRules(t, db)
	now := time.Now().UTC()
	serverID, revisionID := uuid.New(), uuid.New()
	serverURL := strings.Replace(upstream.URL, "127.0.0.1", "localhost", 1)
	server := model.MCPServer{ID: serverID, ServerKey: "input-protection", DisplayName: "Protected MCP", OwnerUserID: "admin", Environment: "dev", Transport: model.MCPPlatformTransportStreamableHTTP, EndpointURL: serverURL, EndpointDisplay: serverURL, AuthType: model.MCPPlatformAuthNone, RiskTier: model.MCPPlatformRiskL2, LifecycleStatus: model.MCPPlatformServerPublished, ActiveRevisionID: &revisionID, ToolCount: 1, PublishedToolCount: 1, CreatedBy: "admin", UpdatedBy: "admin", CreatedAt: now, UpdatedAt: now}
	revision := model.MCPServerRevision{ID: revisionID, ServerID: serverID, RevisionNo: 1, ProtocolVersion: "2025-11-25", ToolsSnapshot: []byte(`[]`), Digest: "digest", Status: model.MCPPlatformServerApproved, CreatedBy: "admin", CreatedAt: now}
	tool := model.MCPToolRevision{ID: uuid.New(), ServerRevisionID: revisionID, UpstreamName: "read", Alias: "read_file", InputSchema: []byte(`{"type":"object"}`), OutputSchema: []byte(`{}`), RiskTier: model.MCPPlatformRiskL2, Status: "approved", CreatedAt: now}
	if err := db.Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&revision).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&tool).Error; err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateClientEndpoint(context.Background(), MCPClientEndpointCreateRequest{ClientKey: "protected-agent", DisplayName: "Protected Agent", ClientType: "service", ServerID: serverID}, "admin", "http://localhost:8084")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RuntimeCall(context.Background(), created.Token, created.ClientKey, tool.Alias, json.RawMessage(`{"path":"../../etc/passwd"}`)); !errors.Is(err, ErrMCPPlatformSecurityBlocked) {
		t.Fatalf("expected traversal input to be blocked, got %v", err)
	}
	if upstreamCalls != 0 {
		t.Fatalf("blocked input reached upstream %d times", upstreamCalls)
	}
}

func TestMCPOnboardingCreatesReleaseAndApproval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		result := map[string]interface{}{}
		if request["method"] == "initialize" {
			result = map[string]interface{}{"protocolVersion": "2025-11-25", "capabilities": map[string]interface{}{}}
		} else if request["method"] == "tools/list" {
			result = map[string]interface{}{"tools": []interface{}{map[string]interface{}{"name": "read", "description": "read-only", "inputSchema": map[string]interface{}{"type": "object"}}}}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": request["id"], "result": result})
	}))
	defer server.Close()

	svc, db := newMCPPlatformTestService(t)
	catalog := &model.MCPCatalog{ID: uuid.New(), CatalogKey: "internal", DisplayName: "Internal", Status: "active", CreatedBy: "owner", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Create(catalog).Error; err != nil {
		t.Fatal(err)
	}
	endpoint := strings.Replace(server.URL, "127.0.0.1", "localhost", 1) + "/mcp"
	request := MCPOnboardingRequest{DisplayName: "L2 remote", EndpointURL: endpoint, AuthType: model.MCPPlatformAuthNone, Environment: "dev", TargetCatalogID: &catalog.ID, PublishPolicy: "manual"}
	job, err := svc.CreateOnboardingJob(context.Background(), request, "idem-release", "owner")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, getErr := svc.GetJob(context.Background(), job.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if current.Status == model.MCPPlatformJobAwaitingApproval || current.Status == model.MCPPlatformJobFailed {
			job = current
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if job.Status != model.MCPPlatformJobAwaitingApproval || job.RevisionID == nil {
		t.Fatalf("expected approval state, got %s (%s)", job.Status, job.ErrorMessage)
	}
	var approval model.MCPApprovalRequest
	if err := db.Where("subject_id = ?", *job.RevisionID).First(&approval).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DecideApproval(context.Background(), approval.ID, approval.RequestDigest, model.MCPPlatformApprovalApproved, "self approval", "owner", model.RoleSecurityDeveloper); !errors.Is(err, ErrMCPPlatformSelfApproval) {
		t.Fatalf("expected self approval to be rejected, got %v", err)
	}
	if _, err := svc.DecideApproval(context.Background(), approval.ID, approval.RequestDigest, model.MCPPlatformApprovalApproved, "approved", "owner", model.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	serverObject, err := svc.GetServer(context.Background(), *job.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	if serverObject.LifecycleStatus != model.MCPPlatformServerPublished {
		t.Fatalf("expected server to be published after approval, got %s", serverObject.LifecycleStatus)
	}
	svc.SetCatalogSigningKey("catalog-signing-key")
	snapshot, err := svc.BuildCatalogSnapshot(context.Background(), catalog.ID)
	if err != nil || snapshot == nil || snapshot.Signature == "" {
		t.Fatalf("expected signed catalog snapshot: %#v %v", snapshot, err)
	}
}

func TestMCPRPCDoesNotSendOpaqueCredentialRef(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer short-lived-secret" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{}}`))
	}))
	defer server.Close()
	svc, _ := newMCPPlatformTestService(t)
	endpoint, err := validateRemoteEndpoint(strings.Replace(server.URL, "127.0.0.1", "localhost", 1), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.rpc(context.Background(), endpoint, "initialize", map[string]interface{}{}, model.MCPPlatformAuthBearer, "credential-ref"); err == nil {
		t.Fatal("expected missing broker to fail closed")
	}
	svc.SetCredentialBroker(func(context.Context, string) (string, error) { return "short-lived-secret", nil })
	if _, err := svc.rpc(context.Background(), endpoint, "initialize", map[string]interface{}{}, model.MCPPlatformAuthBearer, "credential-ref"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRemoteMCPEndpointRejectsSensitiveURLParts(t *testing.T) {
	if _, err := validateRemoteEndpoint("https://user:pass@example.test/mcp", "prod"); err == nil {
		t.Fatal("expected userinfo to be rejected")
	}
	if _, err := validateRemoteEndpoint("https://example.test/mcp?token=secret", "prod"); err == nil {
		t.Fatal("expected query to be rejected")
	}
	if _, err := validateRemoteEndpoint("http://example.test/mcp", "prod"); err == nil {
		t.Fatal("expected production HTTP to be rejected")
	}
	if _, err := validateRemoteEndpoint("http://127.0.0.1:8080/mcp", "dev"); err == nil {
		t.Fatal("expected loopback to be rejected by default")
	}
	if _, err := safeMCPDialContext(context.Background(), "tcp", "localhost:443"); err == nil {
		t.Fatal("expected DNS-resolved loopback to be rejected")
	}
	if endpoint, err := validateRemoteEndpoint("http://aegis-mcp:8085/mcp", "dev"); err != nil || endpoint.Hostname() != "aegis-mcp" {
		t.Fatalf("expected the explicitly named dev self-MCP endpoint to be accepted: %v", err)
	}
	if _, err := validateRemoteEndpoint("http://aegis-mcp:8085/mcp", "prod"); err == nil {
		t.Fatal("expected the self-MCP endpoint to remain unavailable in production")
	}
}

func TestMCPOnboardingUsesIdempotencyAndDiscoversTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		result := map[string]interface{}{}
		switch request["method"] {
		case "initialize":
			result = map[string]interface{}{"protocolVersion": "2025-11-25", "capabilities": map[string]interface{}{}}
		case "tools/list":
			result = map[string]interface{}{"tools": []interface{}{map[string]interface{}{"name": "search", "description": "read-only search", "inputSchema": map[string]interface{}{"type": "object"}}, map[string]interface{}{"name": "search", "description": "read-only duplicate", "inputSchema": map[string]interface{}{"type": "object"}}}}
		default:
			result = map[string]interface{}{}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": request["id"], "result": result})
	}))
	defer server.Close()

	svc, _ := newMCPPlatformTestService(t)
	endpoint := strings.Replace(server.URL, "127.0.0.1", "localhost", 1) + "/mcp"
	request := MCPOnboardingRequest{DisplayName: "Test remote", EndpointURL: endpoint, AuthType: model.MCPPlatformAuthNone, Environment: "dev", PublishPolicy: "auto_if_l1"}
	job, err := svc.CreateOnboardingJob(context.Background(), request, "idem-1", "tester")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.CreateOnboardingJob(context.Background(), request, "idem-1", "tester")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != job.ID {
		t.Fatalf("idempotent create returned different job: %s != %s", second.ID, job.ID)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, getErr := svc.GetJob(context.Background(), job.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if current.Status == model.MCPPlatformJobAwaitingApproval || current.Status == model.MCPPlatformJobActive {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	current, err := svc.GetJob(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != model.MCPPlatformJobAwaitingApproval && current.Status != model.MCPPlatformJobActive {
		t.Fatalf("onboarding did not reach terminal governance state: %s (%s)", current.Status, current.ErrorMessage)
	}
	if current.ServerID == nil || current.RevisionID == nil {
		t.Fatal("expected server and revision references")
	}
	tools, _, err := svc.ListTools(context.Background(), current.RevisionID, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 || tools[0].Alias == tools[1].Alias {
		t.Fatalf("expected stable aliases for duplicate tool names: %#v", tools)
	}
}

func TestMCPPlatformOverviewReturnsSafeCounters(t *testing.T) {
	svc, db := newMCPPlatformTestService(t)
	now := time.Now().UTC()
	if err := db.Create(&model.MCPServer{ID: uuid.New(), ServerKey: "server-1", DisplayName: "Server", OwnerUserID: "tester", Environment: "dev", Transport: model.MCPPlatformTransportStreamableHTTP, EndpointURL: "http://example.test/mcp", EndpointDisplay: "http://example.test/mcp", AuthType: model.MCPPlatformAuthNone, RiskTier: model.MCPPlatformRiskL2, LifecycleStatus: model.MCPPlatformServerPublished, CreatedBy: "tester", UpdatedBy: "tester", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	data, err := svc.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if data["remote_servers"] != int64(1) {
		t.Fatalf("unexpected server count: %#v", data["remote_servers"])
	}
}

func TestMCPPlatformOverviewExcludesRetiredServiceData(t *testing.T) {
	svc, db := newMCPPlatformTestService(t)
	now := time.Now().UTC()
	activeServerID, retiredServerID := uuid.New(), uuid.New()
	activeRevisionID, retiredRevisionID := uuid.New(), uuid.New()
	for _, server := range []model.MCPServer{
		{ID: activeServerID, ServerKey: "overview-active", DisplayName: "Overview Active", OwnerUserID: "tester", Environment: "dev", Transport: model.MCPPlatformTransportStreamableHTTP, EndpointURL: "https://active.example/mcp", EndpointDisplay: "https://active.example/mcp", AuthType: model.MCPPlatformAuthNone, RiskTier: model.MCPPlatformRiskL1, LifecycleStatus: model.MCPPlatformServerPublished, CreatedBy: "tester", UpdatedBy: "tester", CreatedAt: now, UpdatedAt: now},
		{ID: retiredServerID, ServerKey: "overview-retired", DisplayName: "Overview Retired", OwnerUserID: "tester", Environment: "dev", Transport: model.MCPPlatformTransportStreamableHTTP, EndpointURL: "https://retired.example/mcp", EndpointDisplay: "https://retired.example/mcp", AuthType: model.MCPPlatformAuthNone, RiskTier: model.MCPPlatformRiskL1, LifecycleStatus: model.MCPPlatformServerRetired, CreatedBy: "tester", UpdatedBy: "tester", CreatedAt: now, UpdatedAt: now},
	} {
		if err := db.Create(&server).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, revision := range []model.MCPServerRevision{
		{ID: activeRevisionID, ServerID: activeServerID, RevisionNo: 1, ProtocolVersion: "2025-11-25", ToolsSnapshot: []byte(`[]`), Digest: "overview-active", Status: model.MCPPlatformServerApproved, CreatedBy: "tester", CreatedAt: now},
		{ID: retiredRevisionID, ServerID: retiredServerID, RevisionNo: 1, ProtocolVersion: "2025-11-25", ToolsSnapshot: []byte(`[]`), Digest: "overview-retired", Status: model.MCPPlatformServerApproved, CreatedBy: "tester", CreatedAt: now},
	} {
		if err := db.Create(&revision).Error; err != nil {
			t.Fatal(err)
		}
	}
	catalogID := uuid.New()
	if err := db.Create(&model.MCPCatalog{ID: catalogID, CatalogKey: "overview-catalog", DisplayName: "Overview", Status: "active", CreatedBy: "tester", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	activeReleaseID, retiredReleaseID := uuid.New(), uuid.New()
	for _, release := range []model.MCPCatalogRelease{
		{ID: activeReleaseID, CatalogID: catalogID, ServerRevisionID: &activeRevisionID, ReleaseNo: 1, Manifest: []byte(`{}`), ManifestDigest: "active", Status: "published", CreatedBy: "tester", CreatedAt: now},
		{ID: retiredReleaseID, CatalogID: catalogID, ServerRevisionID: &retiredRevisionID, ReleaseNo: 2, Manifest: []byte(`{}`), ManifestDigest: "retired", Status: "published", CreatedBy: "tester", CreatedAt: now},
	} {
		if err := db.Create(&release).Error; err != nil {
			t.Fatal(err)
		}
	}
	activeToolID, retiredToolID := uuid.New(), uuid.New()
	for _, tool := range []model.MCPToolRevision{
		{ID: activeToolID, ServerRevisionID: activeRevisionID, UpstreamName: "active", Alias: "active", InputSchema: []byte(`{}`), OutputSchema: []byte(`{}`), RiskTier: model.MCPPlatformRiskL1, Status: "approved", CreatedAt: now},
		{ID: retiredToolID, ServerRevisionID: retiredRevisionID, UpstreamName: "retired", Alias: "retired", InputSchema: []byte(`{}`), OutputSchema: []byte(`{}`), RiskTier: model.MCPPlatformRiskL1, Status: "approved", CreatedAt: now},
	} {
		if err := db.Create(&tool).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, releaseTool := range []model.MCPCatalogReleaseTool{
		{ID: uuid.New(), ReleaseID: activeReleaseID, ToolRevisionID: activeToolID, ExposedName: "active", InputSchema: []byte(`{}`), OutputSchema: []byte(`{}`), Status: "active", CreatedAt: now},
		{ID: uuid.New(), ReleaseID: retiredReleaseID, ToolRevisionID: retiredToolID, ExposedName: "retired", InputSchema: []byte(`{}`), OutputSchema: []byte(`{}`), Status: "active", CreatedAt: now},
	} {
		if err := db.Create(&releaseTool).Error; err != nil {
			t.Fatal(err)
		}
	}
	invocationID := uuid.New()
	if err := db.Create(&model.MCPInvocation{ID: invocationID, CatalogReleaseID: &retiredReleaseID, ToolRevisionID: &retiredToolID, ToolAlias: "retired", Status: "succeeded", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.MCPSecurityVerdict{ID: uuid.New(), InvocationID: invocationID, DeterministicSeverity: "critical", OverallRisk: "critical", Evidence: []byte(`[]`), UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}

	data, err := svc.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if data["remote_servers"] != int64(1) || data["published_tools"] != int64(1) || data["high_risk_calls_24h"] != int64(0) {
		t.Fatalf("expected overview to exclude retired service data, data=%#v", data)
	}
}
