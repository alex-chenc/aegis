package assistant

import (
	"context"
	"testing"
	"time"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

func TestDefaultAIAnalysisRuntimeConfigMatchesAnalysisFlow(t *testing.T) {
	cfg := DefaultAIAnalysisRuntimeConfig(0)

	if cfg.MaxTotalTurns != 500 {
		t.Fatalf("expected MaxTotalTurns 500, got %d", cfg.MaxTotalTurns)
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
	if cfg.MaxToolCalls != 100 || cfg.MaxToolCallsPerStep != 10 {
		t.Fatalf("expected AI analysis tool limits, got total=%d per_step=%d", cfg.MaxToolCalls, cfg.MaxToolCallsPerStep)
	}
}

func TestExpandComplexTaskToolsAddsSecurityAnalysisTools(t *testing.T) {
	o := &Orchestrator{}
	expanded := o.expandComplexTaskTools(
		[]string{"Investigation.HostAttack.Analyze", "Tool.Search"},
		"host_attack_investigation",
		"帮我排查主机安全事件",
		IntentResult{Domains: []string{"investigation"}, Action: "analyze"},
	)

	assertContainsTool(t, expanded, "Host.List")
	assertContainsTool(t, expanded, "Task.List")
	assertContainsTool(t, expanded, "Vulnerability.List")
	assertContainsTool(t, expanded, "Detection.Alert.List")
	assertContainsTool(t, expanded, "Agent.Process.List")
	assertContainsTool(t, expanded, "Agent.Network.List")
	assertContainsTool(t, expanded, "Tool.Search")
	assertUniqueTools(t, expanded)
}

func TestBuildAgentToolDescriptorsIncludesExpandedSecurityTools(t *testing.T) {
	registry := NewToolRegistry()
	for _, name := range []string{"Detection.Alert.List", "Agent.Process.List"} {
		toolName := name
		err := registry.Register(&ToolSpec{
			Name:        toolName,
			Description: toolName,
			Risk:        ToolRiskReadonly,
			Enabled:     true,
			Handler: func(context.Context, map[string]interface{}) (interface{}, error) {
				return map[string]interface{}{"ok": true}, nil
			},
		})
		if err != nil {
			t.Fatalf("register %s: %v", toolName, err)
		}
	}

	o := &Orchestrator{toolRegistry: registry}
	expanded := o.expandComplexTaskTools(nil, "", "帮我分析主机安全问题", IntentResult{
		Domains: []string{"detection"},
		Action:  "analyze",
	})
	descriptors := o.buildAgentToolDescriptors(expanded)

	var names []string
	for _, desc := range descriptors {
		names = append(names, desc.Name)
	}
	assertContainsTool(t, names, "Detection.Alert.List")
	assertContainsTool(t, names, "Agent.Process.List")
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

func assertContainsTool(t *testing.T, names []string, want string) {
	t.Helper()
	for _, name := range names {
		if name == want {
			return
		}
	}
	t.Fatalf("expected tools to contain %s, got %v", want, names)
}

func assertUniqueTools(t *testing.T, names []string) {
	t.Helper()
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			t.Fatalf("expected unique tools, found duplicate %s in %v", name, names)
		}
		seen[name] = true
	}
}
