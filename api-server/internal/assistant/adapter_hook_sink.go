package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"api-server/internal/model"
	"api-server/internal/repository"
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
	memoryRepo repository.AssistantMemoryRepository

	mu                  sync.Mutex
	stepResultSummaries map[string]string
}

// NewAssistantHookSink 创建 HookSink
func NewAssistantHookSink(runManager *RunManager, sessionID, runID, messageID string, logger *zap.Logger) *AssistantHookSink {
	return &AssistantHookSink{
		runManager:          runManager,
		sessionID:           sessionID,
		runID:               runID,
		messageID:           messageID,
		logger:              logger,
		stepResultSummaries: make(map[string]string),
	}
}

// WithMemoryRepository enables persisting internal reflection results without
// exposing them as visible thinking events.
func (s *AssistantHookSink) WithMemoryRepository(repo repository.AssistantMemoryRepository) *AssistantHookSink {
	s.memoryRepo = repo
	return s
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
			"step_id":        event.StepID,
			"title":          stepTitle,
			"result_summary": s.findStepResultSummary(event.StepID, stepTitle),
		})
		s.publish(EventThinking, map[string]interface{}{
			"content": fmt.Sprintf("步骤完成: %s", stepTitle),
		})

	case agentruntime.HookStepFailed:
		// Step failures can be transient when the runtime retries or applies
		// reflection. Do not expose them as final-looking timeline items.

	case agentruntime.HookStepRetrying:
		// Internal recovery signal; the user-facing timeline should resume at
		// the next tool call or completed/skipped step.

	case agentruntime.HookStepSkipped:
		stepTitle := findAssistantStepTitle(event, event.StepID)
		s.publish(EventThinking, map[string]interface{}{
			"content": fmt.Sprintf("已跳过步骤: %s", stepTitle),
		})

	// --- 模型调用 ---

	case agentruntime.HookModelCallStarted:
		// no-op

	case agentruntime.HookModelCallFinished:
		// 模型中间输出通常是 ReAct JSON、步骤总结或最终答案草稿。
		// 可见最终答案由 orchestrator 在任务结束后统一发布，避免前端出现多份分析报告。
		s.rememberStepResult(event)
		return nil

	// --- 工具调用 ---

	case agentruntime.HookToolCallStarted:
		// 工具调用开始事件已由 ToolGatewayAdapter 回调处理
		// 这里仅发送 thinking 事件
		payload := toMap(event.Payload)
		toolName, _ := payload["tool_name"].(string)
		callID, _ := payload["call_id"].(string)
		s.publish(EventThinking, map[string]interface{}{
			"content":   fmt.Sprintf("正在调用工具: %s", toolName),
			"call_id":   callID,
			"tool_name": toolName,
		})

	case agentruntime.HookToolCallFinished:
		// 正常网关结果由 ToolGatewayAdapter 发布。只有在进入网关前发生
		// 的 descriptor/args/policy/step scope 校验失败需要在这里补发。
		payload := toMap(event.Payload)
		validationStage, _ := payload["validation_stage"].(string)
		if validationStage != "" {
			if s.logger != nil {
				toolName, _ := payload["tool_name"].(string)
				callID, _ := payload["call_id"].(string)
				s.logger.Warn("assistant runtime tool call rejected before gateway",
					zap.String("session_id", s.sessionID),
					zap.String("run_id", s.runID),
					zap.String("step_id", event.StepID),
					zap.String("call_id", callID),
					zap.String("tool_name", toolName),
					zap.String("validation_stage", validationStage),
				)
			}
			s.publish(EventToolError, map[string]interface{}{
				"call_id":          payload["call_id"],
				"tool_name":        payload["tool_name"],
				"error":            payload["error_message"],
				"validation_stage": validationStage,
			})
		}

	// --- 审计 ---

	case agentruntime.HookAuditStarted:
		// 审计是 agent-runtime 的内部一致性检查，不作为前端可见思考展示。

	case agentruntime.HookAuditFinished:
		// payload 为空时会序列化为 null；审计结果仅用于内部恢复，不展示到前端。

	// --- 反思 ---
	// 反思是内部恢复策略：不展示到前端，只持久化为后续经验。

	case agentruntime.HookReflectionStarted:
		// no-op: 不在前端显示“正在反思”

	case agentruntime.HookReflectionFinished:
		s.persistReflection(ctx, event)

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

func (s *AssistantHookSink) persistReflection(ctx context.Context, event agentruntime.HookEvent) {
	if s.memoryRepo == nil {
		return
	}
	payload := toMap(event.Payload)
	if len(payload) == 0 {
		return
	}

	content := formatReflectionMemoryContent(payload)
	if strings.TrimSpace(content) == "" {
		return
	}
	metadata := map[string]interface{}{
		"run_id":     s.runID,
		"message_id": s.messageID,
		"task_id":    event.TaskID,
		"step_id":    event.StepID,
		"payload":    payload,
	}
	if err := s.memoryRepo.Create(ctx, &model.AssistantMemory{
		SessionID:  s.sessionID,
		MemoryType: assistantReflectionMemoryType,
		Content:    content,
		Metadata:   mustMarshalJSON(metadata),
	}); err != nil {
		s.logger.Warn("failed to persist assistant reflection",
			zap.String("session_id", s.sessionID),
			zap.String("run_id", s.runID),
			zap.Error(err),
		)
	}
}

func formatReflectionMemoryContent(payload map[string]interface{}) string {
	parts := make([]string, 0, 4)
	if rootCause := strings.TrimSpace(fmt.Sprint(payload["root_cause"])); rootCause != "" && rootCause != "<nil>" {
		parts = append(parts, "root_cause: "+rootCause)
	}
	if recommendation := strings.TrimSpace(fmt.Sprint(payload["recommendation"])); recommendation != "" && recommendation != "<nil>" {
		parts = append(parts, "recommendation: "+recommendation)
	}
	if query := strings.TrimSpace(fmt.Sprint(payload["experience_query"])); query != "" && query != "<nil>" {
		parts = append(parts, "experience_query: "+query)
	}
	if hint := strings.TrimSpace(fmt.Sprint(payload["correction_hint"])); hint != "" && hint != "<nil>" {
		parts = append(parts, "correction_hint: "+hint)
	}
	return strings.Join(parts, "\n")
}

func (s *AssistantHookSink) publishMessageDelta(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	messageID := s.messageID
	if messageID == "" {
		messageID = "msg_" + s.runID
	}
	s.runManager.Publish(s.sessionID, EventMessageDeltaPayload(s.sessionID, s.runID, messageID, content))
}

func shouldSkipVisibleModelOutput(purpose string) bool {
	switch purpose {
	case string(agentruntime.PurposePlan),
		string(agentruntime.PurposeSummarize),
		string(agentruntime.PurposeCompress),
		string(agentruntime.PurposeClassify),
		string(agentruntime.PurposeReflect),
		string(agentruntime.PurposeAudit),
		string(agentruntime.PurposeCorrect):
		return true
	default:
		return false
	}
}

type assistantModelOutput struct {
	Action      string `json:"action"`
	Summary     string `json:"summary"`
	FinalAnswer string `json:"final_answer"`
	ToolCall    *struct {
		ToolName string `json:"tool_name"`
		Reason   string `json:"reason"`
	} `json:"tool_call"`
	StepResult *struct {
		Result     string        `json:"result"`
		Evidence   []interface{} `json:"evidence"`
		Confidence string        `json:"confidence"`
	} `json:"step_result"`
	Failure *struct {
		Reason      string `json:"reason"`
		Recoverable bool   `json:"recoverable"`
	} `json:"failure"`
	ExperienceRequest *struct {
		Query  string `json:"query"`
		Reason string `json:"reason"`
	} `json:"experience_request"`
}

func formatModelOutputForDisplay(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	var output assistantModelOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return raw
	}

	switch output.Action {
	case "tool_call":
		return formatToolCallModelOutput(output)
	case "step_result":
		if output.StepResult != nil && strings.TrimSpace(output.StepResult.Result) != "" {
			return strings.TrimSpace(output.StepResult.Result)
		}
		return strings.TrimSpace(output.Summary)
	case "fail_step":
		if output.Failure != nil && strings.TrimSpace(output.Failure.Reason) != "" {
			return "失败原因: " + strings.TrimSpace(output.Failure.Reason)
		}
		return strings.TrimSpace(output.Summary)
	case "request_experience":
		parts := []string{}
		if output.Summary != "" {
			parts = append(parts, strings.TrimSpace(output.Summary))
		}
		if output.ExperienceRequest != nil && output.ExperienceRequest.Reason != "" {
			parts = append(parts, "原因: "+strings.TrimSpace(output.ExperienceRequest.Reason))
		}
		return strings.Join(parts, "\n")
	default:
		if strings.TrimSpace(output.FinalAnswer) != "" {
			return strings.TrimSpace(output.FinalAnswer)
		}
		if strings.TrimSpace(output.Summary) != "" {
			return strings.TrimSpace(output.Summary)
		}
		return raw
	}
}

func formatToolCallModelOutput(output assistantModelOutput) string {
	parts := []string{}
	if output.Summary != "" {
		parts = append(parts, strings.TrimSpace(output.Summary))
	}
	if output.ToolCall != nil {
		if output.ToolCall.ToolName != "" {
			parts = append(parts, "工具: "+strings.TrimSpace(output.ToolCall.ToolName))
		}
		if output.ToolCall.Reason != "" {
			parts = append(parts, "原因: "+strings.TrimSpace(output.ToolCall.Reason))
		}
	}
	return strings.Join(parts, "\n")
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

func (s *AssistantHookSink) rememberStepResult(event agentruntime.HookEvent) {
	if strings.TrimSpace(event.StepID) == "" {
		return
	}
	payload := toMap(event.Payload)
	purpose, _ := payload["purpose"].(string)
	if purpose != string(agentruntime.PurposeReact) {
		return
	}
	outputSummary, _ := payload["output_summary"].(string)
	resultSummary := extractStepResultSummary(outputSummary)
	if resultSummary == "" {
		return
	}
	s.mu.Lock()
	s.stepResultSummaries[event.StepID] = resultSummary
	s.mu.Unlock()
}

func (s *AssistantHookSink) findStepResultSummary(stepID, stepTitle string) string {
	if strings.TrimSpace(stepID) != "" {
		s.mu.Lock()
		resultSummary := s.stepResultSummaries[stepID]
		s.mu.Unlock()
		if strings.TrimSpace(resultSummary) != "" {
			return resultSummary
		}
	}
	if strings.TrimSpace(stepTitle) == "" {
		return "步骤已完成"
	}
	return fmt.Sprintf("已完成步骤：%s", stepTitle)
}

func extractStepResultSummary(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var output assistantModelOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return ""
	}
	if output.Action != "step_result" {
		return ""
	}
	if output.StepResult != nil && strings.TrimSpace(output.StepResult.Result) != "" {
		return strings.TrimSpace(output.StepResult.Result)
	}
	return strings.TrimSpace(output.Summary)
}
