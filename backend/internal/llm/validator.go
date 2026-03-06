package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"baseline-system/pkg/logger"

	"go.uber.org/zap"
)

type LLMValidator struct {
	httpClient *http.Client
}

type ValidationResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func NewLLMValidator() *LLMValidator {
	return &LLMValidator{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (v *LLMValidator) ValidateConfig(ctx context.Context, apiKey, baseURL string) (*ValidationResult, error) {
	// Layer 1: Format validation
	if err := v.validateFormat(apiKey, baseURL); err != nil {
		return &ValidationResult{Status: "failed", Message: err.Error()}, nil
	}

	logger.Debug("LLM format validation passed",
		zap.String("base_url", baseURL),
	)

	// Layer 2: Network connectivity validation
	if err := v.validateConnectivity(ctx, apiKey, baseURL); err != nil {
		return &ValidationResult{Status: "failed", Message: err.Error()}, nil
	}

	logger.Debug("LLM connectivity validation passed",
		zap.String("base_url", baseURL),
	)

	// Layer 3: Model availability validation
	if err := v.validateModelAvailability(ctx, apiKey, baseURL); err != nil {
		return &ValidationResult{Status: "failed", Message: err.Error()}, nil
	}

	logger.Info("LLM validation completed successfully",
		zap.String("base_url", baseURL),
	)

	return &ValidationResult{Status: "ok", Message: "连接成功，模型可用"}, nil
}

func (v *LLMValidator) validateFormat(apiKey, baseURL string) error {
	// API Key validation
	if strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("API Key 不能为空")
	}
	if len(apiKey) < 10 {
		return fmt.Errorf("API Key 格式不正确，长度应大于 10 个字符")
	}

	// Base URL validation
	if strings.TrimSpace(baseURL) == "" {
		return fmt.Errorf("Base URL 不能为空")
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("Base URL 格式不正确：%w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("Base URL 必须是 HTTP 或 HTTPS 协议")
	}

	return nil
}

func (v *LLMValidator) validateConnectivity(ctx context.Context, apiKey, baseURL string) error {
	modelsURL := fmt.Sprintf("%s/models", baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", modelsURL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败：%w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	resp, err := v.httpClient.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "timeout") {
			return fmt.Errorf("网络不通，请检查 Base URL 和服务器网络")
		}
		return fmt.Errorf("网络请求失败：%w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("认证失败，请检查 API Key")
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API 返回状态码：%d", resp.StatusCode)
	}

	return nil
}

func (v *LLMValidator) validateModelAvailability(ctx context.Context, apiKey, baseURL string) error {
	completionsURL := fmt.Sprintf("%s/chat/completions", baseURL)

	jsonData := []byte(`{"model":"qwen-plus","messages":[{"role":"user","content":"ping"}],"max_tokens":5}`)

	req, err := http.NewRequestWithContext(ctx, "POST", completionsURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("创建请求失败：%w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("模型测试请求失败：%w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("认证失败，请检查 API Key")
	}

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("模型不可用，请确认模型名称")
	}

	if resp.StatusCode == http.StatusBadRequest {
		return fmt.Errorf("请求参数错误")
	}

	// 200 OK or other success codes are acceptable
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("API 返回状态码：%d", resp.StatusCode)
}
