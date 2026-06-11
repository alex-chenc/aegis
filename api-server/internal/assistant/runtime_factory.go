package assistant

import (
	"context"
	"fmt"
	"time"

	agentruntime "github.com/alex-chenc/agent-runtime"
	"github.com/alex-chenc/agent-runtime/router"
	"go.uber.org/zap"

	"api-server/internal/llm"
	"api-server/internal/llm/adapters"
	"api-server/internal/repository"
)

// RuntimeFactory 运行时工厂（对齐设计文档 4.2 节）
// 集中管理 agent-runtime 实例的创建，Orchestrator 不直接构造 runtime
type RuntimeFactory struct {
	configRepo     *repository.ConfigRepository
	catalog        *ToolCatalog
	selector       *ToolSelector
	toolDispatcher *ToolDispatcher
	runManager     *RunManager
	logger         *zap.Logger
}

// RuntimeFactoryDeps 运行时工厂依赖
type RuntimeFactoryDeps struct {
	ConfigRepo     *repository.ConfigRepository
	Catalog        *ToolCatalog
	Selector       *ToolSelector
	ToolDispatcher *ToolDispatcher
	RunManager     *RunManager
	Logger         *zap.Logger
}

// NewRuntimeFactory 创建运行时工厂
func NewRuntimeFactory(deps RuntimeFactoryDeps) *RuntimeFactory {
	return &RuntimeFactory{
		configRepo:     deps.ConfigRepo,
		catalog:        deps.Catalog,
		selector:       deps.Selector,
		toolDispatcher: deps.ToolDispatcher,
		runManager:     deps.RunManager,
		logger:         deps.Logger,
	}
}

// RuntimeBuildRequest 运行时构建请求
type RuntimeBuildRequest struct {
	SessionID       string                        `json:"session_id"`
	RunID           string                        `json:"run_id"`
	MessageID       string                        `json:"message_id"`
	Operator        string                        `json:"operator"`
	UserInput       string                        `json:"user_input"`
	TaskType        string                        `json:"task_type"`
	ContextRefs     []ContextRefResult            `json:"context_refs,omitempty"`
	PageRoute       string                        `json:"page_route,omitempty"`
	PreviousSummary string                        `json:"previous_summary,omitempty"`
	MaxIterations   int                           `json:"max_iterations,omitempty"`
	SelectedTools   []string                      `json:"selected_tools,omitempty"`
	ToolDescriptors []agentruntime.ToolDescriptor `json:"-"`
}

// RuntimeBuildResult 运行时构建结果
type RuntimeBuildResult struct {
	Runtime       *agentruntime.Runtime  `json:"-"`
	ToolSelection *ToolSelectionResult   `json:"-"`
	UserContext   map[string]interface{} `json:"user_context"`
}

// Build 构建完整的 agent-runtime 实例（对齐设计文档 4.3 节 Build 函数主流程）
func (f *RuntimeFactory) Build(ctx context.Context, req RuntimeBuildRequest) (*RuntimeBuildResult, error) {
	// 1. 构建 LLM 客户端
	llmClient, err := f.BuildLLMClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build LLM client: %w", err)
	}

	// 2. 加载用户上下文
	userContext := f.buildUserContext(req.ContextRefs)

	// 3. 构建工具描述符（使用已选中的工具）
	toolDescriptors := req.ToolDescriptors
	if len(toolDescriptors) == 0 && len(req.SelectedTools) > 0 {
		toolDescriptors = f.catalog.BuildDescriptors(req.SelectedTools)
	}

	// 4. 创建 ToolGateway（实现 agentruntime.ToolGateway）
	toolGateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher: f.toolDispatcher,
		SessionID:  req.SessionID,
		MessageID:  req.MessageID,
		RunID:      req.RunID,
		Operator:   req.Operator,
		Logger:     f.logger,
		OnToolCall: func(callID, toolName string, args interface{}) {
			f.runManager.Publish(req.SessionID, EventToolCallPayload(req.SessionID, req.RunID, req.MessageID, callID, toolName, args))
		},
		OnToolResult: func(callID string, result interface{}) {
			f.runManager.Publish(req.SessionID, EventToolResultPayload(req.SessionID, req.RunID, req.MessageID, callID, result))
		},
		OnToolError: func(callID, errMsg string) {
			f.runManager.Publish(req.SessionID, EventToolErrorPayload(req.SessionID, req.RunID, req.MessageID, callID, errMsg))
		},
		OnApproval: func(approval interface{}) {
			f.runManager.Publish(req.SessionID, EventApprovalRequiredPayload(req.SessionID, req.RunID, req.MessageID, approval))
		},
	})

	// 5. 创建 HookSink
	hookSink := NewAssistantHookSink(f.runManager, req.SessionID, req.RunID, req.MessageID, f.logger)

	// 6. 创建 PromptProvider
	promptProvider := NewAssistantPromptProvider(toolDescriptors, req.ContextRefs, req.TaskType, req.UserInput)

	// 7. 创建 LLM 适配器
	llmAdapter := adapters.NewLLMClientAdapter(llmClient, nil)

	// 8. 构建 agent-runtime 配置
	runtimeConfig := DefaultAgentRuntimeConfig(req.MaxIterations)

	// 9. 创建 TaskRouter（智能提示词路由）
	taskRouter := NewTaskRouterAdapter(llmAdapter, GetPromptFragments(), router.Config{
		EnableLLMRouting:  true,
		LLMTemperature:    0.1,
		LLMTimeout:        30 * time.Second,
		DirectReplyMaxLen: 15,
	})

	// 10. 创建 agent-runtime 实例
	runtime, err := agentruntime.New(
		agentruntime.WithLLMClient(llmAdapter),
		agentruntime.WithToolGateway(toolGateway),
		agentruntime.WithTools(toolDescriptors),
		agentruntime.WithHooks(hookSink),
		agentruntime.WithPromptProvider(promptProvider),
		agentruntime.WithConfig(runtimeConfig),
		agentruntime.WithRouter(taskRouter),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent-runtime: %w", err)
	}

	return &RuntimeBuildResult{
		Runtime:     runtime,
		UserContext: userContext,
	}, nil
}

// BuildLLMClient 构建 LLM 客户端
func (f *RuntimeFactory) BuildLLMClient(ctx context.Context) (*llm.LLMClient, error) {
	config, err := f.configRepo.GetActive()
	if err != nil || config == nil {
		return nil, fmt.Errorf("LLM config not found")
	}

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

// buildUserContext 构建用户上下文
func (f *RuntimeFactory) buildUserContext(contextRefs []ContextRefResult) map[string]interface{} {
	userContext := make(map[string]interface{})
	if len(contextRefs) > 0 {
		refsData := make([]map[string]string, 0, len(contextRefs))
		for _, ref := range contextRefs {
			refsData = append(refsData, map[string]string{
				"object_type": ref.ObjectType,
				"object_id":   ref.ObjectID,
				"title":       ref.Title,
				"summary":     ref.Summary,
			})
		}
		userContext["context_refs"] = refsData
	}
	return userContext
}

// DefaultAgentRuntimeConfig 默认 agent-runtime 配置（对齐设计文档 4.4 节）
func DefaultAgentRuntimeConfig(maxIterations int) agentruntime.RuntimeConfig {
	if maxIterations <= 0 {
		maxIterations = 80
	}
	return agentruntime.RuntimeConfig{
		MaxTotalTurns:         maxIterations,
		MaxPlanSteps:          8,
		MaxStepReactTurns:     8,
		MaxToolCalls:          60,
		MaxToolCallsPerStep:   6,
		MaxToolFailures:       8,
		MaxModelFailures:      5,
		MaxParseFailures:      3,
		MaxNoProgressTurns:    3,
		TaskTimeout:           30 * time.Minute,
		ModelTimeout:          60 * time.Second,
		ToolTimeout:           60 * time.Second,
		HookTimeout:           10 * time.Second,
		EnableReflection:      true,
		EnableAudit:           true,
		EnableCorrection:      true,
		EnableExperience:      false,
		AuditEveryNSteps:      3,
		MaxAudits:             2,
		MaxReflections:        2,
		MaxStepRetries:        2,
		MaxCorrections:        2,
		AllowDynamicNewSteps:  true,
		AllowSkipFailedStep:   true,
		AllowBestEffortAnswer: true,
		AllowHighRiskTools:    false,
		AllowDangerousTools:   false,
		MaxContextTokens:      256000,
		ReservedOutputTokens:  8192,
		EnableContextCompress: true,
		ToolCompressRatio:     0.70,
		StepCompressRatio:     0.80,
		LLMCompressRatio:      0.95,
		CompressTargetRatio:   0.60,
		RecentTurnsToKeep:     6,
	}
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
