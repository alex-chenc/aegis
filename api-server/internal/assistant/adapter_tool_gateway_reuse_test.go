package assistant

import "testing"

func TestCustomCVEStatusResultIsNeverReused(t *testing.T) {
	tool := &ToolSpec{
		Name:       "Vulnerability.CustomQuery.Status",
		Operation:  OpGet,
		Risk:       ToolRiskReadonly,
		Idempotent: true,
	}

	if canReuseAssistantToolResult(tool) {
		t.Fatal("custom CVE status changes over time and must be queried again")
	}
}
