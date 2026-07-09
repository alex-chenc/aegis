package assistant

import (
	"context"
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

func TestToolDecisionEngineRecallsRelevantContractBeyondWrongPreselection(t *testing.T) {
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
	assertContainsTool(t, authorization.ToolNames(), "Host.List")
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
			MinScore:                   0.75,
			ReadonlyMinScore:           0.60,
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
func TestToolDecisionEngineRejectsLowScoreMismatch(t *testing.T) {
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
	// 高风险写工具在只读查询中应被拒绝
	assertNotContainsTool(t, plan.ToolNames(), "Detection.Alert.Block")
	assertRejectedDecision(t, plan, "Detection.Alert.Block")
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
