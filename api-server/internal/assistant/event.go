package assistant

import (
	"time"
)

// AssistantEvent SSE 事件
type AssistantEvent struct {
	Type      string      `json:"type"`
	SessionID string      `json:"session_id"`
	RunID     string      `json:"run_id,omitempty"`
	MessageID string      `json:"message_id,omitempty"`
	Payload   interface{} `json:"payload,omitempty"`
	Error     string      `json:"error,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// EventWriter 事件写入器接口
type EventWriter interface {
	Write(event AssistantEvent) error
}

// SSE Event type constants
const (
	EventThinking                 = "thinking"
	EventMessageDelta             = "message_delta"
	EventIntentDetected           = "intent_detected"
	EventToolsSelected            = "tools_selected"
	EventToolSearch               = "tool_search"
	EventToolExpansion            = "tool_expansion"
	EventPlan                     = "plan"
	EventStepStarted              = "step_started"
	EventStepCompleted            = "step_completed"
	EventToolCall                 = "tool_call"
	EventToolResult               = "tool_result"
	EventToolError                = "tool_error"
	EventApprovalRequired         = "approval_required"
	EventApprovalUpdated          = "approval_updated"
	EventContextRefAdded          = "context_ref_added"
	EventResultCard               = "result_card"
	EventContextBudget            = "context_budget"
	EventContextCompressed        = "context_compressed"
	EventContextCompressionFailed = "context_compression_failed"
	EventDone                     = "done"
	EventError                    = "error"
	EventRunStarted               = "run_started"
	EventRunWaitingApproval       = "run_waiting_approval"
	EventBusinessObject           = "business_object"
)

// NewEvent 创建新事件
func NewEvent(eventType, sessionID, runID string, payload interface{}) AssistantEvent {
	return AssistantEvent{
		Type:      eventType,
		SessionID: sessionID,
		RunID:     runID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
}

// withMessageID 将事件归属到本次运行的助手消息，便于前端把计划、步骤和工具调用实时挂到同一条消息上。
func withMessageID(event AssistantEvent, messageID string) AssistantEvent {
	event.MessageID = messageID
	return event
}

// EventThinking 创建思考事件
func EventThinkingPayload(sessionID, runID, content string) AssistantEvent {
	return NewEvent(EventThinking, sessionID, runID, map[string]interface{}{
		"content": content,
	})
}

// EventMessageDelta 创建消息增量事件
func EventMessageDeltaPayload(sessionID, runID, messageID, delta string) AssistantEvent {
	return withMessageID(NewEvent(EventMessageDelta, sessionID, runID, map[string]interface{}{
		"message_id": messageID,
		"delta":      delta,
	}), messageID)
}

// EventPlanPayload 创建计划事件
func EventPlanPayload(sessionID, runID, messageID string, plan interface{}) AssistantEvent {
	return withMessageID(NewEvent(EventPlan, sessionID, runID, plan), messageID)
}

// EventToolCallPayload 创建工具调用事件
func EventToolCallPayload(sessionID, runID, messageID, callID, toolName string, args interface{}) AssistantEvent {
	return withMessageID(NewEvent(EventToolCall, sessionID, runID, map[string]interface{}{
		"call_id":   callID,
		"tool_name": toolName,
		"args":      args,
	}), messageID)
}

// EventToolResultPayload 创建工具结果事件
func EventToolResultPayload(sessionID, runID, messageID, callID string, result interface{}) AssistantEvent {
	return withMessageID(NewEvent(EventToolResult, sessionID, runID, map[string]interface{}{
		"call_id": callID,
		"result":  result,
	}), messageID)
}

// EventToolErrorPayload 创建工具错误事件
func EventToolErrorPayload(sessionID, runID, messageID, callID, errMsg string) AssistantEvent {
	return withMessageID(NewEvent(EventToolError, sessionID, runID, map[string]interface{}{
		"call_id": callID,
		"error":   errMsg,
	}), messageID)
}

// EventApprovalRequiredPayload 创建审批请求事件
func EventApprovalRequiredPayload(sessionID, runID, messageID string, approval interface{}) AssistantEvent {
	return withMessageID(NewEvent(EventApprovalRequired, sessionID, runID, approval), messageID)
}

// EventResultCardPayload 创建结果卡片事件
func EventResultCardPayload(sessionID, runID string, card interface{}) AssistantEvent {
	return NewEvent(EventResultCard, sessionID, runID, card)
}

// EventDonePayload 创建完成事件
func EventDonePayload(sessionID, runID string) AssistantEvent {
	return NewEvent(EventDone, sessionID, runID, map[string]interface{}{
		"status": "completed",
	})
}

// EventErrorPayload 创建错误事件
func EventErrorPayload(sessionID, runID, errMsg string) AssistantEvent {
	return NewEvent(EventError, sessionID, runID, map[string]interface{}{
		"message": errMsg,
	})
}

// EventBusinessObject 创建业务对象事件（对齐设计文档 19 节）
func EventBusinessObjectPayload(sessionID, runID, objectType, objectID string, data interface{}) AssistantEvent {
	return NewEvent(EventBusinessObject, sessionID, runID, map[string]interface{}{
		"object_type": objectType,
		"object_id":   objectID,
		"data":        data,
	})
}

// EventRunWaitingApprovalPayload 创建等待审批事件
func EventRunWaitingApprovalPayload(sessionID, runID, approvalID, toolName string) AssistantEvent {
	return NewEvent(EventRunWaitingApproval, sessionID, runID, map[string]interface{}{
		"approval_id": approvalID,
		"tool_name":   toolName,
		"status":      "waiting_approval",
	})
}

// EventToolSearchPayload 创建工具搜索事件
func EventToolSearchPayload(sessionID, runID, query string) AssistantEvent {
	return NewEvent(EventToolSearch, sessionID, runID, map[string]interface{}{
		"query": query,
	})
}

// EventToolExpansionPayload 创建工具扩展事件
func EventToolExpansionPayload(sessionID, runID string, addedTools []string) AssistantEvent {
	return NewEvent(EventToolExpansion, sessionID, runID, map[string]interface{}{
		"added_tools": addedTools,
		"status":      "expanded",
	})
}
