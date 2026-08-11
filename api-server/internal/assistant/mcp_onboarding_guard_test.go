package assistant

import "testing"

func TestIsMCPOnboardingRequest(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "Chinese connect", query: "把这个接入到远程 MCP", want: true},
		{name: "English register", query: "register this remote MCP server", want: true},
		{name: "MCP explanation", query: "解释当前系统内的 MCP 聚合方案", want: false},
		{name: "ordinary connect", query: "连接当前系统", want: false},
		{name: "MCP query", query: "查询 MCP 工具列表", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMCPOnboardingRequest(tt.query); got != tt.want {
				t.Fatalf("isMCPOnboardingRequest(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestShouldGuideMCPOnboardingForClarificationContinuation(t *testing.T) {
	input := RunInput{
		UserMessage: "当前系统内的 MCP 聚合方案",
		PendingClarification: &PendingClarification{
			OriginalQuery: "把这个接入到远程 MCP",
			Question:      "请明确要接入的具体对象是什么？",
		},
	}
	if !shouldGuideMCPOnboarding(input) {
		t.Fatal("expected MCP clarification continuation to be handled by control-plane guidance")
	}

	input.UserMessage = "查询主机健康状态"
	if shouldGuideMCPOnboarding(input) {
		t.Fatal("unrelated replacement request must not be intercepted")
	}
}

func TestMCPOnboardingGuidanceLocale(t *testing.T) {
	if got := mcpOnboardingGuidance(LocaleZhCN); got == "" || got == mcpOnboardingGuidance(LocaleEnUS) {
		t.Fatal("expected Chinese MCP onboarding guidance")
	}
	if got := mcpOnboardingGuidance(LocaleEnUS); got == "" || got == mcpOnboardingGuidance(LocaleZhCN) {
		t.Fatal("expected English MCP onboarding guidance")
	}
}
