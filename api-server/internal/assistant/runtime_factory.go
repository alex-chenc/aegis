package assistant

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentruntime "github.com/alex-chenc/agent-runtime"
	"github.com/alex-chenc/agent-runtime/router"
	"go.uber.org/zap"

	"api-server/internal/llm"
	"api-server/internal/llm/adapters"
	"api-server/internal/model"
	"api-server/internal/repository"
)

// RuntimeFactory 运行时工厂（对齐设计文档 4.2 节）
// 集中管理 agent-runtime 实例的创建，Orchestrator 不直接构造 runtime
type RuntimeFactory struct {
	configRepo     *repository.ConfigRepository
	catalog        *ToolCatalog
	toolDispatcher *ToolDispatcher
	runManager     *RunManager
	memoryRepo     repository.AssistantMemoryRepository
	logger         *zap.Logger
}

// RuntimeFactoryDeps 运行时工厂依赖
type RuntimeFactoryDeps struct {
	ConfigRepo     *repository.ConfigRepository
	Catalog        *ToolCatalog
	ToolDispatcher *ToolDispatcher
	RunManager     *RunManager
	MemoryRepo     repository.AssistantMemoryRepository
	Logger         *zap.Logger
}

// NewRuntimeFactory 创建运行时工厂
func NewRuntimeFactory(deps RuntimeFactoryDeps) *RuntimeFactory {
	runtimeLogger := deps.Logger
	if runtimeLogger == nil {
		runtimeLogger = zap.NewNop()
	}
	return &RuntimeFactory{
		configRepo:     deps.ConfigRepo,
		catalog:        deps.Catalog,
		toolDispatcher: deps.ToolDispatcher,
		runManager:     deps.RunManager,
		memoryRepo:     deps.MemoryRepo,
		logger:         runtimeLogger,
	}
}

// RuntimeBuildRequest 运行时构建请求
type RuntimeBuildRequest struct {
	SessionID         string                        `json:"session_id"`
	RunID             string                        `json:"run_id"`
	MessageID         string                        `json:"message_id"`
	Operator          string                        `json:"operator"`
	UserInput         string                        `json:"user_input"`
	TaskType          string                        `json:"task_type"`
	ContextRefs       []ContextRefResult            `json:"context_refs,omitempty"`
	PageRoute         string                        `json:"page_route,omitempty"`
	PreviousSummary   string                        `json:"previous_summary,omitempty"`
	MaxIterations     int                           `json:"max_iterations,omitempty"`
	SelectedTools     []string                      `json:"selected_tools,omitempty"`
	ToolDescriptors   []agentruntime.ToolDescriptor `json:"-"`
	ExecutionPlan     *ToolExecutionPlan            `json:"execution_plan,omitempty"`
	UseAIAnalysisFlow bool                          `json:"use_ai_analysis_flow,omitempty"`
	Locale            string                        `json:"locale,omitempty"`
	ApprovalMode      string                        `json:"approval_mode,omitempty"`
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
	invariantRequest := req
	invariantRequest.ToolDescriptors = toolDescriptors
	if err := validateRuntimeToolElectionInvariant(invariantRequest); err != nil {
		f.logger.Error("assistant runtime rejected an unmapped tool election path",
			zap.String("session_id", req.SessionID),
			zap.String("run_id", req.RunID),
			zap.Int("tool_count", len(toolDescriptors)),
			zap.String("plan_id", toolExecutionPlanID(req.ExecutionPlan)),
			zap.Error(err),
		)
		return nil, err
	}
	// Compiled plan validation: catch bad argument types, missing previous_step
	// producers, and unclosed async completion contracts before the plan reaches
	// the runtime. A deterministic compile error must surface once, not as a
	// repeated identical runtime tool-call failure.
	if f.catalog != nil {
		validator := NewCompiledPlanValidator(f.catalog.Registry(), f.logger)
		if err := validator.Validate(req.ExecutionPlan, toolDescriptors); err != nil {
			f.logger.Error("assistant compiled plan rejected before runtime",
				zap.String("session_id", req.SessionID),
				zap.String("run_id", req.RunID),
				zap.String("plan_id", toolExecutionPlanID(req.ExecutionPlan)),
				zap.Error(err),
			)
			return nil, err
		}
	}
	runtimeStepToolBindings := buildRuntimeStepToolBindings(req.ExecutionPlan, toolDescriptors)

	// 4. 创建 ToolGateway（实现 agentruntime.ToolGateway）
	toolGateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher:              f.toolDispatcher,
		SessionID:               req.SessionID,
		MessageID:               req.MessageID,
		RunID:                   req.RunID,
		Operator:                req.Operator,
		ApprovalMode:            normalizeAssistantApprovalMode(req.ApprovalMode),
		RequireMappedPlan:       true,
		Logger:                  f.logger,
		RunManager:              f.runManager,
		UserInput:               req.UserInput,
		ContextRefs:             req.ContextRefs,
		ExecutionPlan:           req.ExecutionPlan,
		RuntimeStepToolBindings: runtimeStepToolBindings,
		OnToolCall: func(callID, toolName string, args interface{}) {
			f.runManager.Publish(req.SessionID, EventToolCallPayload(req.SessionID, req.RunID, req.MessageID, callID, toolName, args))
		},
		OnToolResult: func(callID string, result interface{}, outcome *agentruntime.ToolOutcome) {
			f.runManager.Publish(req.SessionID, EventToolResultPayload(req.SessionID, req.RunID, req.MessageID, callID, result, outcome))
		},
		OnToolError: func(callID, errMsg string) {
			f.runManager.Publish(req.SessionID, EventToolErrorPayload(req.SessionID, req.RunID, req.MessageID, callID, errMsg))
		},
		OnApproval: func(approval interface{}) {
			f.runManager.Publish(req.SessionID, EventApprovalRequiredPayload(req.SessionID, req.RunID, req.MessageID, approval))
			if typed, ok := approval.(*model.AssistantApproval); ok {
				f.runManager.Publish(req.SessionID, EventRunWaitingApprovalPayload(req.SessionID, req.RunID, typed.ApprovalID, typed.ToolName))
			}
		},
		OnApprovalUpdated: func(approval interface{}) {
			f.runManager.Publish(req.SessionID, withMessageID(NewEvent(EventApprovalUpdated, req.SessionID, req.RunID, approval), req.MessageID))
		},
	})

	// 5. 创建 HookSink
	hookSink := NewAssistantHookSink(f.runManager, req.SessionID, req.RunID, req.MessageID, f.logger).
		WithLocale(req.Locale).
		WithMemoryRepository(f.memoryRepo)

	// 6. 创建 PromptProvider
	reflectionMemories := f.loadReflectionMemories(ctx, req.SessionID, 5)
	promptProvider := NewAssistantPromptProvider(toolDescriptors, req.ContextRefs, req.TaskType, req.UserInput).
		WithLocale(req.Locale).
		WithApprovalMode(req.ApprovalMode).
		WithRuntimeStepToolBindings(runtimeStepToolBindings).
		WithReflectionMemories(reflectionMemories)

	// 7. 创建 LLM 适配器
	llmAdapter := adapters.NewLLMClientAdapter(llmClient, nil)

	// 8. 构建 agent-runtime 配置
	runtimeConfig := DefaultAgentRuntimeConfig(req.MaxIterations)
	profile := "assistant"
	if req.UseAIAnalysisFlow {
		runtimeConfig = DefaultAIAnalysisRuntimeConfig(req.MaxIterations)
		profile = "ai_analysis"
	}
	if req.ExecutionPlan != nil && len(req.ExecutionPlan.Steps) > 0 {
		if len(req.ExecutionPlan.Steps) > runtimeConfig.MaxPlanSteps {
			runtimeConfig.MaxPlanSteps = len(req.ExecutionPlan.Steps)
		}
		applyFixedPlanRuntimeLimits(&runtimeConfig, len(req.ExecutionPlan.Steps))
		runtimeConfig.MaxToolFailures = 3
		runtimeConfig.EnableAudit = false
		runtimeConfig.EnableCorrection = false
		runtimeConfig.AllowDynamicNewSteps = false
		// ToolDispatcher/ApprovalGate remains the Aegis authority for write
		// approval. The runtime policy must allow selected calls to reach it.
		runtimeConfig.AllowHighRiskTools = true
		runtimeConfig.AllowDangerousTools = true
		profile += "_fixed_plan"
	}
	if f.memoryRepo != nil {
		runtimeConfig.EnableExperience = true
	}
	if len(req.SelectedTools) > 0 && len(toolDescriptors) < len(req.SelectedTools) {
		missing := missingSelectedTools(req.SelectedTools, toolDescriptors)
		f.logger.Error("assistant runtime missing selected tools",
			zap.String("session_id", req.SessionID),
			zap.Strings("missing_tools", missing),
		)
		return nil, fmt.Errorf("selected tools are unavailable: %s", strings.Join(missing, ", "))
	}
	f.logger.Info("assistant runtime config selected",
		zap.String("session_id", req.SessionID),
		zap.String("run_id", req.RunID),
		zap.String("profile", profile),
		zap.String("approval_mode", normalizeAssistantApprovalMode(req.ApprovalMode)),
		zap.Int("max_total_turns", runtimeConfig.MaxTotalTurns),
		zap.Int("tool_count", len(toolDescriptors)),
		zap.String("plan_id", toolExecutionPlanID(req.ExecutionPlan)),
		zap.Int("plan_step_count", toolExecutionPlanStepCount(req.ExecutionPlan)),
		zap.Int("runtime_step_count", runtimeExecutionStepCount(req.ExecutionPlan, toolDescriptors)),
		zap.Int("max_tool_calls", runtimeConfig.MaxToolCalls),
		zap.Int("max_tool_calls_per_step", runtimeConfig.MaxToolCallsPerStep),
		zap.Int("max_async_poll_attempts", runtimeConfig.MaxAsyncPollAttempts),
	)

	// 9. 创建 TaskRouter（智能提示词路由）
	taskRouter := NewTaskRouterAdapter(llmAdapter, GetPromptFragments(), router.Config{
		EnableLLMRouting:  true,
		LLMTemperature:    0.1,
		LLMTimeout:        30 * time.Second,
		DirectReplyMaxLen: 15,
	}).WithDirectReplyPrompt(promptProvider.buildDirectReplyPrompt())

	// 10. 创建 agent-runtime 实例
	runtimeOptions := []agentruntime.Option{
		agentruntime.WithLLMClient(llmAdapter),
		agentruntime.WithToolGateway(toolGateway),
		agentruntime.WithTools(toolDescriptors),
		agentruntime.WithHooks(hookSink),
		agentruntime.WithPromptProvider(promptProvider),
		agentruntime.WithConfig(runtimeConfig),
		agentruntime.WithRouter(taskRouter),
	}
	if f.memoryRepo != nil {
		runtimeOptions = append(runtimeOptions, agentruntime.WithExperienceProvider(newAssistantReflectionExperienceProvider(f.memoryRepo, req.SessionID)))
	}

	runtime, err := agentruntime.New(runtimeOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent-runtime: %w", err)
	}

	return &RuntimeBuildResult{
		Runtime:     runtime,
		UserContext: userContext,
	}, nil
}

// validateRuntimeToolElectionInvariant is a fail-closed architecture boundary.
// Tool descriptors without an exact capability-Mapping execution plan would
// restore free model tool_name election and are therefore forbidden.
//
// This must remain a code check, not merely a prompt instruction.
func validateRuntimeToolElectionInvariant(req RuntimeBuildRequest) error {
	if len(req.ToolDescriptors) == 0 {
		if usesMappingBoundExecutionPlan(req.ExecutionPlan) {
			return fmt.Errorf("Mapping-bound execution plan has no runtime tool descriptors")
		}
		return nil
	}
	if !usesMappingBoundExecutionPlan(req.ExecutionPlan) {
		return fmt.Errorf("assistant tool descriptors require a Mapping-bound execution plan")
	}
	if strings.TrimSpace(req.ExecutionPlan.DecisionTraceID) == "" {
		return fmt.Errorf("Mapping-bound execution plan requires a decision trace ID")
	}
	acceptedMappings := make(map[string]struct{}, len(req.ExecutionPlan.DecisionRecords))
	for _, record := range req.ExecutionPlan.DecisionRecords {
		if record.Decision != toolDecisionAccepted {
			continue
		}
		key := strings.TrimSpace(record.ToolName) + "\x00" + strings.TrimSpace(record.Capability)
		acceptedMappings[key] = struct{}{}
	}
	mappedTools := make(map[string]struct{}, len(req.ExecutionPlan.Steps))
	stepIDs := make(map[string]struct{}, len(req.ExecutionPlan.Steps))
	for _, step := range req.ExecutionPlan.Steps {
		if _, exists := stepIDs[step.StepID]; exists {
			return fmt.Errorf("Mapping-bound execution plan contains duplicate step_id %s", step.StepID)
		}
		stepIDs[step.StepID] = struct{}{}
		mappingKey := strings.TrimSpace(step.ToolName) + "\x00" + strings.TrimSpace(step.Capability)
		if _, accepted := acceptedMappings[mappingKey]; !accepted {
			return fmt.Errorf("Mapping-bound step %s has no accepted capability decision record", step.StepID)
		}
		mappedTools[step.ToolName] = struct{}{}
	}
	descriptorTools := make(map[string]struct{}, len(req.ToolDescriptors))
	for _, descriptor := range req.ToolDescriptors {
		if _, exists := descriptorTools[descriptor.Name]; exists {
			return fmt.Errorf("runtime contains duplicate tool descriptor %s", descriptor.Name)
		}
		descriptorTools[descriptor.Name] = struct{}{}
		if _, ok := mappedTools[descriptor.Name]; !ok {
			return fmt.Errorf("runtime tool %s is not present in the Mapping-bound execution plan", descriptor.Name)
		}
	}
	for toolName := range mappedTools {
		if _, ok := descriptorTools[toolName]; !ok {
			return fmt.Errorf("Mapping-bound tool %s has no runtime descriptor", toolName)
		}
	}
	return nil
}

func toolExecutionPlanStepCount(plan *ToolExecutionPlan) int {
	if plan == nil {
		return 0
	}
	return len(plan.Steps)
}

func runtimeExecutionStepCount(plan *ToolExecutionPlan, descriptors []agentruntime.ToolDescriptor) int {
	runtimePlan := runtimePlanFromToolExecutionPlanWithDescriptors(plan, descriptors)
	if runtimePlan == nil {
		return 0
	}
	return len(runtimePlan.Steps)
}

func toolExecutionPlanID(plan *ToolExecutionPlan) string {
	if plan == nil {
		return ""
	}
	return plan.DecisionTraceID
}

func (f *RuntimeFactory) loadReflectionMemories(ctx context.Context, sessionID string, limit int) []string {
	if f.memoryRepo == nil || sessionID == "" {
		return nil
	}
	memories, err := f.memoryRepo.ListBySession(ctx, sessionID, assistantReflectionMemoryType)
	if err != nil {
		f.logger.Warn("failed to load assistant reflection memories",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		return nil
	}
	if limit <= 0 || limit > len(memories) {
		limit = len(memories)
	}
	result := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		if content := strings.TrimSpace(memories[i].Content); content != "" {
			result = append(result, content)
		}
	}
	return result
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

	client := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, llm.DefaultTimeoutSeconds, llm.DefaultMaxRetries)
	return client, nil
}

// buildUserContext 构建用户上下文
func (f *RuntimeFactory) buildUserContext(contextRefs []ContextRefResult) map[string]interface{} {
	userContext := make(map[string]interface{})
	if len(contextRefs) > 0 {
		refsData := make([]map[string]interface{}, 0, len(contextRefs))
		for _, ref := range contextRefs {
			item := map[string]interface{}{
				"object_type": ref.ObjectType,
				"object_id":   ref.ObjectID,
				"title":       ref.Title,
				"summary":     ref.Summary,
			}
			if len(ref.Data) > 0 {
				item["data"] = ref.Data
			}
			refsData = append(refsData, item)
		}
		userContext["context_refs"] = refsData
	}
	return userContext
}

const (
	fixedPlanBaseToolCallsPerStep = 6
	// Fixed workflows include operations such as asset application analysis that
	// normally take several minutes. With the bounded exponential backoff in the
	// runtime, 24 automatic polls provide roughly a ten-minute observation
	// window without turning a stuck operation into an unbounded session.
	fixedPlanMaxAsyncPollAttempts = 24
	fixedPlanAsyncCallOverhead    = 2 // primary operation plus initial completion lookup
	fixedPlanTotalCallReserve     = 4
)

// applyFixedPlanRuntimeLimits keeps the generic tool-call limiter from
// pre-empting the separately bounded asynchronous completion loop. Runtime
// counts automatic completion polls as ordinary tool calls, so both the
// per-step and total budgets must be large enough for every configured poll.
func applyFixedPlanRuntimeLimits(config *agentruntime.RuntimeConfig, stepCount int) {
	if config == nil || stepCount <= 0 {
		return
	}
	if config.MaxAsyncPollAttempts < fixedPlanMaxAsyncPollAttempts {
		config.MaxAsyncPollAttempts = fixedPlanMaxAsyncPollAttempts
	}
	perStepBudget := fixedPlanBaseToolCallsPerStep
	asyncBudget := config.MaxAsyncPollAttempts + fixedPlanAsyncCallOverhead
	if asyncBudget > perStepBudget {
		perStepBudget = asyncBudget
	}
	config.MaxToolCallsPerStep = perStepBudget
	config.MaxToolCalls = stepCount*perStepBudget + fixedPlanTotalCallReserve
}

// DefaultAgentRuntimeConfig 默认 agent-runtime 配置（对齐设计文档 4.4 节）
func DefaultAgentRuntimeConfig(maxIterations int) agentruntime.RuntimeConfig {
	if maxIterations <= 0 {
		maxIterations = 80
	}
	return agentruntime.RuntimeConfig{
		MaxTotalTurns: maxIterations,
		// A dynamic agent plan may legitimately contain discovery, asynchronous
		// completion, execution, and verification steps. Keep a bounded but
		// non-trivial ceiling so valid general-purpose plans are not rejected
		// before their first tool call.
		MaxPlanSteps:            16,
		MaxStepReactTurns:       8,
		MaxToolCalls:            80,
		MaxToolCallsPerStep:     32,
		MaxToolFailures:         8,
		MaxModelFailures:        5,
		MaxParseFailures:        3,
		MaxNoProgressTurns:      3,
		TaskTimeout:             30 * time.Minute,
		ModelTimeout:            1200 * time.Second,
		ToolTimeout:             60 * time.Second,
		HookTimeout:             10 * time.Second,
		AsyncPollInitialBackoff: 1 * time.Second,
		AsyncPollMaxBackoff:     30 * time.Second,
		// Stop a backend operation that remains non-terminal instead of keeping
		// the assistant session in an unbounded "executing" state. With the
		// configured exponential backoff this allows several minutes for normal
		// script generation while preserving a deterministic failure boundary.
		MaxAsyncPollAttempts:  12,
		EnableReflection:      true,
		EnableAudit:           true,
		EnableCorrection:      true,
		EnableExperience:      false,
		AuditEveryNSteps:      3,
		MaxAudits:             2,
		MaxReflections:        2,
		MaxStepRetries:        1,
		MaxCorrections:        2,
		AllowDynamicNewSteps:  true,
		AllowSkipFailedStep:   false,
		AllowBestEffortAnswer: false,
		// Tool descriptors reaching Runtime have already passed Aegis capability
		// mapping and hard gates. Runtime must forward them to Aegis's approval
		// policy instead of applying a conflicting second risk denial.
		AllowHighRiskTools:    true,
		AllowDangerousTools:   true,
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

// DefaultAIAnalysisRuntimeConfig mirrors the AI analysis runtime profile used by
// api-server/internal/llm/adapters.NewAegisRuntime for complex assistant tasks.
func DefaultAIAnalysisRuntimeConfig(maxIterations int) agentruntime.RuntimeConfig {
	if maxIterations <= 0 {
		maxIterations = 500
	}
	return agentruntime.RuntimeConfig{
		MaxTotalTurns:           maxIterations,
		MaxPlanSteps:            16,
		MaxStepReactTurns:       8,
		MaxToolCalls:            160,
		MaxToolCallsPerStep:     32,
		MaxToolFailures:         15,
		MaxModelFailures:        5,
		MaxParseFailures:        3,
		MaxNoProgressTurns:      3,
		TaskTimeout:             2 * time.Hour,
		ModelTimeout:            1200 * time.Second,
		ToolTimeout:             60 * time.Second,
		HookTimeout:             10 * time.Second,
		AsyncPollInitialBackoff: 2 * time.Second,
		AsyncPollMaxBackoff:     30 * time.Second,
		MaxAsyncPollAttempts:    12,
		EnableReflection:        true,
		EnableAudit:             true,
		EnableCorrection:        true,
		EnableExperience:        false,
		AuditEveryNSteps:        3,
		MaxAudits:               2,
		MaxReflections:          3,
		MaxStepRetries:          1,
		MaxCorrections:          2,
		AllowDynamicNewSteps:    true,
		AllowSkipFailedStep:     false,
		AllowBestEffortAnswer:   false,
		// Aegis remains the authorization authority for this pre-filtered tool
		// subset (request_approval/whitelist/full_access).
		AllowHighRiskTools:    true,
		AllowDangerousTools:   true,
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

func missingSelectedTools(selected []string, descriptors []agentruntime.ToolDescriptor) []string {
	available := make(map[string]bool, len(descriptors))
	for _, desc := range descriptors {
		available[desc.Name] = true
	}
	var missing []string
	for _, name := range selected {
		if !available[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

// PromptInput 提示词输入
type PromptInput struct {
	TaskType    string             `json:"task_type"`
	UserMessage string             `json:"user_message"`
	ContextRefs []ContextRefResult `json:"context_refs,omitempty"`
}

// ContextRefResult 上下文引用结果
type ContextRefResult struct {
	ObjectType string                 `json:"object_type"`
	ObjectID   string                 `json:"object_id"`
	Title      string                 `json:"title"`
	Summary    string                 `json:"summary"`
	Data       map[string]interface{} `json:"data,omitempty"`
}
