package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"api-server/internal/repository"
	"api-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestConfigRouter(t *testing.T) *gin.Engine {
	t.Helper()

	if logger.Logger == nil {
		logger.Logger = zap.NewNop()
		logger.Sugar = logger.Logger.Sugar()
	}

	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	createLLMConfigTable := `
		CREATE TABLE llm_configs (
			id text PRIMARY KEY,
			api_key_encrypted text NOT NULL,
			api_key_masked text NOT NULL,
			provider text NOT NULL,
			base_url text NOT NULL,
			model_name text NOT NULL,
			is_active boolean NOT NULL,
			last_test_status text,
			last_test_at datetime,
			created_at datetime,
			updated_at datetime
		)`
	if err := db.Exec(createLLMConfigTable).Error; err != nil {
		t.Fatalf("failed to create llm config table: %v", err)
	}

	configHandler := NewConfigHandler(repository.NewConfigRepository(db, "test-encryption-key"), "test-encryption-key")
	router := gin.New()
	group := router.Group("/api/v1/config")
	group.GET("/llm", configHandler.GetLLMConfig)
	return router
}

func TestGetLLMConfigReturnsDefaultWhenEmpty(t *testing.T) {
	router := newTestConfigRouter(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/llm", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			APIKeyMasked string `json:"api_key_masked"`
			Provider     string `json:"provider"`
			BaseURL      string `json:"base_url"`
			ModelName    string `json:"model_name"`
			IsActive     bool   `json:"is_active"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Code != 0 {
		t.Fatalf("expected code 0, got %d with message %q", response.Code, response.Message)
	}
	if response.Data.APIKeyMasked != "" {
		t.Fatalf("expected empty masked key, got %q", response.Data.APIKeyMasked)
	}
	if response.Data.IsActive {
		t.Fatal("expected inactive default config")
	}
	if response.Data.Provider != "deepseek" {
		t.Fatalf("expected default provider deepseek, got %q", response.Data.Provider)
	}
	if response.Data.BaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("expected deepseek base url, got %q", response.Data.BaseURL)
	}
	if response.Data.ModelName != "deepseek-chat" {
		t.Fatalf("expected deepseek-chat model, got %q", response.Data.ModelName)
	}
}

func TestImageModelConfigRouteRemoved(t *testing.T) {
	router := newTestConfigRouter(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/image-model", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected removed image model route to return 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

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
