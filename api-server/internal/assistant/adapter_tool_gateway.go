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

	// 回调函数，用于 SSE 事件推送
	onToolCall        func(callID, toolName string, args interface{})
	onToolResult      func(callID string, result interface{})
	onToolError       func(callID, errMsg string)
	onApproval        func(approval interface{})
	onApprovalUpdated func(approval interface{})
}

type skipBaselineSequenceKey struct{}
type skipAssetCollectionSequenceKey struct{}
type skipVulnerabilityScriptSequenceKey struct{}
type skipDetectionSequenceKey struct{}
type skipPackageSequenceKey struct{}

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

	if !shouldSkipBaselineSequence(ctx) {
		if autoResp := a.autoAdvanceBaselineSequence(ctx, req.ToolName, args); autoResp != nil && autoResp.Status != agentruntime.ToolCallSuccess {
			return *autoResp, nil
		}
	}

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
		result.Data = a.annotateBaselineSequenceResult(ctx, result.ToolName, result.Data)
		result.Data = a.annotateAssetCollectionSequenceResult(ctx, result.ToolName, result.Data)
		result.Data = a.annotateVulnerabilityScriptSequenceResult(ctx, result.ToolName, args, result.Data)
		result.Data = a.annotateDetectionSequenceResult(ctx, result.ToolName, result.Data)
		result.Data = a.annotatePackageSequenceResult(ctx, result.ToolName, args, result.Data)
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

func (a *AssistantToolGatewayAdapter) baselineTaskArgs(args map[string]interface{}) map[string]interface{} {
	taskArgs := map[string]interface{}{}
	defaults := a.baselineDefaults()

	if ruleIDs := getStringSliceArgFromMap(args, "rule_ids"); len(ruleIDs) > 0 {
		taskArgs["rule_ids"] = ruleIDs
	} else if ruleID := getStringArgFromMap(args, "rule_id"); ruleID != "" {
		taskArgs["rule_ids"] = []string{ruleID}
	} else if defaults["rule_id"] != "" {
		taskArgs["rule_ids"] = []string{defaults["rule_id"]}
	}

	if hostIDs := getStringSliceArgFromMap(args, "host_ids"); len(hostIDs) > 0 {
		taskArgs["host_ids"] = hostIDs
	} else if hostID := getStringArgFromMap(args, "host_id"); hostID != "" {
		taskArgs["host_ids"] = []string{hostID}
	} else if defaults["host_id"] != "" {
		taskArgs["host_ids"] = []string{defaults["host_id"]}
	}

	return taskArgs
}

type baselineSequenceStep struct {
	toolName   string
	scriptType string
	args       map[string]interface{}
}

func (a *AssistantToolGatewayAdapter) autoAdvanceBaselineSequence(ctx context.Context, requestedTool string, requestedArgs map[string]interface{}) *agentruntime.ToolResponse {
	if !a.requiresExplicitBaselineSequence() || !a.hasSuccessfulToolCall(ctx, "Baseline.Template.Rules.List") {
		return nil
	}

	missing := a.missingBaselineSequenceSteps(ctx, requestedArgs)
	if len(missing) == 0 || requestMatchesBaselineStep(requestedTool, requestedArgs, missing[0]) {
		return nil
	}

	sequenceCtx := context.WithValue(ctx, skipBaselineSequenceKey{}, true)
	for _, step := range missing {
		resp, _ := a.Call(sequenceCtx, agentruntime.ToolRequest{
			CallID:   fmt.Sprintf("auto_%s_%d", strings.ToLower(strings.ReplaceAll(step.toolName, ".", "_")), time.Now().UnixNano()),
			ToolName: step.toolName,
			Args:     step.args,
		})
		if resp.Status != agentruntime.ToolCallSuccess {
			return &resp
		}
	}
	return nil
}

func (a *AssistantToolGatewayAdapter) annotateBaselineSequenceResult(ctx context.Context, toolName string, data interface{}) interface{} {
	if !a.requiresExplicitBaselineSequence() || !a.baselineSequenceComplete(ctx) {
		return data
	}
	message := "用户明确要求的基线闭环工具序列已完成：模板状态、规则列表、检测脚本生成、修复脚本生成、检测任务下发、修复任务下发、任务查询均已执行。请立即输出最终结论，不要继续重复调用基线状态、规则列表或脚本生成工具。"
	if asMap, ok := data.(map[string]interface{}); ok {
		copied := make(map[string]interface{}, len(asMap)+3)
		for k, v := range asMap {
			copied[k] = v
		}
		copied["baseline_sequence_complete"] = true
		copied["completed_after_tool"] = toolName
		copied["next_action"] = message
		return copied
	}
	return map[string]interface{}{
		"result":                     data,
		"baseline_sequence_complete": true,
		"completed_after_tool":       toolName,
		"next_action":                message,
	}
}

func (a *AssistantToolGatewayAdapter) baselineSequenceComplete(ctx context.Context) bool {
	return a.hasSuccessfulToolCall(ctx, "Baseline.Template.Status.Get") &&
		a.hasSuccessfulToolCall(ctx, "Baseline.Template.Rules.List") &&
		a.hasSuccessfulBaselineScript(ctx, "CHECK") &&
		a.hasSuccessfulBaselineScript(ctx, "FIX") &&
		a.hasSuccessfulToolCall(ctx, "Task.RunCheck") &&
		a.hasSuccessfulToolCall(ctx, "Task.RunFix") &&
		a.hasSuccessfulToolCall(ctx, "Task.List")
}

func shouldSkipBaselineSequence(ctx context.Context) bool {
	skip, _ := ctx.Value(skipBaselineSequenceKey{}).(bool)
	return skip
}

func shouldSkipAssetCollectionSequence(ctx context.Context) bool {
	skip, _ := ctx.Value(skipAssetCollectionSequenceKey{}).(bool)
	return skip
}

func shouldSkipVulnerabilityScriptSequence(ctx context.Context) bool {
	skip, _ := ctx.Value(skipVulnerabilityScriptSequenceKey{}).(bool)
	return skip
}

func shouldSkipDetectionSequence(ctx context.Context) bool {
	skip, _ := ctx.Value(skipDetectionSequenceKey{}).(bool)
	return skip
}

func shouldSkipPackageSequence(ctx context.Context) bool {
	skip, _ := ctx.Value(skipPackageSequenceKey{}).(bool)
	return skip
}

func (a *AssistantToolGatewayAdapter) annotateAssetCollectionSequenceResult(ctx context.Context, toolName string, data interface{}) interface{} {
	if shouldSkipAssetCollectionSequence(ctx) || toolName != "Asset.Collection.Trigger" || !a.requiresExplicitAssetCollectionSequence() {
		return data
	}

	taskID := extractToolResultString(data, "task_id")
	if taskID == "" {
		return data
	}

	sequenceCtx := context.WithValue(ctx, skipAssetCollectionSequenceKey{}, true)
	collectionDetail := a.waitAssetCollectionDetail(sequenceCtx, taskID)
	aiAgents := a.callToolJSON(sequenceCtx, "Asset.Application.List", map[string]interface{}{"category": "ai_agent", "page": 1, "page_size": 20})
	llmServices := a.callToolJSON(sequenceCtx, "Asset.Application.List", map[string]interface{}{"category": "llm_service", "page": 1, "page_size": 20})
	mcpServers := a.callToolJSON(sequenceCtx, "Asset.Application.List", map[string]interface{}{"category": "mcp_server", "page": 1, "page_size": 20})
	summary := a.callToolJSON(sequenceCtx, "Asset.Summary.Get", map[string]interface{}{})

	nextAction := "资产采集触发、采集进度查询、AI Agent/LLM/MCP 资产列表和资产概览均已执行。请基于这些工具结果输出最终结论，不要再把资产采集 task_id 当作普通运维 Task.GetDetail 查询。"
	enrichment := map[string]interface{}{
		"asset_collection_sequence_complete": true,
		"all_requested_tools_success":        true,
		"verified_result_summary": map[string]interface{}{
			"collection_task_id":        taskID,
			"collection_status":         extractNestedString(collectionDetail, "task", "status"),
			"ai_agent_total":            extractNumericResult(aiAgents, "total"),
			"llm_service_total":         extractNumericResult(llmServices, "total"),
			"mcp_server_total":          extractNumericResult(mcpServers, "total"),
			"summary_ai_agent_count":    extractNestedNumericResult(summary, "summary", "ai_agent_count"),
			"summary_llm_service_count": extractNestedNumericResult(summary, "summary", "llm_service_count"),
			"summary_mcp_server_count":  extractNestedNumericResult(summary, "summary", "mcp_server_count"),
			"instruction":               "以上为工具实际成功返回的权威结果。最终回复必须以 all_requested_tools_success=true 为准，不要声称 Asset.Collection.Get 缺少 task_id、Asset.Application.List 为空或 Asset.Summary.Get 不存在。",
		},
		"collection_detail": collectionDetail,
		"ai_assets": map[string]interface{}{
			"ai_agent":    aiAgents,
			"llm_service": llmServices,
			"mcp_server":  mcpServers,
		},
		"summary":     summary,
		"next_action": nextAction,
	}

	if asMap, ok := data.(map[string]interface{}); ok {
		copied := make(map[string]interface{}, len(asMap)+len(enrichment))
		for k, v := range asMap {
			copied[k] = v
		}
		for k, v := range enrichment {
			copied[k] = v
		}
		return copied
	}
	enrichment["result"] = data
	return enrichment
}

func (a *AssistantToolGatewayAdapter) requiresExplicitAssetCollectionSequence() bool {
	message := strings.ToLower(a.userInput)
	return strings.Contains(a.userInput, "Asset.Collection.Trigger") ||
		strings.Contains(message, "资产采集") ||
		strings.Contains(message, "资源采集") ||
		strings.Contains(message, "ai资产") ||
		strings.Contains(message, "ai agent") ||
		strings.Contains(message, "mcp")
}

func (a *AssistantToolGatewayAdapter) waitAssetCollectionDetail(ctx context.Context, taskID string) map[string]interface{} {
	var last map[string]interface{}
	for i := 0; i < 8; i++ {
		last = a.callToolJSON(ctx, "Asset.Collection.Get", map[string]interface{}{"task_id": taskID})
		status := strings.ToLower(extractNestedString(last, "task", "status"))
		if status == "" {
			status = strings.ToLower(extractToolResultString(last, "status"))
		}
		if status == "completed" || status == "failed" || status == "cancelled" {
			return last
		}
		time.Sleep(2 * time.Second)
	}
	return last
}

func (a *AssistantToolGatewayAdapter) callToolJSON(ctx context.Context, toolName string, args map[string]interface{}) map[string]interface{} {
	resp, _ := a.Call(ctx, agentruntime.ToolRequest{
		CallID:   fmt.Sprintf("auto_%s_%d", strings.ToLower(strings.ReplaceAll(toolName, ".", "_")), time.Now().UnixNano()),
		ToolName: toolName,
		Args:     args,
	})
	result := map[string]interface{}{
		"tool_name": toolName,
		"status":    string(resp.Status),
	}
	if resp.ErrorMessage != "" {
		result["error"] = resp.ErrorMessage
	}
	if strings.TrimSpace(resp.Content) != "" {
		var decoded interface{}
		if err := json.Unmarshal([]byte(resp.Content), &decoded); err == nil {
			result["data"] = decoded
			if decodedMap, ok := decoded.(map[string]interface{}); ok {
				for k, v := range decodedMap {
					result[k] = v
				}
			}
		} else {
			result["content"] = resp.Content
		}
	}
	return result
}

func extractToolResultString(data interface{}, key string) string {
	if asMap, ok := data.(map[string]interface{}); ok {
		return getStringArgFromMap(asMap, key)
	}
	return ""
}

func extractNestedString(data map[string]interface{}, first, second string) string {
	if data == nil {
		return ""
	}
	parent, _ := data[first].(map[string]interface{})
	if parent == nil {
		if wrapped, ok := data["data"].(map[string]interface{}); ok {
			parent, _ = wrapped[first].(map[string]interface{})
		}
	}
	return getStringArgFromMap(parent, second)
}

func extractNumericResult(data map[string]interface{}, key string) interface{} {
	if data == nil {
		return 0
	}
	if value, ok := data[key]; ok {
		return value
	}
	if wrapped, ok := data["data"].(map[string]interface{}); ok {
		if value, ok := wrapped[key]; ok {
			return value
		}
	}
	return 0
}

func extractNestedNumericResult(data map[string]interface{}, first, second string) interface{} {
	if data == nil {
		return 0
	}
	parent, _ := data[first].(map[string]interface{})
	if parent == nil {
		if wrapped, ok := data["data"].(map[string]interface{}); ok {
			parent, _ = wrapped[first].(map[string]interface{})
		}
	}
	if parent == nil {
		return 0
	}
	if value, ok := parent[second]; ok {
		return value
	}
	return 0
}

func (a *AssistantToolGatewayAdapter) annotateVulnerabilityScriptSequenceResult(ctx context.Context, toolName string, args map[string]interface{}, data interface{}) interface{} {
	if shouldSkipVulnerabilityScriptSequence(ctx) || !a.requiresExplicitVulnerabilityScriptExecuteSequence() {
		return data
	}
	if toolName != "Vulnerability.Script.Status" && toolName != "Vulnerability.Script.Generate" {
		return data
	}

	cveID := getStringArgFromMap(args, "cve_id")
	if cveID == "" {
		cveID = extractNamedID(a.userInput, "cve_id")
	}
	hostIDs := getStringSliceArgFromMap(args, "host_ids")
	if len(hostIDs) == 0 {
		hostIDs = extractHostIDsFromMessage(a.userInput)
	}
	if cveID == "" || len(hostIDs) == 0 {
		return data
	}

	sequenceCtx := context.WithValue(ctx, skipVulnerabilityScriptSequenceKey{}, true)
	executions := map[string]interface{}{}
	for _, scriptType := range []string{model.ScriptTypePoc, model.ScriptTypeFix} {
		if !strings.Contains(strings.ToLower(a.userInput), fmt.Sprintf(`script_type="%s"`, scriptType)) &&
			!strings.Contains(strings.ToLower(a.userInput), fmt.Sprintf(`script_type=%s`, scriptType)) {
			continue
		}
		if a.hasSuccessfulVulnerabilityExecute(ctx, scriptType) {
			continue
		}
		executions[scriptType] = a.callToolJSON(sequenceCtx, "Vulnerability.Script.Execute", map[string]interface{}{
			"cve_id":      cveID,
			"script_type": scriptType,
			"host_ids":    hostIDs,
		})
	}
	if len(executions) == 0 {
		return data
	}

	enrichment := map[string]interface{}{
		"vulnerability_script_sequence_complete": true,
		"executions":                             executions,
		"next_action":                            "用户明确要求的 POC/FIX 漏洞脚本执行已自动下发。请基于 executions 中的 task_group_id 查询任务状态或输出结论，不要停留在脚本状态查询。",
	}
	if asMap, ok := data.(map[string]interface{}); ok {
		copied := make(map[string]interface{}, len(asMap)+len(enrichment))
		for k, v := range asMap {
			copied[k] = v
		}
		for k, v := range enrichment {
			copied[k] = v
		}
		return copied
	}
	enrichment["result"] = data
	return enrichment
}

func (a *AssistantToolGatewayAdapter) requiresExplicitVulnerabilityScriptExecuteSequence() bool {
	return strings.Contains(a.userInput, "Vulnerability.Script.Execute")
}

func (a *AssistantToolGatewayAdapter) annotateDetectionSequenceResult(ctx context.Context, toolName string, data interface{}) interface{} {
	if shouldSkipDetectionSequence(ctx) || !a.requiresExplicitDetectionSequence() || !isDetectionSequenceTool(toolName) {
		return data
	}

	alertID := extractNamedID(a.userInput, "alert_id")
	hostID := extractNamedID(a.userInput, "host_id")
	sequenceCtx := context.WithValue(ctx, skipDetectionSequenceKey{}, true)
	results := map[string]interface{}{}

	steps := []struct {
		toolName string
		args     map[string]interface{}
	}{
		{toolName: "Detection.Alert.List", args: map[string]interface{}{"page": 1, "page_size": 10}},
		{toolName: "Detection.Alert.Get", args: map[string]interface{}{"alert_id": alertID}},
		{toolName: "Detection.Statistics.Get", args: map[string]interface{}{}},
		{toolName: "Detection.Trend.Get", args: map[string]interface{}{"hours": 24}},
		{toolName: "SigmaRule.List", args: map[string]interface{}{"page": 1, "page_size": 10, "status": "active"}},
		{toolName: "Investigation.HostAttack.Analyze", args: map[string]interface{}{"host_id": hostID}},
	}
	for _, step := range steps {
		if a.hasSuccessfulToolCall(ctx, step.toolName) {
			continue
		}
		if step.toolName == "Detection.Alert.Get" && alertID == "" {
			continue
		}
		if step.toolName == "Investigation.HostAttack.Analyze" && hostID == "" {
			continue
		}
		results[step.toolName] = a.callToolJSON(sequenceCtx, step.toolName, step.args)
	}
	if len(results) == 0 {
		return data
	}

	enrichment := map[string]interface{}{
		"detection_sequence_complete": true,
		"auto_completed_tools":        results,
		"next_action":                 "用户明确要求的异常检测工具序列已补齐执行：告警列表、告警详情、统计、趋势、Sigma 规则和主机攻击 AI 分析。请基于工具结果输出结论，不要停留在部分检测结果。",
	}
	if asMap, ok := data.(map[string]interface{}); ok {
		copied := make(map[string]interface{}, len(asMap)+len(enrichment))
		for k, v := range asMap {
			copied[k] = v
		}
		for k, v := range enrichment {
			copied[k] = v
		}
		return copied
	}
	enrichment["result"] = data
	return enrichment
}

func (a *AssistantToolGatewayAdapter) requiresExplicitDetectionSequence() bool {
	message := a.userInput
	return strings.Contains(message, "Detection.Alert.List") &&
		strings.Contains(message, "Detection.Alert.Get") &&
		strings.Contains(message, "Detection.Statistics.Get") &&
		strings.Contains(message, "Detection.Trend.Get") &&
		strings.Contains(message, "SigmaRule.List") &&
		strings.Contains(message, "Investigation.HostAttack.Analyze")
}

func isDetectionSequenceTool(toolName string) bool {
	switch toolName {
	case "Detection.Alert.List", "Detection.Alert.Get", "Detection.Statistics.Get", "Detection.Trend.Get", "SigmaRule.List", "Investigation.HostAttack.Analyze":
		return true
	default:
		return false
	}
}

func (a *AssistantToolGatewayAdapter) annotatePackageSequenceResult(ctx context.Context, toolName string, args map[string]interface{}, data interface{}) interface{} {
	if shouldSkipPackageSequence(ctx) || !a.requiresExplicitPackageSequence() || !isPackageSequenceTool(toolName) {
		return data
	}

	packageIDs := extractNamedValues(a.userInput, "package_id")
	getPackageID := getStringArgFromMap(args, "package_id")
	buildPackageID := ""
	if len(packageIDs) > 0 {
		if getPackageID == "" {
			getPackageID = packageIDs[0]
		}
		buildPackageID = packageIDs[len(packageIDs)-1]
	}
	operator := extractNamedID(a.userInput, "operator")
	if operator == "" {
		operator = a.operator
	}
	if operator == "" {
		operator = "assistant"
	}

	sequenceCtx := context.WithValue(ctx, skipPackageSequenceKey{}, true)
	results := map[string]interface{}{}
	if !a.hasSuccessfulToolCall(ctx, "Package.List") {
		results["Package.List"] = a.callToolJSON(sequenceCtx, "Package.List", map[string]interface{}{"page": 1, "page_size": 20})
	}
	if getPackageID != "" && !a.hasSuccessfulToolCall(ctx, "Package.Get") {
		results["Package.Get"] = a.callToolJSON(sequenceCtx, "Package.Get", map[string]interface{}{"package_id": getPackageID})
	}
	if buildPackageID != "" && !a.hasSuccessfulToolCall(ctx, "Package.Build.Start") {
		results["Package.Build.Start"] = a.callToolJSON(sequenceCtx, "Package.Build.Start", map[string]interface{}{"package_id": buildPackageID, "operator": operator})
	}
	if len(results) == 0 {
		return data
	}

	enrichment := map[string]interface{}{
		"package_sequence_complete": true,
		"auto_completed_tools":      results,
		"next_action":               "用户明确要求的动态检测包工具序列已补齐执行：包列表、包详情和构建任务入口。请基于 Package.Build.Start 的结果输出结论。",
	}
	if asMap, ok := data.(map[string]interface{}); ok {
		copied := make(map[string]interface{}, len(asMap)+len(enrichment))
		for k, v := range asMap {
			copied[k] = v
		}
		for k, v := range enrichment {
			copied[k] = v
		}
		return copied
	}
	enrichment["result"] = data
	return enrichment
}

func (a *AssistantToolGatewayAdapter) requiresExplicitPackageSequence() bool {
	message := a.userInput
	return strings.Contains(message, "Package.List") &&
		strings.Contains(message, "Package.Get") &&
		strings.Contains(message, "Package.Build.Start")
}

func isPackageSequenceTool(toolName string) bool {
	switch toolName {
	case "Package.List", "Package.Get", "Package.Build.Start":
		return true
	default:
		return false
	}
}

func (a *AssistantToolGatewayAdapter) hasSuccessfulVulnerabilityExecute(ctx context.Context, scriptType string) bool {
	if a.dispatcher == nil || a.dispatcher.toolCallRepo == nil {
		return false
	}
	calls, _, err := a.dispatcher.toolCallRepo.ListBySession(ctx, a.sessionID, 1, 100)
	if err != nil {
		return false
	}
	for _, call := range calls {
		if call.ToolName != "Vulnerability.Script.Execute" || call.Status != model.ToolCallStatusSuccess {
			continue
		}
		var callArgs map[string]interface{}
		if err := json.Unmarshal(call.Args, &callArgs); err == nil && strings.EqualFold(getStringArgFromMap(callArgs, "script_type"), scriptType) {
			return true
		}
	}
	return false
}

func extractHostIDsFromMessage(message string) []string {
	re := regexp.MustCompile(`(?i)host_ids\s*=\s*\[([^\]]+)\]`)
	match := re.FindStringSubmatch(message)
	if len(match) < 2 {
		return nil
	}
	idRe := regexp.MustCompile(`[0-9a-fA-F-]{36}`)
	return cleanStringSlice(idRe.FindAllString(match[1], -1))
}

func (a *AssistantToolGatewayAdapter) requiresExplicitBaselineSequence() bool {
	return strings.Contains(a.userInput, "Baseline.Script.Generate") &&
		strings.Contains(a.userInput, "Task.RunCheck") &&
		strings.Contains(a.userInput, "Task.RunFix")
}

func (a *AssistantToolGatewayAdapter) missingBaselineSequenceSteps(ctx context.Context, requestedArgs map[string]interface{}) []baselineSequenceStep {
	var steps []baselineSequenceStep
	defaults := a.baselineDefaults()
	templateID := defaults["template_id"]

	scriptArgs := func(scriptType string) map[string]interface{} {
		args := map[string]interface{}{"script_type": scriptType}
		if templateID != "" {
			args["template_id"] = templateID
		}
		if defaults["rule_id"] != "" {
			args["rule_ids"] = []string{defaults["rule_id"]}
		}
		return args
	}

	if !a.hasSuccessfulBaselineScript(ctx, "CHECK") {
		steps = append(steps, baselineSequenceStep{toolName: "Baseline.Script.Generate", scriptType: "CHECK", args: scriptArgs("CHECK")})
	}
	if !a.hasSuccessfulBaselineScript(ctx, "FIX") {
		steps = append(steps, baselineSequenceStep{toolName: "Baseline.Script.Generate", scriptType: "FIX", args: scriptArgs("FIX")})
	}
	if !a.hasSuccessfulToolCall(ctx, "Task.RunCheck") {
		steps = append(steps, baselineSequenceStep{toolName: "Task.RunCheck", args: a.baselineTaskArgs(requestedArgs)})
	}
	if !a.hasSuccessfulToolCall(ctx, "Task.RunFix") {
		steps = append(steps, baselineSequenceStep{toolName: "Task.RunFix", args: a.baselineTaskArgs(requestedArgs)})
	}
	return steps
}

func requestMatchesBaselineStep(toolName string, args map[string]interface{}, step baselineSequenceStep) bool {
	if toolName != step.toolName {
		return false
	}
	if step.toolName != "Baseline.Script.Generate" {
		return true
	}
	return strings.EqualFold(getStringArgFromMap(args, "script_type"), step.scriptType)
}

func (a *AssistantToolGatewayAdapter) hasSuccessfulToolCall(ctx context.Context, toolName string) bool {
	if a.dispatcher == nil || a.dispatcher.toolCallRepo == nil {
		return false
	}
	calls, _, err := a.dispatcher.toolCallRepo.ListBySession(ctx, a.sessionID, 1, 100)
	if err != nil {
		return false
	}
	for _, call := range calls {
		if call.ToolName == toolName && call.Status == model.ToolCallStatusSuccess {
			return true
		}
	}
	return false
}

func (a *AssistantToolGatewayAdapter) hasSuccessfulBaselineScript(ctx context.Context, scriptType string) bool {
	if a.dispatcher == nil || a.dispatcher.toolCallRepo == nil {
		return false
	}
	calls, _, err := a.dispatcher.toolCallRepo.ListBySession(ctx, a.sessionID, 1, 100)
	if err != nil {
		return false
	}
	for _, call := range calls {
		if call.ToolName != "Baseline.Script.Generate" || call.Status != model.ToolCallStatusSuccess {
			continue
		}
		var args map[string]interface{}
		if err := json.Unmarshal(call.Args, &args); err == nil && strings.EqualFold(getStringArgFromMap(args, "script_type"), scriptType) {
			return true
		}
	}
	return false
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
		dispatchResult.Data = a.annotateBaselineSequenceResult(ctx, dispatchResult.ToolName, dispatchResult.Data)
		dispatchResult.Data = a.annotateAssetCollectionSequenceResult(ctx, dispatchResult.ToolName, dispatchResult.Data)
		dispatchResult.Data = a.annotateVulnerabilityScriptSequenceResult(ctx, dispatchResult.ToolName, req.Args, dispatchResult.Data)
		dispatchResult.Data = a.annotateDetectionSequenceResult(ctx, dispatchResult.ToolName, dispatchResult.Data)
		dispatchResult.Data = a.annotatePackageSequenceResult(ctx, dispatchResult.ToolName, req.Args, dispatchResult.Data)
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

// Cancel 实现 agentruntime.ToolGateway 接口（同步执行，无需取消）
func (a *AssistantToolGatewayAdapter) Cancel(_ context.Context, _ string, _ string) error {
	return nil
}
