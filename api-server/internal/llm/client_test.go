package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatCompletionWithMessagesMaxTokensUsesBoundedOutputBudget(t *testing.T) {
	var captured ChatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{}"}}]}`))
	}))
	defer server.Close()

	client := NewLLMClient("test-key", server.URL+"/v1", "test-model", 30, 1)
	result, err := client.ChatCompletionWithMessagesMaxTokens(context.Background(), []Message{{Role: "user", Content: "select tools"}}, 0.1, 4096)
	if err != nil {
		t.Fatalf("completion failed: %v", err)
	}
	if result != "{}" {
		t.Fatalf("unexpected result %q", result)
	}
	if captured.MaxTokens != 4096 {
		t.Fatalf("max_tokens = %d, want 4096", captured.MaxTokens)
	}
}

func TestMiniMaxM2UsesAnthropicEndpointFromOpenAIBaseURL(t *testing.T) {
	client := NewLLMClient("test-key", "https://api.minimaxi.com/v1", "MiniMax-M2.7", 30, 1)

	if !client.usesAnthropicAPI() {
		t.Fatal("expected MiniMax M2 model to use Anthropic-compatible API")
	}

	want := "https://api.minimaxi.com/anthropic/v1/messages"
	if got := client.anthropicMessagesURL(); got != want {
		t.Fatalf("expected anthropic messages url %q, got %q", want, got)
	}
}

func TestMiniMaxOpenAIModelKeepsChatCompletionsEndpoint(t *testing.T) {
	client := NewLLMClient("test-key", "https://api.minimaxi.com/v1", "abab6.5s-chat", 30, 1)

	if client.usesAnthropicAPI() {
		t.Fatal("expected non-M2 MiniMax model to keep OpenAI-compatible API")
	}

	want := "https://api.minimaxi.com/v1/chat/completions"
	if got := client.chatCompletionsURL(); got != want {
		t.Fatalf("expected chat completions url %q, got %q", want, got)
	}
}

func TestIsDashScope_FromDashScopeURL(t *testing.T) {
	client := NewLLMClient("key", "https://dashscope.aliyuncs.com/compatible-mode/v1", "qwen-plus", 30, 1)
	if !client.IsDashScope() {
		t.Fatal("expected dashscope URL to be detected")
	}
}

func TestIsDashScope_FromAliyuncsURL(t *testing.T) {
	client := NewLLMClient("key", "https://custom.aliyuncs.com/v1", "qwen-max", 30, 1)
	if !client.IsDashScope() {
		t.Fatal("expected aliyuncs URL to be detected")
	}
}

func TestIsDashScope_NonDashScope(t *testing.T) {
	client := NewLLMClient("key", "https://api.deepseek.com/v1", "deepseek-chat", 30, 1)
	if client.IsDashScope() {
		t.Fatal("expected non-dashscope URL to not be detected")
	}
}

func TestIsDashScope_OpenAI(t *testing.T) {
	client := NewLLMClient("key", "https://api.openai.com/v1", "gpt-4o-mini", 30, 1)
	if client.IsDashScope() {
		t.Fatal("expected openai URL to not be detected as dashscope")
	}
}

func TestChatCompletionRequest_ResponseFormatMarshal(t *testing.T) {
	req := ChatCompletionRequest{
		Model:          "qwen-plus",
		Messages:       []Message{{Role: "user", Content: "test json output"}},
		ResponseFormat: &ResponseFormat{Type: "json_object"},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if !strings.Contains(string(data), `"response_format":{"type":"json_object"}`) {
		t.Fatalf("expected response_format in JSON, got: %s", data)
	}
}

func TestChatCompletionRequest_NoResponseFormat(t *testing.T) {
	req := ChatCompletionRequest{
		Model:    "qwen-plus",
		Messages: []Message{{Role: "user", Content: "test"}},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(data), "response_format") {
		t.Fatalf("response_format should be omitted when nil, got: %s", data)
	}
}

func TestResponseFormat_WithSchema(t *testing.T) {
	rf := &ResponseFormat{
		Type: "json_schema",
		JSONSchema: &ResponseFormatSchema{
			Name:   "plan",
			Schema: json.RawMessage(`{"type":"object","properties":{"goal":{"type":"string"}}}`),
		},
	}
	data, err := json.Marshal(rf)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if !strings.Contains(string(data), `"type":"json_schema"`) {
		t.Errorf("expected json_schema type, got: %s", data)
	}
	if !strings.Contains(string(data), `"name":"plan"`) {
		t.Errorf("expected schema name, got: %s", data)
	}
}

// === Phase 1: Usage parsing tests ===

func TestUsageStruct_OpenAICompatible(t *testing.T) {
	raw := `{
		"choices": [{"message": {"role": "assistant", "content": "hello"}}],
		"usage": {"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150}
	}`
	var resp ChatCompletionResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("expected Usage to be non-nil")
	}
	if resp.Usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 50 {
		t.Errorf("CompletionTokens = %d, want 50", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", resp.Usage.TotalTokens)
	}
}

func TestUsageStruct_OpenAINoUsage(t *testing.T) {
	raw := `{
		"choices": [{"message": {"role": "assistant", "content": "hello"}}]
	}`
	var resp ChatCompletionResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Usage != nil {
		t.Errorf("expected Usage to be nil when not present, got %+v", resp.Usage)
	}
}

func TestUsageStruct_AnthropicCompatible(t *testing.T) {
	raw := `{
		"content": [{"type": "text", "text": "hello"}],
		"usage": {"input_tokens": 200, "output_tokens": 80}
	}`
	var resp anthropicMessageResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("expected Usage to be non-nil")
	}
	if resp.Usage.InputTokens != 200 {
		t.Errorf("InputTokens = %d, want 200", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 80 {
		t.Errorf("OutputTokens = %d, want 80", resp.Usage.OutputTokens)
	}
}

func TestCompletionResult_StructFields(t *testing.T) {
	result := CompletionResult{
		Content: "test content",
		Model:   "gpt-4",
		Usage: LLMUsage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}
	if result.Content != "test content" {
		t.Errorf("Content = %q, want %q", result.Content, "test content")
	}
	if result.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", result.Model, "gpt-4")
	}
	if result.Usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", result.Usage.PromptTokens)
	}
}

func TestLLMUsage_Struct(t *testing.T) {
	usage := LLMUsage{
		PromptTokens:     1000,
		CompletionTokens: 500,
		TotalTokens:      1500,
	}
	if usage.PromptTokens != 1000 {
		t.Errorf("PromptTokens = %d, want 1000", usage.PromptTokens)
	}
	if usage.CompletionTokens != 500 {
		t.Errorf("CompletionTokens = %d, want 500", usage.CompletionTokens)
	}
	if usage.TotalTokens != 1500 {
		t.Errorf("TotalTokens = %d, want 1500", usage.TotalTokens)
	}
}
