package assistant

import (
	"context"
	"strings"
	"testing"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

func TestAssistantPromptProviderIncludesHostSecurityAnalysisGuide(t *testing.T) {
	provider := NewAssistantPromptProvider([]agentruntime.ToolDescriptor{
		{
			Name:        "Host.List",
			Description: "列出主机",
			ArgsSchema: map[string]interface{}{
				"properties": map[string]interface{}{
					"page_size": map[string]interface{}{"type": "integer"},
					"status":    map[string]interface{}{"type": "string"},
				},
			},
		},
		{Name: "Agent.Process.List", Description: "查询进程"},
		{Name: "Detection.Alert.List", Description: "查询告警"},
	}, nil, "analysis", "分析全部在线主机是否存在安全问题")

	cases := []struct {
		name    string
		purpose agentruntime.LLMPurpose
	}{
		{name: "plan", purpose: agentruntime.PurposePlan},
		{name: "react", purpose: agentruntime.PurposeReact},
		{name: "summarize", purpose: agentruntime.PurposeSummarize},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: tc.purpose})
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			prompt := bundle.SystemPrompt
			for _, want := range []string{
				"主机安全整体分析准则",
				"每台目标主机给出清晰风险等级",
				"Host.List 返回 N 台目标主机后必须覆盖全部 N 台",
				"不要因为“没有告警”就直接判定安全",
			} {
				if !strings.Contains(prompt, want) {
					t.Fatalf("prompt for %s missing %q\n%s", tc.name, want, prompt)
				}
			}
		})
	}
}

func TestAssistantSummarizePromptRequiresClearHostSecurityResult(t *testing.T) {
	provider := NewAssistantPromptProvider(nil, nil, "analysis", "分析主机安全")
	bundle, err := provider.Build(context.Background(), agentruntime.PromptRequest{Purpose: agentruntime.PurposeSummarize})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	for _, want := range []string{
		"安全结论：是否存在安全问题、最高风险等级、最需要立即处理的问题",
		"每台主机分析：主机名/IP、在线状态、风险等级",
		"不得把“没有告警”写成“没有风险”",
		"先给最清晰结论，再给证据",
	} {
		if !strings.Contains(bundle.SystemPrompt, want) {
			t.Fatalf("summarize prompt missing %q\n%s", want, bundle.SystemPrompt)
		}
	}
}
