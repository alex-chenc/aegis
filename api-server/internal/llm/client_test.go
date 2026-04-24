package llm

import "testing"

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
