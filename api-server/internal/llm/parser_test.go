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

func TestParseRulesFromFencedJSONBlock(t *testing.T) {
	// Models sometimes wrap the JSON in a ```json code block, optionally with
	// leading/trailing prose. This is the exact scenario that previously yielded
	// "invalid LLM response format".
	resp := "好的，以下是从文档中提取的基线规则：\n" + "```json" + `
[
  {
    "title": "SSH 密码复杂度要求",
    "check_content": "检查 /etc/pam.d/common-password",
    "fix_content": "添加 pam_pwquality.so"
  }
]
` + "```" + "\n如有疑问请继续提问。"
	rules, err := ParseRules(resp)
	if err != nil {
		t.Fatalf("ParseRules returned error for fenced JSON block: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected one rule, got %d", len(rules))
	}
	if rules[0].Title != "SSH 密码复杂度要求" {
		t.Fatalf("unexpected title: %q", rules[0].Title)
	}
}

func TestParseRulesFencedBlockOnly(t *testing.T) {
	// Response that is ONLY a fenced block with no surrounding prose.
	resp := "```json\n[{\"title\":\"X\",\"check_content\":\"c\",\"fix_content\":\"f\"}]\n```"
	rules, err := ParseRules(resp)
	if err != nil {
		t.Fatalf("ParseRules returned error for fenced-only block: %v", err)
	}
	if len(rules) != 1 || rules[0].Title != "X" {
		t.Fatalf("expected one rule titled X, got %#v", rules)
	}
}

func TestParseRulesStillFailsOnNoJSON(t *testing.T) {
	// Pure prose / refusal with no JSON at all must still surface the error so
	// the caller can retry or mark the template failed.
	resp := "抱歉，我无法从这份文档中提取出结构化规则，请提供更有条理的内容。"
	_, err := ParseRules(resp)
	if err == nil {
		t.Fatal("expected error for non-JSON response")
	}
	if err.Error() != "invalid LLM response format" {
		t.Fatalf("expected 'invalid LLM response format', got %q", err.Error())
	}
}

func TestExtractJSONFencedFallback(t *testing.T) {
	// No top-level bracket, but a fenced block exists -> fallback should find it.
	resp := "说明文字\n```json\n{\"a\":1}\n```"
	got := extractJSON(resp)
	if got == "" {
		t.Fatal("expected extractJSON to fall back to fenced block")
	}
	if !strings.Contains(got, `"a":1`) {
		t.Fatalf("unexpected extracted JSON: %q", got)
	}
}

func TestParseRulesRepairsInvalidBackslashEscapes(t *testing.T) {
	rules, err := ParseRules(`[
  {
    "title": "AIDE配置保护审计工具完整性",
    "check_content": "使用命令 egrep '(\sbin/(audit|au))' /etc/aide/aide.conf 检查审计工具条目",
    "fix_content": "编辑 /etc/aide/aide.conf 添加 /sbin/auditctl p+i+n+u+g+s+b+acl+xattrs+sha512"
  }
]`)
	if err != nil {
		t.Fatalf("ParseRules returned error: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected one rule, got %d", len(rules))
	}
	if !strings.Contains(rules[0].CheckContent, `\sbin`) {
		t.Fatalf("expected literal backslash to be preserved, got %q", rules[0].CheckContent)
	}
}

func TestParseRulesExtractJSONIgnoresBracketsInsideStrings(t *testing.T) {
	rules, err := ParseRules(`LLM说明：
[
  {
    "title": "审计日志格式",
    "check_content": "确认日志格式包含字段 [user] 和 [action]，不要截断 JSON",
    "fix_content": "调整日志模板"
  }
]
后续说明`)
	if err != nil {
		t.Fatalf("ParseRules returned error: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected one rule, got %d", len(rules))
	}
	if rules[0].Title != "审计日志格式" {
		t.Fatalf("unexpected title: %q", rules[0].Title)
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

func TestParseFinalAnswerKeepsIncompleteJSONAfterMarker(t *testing.T) {
	agent := NewReActAgent(nil, nil, "session-test", 1)

	_, finalAnswer := agent.parseFinalAnswer("Final Answer:\n{\n  \"attack_graph\": {")

	if !strings.Contains(finalAnswer, "attack_graph") {
		t.Fatalf("expected incomplete final answer content to be preserved, got %q", finalAnswer)
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

func TestFinalAnswerReadyWaitsForBalancedJSON(t *testing.T) {
	if finalAnswerReady("Final Answer:\n{\n  \"attack_graph\": {", false) {
		t.Fatal("expected incomplete final answer JSON to remain pending")
	}
	if !finalAnswerReady("Final Answer:\n{\"attack_graph\":{\"nodes\":[]}}", false) {
		t.Fatal("expected balanced final answer JSON to be ready")
	}
}

func TestToolIterationLimitForcesFinalAnswerAfterFiftyWhenConfiguredHigher(t *testing.T) {
	limit, forceFinalAnswer := toolIterationLimit(100)

	if limit != 50 {
		t.Fatalf("expected tool iteration limit 50, got %d", limit)
	}
	if !forceFinalAnswer {
		t.Fatal("expected final answer to be forced when max iterations exceeds 50")
	}
}

func TestToolIterationLimitDoesNotForceFinalAnswerAtFiftyOrBelow(t *testing.T) {
	limit, forceFinalAnswer := toolIterationLimit(50)

	if limit != 50 {
		t.Fatalf("expected tool iteration limit 50, got %d", limit)
	}
	if forceFinalAnswer {
		t.Fatal("did not expect forced final answer when max iterations is 50")
	}
}

func TestNormalizeToolNameRejectsPromptPlaceholder(t *testing.T) {
	if got := normalizeToolName("[the action to take, should be one of the available tools]"); got != "" {
		t.Fatalf("expected placeholder tool name to be rejected, got %q", got)
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
