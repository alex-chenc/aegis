package assistant

import (
	"testing"
	"time"
)

func TestRunManagerSubscribeReplaysActiveRunHistory(t *testing.T) {
	manager := NewRunManager()
	run := manager.Start("session-1")

	first := NewEvent(EventThinking, "session-1", run.RunID, map[string]interface{}{"content": "step 1"})
	second := NewEvent(EventMessageDelta, "session-1", run.RunID, map[string]interface{}{"delta": "hello"})
	manager.Publish("session-1", first)
	manager.Publish("session-1", second)

	ch, unsubscribe, err := manager.Subscribe("session-1")
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	defer unsubscribe()

	gotFirst := receiveEvent(t, ch)
	if gotFirst.Type != EventThinking {
		t.Fatalf("expected replayed first event %q, got %q", EventThinking, gotFirst.Type)
	}
	gotSecond := receiveEvent(t, ch)
	if gotSecond.Type != EventMessageDelta {
		t.Fatalf("expected replayed second event %q, got %q", EventMessageDelta, gotSecond.Type)
	}
}

func TestRunManagerSupportsMultipleSubscribers(t *testing.T) {
	manager := NewRunManager()
	run := manager.Start("session-1")

	ch1, unsubscribe1, err := manager.Subscribe("session-1")
	if err != nil {
		t.Fatalf("Subscribe #1 returned error: %v", err)
	}
	defer unsubscribe1()

	ch2, unsubscribe2, err := manager.Subscribe("session-1")
	if err != nil {
		t.Fatalf("Subscribe #2 returned error: %v", err)
	}
	defer unsubscribe2()

	event := NewEvent(EventThinking, "session-1", run.RunID, map[string]interface{}{"content": "live"})
	manager.Publish("session-1", event)

	if got := receiveEvent(t, ch1); got.Type != EventThinking {
		t.Fatalf("subscriber #1 expected %q, got %q", EventThinking, got.Type)
	}
	if got := receiveEvent(t, ch2); got.Type != EventThinking {
		t.Fatalf("subscriber #2 expected %q, got %q", EventThinking, got.Type)
	}
}

func TestRunManagerStartDoesNotInstallFiveMinuteDeadline(t *testing.T) {
	manager := NewRunManager()
	run := manager.Start("session-1")

	if deadline, ok := run.Context().Deadline(); ok {
		t.Fatalf("expected run context without RunManager deadline, got %s", deadline)
	}
}

func receiveEvent(t *testing.T, ch <-chan AssistantEvent) AssistantEvent {
	t.Helper()
	select {
	case event := <-ch:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for assistant event")
		return AssistantEvent{}
	}
}
