package llm

import (
	"dc/config"
	"testing"
)

func TestIsDashScope_DashScopeURL(t *testing.T) {
	cfg := &config.LLMConfig{BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if !client.isDashScope() {
		t.Fatal("expected dashscope URL to be detected")
	}
}

func TestIsDashScope_AliyuncsURL(t *testing.T) {
	cfg := &config.LLMConfig{BaseURL: "https://custom.aliyuncs.com/v1"}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if !client.isDashScope() {
		t.Fatal("expected aliyuncs URL to be detected")
	}
}

func TestIsDashScope_DeepSeek(t *testing.T) {
	cfg := &config.LLMConfig{BaseURL: "https://api.deepseek.com/v1"}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client.isDashScope() {
		t.Fatal("expected deepseek URL to not be detected as dashscope")
	}
}

func TestIsDashScope_OpenAI(t *testing.T) {
	cfg := &config.LLMConfig{BaseURL: "https://api.openai.com/v1"}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client.isDashScope() {
		t.Fatal("expected openai URL to not be detected as dashscope")
	}
}
