package assistant

import "testing"

func TestParseExplicitToolSequence(t *testing.T) {
	specs := []*ToolSpec{
		{Name: "Vulnerability.List", Enabled: true},
		{Name: "Vulnerability.AffectedHosts", Enabled: true},
		{Name: "Vulnerability.Script.Status", Enabled: true},
		{Name: "Vulnerability.Script.Execute", Enabled: true},
		{Name: "Asset.Collection.Trigger", Enabled: true},
	}
	message := `请严格按顺序调用工具，不要只文字说明：
1 Vulnerability.List 参数 query="CVE-2023-50495",page=1,page_size=5。
2 Vulnerability.AffectedHosts 参数 vulnerability_id="87ab90ce-494c-41fd-8db5-0bebfdcabe4b"。
3 Vulnerability.Script.Status 参数 cve_id="CVE-2023-50495", script_type="poc"。
4 Vulnerability.Script.Execute 参数 cve_id="CVE-2023-50495", script_type="fix", host_ids=["cf18f7f7-5b45-46e2-9889-160dddc4ee30"]。
5 Asset.Collection.Trigger 参数 scope=hosts, host_ids=["cf18f7f7-5b45-46e2-9889-160dddc4ee30"], types=["process"], force=true。`

	steps := parseExplicitToolSequence(message, specs)
	if len(steps) != 5 {
		t.Fatalf("expected 5 steps, got %d: %#v", len(steps), steps)
	}
	if steps[0].ToolName != "Vulnerability.List" || steps[0].Args["query"] != "CVE-2023-50495" || steps[0].Args["page"] != 1 {
		t.Fatalf("unexpected first step: %#v", steps[0])
	}
	if steps[3].ToolName != "Vulnerability.Script.Execute" {
		t.Fatalf("unexpected execute step: %#v", steps[3])
	}
	hostIDs, ok := steps[3].Args["host_ids"].([]string)
	if !ok || len(hostIDs) != 1 || hostIDs[0] != "cf18f7f7-5b45-46e2-9889-160dddc4ee30" {
		t.Fatalf("unexpected host_ids: %#v", steps[3].Args["host_ids"])
	}
	if steps[4].Args["force"] != true {
		t.Fatalf("expected force=true, got %#v", steps[4].Args["force"])
	}
}
