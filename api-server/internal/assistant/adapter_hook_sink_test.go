package assistant

import (
	"context"
	"testing"
	"time"

	agentruntime "github.com/alex-chenc/agent-runtime"
	"go.uber.org/zap"
)

func TestAssistantHookSinkPublishesContextBudgetEvents(t *testing.T) {
	manager := NewRunManager()
	run := manager.Start("session-1")
	sink := NewAssistantHookSink(manager, "session-1", run.RunID, "msg-1", zap.NewNop())

	err := sink.Handle(context.Background(), agentruntime.HookEvent{
		Type: agentruntime.HookContextBudgetChecked,
		Snapshot: &agentruntime.TaskSnapshot{
			ContextBudget: &agentruntime.ContextBudgetSnapshot{
				MaxContextTokens:      256000,
				ReservedOutputTokens:  8192,
				EstimatedPromptTokens: 12000,
				ContextRatio:          0.08,
				TotalTokens:           14000,
			},
		},
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	ch, unsubscribe, err := manager.Subscribe("session-1")
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	defer unsubscribe()

	event := receiveEvent(t, ch)
	if event.Type != EventContextBudget {
		t.Fatalf("expected %q event, got %q", EventContextBudget, event.Type)
	}
	if event.MessageID != "msg-1" {
		t.Fatalf("expected message id to be preserved, got %q", event.MessageID)
	}
}

func TestAssistantHookSinkPublishesCompressionEvents(t *testing.T) {
	manager := NewRunManager()
	run := manager.Start("session-1")
	sink := NewAssistantHookSink(manager, "session-1", run.RunID, "msg-1", zap.NewNop())

	err := sink.Handle(context.Background(), agentruntime.HookEvent{
		Type: agentruntime.HookContextCompressed,
		Payload: map[string]interface{}{
			"strategy":       "tool_result",
			"trigger_ratio":  0.72,
			"before_tokens":  float64(120000),
			"after_tokens":   float64(64000),
			"compression_id": "cmp-1",
			"created_at":     time.Now().UTC(),
		},
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	ch, unsubscribe, err := manager.Subscribe("session-1")
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	defer unsubscribe()

	thinking := receiveEvent(t, ch)
	if thinking.Type != EventThinking {
		t.Fatalf("expected thinking event before compression event, got %q", thinking.Type)
	}
	compressed := receiveEvent(t, ch)
	if compressed.Type != EventContextCompressed {
		t.Fatalf("expected %q event, got %q", EventContextCompressed, compressed.Type)
	}
}

func TestAssistantHookSinkPublishesModelOutputAsMessageDelta(t *testing.T) {
	manager := NewRunManager()
	run := manager.Start("session-1")
	sink := NewAssistantHookSink(manager, "session-1", run.RunID, "msg-1", zap.NewNop())

	err := sink.Handle(context.Background(), agentruntime.HookEvent{
		Type: agentruntime.HookModelCallFinished,
		Payload: map[string]interface{}{
			"purpose":        string(agentruntime.PurposeReact),
			"output_summary": `{"action":"step_result","summary":"完成总结","step_result":{"result":"主机状态正常","evidence":[],"confidence":"high"}}`,
		},
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	ch, unsubscribe, err := manager.Subscribe("session-1")
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	defer unsubscribe()

	event := receiveEvent(t, ch)
	if event.Type != EventMessageDelta {
		t.Fatalf("expected %q event, got %q", EventMessageDelta, event.Type)
	}
	if event.MessageID != "msg-1" {
		t.Fatalf("expected message id to be preserved, got %q", event.MessageID)
	}
	payload, ok := event.Payload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %#v", event.Payload)
	}
	if payload["delta"] != "主机状态正常" {
		t.Fatalf("expected parsed model output, got %#v", payload["delta"])
	}
}

func TestAssistantHookSinkSkipsPlanModelOutput(t *testing.T) {
	manager := NewRunManager()
	run := manager.Start("session-1")
	sink := NewAssistantHookSink(manager, "session-1", run.RunID, "msg-1", zap.NewNop())

	err := sink.Handle(context.Background(), agentruntime.HookEvent{
		Type: agentruntime.HookModelCallFinished,
		Payload: map[string]interface{}{
			"purpose":        string(agentruntime.PurposePlan),
			"output_summary": `{"goal":"plan output should use plan event"}`,
		},
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	ch, unsubscribe, err := manager.Subscribe("session-1")
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	defer unsubscribe()

	select {
	case event := <-ch:
		t.Fatalf("expected no event for plan model output, got %q", event.Type)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestAssistantHookSinkSkipsReflectionModelOutput(t *testing.T) {
	manager := NewRunManager()
	run := manager.Start("session-1")
	sink := NewAssistantHookSink(manager, "session-1", run.RunID, "msg-1", zap.NewNop())

	err := sink.Handle(context.Background(), agentruntime.HookEvent{
		Type: agentruntime.HookModelCallFinished,
		Payload: map[string]interface{}{
			"purpose":        string(agentruntime.PurposeReflect),
			"output_summary": `{"root_cause":"工具参数缺失","recommendation":"继续尝试"}`,
		},
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	ch, unsubscribe, err := manager.Subscribe("session-1")
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	defer unsubscribe()

	select {
	case event := <-ch:
		t.Fatalf("expected no event for reflection model output, got %q", event.Type)
	case <-time.After(50 * time.Millisecond):
	}
}
