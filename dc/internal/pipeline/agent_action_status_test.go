package pipeline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNormalizeAgentGuardActionStatusPreservesRealFailure(t *testing.T) {
	hostID := uuid.New()
	actionID := uuid.NewString()
	unitID := uuid.NewString()
	projection, err := NormalizeAgentGuardState(
		"agent_guard_action_status",
		hostID,
		`{"schema":"aegis.agent_guard.v1","action_id":"`+actionID+
			`","command_id":"AG-GUARD-`+actionID+`","execution_unit_id":"`+unitID+
			`","action":"freeze_execution_unit","status":"failed",`+
			`"error_code":"cgroup_freezer_unavailable",`+
			`"error_message":"cgroup v2 freezer is not delegated",`+
			`"result":{"state_changed":false},"occurred_at":"2026-07-30T10:00:00Z"}`,
	)
	if err != nil {
		t.Fatalf("NormalizeAgentGuardState: %v", err)
	}
	if projection.Action == nil || projection.Action.ID.String() != actionID ||
		projection.Action.Status != "failed" ||
		projection.Action.ErrorMessage != "cgroup v2 freezer is not delegated" {
		t.Fatalf("action projection=%#v", projection.Action)
	}
}

func TestNormalizeAgentGuardActionStatusRejectsFakeSuccessAndHostScope(t *testing.T) {
	actionID := uuid.NewString()
	for _, raw := range []string{
		`{"schema":"aegis.agent_guard.v1","action_id":"` + actionID +
			`","command_id":"AG-GUARD-` + actionID + `","execution_unit_id":"` + uuid.NewString() +
			`","action":"freeze_execution_unit","status":"success","executed":false,"state_changed":false,` +
			`"occurred_at":"2026-07-30T10:00:00Z"}`,
		`{"schema":"aegis.agent_guard.v1","action_id":"` + uuid.NewString() +
			`","execution_unit_id":"*","action":"freeze_execution_unit","status":"success",` +
			`"result":{"state_changed":true},"occurred_at":"2026-07-30T10:00:00Z"}`,
	} {
		if _, err := NormalizeAgentGuardState("agent_guard_action_status", uuid.New(), raw); err == nil {
			t.Fatalf("accepted invalid action status: %s", raw)
		}
	}
}

func TestNormalizeAgentGuardActionStatusAcceptsAgentTopLevelExecutionEvidence(t *testing.T) {
	actionID, unitID := uuid.NewString(), uuid.NewString()
	projection, err := NormalizeAgentGuardState(
		"agent_guard_action_status",
		uuid.New(),
		`{"schema":"aegis.agent_guard.v1","action_id":"`+actionID+
			`","command_id":"AG-GUARD-`+actionID+`","execution_unit_id":"`+unitID+
			`","action":"freeze_execution_unit","status":"success",`+
			`"method":"cgroup_v2_freezer","executed":true,"state_changed":true,`+
			`"occurred_at":"2026-07-30T10:00:00Z"}`,
	)
	if err != nil {
		t.Fatalf("NormalizeAgentGuardState: %v", err)
	}
	var result map[string]any
	if projection.Action == nil || json.Unmarshal(projection.Action.Result, &result) != nil ||
		result["executed"] != true || result["state_changed"] != true ||
		result["method"] != "cgroup_v2_freezer" {
		t.Fatalf("action=%#v result=%v", projection.Action, result)
	}
}

func TestNormalizeAgentGuardActionStatusRejectsMismatchedCommandIdentity(t *testing.T) {
	actionID, otherID, unitID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	_, err := NormalizeAgentGuardState(
		"agent_guard_action_status",
		uuid.New(),
		`{"schema":"aegis.agent_guard.v1","action_id":"`+actionID+
			`","command_id":"AG-GUARD-`+otherID+`","execution_unit_id":"`+unitID+
			`","action":"freeze_execution_unit","status":"failed",`+
			`"error_code":"freezer_unavailable","occurred_at":"2026-07-30T10:00:00Z"}`,
	)
	if err == nil {
		t.Fatal("accepted mismatched action_id and command_id")
	}
}

func TestNormalizeUnknownTimeoutAutoResumeRequiresCompleteRealEvidence(t *testing.T) {
	actionID, instanceID, unitID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	valid := `{"schema":"aegis.agent_guard.v1","action_id":"` + actionID +
		`","command_id":"AG-GUARD-` + actionID + `","instance_id":"` + instanceID +
		`","execution_unit_id":"` + unitID + `","action":"auto_resume","status":"success",` +
		`"method":"cgroup_v2_freezer","auto_resume":true,"executed":true,"state_changed":true,` +
		`"occurred_at":"2026-07-30T10:00:00Z"}`
	projection, err := NormalizeAgentGuardState("agent_guard_action_status", uuid.New(), valid)
	if err != nil {
		t.Fatalf("NormalizeAgentGuardState: %v", err)
	}
	if projection.Action == nil || projection.Action.Source != "timeout" ||
		projection.Action.Reason != "automatic freeze timeout elapsed" ||
		projection.Action.InstanceID == nil || projection.Action.ExecutionUnitID == nil {
		t.Fatalf("auto resume projection=%#v", projection.Action)
	}

	for _, invalid := range []string{
		strings.Replace(valid, `"instance_id":"`+instanceID+`",`, "", 1),
		strings.Replace(valid, `"status":"success"`, `"status":"failed"`, 1),
		strings.Replace(valid, `"executed":true,"state_changed":true`, `"executed":false,"state_changed":false`, 1),
	} {
		if _, err := NormalizeAgentGuardState("agent_guard_action_status", uuid.New(), invalid); err == nil {
			t.Fatalf("accepted invalid timeout auto-resume: %s", invalid)
		}
	}
}
