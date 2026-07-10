package assistant

import (
	"testing"

	"api-server/internal/model"
	agentruntime "github.com/alex-chenc/agent-runtime"
)

func TestSessionStatusForGoalOutcomeDoesNotCollapseFailureIntoCompleted(t *testing.T) {
	tests := []struct {
		name    string
		outcome agentruntime.GoalOutcome
		want    string
	}{
		{name: "succeeded", outcome: agentruntime.GoalSucceeded, want: model.SessionStatusCompleted},
		{name: "partial", outcome: agentruntime.GoalPartiallySucceeded, want: model.SessionStatusCompleted},
		{name: "needs input", outcome: agentruntime.GoalNeedsInput, want: model.SessionStatusActive},
		{name: "failed", outcome: agentruntime.GoalFailed, want: model.SessionStatusFailed},
		{name: "missing", outcome: "", want: model.SessionStatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sessionStatusForGoalOutcome(tt.outcome); got != tt.want {
				t.Fatalf("sessionStatusForGoalOutcome(%q) = %q, want %q", tt.outcome, got, tt.want)
			}
		})
	}
}
