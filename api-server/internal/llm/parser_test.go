package llm

import (
	"strings"
	"testing"

	"api-server/pkg/logger"
)

func init() {
	_ = logger.Init(&logger.Config{Level: "info"})
}

func TestParseScript_AddsShebang(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantShebang bool
		wantErr     bool
	}{
		{
			name:        "script without shebang gets one added",
			input:       "echo 'hello world'",
			wantShebang: true,
			wantErr:     false,
		},
		{
			name:        "script with shebang preserved",
			input:       "#!/bin/bash\necho 'hello'",
			wantShebang: true,
			wantErr:     false,
		},
		{
			name:        "script with different shebang preserved",
			input:       "#!/bin/sh\necho 'hello'",
			wantShebang: true,
			wantErr:     false,
		},
		{
			name:        "empty script returns error",
			input:       "",
			wantShebang: false,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseScript(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error for empty script")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.HasPrefix(result, "#!") {
				t.Errorf("script should start with shebang, got: %s", result[:min(20, len(result))])
			}
		})
	}
}

func TestTryParseStepInfersHistoricalLogToolFromFencedJSON(t *testing.T) {
	agent := NewReActAgent(nil, nil, "session-test", 1)

	step, done := agent.tryParseStep(`我将先查询历史日志。

## 第一步：查询历史日志

` + "```json" + `
{
  "host_id": "76de257c-f52e-4990-9554-381498dec603",
  "start_time": "2024-01-15T10:00:00Z",
  "end_time": "2024-01-15T12:00:00Z",
  "filter": "alert OR suspicious"
}
` + "```" + `

等待观察结果。`)

	if !done {
		t.Fatal("expected fenced JSON to be parsed as a complete tool step")
	}
	if step.Action != "QueryHistoricalLogs" {
		t.Fatalf("expected QueryHistoricalLogs, got %q", step.Action)
	}
	if step.ActionInput["host_id"] != "76de257c-f52e-4990-9554-381498dec603" {
		t.Fatalf("unexpected action input: %#v", step.ActionInput)
	}
}

func TestParseFinalAnswerRequiresFinalAnswerMarker(t *testing.T) {
	agent := NewReActAgent(nil, nil, "session-test", 1)

	_, finalAnswer := agent.parseFinalAnswer("我先查询历史日志，然后继续分析。")

	if finalAnswer != "" {
		t.Fatalf("expected no final answer without marker, got %q", finalAnswer)
	}
}

func TestTryParseStepDoesNotPanicWhenClosingBracePrecedesActionInput(t *testing.T) {
	agent := NewReActAgent(nil, nil, "session-test", 1)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("tryParseStep panicked: %v", r)
		}
	}()

	_, done := agent.tryParseStep(`Thought: 我需要继续分析前一次 Observation 中的日志。
日志片段里存在孤立右括号 }
Action: QueryHistoricalLogs
Action Input:
{
`)

	if done {
		t.Fatal("expected incomplete action input to remain pending")
	}
}

func TestFormatObservationTruncatesLargeToolResultForPrompt(t *testing.T) {
	agent := NewReActAgent(nil, nil, "session-test", 1)

	result := map[string]interface{}{
		"logs": strings.Repeat("x", maxObservationChars+1024),
	}

	observation := agent.formatObservation(result, nil)

	if len(observation) <= maxObservationChars {
		t.Fatalf("expected truncation marker to add context, got length %d", len(observation))
	}
	if len(observation) > maxObservationChars+256 {
		t.Fatalf("observation was not bounded enough, got length %d", len(observation))
	}
	if !strings.Contains(observation, "truncated") {
		t.Fatalf("expected truncation marker, got %q", observation[len(observation)-100:])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
