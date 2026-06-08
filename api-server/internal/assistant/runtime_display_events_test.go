package assistant

import "testing"

func TestCompactRuntimeDisplayEventsKeepsToolCallMetadataOnThinking(t *testing.T) {
	events := []AssistantEvent{
		NewEvent(EventThinking, "session-1", "run-1", map[string]interface{}{
			"content":   "正在调用工具: Host.List",
			"call_id":   "call-1",
			"tool_name": "Host.List",
		}),
		EventToolCallPayload("session-1", "run-1", "msg-1", "call-1", "Host.List", map[string]interface{}{}),
	}

	compacted := compactRuntimeDisplayEvents(events, "run-1", "msg-1")

	if len(compacted) != 2 {
		t.Fatalf("expected 2 compacted events, got %d", len(compacted))
	}
	thinking := compacted[0]
	if thinking.Type != EventThinking {
		t.Fatalf("first compacted event = %q, want thinking", thinking.Type)
	}
	if thinking.Payload["call_id"] != "call-1" {
		t.Fatalf("thinking call_id = %#v, want call-1", thinking.Payload["call_id"])
	}
	if thinking.Payload["tool_name"] != "Host.List" {
		t.Fatalf("thinking tool_name = %#v, want Host.List", thinking.Payload["tool_name"])
	}
}

func TestCompactRuntimeDisplayEventsKeepsPlanAndStepEvents(t *testing.T) {
	events := []AssistantEvent{
		EventPlanPayload("session-1", "run-1", "msg-1", map[string]interface{}{
			"goal": "分析全部主机风险",
			"steps": []map[string]interface{}{{
				"step_id": "step-1",
				"title":   "定位全部主机",
				"status":  "pending",
			}},
		}),
		withMessageID(NewEvent(EventStepStarted, "session-1", "run-1", map[string]interface{}{
			"step_id": "step-1",
			"title":   "定位全部主机",
		}), "msg-1"),
		withMessageID(NewEvent(EventStepCompleted, "session-1", "run-1", map[string]interface{}{
			"step_id":        "step-1",
			"title":          "定位全部主机",
			"result_summary": "发现 2 台目标主机",
		}), "msg-1"),
	}

	compacted := compactRuntimeDisplayEvents(events, "run-1", "msg-1")

	if len(compacted) != 3 {
		t.Fatalf("expected 3 compacted events, got %d", len(compacted))
	}
	if compacted[0].Type != EventPlan {
		t.Fatalf("first compacted event = %q, want plan", compacted[0].Type)
	}
	if compacted[0].Payload["goal"] != "分析全部主机风险" {
		t.Fatalf("plan goal = %#v", compacted[0].Payload["goal"])
	}
	if compacted[1].Type != EventStepStarted {
		t.Fatalf("second compacted event = %q, want step_started", compacted[1].Type)
	}
	if compacted[2].Type != EventStepCompleted {
		t.Fatalf("third compacted event = %q, want step_completed", compacted[2].Type)
	}
	if compacted[2].Payload["result_summary"] != "发现 2 台目标主机" {
		t.Fatalf("step result summary = %#v", compacted[2].Payload["result_summary"])
	}
}
