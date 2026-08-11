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
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := NewMCPPlatformService(repository.NewMCPPlatformRepository(db), zap.NewNop())
	svc.client = &http.Client{Timeout: 2 * time.Second}
	return svc, db
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
	created, err := svc.CreateClientEndpoint(context.Background(), MCPClientEndpointCreateRequest{ClientKey: "codex-aegis", DisplayName: "Codex", ClientType: "service", ServerID: serverID, ToolAllowlist: []string{"list_hosts"}}, "admin", "http://localhost:8084")
	if err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || !strings.HasSuffix(created.Endpoint, "/mcp/v1/clients/codex-aegis") || len(created.Tools) != 2 {
		t.Fatalf("unexpected endpoint response: %#v", created)
	}
	tools, err := svc.RuntimeTools(context.Background(), created.Token, "codex-aegis")
	if err != nil || len(tools) != 1 || tools[0].Name != "list_hosts" {
		t.Fatalf("expected filtered runtime tools, tools=%#v err=%v", tools, err)
	}
	if _, err := svc.RuntimeTools(context.Background(), created.Token, "another-client"); !errors.Is(err, ErrMCPPlatformClientEndpointDenied) {
		t.Fatalf("expected endpoint identity mismatch to be denied, got %v", err)
	}
	if _, err := svc.UpdateClientEndpointTools(context.Background(), created.GrantID, []string{}); err != nil {
		t.Fatal(err)
	}
	tools, err = svc.RuntimeTools(context.Background(), created.Token, "codex-aegis")
	if err != nil || len(tools) != 0 {
		t.Fatalf("expected all tools disabled, tools=%#v err=%v", tools, err)
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
	tools, err := svc.ListTools(context.Background(), current.RevisionID)
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
