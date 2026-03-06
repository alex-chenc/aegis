package service

import (
	"baseline-system/internal/llm"
)

// LLMService LLM 服务
type LLMService struct {
	client *llm.LLMClient
}

// NewLLMService 创建 LLM 服务
func NewLLMService(client *llm.LLMClient) *LLMService {
	return &LLMService{
		client: client,
	}
}
