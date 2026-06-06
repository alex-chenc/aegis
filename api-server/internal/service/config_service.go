package service

import (
	"context"
	"fmt"

	"api-server/internal/llm"
	"api-server/internal/repository"
)

// ConfigService 配置服务（对齐设计文档第 14.4 节）
// 从 handler 层下沉，供工具 handler 和页面 handler 共同调用
type ConfigService struct {
	configRepo *repository.ConfigRepository
}

// NewConfigService 创建配置服务
func NewConfigService(configRepo *repository.ConfigRepository) *ConfigService {
	return &ConfigService{configRepo: configRepo}
}

// LLMConfigResult LLM 配置结果
type LLMConfigResult struct {
	BaseURL   string `json:"base_url"`
	ModelName string `json:"model_name"`
	IsActive  bool   `json:"is_active"`
}

// GetLLMConfig 获取 LLM 配置（对齐 Config.LLM.Get 工具，脱敏返回）
func (s *ConfigService) GetLLMConfig(ctx context.Context) (*LLMConfigResult, error) {
	config, err := s.configRepo.GetActive()
	if err != nil || config == nil {
		return nil, fmt.Errorf("LLM config not found")
	}

	return &LLMConfigResult{
		BaseURL:   config.BaseURL,
		ModelName: config.ModelName,
		IsActive:  config.IsActive,
	}, nil
}

// TestLLMConnection 测试 LLM 连接（对齐 Config.LLM.Test 工具）
func (s *ConfigService) TestLLMConnection(ctx context.Context) (string, error) {
	config, err := s.configRepo.GetActive()
	if err != nil || config == nil {
		return "", fmt.Errorf("LLM config not found")
	}

	apiKey, err := s.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt API key: %w", err)
	}

	client := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, 30, 1)
	_, err = client.ChatCompletion(ctx, "You are a test assistant.", "Hello, respond with 'OK'", 0.1)
	if err != nil {
		return fmt.Sprintf("连接失败: %s", err.Error()), nil
	}

	return "连接成功", nil
}

// UpdateLLMConfig 更新 LLM 配置（对齐 Config.LLM.Update 工具）
func (s *ConfigService) UpdateLLMConfig(ctx context.Context, baseURL, modelName, apiKey string) error {
	config, err := s.configRepo.GetActive()
	if err != nil || config == nil {
		return fmt.Errorf("LLM config not found")
	}

	if baseURL != "" {
		config.BaseURL = baseURL
	}
	if modelName != "" {
		config.ModelName = modelName
	}

	return s.configRepo.Upsert(config, apiKey)
}
