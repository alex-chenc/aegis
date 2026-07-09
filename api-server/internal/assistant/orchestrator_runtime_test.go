package assistant

import (
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
