package assistant

import (
	"context"
	"testing"
)

func TestToolDecisionRecorderNilRepo(t *testing.T) {
	recorder := NewToolDecisionRecorder(nil, nil)
	// Should not panic
	err := recorder.Record(context.Background(), "session-1", &ToolExecutionPlan{
		DecisionTraceID: "td_test",
		DecisionRecords: []ToolDecisionRecord{{ToolName: "Host.List", Decision: toolDecisionAccepted}},
	})
	if err != nil {
		t.Fatalf("nil repo should return nil error, got: %v", err)
	}
}

func TestToolDecisionRecorderNilSessionID(t *testing.T) {
	recorder := NewToolDecisionRecorder(nil, nil)
	err := recorder.Record(context.Background(), "", &ToolExecutionPlan{DecisionTraceID: "td_test"})
	if err != nil {
		t.Fatalf("empty sessionID should return nil error, got: %v", err)
	}
}

func TestToolDecisionRecorderNilPlan(t *testing.T) {
	recorder := NewToolDecisionRecorder(nil, nil)
	err := recorder.Record(context.Background(), "session-1", nil)
	if err != nil {
		t.Fatalf("nil plan should return nil error, got: %v", err)
	}
}

func TestToolDecisionRecorderNilSelf(t *testing.T) {
	var recorder *ToolDecisionRecorder
	err := recorder.Record(context.Background(), "session-1", &ToolExecutionPlan{DecisionTraceID: "td_test"})
	if err != nil {
		t.Fatalf("nil recorder should return nil error, got: %v", err)
	}
}
