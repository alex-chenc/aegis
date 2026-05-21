package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestNormalizeImageModelConfigProviderDefaults(t *testing.T) {
	tests := []struct {
		name      string
		input     ImageModelConfigRequest
		provider  string
		baseURL   string
		modelName string
	}{
		{
			name:      "minimax uses image generation defaults",
			input:     ImageModelConfigRequest{Provider: "minimax"},
			provider:  "minimax",
			baseURL:   "https://api.minimax.io/v1",
			modelName: "image-01",
		},
		{
			name:      "unknown provider falls back to custom",
			input:     ImageModelConfigRequest{Provider: "unknown", BaseURL: "https://image.example/v1", ModelName: "demo-image"},
			provider:  "custom",
			baseURL:   "https://image.example/v1",
			modelName: "demo-image",
		},
		{
			name:      "zhipu uses cogview defaults",
			input:     ImageModelConfigRequest{Provider: "zhipu"},
			provider:  "zhipu",
			baseURL:   "https://open.bigmodel.cn/api/paas/v4",
			modelName: "cogview-3-flash",
		},
		{
			name:      "minimax url infers provider",
			input:     ImageModelConfigRequest{BaseURL: "https://api.minimax.io/v1"},
			provider:  "minimax",
			baseURL:   "https://api.minimax.io/v1",
			modelName: "image-01",
		},
		{
			name:      "zhipu url infers provider",
			input:     ImageModelConfigRequest{BaseURL: "https://open.bigmodel.cn/api/paas/v4"},
			provider:  "zhipu",
			baseURL:   "https://open.bigmodel.cn/api/paas/v4",
			modelName: "cogview-3-flash",
		},
		{
			name:      "openai uses dall-e defaults",
			input:     ImageModelConfigRequest{Provider: "openai"},
			provider:  "openai",
			baseURL:   "https://api.openai.com/v1",
			modelName: "dall-e-3",
		},
		{
			name:      "openai url infers provider",
			input:     ImageModelConfigRequest{BaseURL: "https://api.openai.com/v1"},
			provider:  "openai",
			baseURL:   "https://api.openai.com/v1",
			modelName: "dall-e-3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.input
			normalizeImageModelConfigRequest(&req)

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

func TestZhipuImageModelConnectionUsesGenerationsEndpoint(t *testing.T) {
	var receivedPath string
	var receivedModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("expected bearer auth header, got %q", got)
		}

		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		receivedModel = payload.Model

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":123,"data":[{"url":"https://example.test/image.png"}]}`))
	}))
	defer server.Close()

	req := ImageModelConfigRequest{
		APIKey:    "test-key",
		Provider:  "zhipu",
		BaseURL:   server.URL + "/api/paas/v4",
		ModelName: "cogview-3-flash",
	}

	if err := testImageModel(context.Background(), req); err != nil {
		t.Fatalf("expected zhipu image model test to pass, got %v", err)
	}
	if receivedPath != "/api/paas/v4/images/generations" {
		t.Fatalf("expected zhipu generations path, got %q", receivedPath)
	}
	if receivedModel != "cogview-3-flash" {
		t.Fatalf("expected zhipu model, got %q", receivedModel)
	}
}

func TestGenerateZhipuImageModelReturnsImageURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":123,"data":[{"url":"https://example.test/trace.png"}]}`))
	}))
	defer server.Close()

	req := ImageModelConfigRequest{
		APIKey:    "test-key",
		Provider:  "zhipu",
		BaseURL:   server.URL + "/api/paas/v4",
		ModelName: "cogview-3-flash",
	}

	imageURL, err := generateImageModel(context.Background(), req, "生成攻击溯源图")
	if err != nil {
		t.Fatalf("expected zhipu image generation to pass, got %v", err)
	}
	if imageURL != "https://example.test/trace.png" {
		t.Fatalf("expected image url, got %q", imageURL)
	}
}

func TestOpenAIImageModelConnectionUsesGenerationsEndpoint(t *testing.T) {
	var receivedPath string
	var receivedModel string
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("expected bearer auth header, got %q", got)
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		receivedBody = payload
		if m, ok := payload["model"].(string); ok {
			receivedModel = m
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":123,"data":[{"url":"https://example.test/openai.png"}]}`))
	}))
	defer server.Close()

	req := ImageModelConfigRequest{
		APIKey:    "test-key",
		Provider:  "openai",
		BaseURL:   server.URL + "/v1",
		ModelName: "dall-e-3",
	}

	if err := testImageModel(context.Background(), req); err != nil {
		t.Fatalf("expected openai image model test to pass, got %v", err)
	}
	if receivedPath != "/v1/images/generations" {
		t.Fatalf("expected openai generations path, got %q", receivedPath)
	}
	if receivedModel != "dall-e-3" {
		t.Fatalf("expected openai model, got %q", receivedModel)
	}
	if receivedBody["response_format"] != "url" {
		t.Fatalf("expected response_format 'url', got %v", receivedBody["response_format"])
	}
	if _, ok := receivedBody["watermark_enabled"]; ok {
		t.Fatalf("openai request should not contain watermark_enabled")
	}
}

func TestGenerateOpenAIImageModelReturnsImageURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":123,"data":[{"url":"https://example.test/openai.png"}]}`))
	}))
	defer server.Close()

	req := ImageModelConfigRequest{
		APIKey:    "test-key",
		Provider:  "openai",
		BaseURL:   server.URL + "/v1",
		ModelName: "dall-e-3",
	}

	imageURL, err := generateImageModel(context.Background(), req, "A security incident flowchart")
	if err != nil {
		t.Fatalf("expected openai image generation to pass, got %v", err)
	}
	if imageURL != "https://example.test/openai.png" {
		t.Fatalf("expected image url, got %q", imageURL)
	}
}

func TestCustomProviderUsesOpenAIFormat(t *testing.T) {
	var receivedPath string
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		receivedBody = payload

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":123,"data":[{"url":"https://example.test/custom.png"}]}`))
	}))
	defer server.Close()

	req := ImageModelConfigRequest{
		APIKey:    "test-key",
		Provider:  "custom",
		BaseURL:   server.URL + "/v1",
		ModelName: "some-model",
	}

	imageURL, err := generateImageModel(context.Background(), req, "test prompt")
	if err != nil {
		t.Fatalf("expected custom image generation to pass, got %v", err)
	}
	if imageURL != "https://example.test/custom.png" {
		t.Fatalf("expected image url, got %q", imageURL)
	}
	if receivedPath != "/v1/images/generations" {
		t.Fatalf("expected custom to use openai generations path, got %q", receivedPath)
	}
	if receivedBody["response_format"] != "url" {
		t.Fatalf("expected custom to use openai response_format, got %v", receivedBody["response_format"])
	}
}
