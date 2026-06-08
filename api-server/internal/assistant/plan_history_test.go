package assistant

import (
	"encoding/json"
	"testing"
)

func TestExtractPlanFromEventsAppliesStepCompletedResult(t *testing.T) {
	events := []AssistantEvent{
		NewEvent(EventPlan, "session-1", "run-1", map[string]interface{}{
			"goal":   "分析平台安全态势",
			"status": "running",
			"steps": []map[string]interface{}{{
				"step_id": "step-1",
				"title":   "获取整体安全态势统计",
				"status":  "pending",
			}},
		}),
		NewEvent(EventStepCompleted, "session-1", "run-1", map[string]interface{}{
			"step_id":        "step-1",
			"title":          "获取整体安全态势统计",
			"result_summary": "统计完成：发现 0 条今日告警",
		}),
	}

	planJSON := extractPlanFromEvents(events)
	if len(planJSON) == 0 {
		t.Fatal("expected plan json")
	}

	var plan struct {
		Steps []struct {
			StepID        string `json:"step_id"`
			Status        string `json:"status"`
			ResultSummary string `json:"result_summary"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected one step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].Status != "completed" {
		t.Fatalf("step status = %q", plan.Steps[0].Status)
	}
	if plan.Steps[0].ResultSummary != "统计完成：发现 0 条今日告警" {
		t.Fatalf("result_summary = %q", plan.Steps[0].ResultSummary)
	}
}
