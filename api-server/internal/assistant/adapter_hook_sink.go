package assistant

import (
	"context"
	"encoding/json"
	"fmt"

	agentruntime "github.com/alex-chenc/agent-runtime"
	"go.uber.org/zap"
)

// AssistantHookSink 适配 agent-runtime HookSink 接口
// 将 agent-runtime 的 HookEvent 桥接到 RunManager 的 SSE 事件
type AssistantHookSink struct {
	runManager *RunManager
	sessionID  string
	runID      string
	messageID  string
	logger     *zap.Logger
}

// NewAssistantHookSink 创建 HookSink
func NewAssistantHookSink(runManager *RunManager, sessionID, runID, messageID string, logger *zap.Logger) *AssistantHookSink {
	return &AssistantHookSink{
		runManager: runManager,
		sessionID:  sessionID,
		runID:      runID,
		messageID:  messageID,
		logger:     logger,
	}
}

// Handle 实现 agentruntime.HookSink 接口
func (s *AssistantHookSink) Handle(ctx context.Context, event agentruntime.HookEvent) error {
	switch event.Type {

	// --- 任务生命周期 ---

	case agentruntime.HookTaskStarted:
		s.publish(EventThinking, map[string]interface{}{
			"content": "开始执行任务...",
		})

	case agentruntime.HookTaskFinished:
		// 最终消息由 orchestrator 统一发布，此处不重复发送
		// 避免前端收到两次相同内容

	case agentruntime.HookTaskInterrupted:
		s.publish(EventError, map[string]interface{}{
			"message": "任务被中断",
		})

	// --- 经验/计划 ---

	case agentruntime.HookExperienceLoaded:
		s.publish(EventThinking, map[string]interface{}{
			"content": "正在加载历史经验...",
		})

	case agentruntime.HookPlanCreated:
		s.publish(EventThinking, map[string]interface{}{
			"content": "正在制定执行计划...",
		})
		// 发送计划事件
		if event.Snapshot != nil && event.Snapshot.CurrentPlan != nil {
			planJSON, err := json.Marshal(event.Snapshot.CurrentPlan)
			if err == nil {
				s.publish(EventPlan, json.RawMessage(planJSON))
			}
		}

	// --- 步骤生命周期 ---

	case agentruntime.HookStepStarted:
		stepTitle := findAssistantStepTitle(event, event.StepID)
		s.publish(EventStepStarted, map[string]interface{}{
			"step_id": event.StepID,
			"title":   stepTitle,
		})
		s.publish(EventThinking, map[string]interface{}{
			"content": fmt.Sprintf("开始执行步骤: %s", stepTitle),
		})

	case agentruntime.HookStepCompleted:
		stepTitle := findAssistantStepTitle(event, event.StepID)
		s.publish(EventStepCompleted, map[string]interface{}{
			"step_id": event.StepID,
			"title":   stepTitle,
		})
		s.publish(EventThinking, map[string]interface{}{
			"content": fmt.Sprintf("步骤完成: %s", stepTitle),
		})

	case agentruntime.HookStepFailed:
		stepTitle := findAssistantStepTitle(event, event.StepID)
		s.publish(EventThinking, map[string]interface{}{
			"content": fmt.Sprintf("步骤失败: %s", stepTitle),
		})

	case agentruntime.HookStepRetrying:
		stepTitle := findAssistantStepTitle(event, event.StepID)
		s.publish(EventThinking, map[string]interface{}{
			"content": fmt.Sprintf("正在重试步骤: %s", stepTitle),
		})

	case agentruntime.HookStepSkipped:
		stepTitle := findAssistantStepTitle(event, event.StepID)
		s.publish(EventThinking, map[string]interface{}{
			"content": fmt.Sprintf("已跳过步骤: %s", stepTitle),
		})

	// --- 模型调用 ---

	case agentruntime.HookModelCallStarted:
		// no-op

	case agentruntime.HookModelCallFinished:
		payload := toMap(event.Payload)
		if summary, ok := payload["output_summary"].(string); ok && summary != "" {
			s.publish(EventThinking, map[string]interface{}{
				"content": summary,
			})
		}

	// --- 工具调用 ---

	case agentruntime.HookToolCallStarted:
		// 工具调用开始事件已由 ToolGatewayAdapter 回调处理
		// 这里仅发送 thinking 事件
		payload := toMap(event.Payload)
		toolName, _ := payload["tool_name"].(string)
		s.publish(EventThinking, map[string]interface{}{
			"content": fmt.Sprintf("正在调用工具: %s", toolName),
		})

	case agentruntime.HookToolCallFinished:
		// 工具调用完成事件已由 ToolGatewayAdapter 回调处理

	// --- 审计 ---

	case agentruntime.HookAuditStarted:
		s.publish(EventThinking, map[string]interface{}{
			"content": "正在审计执行进度...",
		})

	case agentruntime.HookAuditFinished:
		payload := toMap(event.Payload)
		auditJSON, _ := json.Marshal(payload)
		s.publish(EventThinking, map[string]interface{}{
			"content": fmt.Sprintf("审计完成: %s", string(auditJSON)),
		})

	// --- 反思 ---

	case agentruntime.HookReflectionStarted:
		s.publish(EventThinking, map[string]interface{}{
			"content": "正在反思执行过程...",
		})

	case agentruntime.HookReflectionFinished:
		payload := toMap(event.Payload)
		rootCause, _ := payload["root_cause"].(string)
		if rootCause != "" {
			s.publish(EventThinking, map[string]interface{}{
				"content": fmt.Sprintf("反思结果: %s", rootCause),
			})
		}

	// --- 纠正 ---

	case agentruntime.HookCorrectionApplied:
		payload := toMap(event.Payload)
		reason, _ := payload["reason"].(string)
		if reason != "" {
			s.publish(EventThinking, map[string]interface{}{
				"content": fmt.Sprintf("计划纠正: %s", reason),
			})
		}

	// --- 上下文压缩 ---

	case agentruntime.HookContextBudgetChecked:
		if event.Snapshot != nil && event.Snapshot.ContextBudget != nil {
			s.publish(EventContextBudget, event.Snapshot.ContextBudget)
		}

	case agentruntime.HookContextCompressed:
		payload := toMap(event.Payload)
		strategy, _ := payload["strategy"].(string)
		var beforeTokens, afterTokens int
		if v, ok := payload["before_tokens"].(float64); ok {
			beforeTokens = int(v)
		}
		if v, ok := payload["after_tokens"].(float64); ok {
			afterTokens = int(v)
		}
		s.publish(EventThinking, map[string]interface{}{
			"content": fmt.Sprintf("上下文压缩 (%s): %d → %d tokens", strategy, beforeTokens, afterTokens),
		})
		s.publish(EventContextCompressed, payload)

	case agentruntime.HookContextCompressionFailed:
		payload := toMap(event.Payload)
		errMsg, _ := payload["error"].(string)
		s.publish(EventThinking, map[string]interface{}{
			"content": fmt.Sprintf("上下文压缩失败: %s", errMsg),
		})
		eventPayload := map[string]interface{}{"message": errMsg}
		if len(payload) > 0 {
			eventPayload = payload
		}
		s.publish(EventContextCompressionFailed, eventPayload)
	}

	return nil
}

// publish 发布事件到 RunManager
func (s *AssistantHookSink) publish(eventType string, payload interface{}) {
	event := NewEvent(eventType, s.sessionID, s.runID, payload)
	if s.messageID != "" {
		event.MessageID = s.messageID
	}
	s.runManager.Publish(s.sessionID, event)
}

// toMap 将任意 payload 转换为 map[string]interface{}
func toMap(payload any) map[string]interface{} {
	if payload == nil {
		return nil
	}
	if m, ok := payload.(map[string]interface{}); ok {
		return m
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

// findAssistantStepTitle 从事件快照中查找步骤标题
func findAssistantStepTitle(event agentruntime.HookEvent, stepID string) string {
	if event.Snapshot == nil || event.Snapshot.CurrentPlan == nil {
		return stepID
	}
	for _, step := range event.Snapshot.CurrentPlan.Steps {
		if step.StepID == stepID {
			if step.Title != "" {
				return step.Title
			}
			if step.Objective != "" {
				return step.Objective
			}
			return stepID
		}
	}
	return stepID
}
