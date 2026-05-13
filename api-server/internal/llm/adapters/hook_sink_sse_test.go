package adapters

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	agentruntime "github.com/chenchen511/agent-runtime"

	"api-server/internal/llm"
)

type fakeEventCollector struct {
	thinking string
	content  string
}

func (c *fakeEventCollector) AddThinking(text string) {
	c.thinking += text
}

func (c *fakeEventCollector) AddToolCall(tool, callID string, args interface{}) {}
func (c *fakeEventCollector) AddToolResult(callID string, result interface{})   {}
func (c *fakeEventCollector) AddToolError(callID, errMsg string)                {}
func (c *fakeEventCollector) SetContent(content string)                         { c.content = content }
func (c *fakeEventCollector) SetPlan(plan interface{})                          {}
func (c *fakeEventCollector) AddAudit(audit interface{})                        {}
func (c *fakeEventCollector) AddReflection(reflection interface{})              {}
func (c *fakeEventCollector) AddCorrection(correction interface{})              {}

func TestSSEHookSinkStepFailedIsDistinctAndCollected(t *testing.T) {
	recorder := httptest.NewRecorder()
	collector := &fakeEventCollector{}
	sink := NewSSEHookSink(llm.NewSSEWriter(recorder), collector)

	err := sink.Handle(context.Background(), agentruntime.HookEvent{
		Type:   agentruntime.HookStepFailed,
		StepID: "step-1",
		Snapshot: &agentruntime.TaskSnapshot{
			CurrentPlan: &agentruntime.Plan{
				Steps: []agentruntime.PlanStep{
					{StepID: "step-1", Title: "查询进程树"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("handle step failed: %v", err)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, `"type":"step_failed"`) {
		t.Fatalf("expected step_failed SSE event, body: %s", body)
	}
	if strings.Contains(body, `"type":"step_completed"`) {
		t.Fatalf("step_failed must not be emitted as step_completed, body: %s", body)
	}
	if !strings.Contains(collector.thinking, "步骤失败: 查询进程树") {
		t.Fatalf("expected visible thinking summary to be collected, got %q", collector.thinking)
	}
}

func TestSSEHookSinkTaskFinishedDoesNotCloseStreamEarly(t *testing.T) {
	recorder := httptest.NewRecorder()
	collector := &fakeEventCollector{}
	sink := NewSSEHookSink(llm.NewSSEWriter(recorder), collector)

	err := sink.Handle(context.Background(), agentruntime.HookEvent{
		Type: agentruntime.HookTaskFinished,
		Payload: agentruntime.TaskResult{
			FinalAnswer: `{"attack_graph":{"summary":"确认攻击链"},"conclusions":[]}`,
		},
	})
	if err != nil {
		t.Fatalf("handle task finished: %v", err)
	}

	body := recorder.Body.String()
	if strings.Contains(body, `"type":"done"`) || strings.Contains(body, `"type":"content"`) {
		t.Fatalf("task_finished hook must not emit final content/done before handler persistence, body: %s", body)
	}
	if !strings.Contains(collector.content, "attack_graph") {
		t.Fatalf("final answer should still be collected, got %q", collector.content)
	}
}
