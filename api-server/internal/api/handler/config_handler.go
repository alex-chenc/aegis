package handler

import (
	"bytes"
	"context"
	"encoding/json"
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

type ImageModelConfigRequest struct {
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

type imageModelProviderPreset struct {
	Provider     string
	DefaultURL   string
	DefaultModel string
}

var imageModelProviderPresets = map[string]imageModelProviderPreset{
	"minimax": {
		Provider:     "minimax",
		DefaultURL:   "https://api.minimax.io/v1",
		DefaultModel: "image-01",
	},
	"zhipu": {
		Provider:     "zhipu",
		DefaultURL:   "https://open.bigmodel.cn/api/paas/v4",
		DefaultModel: "cogview-3-flash",
	},
	"openai": {
		Provider:     "openai",
		DefaultURL:   "https://api.openai.com/v1",
		DefaultModel: "dall-e-3",
	},
	"custom": {
		Provider:     "custom",
		DefaultURL:   "https://api.minimax.io/v1",
		DefaultModel: "image-01",
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

func normalizeImageModelConfigRequest(req *ImageModelConfigRequest) {
	req.Provider = normalizeImageModelProvider(req.Provider, req.BaseURL)
	preset := imageModelProviderPresets[req.Provider]

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

func normalizeImageModelProvider(provider, baseURL string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if _, ok := imageModelProviderPresets[normalized]; ok {
		return normalized
	}

	base := strings.ToLower(baseURL)
	switch {
	case strings.Contains(base, "minimax"):
		return "minimax"
	case strings.Contains(base, "bigmodel") || strings.Contains(base, "zhipu"):
		return "zhipu"
	case strings.Contains(base, "openai"):
		return "openai"
	default:
		return "custom"
	}
}

func displayImageModelProvider(provider, baseURL string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if normalized == "" || normalized == "custom" {
		inferred := normalizeImageModelProvider("", baseURL)
		if inferred != "custom" {
			return inferred
		}
	}
	return normalizeImageModelProvider(provider, baseURL)
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

func (h *ConfigHandler) GetImageModelConfig(c *gin.Context) {
	config, err := h.configRepo.GetActiveImageModel()
	if err != nil {
		logger.Error("failed to get image model config", zap.Error(err))
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
			"provider":       displayImageModelProvider(config.Provider, config.BaseURL),
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

func (h *ConfigHandler) SaveImageModelConfig(c *gin.Context) {
	var req ImageModelConfigRequest

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

	normalizeImageModelConfigRequest(&req)

	config := &model.ImageModelConfig{
		ID:           uuid.New(),
		Provider:     req.Provider,
		BaseURL:      strings.TrimRight(req.BaseURL, "/"),
		ModelName:    req.ModelName,
		IsActive:     true,
		APIKeyMasked: maskAPIKey(req.APIKey),
	}

	if err := h.configRepo.UpsertImageModel(config, req.APIKey); err != nil {
		logger.Error("failed to save image model config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to save config",
		})
		return
	}

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

func (h *ConfigHandler) TestImageModelConnection(c *gin.Context) {
	var req ImageModelConfigRequest

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

	normalizeImageModelConfigRequest(&req)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := testImageModel(ctx, req); err != nil {
		logger.Error("image model connection test failed", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{
			"code":    1001,
			"message": fmt.Sprintf("connection failed: %v", err),
		})
		return
	}

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

func testImageModel(ctx context.Context, req ImageModelConfigRequest) error {
	_, err := generateImageModel(ctx, req, "A minimal security incident flowchart with two nodes and one arrow, clean vector style.")
	return err
}

func generateImageModel(ctx context.Context, req ImageModelConfigRequest, prompt string) (string, error) {
	if req.Provider == "zhipu" || strings.Contains(strings.ToLower(req.BaseURL), "bigmodel") {
		return generateZhipuImageModel(ctx, req, prompt)
	}
	if req.Provider == "openai" || req.Provider == "custom" {
		return generateOpenAIImageModel(ctx, req, prompt)
	}
	return generateMinimaxImageModel(ctx, req, prompt)
}

func generateMinimaxImageModel(ctx context.Context, req ImageModelConfigRequest, prompt string) (string, error) {
	endpoint := strings.TrimRight(req.BaseURL, "/") + "/image_generation"
	body := map[string]interface{}{
		"model":            req.ModelName,
		"prompt":           prompt,
		"aspect_ratio":     "16:9",
		"response_format":  "url",
		"n":                1,
		"prompt_optimizer": false,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("image API returned HTTP %d", resp.StatusCode)
	}

	var decoded struct {
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
		Data struct {
			ImageURLs []string `json:"image_urls"`
			Images    []struct {
				URL string `json:"url"`
			} `json:"images"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if decoded.BaseResp.StatusCode != 0 {
		return "", fmt.Errorf("image API status %d: %s", decoded.BaseResp.StatusCode, decoded.BaseResp.StatusMsg)
	}
	if len(decoded.Data.ImageURLs) > 0 && decoded.Data.ImageURLs[0] != "" {
		return decoded.Data.ImageURLs[0], nil
	}
	if len(decoded.Data.Images) > 0 && decoded.Data.Images[0].URL != "" {
		return decoded.Data.Images[0].URL, nil
	}
	return "", nil
}

func generateZhipuImageModel(ctx context.Context, req ImageModelConfigRequest, prompt string) (string, error) {
	endpoint := zhipuImageEndpoint(req.BaseURL)
	body := map[string]interface{}{
		"model":             req.ModelName,
		"prompt":            prompt,
		"size":              "1024x1024",
		"watermark_enabled": true,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("image API returned HTTP %d", resp.StatusCode)
	}

	var decoded struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if decoded.Error.Code != "" || decoded.Error.Message != "" {
		return "", fmt.Errorf("image API status %s: %s", decoded.Error.Code, decoded.Error.Message)
	}
	if decoded.Code != "" || decoded.Message != "" {
		return "", fmt.Errorf("image API status %s: %s", decoded.Code, decoded.Message)
	}
	if len(decoded.Data) == 0 || decoded.Data[0].URL == "" {
		return "", fmt.Errorf("image API returned no image url")
	}
	return decoded.Data[0].URL, nil
}

func generateOpenAIImageModel(ctx context.Context, req ImageModelConfigRequest, prompt string) (string, error) {
	endpoint := zhipuImageEndpoint(req.BaseURL)
	body := map[string]interface{}{
		"model":           req.ModelName,
		"prompt":          prompt,
		"size":            "1024x1024",
		"n":               1,
		"response_format": "url",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("image API returned HTTP %d", resp.StatusCode)
	}

	var decoded struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if decoded.Error.Code != "" || decoded.Error.Message != "" {
		return "", fmt.Errorf("image API error %s: %s", decoded.Error.Code, decoded.Error.Message)
	}
	if len(decoded.Data) == 0 || decoded.Data[0].URL == "" {
		return "", fmt.Errorf("image API returned no image url")
	}
	return decoded.Data[0].URL, nil
}

func zhipuImageEndpoint(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	switch {
	case strings.HasSuffix(base, "/images/generations"):
		return base
	case strings.HasSuffix(base, "/images"):
		return base + "/generations"
	default:
		return base + "/images/generations"
	}
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

func (h *ConfigHandler) GetFullImageModelAPIKey(c *gin.Context) {
	config, err := h.configRepo.GetActiveImageModel()
	if err != nil {
		logger.Error("failed to get image model config", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{
			"code":    500,
			"message": "failed to get config",
		})
		return
	}

	apiKey, err := h.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		logger.Error("failed to decrypt image model API key", zap.Error(err))
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
