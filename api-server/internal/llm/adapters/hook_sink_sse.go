package adapters

import (
	"context"
	"encoding/json"
	"fmt"

	agentruntime "github.com/chenchen511/agent-runtime"

	"api-server/internal/llm"
)

// EventCollector abstracts the response collection side so that this package
// does not import api-server/internal/api/handler directly (which would form
// a cycle).  The handler's SSEResponseCollector can satisfy this interface via
// a thin wrapper if its method signatures differ.
type EventCollector interface {
	AddThinking(text string)
	AddToolCall(tool, callID string, args interface{})
	AddToolResult(callID string, result interface{})
	AddToolError(callID, errMsg string)
	SetContent(content string)
	SetPlan(plan interface{})
	AddAudit(audit interface{})
	AddReflection(reflection interface{})
	AddCorrection(correction interface{})
}

// SSEHookSink bridges agent-runtime HookEvents to SSE streaming responses.
// It implements agent-runtime's HookSink interface.
type SSEHookSink struct {
	writer    *llm.SSEWriter
	collector EventCollector
}

// NewSSEHookSink creates a new SSEHookSink.
func NewSSEHookSink(writer *llm.SSEWriter, collector EventCollector) *SSEHookSink {
	return &SSEHookSink{
		writer:    writer,
		collector: collector,
	}
}

// Handle implements agent-runtime HookSink.  It dispatches each HookEvent
// to the appropriate SSE writer method and collector method.
func (s *SSEHookSink) Handle(ctx context.Context, event agentruntime.HookEvent) error {
	switch event.Type {

	// --- task lifecycle ---

	case agentruntime.HookTaskStarted:
		// no-op; internal tracking only

	case agentruntime.HookTaskFinished:
		payload := toMap(event.Payload)
		if finalAnswer, ok := payload["final_answer"].(string); ok && finalAnswer != "" && s.collector != nil {
			s.collector.SetContent(finalAnswer)
		}

	case agentruntime.HookTaskInterrupted:
		_ = s.writer.WriteError("Analysis interrupted")

	// --- experience / plan ---

	case agentruntime.HookExperienceLoaded:
		s.writeThinking("Loading experience...")

	case agentruntime.HookPlanCreated:
		s.writeThinking("Creating analysis plan...")

		var plan *agentruntime.Plan
		if event.Snapshot != nil {
			plan = event.Snapshot.CurrentPlan
		}
		if plan != nil {
			planJSON, err := json.Marshal(plan)
			if err == nil {
				_ = s.writer.Write(llm.SSEEvent{
					Type:    "plan",
					Content: string(planJSON),
				})
			}
			if s.collector != nil {
				s.collector.SetPlan(plan)
			}
		}

	// --- step lifecycle ---

	case agentruntime.HookStepStarted:
		stepTitle := findStepTitle(event, event.StepID)
		if stepTitle != "" {
			s.writeThinking(fmt.Sprintf("开始执行步骤: %s", stepTitle))
		} else {
			s.writeThinking(fmt.Sprintf("Step %s started...", event.StepID))
		}
		_ = s.writer.Write(llm.SSEEvent{
			Type:   "step_started",
			CallID: event.StepID,
		})

	case agentruntime.HookStepCompleted:
		stepTitle := findStepTitle(event, event.StepID)
		if stepTitle != "" {
			s.writeThinking(fmt.Sprintf("步骤完成: %s", stepTitle))
		} else {
			s.writeThinking(fmt.Sprintf("Step %s completed", event.StepID))
		}
		_ = s.writer.Write(llm.SSEEvent{
			Type:   "step_completed",
			CallID: event.StepID,
		})

	case agentruntime.HookStepFailed:
		stepTitle := findStepTitle(event, event.StepID)
		if stepTitle != "" {
			s.writeThinking(fmt.Sprintf("步骤失败: %s", stepTitle))
		} else {
			s.writeThinking(fmt.Sprintf("Step %s failed", event.StepID))
		}
		_ = s.writer.Write(llm.SSEEvent{
			Type:   "step_failed",
			CallID: event.StepID,
		})

	case agentruntime.HookStepRetrying:
		stepTitle := findStepTitle(event, event.StepID)
		if stepTitle != "" {
			s.writeThinking(fmt.Sprintf("正在重试步骤: %s", stepTitle))
		} else {
			s.writeThinking(fmt.Sprintf("Retrying step %s...", event.StepID))
		}
		_ = s.writer.Write(llm.SSEEvent{
			Type:   "step_retrying",
			CallID: event.StepID,
		})

	case agentruntime.HookStepSkipped:
		stepTitle := findStepTitle(event, event.StepID)
		if stepTitle != "" {
			s.writeThinking(fmt.Sprintf("已跳过步骤: %s", stepTitle))
		} else {
			s.writeThinking(fmt.Sprintf("Step %s skipped", event.StepID))
		}
		_ = s.writer.Write(llm.SSEEvent{
			Type:   "step_skipped",
			CallID: event.StepID,
		})

	// --- model calls ---

	case agentruntime.HookModelCallStarted:
		// no-op

	case agentruntime.HookModelCallFinished:
		payload := toMap(event.Payload)
		if purpose, _ := payload["purpose"].(string); purpose == string(agentruntime.PurposeSummarize) {
			return nil
		}
		if summary, ok := payload["output_summary"].(string); ok && summary != "" {
			s.writeThinking(summary)
		}

	// --- tool calls ---

	case agentruntime.HookToolCallStarted:
		payload := toMap(event.Payload)
		toolName, _ := payload["tool_name"].(string)
		callID, _ := payload["call_id"].(string)
		argsSummary := payload["args_summary"]

		_ = s.writer.WriteToolCall(toolName, callID, argsSummary)
		if s.collector != nil {
			s.collector.AddToolCall(toolName, callID, argsSummary)
		}

	case agentruntime.HookToolCallFinished:
		payload := toMap(event.Payload)
		status, _ := payload["status"].(string)
		callID, _ := payload["call_id"].(string)

		if status == "success" {
			resultSummary := payload["result_summary"]
			var durationMs int64
			if dm, ok := payload["duration_ms"].(float64); ok {
				durationMs = int64(dm)
			}
			_ = s.writer.WriteToolResult(callID, resultSummary, durationMs)
			if s.collector != nil {
				s.collector.AddToolResult(callID, resultSummary)
			}
		} else {
			errMsg, _ := payload["error_message"].(string)
			_ = s.writer.WriteToolError(callID, errMsg)
			if s.collector != nil {
				s.collector.AddToolError(callID, errMsg)
			}
		}

	// --- audit ---

	case agentruntime.HookAuditStarted:
		s.writeThinking("Auditing progress...")

	case agentruntime.HookAuditFinished:
		payload := toMap(event.Payload)
		auditSummary := buildAuditSummary(payload)
		if auditSummary != "" {
			s.writeThinking(auditSummary)
		}
		if auditJSON, err := json.Marshal(payload); err == nil {
			_ = s.writer.Write(llm.SSEEvent{
				Type:    "audit",
				Content: string(auditJSON),
			})
		}
		if s.collector != nil {
			s.collector.AddAudit(payload)
		}

	// --- reflection ---

	case agentruntime.HookReflectionStarted:
		s.writeThinking("Reflecting on failure...")

	case agentruntime.HookReflectionFinished:
		payload := toMap(event.Payload)
		reflSummary := buildReflectionSummary(payload)
		if reflSummary != "" {
			s.writeThinking(reflSummary)
		}
		if reflJSON, err := json.Marshal(payload); err == nil {
			_ = s.writer.Write(llm.SSEEvent{
				Type:    "reflection",
				Content: string(reflJSON),
			})
		}
		if s.collector != nil {
			s.collector.AddReflection(payload)
		}

	// --- correction ---

	case agentruntime.HookCorrectionApplied:
		payload := toMap(event.Payload)
		corrSummary := buildCorrectionSummary(payload)
		if corrSummary != "" {
			s.writeThinking(corrSummary)
		}
		if corrJSON, err := json.Marshal(payload); err == nil {
			_ = s.writer.Write(llm.SSEEvent{
				Type:    "correction",
				Content: string(corrJSON),
			})
		}
		if s.collector != nil {
			s.collector.AddCorrection(payload)
		}

	// --- config ---

	case agentruntime.HookConfigChanged:
		// no-op

	// --- context budget ---

	case agentruntime.HookContextBudgetChecked:
		// Budget data is in Snapshot.ContextBudget, not Payload
		if event.Snapshot != nil && event.Snapshot.ContextBudget != nil {
			if budgetJSON, err := json.Marshal(event.Snapshot.ContextBudget); err == nil {
				_ = s.writer.Write(llm.SSEEvent{
					Type:    "context_budget",
					Content: string(budgetJSON),
				})
			}
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
		s.writeThinking(fmt.Sprintf("上下文压缩 (%s): %d → %d tokens", strategy, beforeTokens, afterTokens))
		if compressJSON, err := json.Marshal(payload); err == nil {
			_ = s.writer.Write(llm.SSEEvent{
				Type:    "context_compressed",
				Content: string(compressJSON),
			})
		}

	case agentruntime.HookContextCompressionFailed:
		payload := toMap(event.Payload)
		errMsg, _ := payload["error"].(string)
		s.writeThinking(fmt.Sprintf("上下文压缩失败: %s", errMsg))
		_ = s.writer.Write(llm.SSEEvent{
			Type:  "context_compression_failed",
			Error: errMsg,
		})
	}

	return nil
}

func (s *SSEHookSink) writeThinking(text string) {
	if text == "" {
		return
	}
	_ = s.writer.WriteThinking(text)
	if s.collector != nil {
		s.collector.AddThinking(text)
	}
}

// toMap converts an arbitrary payload (typically map[string]interface{} after
// JSON round-trip, or a concrete struct) into map[string]interface{}.
func toMap(payload any) map[string]interface{} {
	if payload == nil {
		return nil
	}
	if m, ok := payload.(map[string]interface{}); ok {
		return m
	}
	// Fall back to JSON marshal/unmarshal for struct payloads.
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

func buildAuditSummary(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	decision, _ := payload["decision"].(string)
	riskLevel, _ := payload["risk_level"].(string)
	findings := payload["findings"]

	b, _ := json.Marshal(map[string]interface{}{
		"decision":   decision,
		"risk_level": riskLevel,
		"findings":   findings,
	})
	return string(b)
}

func buildReflectionSummary(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	rootCause, _ := payload["root_cause"].(string)
	recommendation, _ := payload["recommendation"].(string)

	b, _ := json.Marshal(map[string]interface{}{
		"root_cause":     rootCause,
		"recommendation": recommendation,
	})
	return string(b)
}

func buildCorrectionSummary(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	reason, _ := payload["reason"].(string)
	actions := payload["actions"]

	b, _ := json.Marshal(map[string]interface{}{
		"reason":  reason,
		"actions": actions,
	})
	return string(b)
}

// findStepTitle looks up the step title from the plan in the event snapshot.
func findStepTitle(event agentruntime.HookEvent, stepID string) string {
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
