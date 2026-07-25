package assistant

import (
	"context"
	"strings"
	"testing"
)

func TestCompiledPlanValidatorRejectsPreviousStepWithoutMatchingProducerField(t *testing.T) {
	registry := NewToolRegistry()
	registerCompiledValidatorTestTool(t, registry, &ToolSpec{
		Name:        "Host.Resolve",
		Domain:      DomainHost,
		Operation:   OpGet,
		Capability:  "resolve_hosts",
		Description: "Resolve hosts.",
		Risk:        ToolRiskReadonly,
		ResultContract: ToolResultContract{
			FactBindings: []ToolFactBinding{{Kind: "host_resolved", IDField: "host_id"}},
		},
	})
	registerCompiledValidatorTestTool(t, registry, &ToolSpec{
		Name:        "Query.Status",
		Domain:      DomainPackage,
		Operation:   OpGet,
		Capability:  "get_query_status",
		Description: "Get query status.",
		Risk:        ToolRiskReadonly,
		ArgsSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"query_id": map[string]interface{}{"type": "string"}},
			"required":   []string{"query_id"},
		},
	})

	plan := &ToolExecutionPlan{Steps: []ToolPlanStep{
		{StepID: "step-1", ToolName: "Host.Resolve", Capability: "resolve_hosts", Args: map[string]interface{}{}},
		{
			StepID:     "step-2",
			ToolName:   "Query.Status",
			Capability: "get_query_status",
			Args:       map[string]interface{}{},
			ArgSources: map[string]ArgSource{
				"query_id": {SourceType: "previous_step", SourceRef: "query"},
			},
		},
	}}

	err := NewCompiledPlanValidator(registry, nil).Validate(plan, nil)
	if err == nil || !strings.Contains(err.Error(), "query_id") {
		t.Fatalf("expected missing matching producer error, got %v", err)
	}
}

func TestCompiledPlanValidatorRunsBusinessPreflight(t *testing.T) {
	registry := NewToolRegistry()
	registerCompiledValidatorTestTool(t, registry, &ToolSpec{
		Name:        "Host.Resolve",
		Domain:      DomainHost,
		Operation:   OpGet,
		Capability:  "resolve_hosts",
		Description: "Resolve hosts.",
		Risk:        ToolRiskReadonly,
		Preflight: func(_ context.Context, args map[string]interface{}) error {
			if len(args) == 0 {
				return context.Canceled
			}
			return nil
		},
	})

	err := NewCompiledPlanValidator(registry, nil).Validate(&ToolExecutionPlan{Steps: []ToolPlanStep{{
		StepID: "step-1", ToolName: "Host.Resolve", Capability: "resolve_hosts", Args: map[string]interface{}{},
	}}}, nil)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected business preflight error, got %v", err)
	}
}

func registerCompiledValidatorTestTool(t *testing.T, registry *ToolRegistry, tool *ToolSpec) {
	t.Helper()
	tool.Enabled = true
	if tool.ArgsSchema == nil {
		tool.ArgsSchema = map[string]interface{}{"type": "object"}
	}
	tool.Handler = func(context.Context, map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"ok": true}, nil
	}
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register %s: %v", tool.Name, err)
	}
}
