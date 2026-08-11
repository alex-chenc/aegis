package tools

import (
	"testing"

	"api-server/internal/assistant"
	"api-server/internal/service"
)

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
}
