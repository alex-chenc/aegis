package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

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
		Model:    "qwen-plus",
		Messages: []Message{{Role: "user", Content: "test json output"}},
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
