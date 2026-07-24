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

func TestOperationGetResultIsNeverReused(t *testing.T) {
	tool := &ToolSpec{
		Name:       "Operation.Get",
		Operation:  OpGet,
		Risk:       ToolRiskReadonly,
		Idempotent: true,
		ResultContract: ToolResultContract{
			OperationStatusField: "operation_status",
			PendingValues:        []string{"running"},
		},
	}

	if canReuseAssistantToolResult(tool) {
		t.Fatal("durable operation status changes over time and must be queried again")
	}
}
