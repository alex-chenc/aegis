package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agentruntime "github.com/alex-chenc/agent-runtime"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/datatypes"

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
	toolSelector       *ToolSelector
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
	ToolSelector       *ToolSelector
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
		toolSelector:       deps.ToolSelector,
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
	intent := o.intentRouter.Classify(ctx, intentInput)

	// 5. 发布意图检测事件
	o.runManager.Publish(input.SessionID, NewEvent(EventIntentDetected, input.SessionID, input.RunID, map[string]interface{}{
		"domains":    intent.Domains,
		"action":     intent.Action,
		"object":     intent.Object,
		"confidence": intent.Confidence,
	}))

	var intentBreakdown *IntentBreakdown
	if o.intentDecomposer != nil {
		breakdown, err := o.intentDecomposer.Decompose(ctx, IntentDecomposeInput{
			Query:                  input.UserMessage,
			Intent:                 intent,
			ContextRefs:            intentInput.ContextRefs,
			EnableLLMDecomposition: false,
		})
		if err != nil {
			o.logger.Warn("assistant rule intent decomposition failed",
				zap.String("session_id", input.SessionID),
				zap.String("run_id", input.RunID),
				zap.Error(err),
			)
		} else {
			intentBreakdown = breakdown
		}
	}

	// 6. 工具选择：复杂任务优先交给 LLM 根据短目录 + 详情目录选择工具；
	// 规则选择仅用于短句快捷入口或 LLM 不可用时兜底。
	var selection *ToolSelectionResult
	llmSelectedTools := false
	selectionMode := "rules"
	naturalShortcut := detectNaturalOperationShortcut(input.UserMessage)
	if naturalShortcut.Kind != naturalOperationNone {
		selectionMode = "shortcut"
	}
	skipLLMToolSelection := intentBreakdown != nil && intentBreakdown.NeedClarification && intentBreakdown.RequiresWrite
	if shouldUseLLMToolSelection(input.UserMessage, intent) && !skipLLMToolSelection {
		if llmSelection, err := o.selectToolsWithLLM(ctx, input.UserMessage, intent, intentInput.ContextRefs); err == nil && llmSelection != nil && len(llmSelection.SelectedTools) > 0 {
			selection = llmSelection
			llmSelectedTools = true
			selectionMode = "llm"
		} else if err != nil && o.logger != nil {
			o.logger.Warn("llm tool selection failed, falling back to rule selector",
				zap.String("session_id", input.SessionID),
				zap.String("run_id", input.RunID),
				zap.Error(err),
			)
		}
	}
	if selection == nil {
		selection = o.toolSelector.Select(ToolSelectionInput{
			Query:       input.UserMessage,
			ContextRefs: intentInput.ContextRefs,
			Intent:      intent,
			MaxTools:    24,
		})
	}
	useAIAnalysisFlow := o.isComplexTask(input.TaskType, input.UserMessage, intent, selection.SelectedTools)
	if useAIAnalysisFlow && !llmSelectedTools {
		selection.SelectedTools = o.expandComplexTaskTools(selection.SelectedTools, input.TaskType, input.UserMessage, intent)
	}

	if o.intentDecomposer != nil && (intentBreakdown == nil || !intentBreakdown.NeedClarification) {
		breakdown, err := o.intentDecomposer.Decompose(ctx, IntentDecomposeInput{
			Query:                  input.UserMessage,
			Intent:                 intent,
			ContextRefs:            intentInput.ContextRefs,
			CandidateCapabilities:  o.capabilitiesForTools(selection.SelectedTools),
			EnableLLMDecomposition: envBool("ASSISTANT_INTENT_DECOMPOSE_ENABLED", true),
		})
		if err != nil {
			o.logger.Warn("assistant intent decomposition failed, continuing with selector intent",
				zap.String("session_id", input.SessionID),
				zap.String("run_id", input.RunID),
				zap.Error(err),
			)
		} else {
			intentBreakdown = breakdown
		}
	}

	var executionPlan *ToolExecutionPlan
	if o.toolDecisionEngine != nil && o.toolDecisionEngine.config.Enabled {
		plan, err := o.toolDecisionEngine.Decide(ctx, ToolDecisionInput{
			Query:                input.UserMessage,
			Intent:               intent,
			Breakdown:            intentBreakdown,
			ContextRefs:          intentInput.ContextRefs,
			PreliminarySelection: selection,
			UseAIAnalysisFlow:    useAIAnalysisFlow,
		})
		if err != nil {
			o.logger.Warn("assistant tool decision failed, falling back to preliminary selection",
				zap.String("session_id", input.SessionID),
				zap.String("run_id", input.RunID),
				zap.Error(err),
			)
		} else if plan != nil {
			executionPlan = plan

			// 使用 ToolDecisionRecorder 持久化裁决记录
			if o.decisionRecorder != nil {
				_ = o.decisionRecorder.Record(context.Background(), input.SessionID, plan)
			}

			// 使用 ClarificationGate 评估是否需要追问
			clarification := o.clarificationGate.Evaluate(intentBreakdown, plan.ToolNames(), plan.DecisionRecords)
			if plan.NeedClarification || clarification.Required {
				o.mergeSessionMetadata(context.Background(), input.SessionID, map[string]interface{}{
					"intent_breakdown":   intentBreakdown,
					"tool_execution_plan": executionPlan,
					"current_run_status": "clarification_required",
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
			selectionMode = selectionMode + "+decision"
			useAIAnalysisFlow = o.isComplexTask(input.TaskType, input.UserMessage, intent, selection.SelectedTools)
			o.mergeSessionMetadata(context.Background(), input.SessionID, map[string]interface{}{
				"intent_breakdown":    intentBreakdown,
				"tool_execution_plan": executionPlan,
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
	if executionPlan != nil {
		toolSelectionPayload["decision_trace_id"] = executionPlan.DecisionTraceID
		toolSelectionPayload["tool_execution_plan"] = executionPlan
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
	o.logger.Info("using agent-runtime",
		zap.String("session_id", input.SessionID),
		zap.String("runtime_profile", runtimeProfileName(useAIAnalysisFlow)),
		zap.Int("tools_count", len(toolDescriptors)),
	)
	return o.runAgentRuntime(ctx, input, contextRefs, *selection, toolDescriptors, useAIAnalysisFlow, executionPlan)
}

// fallbackResponse 降级响应
func (o *Orchestrator) fallbackResponse(ctx context.Context, input RunInput, reason string) (*RunResult, error) {
	msgID := "msg_" + input.RunID
	response := fmt.Sprintf("抱歉，%s\n\n您的问题是: %s\n\n请稍后重试或联系管理员。", reason, input.UserMessage)

	// 从事件历史中提取 thinking 和 plan 数据
	thinkingContent := o.extractThinkingFromHistory(input.SessionID)
	planData := o.extractPlanFromHistory(input.SessionID)
	o.persistSessionRuntimeEvents(context.Background(), input.SessionID, msgID, compactRuntimeDisplayEvents(o.extractRunHistory(input.SessionID), input.RunID, msgID))

	// 使用 context.Background() 保存消息，避免上下文取消导致保存失败
	saveCtx := context.Background()
	_ = o.messageRepo.Create(saveCtx, &model.AssistantMessage{
		ID:        uuid.New(),
		SessionID: input.SessionID,
		MessageID: msgID,
		Role:      "assistant",
		Content:   response,
		Thinking:  thinkingContent,
		Plan:      planData,
	})

	o.runManager.Publish(input.SessionID, EventDonePayload(input.SessionID, input.RunID))

	return &RunResult{
		MessageID:   msgID,
		FinalAnswer: response,
	}, nil
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

	o.runManager.Publish(input.SessionID, EventDonePayload(input.SessionID, input.RunID))
	return &RunResult{
		MessageID:   msgID,
		FinalAnswer: response,
	}, nil
}

func (o *Orchestrator) capabilitiesForTools(toolNames []string) []string {
	if o == nil || o.toolRegistry == nil {
		return nil
	}
	capabilities := make([]string, 0, len(toolNames))
	for _, name := range toolNames {
		tool, ok := o.toolRegistry.Get(name)
		if !ok || tool == nil || !tool.Enabled {
			continue
		}
		if strings.TrimSpace(tool.Capability) != "" {
			capabilities = append(capabilities, tool.Capability)
			continue
		}
		capabilities = append(capabilities, syntheticToolCapability(tool))
	}
	return dedupeStrings(capabilities)
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
			Description:      tool.Description,
			ArgsSchema:       tool.ArgsSchema,
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

func (o *Orchestrator) expandComplexTaskTools(selected []string, taskType, userMessage string, intent IntentResult) []string {
	extraTools := []string{}

	if shouldUseSecurityToolExpansion(taskType, userMessage, intent) {
		extraTools = append(extraTools,
			"Host.List",
			"Host.Get",
			"Host.AgentStatus.Get",
			"Task.List",
			"Task.GetDetail",
			"Task.RunCheck",
			"Task.RunFix",
			"Vulnerability.List",
			"Vulnerability.AffectedHosts",
			"Software.Installed.Search",
			"Detection.Alert.List",
			"Detection.Alert.Get",
			"Detection.Statistics.Get",
			"Detection.Trend.Get",
			"Agent.Process.List",
			"Agent.Process.Tree",
			"Agent.Network.List",
			"Agent.File.OpenList",
			"Agent.Log.Query",
		)
	}

	if shouldUseBaselineToolExpansion(userMessage, intent) {
		extraTools = append(extraTools,
			"Baseline.Template.List",
			"Baseline.Template.Status.Get",
			"Baseline.Template.Rules.List",
			"Baseline.Script.Generate",
			"Task.RunCheck",
			"Task.RunFix",
			"Task.List",
			"Task.GetDetail",
		)
	}

	if shouldUseAssetCollectionAnalysisToolExpansion(userMessage, intent) {
		extraTools = append(extraTools,
			"Host.List",
			"Asset.Collection.Trigger",
			"Asset.Collection.Get",
			"Asset.Application.List",
			"Asset.Summary.Get",
			"Software.Installed.Search",
			"Vulnerability.List",
			"Vulnerability.AffectedHosts",
		)
	}

	if len(extraTools) == 0 {
		return dedupeToolNames(selected)
	}

	expanded := append([]string{}, selected...)
	expanded = append(expanded, extraTools...)
	return dedupeToolNames(expanded)
}

func shouldUseAssetCollectionAnalysisToolExpansion(userMessage string, intent IntentResult) bool {
	text := normalizeNaturalOperationText(userMessage)
	if !hasAssetCollectionIntent(text) || !hasCompositeNaturalOperationIntent(text) {
		return false
	}
	if strings.Contains(text, "软件") || strings.Contains(text, "漏洞") || strings.Contains(text, "cve") || strings.Contains(text, "mysql") {
		return true
	}
	for _, domain := range intent.Domains {
		if domain == "asset" || domain == "vulnerability" {
			return true
		}
	}
	return false
}

func shouldUseBaselineToolExpansion(userMessage string, intent IntentResult) bool {
	for _, domain := range intent.Domains {
		if domain == "baseline" || domain == "baseline_template" || domain == "baseline_rule" {
			return true
		}
	}
	text := strings.ToLower(userMessage)
	keywords := []string{
		"基线",
		"baseline",
		"检测脚本",
		"修复脚本",
		"脚本生成",
		"下发任务",
		"runcheck",
		"runfix",
		"task.run",
		"task.runcheck",
		"task.runfix",
	}
	for _, keyword := range keywords {
		if strings.Contains(text, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func shouldUseSecurityToolExpansion(taskType, userMessage string, intent IntentResult) bool {
	switch taskType {
	case "investigation", "host_attack_investigation":
		return true
	}
	if intent.Action == "analyze" || intent.Action == "investigate" {
		return true
	}
	securityKeywords := []string{
		"安全", "安全问题", "安全事件", "排查", "风险", "威胁", "攻击", "入侵",
		"溯源", "研判", "告警", "漏洞", "基线", "取证", "异常",
	}
	for _, keyword := range securityKeywords {
		if contains(userMessage, keyword) {
			return true
		}
	}
	for _, domain := range intent.Domains {
		switch domain {
		case "detection", "investigation", "sigma_rule", "block", "agent":
			return true
		}
	}
	return false
}

func dedupeToolNames(names []string) []string {
	seen := make(map[string]bool, len(names))
	result := make([]string, 0, len(names))
	for _, name := range names {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, name)
	}
	return result
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
	// 1. 任务类型明确为复杂任务
	complexTaskTypes := map[string]bool{
		"investigation":             true,
		"host_attack_investigation": true,
		"remediation":               true,
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

	// 5. 如果选中了业务工具（排除 resident 辅助工具），说明需要工具调用，必须走 agent-runtime
	// runDirectLLM 无法执行工具，只能返回文字描述
	// 注意：resident 工具（Tool.Search, Context.Get, Session.Summarize）是无条件追加的辅助工具，
	// 不代表任务本身复杂，不应参与复杂度判断
	residentTools := map[string]bool{
		"Tool.Search":       true,
		"Context.Get":       true,
		"Session.Summarize": true,
	}
	businessToolCount := 0
	for _, name := range selectedTools {
		if !residentTools[name] {
			businessToolCount++
		}
	}
	if businessToolCount > 0 {
		return true
	}

	// 6. 默认为简单任务（问候、闲聊等不需要工具的场景）
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
func (o *Orchestrator) runAgentRuntime(ctx context.Context, input RunInput, contextRefs []ContextObject, selection ToolSelectionResult, toolDescriptors []agentruntime.ToolDescriptor, useAIAnalysisFlow bool, executionPlan *ToolExecutionPlan) (*RunResult, error) {
	convertedRefs := o.convertContextRefs(contextRefs)
	msgID := "msg_" + input.RunID
	maxIterations := 80
	if useAIAnalysisFlow {
		maxIterations = 500
	}
	o.mergeSessionMetadata(ctx, input.SessionID, map[string]interface{}{
		"runtime_profile":    runtimeProfileName(useAIAnalysisFlow),
		"max_total_turns":    maxIterations,
		"current_run_id":     input.RunID,
		"current_message_id": msgID,
		"current_run_status": "running",
		"run_started_at":     time.Now().UTC().Format(time.RFC3339),
	})

	if handled, response, err := o.runNaturalOperationShortcut(ctx, input, msgID, convertedRefs); handled {
		if err != nil {
			return nil, err
		}
		return o.finishNaturalOperationShortcutRun(input, msgID, response, useAIAnalysisFlow, maxIterations)
	}

	// 使用 RuntimeFactory.Build() 创建完整的 agent-runtime 实例
	buildResult, err := o.runtimeFactory.Build(ctx, RuntimeBuildRequest{
		SessionID:         input.SessionID,
		RunID:             input.RunID,
		MessageID:         msgID,
		Operator:          input.UserID,
		UserInput:         input.UserMessage,
		TaskType:          input.TaskType,
		ContextRefs:       convertedRefs,
		SelectedTools:     selection.SelectedTools,
		ToolDescriptors:   toolDescriptors,
		MaxIterations:     maxIterations,
		ExecutionPlan:     executionPlan,
		UseAIAnalysisFlow: useAIAnalysisFlow,
	})
	if err != nil {
		o.logger.Error("failed to build agent-runtime", zap.Error(err))
		return o.fallbackResponse(ctx, input, "创建运行时失败: "+err.Error())
	}

	// 运行 agent-runtime
	taskResult, err := buildResult.Runtime.Run(ctx, agentruntime.TaskInput{
		UserInput:   input.UserMessage,
		UserContext: buildResult.UserContext,
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
	if taskResult != nil {
		contextBudget := effectiveRuntimeContextBudget(taskResult)
		metadata := map[string]interface{}{
			"runtime_profile":         runtimeProfileName(useAIAnalysisFlow),
			"max_total_turns":         maxIterations,
			"current_run_id":          input.RunID,
			"current_message_id":      msgID,
			"current_run_status":      "completed",
			"last_run_completed_at":   time.Now().UTC().Format(time.RFC3339),
			"total_prompt_tokens":     taskResult.Metrics.TotalPromptTokens,
			"total_completion_tokens": taskResult.Metrics.TotalCompletionTokens,
			"total_tokens":            taskResult.Metrics.TotalPromptTokens + taskResult.Metrics.TotalCompletionTokens,
			"compression_count":       len(taskResult.CompressionRecords),
			"context_budget":          contextBudget,
			"compression_records":     taskResult.CompressionRecords,
		}
		o.mergeSessionMetadata(context.Background(), input.SessionID, metadata)
		if contextBudget != nil {
			o.runManager.Publish(input.SessionID, withMessageID(NewEvent(EventContextBudget, input.SessionID, input.RunID, contextBudget), msgID))
		}
	}

	fallbackResponse := ""
	if err == nil {
		if count, summary, fallbackErr := o.executeExplicitToolSequenceFallback(context.Background(), input, msgID); fallbackErr != nil {
			o.logger.Error("explicit tool sequence fallback failed", zap.Error(fallbackErr), zap.String("session_id", input.SessionID))
			err = fallbackErr
		} else if count > 0 {
			fallbackResponse = summary
		}
	}

	// 处理结果
	response := ""

	if err != nil {
		o.logger.Error("agent-runtime error", zap.Error(err))
		response = fmt.Sprintf("抱歉，执行过程中出现错误: %s\n\n您的问题是: %s\n\n请稍后重试或联系管理员。", err.Error(), input.UserMessage)
	} else if fallbackResponse != "" {
		response = fallbackResponse
	} else if taskResult != nil && taskResult.FinalAnswer != "" {
		response = taskResult.FinalAnswer
	} else {
		response = "抱歉，我无法生成响应。请稍后重试。"
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
	saveCtx := context.Background()
	if err := o.messageRepo.Create(saveCtx, &model.AssistantMessage{
		ID:        uuid.New(),
		SessionID: input.SessionID,
		MessageID: msgID,
		Role:      "assistant",
		Content:   response,
		Thinking:  thinkingContent,
		Plan:      planData,
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

// ResumeAfterApprovalRequest 审批恢复请求
type ResumeAfterApprovalRequest struct {
	SessionID  string `json:"session_id"`
	RunID      string `json:"run_id"`
	ApprovalID string `json:"approval_id"`
	Operator   string `json:"operator"`
	Comment    string `json:"comment,omitempty"`
}

// PauseForApproval 暂停运行等待审批（对齐设计文档 18.4 节）
//
// 流程：
//  1. ToolGateway.Call 遇到需要审批的工具
//  2. ApprovalGate.CreateApproval 创建审批记录
//  3. Orchestrator.PauseForApproval 标记 run 为 waiting_approval
//  4. 发送 SSE approval_required 和 run_waiting_approval 事件
//  5. 当前 agent-runtime 调用结束（工具返回 approval_required 错误）
func (o *Orchestrator) PauseForApproval(ctx context.Context, sessionID, runID string, approval *model.AssistantApproval) error {
	run, ok := o.runManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("no active run for session %s", sessionID)
	}

	// 设置审批等待状态
	run.SetWaitingApproval(&WaitingApprovalState{
		ApprovalID:  approval.ApprovalID,
		ToolCallID:  approval.ToolCallID,
		ToolName:    approval.ToolName,
		Operator:    approval.RequestedBy,
		RequestedAt: time.Now(),
	})

	// 发送 SSE 事件
	msgID := "msg_" + runID
	o.runManager.Publish(sessionID, EventApprovalRequiredPayload(sessionID, runID, msgID, map[string]interface{}{
		"approval_id": approval.ApprovalID,
		"tool_name":   approval.ToolName,
		"risk_level":  approval.RiskLevel,
		"title":       approval.Title,
		"expires_at":  approval.ExpiresAt,
	}))

	o.runManager.Publish(sessionID, EventRunWaitingApprovalPayload(sessionID, runID, approval.ApprovalID, approval.ToolName))

	o.logger.Info("run paused for approval",
		zap.String("session_id", sessionID),
		zap.String("run_id", runID),
		zap.String("approval_id", approval.ApprovalID),
		zap.String("tool_name", approval.ToolName),
	)

	return nil
}

// ResumeAfterApproval 审批通过后恢复运行（对齐设计文档 18.4 节）
//
// 流程：
//  1. 用户在前端批准审批
//  2. ApprovalGate.ExecuteApprovedTool 执行原工具
//  3. Orchestrator.ResumeAfterApproval 构造新 TaskInput 继续运行
//  4. 创建新的 agent-runtime 实例，注入"刚才工具结果"
//  5. agent-runtime 继续下一步
func (o *Orchestrator) ResumeAfterApproval(ctx context.Context, req ResumeAfterApprovalRequest) (*RunResult, error) {
	// 1. 获取审批记录
	approval, err := o.approvalGate.GetApproval(ctx, req.ApprovalID)
	if err != nil {
		return nil, fmt.Errorf("approval not found: %w", err)
	}

	// 2. 获取运行状态
	run, ok := o.runManager.Get(req.SessionID)
	if !ok {
		return nil, fmt.Errorf("no active run for session %s", req.SessionID)
	}

	waitingState := run.GetWaitingApproval()
	if waitingState == nil || waitingState.ApprovalID != req.ApprovalID {
		return nil, fmt.Errorf("run is not waiting for approval %s", req.ApprovalID)
	}

	// 3. 执行已批准的工具
	toolResult, err := o.approvalGate.ExecuteApprovedTool(ctx, approval, o.toolDispatcher)
	if err != nil {
		o.logger.Error("failed to execute approved tool", zap.Error(err))
		// 发送错误事件
		o.runManager.Publish(req.SessionID, EventErrorPayload(req.SessionID, run.RunID, fmt.Sprintf("执行已批准工具失败: %s", err.Error())))
		return o.fallbackResponse(ctx, RunInput{
			RunID:     run.RunID,
			SessionID: req.SessionID,
		}, "执行已批准工具失败: "+err.Error())
	}

	// 4. 清除等待状态
	run.ClearWaitingApproval()

	// 5. 发送工具结果事件
	msgID := "msg_" + run.RunID
	o.runManager.Publish(req.SessionID, EventToolResultPayload(req.SessionID, run.RunID, msgID, waitingState.ToolCallID, toolResult.Data))

	// 6. 构造恢复消息，让 agent-runtime 继续
	// 将工具结果作为新的用户消息注入，让 agent-runtime 基于结果继续推理
	resumeMessage := fmt.Sprintf("工具 %s 已执行完成，结果如下：\n%s\n\n请基于此结果继续完成任务。",
		waitingState.ToolName, marshalToString(toolResult.Data))

	// 7. 重新运行 agent-runtime（使用上下文摘要）
	// 获取会话消息历史摘要（用于 agent-runtime 上下文恢复）
	_ = o.buildPreviousSummary(ctx, req.SessionID)

	// 重新构建 runtime 并继续
	_, err = o.runtimeFactory.BuildLLMClient(ctx)
	if err != nil {
		return o.fallbackResponse(ctx, RunInput{
			RunID:       run.RunID,
			SessionID:   req.SessionID,
			UserMessage: resumeMessage,
		}, "LLM 服务不可用")
	}

	// 直接调用 LLM 继续推理（简化实现：将工具结果注入对话）
	response := fmt.Sprintf("工具 %s 执行完成。\n\n结果：%s\n\n基于以上结果，任务已继续处理。",
		waitingState.ToolName, marshalToString(toolResult.Data))

	// 保存消息
	o.runManager.Publish(req.SessionID, EventMessageDeltaPayload(req.SessionID, run.RunID, msgID, response))

	// 从事件历史中提取 thinking 和 plan 数据
	thinkingContent := o.extractThinkingFromHistory(req.SessionID)
	planData := o.extractPlanFromHistory(req.SessionID)

	// 使用 context.Background() 保存消息，避免上下文取消导致保存失败
	saveCtx := context.Background()
	if err := o.messageRepo.Create(saveCtx, &model.AssistantMessage{
		ID:        uuid.New(),
		SessionID: req.SessionID,
		MessageID: msgID,
		Role:      "assistant",
		Content:   response,
		Thinking:  thinkingContent,
		Plan:      planData,
	}); err != nil {
		o.logger.Error("failed to save resume message", zap.Error(err))
	}

	o.runManager.Publish(req.SessionID, EventDonePayload(req.SessionID, run.RunID))

	o.logger.Info("run resumed after approval",
		zap.String("session_id", req.SessionID),
		zap.String("approval_id", req.ApprovalID),
		zap.String("tool_name", waitingState.ToolName),
		zap.Bool("tool_success", toolResult.Success),
	)

	return &RunResult{
		MessageID:   msgID,
		FinalAnswer: response,
	}, nil
}

// buildPreviousSummary 构建之前的对话摘要
func (o *Orchestrator) buildPreviousSummary(ctx context.Context, sessionID string) string {
	messages, err := o.messageRepo.ListBySession(ctx, sessionID, 10)
	if err != nil || len(messages) == 0 {
		return ""
	}

	var summary strings.Builder
	summary.WriteString("之前的对话摘要：\n")
	for _, msg := range messages {
		content := msg.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		summary.WriteString(fmt.Sprintf("[%s] %s\n", msg.Role, content))
	}
	return summary.String()
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
