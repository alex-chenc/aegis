package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"api-server/internal/llm"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ConfigHandler struct {
	configRepo    *repository.ConfigRepository
	encryptionKey string
}

func NewConfigHandler(configRepo *repository.ConfigRepository, encryptionKey string) *ConfigHandler {
	return &ConfigHandler{
		configRepo:    configRepo,
		encryptionKey: encryptionKey,
	}
}

type LLMConfigRequest struct {
	APIKey    string `json:"api_key"`
	Provider  string `json:"provider"`
	BaseURL   string `json:"base_url"`
	ModelName string `json:"model_name"`
}

type llmProviderPreset struct {
	Provider     string
	DefaultURL   string
	DefaultModel string
}

var llmProviderPresets = map[string]llmProviderPreset{
	"deepseek": {
		Provider:     "deepseek",
		DefaultURL:   "https://api.deepseek.com/v1",
		DefaultModel: "deepseek-chat",
	},
	"dashscope": {
		Provider:     "dashscope",
		DefaultURL:   "https://dashscope.aliyuncs.com/compatible-mode/v1",
		DefaultModel: "qwen-plus",
	},
	"minimax": {
		Provider:     "minimax",
		DefaultURL:   "https://api.minimaxi.com/anthropic",
		DefaultModel: "MiniMax-M2.7",
	},
	"openai": {
		Provider:     "openai",
		DefaultURL:   "https://api.openai.com/v1",
		DefaultModel: "gpt-4o-mini",
	},
	"custom": {
		Provider:     "custom",
		DefaultURL:   "https://api.deepseek.com/v1",
		DefaultModel: "deepseek-chat",
	},
}

func normalizeLLMConfigRequest(req *LLMConfigRequest) {
	req.Provider = normalizeLLMProvider(req.Provider, req.BaseURL)
	preset := llmProviderPresets[req.Provider]

	if req.BaseURL == "" {
		req.BaseURL = preset.DefaultURL
	}
	if req.ModelName == "" {
		req.ModelName = preset.DefaultModel
	}
}

func normalizeLLMProvider(provider, baseURL string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if _, ok := llmProviderPresets[normalized]; ok {
		return normalized
	}

	base := strings.ToLower(baseURL)
	switch {
	case strings.Contains(base, "deepseek"):
		return "deepseek"
	case strings.Contains(base, "dashscope") || strings.Contains(base, "aliyuncs"):
		return "dashscope"
	case strings.Contains(base, "minimaxi") || strings.Contains(base, "minimax"):
		return "minimax"
	case strings.Contains(base, "openai"):
		return "openai"
	default:
		return "custom"
	}
}

func displayLLMProvider(provider, baseURL string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if normalized == "" || normalized == "custom" {
		inferred := normalizeLLMProvider("", baseURL)
		if inferred != "custom" {
			return inferred
		}
	}
	return normalizeLLMProvider(provider, baseURL)
}

func (h *ConfigHandler) GetLLMConfig(c *gin.Context) {
	config, err := h.configRepo.GetActive()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeDefaultLLMConfig(c)
			return
		}
		logger.Error("failed to get LLM config", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{
			"code":    500,
			"message": "failed to get config",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"api_key_masked": config.APIKeyMasked,
			"provider":       displayLLMProvider(config.Provider, config.BaseURL),
			"base_url":       config.BaseURL,
			"model_name":     config.ModelName,
			"is_active":      config.IsActive,
		},
	})
}

func writeDefaultLLMConfig(c *gin.Context) {
	preset := llmProviderPresets["deepseek"]
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"api_key_masked": "",
			"provider":       preset.Provider,
			"base_url":       preset.DefaultURL,
			"model_name":     preset.DefaultModel,
			"is_active":      false,
		},
	})
}

func (h *ConfigHandler) SaveLLMConfig(c *gin.Context) {
	var req LLMConfigRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid request body",
		})
		return
	}

	if req.APIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "API key is required",
		})
		return
	}

	normalizeLLMConfigRequest(&req)

	config := &model.LLMConfig{
		ID:           uuid.New(),
		Provider:     req.Provider,
		BaseURL:      req.BaseURL,
		ModelName:    req.ModelName,
		IsActive:     true,
		APIKeyMasked: maskAPIKey(req.APIKey),
	}

	if err := h.configRepo.Upsert(config, req.APIKey); err != nil {
		logger.Error("failed to save LLM config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to save config",
		})
		return
	}

	logger.Info("LLM config saved successfully",
		zap.String("id", config.ID.String()),
		zap.String("provider", config.Provider),
		zap.String("base_url", config.BaseURL),
		zap.String("model_name", config.ModelName),
	)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "config saved successfully",
		"data": gin.H{
			"api_key_masked": config.APIKeyMasked,
			"provider":       config.Provider,
			"base_url":       config.BaseURL,
			"model_name":     config.ModelName,
		},
	})
}

func (h *ConfigHandler) TestLLMConnection(c *gin.Context) {
	var req LLMConfigRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid request body",
		})
		return
	}

	if req.APIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "API key is required",
		})
		return
	}

	normalizeLLMConfigRequest(&req)

	client := llm.NewLLMClient(req.APIKey, req.BaseURL, req.ModelName, 30, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := client.ChatCompletion(ctx, "You are a helpful assistant.", "Hi, please respond with 'OK' to confirm you're working.", 0.1)
	if err != nil {
		logger.Error("LLM connection test failed", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{
			"code":    1001,
			"message": fmt.Sprintf("connection failed: %v", err),
		})
		return
	}

	logger.Info("LLM connection test successful",
		zap.String("provider", req.Provider),
		zap.String("base_url", req.BaseURL),
		zap.String("model_name", req.ModelName),
	)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "connection successful",
		"data": gin.H{
			"status":     "ok",
			"provider":   req.Provider,
			"model_name": req.ModelName,
		},
	})
}

func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "****"
	}
	return apiKey[:4] + "****" + apiKey[len(apiKey)-4:]
}

// GetFullAPIKey 获取完整API Key（需要验证）
func (h *ConfigHandler) GetFullAPIKey(c *gin.Context) {
	config, err := h.configRepo.GetActive()
	if err != nil {
		logger.Error("failed to get LLM config", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{
			"code":    500,
			"message": "failed to get config",
		})
		return
	}

	apiKey, err := h.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		logger.Error("failed to decrypt API key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to decrypt api key",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"api_key": apiKey,
		},
	})
}
