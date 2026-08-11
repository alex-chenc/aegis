package tools

import (
	"testing"

	"api-server/internal/assistant"
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
