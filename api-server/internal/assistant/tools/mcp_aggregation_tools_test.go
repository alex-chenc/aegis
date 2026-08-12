package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/service"
	"github.com/google/uuid"
)

type fakeMCPClientAuthorizationPlatform struct {
	servers []model.MCPServer
	request service.MCPClientEndpointCreateRequest
}

func (f *fakeMCPClientAuthorizationPlatform) FindPublishedServers(_ context.Context, _ string) ([]model.MCPServer, error) {
	return f.servers, nil
}

func (f *fakeMCPClientAuthorizationPlatform) GetServer(_ context.Context, id uuid.UUID) (*model.MCPServer, error) {
	for _, server := range f.servers {
		if server.ID == id {
			copy := server
			return &copy, nil
		}
	}
	return nil, service.ErrMCPPlatformClientEndpointDenied
}

func (f *fakeMCPClientAuthorizationPlatform) CreateClientEndpoint(_ context.Context, request service.MCPClientEndpointCreateRequest, _ string, _ string) (*service.MCPClientEndpointCreated, error) {
	f.request = request
	return &service.MCPClientEndpointCreated{MCPClientEndpointView: service.MCPClientEndpointView{
		ClientID: uuid.New(), ClientKey: request.ClientKey, DisplayName: request.DisplayName, ClientType: request.ClientType,
		Status: "active", GrantID: uuid.New(), ServerID: request.ServerID, ServerName: "Remote MCP aegis-mcp", Endpoint: "http://gateway/mcp/v1/clients/" + request.ClientKey,
	}, Token: "one-time-token"}, nil
}

func TestRegisterMCPAggregationToolsBindsV63WorkflowExposure(t *testing.T) {
	registry := assistant.NewToolRegistry()
	if err := RegisterMCPAggregationTools(registry, MCPAggregationToolDeps{
		Gateway: &assistant.MCPGatewayClient{},
	}); err != nil {
		t.Fatalf("RegisterMCPAggregationTools returned error: %v", err)
	}

	want := map[string]assistant.ToolExposure{
		"list_mcp_catalogs":    assistant.ToolExposureContextual,
		"list_mcp_tools":       assistant.ToolExposureContextual,
		"query_aggregated_mcp": assistant.ToolExposurePrimary,
	}
	for capability, exposure := range want {
		var found bool
		for _, tool := range registry.List() {
			if tool == nil || tool.Capability != capability {
				continue
			}
			found = true
			if tool.ExposurePolicy.Exposure != exposure || len(tool.ExposurePolicy.WorkflowIDs) != 1 || tool.ExposurePolicy.WorkflowIDs[0] != assistant.MCPAggregationQueryWorkflowID {
				t.Fatalf("tool %q exposure = %#v, want %s bound to %s", capability, tool.ExposurePolicy, exposure, assistant.MCPAggregationQueryWorkflowID)
			}
		}
		if !found {
			t.Fatalf("managed MCP capability %q was not registered", capability)
		}
	}
}

func TestRegisterMCPOnboardingToolsBindsGovernedWorkflow(t *testing.T) {
	registry := assistant.NewToolRegistry()
	deps := MCPOnboardingToolDeps{Platform: service.NewMCPPlatformService(nil, nil)}
	if err := RegisterMCPOnboardingTool(registry, deps); err != nil {
		t.Fatalf("RegisterMCPOnboardingTool returned error: %v", err)
	}
	if err := RegisterMCPOnboardingStatusTool(registry, deps); err != nil {
		t.Fatalf("RegisterMCPOnboardingStatusTool returned error: %v", err)
	}

	var primary, companion *assistant.ToolSpec
	for _, tool := range registry.List() {
		switch tool.Capability {
		case "onboard_mcp_server":
			primary = tool
		case "get_mcp_onboarding_status":
			companion = tool
		}
	}
	if primary == nil || companion == nil {
		t.Fatalf("onboarding tools were not registered: %#v", registry.List())
	}
	if primary.Risk != assistant.ToolRiskHigh || !primary.RequiresApproval || primary.AutoCallable {
		t.Fatalf("onboarding primary must be an approval-gated write tool: %#v", primary)
	}
	if primary.ExecutionContract.CompletionCapability != "get_mcp_onboarding_status" {
		t.Fatalf("onboarding completion capability = %q", primary.ExecutionContract.CompletionCapability)
	}
	if len(primary.ExposurePolicy.WorkflowIDs) != 1 || primary.ExposurePolicy.WorkflowIDs[0] != assistant.MCPAggregationOnboardingWorkflowID {
		t.Fatalf("onboarding primary workflow binding = %#v", primary.ExposurePolicy)
	}
	if companion.ExposurePolicy.Exposure != assistant.ToolExposureCompanion {
		t.Fatalf("onboarding status exposure = %s", companion.ExposurePolicy.Exposure)
	}
	if required := companion.ArgsSchema["required"]; required == nil {
		t.Fatal("onboarding status must declare job_id as a required argument")
	}
}

func TestRegisterMCPClientAuthorizationResolvesExistingServiceWithoutEndpoint(t *testing.T) {
	serverID := uuid.New()
	fake := &fakeMCPClientAuthorizationPlatform{servers: []model.MCPServer{{
		ID: serverID, ServerKey: "mcp_aegis", DisplayName: "Remote MCP aegis-mcp", Environment: "dev",
		LifecycleStatus: model.MCPPlatformServerPublished, ActiveRevisionID: func() *uuid.UUID { id := uuid.New(); return &id }(),
	}}}
	registry := assistant.NewToolRegistry()
	if err := RegisterMCPClientAuthorizationTool(registry, MCPClientAuthorizationToolDeps{Platform: fake, PublicGatewayURL: "http://gateway"}); err != nil {
		t.Fatalf("RegisterMCPClientAuthorizationTool returned error: %v", err)
	}
	tool, ok := registry.Get("MCP.Aggregation.Client.Authorize")
	if !ok || tool == nil {
		t.Fatal("client authorization tool was not registered")
	}
	properties := tool.ArgsSchema["properties"].(map[string]interface{})
	if _, exists := properties["endpoint_url"]; exists {
		t.Fatal("client authorization must not request endpoint_url")
	}
	ctx := assistant.WithToolInvocationContext(context.Background(), assistant.ToolInvocationContext{SessionID: "session-1", Operator: "admin"})
	result, err := tool.Handler(ctx, map[string]interface{}{"service_name": "Remote MCP aegis-mcp"})
	if err != nil {
		t.Fatalf("client authorization handler failed: %v", err)
	}
	if fake.request.ServerID != serverID || fake.request.ClientKey == "" {
		t.Fatalf("client authorization request did not bind the named service: %#v", fake.request)
	}
	if result.(map[string]interface{})["token"] != "one-time-token" {
		t.Fatalf("client authorization did not return the one-time token: %#v", result)
	}
}

func TestMCPClientAuthorizationRejectsAmbiguousServiceName(t *testing.T) {
	fake := &fakeMCPClientAuthorizationPlatform{servers: []model.MCPServer{
		{ID: uuid.New(), DisplayName: "Remote MCP aegis-mcp", Environment: "dev", LifecycleStatus: model.MCPPlatformServerPublished, ActiveRevisionID: func() *uuid.UUID { id := uuid.New(); return &id }()},
		{ID: uuid.New(), DisplayName: "Remote MCP aegis-mcp", Environment: "dev", LifecycleStatus: model.MCPPlatformServerPublished, ActiveRevisionID: func() *uuid.UUID { id := uuid.New(); return &id }()},
	}}
	registry := assistant.NewToolRegistry()
	if err := RegisterMCPClientAuthorizationTool(registry, MCPClientAuthorizationToolDeps{Platform: fake}); err != nil {
		t.Fatal(err)
	}
	tool, _ := registry.Get("MCP.Aggregation.Client.Authorize")
	ctx := assistant.WithToolInvocationContext(context.Background(), assistant.ToolInvocationContext{SessionID: "session-1", Operator: "admin"})
	_, err := tool.Handler(ctx, map[string]interface{}{"service_name": "Remote MCP aegis-mcp"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "server_id") {
		t.Fatalf("expected an explicit server_id clarification for duplicate services, got %v", err)
	}
}

func TestMCPClientAuthorizationCollapsesEquivalentDuplicateRegistrations(t *testing.T) {
	oldID, newID := uuid.New(), uuid.New()
	oldRevision, newRevision := uuid.New(), uuid.New()
	now := time.Now().UTC()
	base := func(id, revision uuid.UUID, updatedAt time.Time) model.MCPServer {
		return model.MCPServer{
			ID: id, ServerKey: "mcp_duplicate", DisplayName: "Remote MCP aegis-mcp", Environment: "dev",
			Transport: model.MCPPlatformTransportStreamableHTTP, AuthType: model.MCPPlatformAuthNone,
			RiskTier: model.MCPPlatformRiskL2, EndpointDisplay: "http://aegis-mcp:8085/mcp",
			LifecycleStatus: model.MCPPlatformServerPublished, ActiveRevisionID: &revision,
			CreatedAt: updatedAt.Add(-time.Hour), UpdatedAt: updatedAt,
		}
	}
	old := base(oldID, oldRevision, now.Add(-time.Minute))
	newer := base(newID, newRevision, now)
	newer.ServerKey = "mcp_duplicate_new"
	fake := &fakeMCPClientAuthorizationPlatform{servers: []model.MCPServer{old, newer}}
	registry := assistant.NewToolRegistry()
	if err := RegisterMCPClientAuthorizationTool(registry, MCPClientAuthorizationToolDeps{Platform: fake}); err != nil {
		t.Fatal(err)
	}
	tool, _ := registry.Get("MCP.Aggregation.Client.Authorize")
	ctx := assistant.WithToolInvocationContext(context.Background(), assistant.ToolInvocationContext{SessionID: "session-1", Operator: "admin"})
	if _, err := tool.Handler(ctx, map[string]interface{}{"service_name": "Remote MCP aegis-mcp"}); err != nil {
		t.Fatalf("equivalent duplicate registrations should resolve: %v", err)
	}
	if fake.request.ServerID != newID {
		t.Fatalf("canonical server_id = %s, want newest %s", fake.request.ServerID, newID)
	}
}
