package assistant

import "testing"

func TestToolSelectorDoesNotInjectResidentTools(t *testing.T) {
	registry := NewToolRegistry()
	for _, name := range []string{"Tool.Search", "Context.Get", "Session.Summarize"} {
		if err := registry.Register(&ToolSpec{Name: name, Enabled: true, Risk: ToolRiskReadonly, Handler: noopToolHandler}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	selector := NewToolSelector(NewToolCatalog(registry), registry)
	result := selector.Select(ToolSelectionInput{Query: "针对 CVE-2023-43641 生成修复脚本"})
	for _, name := range []string{"Tool.Search", "Context.Get", "Session.Summarize"} {
		if containsDecisionString(result.SelectedTools, name) {
			t.Fatalf("ordinary business request unexpectedly selected resident tool %s: %v", name, result.SelectedTools)
		}
	}
}
