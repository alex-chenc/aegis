package assistant

import (
	"context"
	"strings"
	"testing"
	"time"

	agentruntime "github.com/alex-chenc/agent-runtime"
	runtimeplan "github.com/alex-chenc/agent-runtime/plan"
)

func TestDefaultAIAnalysisRuntimeConfigMatchesAnalysisFlow(t *testing.T) {
	cfg := DefaultAIAnalysisRuntimeConfig(0)

	if cfg.MaxTotalTurns != 500 {
		t.Fatalf("expected MaxTotalTurns 500, got %d", cfg.MaxTotalTurns)
	}
	if cfg.MaxPlanSteps != 16 {
		t.Fatalf("expected MaxPlanSteps 16, got %d", cfg.MaxPlanSteps)
	}
	if cfg.TaskTimeout != 2*time.Hour {
		t.Fatalf("expected TaskTimeout 2h, got %v", cfg.TaskTimeout)
	}
	if !cfg.EnableContextCompress {
		t.Fatalf("expected context compression to be enabled")
	}
	if !cfg.EnableReflection || !cfg.EnableAudit || !cfg.EnableCorrection {
		t.Fatalf("expected reflection, audit, and correction to be enabled")
	}
	if cfg.MaxToolCalls != 160 || cfg.MaxToolCallsPerStep != 32 {
		t.Fatalf("expected AI analysis tool limits, got total=%d per_step=%d", cfg.MaxToolCalls, cfg.MaxToolCallsPerStep)
	}
	if cfg.AsyncPollInitialBackoff != 2*time.Second || cfg.AsyncPollMaxBackoff != 30*time.Second {
		t.Fatalf("expected bounded async polling backoff, got initial=%s max=%s", cfg.AsyncPollInitialBackoff, cfg.AsyncPollMaxBackoff)
	}
	if cfg.MaxAsyncPollAttempts != 12 {
		t.Fatalf("expected bounded async polling attempts, got %d", cfg.MaxAsyncPollAttempts)
	}
	if !cfg.AllowHighRiskTools || !cfg.AllowDangerousTools {
		t.Fatal("expected pre-authorized Aegis tools to reach the Aegis approval policy")
	}
}

func TestEffectiveRuntimeContextBudgetUsesObservedPromptTokens(t *testing.T) {
	result := &agentruntime.TaskResult{
		ContextBudget: &agentruntime.ContextBudgetSnapshot{
			MaxContextTokens:      256000,
			ReservedOutputTokens:  8192,
			EstimatedPromptTokens: 32,
			ContextRatio:          0.032125,
		},
		ModelCalls: []agentruntime.ModelCallRecord{
			{PromptTokens: 1200, CompletionTokens: 100},
			{PromptTokens: 24000, CompletionTokens: 300},
		},
		Metrics: agentruntime.RuntimeMetrics{
			TotalPromptTokens:     25200,
			TotalCompletionTokens: 400,
		},
	}

	budget := effectiveRuntimeContextBudget(result)
	if budget == nil {
		t.Fatal("expected budget")
	}
	if budget.EstimatedPromptTokens != 24000 {
		t.Fatalf("estimated prompt tokens = %d, want 24000", budget.EstimatedPromptTokens)
	}
	if budget.PromptTokensObserved != 24000 {
		t.Fatalf("observed prompt tokens = %d, want 24000", budget.PromptTokensObserved)
	}
	if budget.TotalTokens != 25600 {
		t.Fatalf("total tokens = %d, want 25600", budget.TotalTokens)
	}
	if budget.ContextRatio <= 0.09 {
		t.Fatalf("context ratio was not recomputed from observed prompt tokens: %f", budget.ContextRatio)
	}
}

func TestRuntimePlanFromToolExecutionPlanKeepsRepeatedToolArgs(t *testing.T) {
	plan := &ToolExecutionPlan{
		Goal:            "generate poc and fix",
		DecisionTraceID: "td-1",
		Steps: []ToolPlanStep{
			{StepID: "step_01", ToolName: "Vulnerability.Script.Generate", Reason: "generate poc", Risk: string(ToolRiskMedium), Args: map[string]interface{}{"cve_id": "CVE-2021-45340", "script_type": "poc"}},
			{StepID: "step_02", ToolName: "Vulnerability.Script.Generate", Reason: "generate fix", Risk: string(ToolRiskMedium), Args: map[string]interface{}{"cve_id": "CVE-2021-45340", "script_type": "fix"}},
			{StepID: "step_03", ToolName: "Vulnerability.Script.Status", Reason: "wait for poc", Risk: string(ToolRiskReadonly), Args: map[string]interface{}{"cve_id": "CVE-2021-45340", "script_type": "poc"}},
			{StepID: "step_04", ToolName: "Vulnerability.Script.Status", Reason: "wait for fix", Risk: string(ToolRiskReadonly), Args: map[string]interface{}{"cve_id": "CVE-2021-45340", "script_type": "fix"}},
		},
	}
	runtimePlan := runtimePlanFromToolExecutionPlan(plan)
	if runtimePlan == nil || len(runtimePlan.Steps) != 4 {
		t.Fatalf("runtime plan = %#v", runtimePlan)
	}
	if runtimePlan.Steps[0].ToolArgs["script_type"] != "poc" {
		t.Fatalf("first script_type = %#v", runtimePlan.Steps[0].ToolArgs["script_type"])
	}
	if runtimePlan.Steps[1].ToolArgs["script_type"] != "fix" {
		t.Fatalf("second script_type = %#v", runtimePlan.Steps[1].ToolArgs["script_type"])
	}
	if len(runtimePlan.Steps[1].Dependencies) != 1 || runtimePlan.Steps[1].Dependencies[0] != "step_01" {
		t.Fatalf("second step dependencies = %#v", runtimePlan.Steps[1].Dependencies)
	}
	for index, step := range runtimePlan.Steps {
		expectedTool := plan.Steps[index].ToolName
		if len(step.AllowedTools) != 1 || step.AllowedTools[0] != expectedTool {
			t.Fatalf("step tool scope = %#v", step.AllowedTools)
		}
	}
	validation := runtimeplan.NewValidator(10, []agentruntime.ToolDescriptor{
		{Name: "Vulnerability.Script.Generate"},
		{Name: "Vulnerability.Script.Status"},
	}, nil).Validate(runtimePlan)
	if !validation.Valid {
		t.Fatalf("runtime plan must satisfy agent-runtime validation: %#v", validation.Errors)
	}
}

func TestAssistantRuntimeAlwaysUsesMappedAuthorizationAsInitialPlan(t *testing.T) {
	authorization := &ToolExecutionPlan{
		Goal: "arbitrary user goal",
		Steps: []ToolPlanStep{{
			StepID:     "authorized_01",
			ToolName:   "Example.Apply",
			Capability: "apply_example",
		}},
	}
	plan := runtimeInitialPlanForAssistant(authorization)
	if plan == nil || len(plan.Steps) != 1 {
		t.Fatalf("mapped authorization must be the runtime plan, got %#v", plan)
	}
	if got := plan.Steps[0].AllowedTools; len(got) != 1 || got[0] != "Example.Apply" {
		t.Fatalf("runtime step is not bound to the mapped tool: %#v", got)
	}
}

func TestAssistantRuntimeUsesFixedPlanForBaselineCompliance(t *testing.T) {
	authorization := &ToolExecutionPlan{
		Goal: "run baseline compliance",
		Steps: []ToolPlanStep{
			{StepID: "authorized_01", Capability: "resolve_hosts", ToolName: "Host.Resolve", Args: map[string]interface{}{"target_scope": "all_online_hosts"}},
			{StepID: "authorized_02", Capability: "run_baseline_compliance", ToolName: "Baseline.Compliance.Run", Args: map[string]interface{}{"target_scope": "all_online_hosts", "template_selector": "cis-ubuntu"}},
			{StepID: "authorized_03", Capability: "get_operation_status", ToolName: "Operation.Get"},
		},
	}
	descriptors := []agentruntime.ToolDescriptor{
		{Name: "Host.Resolve"},
		{Name: "Baseline.Compliance.Run", CompletionTools: []string{"Operation.Get"}},
		{Name: "Operation.Get"},
	}
	plan := runtimeInitialPlanForAssistantWithDescriptors(authorization, descriptors)
	if plan == nil || len(plan.Steps) != 2 {
		t.Fatalf("baseline workflow must group its mapped completion tool, got %#v", plan)
	}
	if plan.Steps[0].ToolArgs["target_scope"] != "all_online_hosts" {
		t.Fatalf("fixed plan lost deterministic arguments: %#v", plan.Steps)
	}
	if len(plan.Steps[1].AllowedTools) != 2 ||
		plan.Steps[1].AllowedTools[0] != "Baseline.Compliance.Run" ||
		plan.Steps[1].AllowedTools[1] != "Operation.Get" {
		t.Fatalf("async step did not retain its mapped completion boundary: %#v", plan.Steps[1].AllowedTools)
	}
	if plan.Steps[1].ToolArgs != nil {
		t.Fatalf("grouped async step args must be bound per tool by the gateway: %#v", plan.Steps[1].ToolArgs)
	}
	if strings.Contains(plan.Steps[0].Title, "Host.Resolve") ||
		strings.Contains(plan.Steps[1].Title, "Baseline.Compliance.Run") {
		t.Fatalf("plan titles must describe business actions, not expose tool names: %#v", plan.Steps)
	}
	if !strings.Contains(plan.Steps[0].ExpectedOutput, "immediately complete") ||
		!strings.Contains(plan.Steps[1].Objective, "Operation.Get") {
		t.Fatalf("runtime steps lack deterministic completion instructions: %#v", plan.Steps)
	}
}

func TestRuntimeToolElectionInvariantRejectsDescriptorsWithoutMappedPlan(t *testing.T) {
	err := validateRuntimeToolElectionInvariant(RuntimeBuildRequest{
		ToolDescriptors: []agentruntime.ToolDescriptor{{Name: "Example.Apply"}},
	})
	if err == nil {
		t.Fatal("tool descriptors without a Mapping-bound execution plan must fail closed")
	}

	err = validateRuntimeToolElectionInvariant(RuntimeBuildRequest{
		ToolDescriptors: []agentruntime.ToolDescriptor{{Name: "Example.Apply"}},
		ExecutionPlan: &ToolExecutionPlan{
			DecisionTraceID: "td_test",
			Steps: []ToolPlanStep{{
				StepID:     "authorized_01",
				ToolName:   "Example.Apply",
				Capability: "apply_example",
			}},
			DecisionRecords: []ToolDecisionRecord{{
				ToolName:   "Example.Apply",
				Capability: "apply_example",
				Decision:   toolDecisionAccepted,
			}},
		},
	})
	if err != nil {
		t.Fatalf("mapped runtime request rejected: %v", err)
	}

	err = validateRuntimeToolElectionInvariant(RuntimeBuildRequest{
		ToolDescriptors: []agentruntime.ToolDescriptor{{Name: "Example.Discover"}},
		ExecutionPlan: &ToolExecutionPlan{
			DecisionTraceID: "td_test",
			Steps: []ToolPlanStep{{
				StepID:     "authorized_01",
				ToolName:   "Example.Apply",
				Capability: "apply_example",
			}},
			DecisionRecords: []ToolDecisionRecord{{
				ToolName:   "Example.Apply",
				Capability: "apply_example",
				Decision:   toolDecisionAccepted,
			}},
		},
	})
	if err == nil {
		t.Fatal("descriptor and Mapping plan must have the exact same tool set")
	}

	err = validateRuntimeToolElectionInvariant(RuntimeBuildRequest{
		ToolDescriptors: []agentruntime.ToolDescriptor{{Name: "Example.Apply"}},
		ExecutionPlan: &ToolExecutionPlan{
			DecisionTraceID: "td_test",
			Steps: []ToolPlanStep{
				{StepID: "authorized_01", ToolName: "Example.Apply", Capability: "apply_example"},
				{StepID: "authorized_01", ToolName: "Example.Apply", Capability: "apply_example"},
			},
			DecisionRecords: []ToolDecisionRecord{{
				ToolName:   "Example.Apply",
				Capability: "apply_example",
				Decision:   toolDecisionAccepted,
			}},
		},
	})
	if err == nil {
		t.Fatal("duplicate Mapping step IDs must fail closed")
	}
}

func TestValidateRuntimeFinalAnswerRejectsInternalJSONObject(t *testing.T) {
	if err := validateRuntimeFinalAnswer(`{"enabled":true,"max_rounds":3}`); err == nil {
		t.Fatal("internal JSON object must not be accepted as the final user answer")
	}
	if err := validateRuntimeFinalAnswer("任务未创建，未执行任何修复。"); err != nil {
		t.Fatalf("natural-language final answer rejected: %v", err)
	}
}

func TestNormalizeBlockedMappingPlanStepsMarksTransitiveDependenciesSkipped(t *testing.T) {
	result := &agentruntime.TaskResult{
		FinalPlan: &agentruntime.Plan{Steps: []agentruntime.PlanStep{
			{StepID: "step_1", Status: agentruntime.StepFailed},
			{StepID: "step_2", Status: agentruntime.StepPending, Dependencies: []string{"step_1"}},
			{StepID: "step_3", Status: agentruntime.StepPending, Dependencies: []string{"step_2"}},
			{StepID: "step_independent", Status: agentruntime.StepPending},
		}},
	}

	skipped := normalizeBlockedMappingPlanSteps(result)
	if len(skipped) != 2 {
		t.Fatalf("expected two blocked steps, got %#v", skipped)
	}
	if result.FinalPlan.Steps[1].Status != agentruntime.StepSkipped ||
		result.FinalPlan.Steps[2].Status != agentruntime.StepSkipped {
		t.Fatalf("transitive blocked steps were not normalized: %#v", result.FinalPlan.Steps)
	}
	if result.FinalPlan.Steps[3].Status != agentruntime.StepPending {
		t.Fatalf("independent pending step must not be changed: %#v", result.FinalPlan.Steps[3])
	}
}

func TestApplyEffectiveApprovalModeUpdatesAuthorizationArtifact(t *testing.T) {
	plan := &ToolExecutionPlan{
		Steps: []ToolPlanStep{{ToolName: "Baseline.Compliance.Run", RequiresApproval: true}},
		DecisionRecords: []ToolDecisionRecord{{
			ToolName:      "Baseline.Compliance.Run",
			Decision:      toolDecisionAccepted,
			ApprovalState: "required",
		}},
	}

	applyEffectiveApprovalMode(plan, "full_access")

	if plan.Steps[0].RequiresApproval {
		t.Fatal("full-access authorization step must not require approval")
	}
	if plan.DecisionRecords[0].ApprovalState != "not_required" {
		t.Fatalf("approval state = %q, want not_required", plan.DecisionRecords[0].ApprovalState)
	}
	if plan.DecisionRecords[0].Evidence["approval_mode"] != "full_access" {
		t.Fatalf("approval mode evidence missing: %#v", plan.DecisionRecords[0].Evidence)
	}
}

func TestBuildAgentToolDescriptorsUsesRuntimeCompatibleNumberSchema(t *testing.T) {
	registry := NewToolRegistry()
	if err := registry.Register(&ToolSpec{
		Name:      "Example.List",
		Domain:    DomainSystem,
		Operation: OpList,
		Risk:      ToolRiskReadonly,
		Enabled:   true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page": map[string]interface{}{"type": "integer"},
			},
		},
		Handler: func(context.Context, map[string]interface{}) (interface{}, error) {
			return nil, nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	orchestrator := &Orchestrator{toolRegistry: registry}
	descriptors := orchestrator.buildAgentToolDescriptors([]string{"Example.List"})
	if len(descriptors) != 1 {
		t.Fatalf("descriptors = %d, want 1", len(descriptors))
	}
	properties := descriptors[0].ArgsSchema["properties"].(map[string]interface{})
	if got := properties["page"].(map[string]interface{})["type"]; got != "number" {
		t.Fatalf("runtime page schema type = %v, want number", got)
	}
}

func TestBuildAgentToolDescriptorsResolvesAuthorizedCompletionTool(t *testing.T) {
	registry := NewToolRegistry()
	handler := func(context.Context, map[string]interface{}) (interface{}, error) {
		return nil, nil
	}
	if err := registry.Register(&ToolSpec{
		Name:       "Example.Generate",
		Domain:     DomainSystem,
		Operation:  OpGenerate,
		Capability: "generate_example",
		Risk:       ToolRiskMedium,
		Enabled:    true,
		ExecutionContract: ToolExecutionContract{
			Mode:                 ToolExecutionAsynchronous,
			CompletionCapability: "get_example_status",
		},
		ArgsSchema: map[string]interface{}{"type": "object"},
		Handler:    handler,
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(&ToolSpec{
		Name:       "Example.Status",
		Domain:     DomainSystem,
		Operation:  OpGet,
		Capability: "get_example_status",
		Risk:       ToolRiskReadonly,
		Enabled:    true,
		Idempotent: true,
		ArgsSchema: map[string]interface{}{"type": "object"},
		Handler:    handler,
	}); err != nil {
		t.Fatal(err)
	}

	orchestrator := &Orchestrator{toolRegistry: registry}
	descriptors := orchestrator.buildAgentToolDescriptors([]string{"Example.Generate", "Example.Status"})
	if len(descriptors) != 2 {
		t.Fatalf("descriptors = %d, want 2", len(descriptors))
	}
	if got := descriptors[0].CompletionTools; len(got) != 1 || got[0] != "Example.Status" {
		t.Fatalf("completion tools = %#v, want Example.Status", got)
	}

	descriptors = orchestrator.buildAgentToolDescriptors([]string{"Example.Generate"})
	if len(descriptors[0].CompletionTools) != 0 {
		t.Fatalf("unauthorized completion tool leaked into descriptor: %#v", descriptors[0].CompletionTools)
	}
}
