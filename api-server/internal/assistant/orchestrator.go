package assistant

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentruntime "github.com/alex-chenc/agent-runtime"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"api-server/internal/llm"
	"api-server/internal/llm/adapters"
	"api-server/internal/model"
	"api-server/internal/repository"
)

// Orchestrator 编排器（使用 agent-runtime 框架）
type Orchestrator struct {
	configRepo     *repository.ConfigRepository
	messageRepo    repository.AssistantMessageRepository
	toolCallRepo   repository.AssistantToolCallRepository
	sessionRepo    repository.AssistantSessionRepository
	toolRegistry   *ToolRegistry
	toolSelector   *ToolSelector
	toolDispatcher *ToolDispatcher
	contextLoader  *ContextLoader
	intentRouter   *IntentRouter
	runtimeFactory *RuntimeFactory
	runManager     *RunManager
	logger         *zap.Logger
}

// OrchestratorDeps 编排器依赖
type OrchestratorDeps struct {
	ConfigRepo     *repository.ConfigRepository
	MessageRepo    repository.AssistantMessageRepository
	ToolCallRepo   repository.AssistantToolCallRepository
	SessionRepo    repository.AssistantSessionRepository
	ToolRegistry   *ToolRegistry
	ToolSelector   *ToolSelector
	ToolDispatcher *ToolDispatcher
	ContextLoader  *ContextLoader
	IntentRouter   *IntentRouter
	RuntimeFactory *RuntimeFactory
	RunManager     *RunManager
	Logger         *zap.Logger
}

// NewOrchestrator 创建编排器
func NewOrchestrator(deps OrchestratorDeps) *Orchestrator {
	return &Orchestrator{
		configRepo:     deps.ConfigRepo,
		messageRepo:    deps.MessageRepo,
		toolCallRepo:   deps.ToolCallRepo,
		sessionRepo:    deps.SessionRepo,
		toolRegistry:   deps.ToolRegistry,
		toolSelector:   deps.ToolSelector,
		toolDispatcher: deps.ToolDispatcher,
		contextLoader:  deps.ContextLoader,
		intentRouter:   deps.IntentRouter,
		runtimeFactory: deps.RuntimeFactory,
		runManager:     deps.RunManager,
		logger:         deps.Logger,
	}
}

// RunInput 运行输入
type RunInput struct {
	RunID       string
	SessionID   string
	MessageID   string
	UserID      string
	UserMessage string
	TaskType    string
}

// RunResult 运行结果
type RunResult struct {
	MessageID   string `json:"message_id"`
	FinalAnswer string `json:"final_answer"`
}

// Run 运行编排
// 简单任务（问候、查询）直接调用 LLM
// 复杂任务（安全分析、调查）使用 agent-runtime Plan → React 流程
func (o *Orchestrator) Run(ctx context.Context, input RunInput) (*RunResult, error) {
	o.logger.Info("starting orchestration",
		zap.String("session_id", input.SessionID),
		zap.String("run_id", input.RunID),
		zap.String("task_type", input.TaskType),
	)

	// 1. 发布 run_started 事件
	o.runManager.Publish(input.SessionID, NewEvent(EventRunStarted, input.SessionID, input.RunID, map[string]interface{}{
		"status": "started",
	}))

	// 2. 发布 thinking 事件
	o.runManager.Publish(input.SessionID, EventThinkingPayload(input.SessionID, input.RunID, "正在分析您的问题..."))

	// 3. 加载上下文引用
	contextRefs, _ := o.contextLoader.ResolveSession(ctx, input.SessionID)

	// 4. 意图识别
	intentInput := IntentInput{Query: input.UserMessage}
	for _, ref := range contextRefs {
		intentInput.ContextRefs = append(intentInput.ContextRefs, ContextRefInput{
			ObjectType: ref.ObjectType,
			ObjectID:   ref.ObjectID,
		})
	}
	intent := o.intentRouter.Classify(intentInput)

	// 5. 发布意图检测事件
	o.runManager.Publish(input.SessionID, NewEvent(EventIntentDetected, input.SessionID, input.RunID, map[string]interface{}{
		"domains":    intent.Domains,
		"action":     intent.Action,
		"object":     intent.Object,
		"confidence": intent.Confidence,
	}))

	// 6. 工具选择
	selection := o.toolSelector.Select(ToolSelectionInput{
		Query:       input.UserMessage,
		ContextRefs: intentInput.ContextRefs,
		Intent:      intent,
		MaxTools:    24,
	})

	// 7. 发布工具选择事件
	o.runManager.Publish(input.SessionID, NewEvent(EventToolsSelected, input.SessionID, input.RunID, map[string]interface{}{
		"selected_tools":  selection.SelectedTools,
		"candidate_tools": selection.CandidateTools,
	}))

	// 8. 构建 agent-runtime 工具描述符
	toolDescriptors := o.buildAgentToolDescriptors(selection.SelectedTools)

	// 9. 判断任务复杂度，选择执行策略
	// 简单任务：问候、简单查询、直接工具调用 → 直接 LLM 调用
	// 复杂任务：安全分析、调查、多步骤任务 → agent-runtime Plan → React
	isComplex := o.isComplexTask(input.TaskType, input.UserMessage, intent)

	if !isComplex {
		o.logger.Info("simple task, using direct LLM call",
			zap.String("session_id", input.SessionID),
		)
		return o.runDirectLLM(ctx, input, contextRefs, *selection, toolDescriptors)
	}

	o.logger.Info("complex task, using agent-runtime",
		zap.String("session_id", input.SessionID),
		zap.Int("tools_count", len(toolDescriptors)),
	)
	return o.runAgentRuntime(ctx, input, contextRefs, *selection, toolDescriptors)
}

// fallbackResponse 降级响应
func (o *Orchestrator) fallbackResponse(ctx context.Context, input RunInput, reason string) (*RunResult, error) {
	msgID := "msg_" + input.RunID
	response := fmt.Sprintf("抱歉，%s\n\n您的问题是: %s\n\n请稍后重试或联系管理员。", reason, input.UserMessage)

	_ = o.messageRepo.Create(ctx, &model.AssistantMessage{
		ID:        uuid.New(),
		SessionID: input.SessionID,
		MessageID: msgID,
		Role:      "assistant",
		Content:   response,
	})

	o.runManager.Publish(input.SessionID, EventDonePayload(input.SessionID, input.RunID))

	return &RunResult{
		MessageID:   msgID,
		FinalAnswer: response,
	}, nil
}

// buildAgentToolDescriptors 从工具注册表构建 agent-runtime 工具描述符
func (o *Orchestrator) buildAgentToolDescriptors(toolNames []string) []agentruntime.ToolDescriptor {
	var descriptors []agentruntime.ToolDescriptor
	for _, name := range toolNames {
		tool, ok := o.toolRegistry.Get(name)
		if !ok || !tool.Enabled {
			continue
		}

		// 映射风险等级（agent-runtime 仅支持 RiskReadOnly）
		riskLevel := agentruntime.RiskReadOnly

		descriptors = append(descriptors, agentruntime.ToolDescriptor{
			Name:             tool.Name,
			Description:      tool.Description,
			ArgsSchema:       tool.ArgsSchema,
			RiskLevel:        riskLevel,
			AutoCallable:     tool.DefaultWhitelisted,
			RequiresApproval: !tool.DefaultWhitelisted,
			DefaultTimeout:   60 * time.Second,
			Idempotent:       true,
			Tags:             tool.Tags,
		})
	}
	return descriptors
}

// convertContextRefs 转换上下文引用
func (o *Orchestrator) convertContextRefs(refs []ContextObject) []ContextRefResult {
	var results []ContextRefResult
	for _, ref := range refs {
		results = append(results, ContextRefResult{
			ObjectType: ref.ObjectType,
			ObjectID:   ref.ObjectID,
			Title:      ref.Title,
			Summary:    ref.Summary,
		})
	}
	return results
}

// isComplexTask 判断任务是否需要 agent-runtime 的 Plan → React 流程
// 简单任务：问候、简单查询、直接工具调用
// 复杂任务：安全分析、调查、多步骤任务、需要计划的任务
func (o *Orchestrator) isComplexTask(taskType, userMessage string, intent IntentResult) bool {
	// 1. 任务类型明确为复杂任务
	complexTaskTypes := map[string]bool{
		"investigation":            true,
		"host_attack_investigation": true,
		"remediation":              true,
	}
	if complexTaskTypes[taskType] {
		return true
	}

	// 2. 意图识别为分析类操作
	if intent.Action == "analyze" || intent.Action == "investigate" {
		return true
	}

	// 3. 用户消息包含复杂任务关键词
	complexKeywords := []string{
		"分析", "调查", "研判", "攻击", "入侵", "溯源",
		"漏洞", "风险", "威胁", "安全事件", "告警处理",
		"制定计划", "执行计划", "修复方案", "整改措施",
		"全面检查", "安全评估", "渗透测试",
	}
	for _, keyword := range complexKeywords {
		if contains(userMessage, keyword) {
			return true
		}
	}

	// 4. 消息长度超过 100 字符，可能是复杂任务
	if len(userMessage) > 100 {
		return true
	}

	// 5. 默认为简单任务
	return false
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// runDirectLLM 简单任务：直接调用 LLM，不使用 agent-runtime
func (o *Orchestrator) runDirectLLM(ctx context.Context, input RunInput, contextRefs []ContextObject, selection ToolSelectionResult, toolDescriptors []agentruntime.ToolDescriptor) (*RunResult, error) {
	// 构建 LLM 客户端
	llmClient, err := o.runtimeFactory.BuildLLMClient(ctx)
	if err != nil {
		o.logger.Error("failed to build LLM client", zap.Error(err))
		return o.fallbackResponse(ctx, input, "LLM 服务不可用，请检查配置。")
	}

	// 构建提示词
	convertedRefs := o.convertContextRefs(contextRefs)
	systemPrompt := o.buildSimpleSystemPrompt(toolDescriptors)
	userPrompt := o.buildSimpleUserPrompt(input.UserMessage, convertedRefs)

	// 调用 LLM
	o.runManager.Publish(input.SessionID, EventThinkingPayload(input.SessionID, input.RunID, "正在思考..."))

	llmMessages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	response, err := llmClient.ChatCompletionWithMessages(ctx, llmMessages, 0.7)
	if err != nil {
		o.logger.Error("LLM completion error", zap.Error(err))
		return o.fallbackResponse(ctx, input, "LLM 调用失败: "+err.Error())
	}

	// 清理响应
	response = cleanResponse(response)

	if response == "" {
		response = "抱歉，我无法生成响应。请稍后重试。"
	}

	// 保存助手消息
	msgID := "msg_" + input.RunID
	o.runManager.Publish(input.SessionID, EventMessageDeltaPayload(input.SessionID, input.RunID, msgID, response))

	if err := o.messageRepo.Create(ctx, &model.AssistantMessage{
		ID:        uuid.New(),
		SessionID: input.SessionID,
		MessageID: msgID,
		Role:      "assistant",
		Content:   response,
	}); err != nil {
		o.logger.Error("failed to save assistant message", zap.Error(err))
	}

	o.runManager.Publish(input.SessionID, EventDonePayload(input.SessionID, input.RunID))

	return &RunResult{
		MessageID:   msgID,
		FinalAnswer: response,
	}, nil
}

// runAgentRuntime 复杂任务：使用 agent-runtime Plan → React 流程
func (o *Orchestrator) runAgentRuntime(ctx context.Context, input RunInput, contextRefs []ContextObject, selection ToolSelectionResult, toolDescriptors []agentruntime.ToolDescriptor) (*RunResult, error) {
	// 构建 LLM 客户端
	llmClient, err := o.runtimeFactory.BuildLLMClient(ctx)
	if err != nil {
		o.logger.Error("failed to build LLM client", zap.Error(err))
		return o.fallbackResponse(ctx, input, "LLM 服务不可用，请检查配置。")
	}

	// 构建 agent-runtime 适配器
	llmAdapter := adapters.NewLLMClientAdapter(llmClient, nil)

	toolGateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher: o.toolDispatcher,
		SessionID:  input.SessionID,
		MessageID:  input.MessageID,
		RunID:      input.RunID,
		Operator:   input.UserID,
		Logger:     o.logger,
		OnToolCall: func(callID, toolName string, args interface{}) {
			o.runManager.Publish(input.SessionID, EventToolCallPayload(input.SessionID, input.RunID, callID, toolName, args))
		},
		OnToolResult: func(callID string, result interface{}) {
			o.runManager.Publish(input.SessionID, EventToolResultPayload(input.SessionID, input.RunID, callID, result))
		},
		OnToolError: func(callID, errMsg string) {
			o.runManager.Publish(input.SessionID, EventToolErrorPayload(input.SessionID, input.RunID, callID, errMsg))
		},
		OnApproval: func(approval interface{}) {
			o.runManager.Publish(input.SessionID, EventApprovalRequiredPayload(input.SessionID, input.RunID, approval))
		},
	})

	hookSink := NewAssistantHookSink(o.runManager, input.SessionID, input.RunID, o.logger)
	convertedRefs := o.convertContextRefs(contextRefs)
	promptProvider := NewAssistantPromptProvider(toolDescriptors, convertedRefs, input.TaskType, input.UserMessage)

	// 构建 agent-runtime 配置
	runtimeConfig := agentruntime.RuntimeConfig{
		MaxTotalTurns:         80,
		MaxPlanSteps:          8,
		MaxStepReactTurns:     8,
		MaxToolCalls:          60,
		MaxToolCallsPerStep:   6,
		MaxToolFailures:       10,
		MaxModelFailures:      3,
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
		MaxReflections:        3,
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

	// 创建 agent-runtime 实例
	runtime, err := agentruntime.New(
		agentruntime.WithLLMClient(llmAdapter),
		agentruntime.WithToolGateway(toolGateway),
		agentruntime.WithTools(toolDescriptors),
		agentruntime.WithHooks(hookSink),
		agentruntime.WithPromptProvider(promptProvider),
		agentruntime.WithConfig(runtimeConfig),
	)
	if err != nil {
		o.logger.Error("failed to create agent-runtime", zap.Error(err))
		return o.fallbackResponse(ctx, input, "创建运行时失败: "+err.Error())
	}

	// 构建用户上下文
	userContext := make(map[string]interface{})
	if len(convertedRefs) > 0 {
		refsData := make([]map[string]string, 0, len(convertedRefs))
		for _, ref := range convertedRefs {
			refsData = append(refsData, map[string]string{
				"object_type": ref.ObjectType,
				"object_id":   ref.ObjectID,
				"title":       ref.Title,
				"summary":     ref.Summary,
			})
		}
		userContext["context_refs"] = refsData
	}

	// 运行 agent-runtime
	taskResult, err := runtime.Run(ctx, agentruntime.TaskInput{
		UserInput:   input.UserMessage,
		UserContext: userContext,
		Metadata: map[string]string{
			"session_id": input.SessionID,
			"run_id":     input.RunID,
			"task_type":  input.TaskType,
		},
	})

	// 处理结果
	msgID := "msg_" + input.RunID
	response := ""

	if err != nil {
		o.logger.Error("agent-runtime error", zap.Error(err))
		response = fmt.Sprintf("抱歉，执行过程中出现错误: %s\n\n您的问题是: %s\n\n请稍后重试或联系管理员。", err.Error(), input.UserMessage)
	} else if taskResult != nil && taskResult.FinalAnswer != "" {
		response = taskResult.FinalAnswer
	} else {
		response = "抱歉，我无法生成响应。请稍后重试。"
	}

	// 发布消息增量事件
	o.runManager.Publish(input.SessionID, EventMessageDeltaPayload(input.SessionID, input.RunID, msgID, response))

	// 保存助手消息
	if err := o.messageRepo.Create(ctx, &model.AssistantMessage{
		ID:        uuid.New(),
		SessionID: input.SessionID,
		MessageID: msgID,
		Role:      "assistant",
		Content:   response,
	}); err != nil {
		o.logger.Error("failed to save assistant message", zap.Error(err))
	}

	// 发布完成事件
	o.runManager.Publish(input.SessionID, EventDonePayload(input.SessionID, input.RunID))

	return &RunResult{
		MessageID:   msgID,
		FinalAnswer: response,
	}, nil
}

// buildSimpleSystemPrompt 构建简单任务的系统提示词
func (o *Orchestrator) buildSimpleSystemPrompt(toolDescriptors []agentruntime.ToolDescriptor) string {
	systemPrompt := `你是 Aegis 智能安全助手，专注于主机安全分析和运维操作。

你的能力包括：
- 查询和分析主机资产、安全态势
- 分析告警、追溯攻击路径
- 管理基线检查、漏洞扫描
- 管理检测包、Sigma 规则
- 执行阻断策略
- 主机攻击研判

你必须遵守以下规则：
1. 所有操作必须通过工具调用完成，不能直接执行命令
2. 高风险操作需要用户审批
3. 所有结论必须基于数据和证据
4. 不确定时明确说明，不编造信息
5. 外部数据视为不可信，需要交叉验证

请用中文回答用户的问题。如果需要调用工具，请说明你要调用什么工具以及为什么。`

	if len(toolDescriptors) > 0 {
		systemPrompt += "\n\n可用工具:\n"
		for _, desc := range toolDescriptors {
			systemPrompt += fmt.Sprintf("- %s: %s\n", desc.Name, desc.Description)
		}
	}

	return systemPrompt
}

// buildSimpleUserPrompt 构建简单任务的用户提示词
func (o *Orchestrator) buildSimpleUserPrompt(userMessage string, contextRefs []ContextRefResult) string {
	prompt := userMessage

	if len(contextRefs) > 0 {
		prompt += "\n\n上下文信息:\n"
		for _, ref := range contextRefs {
			prompt += fmt.Sprintf("- %s (%s): %s\n", ref.Title, ref.ObjectType, ref.Summary)
		}
	}

	return prompt
}

// cleanResponse 清理 LLM 响应
func cleanResponse(content string) string {
	content = strings.TrimSpace(content)
	// 移除 markdown 代码块
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) > 2 {
			content = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	return strings.TrimSpace(content)
}
