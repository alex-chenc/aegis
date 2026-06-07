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
