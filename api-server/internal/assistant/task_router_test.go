package assistant

import (
	"testing"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

func TestShouldForceSimpleToolRouteForExplicitToolDirective(t *testing.T) {
	tools := []agentruntime.ToolDescriptor{
		{Name: "Asset.Collection.Trigger"},
		{Name: "Asset.Application.List"},
	}

	message := `请严格按顺序调用工具：1 Asset.Collection.Trigger 参数 scope=hosts；2 Asset.Application.List 参数 category=ai_agent。不要只文字说明。`

	if !shouldForceSimpleToolRoute(message, tools) {
		t.Fatalf("expected explicit tool directive to use simple tool route")
	}
}

func TestShouldForceSimpleToolRouteIgnoresPlainAnalysis(t *testing.T) {
	tools := []agentruntime.ToolDescriptor{
		{Name: "Detection.Alert.List"},
	}

	if shouldForceSimpleToolRoute("分析这台主机是否存在安全问题", tools) {
		t.Fatalf("plain analysis should keep normal router behavior")
	}
}
