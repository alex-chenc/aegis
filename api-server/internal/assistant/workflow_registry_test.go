package assistant

import (
	"context"
	"testing"
)

func TestWorkflowRegistryMatchesBaselineWithoutReturningEveryModule(t *testing.T) {
	matches := NewWorkflowRegistry().Match(IntentResult{
		Domains:     []string{"baseline"},
		Operations:  []string{"execute"},
		ObjectTypes: []string{"baseline_template", "host"},
	})
	if len(matches) == 0 || len(matches) >= len(builtinWorkflowSpecs()) {
		t.Fatalf("workflow matches were not scoped: %#v", matches)
	}
	found := false
	for _, match := range matches {
		if match.ID == "baseline_compliance" {
			found = true
		}
	}
	if !found {
		t.Fatalf("baseline workflow missing: %#v", matches)
	}
}

func TestIntentBreakdownRejectsInventedWorkflowID(t *testing.T) {
	breakdown := &IntentBreakdown{
		Goal:          "Run a baseline check.",
		Scope:         IntentScope{Kind: "all"},
		RiskHint:      "high",
		WorkflowIDs:   []string{"invented_workflow"},
		Confidence:    0.9,
		Objects:       []IntentObject{},
		MissingInfo:   []MissingInfo{},
		Parameters:    IntentParameters{},
		RequiresWrite: true,
	}
	if err := validateIntentBreakdownAgainstCatalog(breakdown, nil, []WorkflowSpec{{ID: "baseline_compliance"}}); err == nil {
		t.Fatal("expected an invented workflow ID to be rejected")
	}
}

func TestWorkflowRegistryResolvesOnlyExactRegisteredIDs(t *testing.T) {
	registry := NewWorkflowRegistry()
	resolved, err := registry.Resolve([]string{detectionPackageLifecycleWorkflowID})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if len(resolved) != 1 || resolved[0].ID != detectionPackageLifecycleWorkflowID {
		t.Fatalf("resolved workflows = %#v", resolved)
	}
	if _, err := registry.Resolve([]string{"invented_detection_workflow"}); err == nil {
		t.Fatal("expected an unregistered workflow ID to be rejected")
	}
}

func TestWorkflowRegistryIncludesAgentGuardObservationAndControlContracts(t *testing.T) {
	registry := NewWorkflowRegistry()
	observation, err := registry.Resolve([]string{agentGuardObservationWorkflowID})
	if err != nil || len(observation) != 1 {
		t.Fatalf("agent guard observation workflow = %#v, err=%v", observation, err)
	}
	if observation[0].Domain != DomainAgentGuard || !containsExactString(observation[0].ExposedCapabilities, "query_agent_conversations") {
		t.Fatalf("agent guard observation contract = %#v", observation[0])
	}
	control, err := registry.Resolve([]string{agentGuardControlWorkflowID})
	if err != nil || len(control) != 1 {
		t.Fatalf("agent guard control workflow = %#v, err=%v", control, err)
	}
	if control[0].Risk != ToolRiskHigh || !containsExactString(control[0].ExposedCapabilities, "kill_agent_guard_instance") {
		t.Fatalf("agent guard control contract = %#v", control[0])
	}
}

func TestWorkflowRegistryIncludesManagedMCPAggregationContract(t *testing.T) {
	workflow, err := NewWorkflowRegistry().Resolve([]string{MCPAggregationQueryWorkflowID})
	if err != nil || len(workflow) != 1 {
		t.Fatalf("managed MCP workflow = %#v, err=%v", workflow, err)
	}
	if workflow[0].Version != "6.3" || workflow[0].Domain != DomainExternalMCP {
		t.Fatalf("managed MCP workflow metadata = %#v", workflow[0])
	}
	want := []string{"list_mcp_catalogs", "list_mcp_tools", "query_aggregated_mcp", "get_mcp_invocation"}
	if len(workflow[0].ExposedCapabilities) != len(want) {
		t.Fatalf("managed MCP capabilities = %#v, want %#v", workflow[0].ExposedCapabilities, want)
	}
	for _, capability := range want {
		if !containsExactString(workflow[0].ExposedCapabilities, capability) {
			t.Fatalf("managed MCP capability %q missing from %#v", capability, workflow[0].ExposedCapabilities)
		}
	}
}

func TestCapabilityCatalogUsesClosedSelectedWorkflowAllowlist(t *testing.T) {
	toolRegistry := NewToolRegistry()
	for _, tool := range []*ToolSpec{
		{
			Name:        "Package.Draft.Generate",
			Domain:      DomainPackage,
			Operation:   OpGenerate,
			Capability:  "generate_detection_package_draft",
			Description: "Generate a dynamic detection package draft.",
			ObjectTypes: []string{"detection_package", "package"},
			Risk:        ToolRiskMedium,
			Enabled:     true,
			ArgsSchema:  map[string]interface{}{"type": "object"},
			Handler:     func(context.Context, map[string]interface{}) (interface{}, error) { return nil, nil },
		},
		{
			Name:        "Vulnerability.AffectedHosts",
			Domain:      DomainVulnerability,
			Operation:   OpGet,
			Capability:  "get_vulnerability_affected_hosts",
			Description: "List hosts affected by a vulnerability.",
			ObjectTypes: []string{"vulnerability", "host"},
			Risk:        ToolRiskReadonly,
			Enabled:     true,
			ArgsSchema:  map[string]interface{}{"type": "object"},
			Handler:     func(context.Context, map[string]interface{}) (interface{}, error) { return nil, nil },
		},
	} {
		if err := toolRegistry.Register(tool); err != nil {
			t.Fatal(err)
		}
	}

	workflows, err := NewWorkflowRegistry().Resolve([]string{detectionPackageLifecycleWorkflowID})
	if err != nil {
		t.Fatal(err)
	}
	catalog := (&Orchestrator{toolRegistry: toolRegistry}).buildCapabilityCatalog(
		IntentResult{
			Domains:     []string{"package", "vulnerability"},
			ObjectTypes: []string{"detection_package", "vulnerability", "cve"},
			WorkflowIDs: []string{detectionPackageLifecycleWorkflowID},
		},
		nil,
		workflows,
	)
	got := make(map[string]bool, len(catalog))
	for _, item := range catalog {
		got[item.Capability] = true
	}
	if !got["generate_detection_package_draft"] {
		t.Fatalf("package capability missing from catalog: %#v", got)
	}
	if got["get_vulnerability_affected_hosts"] {
		t.Fatalf("vulnerability capability leaked outside the closed package workflow: %#v", got)
	}
}

func TestWeakPasswordWorkflowCatalogIncludesHostResolution(t *testing.T) {
	toolRegistry := NewToolRegistry()
	for _, tool := range []*ToolSpec{
		{
			Name:               "Host.Resolve",
			Domain:             DomainHost,
			Operation:          OpGet,
			Capability:         "resolve_hosts",
			Description:        "Resolve host selectors.",
			ObjectTypes:        []string{"host"},
			Risk:               ToolRiskReadonly,
			Enabled:            true,
			DefaultWhitelisted: true,
			ArgsSchema:         map[string]interface{}{"type": "object"},
			Handler:            func(context.Context, map[string]interface{}) (interface{}, error) { return nil, nil },
		},
		{
			Name:        "Credential.WeakPassword.Scan",
			Domain:      DomainDetection,
			Operation:   OpExecute,
			Capability:  "weak_password_scan",
			Description: "Start a weak-password assessment.",
			ObjectTypes: []string{"candidate_application"},
			Risk:        ToolRiskMedium,
			Enabled:     true,
			ArgsSchema:  map[string]interface{}{"type": "object"},
			Handler:     func(context.Context, map[string]interface{}) (interface{}, error) { return nil, nil },
		},
	} {
		if err := toolRegistry.Register(tool); err != nil {
			t.Fatal(err)
		}
	}

	workflows, err := NewWorkflowRegistry().Resolve([]string{weakPasswordAssessmentWorkflowID})
	if err != nil {
		t.Fatal(err)
	}
	catalog := (&Orchestrator{toolRegistry: toolRegistry}).buildCapabilityCatalog(
		IntentResult{
			Domains:     []string{"asset"},
			ObjectTypes: []string{"host"},
			WorkflowIDs: []string{weakPasswordAssessmentWorkflowID},
		},
		nil,
		workflows,
	)
	got := make(map[string]bool, len(catalog))
	for _, item := range catalog {
		got[item.Capability] = true
	}
	if !got["resolve_hosts"] {
		t.Fatalf("weak-password workflow must expose host resolution for hostname/IP targets: %#v", got)
	}
}

func TestCapabilityCatalogIncludesSelectedMultiWorkflowCapabilities(t *testing.T) {
	toolRegistry := NewToolRegistry()
	for _, tool := range []*ToolSpec{
		{
			Name:        "Asset.Collection.Trigger",
			Domain:      DomainAsset,
			Operation:   OpExecute,
			Capability:  "trigger_asset_collection",
			Description: "Trigger asset collection.",
			ObjectTypes: []string{"asset", "host"},
			Risk:        ToolRiskMedium,
			Enabled:     true,
			ExecutionContract: ToolExecutionContract{
				Mode:                 ToolExecutionAsynchronous,
				CompletionCapability: "get_asset_collection_task",
			},
			ArgsSchema: map[string]interface{}{"type": "object"},
			Handler:    func(context.Context, map[string]interface{}) (interface{}, error) { return nil, nil },
		},
		{
			Name:        "Asset.Collection.Get",
			Domain:      DomainAsset,
			Operation:   OpGet,
			Capability:  "get_asset_collection_task",
			Description: "Get asset collection progress.",
			ObjectTypes: []string{"asset_collection"},
			Risk:        ToolRiskReadonly,
			Enabled:     true,
			ArgsSchema:  map[string]interface{}{"type": "object"},
			Handler:     func(context.Context, map[string]interface{}) (interface{}, error) { return nil, nil },
		},
		{
			Name:        "Vulnerability.Scan.Start",
			Domain:      DomainVulnerability,
			Operation:   OpExecute,
			Capability:  "start_vulnerability_scan",
			Description: "Start a vulnerability scan.",
			ObjectTypes: []string{"vulnerability", "host"},
			Risk:        ToolRiskMedium,
			Enabled:     true,
			ExecutionContract: ToolExecutionContract{
				Mode:                 ToolExecutionAsynchronous,
				CompletionCapability: "get_vulnerability_scan_status",
			},
			ArgsSchema: map[string]interface{}{"type": "object"},
			Handler:    func(context.Context, map[string]interface{}) (interface{}, error) { return nil, nil },
		},
		{
			Name:        "Vulnerability.Scan.Status",
			Domain:      DomainVulnerability,
			Operation:   OpGet,
			Capability:  "get_vulnerability_scan_status",
			Description: "Get vulnerability scan progress.",
			ObjectTypes: []string{"vulnerability_scan"},
			Risk:        ToolRiskReadonly,
			Enabled:     true,
			ArgsSchema:  map[string]interface{}{"type": "object"},
			Handler:     func(context.Context, map[string]interface{}) (interface{}, error) { return nil, nil },
		},
		{
			Name:        "Credential.WeakPassword.AnalyzeApplications",
			Domain:      DomainDetection,
			Operation:   OpGenerate,
			Capability:  "weak_password_asset_analysis",
			Description: "Analyze password-authenticated applications.",
			ObjectTypes: []string{"candidate_application"},
			Risk:        ToolRiskLow,
			Enabled:     true,
			ArgsSchema:  map[string]interface{}{"type": "object"},
			Handler:     func(context.Context, map[string]interface{}) (interface{}, error) { return nil, nil },
		},
		{
			Name:        "Credential.WeakPassword.Scan",
			Domain:      DomainDetection,
			Operation:   OpExecute,
			Capability:  "weak_password_scan",
			Description: "Start a weak-password assessment.",
			ObjectTypes: []string{"candidate_application"},
			Risk:        ToolRiskMedium,
			Enabled:     true,
			ArgsSchema:  map[string]interface{}{"type": "object"},
			Handler:     func(context.Context, map[string]interface{}) (interface{}, error) { return nil, nil },
		},
		{
			Name:        "Credential.WeakPassword.QueryProgress",
			Domain:      DomainDetection,
			Operation:   OpGet,
			Capability:  "weak_password_progress",
			Description: "Get weak-password assessment progress.",
			ObjectTypes: []string{"task"},
			Risk:        ToolRiskReadonly,
			Enabled:     true,
			ArgsSchema:  map[string]interface{}{"type": "object"},
			Handler:     func(context.Context, map[string]interface{}) (interface{}, error) { return nil, nil },
		},
	} {
		if err := toolRegistry.Register(tool); err != nil {
			t.Fatal(err)
		}
	}

	workflows, err := NewWorkflowRegistry().Resolve([]string{
		assetInventoryWorkflowID,
		vulnerabilityAssessmentWorkflowID,
		"weak_password_assessment",
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog := (&Orchestrator{toolRegistry: toolRegistry}).buildCapabilityCatalog(
		IntentResult{
			Domains:     []string{"asset", "vulnerability"},
			ObjectTypes: []string{"host", "asset", "vulnerability", "credential"},
			WorkflowIDs: []string{
				assetInventoryWorkflowID,
				vulnerabilityAssessmentWorkflowID,
				"weak_password_assessment",
			},
		},
		nil,
		workflows,
	)
	got := make(map[string]bool, len(catalog))
	for _, item := range catalog {
		got[item.Capability] = true
	}
	for _, capability := range []string{
		"trigger_asset_collection",
		"get_asset_collection_task",
		"start_vulnerability_scan",
		"get_vulnerability_scan_status",
		"weak_password_asset_analysis",
		"weak_password_scan",
		"weak_password_progress",
	} {
		if !got[capability] {
			t.Fatalf("selected workflow capability %q missing from catalog: %#v", capability, got)
		}
	}

	breakdown := &IntentBreakdown{
		Goal:          "Collect assets, scan vulnerabilities, and assess weak passwords on host 192.168.152.159.",
		Domains:       []string{"asset", "vulnerability"},
		Actions:       []string{"collect", "scan"},
		Objects:       []IntentObject{{Type: "host", ID: "192.168.152.159", Selector: "ip_address"}},
		Scope:         IntentScope{Kind: "specific", ObjectIDs: []string{"192.168.152.159"}},
		Parameters:    IntentParameters{"host_ip": "192.168.152.159"},
		RiskHint:      "high",
		RequiresWrite: true,
		WorkflowIDs: []string{
			assetInventoryWorkflowID,
			vulnerabilityAssessmentWorkflowID,
			"weak_password_assessment",
		},
		CandidateCapabilities: []string{
			"trigger_asset_collection",
			"get_asset_collection_task",
			"start_vulnerability_scan",
			"get_vulnerability_scan_status",
			"weak_password_asset_analysis",
			"weak_password_scan",
			"weak_password_progress",
		},
		Confidence: 0.95,
	}
	if err := validateIntentBreakdownAgainstCatalog(breakdown, catalog, workflows); err != nil {
		t.Fatalf("latest multi-workflow intent must validate against its capability catalog: %v", err)
	}
}
