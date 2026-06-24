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

	"api-server/internal/model"
)

type naturalOperationKind string

const (
	naturalOperationNone              naturalOperationKind = ""
	naturalOperationAssetCollection   naturalOperationKind = "asset_collection"
	naturalOperationVulnerabilityScan naturalOperationKind = "vulnerability_scan"
	naturalOperationBaselineScan      naturalOperationKind = "baseline_scan"
	naturalOperationDetectionCheck    naturalOperationKind = "detection_check"
)

type naturalOperationShortcut struct {
	Kind naturalOperationKind
}

func detectNaturalOperationShortcut(message string) naturalOperationShortcut {
	text := normalizeNaturalOperationText(message)
	if text == "" || shouldForceExplicitToolSequence(message) || containsExplicitToolReference(text) || looksLikeHowToQuestion(text) {
		return naturalOperationShortcut{}
	}
	if hasCompositeNaturalOperationIntent(text) {
		return naturalOperationShortcut{}
	}

	switch {
	case hasAssetCollectionIntent(text):
		return naturalOperationShortcut{Kind: naturalOperationAssetCollection}
	case hasVulnerabilityScanIntent(text):
		return naturalOperationShortcut{Kind: naturalOperationVulnerabilityScan}
	case hasBaselineScanIntent(text):
		return naturalOperationShortcut{Kind: naturalOperationBaselineScan}
	case hasDetectionCheckIntent(text):
		return naturalOperationShortcut{Kind: naturalOperationDetectionCheck}
	default:
		return naturalOperationShortcut{}
	}
}

func (o *Orchestrator) runNaturalOperationShortcut(ctx context.Context, input RunInput, messageID string, contextRefs []ContextRefResult) (bool, string, error) {
	shortcut := detectNaturalOperationShortcut(input.UserMessage)
	switch shortcut.Kind {
	case naturalOperationAssetCollection:
		return o.runAssetCollectionShortcut(ctx, input, messageID, contextRefs)
	case naturalOperationVulnerabilityScan:
		return true, vulnerabilityScanClarification(), nil
	case naturalOperationBaselineScan:
		return true, baselineScanClarification(), nil
	case naturalOperationDetectionCheck:
		return true, detectionCheckClarification(), nil
	default:
		return false, "", nil
	}
}

func (o *Orchestrator) runAssetCollectionShortcut(ctx context.Context, input RunInput, messageID string, contextRefs []ContextRefResult) (bool, string, error) {
	if o.toolDispatcher == nil || o.toolRegistry == nil {
		return false, "", nil
	}
	if _, ok := o.toolRegistry.Get("Asset.Collection.Trigger"); !ok {
		return false, "", nil
	}

	if o.runManager != nil {
		o.runManager.Publish(input.SessionID, EventThinkingPayload(input.SessionID, input.RunID, "识别到资产采集指令，正在直接调用资产采集接口..."))
	}

	gateway := o.newShortcutToolGateway(input, messageID, contextRefs)
	resp, err := gateway.Call(ctx, agentruntime.ToolRequest{
		CallID:   fmt.Sprintf("shortcut_asset_collection_%d", time.Now().UnixNano()),
		ToolName: "Asset.Collection.Trigger",
		Args: map[string]interface{}{
			"scope": "all_hosts",
			"types": []string{"process"},
			"force": true,
		},
	})
	if err != nil {
		return true, "", err
	}
	if o.hasRejectedApproval(input.SessionID) {
		return true, "", context.Canceled
	}
	if resp.Status != agentruntime.ToolCallSuccess {
		return true, assetCollectionFailedAnswer(resp.ErrorMessage), nil
	}
	return true, assetCollectionShortcutAnswer(resp.Content), nil
}

func (o *Orchestrator) newShortcutToolGateway(input RunInput, messageID string, contextRefs []ContextRefResult) *AssistantToolGatewayAdapter {
	return NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher:  o.toolDispatcher,
		SessionID:   input.SessionID,
		MessageID:   messageID,
		RunID:       input.RunID,
		Operator:    input.UserID,
		Logger:      o.logger,
		RunManager:  o.runManager,
		UserInput:   input.UserMessage,
		ContextRefs: contextRefs,
		OnToolCall: func(callID, toolName string, args interface{}) {
			if o.runManager != nil {
				o.runManager.Publish(input.SessionID, EventToolCallPayload(input.SessionID, input.RunID, messageID, callID, toolName, args))
			}
		},
		OnToolResult: func(callID string, result interface{}) {
			if o.runManager != nil {
				o.runManager.Publish(input.SessionID, EventToolResultPayload(input.SessionID, input.RunID, messageID, callID, result))
			}
		},
		OnToolError: func(callID, errMsg string) {
			if o.runManager != nil {
				o.runManager.Publish(input.SessionID, EventToolErrorPayload(input.SessionID, input.RunID, messageID, callID, errMsg))
			}
		},
		OnApproval: func(approval interface{}) {
			if o.runManager == nil {
				return
			}
			o.runManager.Publish(input.SessionID, EventApprovalRequiredPayload(input.SessionID, input.RunID, messageID, approval))
			if typed, ok := approval.(*model.AssistantApproval); ok {
				o.runManager.Publish(input.SessionID, EventRunWaitingApprovalPayload(input.SessionID, input.RunID, typed.ApprovalID, typed.ToolName))
			}
		},
		OnApprovalUpdated: func(approval interface{}) {
			if o.runManager != nil {
				o.runManager.Publish(input.SessionID, withMessageID(NewEvent(EventApprovalUpdated, input.SessionID, input.RunID, approval), messageID))
			}
		},
	})
}

func (o *Orchestrator) finishNaturalOperationShortcutRun(input RunInput, messageID string, response string, useAIAnalysisFlow bool, maxIterations int) (*RunResult, error) {
	o.mergeSessionMetadata(context.Background(), input.SessionID, map[string]interface{}{
		"runtime_profile":       runtimeProfileName(useAIAnalysisFlow),
		"max_total_turns":       maxIterations,
		"current_run_id":        input.RunID,
		"current_message_id":    messageID,
		"current_run_status":    "completed",
		"last_run_completed_at": time.Now().UTC().Format(time.RFC3339),
		"natural_operation":     true,
	})

	if o.runManager != nil {
		o.runManager.Publish(input.SessionID, EventMessageDeltaPayload(input.SessionID, input.RunID, messageID, response))
		o.persistSessionRuntimeEvents(
			context.Background(),
			input.SessionID,
			messageID,
			compactRuntimeDisplayEvents(o.extractRunHistory(input.SessionID), input.RunID, messageID),
		)
	}

	if o.messageRepo != nil {
		if err := o.messageRepo.Create(context.Background(), &model.AssistantMessage{
			ID:        uuid.New(),
			SessionID: input.SessionID,
			MessageID: messageID,
			Role:      "assistant",
			Content:   response,
			Thinking:  o.extractThinkingFromHistory(input.SessionID),
			Plan:      o.extractPlanFromHistory(input.SessionID),
		}); err != nil && o.logger != nil {
			o.logger.Error("failed to save natural operation assistant message", zap.Error(err))
		}
	}

	if o.runManager != nil {
		o.runManager.Publish(input.SessionID, EventDonePayload(input.SessionID, input.RunID))
	}
	return &RunResult{MessageID: messageID, FinalAnswer: response}, nil
}

func (o *Orchestrator) hasRejectedApproval(sessionID string) bool {
	if o.runManager == nil {
		return false
	}
	run, ok := o.runManager.Get(sessionID)
	if !ok {
		return false
	}
	return run.RejectedApproval() != nil
}

func normalizeNaturalOperationText(message string) string {
	text := strings.TrimSpace(strings.ToLower(message))
	replacer := strings.NewReplacer(
		"，", " ",
		"。", " ",
		"；", " ",
		";", " ",
		"！", " ",
		"!", " ",
		"？", " ",
		"?", " ",
		"\n", " ",
		"\t", " ",
	)
	text = replacer.Replace(text)
	return strings.Join(strings.Fields(text), " ")
}

func containsExplicitToolReference(text string) bool {
	for _, marker := range []string{
		"asset.",
		"vulnerability.",
		"baseline.",
		"task.",
		"detection.",
		"credential.",
		"host.",
		"agent.",
		"sigmarule.",
		"investigation.",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func looksLikeHowToQuestion(text string) bool {
	for _, keyword := range []string{"怎么", "如何", "怎样", "是什么", "介绍", "说明", "文档", "原理", "为什么"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func hasAssetCollectionIntent(text string) bool {
	if strings.Contains(text, "采集结果") || strings.Contains(text, "采集进度") || strings.Contains(text, "采集状态") {
		return false
	}
	return strings.Contains(text, "资产采集") ||
		strings.Contains(text, "资源采集") ||
		strings.Contains(text, "采集资产") ||
		strings.Contains(text, "采集资源") ||
		strings.Contains(text, "资产收集") ||
		strings.Contains(text, "资源收集")
}

func hasCompositeNaturalOperationIntent(text string) bool {
	if !hasAssetCollectionIntent(text) {
		return false
	}
	for _, keyword := range []string{
		"并", "然后", "同时", "之后", "再",
		"分析", "评估", "研判", "关联", "找出", "识别",
		"哪些主机", "那个主机", "这个主机", "是否",
		"软件", "mysql", "mariadb", "nginx", "postgres", "redis", "docker",
		"漏洞", "cve", "poc", "风险", "受影响", "修复",
	} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func hasVulnerabilityScanIntent(text string) bool {
	return strings.Contains(text, "漏洞扫描") ||
		strings.Contains(text, "漏洞检测") ||
		strings.Contains(text, "poc检测") ||
		strings.Contains(text, "poc 检测") ||
		strings.Contains(text, "漏洞修复")
}

func hasBaselineScanIntent(text string) bool {
	return strings.Contains(text, "基线扫描") ||
		strings.Contains(text, "基线检测") ||
		strings.Contains(text, "基线检查") ||
		strings.Contains(text, "合规检测") ||
		strings.Contains(text, "基线修复")
}

func hasDetectionCheckIntent(text string) bool {
	return strings.Contains(text, "异常检测") ||
		strings.Contains(text, "异常分析") ||
		strings.Contains(text, "威胁检测") ||
		strings.Contains(text, "安全事件分析")
}

func vulnerabilityScanClarification() string {
	return "我理解你要做漏洞扫描。请补充目标和动作，我再继续执行：\n\n1. 扫描范围：全部在线主机，还是指定主机 ID/IP？\n2. 扫描方式：只做资产漏洞匹配，还是也执行 POC 检测？\n3. 处置动作：只生成报告，还是需要生成并下发修复脚本？"
}

func baselineScanClarification() string {
	return "我理解你要做基线扫描。请补充目标和基线范围，我再继续执行：\n\n1. 扫描主机：全部在线主机，还是指定主机 ID/IP？\n2. 基线模板：使用已上传/已识别的模板，还是先上传基线文件？\n3. 执行动作：只检测，还是同时生成修复脚本并下发修复任务？"
}

func detectionCheckClarification() string {
	return "我理解你要做异常检测。请补充检测范围，我再继续执行：\n\n1. 目标主机：全部在线主机，还是指定主机 ID/IP？\n2. 检测内容：异常事件 AI 分析、规则识别，还是动态检测包构建/下发？\n3. 时间范围：最近 24 小时，还是指定起止时间？"
}

func assetCollectionShortcutAnswer(content string) string {
	summary := map[string]interface{}{}
	if strings.TrimSpace(content) != "" {
		_ = json.Unmarshal([]byte(content), &summary)
	}
	taskID := extractToolResultString(summary, "task_id")
	if taskID == "" {
		taskID = extractNestedString(summary, "verified_result_summary", "collection_task_id")
	}
	status := extractNestedString(summary, "verified_result_summary", "collection_status")
	if status == "" {
		status = extractToolResultString(summary, "status")
	}
	aiAgents := naturalOperationNumber(extractNestedNumericResult(summary, "verified_result_summary", "ai_agent_total"))
	llmServices := naturalOperationNumber(extractNestedNumericResult(summary, "verified_result_summary", "llm_service_total"))
	mcpServers := naturalOperationNumber(extractNestedNumericResult(summary, "verified_result_summary", "mcp_server_total"))

	lines := []string{"已开始对全部在线主机进行资产采集，并同步查询了采集进度、AI 应用资产和资产概览。"}
	if taskID != "" {
		lines = append(lines, fmt.Sprintf("采集任务：%s", taskID))
	}
	if status != "" {
		lines = append(lines, fmt.Sprintf("当前状态：%s", status))
	}
	if aiAgents > 0 || llmServices > 0 || mcpServers > 0 {
		lines = append(lines, fmt.Sprintf("已识别 AI Agent %d 个、LLM 服务 %d 个、MCP Server %d 个。", aiAgents, llmServices, mcpServers))
	}
	lines = append(lines, "任务进度和采集结果会同步到运维模式的资产采集任务与资产库页面。")
	return strings.Join(lines, "\n")
}

func assetCollectionFailedAnswer(errMsg string) string {
	errMsg = strings.TrimSpace(errMsg)
	if errMsg == "" {
		errMsg = "资产采集工具未返回成功状态"
	}
	return "资产采集没有成功启动：" + errMsg + "\n\n请确认至少有一台 Agent 在线，或补充要采集的主机范围后重试。"
}

func naturalOperationNumber(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case int32:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case json.Number:
		i, _ := typed.Int64()
		return int(i)
	default:
		return 0
	}
}
