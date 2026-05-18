package adapters

import (
	"testing"

	agentruntime "github.com/chenchen511/agent-runtime"

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
