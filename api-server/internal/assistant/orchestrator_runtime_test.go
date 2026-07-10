package assistant

import (
	"context"
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

func TestAssistantRuntimeDoesNotUseAuthorizationAsInitialPlan(t *testing.T) {
	authorization := &ToolExecutionPlan{
		Goal: "arbitrary user goal",
		Steps: []ToolPlanStep{{
			StepID:   "authorized_01",
			ToolName: "Example.Apply",
		}},
	}
	if plan := runtimeInitialPlanForAssistant(authorization); plan != nil {
		t.Fatalf("pure agent mode must let agent-runtime create the only plan, got %#v", plan)
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
