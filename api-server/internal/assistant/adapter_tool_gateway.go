package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"api-server/internal/model"
	agentruntime "github.com/alex-chenc/agent-runtime"
	"go.uber.org/zap"
)

// capturedStepOutcome holds the reference-bearing fields extracted from a prior
// step's tool outcome. It is the deterministic source for binding a downstream
// step's previous_step arguments, so the model never has to regenerate values
// like operation_id, scan_id, query_id or task_id.
type capturedStepOutcome struct {
	operationRef map[string]string
	sideEffects  []map[string]interface{}
	facts        []map[string]interface{}
	terminal     bool
}

// AssistantToolGatewayAdapter 适配 agent-runtime ToolGateway 接口
// 将 agent-runtime 的工具调用桥接到 assistant 的 ToolDispatcher
type AssistantToolGatewayAdapter struct {
	dispatcher        *ToolDispatcher
	sessionID         string
	messageID         string
	runID             string
	operator          string
	approvalMode      string
	requireMappedPlan bool
	logger            *zap.Logger
	runManager        *RunManager

	// executionPlan is the immutable capability-to-tool Mapping artifact. Every
	// Assistant tool call must remain inside this backend-compiled authority.
	executionPlan *ToolExecutionPlan
	// runtimeStepToolBindings is the immutable per-runtime-step allowlist
	// compiled from Mapping. An asynchronous producer step may contain its
	// already-mapped completion tool; no other tool can enter that boundary.
	runtimeStepToolBindings map[string][]string

	// priorStepOutcomes captures each Mapping-bound step's extracted operation
	// and side-effect references, keyed by step_id. It is the deterministic
	// binding source for downstream steps whose required arguments declare a
	// previous_step source. The runtime executes Mapping-bound steps in
	// dependency order, so a producer's outcome is always captured before its
	// consumer's Prepare runs.
	priorStepOutcomes map[string]capturedStepOutcome
	outcomesMu        sync.Mutex

	// 回调函数，用于 SSE 事件推送
	onToolCall        func(callID, toolName string, args interface{})
	onToolResult      func(callID string, result interface{}, outcome *agentruntime.ToolOutcome)
	onToolError       func(callID, errMsg string)
	onApproval        func(approval interface{})
	onApprovalUpdated func(approval interface{})
}

// AssistantToolGatewayConfig 适配器配置
type AssistantToolGatewayConfig struct {
	Dispatcher              *ToolDispatcher
	SessionID               string
	MessageID               string
	RunID                   string
	Operator                string
	ApprovalMode            string
	RequireMappedPlan       bool
	Logger                  *zap.Logger
	RunManager              *RunManager
	UserInput               string
	ContextRefs             []ContextRefResult
	ExecutionPlan           *ToolExecutionPlan
	RuntimeStepToolBindings map[string][]string
	OnToolCall              func(callID, toolName string, args interface{})
	OnToolResult            func(callID string, result interface{}, outcome *agentruntime.ToolOutcome)
	OnToolError             func(callID, errMsg string)
	OnApproval              func(approval interface{})
	OnApprovalUpdated       func(approval interface{})
}

// NewAssistantToolGatewayAdapter 创建适配器
func NewAssistantToolGatewayAdapter(cfg AssistantToolGatewayConfig) *AssistantToolGatewayAdapter {
	approvalMode := strings.TrimSpace(cfg.ApprovalMode)
	if approvalMode != "" {
		approvalMode = normalizeAssistantApprovalMode(approvalMode)
	}
	return &AssistantToolGatewayAdapter{
		dispatcher:              cfg.Dispatcher,
		sessionID:               cfg.SessionID,
		messageID:               cfg.MessageID,
		runID:                   cfg.RunID,
		operator:                cfg.Operator,
		approvalMode:            approvalMode,
		requireMappedPlan:       cfg.RequireMappedPlan,
		logger:                  cfg.Logger,
		runManager:              cfg.RunManager,
		executionPlan:           cfg.ExecutionPlan,
		runtimeStepToolBindings: cloneRuntimeStepToolBindings(cfg.RuntimeStepToolBindings),
		priorStepOutcomes:       make(map[string]capturedStepOutcome),
		onToolCall:              cfg.OnToolCall,
		onToolResult:            cfg.OnToolResult,
		onToolError:             cfg.OnToolError,
		onApproval:              cfg.OnApproval,
		onApprovalUpdated:       cfg.OnApprovalUpdated,
	}
}

// Call 实现 agentruntime.ToolGateway 接口
func (a *AssistantToolGatewayAdapter) Call(ctx context.Context, req agentruntime.ToolRequest) (agentruntime.ToolResponse, error) {
	startedAt := time.Now()

	if req.Context == nil || req.Context["aegis_prepared"] != "true" {
		prepared, err := a.Prepare(ctx, req)
		if err != nil {
			return agentruntime.ToolResponse{}, err
		}
		req = prepared
	}
	args := req.Args
	asyncPoll := req.Context != nil && req.Context["agent_runtime_async_poll"] == "true"
	pollAttempt := ""
	if req.Context != nil {
		pollAttempt = req.Context["agent_runtime_poll_attempt"]
	}

	if !asyncPoll {
		if cachedResp, ok := a.reuseSuccessfulReadonlyToolCall(ctx, req, args, startedAt); ok {
			return cachedResp, nil
		}
	}

	// 通知工具调用开始
	if a.onToolCall != nil && !asyncPoll {
		a.onToolCall(req.CallID, req.ToolName, args)
	}

	// 通过 ToolDispatcher 调度执行
	result, err := a.dispatcher.Dispatch(ctx, DispatchRequest{
		SessionID:    a.sessionID,
		MessageID:    a.messageID,
		RunID:        a.runID,
		StepID:       req.StepID,
		CallID:       req.CallID,
		ToolName:     req.ToolName,
		Args:         args,
		Operator:     a.operator,
		ApprovalMode: a.approvalMode,
		Polling:      asyncPoll,
	})

	endedAt := time.Now()

	if err != nil {
		errMsg := fmt.Sprintf("tool dispatch error: %v", err)
		if a.onToolError != nil {
			a.onToolError(req.CallID, errMsg)
		}
		return agentruntime.ToolResponse{
			CallID:       req.CallID,
			ToolName:     req.ToolName,
			Status:       agentruntime.ToolCallFailed,
			ErrorMessage: errMsg,
			Summary:      fmt.Sprintf("工具 %s 调度失败", req.ToolName),
			StartedAt:    startedAt,
			EndedAt:      endedAt,
		}, nil
	}

	// 处理需要审批的情况
	if result.ApprovalRequired {
		approval := result.Approval
		if approval == nil {
			approval = &model.AssistantApproval{
				ApprovalID: result.ApprovalID,
				SessionID:  a.sessionID,
				ToolCallID: result.CallID,
				ToolName:   result.ToolName,
				Status:     model.ApprovalStatusPending,
			}
		}
		if a.runManager != nil {
			if run, ok := a.runManager.Get(a.sessionID); ok {
				run.SetWaitingApproval(&WaitingApprovalState{
					ApprovalID:  approval.ApprovalID,
					ToolCallID:  result.CallID,
					ToolName:    result.ToolName,
					Args:        args,
					Operator:    a.operator,
					RequestedAt: time.Now(),
				})
			}
		}
		if a.onApproval != nil {
			a.onApproval(approval)
		}
		return a.waitApprovalAndExecute(ctx, req, approval, startedAt)
	}

	// 处理执行结果
	if result.Success {
		resultJSON, _ := json.Marshal(result.Data)
		tool, _ := a.dispatcher.registry.Get(req.ToolName)
		outcome := normalizeToolOutcome(tool, result.Data)
		// Capture the operation/side-effect references so a downstream step can
		// deterministically bind its previous_step arguments from this result.
		// Captured on every successful call (including non-terminal Accepted and
		// async poll refreshes) so the reference is available as soon as the
		// producer returns it.
		a.captureStepOutcome(req.StepID, req.ToolName, outcome)
		a.logToolOutcome(req, outcome)
		if a.onToolResult != nil && (!asyncPoll || outcome.Terminal) {
			a.onToolResult(result.CallID, result.Data, outcome)
		}
		if asyncPoll && outcome.Terminal && a.logger != nil {
			a.logger.Info("assistant asynchronous operation reached terminal state",
				zap.String("session_id", a.sessionID),
				zap.String("run_id", a.runID),
				zap.String("call_id", result.CallID),
				zap.String("tool_name", result.ToolName),
				zap.String("poll_attempt", pollAttempt),
				zap.String("operation_status", string(outcome.OperationStatus)),
			)
		}
		return agentruntime.ToolResponse{
			CallID:    req.CallID,
			ToolName:  req.ToolName,
			Status:    agentruntime.ToolCallSuccess,
			Content:   string(resultJSON),
			Summary:   outcome.Message,
			StartedAt: startedAt,
			EndedAt:   endedAt,
			Outcome:   outcome,
		}, nil
	}

	// 工具执行失败
	if a.onToolError != nil {
		a.onToolError(result.CallID, result.Error)
	}
	return agentruntime.ToolResponse{
		CallID:       req.CallID,
		ToolName:     req.ToolName,
		Status:       agentruntime.ToolCallFailed,
		ErrorMessage: result.Error,
		Summary:      fmt.Sprintf("工具 %s 执行失败", req.ToolName),
		StartedAt:    startedAt,
		EndedAt:      endedAt,
	}, nil
}

// Prepare implements agentruntime.ToolRequestPreparer. It applies authoritative
// plan arguments, allowlisted normalization, and backend schema/business
// validation before agent-runtime can treat the candidate as an executable
// tool call.
func (a *AssistantToolGatewayAdapter) Prepare(ctx context.Context, req agentruntime.ToolRequest) (agentruntime.ToolRequest, error) {
	if err := a.validateMappedToolSelection(req); err != nil {
		if a.logger != nil {
			a.logger.Warn("assistant runtime attempted to replace a Mapping-bound tool",
				zap.String("session_id", a.sessionID),
				zap.String("run_id", a.runID),
				zap.String("step_id", req.StepID),
				zap.String("requested_tool", req.ToolName),
				zap.Error(err),
			)
		}
		return req, err
	}
	args := make(map[string]interface{}, len(req.Args)+4)
	for key, value := range req.Args {
		args[key] = value
	}
	args = a.applyPlanArgs(req.StepID, req.ToolName, args)
	// Deterministically bind previous_step arguments from prior step outcomes
	// captured during this run. This keeps operation/scan/query/task references
	// fully backend-driven: the model never regenerates them. If a REQUIRED
	// previous_step argument cannot be resolved (e.g. its producer step did not
	// run or did not produce the reference), skip dispatch with a clear reason
	// rather than letting schema validation reject a missing required argument.
	args, skipReason := a.resolvePreviousStepArgs(req.StepID, req.ToolName, args)
	if skipReason != "" {
		if a.logger != nil {
			a.logger.Info("assistant mapping step skipped: unresolvable previous_step dependency",
				zap.String("session_id", a.sessionID),
				zap.String("run_id", a.runID),
				zap.String("step_id", req.StepID),
				zap.String("tool_name", req.ToolName),
				zap.String("reason", skipReason),
			)
		}
		if req.Context == nil {
			req.Context = make(map[string]string)
		}
		req.Context["aegis_prepared"] = "true"
		req.Args = args
		return req, fmt.Errorf("assistant step skipped: %s", skipReason)
	}
	if a.dispatcher != nil {
		prepared, err := a.dispatcher.PrepareInvocation(ctx, ToolInvocationFilterRequest{
			SessionID: a.sessionID,
			MessageID: a.messageID,
			RunID:     a.runID,
			StepID:    req.StepID,
			Phase:     ToolInvocationPhaseCandidate,
			ToolName:  req.ToolName,
			Args:      args,
		})
		if err != nil {
			return req, err
		}
		args = prepared.Args
	}

	if req.Context == nil {
		req.Context = make(map[string]string)
	}
	req.Context["aegis_prepared"] = "true"
	req.Args = args
	return req, nil
}

// validateMappedToolSelection enforces the second half of the tool-election
// invariant at the last pre-runtime boundary. A model may copy the one tool
// bound to its current step, but it may not add or replace tool_name.
func (a *AssistantToolGatewayAdapter) validateMappedToolSelection(req agentruntime.ToolRequest) error {
	if a == nil || a.executionPlan == nil || len(a.executionPlan.Steps) == 0 {
		if a != nil && a.requireMappedPlan {
			return fmt.Errorf("assistant tool invocation requires a Mapping-bound execution plan")
		}
		return nil
	}
	if a.requireMappedPlan && strings.TrimSpace(req.StepID) == "" {
		return fmt.Errorf("assistant tool invocation requires a Mapping-bound step_id")
	}
	if strings.TrimSpace(req.StepID) != "" {
		if allowed, exists := a.runtimeStepToolBindings[req.StepID]; exists {
			for _, toolName := range allowed {
				if toolName == req.ToolName {
					return nil
				}
			}
			return fmt.Errorf("tool %s is not Mapping-bound to runtime step %s", req.ToolName, req.StepID)
		}
		for _, step := range a.executionPlan.Steps {
			if step.StepID != req.StepID {
				continue
			}
			if step.ToolName != req.ToolName {
				return fmt.Errorf("tool %s is not the Mapping-bound tool for step %s", req.ToolName, req.StepID)
			}
			return nil
		}
		return fmt.Errorf("step %s is not present in the Mapping-bound execution plan", req.StepID)
	}

	// Compatibility callers may omit step_id only when the requested tool is
	// still present in the mapped plan. Production fixed plans always provide
	// step_id, including repeated uses of the same tool.
	for _, step := range a.executionPlan.Steps {
		if step.ToolName == req.ToolName {
			return nil
		}
	}
	return fmt.Errorf("tool %s is not present in the Mapping-bound execution plan", req.ToolName)
}

func (a *AssistantToolGatewayAdapter) reuseSuccessfulReadonlyToolCall(ctx context.Context, req agentruntime.ToolRequest, args map[string]interface{}, startedAt time.Time) (agentruntime.ToolResponse, bool) {
	if a.dispatcher == nil || a.dispatcher.toolCallRepo == nil || a.dispatcher.registry == nil {
		return agentruntime.ToolResponse{}, false
	}
	tool, ok := a.dispatcher.registry.Get(req.ToolName)
	if !ok || !canReuseAssistantToolResult(tool) {
		return agentruntime.ToolResponse{}, false
	}

	calls, _, err := a.dispatcher.toolCallRepo.ListBySession(ctx, a.sessionID, 1, 100)
	if err != nil {
		if a.logger != nil {
			a.logger.Debug("failed to inspect previous assistant tool calls for reuse",
				zap.String("session_id", a.sessionID),
				zap.String("tool_name", req.ToolName),
				zap.Error(err),
			)
		}
		return agentruntime.ToolResponse{}, false
	}

	currentArgs := canonicalToolArgs(args)
	for _, call := range calls {
		if call.MessageID != a.messageID ||
			call.ToolName != req.ToolName ||
			call.Status != model.ToolCallStatusSuccess ||
			canonicalToolArgs(unmarshalJSON(call.Args)) != currentArgs {
			continue
		}
		if a.logger != nil {
			a.logger.Info("reusing successful assistant tool call",
				zap.String("session_id", a.sessionID),
				zap.String("message_id", a.messageID),
				zap.String("tool_name", req.ToolName),
				zap.String("original_call_id", call.CallID),
				zap.String("runtime_call_id", req.CallID),
			)
		}
		return agentruntime.ToolResponse{
			CallID:    req.CallID,
			ToolName:  req.ToolName,
			Status:    agentruntime.ToolCallSuccess,
			Content:   string(call.Result),
			Summary:   "Reused the successful result from the same run.",
			StartedAt: startedAt,
			EndedAt:   time.Now(),
			Outcome:   normalizeToolOutcome(tool, unmarshalJSON(call.Result)),
		}, true
	}
	return agentruntime.ToolResponse{}, false
}

func canReuseAssistantToolResult(tool *ToolSpec) bool {
	if tool == nil {
		return false
	}
	if tool.ResultContract.OperationStatusField != "" && len(tool.ResultContract.PendingValues) > 0 {
		return false
	}
	// 状态工具的返回值会随后台任务推进而变化，同一轮内也不能复用旧结果。
	// 固定执行计划允许一个状态步骤多次轮询，因此所有 *.Status* 工具都
	// 必须直达真实处理器，不能被同消息内的成功缓存短路。
	if strings.Contains(strings.ToLower(tool.Name), ".status") {
		return false
	}
	if tool.Risk == ToolRiskReadonly {
		return true
	}
	return tool.Idempotent && (tool.Operation == OpList || tool.Operation == OpGet || tool.Operation == OpSearch)
}

func canonicalToolArgs(args map[string]interface{}) string {
	if args == nil {
		return "{}"
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (a *AssistantToolGatewayAdapter) waitApprovalAndExecute(ctx context.Context, req agentruntime.ToolRequest, approval *model.AssistantApproval, startedAt time.Time) (agentruntime.ToolResponse, error) {
	if a.runManager == nil {
		return agentruntime.ToolResponse{
			CallID:       req.CallID,
			ToolName:     req.ToolName,
			Status:       agentruntime.ToolCallFailed,
			Summary:      fmt.Sprintf("工具 %s 需要审批，但运行管理器不可用", req.ToolName),
			ErrorMessage: "approval manager unavailable",
			StartedAt:    startedAt,
			EndedAt:      time.Now(),
		}, nil
	}

	run, ok := a.runManager.Get(a.sessionID)
	if !ok {
		return agentruntime.ToolResponse{
			CallID:       req.CallID,
			ToolName:     req.ToolName,
			Status:       agentruntime.ToolCallFailed,
			Summary:      fmt.Sprintf("工具 %s 需要审批，但运行已结束", req.ToolName),
			ErrorMessage: "approval run unavailable",
			StartedAt:    startedAt,
			EndedAt:      time.Now(),
		}, nil
	}

	decision, err := run.WaitForApproval(ctx, approval.ApprovalID)
	if err != nil {
		errMsg := fmt.Sprintf("approval wait failed: %v", err)
		if a.onToolError != nil {
			a.onToolError(req.CallID, errMsg)
		}
		return agentruntime.ToolResponse{
			CallID:       req.CallID,
			ToolName:     req.ToolName,
			Status:       agentruntime.ToolCallFailed,
			Summary:      fmt.Sprintf("工具 %s 等待审批失败", req.ToolName),
			ErrorMessage: errMsg,
			StartedAt:    startedAt,
			EndedAt:      time.Now(),
		}, nil
	}

	if !decision.Approved {
		run.MarkApprovalRejected(decision)
		errMsg := "approval rejected"
		if decision.Comment != "" {
			errMsg = "approval rejected: " + decision.Comment
		}
		if a.onToolError != nil {
			a.onToolError(req.CallID, errMsg)
		}
		return agentruntime.ToolResponse{
			CallID:       req.CallID,
			ToolName:     req.ToolName,
			Status:       agentruntime.ToolCallFailed,
			Summary:      fmt.Sprintf("工具 %s 审批已拒绝", req.ToolName),
			ErrorMessage: errMsg,
			StartedAt:    startedAt,
			EndedAt:      time.Now(),
		}, nil
	}

	dispatchResult, err := a.dispatcher.ExecuteApprovedTool(ctx, approval.ApprovalID, decision.Operator)
	if err != nil {
		errMsg := fmt.Sprintf("approved tool execution failed: %v", err)
		_ = a.dispatcher.approvalGate.MarkFailed(context.Background(), approval.ApprovalID, errMsg)
		if updated, getErr := a.dispatcher.approvalGate.GetApproval(context.Background(), approval.ApprovalID); getErr == nil && a.onApprovalUpdated != nil {
			a.onApprovalUpdated(updated)
		}
		if a.onToolError != nil {
			a.onToolError(req.CallID, errMsg)
		}
		return agentruntime.ToolResponse{
			CallID:       req.CallID,
			ToolName:     req.ToolName,
			Status:       agentruntime.ToolCallFailed,
			Summary:      fmt.Sprintf("工具 %s 审批后执行失败", req.ToolName),
			ErrorMessage: errMsg,
			StartedAt:    startedAt,
			EndedAt:      time.Now(),
		}, nil
	}

	if dispatchResult.Success {
		_ = a.dispatcher.approvalGate.MarkExecuted(context.Background(), approval.ApprovalID)
	} else {
		_ = a.dispatcher.approvalGate.MarkFailed(context.Background(), approval.ApprovalID, dispatchResult.Error)
	}
	if updated, getErr := a.dispatcher.approvalGate.GetApproval(context.Background(), approval.ApprovalID); getErr == nil && a.onApprovalUpdated != nil {
		a.onApprovalUpdated(updated)
	}
	run.ClearWaitingApproval()

	if dispatchResult.Success {
		resultJSON, _ := json.Marshal(dispatchResult.Data)
		tool, _ := a.dispatcher.registry.Get(req.ToolName)
		outcome := normalizeToolOutcome(tool, dispatchResult.Data)
		a.logToolOutcome(req, outcome)
		if a.onToolResult != nil {
			a.onToolResult(dispatchResult.CallID, dispatchResult.Data, outcome)
		}
		return agentruntime.ToolResponse{
			CallID:    req.CallID,
			ToolName:  req.ToolName,
			Status:    agentruntime.ToolCallSuccess,
			Content:   string(resultJSON),
			Summary:   outcome.Message,
			StartedAt: startedAt,
			EndedAt:   time.Now(),
			Outcome:   outcome,
		}, nil
	}

	if a.onToolError != nil {
		a.onToolError(dispatchResult.CallID, dispatchResult.Error)
	}
	return agentruntime.ToolResponse{
		CallID:       req.CallID,
		ToolName:     req.ToolName,
		Status:       agentruntime.ToolCallFailed,
		ErrorMessage: dispatchResult.Error,
		Summary:      fmt.Sprintf("工具 %s 审批后执行失败", req.ToolName),
		StartedAt:    startedAt,
		EndedAt:      time.Now(),
	}, nil
}

func (a *AssistantToolGatewayAdapter) logToolOutcome(req agentruntime.ToolRequest, outcome *agentruntime.ToolOutcome) {
	if a == nil || a.logger == nil || outcome == nil {
		return
	}
	fields := []zap.Field{
		zap.String("session_id", a.sessionID),
		zap.String("run_id", a.runID),
		zap.String("step_id", req.StepID),
		zap.String("call_id", req.CallID),
		zap.String("tool_name", req.ToolName),
		zap.String("capability", outcome.Capability),
		zap.String("operation_status", string(outcome.OperationStatus)),
		zap.Bool("terminal", outcome.Terminal),
	}
	if !outcome.Terminal || outcome.OperationStatus == agentruntime.OperationFailed {
		a.logger.Info("assistant tool business outcome observed", fields...)
		return
	}
	a.logger.Debug("assistant tool business outcome observed", fields...)
}

// applyPlanArgs applies caller-authorized args. The exact step_id is preferred
// so repeated calls to the same tool keep independent values.
func (a *AssistantToolGatewayAdapter) applyPlanArgs(stepID, toolName string, args map[string]interface{}) map[string]interface{} {
	if a.executionPlan == nil || len(a.executionPlan.Steps) == 0 {
		return args
	}
	var matched *ToolPlanStep
	var toolMatches int
	for _, step := range a.executionPlan.Steps {
		if step.ToolName != toolName {
			continue
		}
		toolMatches++
		if stepID != "" && step.StepID == stepID {
			stepCopy := step
			matched = &stepCopy
			break
		}
		if matched == nil {
			stepCopy := step
			matched = &stepCopy
		}
	}
	if matched == nil || (toolMatches > 1 && (stepID == "" || matched.StepID != stepID)) {
		return args
	}
	if stepID != "" && matched.StepID != stepID && !runtimeStepAllowsTool(a.runtimeStepToolBindings, stepID, toolName) {
		return args
	}
	// A fixed step is already authorized and fully bound by the backend.
	// Returning only compiled arguments prevents legacy or invented model
	// fields from surviving into strict schema validation.
	compiled := make(map[string]interface{}, len(matched.Args))
	for key, value := range matched.Args {
		compiled[key] = value
	}
	return compiled
}

// captureStepOutcome records the operation and side-effect references extracted
// from a successful tool outcome, keyed by step_id. Async polls refresh the
// same entry until the operation reaches a terminal state; a previously
// captured reference is retained when a later poll omits it.
func (a *AssistantToolGatewayAdapter) captureStepOutcome(stepID, toolName string, outcome *agentruntime.ToolOutcome) {
	if a == nil || outcome == nil || strings.TrimSpace(stepID) == "" {
		return
	}
	captured := capturedStepOutcome{
		operationRef: outcome.OperationRef,
		sideEffects:  outcome.SideEffects,
		facts:        outcome.Facts,
		terminal:     outcome.Terminal,
	}
	a.outcomesMu.Lock()
	if existing, ok := a.priorStepOutcomes[stepID]; ok && len(captured.operationRef) == 0 {
		captured.operationRef = existing.operationRef
	}
	a.priorStepOutcomes[stepID] = captured
	a.outcomesMu.Unlock()
	if a.logger != nil {
		a.logger.Debug("assistant prior step outcome captured for binding",
			zap.String("session_id", a.sessionID),
			zap.String("run_id", a.runID),
			zap.String("step_id", stepID),
			zap.String("tool_name", toolName),
			zap.Bool("terminal", captured.terminal),
			zap.Strings("operation_ref_fields", refKeys(captured.operationRef)),
		)
	}
}

// resolvePreviousStepArgs deterministically binds arguments whose declared
// source is previous_step. It matches the argument name against operation and
// side-effect reference field names captured from prior step outcomes (most
// recent first). It returns the resolved args and, when a REQUIRED previous_step
// argument cannot be resolved, a non-empty skipReason so the caller can skip
// dispatch instead of failing schema validation on a missing argument.
func (a *AssistantToolGatewayAdapter) resolvePreviousStepArgs(stepID, toolName string, args map[string]interface{}) (map[string]interface{}, string) {
	if a == nil || a.executionPlan == nil || len(a.executionPlan.Steps) == 0 {
		return args, ""
	}
	step := a.findPlanStep(stepID, toolName)
	if step == nil || len(step.ArgSources) == 0 {
		return args, ""
	}
	tool, _ := a.dispatcher.registry.Get(toolName)
	requiredSet := make(map[string]bool)
	if tool != nil {
		for _, name := range requiredArgsFromToolSchema(tool.ArgsSchema) {
			requiredSet[name] = true
		}
	}

	resolved := cloneInvocationArgs(args)
	includeCurrentOutcome := step.StepID != stepID &&
		runtimeStepAllowsTool(a.runtimeStepToolBindings, stepID, toolName)
	priorOutcomes := a.orderedPriorOutcomes(stepID, includeCurrentOutcome)
	var missingRequired []string
	for argName, source := range step.ArgSources {
		if !strings.EqualFold(strings.TrimSpace(source.SourceType), "previous_step") {
			continue
		}
		// A previous_step argument is deterministically bound from the prior
		// step's outcome. When a producer outcome is available it ALWAYS takes
		// precedence over any model-supplied value, so the model can never
		// regenerate or invent an operation/scan/query/task reference.
		if value, ok := resolveRefFromOutcomes(priorOutcomes, argName); ok {
			resolved[argName] = value
			if a.logger != nil {
				a.logger.Debug("assistant previous_step argument bound from prior step",
					zap.String("session_id", a.sessionID),
					zap.String("run_id", a.runID),
					zap.String("step_id", stepID),
					zap.String("tool_name", toolName),
					zap.String("arg_name", argName),
				)
			}
			continue
		}
		// Fallback for *_ids arguments: aggregate entity IDs from prior step
		// facts (e.g. host_resolved facts[].id -> host_ids). This lets the
		// asset_inventory compiler bind host_ids from a Host.Resolve outcome
		// without the model regenerating UUIDs.
		if ids, ok := resolveEntityIDsFromFacts(priorOutcomes, argName); ok {
			resolved[argName] = ids
			if a.logger != nil {
				a.logger.Debug("assistant previous_step ids argument bound from prior step facts",
					zap.String("session_id", a.sessionID),
					zap.String("run_id", a.runID),
					zap.String("step_id", stepID),
					zap.String("tool_name", toolName),
					zap.String("arg_name", argName),
					zap.Int("id_count", len(ids)),
				)
			}
			continue
		}
		// No producer produced this reference. A required previous_step
		// argument must not fall back to a model-invented value; skip the step
		// so the gap is observable instead of silently dispatching bad input.
		if requiredSet[argName] {
			missingRequired = append(missingRequired, argName)
		}
	}
	if len(missingRequired) == 0 {
		return resolved, ""
	}
	sort.Strings(missingRequired)
	return resolved, "missing previous_step argument(s): " + strings.Join(missingRequired, ", ")
}

// findPlanStep returns the Mapping-bound plan step for the given step_id (and
// tool name when available). The runtime always supplies step_id for fixed
// plans, so an exact step_id match is preferred; tool name is a fallback for
// compatibility callers that omit it.
func (a *AssistantToolGatewayAdapter) findPlanStep(stepID, toolName string) *ToolPlanStep {
	if a.executionPlan == nil {
		return nil
	}
	if strings.TrimSpace(stepID) != "" {
		for _, step := range a.executionPlan.Steps {
			if step.StepID == stepID {
				if toolName == "" || step.ToolName == toolName {
					stepCopy := step
					return &stepCopy
				}
			}
		}
	}
	for _, step := range a.executionPlan.Steps {
		if toolName != "" && step.ToolName == toolName {
			stepCopy := step
			return &stepCopy
		}
	}
	return nil
}

// orderedPriorOutcomes returns captured outcomes in Mapping order. The current
// runtime step is included only for an explicitly mapped completion tool that
// shares its asynchronous producer's backend-authorized runtime step.
func (a *AssistantToolGatewayAdapter) orderedPriorOutcomes(currentStepID string, includeCurrent bool) []capturedStepOutcome {
	if a == nil || a.executionPlan == nil {
		return nil
	}
	a.outcomesMu.Lock()
	defer a.outcomesMu.Unlock()
	var ordered []capturedStepOutcome
	for _, step := range a.executionPlan.Steps {
		if step.StepID == currentStepID && !includeCurrent {
			continue
		}
		if outcome, ok := a.priorStepOutcomes[step.StepID]; ok {
			ordered = append(ordered, outcome)
		}
	}
	return ordered
}

func cloneRuntimeStepToolBindings(source map[string][]string) map[string][]string {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string][]string, len(source))
	for stepID, tools := range source {
		cloned[stepID] = append([]string{}, tools...)
	}
	return cloned
}

func runtimeStepAllowsTool(bindings map[string][]string, stepID, toolName string) bool {
	for _, candidate := range bindings[stepID] {
		if candidate == toolName {
			return true
		}
	}
	return false
}

// resolveRefFromOutcomes searches captured prior step outcomes (most recent
// first) for an operation or side-effect reference whose field name matches
// argName. The discriminator "type" key is ignored.
func resolveRefFromOutcomes(outcomes []capturedStepOutcome, argName string) (string, bool) {
	for i := len(outcomes) - 1; i >= 0; i-- {
		outcome := outcomes[i]
		if outcome.operationRef != nil {
			if value, ok := outcome.operationRef[argName]; ok && value != "" {
				return value, true
			}
		}
		for _, sideEffect := range outcome.sideEffects {
			if value, ok := sideEffect[argName]; ok {
				if s, _ := value.(string); s != "" {
					return s, true
				}
			}
		}
	}
	return "", false
}

// resolveEntityIDsFromFacts aggregates entity IDs from prior step outcome facts.
// For a *_ids argument (e.g. host_ids) it looks for facts whose kind matches
// the entity resolution pattern (e.g. host_resolved) and collects their id
// fields. This lets the asset_inventory compiler bind host_ids from a
// Host.Resolve outcome without the model regenerating UUIDs.
func resolveEntityIDsFromFacts(outcomes []capturedStepOutcome, argName string) ([]string, bool) {
	entity := strings.TrimSuffix(strings.ToLower(argName), "_ids")
	if entity == argName || entity == "" {
		return nil, false
	}
	factKind := entity + "_resolved"
	var ids []string
	for _, outcome := range outcomes {
		for _, fact := range outcome.facts {
			if fact == nil {
				continue
			}
			if !strings.EqualFold(stringValue(fact["kind"]), factKind) {
				continue
			}
			if id := stringValue(fact["id"]); id != "" {
				ids = append(ids, id)
			}
		}
	}
	return dedupeStrings(ids), len(ids) > 0
}

func refKeys(ref map[string]string) []string {
	if len(ref) == 0 {
		return nil
	}
	keys := make([]string, 0, len(ref))
	for key := range ref {
		if key == "type" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func resultDataItems(result map[string]interface{}) []map[string]interface{} {
	raw, ok := result["data"].([]interface{})
	if !ok {
		return nil
	}
	items := make([]map[string]interface{}, 0, len(raw))
	for _, value := range raw {
		if item, ok := value.(map[string]interface{}); ok {
			items = append(items, item)
		}
	}
	return items
}

func numericValue(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func stringValue(value interface{}) string {
	if typed, ok := value.(string); ok {
		return strings.TrimSpace(typed)
	}
	return ""
}

// Cancel 实现 agentruntime.ToolGateway 接口（同步执行，无需取消）
func (a *AssistantToolGatewayAdapter) Cancel(_ context.Context, _ string, _ string) error {
	return nil
}
