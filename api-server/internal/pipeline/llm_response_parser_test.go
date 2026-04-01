package pipeline

import (
	"testing"
)

func TestParseLLMResponse_ValidJSON(t *testing.T) {
	response := `{
        "alerts": [
            {
                "mitre_id": "T1059.004",
                "severity": "critical",
                "pid": 12345,
                "description": "反弹shell",
                "block_action": "kill_process",
                "block_target": "12345"
            }
        ],
        "tool_calls": [],
        "rule_adjustments": []
    }`

	output, err := ParseLLMResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(output.Alerts))
	}

	if output.Alerts[0].MitreID != "T1059.004" {
		t.Errorf("expected mitre_id T1059.004, got %s", output.Alerts[0].MitreID)
	}
}

func TestParseLLMResponse_MarkdownCodeBlock(t *testing.T) {
	response := "Here is the analysis:\n```json\n{\"alerts\": [], \"tool_calls\": [], \"rule_adjustments\": []}\n```"

	output, err := ParseLLMResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output == nil {
		t.Fatal("expected non-nil output")
	}
}

func TestParseLLMResponse_InvalidSeverity(t *testing.T) {
	response := `{
        "alerts": [{"mitre_id": "T1059.004", "severity": "invalid"}],
        "tool_calls": [],
        "rule_adjustments": []
    }`

	_, err := ParseLLMResponse(response)
	if err == nil {
		t.Error("expected error for invalid severity")
	}
}

func TestParseLLMResponse_ToolCallLimit(t *testing.T) {
	toolCalls := "["
	for i := 0; i < 15; i++ {
		if i > 0 {
			toolCalls += ","
		}
		toolCalls += `{"tool": "test", "params": {}, "reason": "test"}`
	}
	toolCalls += "]"

	response := `{"alerts": [], "tool_calls": ` + toolCalls + `, "rule_adjustments": []}`

	output, err := ParseLLMResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.ToolCalls) > 10 {
		t.Errorf("expected max 10 tool calls, got %d", len(output.ToolCalls))
	}
}
