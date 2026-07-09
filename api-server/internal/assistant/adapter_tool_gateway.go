package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"api-server/internal/model"
	agentruntime "github.com/alex-chenc/agent-runtime"
	"go.uber.org/zap"
)

// AssistantToolGatewayAdapter 适配 agent-runtime ToolGateway 接口
// 将 agent-runtime 的工具调用桥接到 assistant 的 ToolDispatcher
type AssistantToolGatewayAdapter struct {
	dispatcher *ToolDispatcher
	sessionID  string
	messageID  string
	runID      string
	operator   string
	logger     *zap.Logger
	runManager *RunManager

	// executionPlan is retained for non-Assistant compatibility callers that
	// explicitly supply a fixed plan. Pure-agent Assistant runs always leave it nil.
	executionPlan *ToolExecutionPlan

	// 回调函数，用于 SSE 事件推送
	onToolCall        func(callID, toolName string, args interface{})
	onToolResult      func(callID string, result interface{}, outcome *agentruntime.ToolOutcome)
	onToolError       func(callID, errMsg string)
	onApproval        func(approval interface{})
	onApprovalUpdated func(approval interface{})
}

// AssistantToolGatewayConfig 适配器配置
type AssistantToolGatewayConfig struct {
	Dispatcher        *ToolDispatcher
	SessionID         string
	MessageID         string
	RunID             string
	Operator          string
	Logger            *zap.Logger
	RunManager        *RunManager
	UserInput         string
	ContextRefs       []ContextRefResult
	ExecutionPlan     *ToolExecutionPlan
	OnToolCall        func(callID, toolName string, args interface{})
	OnToolResult      func(callID string, result interface{}, outcome *agentruntime.ToolOutcome)
	OnToolError       func(callID, errMsg string)
	OnApproval        func(approval interface{})
	OnApprovalUpdated func(approval interface{})
}

// NewAssistantToolGatewayAdapter 创建适配器
func NewAssistantToolGatewayAdapter(cfg AssistantToolGatewayConfig) *AssistantToolGatewayAdapter {
	return &AssistantToolGatewayAdapter{
		dispatcher:        cfg.Dispatcher,
		sessionID:         cfg.SessionID,
		messageID:         cfg.MessageID,
		runID:             cfg.RunID,
		operator:          cfg.Operator,
		logger:            cfg.Logger,
		runManager:        cfg.RunManager,
		executionPlan:     cfg.ExecutionPlan,
		onToolCall:        cfg.OnToolCall,
		onToolResult:      cfg.OnToolResult,
		onToolError:       cfg.OnToolError,
		onApproval:        cfg.OnApproval,
		onApprovalUpdated: cfg.OnApprovalUpdated,
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

	if cachedResp, ok := a.reuseSuccessfulReadonlyToolCall(ctx, req, args, startedAt); ok {
		return cachedResp, nil
	}

	// 通知工具调用开始
	if a.onToolCall != nil {
		a.onToolCall(req.CallID, req.ToolName, args)
	}

	// 通过 ToolDispatcher 调度执行
	result, err := a.dispatcher.Dispatch(ctx, DispatchRequest{
		SessionID: a.sessionID,
		MessageID: a.messageID,
		RunID:     a.runID,
		CallID:    req.CallID,
		ToolName:  req.ToolName,
		Args:      args,
		Operator:  a.operator,
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
		a.logToolOutcome(req, outcome)
		if a.onToolResult != nil {
			a.onToolResult(result.CallID, result.Data, outcome)
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

// Prepare implements agentruntime.ToolRequestPreparer. It only applies
// caller-supplied compatibility plan args before schema validation. Pure-agent
// mode does not pass such a plan, so Runtime remains responsible for deriving
// every business argument from user context and observed tool results.
func (a *AssistantToolGatewayAdapter) Prepare(ctx context.Context, req agentruntime.ToolRequest) (agentruntime.ToolRequest, error) {
	_ = ctx
	args := make(map[string]interface{}, len(req.Args)+4)
	for key, value := range req.Args {
		args[key] = value
	}
	args = a.applyPlanArgs(req.StepID, req.ToolName, args)

	if req.Context == nil {
		req.Context = make(map[string]string)
	}
	req.Context["aegis_prepared"] = "true"
	req.Args = args
	return req, nil
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
	for _, step := range a.executionPlan.Steps {
		if step.ToolName != toolName || len(step.Args) == 0 {
			continue
		}
		if stepID != "" && step.StepID == stepID {
			stepCopy := step
			matched = &stepCopy
			break
		}
		if stepID == "" {
			if matched != nil {
				// Ambiguous legacy request: do not select an arbitrary repeated
				// step. agent-runtime always supplies step_id for fixed plans.
				return args
			}
			stepCopy := step
			matched = &stepCopy
		}
	}
	if matched == nil {
		return args
	}
	for key, value := range matched.Args {
		args[key] = value
	}
	return args
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
