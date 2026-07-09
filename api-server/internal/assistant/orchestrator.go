package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	agentruntime "github.com/alex-chenc/agent-runtime"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"api-server/internal/model"
	"api-server/internal/repository"
)

// Orchestrator 编排器（使用 agent-runtime 框架）
type Orchestrator struct {
	configRepo         *repository.ConfigRepository
	messageRepo        repository.AssistantMessageRepository
	toolCallRepo       repository.AssistantToolCallRepository
	sessionRepo        repository.AssistantSessionRepository
	toolRegistry       *ToolRegistry
	intentDecomposer   *IntentDecomposer
	toolDecisionEngine *ToolDecisionEngine
	toolDispatcher     *ToolDispatcher
	approvalGate       *ApprovalGate
	contextLoader      *ContextLoader
	intentRouter       *IntentRouter
	runtimeFactory     *RuntimeFactory
	runManager         *RunManager
	clarificationGate  *ClarificationGate
	decisionRecorder   *ToolDecisionRecorder
	logger             *zap.Logger
}

// OrchestratorDeps 编排器依赖
type OrchestratorDeps struct {
	ConfigRepo         *repository.ConfigRepository
	MessageRepo        repository.AssistantMessageRepository
	ToolCallRepo       repository.AssistantToolCallRepository
	SessionRepo        repository.AssistantSessionRepository
	ToolRegistry       *ToolRegistry
	IntentDecomposer   *IntentDecomposer
	ToolDecisionEngine *ToolDecisionEngine
	ToolDispatcher     *ToolDispatcher
	ApprovalGate       *ApprovalGate
	ContextLoader      *ContextLoader
	IntentRouter       *IntentRouter
	RuntimeFactory     *RuntimeFactory
	RunManager         *RunManager
	ClarificationGate  *ClarificationGate
	DecisionRecorder   *ToolDecisionRecorder
	Logger             *zap.Logger
}

// NewOrchestrator 创建编排器
func NewOrchestrator(deps OrchestratorDeps) *Orchestrator {
	logger := deps.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	clarificationGate := deps.ClarificationGate
	if clarificationGate == nil && deps.ToolDecisionEngine != nil {
		clarificationGate = NewClarificationGate(deps.ToolDecisionEngine.config, logger)
	}
	decisionRecorder := deps.DecisionRecorder
	if decisionRecorder == nil {
		decisionRecorder = NewToolDecisionRecorder(deps.SessionRepo, logger)
	}
	return &Orchestrator{
		configRepo:         deps.ConfigRepo,
		messageRepo:        deps.MessageRepo,
		toolCallRepo:       deps.ToolCallRepo,
		sessionRepo:        deps.SessionRepo,
		toolRegistry:       deps.ToolRegistry,
		intentDecomposer:   deps.IntentDecomposer,
		toolDecisionEngine: deps.ToolDecisionEngine,
		toolDispatcher:     deps.ToolDispatcher,
		approvalGate:       deps.ApprovalGate,
		contextLoader:      deps.ContextLoader,
		intentRouter:       deps.IntentRouter,
		runtimeFactory:     deps.RuntimeFactory,
		runManager:         deps.RunManager,
		clarificationGate:  clarificationGate,
		decisionRecorder:   decisionRecorder,
		logger:             logger,
	}
}

// RunInput 运行输入（对齐设计文档 6 节）
type RunInput struct {
	RunID       string
	SessionID   string
	MessageID   string
	UserID      string
	UserMessage string
	TaskType    string
	ContextRefs []model.AssistantContextRef
}

// RunResult 运行结果
type RunResult struct {
	MessageID   string                   `json:"message_id"`
	FinalAnswer string                   `json:"final_answer"`
	RunStatus   string                   `json:"run_status"`
	GoalOutcome agentruntime.GoalOutcome `json:"goal_outcome"`
}

// Run 运行编排。所有请求统一进入 agent-runtime；Runtime 根据任务复杂度决定直接回答或 Plan → React。
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

	// 3. 加载上下文引用（优先使用 RunInput 携带的，避免重复查询）
	var contextRefs []ContextObject
	if len(input.ContextRefs) > 0 {
		for _, ref := range input.ContextRefs {
			obj := ContextObject{
				ObjectType: ref.ObjectType,
				ObjectID:   ref.ObjectID,
				Title:      ref.Title,
				Summary:    ref.Summary,
				RoutePath:  ref.RoutePath,
				Data:       unmarshalJSON(ref.Snapshot),
			}
			// 尝试加载完整数据
			if o.contextLoader != nil {
				if resolved, err := o.contextLoader.Resolve(ctx, ref.ObjectType, ref.ObjectID); err == nil && resolved != nil {
					if resolved.Data != nil {
						obj.Data = resolved.Data
					}
					if obj.Title == "" {
						obj.Title = resolved.Title
					}
					if obj.Summary == "" {
						obj.Summary = resolved.Summary
					}
				}
			}
			contextRefs = append(contextRefs, obj)
		}
	} else if o.contextLoader != nil {
		contextRefs, _ = o.contextLoader.ResolveSession(ctx, input.SessionID)
	}

	// 4. 意图识别
	intentInput := IntentInput{Query: input.UserMessage}
	for _, ref := range contextRefs {
		intentInput.ContextRefs = append(intentInput.ContextRefs, ContextRefInput{
			ObjectType: ref.ObjectType,
			ObjectID:   ref.ObjectID,
		})
	}
	intent, err := o.intentRouter.Classify(ctx, intentInput)
	if err != nil {
		return nil, fmt.Errorf("classify assistant intent: %w", err)
	}

	// 5. 发布意图检测事件
	o.runManager.Publish(input.SessionID, NewEvent(EventIntentDetected, input.SessionID, input.RunID, map[string]interface{}{
		"domains":    intent.Domains,
		"action":     intent.Action,
		"object":     intent.Object,
		"confidence": intent.Confidence,
	}))
	if o.intentDecomposer == nil {
		return nil, fmt.Errorf("assistant intent decomposer unavailable")
	}
	capabilityCatalog := o.buildCapabilityCatalog()
	intentBreakdown, err := o.intentDecomposer.Decompose(ctx, IntentDecomposeInput{
		Query:                  input.UserMessage,
		Intent:                 intent,
		ContextRefs:            intentInput.ContextRefs,
		AvailableCapabilities:  capabilityCatalog,
		EnableLLMDecomposition: true,
	})
	if err != nil {
		o.logger.Error("assistant llm intent decomposition failed",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("decompose assistant intent with llm: %w", err)
	}

	// 6. 工具选择：LLM 只从实时英文 capability 目录中选择能力，后端通过
	// exact mapping 解析工具并执行安全硬门。不存在工具评分、领域召回或预选绕过。
	selection := &ToolSelectionResult{
		Query:  input.UserMessage,
		Intent: intent,
	}
	selectionMode := "capability_mapping"
	useAIAnalysisFlow := o.isComplexTask(input.TaskType, input.UserMessage, intent, selection.SelectedTools)

	var authorization *ToolExecutionPlan
	if o.toolDecisionEngine != nil && o.toolDecisionEngine.config.Enabled {
		plan, err := o.toolDecisionEngine.Decide(ctx, ToolDecisionInput{
			Query:             input.UserMessage,
			Intent:            intent,
			Breakdown:         intentBreakdown,
			ContextRefs:       intentInput.ContextRefs,
			UseAIAnalysisFlow: useAIAnalysisFlow,
		})
		if err != nil {
			o.logger.Error("assistant tool decision failed",
				zap.String("session_id", input.SessionID),
				zap.String("run_id", input.RunID),
				zap.Error(err),
			)
			return nil, fmt.Errorf("decide assistant tools: %w", err)
		} else if plan != nil {
			authorization = plan

			// 使用 ToolDecisionRecorder 持久化裁决记录
			if o.decisionRecorder != nil {
				_ = o.decisionRecorder.Record(context.Background(), input.SessionID, plan)
			}

			// 使用 ClarificationGate 评估是否需要追问
			clarification := o.clarificationGate.Evaluate(intentBreakdown, plan.ToolNames(), plan.DecisionRecords)
			if plan.NeedClarification || clarification.Required {
				o.mergeSessionMetadata(context.Background(), input.SessionID, map[string]interface{}{
					"intent_breakdown":   intentBreakdown,
					"tool_authorization": authorization,
					"current_run_status": "needs_input",
					"goal_outcome":       agentruntime.GoalNeedsInput,
				})
				question := plan.ClarifyingQuestion
				if question == "" {
					question = clarification.Question
				}
				o.runManager.Publish(input.SessionID, NewEvent(EventToolsSelected, input.SessionID, input.RunID, map[string]interface{}{
					"selected_tools":        []string{},
					"candidate_tools":       selection.CandidateTools,
					"runtime_profile":       runtimeProfileName(false),
					"max_total_turns":       maxRuntimeTurns(false),
					"selection_mode":        selectionMode,
					"decision_trace_id":     plan.DecisionTraceID,
					"need_clarification":    true,
					"clarifying_question":   question,
					"rejected_tool_records": plan.RejectedToolRecords,
				}))
				plan.ClarifyingQuestion = question
				return o.clarificationResponse(ctx, input, plan)
			}
			selection.SelectedTools = plan.ToolNames()
			selection.CandidateTools = dedupeStrings(append(selection.CandidateTools, selection.SelectedTools...))
			selectionMode = selectionMode + "+hard_gates"
			useAIAnalysisFlow = o.isComplexTask(input.TaskType, input.UserMessage, intent, selection.SelectedTools)
			o.mergeSessionMetadata(context.Background(), input.SessionID, map[string]interface{}{
				"intent_breakdown":   intentBreakdown,
				"tool_authorization": authorization,
			})
		}
	}

	// 7. 发布工具选择事件
	toolSelectionPayload := map[string]interface{}{
		"selected_tools":  selection.SelectedTools,
		"candidate_tools": selection.CandidateTools,
		"runtime_profile": runtimeProfileName(useAIAnalysisFlow),
		"max_total_turns": maxRuntimeTurns(useAIAnalysisFlow),
		"selection_mode":  selectionMode,
	}
	if authorization != nil {
		toolSelectionPayload["decision_trace_id"] = authorization.DecisionTraceID
		toolSelectionPayload["tool_authorization"] = authorization
	}
	if intentBreakdown != nil {
		toolSelectionPayload["intent_breakdown"] = intentBreakdown
	}
	o.runManager.Publish(input.SessionID, NewEvent(EventToolsSelected, input.SessionID, input.RunID, toolSelectionPayload))

	// 8. 构建 agent-runtime 工具描述符
	toolDescriptors := o.buildAgentToolDescriptors(selection.SelectedTools)

	// 9. 统一走 agent-runtime，由 LLM 自行判断任务复杂度和回复方式
	// - 问候/闲聊：LLM 直接回复自然语言
	// - 简单任务（<3步）：跳过计划，直接 ReAct 执行
	// - 复杂任务（>=3步）：生成完整计划，按步骤执行
	o.logger.Info("assistant request routed to generic agent runtime",
		zap.String("session_id", input.SessionID),
		zap.String("run_id", input.RunID),
		zap.String("runtime_profile", runtimeProfileName(useAIAnalysisFlow)),
		zap.String("planning_mode", "agent_runtime_dynamic"),
		zap.String("selection_mode", selectionMode),
		zap.Strings("selected_tools", selection.SelectedTools),
		zap.Int("tools_count", len(toolDescriptors)),
	)
	return o.runAgentRuntime(ctx, input, contextRefs, *selection, toolDescriptors, useAIAnalysisFlow, authorization)
}

func (o *Orchestrator) clarificationResponse(ctx context.Context, input RunInput, plan *ToolExecutionPlan) (*RunResult, error) {
	_ = ctx
	msgID := "msg_" + input.RunID
	response := strings.TrimSpace(plan.ClarifyingQuestion)
	if response == "" {
		response = "请补充要操作的对象和范围后再执行。"
	}

	o.runManager.Publish(input.SessionID, EventMessageDeltaPayload(input.SessionID, input.RunID, msgID, response))
	o.persistSessionRuntimeEvents(
		context.Background(),
		input.SessionID,
		msgID,
		compactRuntimeDisplayEvents(o.extractRunHistory(input.SessionID), input.RunID, msgID),
	)

	saveCtx := context.Background()
	if err := o.messageRepo.Create(saveCtx, &model.AssistantMessage{
		ID:        uuid.New(),
		SessionID: input.SessionID,
		MessageID: msgID,
		Role:      "assistant",
		Content:   response,
		Thinking:  o.extractThinkingFromHistory(input.SessionID),
		Plan:      mustMarshalJSON(plan),
	}); err != nil {
		o.logger.Error("failed to save assistant clarification message",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.Error(err),
		)
	}

	return &RunResult{
		MessageID:   msgID,
		FinalAnswer: response,
		RunStatus:   "needs_input",
		GoalOutcome: agentruntime.GoalNeedsInput,
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

		// 使用 tool_catalog.go 中的 toRuntimeRisk 映射风险等级
		riskLevel := toRuntimeRisk(tool.Risk)

		descriptors = append(descriptors, agentruntime.ToolDescriptor{
			Name:             tool.Name,
			Description:      modelFacingToolDescription(tool),
			ArgsSchema:       normalizeRuntimeArgsSchema(tool.ArgsSchema),
			ResultSchema:     normalizeRuntimeArgsSchema(tool.ResultSchema),
			RiskLevel:        riskLevel,
			AutoCallable:     tool.DefaultWhitelisted,
			RequiresApproval: !tool.DefaultWhitelisted,
			DefaultTimeout:   defaultTimeout(tool.DefaultTimeout),
			Idempotent:       tool.Idempotent,
			Tags:             tool.Tags,
		})
	}
	return descriptors
}

func (o *Orchestrator) buildCapabilityCatalog() []CapabilityCatalogItem {
	if o == nil || o.toolRegistry == nil {
		return nil
	}
	tools := o.toolRegistry.List()
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Capability < tools[j].Capability
	})
	catalog := make([]CapabilityCatalogItem, 0, len(tools))
	for _, tool := range tools {
		if tool == nil || !tool.Enabled || isResidentTool(tool.Name) {
			continue
		}
		contract := BuildToolUseContract(tool)
		catalog = append(catalog, CapabilityCatalogItem{
			Capability:    contract.Capability,
			Domain:        string(tool.Domain),
			Operation:     string(tool.Operation),
			ObjectTypes:   append([]string{}, tool.ObjectTypes...),
			Risk:          string(tool.Risk),
			ExecutionMode: tool.ExecutionContract.Mode,
			Description:   modelFacingToolDescription(tool),
		})
	}
	return catalog
}

func runtimeProfileName(useAIAnalysisFlow bool) string {
	if useAIAnalysisFlow {
		return "ai_analysis"
	}
	return "assistant"
}

func maxRuntimeTurns(useAIAnalysisFlow bool) int {
	if useAIAnalysisFlow {
		return 500
	}
	return 80
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
			Data:       normalizeContextData(ref.Data),
		})
	}
	return results
}

func normalizeContextData(data interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}
	if typed, ok := data.(map[string]interface{}); ok {
		return typed
	}
	return map[string]interface{}{"value": data}
}

// isComplexTask 判断任务是否需要 agent-runtime 的 Plan → React 流程
// 简单任务：问候、闲聊、不需要工具调用的通用问题
// 复杂任务：需要工具调用的数据查询、安全分析、调查、多步骤任务
func (o *Orchestrator) isComplexTask(taskType, userMessage string, intent IntentResult, selectedTools []string) bool {
	_ = taskType
	_ = userMessage
	_ = intent
	// Profile selection only allocates a larger execution budget. Task
	// complexity and plan shape are decided by agent-runtime's LLM router.
	for _, name := range selectedTools {
		if !isResidentTool(name) {
			return true
		}
	}
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

// runAgentRuntime 统一执行入口：使用 agent-runtime Plan → React 流程
// 使用 RuntimeFactory.Build() 集中创建 runtime（对齐设计文档 4.3 节）
func (o *Orchestrator) runAgentRuntime(ctx context.Context, input RunInput, contextRefs []ContextObject, selection ToolSelectionResult, toolDescriptors []agentruntime.ToolDescriptor, useAIAnalysisFlow bool, authorization *ToolExecutionPlan) (*RunResult, error) {
	msgID := "msg_" + input.RunID
	maxIterations := 80
	if useAIAnalysisFlow {
		maxIterations = 500
	}
	o.mergeSessionMetadata(ctx, input.SessionID, map[string]interface{}{
		"runtime_profile":    runtimeProfileName(useAIAnalysisFlow),
		"planning_mode":      "agent_runtime_dynamic",
		"max_total_turns":    maxIterations,
		"current_run_id":     input.RunID,
		"current_message_id": msgID,
		"current_run_status": "running",
		"run_started_at":     time.Now().UTC().Format(time.RFC3339),
	})

	// 使用 RuntimeFactory.Build() 创建完整的 agent-runtime 实例
	buildResult, err := o.runtimeFactory.Build(ctx, RuntimeBuildRequest{
		SessionID:         input.SessionID,
		RunID:             input.RunID,
		MessageID:         msgID,
		Operator:          input.UserID,
		UserInput:         input.UserMessage,
		TaskType:          input.TaskType,
		ContextRefs:       o.convertContextRefs(contextRefs),
		SelectedTools:     selection.SelectedTools,
		ToolDescriptors:   toolDescriptors,
		MaxIterations:     maxIterations,
		ExecutionPlan:     nil,
		UseAIAnalysisFlow: useAIAnalysisFlow,
	})
	if err != nil {
		o.logger.Error("failed to build agent-runtime", zap.Error(err))
		return nil, fmt.Errorf("build agent runtime: %w", err)
	}

	// 运行 agent-runtime
	taskResult, err := buildResult.Runtime.Run(ctx, agentruntime.TaskInput{
		UserInput:   input.UserMessage,
		UserContext: buildResult.UserContext,
		InitialPlan: runtimeInitialPlanForAssistant(authorization),
		Metadata: map[string]string{
			"session_id": input.SessionID,
			"run_id":     input.RunID,
			"task_type":  input.TaskType,
		},
	})
	if run, ok := o.runManager.Get(input.SessionID); ok {
		if decision := run.RejectedApproval(); decision != nil {
			o.mergeSessionMetadata(context.Background(), input.SessionID, map[string]interface{}{
				"current_run_status":     "approval_rejected",
				"last_run_rejected_at":   time.Now().UTC().Format(time.RFC3339),
				"rejected_approval_id":   decision.ApprovalID,
				"rejected_approval_note": decision.Comment,
			})
			return nil, context.Canceled
		}
	}
	if err != nil && ctx.Err() != nil {
		o.mergeSessionMetadata(context.Background(), input.SessionID, map[string]interface{}{
			"current_run_status":    "cancelled",
			"last_run_cancelled_at": time.Now().UTC().Format(time.RFC3339),
		})
		return nil, ctx.Err()
	}
	if err != nil {
		o.logger.Error("agent-runtime error", zap.Error(err))
		return nil, fmt.Errorf("agent runtime failed: %w", err)
	}
	if taskResult == nil {
		return nil, fmt.Errorf("agent runtime returned no result")
	}
	saveCtx := context.Background()
	o.persistRuntimeToolCallRecords(saveCtx, input.SessionID, msgID, taskResult)
	if err := validateRuntimeFinalAnswer(taskResult.FinalAnswer); err != nil {
		return nil, err
	}

	evidence := buildRuntimeEvidenceLedger(taskResult)
	response := taskResult.FinalAnswer
	evidenceConflicts := validateRuntimeEvidenceConsistency(response, evidence)
	if len(evidenceConflicts) > 0 {
		o.logger.Warn("assistant final answer contradicted runtime tool evidence",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.String("message_id", msgID),
			zap.Strings("conflict_codes", evidenceConflicts),
		)
		response = buildEvidenceGroundedFallback(evidence)
	}
	goalOutcome := taskResult.GoalOutcome
	if goalOutcome == "" {
		goalOutcome = agentruntime.GoalFailed
	}
	if len(evidenceConflicts) > 0 && goalOutcome == agentruntime.GoalSucceeded {
		goalOutcome = agentruntime.GoalPartiallySucceeded
	}
	contextBudget := effectiveRuntimeContextBudget(taskResult)
	metadata := map[string]interface{}{
		"runtime_profile":         runtimeProfileName(useAIAnalysisFlow),
		"max_total_turns":         maxIterations,
		"current_run_id":          input.RunID,
		"current_message_id":      msgID,
		"runtime_status":          taskResult.Status,
		"goal_outcome":            goalOutcome,
		"current_run_status":      assistantRunStatus(goalOutcome),
		"last_run_completed_at":   time.Now().UTC().Format(time.RFC3339),
		"total_prompt_tokens":     taskResult.Metrics.TotalPromptTokens,
		"total_completion_tokens": taskResult.Metrics.TotalCompletionTokens,
		"total_tokens":            taskResult.Metrics.TotalPromptTokens + taskResult.Metrics.TotalCompletionTokens,
		"compression_count":       len(taskResult.CompressionRecords),
		"context_budget":          contextBudget,
		"compression_records":     taskResult.CompressionRecords,
		"runtime_evidence":        evidence,
		"evidence_conflicts":      evidenceConflicts,
	}
	o.mergeSessionMetadata(context.Background(), input.SessionID, metadata)
	if contextBudget != nil {
		o.runManager.Publish(input.SessionID, withMessageID(NewEvent(EventContextBudget, input.SessionID, input.RunID, contextBudget), msgID))
	}

	// 发布消息增量事件
	o.runManager.Publish(input.SessionID, EventMessageDeltaPayload(input.SessionID, input.RunID, msgID, response))

	// 从事件历史中提取 thinking 和 plan 数据
	thinkingContent := o.extractThinkingFromHistory(input.SessionID)
	planData := o.extractPlanFromHistory(input.SessionID)
	o.persistSessionRuntimeEvents(
		context.Background(),
		input.SessionID,
		msgID,
		compactRuntimeDisplayEvents(o.extractRunHistory(input.SessionID), input.RunID, msgID),
	)

	// 使用 context.Background() 保存消息，避免上下文取消导致保存失败
	if err := o.messageRepo.Create(saveCtx, &model.AssistantMessage{
		ID:        uuid.New(),
		SessionID: input.SessionID,
		MessageID: msgID,
		Role:      "assistant",
		Content:   response,
		Thinking:  thinkingContent,
		Plan:      planData,
		ToolCalls: o.toolCallsForMessage(saveCtx, input.SessionID, msgID),
	}); err != nil {
		o.logger.Error("failed to save assistant message", zap.Error(err))
	}

	return &RunResult{
		MessageID:   msgID,
		FinalAnswer: response,
		RunStatus:   assistantRunStatus(goalOutcome),
		GoalOutcome: goalOutcome,
	}, nil
}

func assistantRunStatus(outcome agentruntime.GoalOutcome) string {
	switch outcome {
	case agentruntime.GoalSucceeded:
		return "completed"
	case agentruntime.GoalPartiallySucceeded:
		return "completed_with_failures"
	case agentruntime.GoalNeedsInput:
		return "needs_input"
	default:
		return "failed"
	}
}

func effectiveRuntimeContextBudget(result *agentruntime.TaskResult) *agentruntime.ContextBudgetSnapshot {
	if result == nil {
		return nil
	}

	budget := result.ContextBudget
	if budget == nil {
		budget = &agentruntime.ContextBudgetSnapshot{
			MaxContextTokens:     256000,
			ReservedOutputTokens: 8192,
		}
	}

	effective := *budget
	maxPromptTokens := 0
	for _, call := range result.ModelCalls {
		if call.PromptTokens > maxPromptTokens {
			maxPromptTokens = call.PromptTokens
		}
	}
	if maxPromptTokens == 0 {
		maxPromptTokens = result.Metrics.TotalPromptTokens
	}

	if maxPromptTokens > effective.EstimatedPromptTokens {
		effective.EstimatedPromptTokens = maxPromptTokens
	}
	if maxPromptTokens > effective.PromptTokensObserved {
		effective.PromptTokensObserved = maxPromptTokens
	}
	if result.Metrics.TotalCompletionTokens > effective.CompletionTokens {
		effective.CompletionTokens = result.Metrics.TotalCompletionTokens
	}
	totalTokens := result.Metrics.TotalPromptTokens + result.Metrics.TotalCompletionTokens
	if totalTokens > effective.TotalTokens {
		effective.TotalTokens = totalTokens
	}
	if len(result.CompressionRecords) > effective.CompressionCount {
		effective.CompressionCount = len(result.CompressionRecords)
	}
	if effective.MaxContextTokens <= 0 {
		effective.MaxContextTokens = 256000
	}
	if effective.ReservedOutputTokens <= 0 {
		effective.ReservedOutputTokens = 8192
	}
	effective.ContextRatio = float64(effective.EstimatedPromptTokens+effective.ReservedOutputTokens) / float64(effective.MaxContextTokens)
	return &effective
}

func validateRuntimeFinalAnswer(answer string) error {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return fmt.Errorf("agent runtime returned an empty final answer")
	}
	var control map[string]interface{}
	if err := json.Unmarshal([]byte(answer), &control); err != nil {
		return nil
	}
	action, _ := control["action"].(string)
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "tool_call", "additional_capability_request", "need_user_input", "clarification_required":
		return fmt.Errorf("agent runtime ended with unfinished control action %q", action)
	}
	return nil
}

func (o *Orchestrator) persistRuntimeToolCallRecords(
	ctx context.Context,
	sessionID string,
	messageID string,
	result *agentruntime.TaskResult,
) {
	if o.toolCallRepo == nil || result == nil {
		return
	}

	for _, runtimeCall := range result.ToolCalls {
		if strings.TrimSpace(runtimeCall.CallID) == "" || strings.TrimSpace(runtimeCall.ToolName) == "" {
			continue
		}

		existing, err := o.toolCallRepo.FindByCallID(ctx, runtimeCall.CallID)
		if err == nil && existing != nil {
			continue
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			o.logger.Warn("failed to check runtime tool call persistence",
				zap.String("session_id", sessionID),
				zap.String("message_id", messageID),
				zap.String("call_id", runtimeCall.CallID),
				zap.String("tool_name", runtimeCall.ToolName),
				zap.Error(err),
			)
			continue
		}

		domain := string(DomainSystem)
		riskLevel := string(runtimeCall.RiskLevel)
		if o.toolRegistry != nil {
			if spec, ok := o.toolRegistry.Get(runtimeCall.ToolName); ok {
				domain = string(spec.Domain)
				riskLevel = string(spec.Risk)
			}
		}
		if riskLevel == "" {
			riskLevel = string(ToolRiskReadonly)
		}

		status := model.ToolCallStatusFailed
		if runtimeCall.Status == agentruntime.ToolCallSuccess {
			status = model.ToolCallStatusSuccess
		}
		durationMs := runtimeCall.EndedAt.Sub(runtimeCall.StartedAt).Milliseconds()
		if runtimeCall.EndedAt.IsZero() || runtimeCall.StartedAt.IsZero() || durationMs < 0 {
			durationMs = 0
		}
		createdAt := runtimeCall.StartedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		argsSummary := strings.TrimSpace(runtimeCall.ArgsSummary)
		if runtimeCall.ValidationStage != "" {
			if argsSummary != "" {
				argsSummary += " "
			}
			argsSummary += "[validation_stage=" + runtimeCall.ValidationStage + "]"
		}

		call := &model.AssistantToolCall{
			ID:            uuid.New(),
			SessionID:     sessionID,
			MessageID:     messageID,
			CallID:        runtimeCall.CallID,
			ToolName:      runtimeCall.ToolName,
			Domain:        domain,
			RiskLevel:     riskLevel,
			Status:        status,
			Args:          mustMarshalJSON(map[string]interface{}{}),
			ArgsSummary:   argsSummary,
			ResultSummary: runtimeCall.ResultSummary,
			ErrorMessage:  runtimeCall.ErrorMessage,
			DurationMs:    durationMs,
			CreatedAt:     createdAt,
			UpdatedAt:     createdAt,
		}
		if runtimeCall.Outcome != nil {
			terminal := runtimeCall.Outcome.Terminal
			call.OperationStatus = string(runtimeCall.Outcome.OperationStatus)
			call.OperationTerminal = &terminal
			call.Outcome = mustMarshalJSON(runtimeCall.Outcome)
		}
		if err := o.toolCallRepo.Create(ctx, call); err != nil {
			o.logger.Warn("failed to persist runtime tool call record",
				zap.String("session_id", sessionID),
				zap.String("message_id", messageID),
				zap.String("call_id", runtimeCall.CallID),
				zap.String("tool_name", runtimeCall.ToolName),
				zap.String("validation_stage", runtimeCall.ValidationStage),
				zap.Error(err),
			)
		}
	}
}

func (o *Orchestrator) toolCallsForMessage(ctx context.Context, sessionID, messageID string) datatypes.JSON {
	if o.toolCallRepo == nil || sessionID == "" || messageID == "" {
		return nil
	}
	calls, _, err := o.toolCallRepo.ListBySession(ctx, sessionID, 1, 100)
	if err != nil {
		o.logger.Warn("failed to load assistant tool calls for message",
			zap.String("session_id", sessionID),
			zap.String("message_id", messageID),
			zap.Error(err),
		)
		return nil
	}
	filtered := make([]model.AssistantToolCall, 0, len(calls))
	for _, call := range calls {
		if call.MessageID == messageID {
			filtered = append(filtered, call)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
	})
	if len(filtered) == 0 {
		return nil
	}
	return mustMarshalJSON(filtered)
}

func (o *Orchestrator) mergeSessionMetadata(ctx context.Context, sessionID string, updates map[string]interface{}) {
	if o.sessionRepo == nil || sessionID == "" || len(updates) == 0 {
		return
	}

	session, err := o.sessionRepo.FindBySessionID(ctx, sessionID)
	if err != nil {
		o.logger.Warn("failed to load assistant session metadata",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		return
	}

	metadata := unmarshalJSON(session.Metadata)
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	for key, value := range updates {
		if value == nil {
			continue
		}
		metadata[key] = value
	}
	session.Metadata = mustMarshalJSON(metadata)
	if err := o.sessionRepo.Update(ctx, session); err != nil {
		o.logger.Warn("failed to update assistant session metadata",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}
}

func (o *Orchestrator) persistSessionRuntimeEvents(ctx context.Context, sessionID, messageID string, events []assistantRuntimeDisplayEvent) {
	if o.sessionRepo == nil || sessionID == "" || messageID == "" || len(events) == 0 {
		return
	}

	session, err := o.sessionRepo.FindBySessionID(ctx, sessionID)
	if err != nil {
		o.logger.Warn("failed to load assistant session for runtime event persistence",
			zap.String("session_id", sessionID),
			zap.String("message_id", messageID),
			zap.Error(err),
		)
		return
	}

	metadata := unmarshalJSON(session.Metadata)
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	byMessage := make(map[string]interface{})
	if existing, ok := metadata[assistantRuntimeEventsMetadataKey].(map[string]interface{}); ok {
		for key, value := range existing {
			byMessage[key] = value
		}
	}
	byMessage[messageID] = events
	metadata[assistantRuntimeEventsMetadataKey] = byMessage

	session.Metadata = mustMarshalJSON(metadata)
	if err := o.sessionRepo.Update(ctx, session); err != nil {
		o.logger.Warn("failed to persist assistant runtime events",
			zap.String("session_id", sessionID),
			zap.String("message_id", messageID),
			zap.Error(err),
		)
	}
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

// extractThinkingFromHistory 从事件历史中提取 thinking 内容
// 返回 JSON 数组，每个元素是一个思考步骤
func (o *Orchestrator) extractThinkingFromHistory(sessionID string) datatypes.JSON {
	run, ok := o.runManager.Get(sessionID)
	if !ok {
		return nil
	}

	var thinkingSteps []string
	for _, event := range run.History() {
		if event.Type == EventThinking {
			if payload, ok := event.Payload.(map[string]interface{}); ok {
				if content, ok := payload["content"].(string); ok && content != "" {
					thinkingSteps = append(thinkingSteps, content)
				}
			}
		}
	}

	if len(thinkingSteps) == 0 {
		return nil
	}

	jsonBytes, err := json.Marshal(thinkingSteps)
	if err != nil {
		return nil
	}
	return datatypes.JSON(jsonBytes)
}

func (o *Orchestrator) extractRunHistory(sessionID string) []AssistantEvent {
	run, ok := o.runManager.Get(sessionID)
	if !ok {
		return nil
	}
	return run.History()
}

// extractPlanFromHistory 从事件历史中提取 plan 数据
// 返回最后一个 plan 事件的 JSON 数据
func (o *Orchestrator) extractPlanFromHistory(sessionID string) datatypes.JSON {
	run, ok := o.runManager.Get(sessionID)
	if !ok {
		return nil
	}
	return extractPlanFromEvents(run.History())
}
