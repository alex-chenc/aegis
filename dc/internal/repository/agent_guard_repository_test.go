package repository

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"dc/internal/model"

	"github.com/google/uuid"
)

func TestMergeDeliveryStatusAllowsReconnectApplyWithoutRegression(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		incoming string
		want     string
	}{
		{name: "reconnect applies prior dispatch failure", current: "failed", incoming: "applied", want: "applied"},
		{name: "late received cannot regress applied", current: "applied", incoming: "received", want: "applied"},
		{name: "late failed cannot regress applied", current: "applied", incoming: "failed", want: "applied"},
		{name: "unsupported agent is terminal", current: "unsupported_agent_version", incoming: "applied", want: "unsupported_agent_version"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := mergeDeliveryStatus(test.current, test.incoming); got != test.want {
				t.Fatalf("mergeDeliveryStatus(%q, %q) = %q, want %q", test.current, test.incoming, got, test.want)
			}
		})
	}
}

func TestUnknownActionInsertIsRestrictedToRealTimeoutAutoResume(t *testing.T) {
	now := time.Now().UTC()
	actionID, hostID, instanceID, unitID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	valid := &model.AgentGuardAction{
		ID: actionID, CommandID: "AG-GUARD-" + actionID.String(), HostID: hostID,
		InstanceID: &instanceID, ExecutionUnitID: &unitID,
		Action: "auto_resume", Source: "timeout", Status: "success",
		Reason:      "automatic freeze timeout elapsed",
		Result:      json.RawMessage(`{"executed":true,"state_changed":true}`),
		RequestedAt: now, UpdatedAt: now,
	}
	if !isInsertableTimeoutAutoResume(valid) {
		t.Fatal("rejected valid timeout auto-resume")
	}
	tests := []struct {
		name   string
		mutate func(*model.AgentGuardAction)
	}{
		{name: "unknown manual freeze", mutate: func(a *model.AgentGuardAction) { a.Action = "freeze_execution_unit" }},
		{name: "manual source", mutate: func(a *model.AgentGuardAction) { a.Source = "manual" }},
		{name: "missing instance", mutate: func(a *model.AgentGuardAction) { a.InstanceID = nil }},
		{name: "mismatched command identity", mutate: func(a *model.AgentGuardAction) { a.CommandID = "AG-GUARD-" + uuid.NewString() }},
		{name: "failed status", mutate: func(a *model.AgentGuardAction) { a.Status = "failed" }},
		{name: "no real transition evidence", mutate: func(a *model.AgentGuardAction) {
			a.Result = json.RawMessage(`{"executed":false,"state_changed":false}`)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copy := *valid
			test.mutate(&copy)
			if isInsertableTimeoutAutoResume(&copy) {
				t.Fatalf("accepted invalid timeout auto-resume: %#v", copy)
			}
		})
	}
}

func TestAgentGuardActionStatusNeverRegressesOrOverwritesTerminalEvidence(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name                  string
		current, incoming     string
		currentAt, incomingAt time.Time
		want                  bool
	}{
		{name: "pending advances to running", current: "pending", incoming: "running", currentAt: now, incomingAt: now.Add(time.Second), want: true},
		{name: "running cannot regress to dispatching", current: "running", incoming: "dispatching", currentAt: now, incomingAt: now.Add(time.Second)},
		{name: "older same state is replay", current: "dispatching", incoming: "dispatching", currentAt: now, incomingAt: now.Add(-time.Second)},
		{name: "running accepts real failure", current: "running", incoming: "failed", currentAt: now, incomingAt: now.Add(time.Second), want: true},
		{name: "success is terminal against late failure", current: "success", incoming: "failed", currentAt: now, incomingAt: now.Add(time.Second)},
		{name: "failure is terminal against fake success", current: "failed", incoming: "success", currentAt: now, incomingAt: now.Add(time.Second)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := shouldApplyActionStatus(
				&model.AgentGuardAction{Status: test.current, UpdatedAt: test.currentAt},
				&model.AgentGuardAction{Status: test.incoming, UpdatedAt: test.incomingAt},
			)
			if got != test.want {
				t.Fatalf("shouldApplyActionStatus(%s,%s)=%t want %t", test.current, test.incoming, got, test.want)
			}
		})
	}
}

func TestNormalizedAgentAssetAliasesCoverCollectorNames(t *testing.T) {
	if got := normalizedAgentAssetAliases("codex"); !slices.Equal(got, []string{"codex", "openaicodex"}) {
		t.Fatalf("Codex aliases = %v", got)
	}
	if got := normalizedAgentAssetAliases("claude-code"); !slices.Equal(got, []string{"claude", "claudecode"}) {
		t.Fatalf("Claude aliases = %v", got)
	}
}
