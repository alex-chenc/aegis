package assistant

import (
	"context"
	"strings"
	"testing"
)

func TestIntentRouterRequiresLLMInsteadOfRuleFallback(t *testing.T) {
	router := NewIntentRouter()
	_, err := router.Classify(context.Background(), IntentInput{Query: "主机列表"})
	if err == nil || !strings.Contains(err.Error(), "client factory is nil") {
		t.Fatalf("expected missing LLM factory error, got %v", err)
	}
}

func TestIntentRouterContractAcceptsOpenActionAndDomain(t *testing.T) {
	err := validateLLMIntentResult(IntentResult{
		Domains:    []string{"custom_domain"},
		Action:     "compare_and_reconcile",
		RiskHint:   ToolRiskReadonly,
		Confidence: 0.9,
	})
	if err != nil {
		t.Fatalf("open LLM intent values should be accepted: %v", err)
	}
}
