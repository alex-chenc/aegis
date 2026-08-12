package assistant

import (
	"context"
	"reflect"
	"testing"
)

func TestToolDecisionEngineRejectsConceptWriteTool(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "Asset.Collection.Trigger",
		Domain:             DomainAsset,
		Operation:          OpExecute,
		Capability:         "trigger_asset_collection",
		Description:        "触发资产采集任务",
		ObjectTypes:        []string{"asset", "host"},
		Risk:               ToolRiskMedium,
		DefaultWhitelisted: false,
	})

	intent := IntentResult{Domains: []string{"asset"}, Action: "query", Object: "asset", RiskHint: ToolRiskReadonly, Confidence: 0.8}
	breakdown := makeDecisionBreakdown(t, "资产采集是什么", intent, nil, []string{"trigger_asset_collection"})
	engine := newDecisionTestEngine(registry)

	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:     "资产采集是什么",
		Intent:    intent,
		Breakdown: breakdown,
		PreliminarySelection: &ToolSelectionResult{
			SelectedTools:  []string{"Asset.Collection.Trigger"},
			CandidateTools: []string{"Asset.Collection.Trigger"},
		},
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	assertNotContainsTool(t, plan.ToolNames(), "Asset.Collection.Trigger")
	assertRejectedDecision(t, plan, "Asset.Collection.Trigger")
	if plan.NeedClarification {
		t.Fatalf("concept explanation should not require clarification, got %q", plan.ClarifyingQuestion)
	}
}

func TestToolDecisionEngineBindsMCPOnboardingEndpointFromUserMessage(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "MCP.Aggregation.Server.Onboard",
		Domain:             DomainExternalMCP,
		Operation:          OpCreate,
		Capability:         "onboard_mcp_server",
		Description:        "Create a governed remote MCP onboarding job.",
		Risk:               ToolRiskHigh,
		DefaultWhitelisted: false,
		RequiresApproval:   true,
		ArgsSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"endpoint_url": map[string]interface{}{"type": "string"}},
			"required":   []string{"endpoint_url"},
		},
	})
	engine := newDecisionTestEngine(registry)
	endpoint := "http://aegis-mcp:8085/mcp"
	query := "把这个接入到" + endpoint + "把这个接入到远程 MCP"
	intent := IntentResult{Domains: []string{"external_mcp"}, Action: "create", Object: "mcp_server", RiskHint: ToolRiskHigh, NeedWrite: true, NeedApproval: true, Confidence: 0.95}
	breakdown := makeDecisionBreakdown(t, query, intent, nil, []string{"onboard_mcp_server"})
	plan, err := engine.Decide(context.Background(), ToolDecisionInput{Query: query, Intent: intent, Breakdown: breakdown})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	step := findPlanStep(plan, "MCP.Aggregation.Server.Onboard")
	if step == nil {
		t.Fatalf("expected onboarding step, got tools=%v rejected=%#v", plan.ToolNames(), plan.RejectedToolRecords)
	}
	if got := step.Args["endpoint_url"]; got != endpoint {
		t.Fatalf("endpoint_url = %#v, want %q", got, endpoint)
	}
	if source := step.ArgSources["endpoint_url"]; source.SourceType != "user_message" {
		t.Fatalf("endpoint_url source = %#v, want user_message", source)
	}
	if err := NewCompiledPlanValidator(registry, nil).Validate(plan, nil); err != nil {
		t.Fatalf("onboarding plan must validate without a previous-step dependency: %v", err)
	}
}

func TestToolDecisionEngineBindsMCPOnboardingStatusFromPreviousStep(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "MCP.Aggregation.Server.Onboard",
		Domain:             DomainExternalMCP,
		Operation:          OpCreate,
		Capability:         "onboard_mcp_server",
		Description:        "Create a governed remote MCP onboarding job.",
		Risk:               ToolRiskHigh,
		DefaultWhitelisted: false,
		RequiresApproval:   true,
		ExecutionContract: ToolExecutionContract{
			Mode:                 ToolExecutionAsynchronous,
			CompletionCapability: "get_mcp_onboarding_status",
		},
		ResultContract: ToolResultContract{OperationRefFields: []string{"job_id"}},
		ArgsSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"endpoint_url": map[string]interface{}{"type": "string"}},
			"required":   []string{"endpoint_url"},
		},
	})
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "MCP.Aggregation.Server.Onboarding.Get",
		Domain:             DomainExternalMCP,
		Operation:          OpGet,
		Capability:         "get_mcp_onboarding_status",
		Description:        "Get the status of an MCP onboarding job.",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"job_id": map[string]interface{}{"type": "string", "format": "uuid"}},
			"required":   []string{"job_id"},
		},
	})

	query := "把这个接入到http://aegis-mcp:8085/mcp"
	intent := IntentResult{Domains: []string{"external_mcp"}, Action: "create", Object: "mcp_server", RiskHint: ToolRiskHigh, NeedWrite: true, NeedApproval: true, Confidence: 0.95}
	breakdown := makeDecisionBreakdown(t, query, intent, nil, []string{"onboard_mcp_server", "get_mcp_onboarding_status"})
	plan, err := newDecisionTestEngine(registry).Decide(context.Background(), ToolDecisionInput{Query: query, Intent: intent, Breakdown: breakdown})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	status := findPlanStep(plan, "MCP.Aggregation.Server.Onboarding.Get")
	if status == nil {
		t.Fatalf("expected onboarding status step, got tools=%v rejected=%#v", plan.ToolNames(), plan.RejectedToolRecords)
	}
	if source := status.ArgSources["job_id"]; source.SourceType != "previous_step" {
		t.Fatalf("job_id source = %#v, want previous_step", source)
	}
	if _, ok := status.Args["job_id"]; ok {
		t.Fatalf("job_id must be injected from the onboarding outcome, got compiled args %#v", status.Args)
	}
	if err := NewCompiledPlanValidator(registry, nil).Validate(plan, nil); err != nil {
		t.Fatalf("onboarding status dependency must validate against job_id result: %v", err)
	}
}

func TestToolDecisionEngineDoesNotBindAgentGuardScopeToMissingPreviousStep(t *testing.T) {
	registry := NewToolRegistry()
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "AgentGuard.Posture.Assess",
		Domain:             DomainAgentGuard,
		Operation:          OpGet,
		Capability:         "assess_agent_guard_posture",
		Description:        "Assess Agent Guard posture.",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		ArgsSchema:         map[string]interface{}{"type": "object"},
	})
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "AgentGuard.Scope.Investigate",
		Domain:             DomainAgentGuard,
		Operation:          OpGet,
		Capability:         "investigate_agent_guard_scope",
		Description:        "Investigate one exact Agent Guard scope.",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"scope_type": map[string]interface{}{"type": "string"}, "scope_id": map[string]interface{}{"type": "string"}},
			"required":   []string{"scope_type", "scope_id"},
		},
	})
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "AgentGuard.Evidence.List",
		Domain:             DomainAgentGuard,
		Operation:          OpList,
		Capability:         "list_agent_guard_evidence",
		Description:        "List Agent Guard evidence.",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"kind": map[string]interface{}{"type": "string"}},
			"required":   []string{"kind"},
		},
	})
	engine := newDecisionTestEngine(registry)
	intent := IntentResult{Domains: []string{"agent_guard"}, Action: "query", Object: "ai_agent", RiskHint: ToolRiskReadonly, Confidence: 0.9}

	t.Run("missing exact scope is rejected instead of becoming previous_step", func(t *testing.T) {
		breakdown := makeDecisionBreakdown(t, "分析智能体安全问题", intent, nil, []string{"assess_agent_guard_posture", "investigate_agent_guard_scope", "list_agent_guard_evidence"})
		plan, err := engine.Decide(context.Background(), ToolDecisionInput{Query: "分析智能体安全问题", Intent: intent, Breakdown: breakdown})
		if err != nil {
			t.Fatalf("Decide returned error: %v", err)
		}
		if containsExactString(plan.ToolNames(), "AgentGuard.Scope.Investigate") {
			t.Fatalf("scope investigation must not be authorized without an exact reference: %#v", plan.ToolNames())
		}
		assertRejectedDecision(t, plan, "AgentGuard.Scope.Investigate")
		assertRejectedDecision(t, plan, "AgentGuard.Evidence.List")
		if err := NewCompiledPlanValidator(registry, nil).Validate(plan, nil); err != nil {
			t.Fatalf("authorized plan must remain valid after scope rejection: %v", err)
		}
	})

	t.Run("exact scope is deterministically bound", func(t *testing.T) {
		breakdown := makeDecisionBreakdown(t, "调查 finding f-123", intent, nil, []string{"investigate_agent_guard_scope"})
		breakdown.Parameters = IntentParameters{"scope_type": "finding", "scope_id": "f-123"}
		plan, err := engine.Decide(context.Background(), ToolDecisionInput{Query: "调查 finding f-123", Intent: intent, Breakdown: breakdown})
		if err != nil {
			t.Fatalf("Decide returned error: %v", err)
		}
		step := findPlanStep(plan, "AgentGuard.Scope.Investigate")
		if step == nil {
			t.Fatalf("expected exact scope step, got %#v", plan.ToolNames())
		}
		if step.Args["scope_type"] != "finding" || step.Args["scope_id"] != "f-123" {
			t.Fatalf("scope args = %#v", step.Args)
		}
		if step.ArgSources["scope_type"].SourceType == "previous_step" || step.ArgSources["scope_id"].SourceType == "previous_step" {
			t.Fatalf("exact scope args must not depend on previous_step: %#v", step.ArgSources)
		}
		if err := NewCompiledPlanValidator(registry, nil).Validate(plan, nil); err != nil {
			t.Fatalf("exact scope plan must validate: %v", err)
		}
	})
}

func TestToolDecisionEngineDoesNotAuthorizeCompanionsOfRejectedProducer(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	for _, tool := range []*ToolSpec{
		{
			Name:               "Example.Generate",
			Domain:             DomainPackage,
			Operation:          OpGenerate,
			Capability:         "generate_example",
			Description:        "Generate an example artifact.",
			Risk:               ToolRiskMedium,
			DefaultWhitelisted: false,
			ExecutionContract: ToolExecutionContract{
				CompletionCapability:  "get_example_status",
				DiscoveryCapabilities: []string{"list_examples"},
			},
		},
		{
			Name:               "Example.Status",
			Domain:             DomainPackage,
			Operation:          OpGet,
			Capability:         "get_example_status",
			Description:        "Get example generation status.",
			Risk:               ToolRiskReadonly,
			DefaultWhitelisted: true,
		},
		{
			Name:               "Example.List",
			Domain:             DomainPackage,
			Operation:          OpList,
			Capability:         "list_examples",
			Description:        "List example artifacts.",
			Risk:               ToolRiskReadonly,
			DefaultWhitelisted: true,
		},
	} {
		registerDecisionTestTool(t, registry, tool)
	}

	intent := IntentResult{Domains: []string{"package"}, Action: "query", Object: "package", RiskHint: ToolRiskReadonly, Confidence: 0.9}
	breakdown := makeDecisionBreakdown(t, "explain example generation", intent, nil, []string{"generate_example"})
	breakdown.RequiresWrite = false

	plan, err := newDecisionTestEngine(registry).Decide(context.Background(), ToolDecisionInput{
		Query:     "explain example generation",
		Intent:    intent,
		Breakdown: breakdown,
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	for _, name := range []string{"Example.Generate", "Example.Status", "Example.List"} {
		assertNotContainsTool(t, plan.ToolNames(), name)
	}
}

func TestToolDecisionEngineBuildsDynamicDetectionPackageDraftPlan(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "Package.Draft.Generate",
		Domain:             DomainPackage,
		Operation:          OpGenerate,
		Capability:         "generate_detection_package_draft",
		Description:        "Generate a dynamic detection package draft.",
		ObjectTypes:        []string{"detection_package", "package"},
		Risk:               ToolRiskMedium,
		DefaultWhitelisted: false,
		ResultContract: ToolResultContract{
			OperationRefFields: []string{"package_id"},
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"cve_id":                    map[string]interface{}{"type": "string"},
				"vulnerability_description": map[string]interface{}{"type": "string"},
				"exploitation_chain":        map[string]interface{}{"type": "string"},
			},
			"required":             []string{"cve_id", "vulnerability_description"},
			"additionalProperties": false,
		},
	})
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "Package.Build.Start",
		Domain:             DomainPackage,
		Operation:          OpExecute,
		Capability:         "start_detection_package_build",
		Description:        "Build a dynamic detection package.",
		ObjectTypes:        []string{"detection_package", "package"},
		Risk:               ToolRiskMedium,
		DefaultWhitelisted: false,
		ExecutionContract: ToolExecutionContract{
			Mode:                 ToolExecutionAsynchronous,
			CompletionCapability: "get_detection_package_build_status",
		},
		ResultContract: ToolResultContract{OperationRefFields: []string{"package_id"}},
		ArgsSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"package_id": map[string]interface{}{"type": "string"}},
			"required":   []string{"package_id"},
		},
	})
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "Package.Build.Status",
		Domain:             DomainPackage,
		Operation:          OpGet,
		Capability:         "get_detection_package_build_status",
		Description:        "Get dynamic detection package build status.",
		ObjectTypes:        []string{"detection_package", "package"},
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		ResultContract: ToolResultContract{
			SatisfiesCapabilities: []string{"start_detection_package_build"},
		},
		ArgsSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"package_id": map[string]interface{}{"type": "string"}},
			"required":   []string{"package_id"},
		},
	})

	query := "CVE-2026-31431 通过 AF_ALG、pipe 和 splice 实现本地提权，请使用动态检测包检测"
	breakdown := &IntentBreakdown{
		Goal:                  "Detect CVE-2026-31431 using a dynamic detection package",
		Actions:               []string{"detect"},
		Objects:               []IntentObject{{Type: "cve", ID: "CVE-2026-31431"}, {Type: "dynamic_detection"}},
		Parameters:            IntentParameters{"cve_id": "CVE-2026-31431", "detection_method": "dynamic_detection_package"},
		WorkflowIDs:           []string{detectionPackageLifecycleWorkflowID},
		CandidateCapabilities: []string{"resolve_hosts", "generate_vulnerability_script"},
		RiskHint:              string(ToolRiskReadonly),
		Confidence:            0.9,
	}
	normalizeIntentBreakdown(breakdown, IntentDecomposeInput{Query: query})

	plan, err := newDecisionTestEngine(registry).Decide(context.Background(), ToolDecisionInput{
		Query:     query,
		Intent:    IntentResult{Domains: []string{"cybersecurity"}, Action: "detect", Confidence: 0.9},
		Breakdown: breakdown,
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if plan.NeedClarification {
		t.Fatalf("dynamic package request should compile without host clarification: %q", plan.ClarifyingQuestion)
	}
	if got, want := plan.ToolNames(), []string{"Package.Draft.Generate", "Package.Build.Start", "Package.Build.Status"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected dynamic package plan: got %v want %v records=%#v capabilities=%#v", got, want, plan.DecisionRecords, breakdown.CandidateCapabilities)
	}
	if plan.RequiredOutcome != "detection_package_enabled" {
		t.Fatalf("required outcome = %q, want detection_package_enabled", plan.RequiredOutcome)
	}
	step := plan.Steps[0]
	if step.Args["cve_id"] != "CVE-2026-31431" || step.Args["vulnerability_description"] != query || step.Args["exploitation_chain"] != query {
		t.Fatalf("dynamic package args were not deterministically bound: %#v", step.Args)
	}
}

func TestToolDecisionEngineAuthorizesOnlySelectedToolsWithoutWorkflowExpansion(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	for _, spec := range []*ToolSpec{
		{Name: "Asset.Collection.Trigger", Domain: DomainAsset, Operation: OpExecute, Capability: "trigger_asset_collection", Description: "触发资产采集", ObjectTypes: []string{"asset", "host"}, Risk: ToolRiskMedium, DefaultWhitelisted: false},
		{Name: "Unrelated.Task.Get", Domain: DomainTask, Operation: OpGet, Capability: "get_unrelated_task", Description: "查询无关任务", ObjectTypes: []string{"task"}, Risk: ToolRiskReadonly, DefaultWhitelisted: true},
	} {
		registerDecisionTestTool(t, registry, spec)
	}

	query := "对全部在线主机做资产采集并分析 MySQL 漏洞"
	intent := IntentResult{Domains: []string{"asset", "vulnerability"}, Action: "analyze", Object: "asset", NeedWrite: true, RiskHint: ToolRiskMedium, Confidence: 0.8}
	breakdown := makeDecisionBreakdown(t, query, intent, nil, []string{"trigger_asset_collection"})
	breakdown.Scope = IntentScope{Kind: "online_hosts", Source: "llm"}
	engine := newDecisionTestEngine(registry)

	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:     query,
		Intent:    intent,
		Breakdown: breakdown,
		PreliminarySelection: &ToolSelectionResult{
			SelectedTools:  []string{"Asset.Collection.Trigger"},
			CandidateTools: []string{"Asset.Collection.Trigger"},
		},
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if got := plan.ToolNames(); len(got) != 1 || got[0] != "Asset.Collection.Trigger" {
		t.Fatalf("authorization layer must not expand a scenario workflow, got %v", got)
	}
	step := findPlanStep(plan, "Asset.Collection.Trigger")
	if step == nil || !step.RequiresApproval {
		t.Fatalf("Asset.Collection.Trigger should require approval, got %#v", step)
	}
}

func TestToolDecisionEngineOrdersSigmaImportBeforeEnableAndBindsRealRuleID(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	for _, spec := range []*ToolSpec{
		{
			Name: "SigmaRule.Import", Domain: DomainSigmaRule, Operation: OpCreate,
			Capability: "import_sigma_rule", Description: "Import a Sigma rule.",
			ObjectTypes: []string{"file", "sigma_rule"}, Risk: ToolRiskMedium,
			DefaultWhitelisted: false,
			ArgsSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_id": map[string]interface{}{"type": "string"},
				},
				"required": []string{"file_id"},
			},
			ResultContract: ToolResultContract{OperationRefFields: []string{"rule_id"}},
		},
		{
			Name: "SigmaRule.Enable", Domain: DomainSigmaRule, Operation: OpUpdate,
			Capability: "enable_sigma_rule", Description: "Enable a Sigma rule.",
			ObjectTypes: []string{"sigma_rule"}, Risk: ToolRiskMedium,
			DefaultWhitelisted: false,
			ArgsSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"rule_id": map[string]interface{}{"type": "string"},
				},
				"required": []string{"rule_id"},
			},
		},
	} {
		registerDecisionTestTool(t, registry, spec)
	}
	breakdown := makeDecisionBreakdown(t,
		"解析附件并开启这个规则的检测",
		IntentResult{Domains: []string{"sigma_rule"}, Action: "enable", Object: "sigma_rule", NeedWrite: true, RiskHint: ToolRiskMedium, Confidence: 0.9},
		nil,
		[]string{"enable_sigma_rule", "import_sigma_rule"},
	)
	breakdown.Objects = []IntentObject{
		{Type: "file", ID: "file-1"},
		{Type: "sigma_rule", ID: "rule-from-yaml"},
	}
	plan, err := newDecisionTestEngine(registry).Decide(context.Background(), ToolDecisionInput{
		Query:       "解析附件并开启这个规则的检测",
		Intent:      IntentResult{Domains: []string{"sigma_rule"}, Action: "enable", Object: "sigma_rule", NeedWrite: true, RiskHint: ToolRiskMedium, Confidence: 0.9},
		Breakdown:   breakdown,
		ContextRefs: []ContextRefInput{{ObjectType: "file", ObjectID: "file-1", Title: "rule.yml", Summary: "id: rule-from-yaml"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.ToolNames(); len(got) != 2 || got[0] != "SigmaRule.Import" || got[1] != "SigmaRule.Enable" {
		t.Fatalf("sigma lifecycle order = %v", got)
	}
	enable := findPlanStep(plan, "SigmaRule.Enable")
	if enable == nil {
		t.Fatal("SigmaRule.Enable step missing")
	}
	if _, exists := enable.Args["rule_id"]; exists {
		t.Fatalf("enable must not reuse an unpersisted YAML rule ID: %#v", enable.Args)
	}
	if source := enable.ArgSources["rule_id"]; source.SourceType != "previous_step" {
		t.Fatalf("enable rule_id source = %#v, want previous_step", source)
	}
}

func TestToolDecisionEngineDoesNotUseDomainRecallOrWrongPreselection(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	for _, spec := range []*ToolSpec{
		{Name: "Host.List", Domain: DomainHost, Operation: OpList, Capability: "list_hosts", Description: "list hosts", ObjectTypes: []string{"host"}, Risk: ToolRiskReadonly, DefaultWhitelisted: true},
		{Name: "Host.AgentStatus.Get", Domain: DomainHost, Operation: OpGet, Capability: "get_agent_status", Description: "agent status statistics", ObjectTypes: []string{"host"}, Risk: ToolRiskReadonly, DefaultWhitelisted: true},
	} {
		registerDecisionTestTool(t, registry, spec)
	}
	intent := IntentResult{Domains: []string{"host"}, Action: "list", Object: "host", RiskHint: ToolRiskReadonly, Confidence: 0.9}
	breakdown := makeDecisionBreakdown(t, "list online hosts", intent, nil, []string{"query_host_list"})

	authorization, err := newDecisionTestEngine(registry).Decide(context.Background(), ToolDecisionInput{
		Query:     "list online hosts",
		Intent:    intent,
		Breakdown: breakdown,
		PreliminarySelection: &ToolSelectionResult{
			SelectedTools: []string{"Host.AgentStatus.Get"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(authorization.ToolNames()) != 0 {
		t.Fatalf("unmapped capability and wrong preselection must not authorize tools, got %v", authorization.ToolNames())
	}
}

func TestToolDecisionEngineRoundsExplicitToolThroughCapabilityMapping(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "Example.Apply",
		Domain:             DomainSystem,
		Operation:          OpExecute,
		Capability:         "apply_example",
		Description:        "apply example",
		ObjectTypes:        []string{"example"},
		Risk:               ToolRiskMedium,
		DefaultWhitelisted: false,
	})
	intent := IntentResult{
		Domains:          []string{"system"},
		Action:           "execute",
		Object:           "example",
		NeedWrite:        true,
		RiskHint:         ToolRiskMedium,
		Confidence:       0.9,
		ExplicitToolName: "Example.Apply",
	}
	breakdown := makeDecisionBreakdown(t, "execute Example.Apply", intent, nil, nil)

	plan, err := newDecisionTestEngine(registry).Decide(context.Background(), ToolDecisionInput{
		Query:     "execute Example.Apply",
		Intent:    intent,
		Breakdown: breakdown,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.ToolNames(); len(got) != 1 || got[0] != "Example.Apply" {
		t.Fatalf("explicit tool did not round-trip through Mapping: %v", got)
	}
	step := findPlanStep(plan, "Example.Apply")
	if step == nil || step.Capability != "apply_example" {
		t.Fatalf("mapped capability missing from execution step: %#v", step)
	}
}

func TestToolDecisionEngineAllowsRequiredArgFromPreviousStepForAnyTool(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	for _, spec := range []*ToolSpec{
		{Name: "Example.Discover", Domain: DomainSystem, Operation: OpGet, Capability: "discover_resource", Description: "discover a resource", ObjectTypes: []string{"resource"}, Risk: ToolRiskReadonly, DefaultWhitelisted: true},
		{Name: "Example.Apply", Domain: DomainSystem, Operation: OpExecute, Capability: "apply_resource", Description: "apply the discovered resource", ObjectTypes: []string{"resource"}, Risk: ToolRiskMedium, DefaultWhitelisted: false, ArgsSchema: requiredSchema("resource_id")},
	} {
		registerDecisionTestTool(t, registry, spec)
	}

	query := "发现资源并应用它"
	intent := IntentResult{Domains: []string{"system"}, Action: "execute", Object: "resource", NeedWrite: true, RiskHint: ToolRiskMedium, Confidence: 0.8}
	breakdown := makeDecisionBreakdown(t, query, intent, nil, []string{"discover_resource", "apply_resource"})
	engine := newDecisionTestEngine(registry)

	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:     query,
		Intent:    intent,
		Breakdown: breakdown,
		PreliminarySelection: &ToolSelectionResult{
			SelectedTools:  []string{"Example.Discover", "Example.Apply"},
			CandidateTools: []string{"Example.Discover", "Example.Apply"},
		},
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if plan.NeedClarification {
		t.Fatalf("runtime can resolve resource_id from the previous result, got clarification %q", plan.ClarifyingQuestion)
	}
	for _, want := range []string{"Example.Discover", "Example.Apply"} {
		assertContainsTool(t, plan.ToolNames(), want)
	}
	apply := findPlanStep(plan, "Example.Apply")
	if apply == nil || apply.ArgSources["resource_id"].SourceType != "previous_step" {
		t.Fatalf("expected generic previous-step binding, got %#v", apply)
	}
}

func TestToolDecisionEngineBindsBaselineWorkflowScopeTemplateAndRemediation(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	for _, spec := range []*ToolSpec{
		{Name: "Host.Resolve", Domain: DomainHost, Operation: OpGet, Capability: "resolve_hosts", Description: "resolve hosts", ObjectTypes: []string{"host"}, Risk: ToolRiskReadonly, DefaultWhitelisted: true},
		{Name: "Baseline.Compliance.Run", Domain: DomainBaseline, Operation: OpExecute, Capability: "run_baseline_compliance", Description: "run baseline compliance", ObjectTypes: []string{"host", "baseline_template"}, Risk: ToolRiskHigh, DefaultWhitelisted: false, RequiresApproval: true, ExecutionContract: ToolExecutionContract{Mode: ToolExecutionAsynchronous, CompletionCapability: "get_operation_status"}, ArgsSchema: requiredSchema("template_selector")},
		{Name: "Operation.Get", Domain: DomainSystem, Operation: OpGet, Capability: "get_operation_status", Description: "get operation", ObjectTypes: []string{"operation"}, Risk: ToolRiskReadonly, DefaultWhitelisted: true, ArgsSchema: requiredSchema("operation_id")},
	} {
		registerDecisionTestTool(t, registry, spec)
	}
	intent := IntentResult{Domains: []string{"baseline"}, Action: "execute", NeedWrite: true, RiskHint: ToolRiskHigh, Confidence: 0.95}
	breakdown := makeDecisionBreakdown(t, "scan every online host with CIS Ubuntu and remediate five rounds", intent, nil, []string{"resolve_hosts", "run_baseline_compliance"})
	breakdown.Scope = IntentScope{Kind: "all_alive_hosts", Source: "llm"}
	breakdown.Objects = append(breakdown.Objects, IntentObject{Type: "baseline_template", ID: "CIS_Ubuntu_Linux_24.04_LTS_Benchmark_v2.0.0-12-971.pdf", Source: "llm"})
	breakdown.Parameters = IntentParameters{"auto_remediate": true, "remediation_rounds": float64(5), "baseline_rules": "all"}

	plan, err := newDecisionTestEngine(registry).Decide(context.Background(), ToolDecisionInput{Query: "run baseline", Intent: intent, Breakdown: breakdown})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.ToolNames(); len(got) != 3 || got[0] != "Host.Resolve" || got[1] != "Baseline.Compliance.Run" || got[2] != "Operation.Get" {
		t.Fatalf("unexpected deterministic baseline plan order: %v", got)
	}
	resolve := findPlanStep(plan, "Host.Resolve")
	baseline := findPlanStep(plan, "Baseline.Compliance.Run")
	if resolve.Args["target_scope"] != "all_online_hosts" {
		t.Fatalf("host scope was not normalized: %#v", resolve.Args)
	}
	if baseline.Args["target_scope"] != "all_online_hosts" || baseline.Args["template_selector"] != "CIS_Ubuntu_Linux_24.04_LTS_Benchmark_v2.0.0-12-971.pdf" || baseline.Args["scope"] != "all_rules" {
		t.Fatalf("baseline deterministic args missing: %#v", baseline.Args)
	}
	remediation, ok := baseline.Args["remediation"].(map[string]interface{})
	if !ok || remediation["enabled"] != true || remediation["max_rounds"] != 5 {
		t.Fatalf("remediation policy missing: %#v", baseline.Args)
	}
}

func TestToolDecisionEngineCompilesCapturedBaselineIntentWithoutModelArguments(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	for _, spec := range []*ToolSpec{
		{Name: "Host.Resolve", Domain: DomainHost, Operation: OpGet, Capability: "resolve_hosts", Description: "resolve hosts", ObjectTypes: []string{"host"}, Risk: ToolRiskReadonly, DefaultWhitelisted: true},
		{Name: "Baseline.Compliance.Run", Domain: DomainBaseline, Operation: OpExecute, Capability: "run_baseline_compliance", Description: "run baseline compliance", ObjectTypes: []string{"host", "baseline_template"}, Risk: ToolRiskHigh, DefaultWhitelisted: false, RequiresApproval: true, ExecutionContract: ToolExecutionContract{Mode: ToolExecutionAsynchronous, CompletionCapability: "get_operation_status"}, ArgsSchema: requiredSchema("template_selector")},
		{Name: "Operation.Get", Domain: DomainSystem, Operation: OpGet, Capability: "get_operation_status", Description: "get operation", ObjectTypes: []string{"operation"}, Risk: ToolRiskReadonly, DefaultWhitelisted: true, ArgsSchema: requiredSchema("operation_id")},
	} {
		registerDecisionTestTool(t, registry, spec)
	}
	intent := IntentResult{Domains: []string{"baseline"}, Action: "execute", NeedWrite: true, RiskHint: ToolRiskHigh, Confidence: 0.95}
	breakdown := makeDecisionBreakdown(t, "给存活的机器下发基线并自动修复5轮", intent, nil, []string{"resolve_hosts", "run_baseline_compliance"})
	breakdown.Scope = IntentScope{Kind: "unspecified"}
	breakdown.Objects = []IntentObject{
		{Type: "machine", Selector: "live"},
		{Type: "baseline", ID: "CIS_Ubuntu_Linux_24.04_LTS_Benchmark_v2.0.0-12-971.pdf"},
	}
	breakdown.Parameters = IntentParameters{"auto_repair": true, "repair_rounds": float64(5)}
	breakdown.WorkflowIDs = []string{"baseline_compliance"}
	normalizeIntentBreakdown(breakdown, IntentDecomposeInput{Query: "给存活的机器下发基线并自动修复5轮", Intent: intent})

	plan, err := newDecisionTestEngine(registry).Decide(context.Background(), ToolDecisionInput{Query: "run baseline", Intent: intent, Breakdown: breakdown})
	if err != nil {
		t.Fatal(err)
	}
	if !usesDeterministicBaselineWorkflow(plan) {
		t.Fatalf("captured production intent must produce a fixed baseline plan: %#v", plan)
	}
	baseline := findPlanStep(plan, "Baseline.Compliance.Run")
	remediation, ok := baseline.Args["remediation"].(map[string]interface{})
	if !ok || remediation["enabled"] != true || remediation["max_rounds"] != 5 {
		t.Fatalf("compiled remediation = %#v, want enabled with five rounds", baseline.Args)
	}
}

func TestToolDecisionEngineCompilesLatestProductionBaselineAliases(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	for _, spec := range []*ToolSpec{
		{Name: "Host.Resolve", Domain: DomainHost, Operation: OpGet, Capability: "resolve_hosts", Description: "resolve hosts", ObjectTypes: []string{"host"}, Risk: ToolRiskReadonly, DefaultWhitelisted: true},
		{Name: "Baseline.Compliance.Run", Domain: DomainBaseline, Operation: OpExecute, Capability: "run_baseline_compliance", Description: "run baseline compliance", ObjectTypes: []string{"host", "baseline_template"}, Risk: ToolRiskHigh, DefaultWhitelisted: false, RequiresApproval: true, ExecutionContract: ToolExecutionContract{Mode: ToolExecutionAsynchronous, CompletionCapability: "get_operation_status"}, ArgsSchema: requiredSchema("template_selector")},
		{Name: "Operation.Get", Domain: DomainSystem, Operation: OpGet, Capability: "get_operation_status", Description: "get operation", ObjectTypes: []string{"operation"}, Risk: ToolRiskReadonly, DefaultWhitelisted: true, ArgsSchema: requiredSchema("operation_id")},
	} {
		registerDecisionTestTool(t, registry, spec)
	}
	intent := IntentResult{Domains: []string{"baseline"}, Action: "execute", NeedWrite: true, RiskHint: ToolRiskHigh, Confidence: 0.95}
	breakdown := makeDecisionBreakdown(t, "给存活机器下发基线并自动修复5轮", intent, nil, []string{"resolve_hosts", "run_baseline_compliance"})
	breakdown.Scope = IntentScope{Kind: "all_online_hosts"}
	breakdown.Objects = []IntentObject{
		{Type: "host", Selector: "alive"},
		{Type: "baseline_template", Selector: "CIS_Ubuntu_Linux_24.04_LTS_Benchmark_v2.0.0-12-971.pdf"},
	}
	breakdown.Parameters = IntentParameters{"remediation_enabled": true, "retry_rounds": float64(5)}
	breakdown.WorkflowIDs = []string{"baseline_compliance"}
	normalizeIntentBreakdown(breakdown, IntentDecomposeInput{Query: "给存活机器下发基线并自动修复5轮", Intent: intent})

	plan, err := newDecisionTestEngine(registry).Decide(context.Background(), ToolDecisionInput{
		Query:     "run baseline",
		Intent:    intent,
		Breakdown: breakdown,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !usesDeterministicBaselineWorkflow(plan) {
		t.Fatalf("latest production intent must bind a deterministic baseline plan: %#v", plan)
	}
	baseline := findPlanStep(plan, "Baseline.Compliance.Run")
	if baseline == nil || baseline.Args["template_selector"] != "CIS_Ubuntu_Linux_24.04_LTS_Benchmark_v2.0.0-12-971.pdf" {
		t.Fatalf("template selector was not compiled: %#v", baseline)
	}
	remediation, ok := baseline.Args["remediation"].(map[string]interface{})
	if !ok || remediation["enabled"] != true || remediation["max_rounds"] != 5 {
		t.Fatalf("latest remediation aliases did not preserve enabled five-round repair: %#v", baseline.Args)
	}
}

func TestToolDecisionEngineClarifiesVagueRepair(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "Task.RunFix",
		Domain:             DomainBaseline,
		Operation:          OpExecute,
		Description:        "触发基线修复",
		ObjectTypes:        []string{"baseline_rule", "host"},
		Risk:               ToolRiskHigh,
		DefaultWhitelisted: false,
		ArgsSchema:         requiredSchema("rule_ids", "host_ids"),
	})
	intent := IntentResult{Domains: []string{"baseline"}, Action: "execute", NeedWrite: true, RiskHint: ToolRiskHigh, Confidence: 0.5}
	breakdown := makeDecisionBreakdown(t, "帮我修复一下", intent, nil, nil)
	breakdown.NeedClarification = true
	breakdown.MissingInfo = []MissingInfo{{Field: "target", Reason: "repair target is missing", Question: "请确认要修复的对象和范围。"}}
	breakdown.ClarifyingQuestion = "请确认要修复的对象和范围。"
	engine := newDecisionTestEngine(registry)

	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:     "帮我修复一下",
		Intent:    intent,
		Breakdown: breakdown,
		PreliminarySelection: &ToolSelectionResult{
			SelectedTools:  []string{"Task.RunFix"},
			CandidateTools: []string{"Task.RunFix"},
		},
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if !plan.NeedClarification || plan.ClarifyingQuestion == "" {
		t.Fatalf("expected clarification, got %#v", plan)
	}
	if !containsSubstring(plan.ClarifyingQuestion, "修复的对象") {
		t.Fatalf("expected original repair clarification question, got %q", plan.ClarifyingQuestion)
	}
	assertNotContainsTool(t, plan.ToolNames(), "Task.RunFix")
}

func TestToolDecisionEngineClarifiesBlockWithoutAlert(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "Detection.Alert.Block",
		Domain:             DomainDetection,
		Operation:          OpExecute,
		Capability:         "block_detection_alert",
		Description:        "阻断告警",
		ObjectTypes:        []string{"alert", "detection"},
		Risk:               ToolRiskCritical,
		DefaultWhitelisted: false,
		ArgsSchema:         requiredSchema("alert_id"),
	})
	intent := IntentResult{Domains: []string{"detection"}, Action: "block", Object: "alert", NeedWrite: true, RiskHint: ToolRiskCritical, Confidence: 0.8}
	breakdown := makeDecisionBreakdown(t, "阻断这个告警", intent, nil, []string{"block_detection_alert"})
	engine := newDecisionTestEngine(registry)

	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:     "阻断这个告警",
		Intent:    intent,
		Breakdown: breakdown,
		PreliminarySelection: &ToolSelectionResult{
			SelectedTools:  []string{"Detection.Alert.Block"},
			CandidateTools: []string{"Detection.Alert.Block"},
		},
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if !plan.NeedClarification {
		t.Fatalf("expected clarification, got %#v", plan)
	}
}

func TestToolDecisionEngineAllowsBlockWithAlertContext(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	for _, spec := range []*ToolSpec{
		{Name: "Detection.Alert.Get", Domain: DomainDetection, Operation: OpGet, Capability: "get_detection_alert", Description: "查询告警", ObjectTypes: []string{"alert"}, Risk: ToolRiskReadonly, DefaultWhitelisted: true, ArgsSchema: requiredSchema("alert_id")},
		{Name: "Detection.Alert.Block", Domain: DomainDetection, Operation: OpExecute, Capability: "block_detection_alert", Description: "阻断告警", ObjectTypes: []string{"alert", "detection"}, Risk: ToolRiskCritical, DefaultWhitelisted: false, ArgsSchema: requiredSchema("alert_id")},
	} {
		registerDecisionTestTool(t, registry, spec)
	}
	refs := []ContextRefInput{{ObjectType: "alert", ObjectID: "alert-001"}}
	intent := IntentResult{Domains: []string{"detection"}, Action: "block", Object: "alert", NeedWrite: true, RiskHint: ToolRiskCritical, Confidence: 0.8}
	breakdown := makeDecisionBreakdown(t, "阻断这个告警", intent, refs, []string{"block_detection_alert"})
	engine := newDecisionTestEngine(registry)

	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:       "阻断这个告警",
		Intent:      intent,
		Breakdown:   breakdown,
		ContextRefs: refs,
		PreliminarySelection: &ToolSelectionResult{
			SelectedTools:  []string{"Detection.Alert.Block"},
			CandidateTools: []string{"Detection.Alert.Block"},
		},
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if plan.NeedClarification {
		t.Fatalf("did not expect clarification: %#v", plan)
	}
	step := findPlanStep(plan, "Detection.Alert.Block")
	if step == nil {
		t.Fatalf("expected Detection.Alert.Block in plan, got %v", plan.ToolNames())
	}
	if step.Args["alert_id"] != "alert-001" {
		t.Fatalf("expected alert_id from context, got args %#v", step.Args)
	}
	if !step.RequiresApproval {
		t.Fatalf("expected block step to require approval")
	}
}

func TestToolDecisionEngineRecordsUnmappedCapability(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	intent := IntentResult{Domains: []string{"host"}, Action: "query", Confidence: 0.8}
	breakdown := makeDecisionBreakdown(t, "查询主机", intent, nil, []string{"delete_everything"})
	engine := newDecisionTestEngine(registry)

	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:     "查询主机",
		Intent:    intent,
		Breakdown: breakdown,
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	for _, record := range plan.RejectedToolRecords {
		if record.Capability == "delete_everything" && record.Decision == toolDecisionRejected {
			return
		}
	}
	t.Fatalf("expected rejected record for unmapped capability, got %#v", plan.RejectedToolRecords)
}

func newDecisionTestRegistry(t *testing.T) *ToolRegistry {
	t.Helper()
	registry := NewToolRegistry()
	for _, spec := range []*ToolSpec{
		{Name: "Tool.Search", Domain: DomainSystem, Operation: OpSearch, Capability: "search_available_tools", Description: "搜索工具", Risk: ToolRiskReadonly, DefaultWhitelisted: true},
		{Name: "Context.Get", Domain: DomainSystem, Operation: OpGet, Capability: "get_session_context", Description: "查询上下文", Risk: ToolRiskReadonly, DefaultWhitelisted: true},
		{Name: "Session.Summarize", Domain: DomainSystem, Operation: OpGet, Capability: "summarize_session", Description: "总结会话", Risk: ToolRiskReadonly, DefaultWhitelisted: true},
	} {
		registerDecisionTestTool(t, registry, spec)
	}
	return registry
}

func registerDecisionTestTool(t *testing.T, registry *ToolRegistry, spec *ToolSpec) {
	t.Helper()
	spec.Enabled = true
	spec.Handler = func(context.Context, map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"ok": true}, nil
	}
	if err := registry.Register(spec); err != nil {
		t.Fatalf("register %s: %v", spec.Name, err)
	}
}

func newDecisionTestEngine(registry *ToolRegistry) *ToolDecisionEngine {
	return NewToolDecisionEngine(ToolDecisionEngineDeps{
		Registry: registry,
		Mapper:   NewToolCapabilityMapper(registry),
		Config: ToolDecisionConfig{
			Enabled:                    true,
			ClarificationRequiredWrite: true,
			PostconditionCheckEnabled:  true,
		},
	})
}

func makeDecisionBreakdown(t *testing.T, query string, intent IntentResult, refs []ContextRefInput, capabilities []string) *IntentBreakdown {
	t.Helper()
	action := intent.Action
	if action == "" {
		action = "query"
	}
	objects := make([]IntentObject, 0, len(refs)+2)
	if intent.Object != "" {
		objects = append(objects, IntentObject{Type: intent.Object, Source: "llm"})
	}
	for _, ref := range refs {
		objects = append(objects, IntentObject{Type: ref.ObjectType, ID: ref.ObjectID, Source: "context_ref"})
	}
	scope := IntentScope{Kind: "unspecified"}
	normalizedCapabilities := append([]string{}, capabilities...)
	return &IntentBreakdown{
		Goal:                  query,
		Domains:               append([]string{}, intent.Domains...),
		Actions:               []string{action},
		Objects:               objects,
		Scope:                 scope,
		RequiresWrite:         intent.NeedWrite,
		RiskHint:              string(intent.RiskHint),
		CandidateCapabilities: normalizedCapabilities,
		Reason:                intent.Reason,
		Confidence:            intent.Confidence,
	}
}

func requiredSchema(args ...string) map[string]interface{} {
	required := make([]string, len(args))
	copy(required, args)
	return map[string]interface{}{"required": required}
}

func assertRejectedDecision(t *testing.T, plan *ToolExecutionPlan, toolName string) {
	t.Helper()
	for _, record := range plan.RejectedToolRecords {
		if record.ToolName == toolName && record.Decision != toolDecisionAccepted {
			return
		}
	}
	t.Fatalf("expected rejected decision for %s, got %#v", toolName, plan.RejectedToolRecords)
}

func assertNotContainsTool(t *testing.T, names []string, unwanted string) {
	t.Helper()
	for _, name := range names {
		if name == unwanted {
			t.Fatalf("expected tools not to contain %s, got %v", unwanted, names)
		}
	}
}

func assertContainsTool(t *testing.T, names []string, want string) {
	t.Helper()
	for _, name := range names {
		if name == want {
			return
		}
	}
	t.Fatalf("expected tools to contain %s, got %v", want, names)
}

func findPlanStep(plan *ToolExecutionPlan, toolName string) *ToolPlanStep {
	if plan == nil {
		return nil
	}
	for i := range plan.Steps {
		if plan.Steps[i].ToolName == toolName {
			return &plan.Steps[i]
		}
	}
	return nil
}

func findPlanSteps(plan *ToolExecutionPlan, toolName string) []ToolPlanStep {
	if plan == nil {
		return nil
	}
	steps := make([]ToolPlanStep, 0, 2)
	for _, step := range plan.Steps {
		if step.ToolName == toolName {
			steps = append(steps, step)
		}
	}
	return steps
}

// ---- 设计文档 11.1 节补充测试用例 ----

// 只读主机查询：应命中 Host.List，无审批
func TestToolDecisionEngineReadOnlyHostQuery(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "Host.List",
		Domain:             DomainHost,
		Operation:          OpList,
		Capability:         "list_hosts",
		Description:        "查询主机列表",
		ObjectTypes:        []string{"host"},
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
	})

	intent := IntentResult{Domains: []string{"host"}, Action: "query", Object: "host", RiskHint: ToolRiskReadonly, Confidence: 0.9}
	breakdown := makeDecisionBreakdown(t, "有哪些在线主机", intent, nil, []string{"list_hosts"})
	engine := newDecisionTestEngine(registry)

	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:     "有哪些在线主机",
		Intent:    intent,
		Breakdown: breakdown,
		PreliminarySelection: &ToolSelectionResult{
			SelectedTools:  []string{"Host.List"},
			CandidateTools: []string{"Host.List"},
		},
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if plan.NeedClarification {
		t.Fatalf("read-only query should not need clarification, got %q", plan.ClarifyingQuestion)
	}
	step := findPlanStep(plan, "Host.List")
	if step == nil {
		t.Fatalf("expected Host.List in plan, got %v", plan.ToolNames())
	}
	if step.RequiresApproval {
		t.Fatalf("Host.List should not require approval")
	}
	if step.Risk != string(ToolRiskReadonly) {
		t.Fatalf("expected readonly risk, got %q", step.Risk)
	}
}

// 评分不足：候选工具相似但对象不匹配，应拒绝或追问，不执行写工具
func TestToolDecisionEngineRejectsUnmappedCapabilityWithoutPreselectionBypass(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "Detection.Alert.Block",
		Domain:             DomainDetection,
		Operation:          OpExecute,
		Capability:         "block_detection_alert",
		Description:        "阻断告警",
		ObjectTypes:        []string{"alert", "detection"},
		Risk:               ToolRiskCritical,
		DefaultWhitelisted: false,
		ArgsSchema:         requiredSchema("alert_id"),
	})

	// 查询主机，但候选工具是告警阻断（对象不匹配）
	intent := IntentResult{Domains: []string{"host"}, Action: "query", Object: "host", RiskHint: ToolRiskReadonly, Confidence: 0.8}
	breakdown := makeDecisionBreakdown(t, "查询主机列表", intent, nil, []string{"list_hosts"})
	engine := newDecisionTestEngine(registry)

	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:     "查询主机列表",
		Intent:    intent,
		Breakdown: breakdown,
		PreliminarySelection: &ToolSelectionResult{
			SelectedTools:  []string{"Detection.Alert.Block"},
			CandidateTools: []string{"Detection.Alert.Block"},
		},
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	// 预选工具不属于精确 capability mapping，因此不能进入授权集合。
	assertNotContainsTool(t, plan.ToolNames(), "Detection.Alert.Block")
	if len(plan.RejectedToolRecords) == 0 || plan.RejectedToolRecords[0].Capability != "list_hosts" {
		t.Fatalf("expected an unmapped capability rejection, got %#v", plan.RejectedToolRecords)
	}
}

// 概念解释：明确的 denied_intents 匹配检查
func TestToolDecisionEngineDeniedIntentMatch(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	registerDecisionTestTool(t, registry, &ToolSpec{
		Name:               "Asset.Collection.Trigger",
		Domain:             DomainAsset,
		Operation:          OpExecute,
		Capability:         "trigger_asset_collection",
		Description:        "触发资产采集",
		ObjectTypes:        []string{"asset", "host"},
		Risk:               ToolRiskMedium,
		DefaultWhitelisted: false,
	})

	intent := IntentResult{Domains: []string{"asset"}, Action: "query", Object: "asset", RiskHint: ToolRiskReadonly, Confidence: 0.8}
	// 明确的候选能力命中 denied_intents（explain_asset_collection 不在 denied 中，但 query_asset_collection_history 在）
	breakdown := makeDecisionBreakdown(t, "资产采集的历史记录", intent, nil, []string{"query_asset_collection_history"})
	engine := newDecisionTestEngine(registry)

	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:     "资产采集的历史记录",
		Intent:    intent,
		Breakdown: breakdown,
		PreliminarySelection: &ToolSelectionResult{
			SelectedTools:  []string{"Asset.Collection.Trigger"},
			CandidateTools: []string{"Asset.Collection.Trigger"},
		},
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	// 候选能力 query_asset_collection_history 命中 denied_intents，应拒绝
	assertNotContainsTool(t, plan.ToolNames(), "Asset.Collection.Trigger")
}

// 状态机越级：未创建任务直接查询采集任务结果
func TestToolDecisionEngineStateMachineViolation(t *testing.T) {
	registry := newDecisionTestRegistry(t)
	for _, spec := range []*ToolSpec{
		{Name: "Asset.Collection.Get", Domain: DomainAsset, Operation: OpGet, Capability: "get_asset_collection_task", Description: "查询采集任务", ObjectTypes: []string{"asset_collection"}, Risk: ToolRiskReadonly, DefaultWhitelisted: true, ArgsSchema: requiredSchema("task_id")},
	} {
		registerDecisionTestTool(t, registry, spec)
	}

	intent := IntentResult{Domains: []string{"asset"}, Action: "query", Object: "asset", RiskHint: ToolRiskReadonly, Confidence: 0.8}
	// 不提供 task_id 上下文，也没有前置步骤
	breakdown := makeDecisionBreakdown(t, "查看采集任务结果", intent, nil, []string{"get_asset_collection_task"})
	engine := newDecisionTestEngine(registry)

	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:     "查看采集任务结果",
		Intent:    intent,
		Breakdown: breakdown,
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	// 缺少 task_id，应进入追问
	if !plan.NeedClarification {
		t.Fatalf("expected clarification for missing task_id, got plan with tools: %v", plan.ToolNames())
	}
}

// ToolResultVerifier 测试
func TestToolResultVerifierPassesWithValidResult(t *testing.T) {
	verifier := NewToolResultVerifier(nil)
	step := ToolPlanStep{
		ToolName:       "Asset.Collection.Trigger",
		Postconditions: []string{"task_id_created"},
	}
	result := ToolExecutionResult{
		Success: true,
		Data:    map[string]interface{}{"task_id": "task-001"},
	}
	verifyResult := verifier.Verify(context.Background(), step, result)
	if !verifyResult.Passed {
		t.Fatalf("expected verification to pass, got violations: %v", verifyResult.Violations)
	}
}

func TestToolResultVerifierFailsWithMissingTaskID(t *testing.T) {
	verifier := NewToolResultVerifier(nil)
	step := ToolPlanStep{
		ToolName:       "Asset.Collection.Trigger",
		Postconditions: []string{"task_id_created"},
	}
	result := ToolExecutionResult{
		Success: true,
		Data:    map[string]interface{}{"status": "ok"},
	}
	verifyResult := verifier.Verify(context.Background(), step, result)
	if verifyResult.Passed {
		t.Fatalf("expected verification to fail for missing task_id")
	}
	if len(verifyResult.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(verifyResult.Violations))
	}
}

func TestToolResultVerifierFailsWithExecutionError(t *testing.T) {
	verifier := NewToolResultVerifier(nil)
	step := ToolPlanStep{
		ToolName:       "Asset.Collection.Trigger",
		Postconditions: []string{"task_id_created"},
	}
	result := ToolExecutionResult{
		Success: false,
		Error:   "connection timeout",
	}
	verifyResult := verifier.Verify(context.Background(), step, result)
	if verifyResult.Passed {
		t.Fatalf("expected verification to fail for failed execution")
	}
}

func TestToolResultVerifierPassesWithNoPostconditions(t *testing.T) {
	verifier := NewToolResultVerifier(nil)
	step := ToolPlanStep{
		ToolName: "Host.List",
	}
	result := ToolExecutionResult{
		Success: true,
		Data:    map[string]interface{}{"hosts": []string{}},
	}
	verifyResult := verifier.Verify(context.Background(), step, result)
	if !verifyResult.Passed {
		t.Fatalf("expected verification to pass with no postconditions, got: %v", verifyResult)
	}
}

func TestToolResultVerifierRejectsUnknownPostcondition(t *testing.T) {
	verifier := NewToolResultVerifier(nil)
	result := verifier.Verify(context.Background(), ToolPlanStep{
		StepID: "step-unknown", ToolName: "Example.Tool", Postconditions: []string{"unknown_condition"},
	}, ToolExecutionResult{Success: true, Data: map[string]interface{}{"ok": true}})
	if result.Passed {
		t.Fatalf("expected unknown postcondition to fail: %#v", result)
	}
}
