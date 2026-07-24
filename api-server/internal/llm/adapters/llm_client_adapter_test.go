package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	agentruntime "github.com/alex-chenc/agent-runtime"
	"go.uber.org/zap"

	"api-server/internal/llm"
	applogger "api-server/pkg/logger"
)

func TestCompleteForwardsRuntimeJSONSchemaAndFallsBackOnce(t *testing.T) {
	previousLogger := applogger.Logger
	applogger.Logger = zap.NewNop()
	defer func() { applogger.Logger = previousLogger }()

	var mu sync.Mutex
	requests := make([]llm.ChatCompletionRequest, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request llm.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		requests = append(requests, request)
		mu.Unlock()
		if request.ResponseFormat != nil {
			http.Error(w, "json_schema is not supported by this model", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"action\":\"fail_step\",\"summary\":\"stop\",\"failure\":{\"reason\":\"test\",\"recoverable\":false}}"}}]}`))
	}))
	defer server.Close()

	client := llm.NewLLMClient("test-key", server.URL+"/v1", "test-model", 30, 1)
	adapter := NewLLMClientAdapter(client, nil)
	response, err := adapter.Complete(context.Background(), agentruntime.LLMRequest{
		TaskID:  "task-1",
		StepID:  "step-1",
		Purpose: agentruntime.PurposeReact,
		Messages: []agentruntime.LLMMessage{
			{Role: "system", Content: "Return exactly one JSON object."},
			{Role: "user", Content: "test"},
		},
		ResponseFormat: &agentruntime.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &agentruntime.ResponseFormatSchema{
				Name:   "react_action",
				Schema: json.RawMessage(`{"type":"object","properties":{"action":{"type":"string"}}}`),
				Strict: false,
			},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Content == "" {
		t.Fatal("Complete() returned empty content")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want schema request plus one fallback", len(requests))
	}
	if requests[0].ResponseFormat == nil || requests[0].ResponseFormat.Type != "json_schema" {
		t.Fatalf("first response_format = %#v, want json_schema", requests[0].ResponseFormat)
	}
	if requests[0].ResponseFormat.JSONSchema == nil || requests[0].ResponseFormat.JSONSchema.Name != "react_action" {
		t.Fatalf("first JSON schema = %#v", requests[0].ResponseFormat.JSONSchema)
	}
	if requests[1].ResponseFormat != nil {
		t.Fatalf("fallback response_format = %#v, want nil", requests[1].ResponseFormat)
	}
}

func TestCompleteEnforcesJSONModeForFinalSummary(t *testing.T) {
	var captured llm.ChatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"final_answer\":\"任务未执行。\"}"}}]}`))
	}))
	defer server.Close()

	client := llm.NewLLMClient("test-key", server.URL+"/v1", "test-model", 30, 1)
	adapter := NewLLMClientAdapter(client, nil)
	response, err := adapter.Complete(context.Background(), agentruntime.LLMRequest{
		TaskID:         "task-summary",
		Purpose:        agentruntime.PurposeSummarize,
		ResponseSchema: "final_summary",
		Messages: []agentruntime.LLMMessage{
			{Role: "system", Content: "Return the final answer."},
			{Role: "user", Content: "Summarize the evidence."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != `{"final_answer":"任务未执行。"}` {
		t.Fatalf("summary content = %q", response.Content)
	}
	if captured.ResponseFormat == nil || captured.ResponseFormat.Type != "json_object" {
		t.Fatalf("summary response_format = %#v, want json_object", captured.ResponseFormat)
	}
}

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
