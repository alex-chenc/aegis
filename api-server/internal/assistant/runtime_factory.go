package assistant

import (
	"context"
	"fmt"
	"time"

	"api-server/internal/llm"
	"api-server/internal/repository"
	"go.uber.org/zap"
)

// RuntimeFactory 运行时工厂
type RuntimeFactory struct {
	configRepo *repository.ConfigRepository
	logger     *zap.Logger
}

// NewRuntimeFactory 创建运行时工厂
func NewRuntimeFactory(configRepo *repository.ConfigRepository, logger *zap.Logger) *RuntimeFactory {
	return &RuntimeFactory{
		configRepo: configRepo,
		logger:     logger,
	}
}

// RuntimeConfig 运行时配置
type RuntimeConfig struct {
	MaxTotalTurns      int           `json:"max_total_turns"`
	MaxPlanSteps       int           `json:"max_plan_steps"`
	MaxStepReactTurns  int           `json:"max_step_react_turns"`
	MaxToolCalls       int           `json:"max_tool_calls"`
	MaxToolCallsPerStep int          `json:"max_tool_calls_per_step"`
	TaskTimeout        time.Duration `json:"task_timeout"`
	ModelTimeout       time.Duration `json:"model_timeout"`
	ToolTimeout        time.Duration `json:"tool_timeout"`
	EnableReflection   bool          `json:"enable_reflection"`
	EnableAudit        bool          `json:"enable_audit"`
	EnableCorrection   bool          `json:"enable_correction"`
	MaxContextTokens   int           `json:"max_context_tokens"`
	ReservedOutputTokens int         `json:"reserved_output_tokens"`
	EnableContextCompress bool       `json:"enable_context_compress"`
}

// DefaultAssistantRuntimeConfig 默认助手运行时配置
func DefaultAssistantRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		MaxTotalTurns:      80,
		MaxPlanSteps:       8,
		MaxStepReactTurns:  8,
		MaxToolCalls:       60,
		MaxToolCallsPerStep: 6,
		TaskTimeout:        30 * time.Minute,
		ModelTimeout:       60 * time.Second,
		ToolTimeout:        60 * time.Second,
		EnableReflection:   true,
		EnableAudit:        true,
		EnableCorrection:   true,
		MaxContextTokens:   256000,
		ReservedOutputTokens: 8192,
		EnableContextCompress: true,
	}
}

// BuildLLMClient 构建 LLM 客户端
func (f *RuntimeFactory) BuildLLMClient(ctx context.Context) (*llm.LLMClient, error) {
	// Load LLM config from database
	config, err := f.configRepo.GetActive()
	if err != nil || config == nil {
		return nil, fmt.Errorf("LLM config not found")
	}

	// Decrypt API key (与 AI 分析页面保持一致)
	apiKey, err := f.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt API key: %w", err)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("LLM API key not configured")
	}

	client := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, 60, 3)

	return client, nil
}

// PromptInput 提示词输入
type PromptInput struct {
	TaskType    string             `json:"task_type"`
	UserMessage string             `json:"user_message"`
	ContextRefs []ContextRefResult `json:"context_refs,omitempty"`
}

// ContextRefResult 上下文引用结果
type ContextRefResult struct {
	ObjectType string `json:"object_type"`
	ObjectID   string `json:"object_id"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
}
