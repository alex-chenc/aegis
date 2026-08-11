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
	recoveryManager    *RecoveryManager
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
	RecoveryManager    *RecoveryManager
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
		recoveryManager:    deps.RecoveryManager,
		logger:             logger,
	}
}

// RunInput 运行输入（对齐设计文档 6 节）
type RunInput struct {
	RunID                string
	SessionID            string
	MessageID            string
	UserID               string
	UserMessage          string
	OriginalUserMessage  string
	TaskType             string
	ContextRefs          []model.AssistantContextRef
	Locale               string
	PendingClarification *PendingClarification
	RecoveryContext      *RecoveryResumeContext
}

const (
	// Intent classification and decomposition are bounded control-plane stages.
	// They only produce routing metadata, so a slow reasoning provider must not
	// hold an Assistant run for the full long-running task budget.
	assistantIntentStageTimeout    = 90 * time.Second
	assistantDecomposeStageTimeout = 90 * time.Second
)

// RunResult 运行结果
type RunResult struct {
	MessageID   string                          `json:"message_id"`
	FinalAnswer string                          `json:"final_answer"`
	RunStatus   string                          `json:"run_status"`
	GoalOutcome agentruntime.GoalOutcome        `json:"goal_outcome"`
	Recovery    *model.AssistantRecoveryRequest `json:"recovery,omitempty"`
}

// Run 运行编排。所有请求统一进入 agent-runtime；Runtime 根据任务复杂度决定直接回答或 Plan → React。
func (o *Orchestrator) Run(ctx context.Context, input RunInput) (*RunResult, error) {
	if strings.TrimSpace(input.OriginalUserMessage) == "" {
		input.OriginalUserMessage = input.UserMessage
	}
	if input.RecoveryContext != nil {
		input.UserMessage = buildRecoveryResumeQuery(input.UserMessage, input.RecoveryContext)
		o.logger.Info("assistant recovery context attached to linked run",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.String("recovery_id", input.RecoveryContext.RecoveryID),
			zap.String("recovery_code", input.RecoveryContext.Code),
			zap.String("selected_action_id", input.RecoveryContext.SelectedActionID),
		)
	}
	turnQuery := strings.TrimSpace(input.UserMessage)
	approvalMode := o.snapshotApprovalMode(ctx)
	o.logger.Info("starting orchestration",
		zap.String("session_id", input.SessionID),
		zap.String("run_id", input.RunID),
		zap.String("task_type", input.TaskType),
		zap.String("locale", NormalizeLocale(input.Locale)),
		zap.String("approval_mode", approvalMode),
	)

	// 1. 发布 run_started 事件
	o.runManager.Publish(input.SessionID, NewEvent(EventRunStarted, input.SessionID, input.RunID, map[string]interface{}{
		"status":        "started",
		"approval_mode": approvalMode,
	}))

	// 2. 发布 thinking 事件
	o.runManager.Publish(input.SessionID, EventThinkingPayload(input.SessionID, input.RunID,
		localized(input.Locale, "正在分析您的问题...", "Analyzing your request...")))

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

	// MCP Server onboarding is a control-plane mutation in V6.3. Handle it
	// deterministically before the LLM stages so the Assistant does not ask an
	// ambiguous clarification or fall back to a misleading “missing context”
	// answer. The actual onboarding remains available from the MCP aggregation
	// control page, followed by explicit Client authorization and configuration.
	if shouldGuideMCPOnboarding(input) {
		o.logger.Info("assistant MCP onboarding routed to control-plane guidance",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.Bool("resumed_pending_clarification", input.PendingClarification != nil),
		)
		return o.mcpOnboardingGuidanceResponse(input)
	}

	// 4. 意图识别
	workflowRegistry := NewWorkflowRegistry()
	intentInput := IntentInput{
		Query:                input.UserMessage,
		ContextRefs:          buildIntentContextRefs(contextRefs),
		AvailableWorkflows:   workflowRegistry.List(),
		RequiredWorkflowIDs:  explicitWorkflowRequirements(input.UserMessage),
		PendingClarification: input.PendingClarification,
	}
	intentCtx, cancelIntent := context.WithTimeout(ctx, assistantIntentStageTimeout)
	intent, err := o.intentRouter.Classify(intentCtx, intentInput)
	cancelIntent()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			o.logger.Warn("assistant intent classification timed out",
				zap.String("session_id", input.SessionID),
				zap.String("run_id", input.RunID),
				zap.Duration("timeout", assistantIntentStageTimeout),
			)
		}
		return nil, fmt.Errorf("classify assistant intent: %w", err)
	}
	effectiveQuery, effectiveWorkflowIDs, resumedPending := resolveContinuationQuery(
		input.UserMessage,
		intent,
		input.PendingClarification,
	)
	if resumedPending {
		intent.WorkflowIDs = effectiveWorkflowIDs
		input.UserMessage = effectiveQuery
		mergePendingBreakdownIntoIntent(&intent, input.PendingClarification)
		o.logger.Info("assistant clarification resumed pending goal",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.Strings("workflow_ids", intent.WorkflowIDs),
		)
	} else if input.PendingClarification != nil && intent.ContinuationMode == "new_request" {
		o.mergeSessionMetadata(context.Background(), input.SessionID, map[string]interface{}{
			pendingClarificationMetadataKey: nil,
		})
		o.logger.Info("assistant pending clarification replaced by new request",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
		)
	}
	workflowCards, err := workflowRegistry.Resolve(intent.WorkflowIDs)
	if err != nil {
		o.logger.Error("assistant first-layer workflow contract resolution failed",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.Strings("workflow_ids", intent.WorkflowIDs),
			zap.Error(err),
		)
		return nil, fmt.Errorf("resolve assistant intent workflows: %w", err)
	}

	// 5. 发布意图检测事件
	o.runManager.Publish(input.SessionID, NewEvent(EventIntentDetected, input.SessionID, input.RunID, map[string]interface{}{
		"domains":      intent.Domains,
		"action":       intent.Action,
		"object":       intent.Object,
		"workflow_ids": intent.WorkflowIDs,
		"confidence":   intent.Confidence,
	}))
	if o.intentDecomposer == nil {
		return nil, fmt.Errorf("assistant intent decomposer unavailable")
	}
	capabilityCatalog := o.buildCapabilityCatalog(intent, intentInput.ContextRefs, workflowCards)
	workflowIDs := make([]string, 0, len(workflowCards))
	for _, workflow := range workflowCards {
		workflowIDs = append(workflowIDs, workflow.ID)
	}
	o.logger.Info("assistant capability catalog filtered",
		zap.String("session_id", input.SessionID),
		zap.String("run_id", input.RunID),
		zap.Strings("domains", intent.Domains),
		zap.Strings("object_types", intent.ObjectTypes),
		zap.Strings("workflow_ids", workflowIDs),
		zap.String("routing_mode", "closed_first_layer_contract"),
		zap.Int("registered_count", o.toolRegistry.Count()),
		zap.Int("exposed_count", len(capabilityCatalog)),
	)
	decomposeCtx, cancelDecompose := context.WithTimeout(ctx, assistantDecomposeStageTimeout)
	intentBreakdown, err := o.intentDecomposer.Decompose(decomposeCtx, IntentDecomposeInput{
		Query:                  input.UserMessage,
		Intent:                 intent,
		ContextRefs:            intentInput.ContextRefs,
		AvailableCapabilities:  capabilityCatalog,
		AvailableWorkflows:     workflowCards,
		EnableLLMDecomposition: true,
	})
	cancelDecompose()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			o.logger.Warn("assistant intent decomposition timed out",
				zap.String("session_id", input.SessionID),
				zap.String("run_id", input.RunID),
				zap.Duration("timeout", assistantDecomposeStageTimeout),
			)
		}
		o.logger.Error("assistant llm intent decomposition failed",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("decompose assistant intent with llm: %w", err)
	}
	applyRecoveryResumeContextToBreakdown(intentBreakdown, input.RecoveryContext)

	// 6. 工具选择安全不变量：LLM 只返回 capability；后端 exact Mapping
	// 是唯一工具选举边界。Mapping 结果会作为 Runtime 的不可变 initial
	// plan，后续 Planner/ReAct 不得再通过自由 tool_name 选举工具。
	selection := &ToolSelectionResult{
		Query:  input.UserMessage,
		Intent: intent,
	}
	selectionMode := "capability_mapping"
	useAIAnalysisFlow := o.isComplexTask(input.TaskType, input.UserMessage, intent, selection.SelectedTools)

	if o.toolDecisionEngine == nil || !o.toolDecisionEngine.config.Enabled {
		return nil, fmt.Errorf("assistant capability Mapping engine is required and cannot be disabled")
	}
	var authorization *ToolExecutionPlan
	{
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
			applyEffectiveApprovalMode(plan, approvalMode)
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
					"approval_mode":      approvalMode,
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
				rootQuery := turnQuery
				if resumedPending && input.PendingClarification != nil {
					rootQuery = input.PendingClarification.OriginalQuery
				}
				return o.clarificationResponse(ctx, input, intent, intentBreakdown, plan, rootQuery)
			}
			selection.SelectedTools = plan.ToolNames()
			selection.CandidateTools = dedupeStrings(append(selection.CandidateTools, selection.SelectedTools...))
			selectionMode = selectionMode + "+hard_gates"
			useAIAnalysisFlow = o.isComplexTask(input.TaskType, input.UserMessage, intent, selection.SelectedTools)
			o.mergeSessionMetadata(context.Background(), input.SessionID, map[string]interface{}{
				"intent_breakdown":   intentBreakdown,
				"tool_authorization": authorization,
				"approval_mode":      approvalMode,
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
		"approval_mode":   approvalMode,
	}
	if authorization != nil {
		toolSelectionPayload["decision_trace_id"] = authorization.DecisionTraceID
		toolSelectionPayload["tool_authorization"] = authorization
	}
	if intentBreakdown != nil {
		toolSelectionPayload["intent_breakdown"] = intentBreakdown
	}
	if resumedPending {
		o.mergeSessionMetadata(context.Background(), input.SessionID, map[string]interface{}{
			pendingClarificationMetadataKey: nil,
		})
	}
	o.runManager.Publish(input.SessionID, NewEvent(EventToolsSelected, input.SessionID, input.RunID, toolSelectionPayload))

	// 8. 构建 agent-runtime 工具描述符
	toolDescriptors := o.buildAgentToolDescriptors(selection.SelectedTools)

	planningMode := "no_tool_model_response"
	if usesMappingBoundExecutionPlan(authorization) {
		planningMode = "mapping_bound_execution"
	}
	// 9. agent-runtime may produce a direct response when there are no mapped
	// tools. Tool-enabled runs always receive the Mapping-bound initial plan.
	o.logger.Info("assistant request routed to Mapping-bound runtime",
		zap.String("session_id", input.SessionID),
		zap.String("run_id", input.RunID),
		zap.String("runtime_profile", runtimeProfileName(useAIAnalysisFlow)),
		zap.String("planning_mode", planningMode),
		zap.String("selection_mode", selectionMode),
		zap.String("approval_mode", approvalMode),
		zap.Strings("selected_tools", selection.SelectedTools),
		zap.Int("tools_count", len(toolDescriptors)),
	)
	return o.runAgentRuntime(ctx, input, contextRefs, *selection, toolDescriptors, useAIAnalysisFlow, authorization, approvalMode)
}

func (o *Orchestrator) mcpOnboardingGuidanceResponse(input RunInput) (*RunResult, error) {
	msgID := "msg_" + input.RunID
	response := mcpOnboardingGuidance(input.Locale)

	o.runManager.Publish(input.SessionID, NewEvent(EventIntentDetected, input.SessionID, input.RunID, map[string]interface{}{
		"domains":      []string{string(DomainExternalMCP)},
		"action":       "control_plane_guidance",
		"object":       "mcp_aggregation",
		"workflow_ids": []string{},
		"confidence":   1.0,
	}))
	o.runManager.Publish(input.SessionID, NewEvent(EventToolsSelected, input.SessionID, input.RunID, map[string]interface{}{
		"selected_tools":  []string{},
		"candidate_tools": []string{},
		"selection_mode":  "control_plane_guidance",
	}))
	o.runManager.Publish(input.SessionID, EventMessageDeltaPayload(input.SessionID, input.RunID, msgID, response))

	o.mergeSessionMetadata(context.Background(), input.SessionID, map[string]interface{}{
		pendingClarificationMetadataKey: nil,
		"current_run_status":            "completed",
		"goal_outcome":                  agentruntime.GoalSucceeded,
	})
	o.persistSessionRuntimeEvents(
		context.Background(),
		input.SessionID,
		msgID,
		compactRuntimeDisplayEvents(o.extractRunHistory(input.SessionID), input.RunID, msgID),
	)

	if o.messageRepo != nil {
		if err := o.messageRepo.Create(context.Background(), &model.AssistantMessage{
			ID:        uuid.New(),
			SessionID: input.SessionID,
			MessageID: msgID,
			Role:      "assistant",
			Content:   response,
			Thinking:  o.extractThinkingFromHistory(input.SessionID),
		}); err != nil {
			o.logger.Error("failed to save MCP onboarding guidance message",
				zap.String("session_id", input.SessionID),
				zap.String("run_id", input.RunID),
				zap.Error(err),
			)
		}
	}

	return &RunResult{
		MessageID:   msgID,
		FinalAnswer: response,
		RunStatus:   "completed",
		GoalOutcome: agentruntime.GoalSucceeded,
	}, nil
}

func (o *Orchestrator) clarificationResponse(
	ctx context.Context,
	input RunInput,
	intent IntentResult,
	breakdown *IntentBreakdown,
	plan *ToolExecutionPlan,
	originalQuery string,
) (*RunResult, error) {
	_ = ctx
	msgID := "msg_" + input.RunID
	response := strings.TrimSpace(plan.ClarifyingQuestion)
	if response == "" {
		response = localized(input.Locale, "请补充要操作的对象和范围后再执行。", "Specify the target and scope before continuing.")
	}
	var missingInfo []MissingInfo
	if breakdown != nil {
		missingInfo = append(missingInfo, breakdown.MissingInfo...)
	}
	o.mergeSessionMetadata(context.Background(), input.SessionID, map[string]interface{}{
		pendingClarificationMetadataKey: PendingClarification{
			OriginalQuery:   strings.TrimSpace(originalQuery),
			Goal:            strings.TrimSpace(plan.Goal),
			Question:        response,
			WorkflowIDs:     dedupeStrings(intent.WorkflowIDs),
			MissingInfo:     missingInfo,
			IntentBreakdown: breakdown,
		},
	})

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

func mergePendingBreakdownIntoIntent(intent *IntentResult, pending *PendingClarification) {
	if intent == nil || pending == nil || pending.IntentBreakdown == nil {
		return
	}
	breakdown := pending.IntentBreakdown
	intent.Domains = dedupeStrings(append(intent.Domains, breakdown.Domains...))
	for _, object := range breakdown.Objects {
		if objectType := strings.TrimSpace(object.Type); objectType != "" {
			intent.ObjectTypes = append(intent.ObjectTypes, objectType)
		}
	}
	intent.ObjectTypes = dedupeStrings(intent.ObjectTypes)
	if breakdown.RequiresWrite {
		intent.NeedWrite = true
	}
}

// buildAgentToolDescriptors 从工具注册表构建 agent-runtime 工具描述符
func (o *Orchestrator) buildAgentToolDescriptors(toolNames []string) []agentruntime.ToolDescriptor {
	selected := make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		selected[name] = struct{}{}
	}
	toolByCapability := make(map[string]string)
	for _, tool := range o.toolRegistry.List() {
		if tool == nil || !tool.Enabled {
			continue
		}
		if _, ok := selected[tool.Name]; !ok {
			continue
		}
		if capability := strings.TrimSpace(tool.Capability); capability != "" {
			toolByCapability[capability] = tool.Name
		}
	}

	var descriptors []agentruntime.ToolDescriptor
	for _, name := range toolNames {
		tool, ok := o.toolRegistry.Get(name)
		if !ok || !tool.Enabled {
			continue
		}

		// 使用 tool_catalog.go 中的 toRuntimeRisk 映射风险等级
		riskLevel := toRuntimeRisk(tool.Risk)
		var completionTools []string
		if completionCapability := strings.TrimSpace(tool.ExecutionContract.CompletionCapability); completionCapability != "" {
			if completionTool, ok := toolByCapability[completionCapability]; ok {
				completionTools = append(completionTools, completionTool)
			}
		}

		descriptors = append(descriptors, agentruntime.ToolDescriptor{
			Name:             tool.Name,
			Description:      modelFacingToolDescription(tool),
			ArgsSchema:       normalizeRuntimeArgsSchema(tool.ArgsSchema),
			ResultSchema:     normalizeRuntimeArgsSchema(tool.ResultSchema),
			CompletionTools:  completionTools,
			Prerequisites:    toRuntimePrerequisites(tool.ExecutionContract.Prerequisites),
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

func (o *Orchestrator) buildCapabilityCatalog(intent IntentResult, contextRefs []ContextRefInput, workflowSpecs []WorkflowSpec) []CapabilityCatalogItem {
	if o == nil || o.toolRegistry == nil || len(workflowSpecs) == 0 {
		return nil
	}
	objectTypes := append([]string{}, intent.ObjectTypes...)
	for _, ref := range contextRefs {
		if strings.TrimSpace(ref.ObjectType) != "" {
			objectTypes = append(objectTypes, ref.ObjectType)
		}
	}
	workflowIDs := make([]string, 0, len(workflowSpecs))
	for _, workflow := range workflowSpecs {
		workflowIDs = append(workflowIDs, workflow.ID)
	}
	closedCapabilities, useClosedAllowlist := closedWorkflowCapabilityAllowlist(workflowSpecs)
	exposureResolver := NewToolExposureResolver(o.toolRegistry)
	var tools []*ToolSpec
	if useClosedAllowlist {
		tools = exposureResolver.intentCatalogForCapabilities(closedCapabilities, "")
	} else {
		tools = exposureResolver.IntentCatalog(ToolExposureContext{
			Domains:     intent.Domains,
			ObjectTypes: dedupeStrings(objectTypes),
			WorkflowIDs: workflowIDs,
		})
	}
	sort.Slice(tools, func(i, j int) bool {
		if tools[i].ExposurePolicy.CatalogPriority != tools[j].ExposurePolicy.CatalogPriority {
			return tools[i].ExposurePolicy.CatalogPriority > tools[j].ExposurePolicy.CatalogPriority
		}
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

func closedWorkflowCapabilityAllowlist(workflows []WorkflowSpec) (map[string]bool, bool) {
	if len(workflows) == 0 {
		return nil, false
	}
	allowed := make(map[string]bool)
	for _, workflow := range workflows {
		if len(workflow.ExposedCapabilities) == 0 {
			return nil, false
		}
		for _, capability := range workflow.ExposedCapabilities {
			if capability = strings.ToLower(strings.TrimSpace(capability)); capability != "" {
				allowed[capability] = true
			}
		}
	}
	return allowed, true
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
func (o *Orchestrator) runAgentRuntime(ctx context.Context, input RunInput, contextRefs []ContextObject, selection ToolSelectionResult, toolDescriptors []agentruntime.ToolDescriptor, useAIAnalysisFlow bool, authorization *ToolExecutionPlan, approvalMode string) (*RunResult, error) {
	msgID := "msg_" + input.RunID
	executionPlan := mappingBoundExecutionPlanForAssistant(authorization)
	planningMode := "no_tool_model_response"
	if executionPlan != nil {
		planningMode = "mapping_bound_execution"
		o.logger.Info("assistant runtime received immutable Mapping-bound execution plan",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.String("plan_id", executionPlan.DecisionTraceID),
			zap.Int("plan_step_count", len(executionPlan.Steps)),
		)
	}
	maxIterations := 80
	if useAIAnalysisFlow {
		maxIterations = 500
	}
	o.mergeSessionMetadata(ctx, input.SessionID, map[string]interface{}{
		"runtime_profile":    runtimeProfileName(useAIAnalysisFlow),
		"planning_mode":      planningMode,
		"max_total_turns":    maxIterations,
		"current_run_id":     input.RunID,
		"current_message_id": msgID,
		"current_run_status": "running",
		"run_started_at":     time.Now().UTC().Format(time.RFC3339),
		"locale":             NormalizeLocale(input.Locale),
		"approval_mode":      normalizeAssistantApprovalMode(approvalMode),
	})

	runtimeCtx, stopRuntimeForRecovery := context.WithCancel(ctx)
	defer stopRuntimeForRecovery()

	// 使用 RuntimeFactory.Build() 创建完整的 agent-runtime 实例
	buildResult, err := o.runtimeFactory.Build(ctx, RuntimeBuildRequest{
		SessionID:         input.SessionID,
		RunID:             input.RunID,
		MessageID:         msgID,
		Operator:          input.UserID,
		UserInput:         input.UserMessage,
		OriginalUserInput: input.OriginalUserMessage,
		TaskType:          input.TaskType,
		ContextRefs:       o.convertContextRefs(contextRefs),
		SelectedTools:     selection.SelectedTools,
		ToolDescriptors:   toolDescriptors,
		MaxIterations:     maxIterations,
		ExecutionPlan:     executionPlan,
		UseAIAnalysisFlow: useAIAnalysisFlow,
		Locale:            NormalizeLocale(input.Locale),
		ApprovalMode:      normalizeAssistantApprovalMode(approvalMode),
		StopForRecovery:   stopRuntimeForRecovery,
	})
	if err != nil {
		o.logger.Error("failed to build agent-runtime", zap.Error(err))
		return nil, fmt.Errorf("build agent runtime: %w", err)
	}

	// 运行 agent-runtime
	taskResult, err := buildResult.Runtime.Run(runtimeCtx, agentruntime.TaskInput{
		UserInput:   input.UserMessage,
		UserContext: buildResult.UserContext,
		InitialPlan: runtimeInitialPlanForAssistantWithDescriptors(executionPlan, toolDescriptors),
		Metadata: map[string]string{
			"session_id":    input.SessionID,
			"run_id":        input.RunID,
			"task_type":     input.TaskType,
			"locale":        NormalizeLocale(input.Locale),
			"approval_mode": normalizeAssistantApprovalMode(approvalMode),
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
	var blockedSteps []agentruntime.PlanStep
	if usesMappingBoundExecutionPlan(executionPlan) {
		blockedSteps = normalizeBlockedMappingPlanSteps(taskResult)
	}
	if len(blockedSteps) > 0 {
		blockedStepIDs := make([]string, 0, len(blockedSteps))
		for _, step := range blockedSteps {
			blockedStepIDs = append(blockedStepIDs, step.StepID)
			o.runManager.Publish(input.SessionID, withMessageID(NewEvent(EventStepSkipped, input.SessionID, input.RunID, map[string]interface{}{
				"step_id": step.StepID,
				"title":   step.Title,
				"status":  "skipped",
				"reason":  "blocked_by_failed_dependency",
			}), msgID))
		}
		o.logger.Info("assistant Mapping-bound steps skipped after dependency failure",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.Strings("step_ids", blockedStepIDs),
		)
	}
	saveCtx := context.Background()
	o.persistRuntimeToolCallRecords(saveCtx, input.SessionID, msgID, taskResult)
	evidence := buildRuntimeEvidenceLedger(taskResult)
	response := taskResult.FinalAnswer
	evidenceConflicts := make([]string, 0, 4)
	if normalized, err := normalizeRuntimeFinalAnswer(response); err != nil {
		o.logger.Warn("assistant final answer violated the output contract",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.String("message_id", msgID),
			zap.Error(err),
		)
		evidenceConflicts = append(evidenceConflicts, "invalid_final_answer_contract")
		response = buildEvidenceGroundedFallback(evidence)
	} else {
		response = normalized
	}
	evidenceConflicts = append(evidenceConflicts, validateRuntimeEvidenceConsistency(response, evidence)...)
	evidenceConflicts = dedupeStrings(evidenceConflicts)
	if len(evidenceConflicts) > 0 {
		o.logger.Warn("assistant final answer conflicted with or omitted runtime tool evidence",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.String("message_id", msgID),
			zap.Strings("conflict_codes", evidenceConflicts),
		)
		if !containsExactString(evidenceConflicts, "invalid_final_answer_contract") {
			response = buildEvidenceGroundedFallback(evidence)
		}
	}
	goalOutcome := taskResult.GoalOutcome
	if goalOutcome == "" {
		goalOutcome = agentruntime.GoalFailed
	}
	if len(evidenceConflicts) > 0 && goalOutcome == agentruntime.GoalSucceeded {
		goalOutcome = agentruntime.GoalPartiallySucceeded
	}
	if authorization != nil &&
		authorization.RequiredOutcome == "detection_package_enabled" &&
		hasFailedDetectionPackageStage(evidence) {
		if strings.TrimSpace(evidence.DetectionPackageID) == "" {
			goalOutcome = agentruntime.GoalFailed
		} else {
			goalOutcome = agentruntime.GoalPartiallySucceeded
		}
		o.logger.Warn("assistant detection package lifecycle stopped after failed stage",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.String("package_id", evidence.DetectionPackageID),
			zap.Strings("failed_tools", evidence.FailedToolNames),
		)
	}
	var pendingLifecycle *PendingClarification
	var pendingRecovery *model.AssistantRecoveryRequest
	if authorization != nil &&
		strings.TrimSpace(authorization.DeferredClarification) != "" &&
		goalOutcome != agentruntime.GoalFailed {
		goalOutcome = agentruntime.GoalNeedsInput
		question := strings.TrimSpace(authorization.DeferredClarification)
		pendingLifecycle = &PendingClarification{
			OriginalQuery: originalRunQuery(input),
			Goal:          authorization.Goal,
			Question:      question,
			WorkflowIDs:   dedupeStrings(authorization.RemainingWorkflowIDs),
			Artifacts: map[string]string{
				"package_id": evidence.DetectionPackageID,
			},
		}
		response = strings.TrimSpace(response) + "\n\n" + question
		o.logger.Info("assistant ordered workflow paused for deferred clarification",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.Strings("remaining_workflow_ids", authorization.RemainingWorkflowIDs),
		)
	} else if shouldPauseDetectionPackageBeforeActivation(authorization, evidence, goalOutcome) {
		goalOutcome = agentruntime.GoalNeedsInput
		stage := strings.TrimSpace(evidence.DetectionPackageStatus)
		if stage == "" {
			stage = "build_pending"
		}
		question := localized(
			input.Locale,
			"检测包尚未签名并启用。若构建处于待审核状态，请先完成审核；随后回复“继续”以签名、启用并分发。",
			"The package is not signed and enabled yet. Complete build review if required, then reply \"continue\" to sign, enable, and distribute it.",
		)
		pendingLifecycle = &PendingClarification{
			OriginalQuery: originalRunQuery(input),
			Goal:          authorization.Goal,
			Question:      question,
			WorkflowIDs:   dedupeStrings(authorization.WorkflowIDs),
			Artifacts: map[string]string{
				"package_id": evidence.DetectionPackageID,
				"status":     stage,
			},
		}
		response = strings.TrimSpace(response) + "\n\n" + question
		o.logger.Info("assistant detection package lifecycle paused before activation",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.String("package_id", evidence.DetectionPackageID),
			zap.String("package_status", stage),
			zap.String("required_outcome", authorization.RequiredOutcome),
		)
	}
	if o.recoveryManager != nil {
		if recoveryRequest, recoveryErr := o.recoveryManager.FindPendingByRun(saveCtx, input.RunID); recoveryErr == nil {
			pendingRecovery = recoveryRequest
			goalOutcome = agentruntime.GoalNeedsInput
			response = buildRecoveryRequiredAnswer(recoveryRequest, input.Locale)
			o.logger.Info("assistant run paused for recoverable tool blocker",
				zap.String("session_id", input.SessionID),
				zap.String("run_id", input.RunID),
				zap.String("recovery_id", recoveryRequest.RecoveryID),
				zap.String("tool_name", recoveryRequest.ToolName),
				zap.String("error_code", recoveryRequest.Code),
			)
		} else if !errors.Is(recoveryErr, gorm.ErrRecordNotFound) {
			o.logger.Error("failed to inspect pending recovery after runtime",
				zap.String("session_id", input.SessionID),
				zap.String("run_id", input.RunID),
				zap.Error(recoveryErr),
			)
		}
	}
	if goalOutcome == agentruntime.GoalFailed {
		// agent-runtime may finish its control loop normally after a failed
		// plan. Never expose that transport completion as a completed task.
		response = buildFailedGoalFallback(evidence)
		o.logger.Warn("assistant final answer replaced for failed goal outcome",
			zap.String("session_id", input.SessionID),
			zap.String("run_id", input.RunID),
			zap.Int("failed_tool_count", len(evidence.FailedToolNames)),
			zap.Strings("failed_tools", evidence.FailedToolNames),
			zap.Int("task_group_count", len(evidence.TaskGroupIDs)),
		)
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
	if pendingRecovery != nil {
		metadata["pending_recovery_id"] = pendingRecovery.RecoveryID
	}
	if pendingLifecycle != nil {
		metadata[pendingClarificationMetadataKey] = pendingLifecycle
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
		Recovery:    pendingRecovery,
	}, nil
}

func buildRecoveryRequiredAnswer(request *model.AssistantRecoveryRequest, locale string) string {
	if request == nil {
		return localized(locale, "任务已暂停，需要您选择处理方式。", "The task is paused and needs your decision.")
	}
	title := localized(locale, "任务已安全暂停，需要您选择处理方式。", "The task was safely paused and needs your decision.")
	var b strings.Builder
	b.WriteString(title)
	if strings.TrimSpace(request.Summary) != "" {
		b.WriteString("\n\n- " + strings.TrimSpace(request.Summary))
	}
	if strings.TrimSpace(request.Detail) != "" {
		b.WriteString("\n- " + strings.TrimSpace(request.Detail))
	}
	b.WriteString(localized(
		locale,
		"\n- 请在恢复决策卡中查看影响并选择下一步；在您确认前，系统不会扩大安全边界。",
		"\n- Review the impact and choose the next step in the recovery card. No security boundary will be expanded before you confirm.",
	))
	return b.String()
}

func shouldPauseDetectionPackageBeforeActivation(
	authorization *ToolExecutionPlan,
	evidence runtimeEvidenceLedger,
	goalOutcome agentruntime.GoalOutcome,
) bool {
	if authorization == nil ||
		authorization.RequiredOutcome != "detection_package_enabled" ||
		containsExactString(evidence.ActualToolNames, "Package.Enable") ||
		goalOutcome == agentruntime.GoalFailed {
		return false
	}
	for _, failedTool := range evidence.FailedToolNames {
		if strings.HasPrefix(strings.TrimSpace(failedTool), "Package.") {
			return false
		}
	}
	return true
}

func hasFailedDetectionPackageStage(evidence runtimeEvidenceLedger) bool {
	for _, failedTool := range evidence.FailedToolNames {
		if strings.HasPrefix(strings.TrimSpace(failedTool), "Package.") {
			return true
		}
	}
	return false
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

func normalizeRuntimeFinalAnswer(answer string) (string, error) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "", fmt.Errorf("agent runtime returned an empty final answer")
	}
	var control map[string]interface{}
	if err := json.Unmarshal([]byte(answer), &control); err != nil {
		return answer, nil
	}
	if wrapped, _ := control["final_answer"].(string); strings.TrimSpace(wrapped) != "" {
		return strings.TrimSpace(wrapped), nil
	}
	action, _ := control["action"].(string)
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "tool_call", "additional_capability_request", "need_user_input", "clarification_required":
		return "", fmt.Errorf("agent runtime ended with unfinished control action %q", action)
	case "step_result":
		if stepResult, ok := control["step_result"].(map[string]interface{}); ok {
			if result, _ := stepResult["result"].(string); strings.TrimSpace(result) != "" {
				return strings.TrimSpace(result), nil
			}
		}
		return "", fmt.Errorf("agent runtime returned step_result without a user-facing result")
	}
	return "", fmt.Errorf("agent runtime returned a JSON object without a valid final_answer wrapper")
}

func validateRuntimeFinalAnswer(answer string) error {
	_, err := normalizeRuntimeFinalAnswer(answer)
	return err
}

const intentContextSummaryMaxRunes = 4000

func buildIntentContextRefs(refs []ContextObject) []ContextRefInput {
	result := make([]ContextRefInput, 0, len(refs))
	for _, ref := range refs {
		result = append(result, ContextRefInput{
			ObjectType: ref.ObjectType,
			ObjectID:   ref.ObjectID,
			Title:      strings.TrimSpace(ref.Title),
			Summary:    truncateIntentContextSummary(ref.Summary),
		})
	}
	return result
}

func truncateIntentContextSummary(summary string) string {
	runes := []rune(strings.TrimSpace(summary))
	if len(runes) <= intentContextSummaryMaxRunes {
		return string(runes)
	}
	return string(runes[:intentContextSummaryMaxRunes]) + "\n..."
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
		// Descriptor, preparation, argument, policy, and step-scope failures
		// happen before the Aegis gateway accepts an executable invocation.
		// Keep them in runtime evidence, but never persist them as durable or
		// user-visible tool calls.
		if strings.TrimSpace(runtimeCall.ValidationStage) != "" {
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
	applySessionMetadataUpdates(metadata, updates)
	session.Metadata = mustMarshalJSON(metadata)
	if err := o.sessionRepo.Update(ctx, session); err != nil {
		o.logger.Warn("failed to update assistant session metadata",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}
}

func applySessionMetadataUpdates(metadata, updates map[string]interface{}) {
	for key, value := range updates {
		if value == nil {
			delete(metadata, key)
			continue
		}
		metadata[key] = value
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
