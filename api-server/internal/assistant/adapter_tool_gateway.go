package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"api-server/internal/model"
	agentruntime "github.com/alex-chenc/agent-runtime"
	"go.uber.org/zap"
)

// AssistantToolGatewayAdapter 适配 agent-runtime ToolGateway 接口
// 将 agent-runtime 的工具调用桥接到 assistant 的 ToolDispatcher
type AssistantToolGatewayAdapter struct {
	dispatcher  *ToolDispatcher
	sessionID   string
	messageID   string
	runID       string
	operator    string
	logger      *zap.Logger
	runManager  *RunManager
	userInput   string
	contextRefs []ContextRefResult

	// executionPlan 来自 ToolDecisionEngine.Decide()，包含预绑定参数。
	// 当 LLM 调用工具时，计划中的预绑定参数作为默认值合并（LLM 参数优先）。
	executionPlan *ToolExecutionPlan

	// 回调函数，用于 SSE 事件推送
	onToolCall        func(callID, toolName string, args interface{})
	onToolResult      func(callID string, result interface{})
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
	OnToolResult      func(callID string, result interface{})
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
		userInput:         cfg.UserInput,
		contextRefs:       cfg.ContextRefs,
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

	// 解析参数
	args := make(map[string]interface{})
	if req.Args != nil {
		for k, v := range req.Args {
			args[k] = v
		}
	}
	args = a.normalizeBaselineToolArgs(req.ToolName, args)
	args = a.applyPlanArgs(req.ToolName, args)

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
		if a.onToolResult != nil {
			a.onToolResult(result.CallID, result.Data)
		}
		return agentruntime.ToolResponse{
			CallID:    req.CallID,
			ToolName:  req.ToolName,
			Status:    agentruntime.ToolCallSuccess,
			Content:   string(resultJSON),
			Summary:   fmt.Sprintf("工具 %s 执行成功", req.ToolName),
			StartedAt: startedAt,
			EndedAt:   endedAt,
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
			Summary:   fmt.Sprintf("工具 %s 已复用本轮会话中相同参数的成功结果", req.ToolName),
			StartedAt: startedAt,
			EndedAt:   time.Now(),
		}, true
	}
	return agentruntime.ToolResponse{}, false
}

func canReuseAssistantToolResult(tool *ToolSpec) bool {
	if tool == nil {
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

func (a *AssistantToolGatewayAdapter) normalizeBaselineToolArgs(toolName string, args map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{}, len(args)+3)
	for k, v := range args {
		normalized[k] = v
	}

	defaults := a.baselineDefaults()
	switch toolName {
	case "Baseline.Template.Status.Get", "Baseline.Template.Rules.List":
		if getStringArgFromMap(normalized, "template_id") == "" && defaults["template_id"] != "" {
			normalized["template_id"] = defaults["template_id"]
		}
	case "Baseline.Script.Generate":
		if getStringArgFromMap(normalized, "template_id") == "" && defaults["template_id"] != "" {
			normalized["template_id"] = defaults["template_id"]
		}
		if _, ok := normalized["rule_ids"]; !ok {
			if ruleID := getStringArgFromMap(normalized, "rule_id"); ruleID != "" {
				normalized["rule_ids"] = []string{ruleID}
			} else if defaults["rule_id"] != "" && getStringArgFromMap(normalized, "template_id") == "" {
				normalized["rule_ids"] = []string{defaults["rule_id"]}
			}
		}
	case "Task.RunCheck", "Task.RunFix":
		if _, ok := normalized["rule_ids"]; !ok {
			if ruleID := getStringArgFromMap(normalized, "rule_id"); ruleID != "" {
				normalized["rule_ids"] = []string{ruleID}
			} else if defaults["rule_id"] != "" {
				normalized["rule_ids"] = []string{defaults["rule_id"]}
			}
		}
		if _, ok := normalized["host_ids"]; !ok {
			if hostID := getStringArgFromMap(normalized, "host_id"); hostID != "" {
				normalized["host_ids"] = []string{hostID}
			} else if defaults["host_id"] != "" {
				normalized["host_ids"] = []string{defaults["host_id"]}
			}
		}
	}
	return normalized
}

func (a *AssistantToolGatewayAdapter) baselineDefaults() map[string]string {
	defaults := map[string]string{
		"template_id": extractNamedID(a.userInput, "template_id"),
		"rule_id":     extractNamedID(a.userInput, "rule_id"),
		"host_id":     extractNamedID(a.userInput, "host_id"),
	}
	for _, ref := range a.contextRefs {
		if defaults["template_id"] == "" && ref.ObjectType == "baseline_template" {
			defaults["template_id"] = strings.TrimSpace(ref.ObjectID)
		}
		if defaults["rule_id"] == "" && ref.ObjectType == "baseline_rule" {
			defaults["rule_id"] = strings.TrimSpace(ref.ObjectID)
		}
		if defaults["host_id"] == "" && ref.ObjectType == "host" {
			defaults["host_id"] = strings.TrimSpace(ref.ObjectID)
		}
	}
	return defaults
}

func extractNamedID(message, key string) string {
	re := regexp.MustCompile(regexp.QuoteMeta(key) + `\s*=\s*([0-9a-fA-F-]{36}|[^\s，,。；;]+)`)
	match := re.FindStringSubmatch(message)
	if len(match) < 2 {
		return ""
	}
	return strings.Trim(match[1], " \t\r\n，,。；;\"'")
}

func extractNamedValues(message, key string) []string {
	re := regexp.MustCompile(regexp.QuoteMeta(key) + `\s*=\s*([0-9a-fA-F-]{36}|[^\s，,。；;]+)`)
	matches := re.FindAllStringSubmatch(message, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		value := strings.Trim(match[1], " \t\r\n，,。；;\"'")
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func getStringArgFromMap(args map[string]interface{}, key string) string {
	if args == nil {
		return ""
	}
	switch v := args[key].(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func getStringSliceArgFromMap(args map[string]interface{}, key string) []string {
	if args == nil {
		return nil
	}
	switch v := args[key].(type) {
	case []string:
		return cleanStringSlice(v)
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				result = append(result, strings.TrimSpace(s))
			}
		}
		return result
	default:
		return nil
	}
}

func cleanStringSlice(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, strings.TrimSpace(value))
		}
	}
	return result
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
		if a.onToolResult != nil {
			a.onToolResult(dispatchResult.CallID, dispatchResult.Data)
		}
		return agentruntime.ToolResponse{
			CallID:    req.CallID,
			ToolName:  req.ToolName,
			Status:    agentruntime.ToolCallSuccess,
			Content:   string(resultJSON),
			Summary:   fmt.Sprintf("工具 %s 审批后执行成功", req.ToolName),
			StartedAt: startedAt,
			EndedAt:   time.Now(),
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

// applyPlanArgs merges pre-bound args from the ToolExecutionPlan as defaults.
// LLM-provided args take priority; plan args fill in missing keys only.
func (a *AssistantToolGatewayAdapter) applyPlanArgs(toolName string, args map[string]interface{}) map[string]interface{} {
	if a.executionPlan == nil || len(a.executionPlan.Steps) == 0 {
		return args
	}
	for _, step := range a.executionPlan.Steps {
		if step.ToolName != toolName || len(step.Args) == 0 {
			continue
		}
		for k, v := range step.Args {
			if _, exists := args[k]; !exists {
				args[k] = v
			}
		}
		break
	}
	return args
}

// Cancel 实现 agentruntime.ToolGateway 接口（同步执行，无需取消）
func (a *AssistantToolGatewayAdapter) Cancel(_ context.Context, _ string, _ string) error {
	return nil
}
