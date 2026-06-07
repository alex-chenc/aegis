package adapters

import (
	"testing"

	agentruntime "github.com/alex-chenc/agent-runtime"

	"api-server/internal/llm"
)

func TestInjectAlertContext_WithEmptyMessages(t *testing.T) {
	alertCtx := map[string]interface{}{
		"rule_name": "test_rule",
		"host_id":   "host-1",
	}
	adapter := &LLMClientAdapter{
		client:   nil,
		alertCtx: alertCtx,
	}

	messages := []llm.Message{}
	result := adapter.injectAlertContext(messages)

	if len(result) == 0 {
		t.Fatal("injectAlertContext should return at least one message")
	}

	if result[0].Role != "system" {
		t.Errorf("expected first message role to be 'system', got %q", result[0].Role)
	}
}

func TestInjectAlertContext_WithExistingSystemMessage(t *testing.T) {
	alertCtx := map[string]interface{}{
		"rule_name": "test_rule",
	}
	adapter := &LLMClientAdapter{
		client:   nil,
		alertCtx: alertCtx,
	}

	messages := []llm.Message{
		{Role: "system", Content: "existing system prompt"},
		{Role: "user", Content: "user input"},
	}
	result := adapter.injectAlertContext(messages)

	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}

	if result[0].Content == "existing system prompt" {
		t.Error("system message should have alert context appended")
	}
}

func TestInjectAlertContext_NilAlertCtx(t *testing.T) {
	adapter := &LLMClientAdapter{
		client:   nil,
		alertCtx: nil,
	}

	messages := []llm.Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "user input"},
	}
	result := adapter.injectAlertContext(messages)

	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}

	if result[0].Content != "system prompt" {
		t.Error("system message should not be modified when alertCtx is nil")
	}
}

func TestTemperatureForPurpose(t *testing.T) {
	adapter := &LLMClientAdapter{}

	tests := []struct {
		purpose agentruntime.LLMPurpose
		wantMin float64
		wantMax float64
	}{
		{agentruntime.PurposePlan, 0.3, 0.5},
		{agentruntime.PurposeReact, 0.6, 0.8},
		{agentruntime.PurposeAudit, 0.2, 0.4},
		{agentruntime.PurposeSummarize, 0.2, 0.4},
	}

	for _, tt := range tests {
		temp := adapter.temperatureForPurpose(tt.purpose)
		if temp < tt.wantMin || temp > tt.wantMax {
			t.Errorf("temperatureForPurpose(%s) = %f, want [%f, %f]", tt.purpose, temp, tt.wantMin, tt.wantMax)
		}
	}
}

func TestIsContextualPurpose(t *testing.T) {
	tests := []struct {
		purpose agentruntime.LLMPurpose
		want    bool
	}{
		{agentruntime.PurposePlan, true},
		{agentruntime.PurposeReact, true},
		{agentruntime.PurposeSummarize, true},
		{agentruntime.PurposeAudit, false},
		{agentruntime.PurposeReflect, false},
		{agentruntime.PurposeCorrect, false},
	}

	for _, tt := range tests {
		got := isContextualPurpose(tt.purpose)
		if got != tt.want {
			t.Errorf("isContextualPurpose(%s) = %v, want %v", tt.purpose, got, tt.want)
		}
	}
}

func TestContainsJSONKeyword_WithJSON(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "请按照JSON格式输出结果"},
	}
	if !containsJSONKeyword(messages) {
		t.Error("expected JSON keyword to be found")
	}
}

func TestContainsJSONKeyword_WithoutJSON(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "analyze this event"},
	}
	if containsJSONKeyword(messages) {
		t.Error("expected no JSON keyword")
	}
}

func TestContainsJSONKeyword_CaseInsensitive(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "return as Json format"},
	}
	if !containsJSONKeyword(messages) {
		t.Error("expected case-insensitive JSON match")
	}
}

func TestContainsJSONKeyword_InSystemMessage(t *testing.T) {
	messages := []llm.Message{
		{Role: "system", Content: "You are a helper. Output valid json."},
		{Role: "user", Content: "analyze"},
	}
	if !containsJSONKeyword(messages) {
		t.Error("expected JSON keyword in system message")
	}
}

func TestContainsJSONKeyword_EmptyMessages(t *testing.T) {
	if containsJSONKeyword(nil) {
		t.Error("expected false for nil messages")
	}
	if containsJSONKeyword([]llm.Message{}) {
		t.Error("expected false for empty messages")
	}
}

// === Phase 2: Usage passthrough tests ===

func TestTemperatureForPurpose_Compress(t *testing.T) {
	adapter := &LLMClientAdapter{}
	temp := adapter.temperatureForPurpose(agentruntime.PurposeCompress)
	if temp < 0.2 || temp > 0.4 {
		t.Errorf("temperatureForPurpose(compress) = %f, want [0.2, 0.4]", temp)
	}
}

func TestIsContextualPurpose_Compress(t *testing.T) {
	// PurposeCompress should NOT be contextual (no alert injection needed for compression calls)
	if isContextualPurpose(agentruntime.PurposeCompress) {
		t.Error("expected PurposeCompress to not be contextual")
	}
}

// === Phase 3: Tool call extraction from natural language tests ===

func TestExtractToolCallFromText_WithHostList(t *testing.T) {
	input := "我将帮您查询当前的主机情况。首先，我会调用 Host.List 工具来获取所有主机的列表和概览信息"
	result := extractToolCallFromText(input)
	if result == "" {
		t.Fatal("expected non-empty result for Host.List")
	}
	t.Logf("Result: %s", result)
	if !containsSubstring(result, `"tool_name":"Host.List"`) {
		t.Error("expected tool_name to be Host.List")
	}
}

func TestExtractToolCallFromText_WithHostFindOffline(t *testing.T) {
	input := "需要调用 Host.FindOffline 来查找离线主机"
	result := extractToolCallFromText(input)
	if result == "" {
		t.Fatal("expected non-empty result for Host.FindOffline alias")
	}
	t.Logf("Result: %s", result)
	if !containsSubstring(result, `"tool_name":"Host.AgentStatus.Get"`) {
		t.Error("expected Host.FindOffline to map to Host.AgentStatus.Get")
	}
}

func TestExtractToolCallFromText_NoToolName(t *testing.T) {
	input := "你好，我是智能助手"
	result := extractToolCallFromText(input)
	if result != "" {
		t.Errorf("expected empty result, got: %s", result)
	}
}

func TestExtractToolCallFromText_AlreadyJSON(t *testing.T) {
	input := `{"action":"tool_call","tool_call":{"tool_name":"Host.List","args":{}}}`
	result := extractToolCallFromText(input)
	if result != "" {
		t.Errorf("expected empty result for already-JSON input, got: %s", result)
	}
}

func TestNormalizeToolCallFormat_NaturalLanguage(t *testing.T) {
	input := "我将帮您查询当前的主机情况。首先，我会调用 Host.List 工具来获取所有主机的列表和概览信息"
	result := normalizeToolCallFormat(input)
	t.Logf("Input: %s", input)
	t.Logf("Output: %s", result)
	if result == input {
		t.Error("normalizeToolCallFormat did not transform natural language input")
	}
	if !containsSubstring(result, `"action":"tool_call"`) {
		t.Error("expected output to contain action:tool_call")
	}
}

func TestNormalizeToolCallFormat_AlreadyNormalized(t *testing.T) {
	input := `{"action":"tool_call","tool_call":{"tool_name":"Host.List","args":{}}}`
	result := normalizeToolCallFormat(input)
	if result != input {
		t.Errorf("expected unchanged output for already-normalized input")
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
