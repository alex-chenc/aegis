package handler

import "testing"

func TestNormalizeLLMConfigProviderDefaults(t *testing.T) {
	tests := []struct {
		name      string
		input     LLMConfigRequest
		provider  string
		baseURL   string
		modelName string
	}{
		{
			name:      "minimax uses anthropic token plan defaults",
			input:     LLMConfigRequest{Provider: "minimax"},
			provider:  "minimax",
			baseURL:   "https://api.minimaxi.com/anthropic",
			modelName: "MiniMax-M2.7",
		},
		{
			name:      "dashscope keeps explicit model",
			input:     LLMConfigRequest{Provider: "dashscope", ModelName: "qwen-max"},
			provider:  "dashscope",
			baseURL:   "https://dashscope.aliyuncs.com/compatible-mode/v1",
			modelName: "qwen-max",
		},
		{
			name:      "unknown provider falls back to custom",
			input:     LLMConfigRequest{Provider: "unknown", BaseURL: "https://llm.example/v1", ModelName: "demo-model"},
			provider:  "custom",
			baseURL:   "https://llm.example/v1",
			modelName: "demo-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.input
			normalizeLLMConfigRequest(&req)

			if req.Provider != tt.provider {
				t.Fatalf("expected provider %q, got %q", tt.provider, req.Provider)
			}
			if req.BaseURL != tt.baseURL {
				t.Fatalf("expected base url %q, got %q", tt.baseURL, req.BaseURL)
			}
			if req.ModelName != tt.modelName {
				t.Fatalf("expected model %q, got %q", tt.modelName, req.ModelName)
			}
		})
	}
}

func TestDisplayLLMProviderInfersLegacyCustomRows(t *testing.T) {
	provider := displayLLMProvider("custom", "https://api.minimaxi.com/v1")

	if provider != "minimax" {
		t.Fatalf("expected legacy custom MiniMax URL to display as minimax, got %q", provider)
	}
}
