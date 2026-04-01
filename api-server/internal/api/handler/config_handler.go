package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"api-server/internal/llm"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
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
	BaseURL   string `json:"base_url"`
	ModelName string `json:"model_name"`
}

func (h *ConfigHandler) GetLLMConfig(c *gin.Context) {
	config, err := h.configRepo.GetActive()
	if err != nil {
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
			"base_url":       config.BaseURL,
			"model_name":     config.ModelName,
			"is_active":      config.IsActive,
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

	if req.BaseURL == "" {
		req.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}

	if req.ModelName == "" {
		req.ModelName = "qwen-plus"
	}

	config := &model.LLMConfig{
		ID:           uuid.New(),
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
		zap.String("base_url", config.BaseURL),
		zap.String("model_name", config.ModelName),
	)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "config saved successfully",
		"data": gin.H{
			"api_key_masked": config.APIKeyMasked,
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

	if req.BaseURL == "" {
		req.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}

	if req.ModelName == "" {
		req.ModelName = "qwen-plus"
	}

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
		zap.String("base_url", req.BaseURL),
		zap.String("model_name", req.ModelName),
	)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "connection successful",
		"data": gin.H{
			"status":     "ok",
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
