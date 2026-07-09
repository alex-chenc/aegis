package assistant

import (
	"context"
	"testing"
)

func TestShortToolBriefLimitsDescription(t *testing.T) {
	brief := shortToolBrief(&ToolSpec{
		Description: "触发资产采集任务，复用运维模式的资产采集服务采集进程、应用和 AI 资产，并返回采集进度引用",
	})
	if brief == "" {
		t.Fatal("expected non-empty brief")
	}
	if len([]rune(brief)) > 30 {
		t.Fatalf("brief too long: %q", brief)
	}
}

func TestShortToolBriefExpandsVeryShortDescription(t *testing.T) {
	brief := shortToolBrief(&ToolSpec{
		Description: "列出主机",
		Capability:  "list_hosts",
		Aliases:     []string{"主机列表", "资产列表"},
		Tags:        []string{"host"},
	})
	if len([]rune(brief)) < 20 {
		t.Fatalf("brief too short: %q", brief)
	}
	if len([]rune(brief)) > 30 {
		t.Fatalf("brief too long: %q", brief)
	}
}

func TestUnmarshalFirstJSONObjectParsesFencedResponse(t *testing.T) {
	var result llmToolSelectionDraft
	raw := "```json\n{\"selected_tools\":[\"Host.List\"],\"detail_requests\":[\"资产\"],\"reason\":\"ok\"}\n```"
	if err := unmarshalFirstJSONObject(raw, &result); err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.SelectedTools) != 1 || result.SelectedTools[0] != "Host.List" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestNormalizeLLMSelectedToolsFiltersUnknownCriticalWithoutResidentInjection(t *testing.T) {
	registry := NewToolRegistry()
	for _, spec := range []*ToolSpec{
		{Name: "Host.List", Risk: ToolRiskReadonly, Enabled: true, Handler: noopToolHandler},
		{Name: "Danger.Delete", Risk: ToolRiskCritical, Enabled: true, Handler: noopToolHandler},
		{Name: "Tool.Search", Risk: ToolRiskReadonly, Enabled: true, Handler: noopToolHandler},
		{Name: "Context.Get", Risk: ToolRiskReadonly, Enabled: true, Handler: noopToolHandler},
		{Name: "Session.Summarize", Risk: ToolRiskReadonly, Enabled: true, Handler: noopToolHandler},
	} {
		if err := registry.Register(spec); err != nil {
			t.Fatalf("register %s: %v", spec.Name, err)
		}
	}
	o := &Orchestrator{toolRegistry: registry}

	selected := o.normalizeLLMSelectedTools([]string{"Host.List", "Missing.Tool", "Danger.Delete"})
	assertContainsTool(t, selected, "Host.List")
	assertNotContainsTool(t, selected, "Tool.Search")
	assertNotContainsTool(t, selected, "Context.Get")
	assertNotContainsTool(t, selected, "Session.Summarize")
	for _, name := range selected {
		if name == "Danger.Delete" || name == "Missing.Tool" {
			t.Fatalf("unexpected selected tools: %v", selected)
		}
	}
}

func TestShouldUseLLMToolSelectionForAllOperationalRequests(t *testing.T) {
	if !shouldUseLLMToolSelection("进行资产采集", IntentResult{Action: "execute"}) {
		t.Fatal("expected asset collection to use generic llm tool selection")
	}
	if !shouldUseLLMToolSelection("进行基线扫描", IntentResult{Action: "execute"}) {
		t.Fatal("expected baseline scan to use generic llm tool selection")
	}
	if !shouldUseLLMToolSelection("进行资产采集任务，并分析那个主机上有 MySQL 软件，并分析此 MySql 软件是否有漏洞", IntentResult{Action: "analyze"}) {
		t.Fatal("expected composite asset and vulnerability analysis to use llm tool selection")
	}
}

func noopToolHandler(context.Context, map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"ok": true}, nil
}
