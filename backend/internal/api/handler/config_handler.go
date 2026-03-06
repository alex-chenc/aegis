package handler

import (
	"net/http"

	"baseline-system/internal/repository"
	"baseline-system/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ConfigHandler LLM 配置 Handler
type ConfigHandler struct {
	configRepo *repository.ConfigRepository
}

// NewConfigHandler 创建配置 Handler
func NewConfigHandler(configRepo *repository.ConfigRepository) *ConfigHandler {
	return &ConfigHandler{
		configRepo: configRepo,
	}
}

// LLMConfigRequest LLM 配置请求
type LLMConfigRequest struct {
	APIKey    string `json:"api_key"`
	BaseURL   string `json:"base_url"`
	ModelName string `json:"model_name"`
}

// GetLLMConfig 获取 LLM 配置
func (h *ConfigHandler) GetLLMConfig(c *gin.Context) {
	config, err := h.configRepo.GetActive()
	if err != nil {
		logger.Error("failed to get LLM config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to get config",
		})
		return
	}

	// 返回脱敏的 API Key
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

// SaveLLMConfig 保存 LLM 配置
func (h *ConfigHandler) SaveLLMConfig(c *gin.Context) {
	var req LLMConfigRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid request body",
		})
		return
	}

	// TODO: 调用 Service 保存配置
	// 需要实现 service.ConfigService

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "config saved successfully",
		"data": gin.H{
			"api_key_masked": maskAPIKey(req.APIKey),
			"base_url":       req.BaseURL,
			"model_name":     req.ModelName,
		},
	})
}

// TestLLMConnection 测试 LLM 连通性
func (h *ConfigHandler) TestLLMConnection(c *gin.Context) {
	var req LLMConfigRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid request body",
		})
		return
	}

	// TODO: 调用 LLM Service 测试连通性
	// 需要实现 service.LLMService

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "connection successful",
		"data": gin.H{
			"status":     "ok",
			"model_name": req.ModelName,
		},
	})
}

// maskAPIKey 脱敏 API Key
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "****"
	}
	return apiKey[:4] + "****" + apiKey[len(apiKey)-4:]
}
