package assistant

import "testing"

func TestEstimateQueryComplexityHandlesChineseCompoundTasks(t *testing.T) {
	got := estimateQueryComplexity("分析最近的安全事件并给出修复建议")
	if got < 3 {
		t.Fatalf("expected Chinese analysis request to be complex, got score %d", got)
	}
}

func TestShouldUseLLMForIntentKeepsSimpleHighConfidenceQuery(t *testing.T) {
	result := IntentResult{
		Action:     "query",
		Domains:    []string{"host"},
		Confidence: 0.8,
	}

	if shouldUseLLMForIntent("主机列表", result) {
		t.Fatal("expected simple high-confidence query to stay on rules")
	}
}

func TestShouldUseLLMForIntentEscalatesLowConfidenceChineseQuery(t *testing.T) {
	result := IntentResult{
		Action:     "query",
		Domains:    []string{"host"},
		Confidence: 0.3,
	}

	if !shouldUseLLMForIntent("帮我判断这些风险为什么集中出现", result) {
		t.Fatal("expected low-confidence Chinese query to use LLM classification")
	}
}

func TestClassifyByRulesKeepsGreetingAsDirectAnswer(t *testing.T) {
	router := NewIntentRouter()
	result := router.classifyByRules(IntentInput{Query: "你好"})

	if result.Action != "answer" {
		t.Fatalf("expected greeting to be direct answer, got action %q", result.Action)
	}
	if len(result.Domains) != 0 {
		t.Fatalf("expected greeting to avoid business domains, got %v", result.Domains)
	}
}
