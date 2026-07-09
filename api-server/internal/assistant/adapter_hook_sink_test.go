package assistant

import (
	"context"
	"strings"
	"testing"
	"time"

	"api-server/internal/model"
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

func TestAssistantHookSinkDoesNotPublishModelOutputAsMessageDelta(t *testing.T) {
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

	select {
	case event := <-ch:
		t.Fatalf("expected no event for intermediate model output, got %q", event.Type)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestAssistantHookSinkPublishesPreGatewayValidationFailure(t *testing.T) {
	manager := NewRunManager()
	run := manager.Start("session-1")
	sink := NewAssistantHookSink(manager, "session-1", run.RunID, "msg-1", zap.NewNop())

	err := sink.Handle(context.Background(), agentruntime.HookEvent{
		Type: agentruntime.HookToolCallFinished,
		Payload: map[string]interface{}{
			"call_id":          "call-invalid",
			"tool_name":        "Vulnerability.GenerateShell",
			"status":           string(agentruntime.ToolCallFailed),
			"error_message":    "tool not found",
			"validation_stage": "descriptor",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch, unsubscribe, err := manager.Subscribe("session-1")
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	event := receiveEvent(t, ch)
	if event.Type != EventToolError {
		t.Fatalf("event type = %q", event.Type)
	}
	payload := toMap(event.Payload)
	if payload["validation_stage"] != "descriptor" {
		t.Fatalf("validation_stage = %#v", payload["validation_stage"])
	}
}

func TestAssistantHookSinkPublishesStepCompletedWithResultSummary(t *testing.T) {
	manager := NewRunManager()
	run := manager.Start("session-1")
	sink := NewAssistantHookSink(manager, "session-1", run.RunID, "msg-1", zap.NewNop())

	err := sink.Handle(context.Background(), agentruntime.HookEvent{
		Type:   agentruntime.HookModelCallFinished,
		StepID: "step-1",
		Payload: map[string]interface{}{
			"purpose":        string(agentruntime.PurposeReact),
			"output_summary": `{"action":"step_result","summary":"主机状态采集完成","step_result":{"result":"已获取 2 台主机，2 台在线","evidence":["Host.List","Host.AgentStatus.Get"],"confidence":"high"}}`,
		},
	})
	if err != nil {
		t.Fatalf("Handle model output returned error: %v", err)
	}

	err = sink.Handle(context.Background(), agentruntime.HookEvent{
		Type:   agentruntime.HookStepCompleted,
		StepID: "step-1",
		Snapshot: &agentruntime.TaskSnapshot{
			CurrentPlan: &agentruntime.Plan{
				Steps: []agentruntime.PlanStep{{
					StepID: "step-1",
					Title:  "获取主机资产和在线状态",
				}},
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

	stepEvent := receiveEvent(t, ch)
	if stepEvent.Type != EventStepCompleted {
		t.Fatalf("expected %q event, got %q", EventStepCompleted, stepEvent.Type)
	}
	payload := toMap(stepEvent.Payload)
	if payload["result_summary"] != "已获取 2 台主机，2 台在线" {
		t.Fatalf("result_summary = %#v", payload["result_summary"])
	}

	thinking := receiveEvent(t, ch)
	if thinking.Type != EventThinking {
		t.Fatalf("expected thinking event after step_completed, got %q", thinking.Type)
	}
}

func TestAssistantHookSinkPublishesFailedAndRetryingStepStatus(t *testing.T) {
	manager := NewRunManager()
	run := manager.Start("session-1")
	sink := NewAssistantHookSink(manager, "session-1", run.RunID, "msg-1", zap.NewNop())

	for _, testCase := range []struct {
		hook agentruntime.HookEventType
		want string
	}{
		{hook: agentruntime.HookStepFailed, want: EventStepFailed},
		{hook: agentruntime.HookStepRetrying, want: EventStepRetrying},
	} {
		err := sink.Handle(context.Background(), agentruntime.HookEvent{
			Type:   testCase.hook,
			StepID: "step-1",
			Snapshot: &agentruntime.TaskSnapshot{
				CurrentPlan: &agentruntime.Plan{
					Steps: []agentruntime.PlanStep{{
						StepID: "step-1",
						Title:  "获取主机资产和在线状态",
					}},
				},
			},
		})
		if err != nil {
			t.Fatalf("Handle returned error for %q: %v", testCase.hook, err)
		}
	}

	ch, unsubscribe, err := manager.Subscribe("session-1")
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	defer unsubscribe()

	for _, want := range []string{EventStepFailed, EventStepRetrying} {
		event := receiveEvent(t, ch)
		if event.Type != want {
			t.Fatalf("event type = %q, want %q", event.Type, want)
		}
	}
}

func TestAssistantHookSinkDoesNotPublishAuditEvents(t *testing.T) {
	manager := NewRunManager()
	run := manager.Start("session-1")
	sink := NewAssistantHookSink(manager, "session-1", run.RunID, "msg-1", zap.NewNop())

	for _, eventType := range []agentruntime.HookEventType{
		agentruntime.HookAuditStarted,
		agentruntime.HookAuditFinished,
	} {
		err := sink.Handle(context.Background(), agentruntime.HookEvent{
			Type: eventType,
			Payload: map[string]interface{}{
				"checked": true,
			},
		})
		if err != nil {
			t.Fatalf("Handle returned error for %q: %v", eventType, err)
		}
	}

	ch, unsubscribe, err := manager.Subscribe("session-1")
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	defer unsubscribe()

	select {
	case event := <-ch:
		t.Fatalf("expected no visible event for audit hooks, got %q", event.Type)
	case <-time.After(50 * time.Millisecond):
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

func TestAssistantHookSinkPersistsReflectionWithoutPublishingThinking(t *testing.T) {
	manager := NewRunManager()
	run := manager.Start("session-1")
	repo := &fakeAssistantMemoryRepo{}
	sink := NewAssistantHookSink(manager, "session-1", run.RunID, "msg-1", zap.NewNop()).
		WithMemoryRepository(repo)

	err := sink.Handle(context.Background(), agentruntime.HookEvent{
		Type:   agentruntime.HookReflectionFinished,
		TaskID: "task-1",
		StepID: "step-1",
		Payload: map[string]interface{}{
			"root_cause":     "工具参数缺失",
			"recommendation": "retry_step",
		},
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if len(repo.created) != 1 {
		t.Fatalf("expected one persisted reflection, got %d", len(repo.created))
	}
	if repo.created[0].MemoryType != assistantReflectionMemoryType {
		t.Fatalf("memory type = %q, want %q", repo.created[0].MemoryType, assistantReflectionMemoryType)
	}
	if repo.created[0].Content == "" || !strings.Contains(repo.created[0].Content, "工具参数缺失") {
		t.Fatalf("unexpected reflection content: %q", repo.created[0].Content)
	}

	ch, unsubscribe, err := manager.Subscribe("session-1")
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	defer unsubscribe()

	select {
	case event := <-ch:
		t.Fatalf("expected no visible reflection event, got %q", event.Type)
	case <-time.After(50 * time.Millisecond):
	}
}

type fakeAssistantMemoryRepo struct {
	created []model.AssistantMemory
}

func (f *fakeAssistantMemoryRepo) Create(_ context.Context, memory *model.AssistantMemory) error {
	f.created = append(f.created, *memory)
	return nil
}

func (f *fakeAssistantMemoryRepo) ListBySession(context.Context, string, string) ([]model.AssistantMemory, error) {
	return nil, nil
}

func (f *fakeAssistantMemoryRepo) DeleteBySession(context.Context, string) error {
	return nil
}
