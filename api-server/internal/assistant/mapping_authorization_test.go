package assistant

import (
	"context"
	"testing"
)

func TestIntentBreakdownRejectsCapabilityOutsideRuntimeCatalog(t *testing.T) {
	breakdown := &IntentBreakdown{
		Goal:                  "generate and execute a script",
		Scope:                 IntentScope{Kind: "online_hosts"},
		CandidateCapabilities: []string{"deploy_check"},
		Confidence:            0.9,
	}
	catalog := []CapabilityCatalogItem{{
		Capability: "execute_vulnerability_host_scripts",
		Domain:     "vulnerability",
		Operation:  "execute",
	}}

	if err := validateIntentBreakdownAgainstCatalog(breakdown, catalog, nil); err == nil {
		t.Fatal("capability outside the runtime catalog must be rejected")
	}
}

func TestToolDecisionUsesExactMappingWithoutScoreThreshold(t *testing.T) {
	registry := NewToolRegistry()
	if err := registry.Register(&ToolSpec{
		Name:               "Example.Status",
		Domain:             DomainSystem,
		Operation:          OpGet,
		Capability:         "get_example_status",
		Description:        "Get example status",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		Enabled:            true,
		Handler:            noopToolHandler,
	}); err != nil {
		t.Fatal(err)
	}
	engine := NewToolDecisionEngine(ToolDecisionEngineDeps{
		Registry: registry,
		Mapper:   NewToolCapabilityMapper(registry),
		Config:   ToolDecisionConfig{Enabled: true},
	})
	breakdown := &IntentBreakdown{
		Goal:                  "inspect status",
		Scope:                 IntentScope{Kind: "unspecified"},
		CandidateCapabilities: []string{"get_example_status"},
		RiskHint:              string(ToolRiskReadonly),
		Confidence:            0.9,
	}

	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:     "inspect status",
		Intent:    IntentResult{Action: "query", RiskHint: ToolRiskReadonly},
		Breakdown: breakdown,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.ToolNames(); len(got) != 1 || got[0] != "Example.Status" {
		t.Fatalf("exactly mapped tool should be authorized without scoring, got %v", got)
	}
}

func TestToolDecisionDoesNotAuthorizeWrongPreselection(t *testing.T) {
	registry := NewToolRegistry()
	for _, spec := range []*ToolSpec{
		{
			Name:               "Example.Status",
			Domain:             DomainSystem,
			Operation:          OpGet,
			Capability:         "get_example_status",
			Description:        "Get example status",
			Risk:               ToolRiskReadonly,
			DefaultWhitelisted: true,
			Enabled:            true,
			Handler:            noopToolHandler,
		},
		{
			Name:               "Wrong.Execute",
			Domain:             DomainTask,
			Operation:          OpExecute,
			Capability:         "execute_wrong_task",
			Description:        "Execute an unrelated task",
			Risk:               ToolRiskMedium,
			DefaultWhitelisted: false,
			Enabled:            true,
			Handler:            noopToolHandler,
		},
	} {
		if err := registry.Register(spec); err != nil {
			t.Fatal(err)
		}
	}
	engine := NewToolDecisionEngine(ToolDecisionEngineDeps{
		Registry: registry,
		Mapper:   NewToolCapabilityMapper(registry),
		Config:   ToolDecisionConfig{Enabled: true},
	})
	breakdown := &IntentBreakdown{
		Goal:                  "inspect status",
		Scope:                 IntentScope{Kind: "unspecified"},
		CandidateCapabilities: []string{"get_example_status"},
		RiskHint:              string(ToolRiskReadonly),
		Confidence:            0.9,
	}

	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:     "inspect status",
		Intent:    IntentResult{Action: "query", RiskHint: ToolRiskReadonly},
		Breakdown: breakdown,
		PreliminarySelection: &ToolSelectionResult{
			SelectedTools: []string{"Wrong.Execute"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if containsDecisionString(plan.ToolNames(), "Wrong.Execute") {
		t.Fatalf("preselected tool outside capability mapping must not be authorized: %v", plan.ToolNames())
	}
}

func TestCapabilityMappingAddsOnlyReadonlyDeclaredCompanions(t *testing.T) {
	registry := NewToolRegistry()
	for _, spec := range []*ToolSpec{
		{
			Name:               "Example.Generate",
			Domain:             DomainSystem,
			Operation:          OpGenerate,
			Capability:         "generate_example",
			Description:        "Generate example",
			Risk:               ToolRiskMedium,
			DefaultWhitelisted: false,
			Enabled:            true,
			Handler:            noopToolHandler,
			ExecutionContract: ToolExecutionContract{
				Mode:                 ToolExecutionAsynchronous,
				CompletionCapability: "get_example_status",
			},
		},
		{
			Name:               "Example.Status",
			Domain:             DomainSystem,
			Operation:          OpGet,
			Capability:         "get_example_status",
			Description:        "Get example status",
			Risk:               ToolRiskReadonly,
			DefaultWhitelisted: true,
			Enabled:            true,
			Handler:            noopToolHandler,
		},
		{
			Name:               "Wrong.Execute",
			Domain:             DomainSystem,
			Operation:          OpExecute,
			Capability:         "execute_wrong",
			Description:        "Execute wrong operation",
			Risk:               ToolRiskHigh,
			DefaultWhitelisted: false,
			Enabled:            true,
			Handler:            noopToolHandler,
		},
	} {
		if err := registry.Register(spec); err != nil {
			t.Fatal(err)
		}
	}
	mapper := NewToolCapabilityMapper(registry)
	companions := mapper.ReadonlyCompanionToolNames([]string{"Example.Generate"})
	if len(companions) != 1 || companions[0] != "Example.Status" {
		t.Fatalf("readonly completion companion = %v", companions)
	}
	if containsDecisionString(companions, "Wrong.Execute") {
		t.Fatalf("undeclared write tool must not be exposed: %v", companions)
	}
}

func TestCapabilityMappingExpandsNestedLowRiskCompanionStatus(t *testing.T) {
	registry := NewToolRegistry()
	for _, spec := range []*ToolSpec{
		{
			Name:       "Example.Generate",
			Capability: "generate_example",
			Risk:       ToolRiskMedium,
			Enabled:    true,
			ExecutionContract: ToolExecutionContract{
				DiscoveryCapabilities: []string{"start_example_lookup"},
			},
			Handler: noopToolHandler,
		},
		{
			Name:       "Example.Lookup.Start",
			Capability: "start_example_lookup",
			Risk:       ToolRiskLow,
			Enabled:    true,
			ExecutionContract: ToolExecutionContract{
				CompletionCapability: "get_example_lookup_status",
			},
			Handler: noopToolHandler,
		},
		{
			Name:       "Example.Lookup.Status",
			Capability: "get_example_lookup_status",
			Risk:       ToolRiskReadonly,
			Enabled:    true,
			Handler:    noopToolHandler,
		},
	} {
		if err := registry.Register(spec); err != nil {
			t.Fatal(err)
		}
	}

	names := NewToolCapabilityMapper(registry).ReadonlyCompanionToolNames([]string{"Example.Generate"})
	for _, want := range []string{"Example.Lookup.Start", "Example.Lookup.Status"} {
		if !containsDecisionString(names, want) {
			t.Fatalf("nested companion %s missing from %v", want, names)
		}
	}
}
