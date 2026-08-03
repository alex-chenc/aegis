package service

import (
	"context"
	"encoding/json"
	"testing"

	"api-server/internal/model"

	"github.com/google/uuid"
)

type fakeAgentGuardWSBroadcaster struct {
	messages []WSMessage
}

type fakeAgentGuardActionStatusUpdater struct {
	report AgentGuardActionStatusReport
	action *model.AgentGuardAction
	err    error
}

func (f *fakeAgentGuardActionStatusUpdater) ApplyReportedStatus(
	_ context.Context,
	report AgentGuardActionStatusReport,
) (*model.AgentGuardAction, error) {
	f.report = report
	return f.action, f.err
}

func (f *fakeAgentGuardWSBroadcaster) Broadcast(message WSMessage) {
	f.messages = append(f.messages, message)
}

func TestAgentGuardRealtimeHandlerBroadcastsOnlyBoundedTypedSummary(t *testing.T) {
	broadcaster := &fakeAgentGuardWSBroadcaster{}
	handler := NewAgentGuardRealtimeHandler(broadcaster, nil)
	raw, err := json.Marshal(map[string]any{
		"event_id":   "event-1",
		"host_id":    "host-1",
		"event_type": "agent_behavior",
		"severity":   "high",
		"event_data": `{
			"instance_id":"instance-1",
			"session_id":"session-1",
			"execution_unit_id":"unit-1",
			"category":"process",
			"operation":"exec",
			"actor":{"argv":["curl","--token","TEST_SECRET"]},
			"resource":{"identity":"/sensitive/path"}
		}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := handler.HandleKafkaMessage(context.Background(), nil, raw); err != nil {
		t.Fatalf("HandleKafkaMessage: %v", err)
	}
	if len(broadcaster.messages) != 1 || broadcaster.messages[0].Type != "agent_guard.behavior_created" {
		t.Fatalf("unexpected messages: %#v", broadcaster.messages)
	}
	encoded, _ := json.Marshal(broadcaster.messages[0])
	for _, secret := range []string{"TEST_SECRET", "/sensitive/path", "argv", "resource"} {
		if stringContains(string(encoded), secret) {
			t.Fatalf("websocket summary leaked %q: %s", secret, encoded)
		}
	}
}

func TestAgentGuardRealtimeHandlerIgnoresUnrelatedAndMalformedEvents(t *testing.T) {
	broadcaster := &fakeAgentGuardWSBroadcaster{}
	handler := NewAgentGuardRealtimeHandler(broadcaster, nil)

	if err := handler.HandleKafkaMessage(context.Background(), nil, []byte(`{"event_type":"process_exec"}`)); err != nil {
		t.Fatalf("unrelated event: %v", err)
	}
	if len(broadcaster.messages) != 0 {
		t.Fatalf("unrelated event was broadcast: %#v", broadcaster.messages)
	}
	if err := handler.HandleKafkaMessage(context.Background(), nil, []byte(`not-json`)); err == nil {
		t.Fatal("malformed envelope must return an error")
	}
}

func TestAgentGuardRealtimeHandlerRefreshesForSessionLifecycle(t *testing.T) {
	broadcaster := &fakeAgentGuardWSBroadcaster{}
	handler := NewAgentGuardRealtimeHandler(broadcaster, nil)
	raw := []byte(`{
		"event_id":"event-session-1",
		"host_id":"host-1",
		"event_type":"agent_behavior_session_started",
		"event_data_json":"{\"instance_id\":\"instance-1\",\"session_id\":\"session-1\",\"status\":\"active\"}"
	}`)

	if err := handler.HandleKafkaMessage(context.Background(), nil, raw); err != nil {
		t.Fatalf("HandleKafkaMessage: %v", err)
	}
	if len(broadcaster.messages) != 1 ||
		broadcaster.messages[0].Type != "agent_guard.instance_updated" {
		t.Fatalf("unexpected messages: %#v", broadcaster.messages)
	}
}

func TestAgentGuardRealtimeHandlerPersistsValidatedActionBeforeBoundedBroadcast(t *testing.T) {
	broadcaster := &fakeAgentGuardWSBroadcaster{}
	actionID, hostID, instanceID, unitID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	updater := &fakeAgentGuardActionStatusUpdater{action: &model.AgentGuardAction{
		ID: actionID, CommandID: "AG-GUARD-" + actionID.String(), HostID: hostID,
		InstanceID: &instanceID, ExecutionUnitID: &unitID,
		Action: model.AgentGuardActionFreezeExecutionUnit, Status: model.AgentGuardActionStatusSuccess,
	}}
	handler := NewAgentGuardRealtimeHandler(broadcaster, nil)
	handler.SetActionStatusUpdater(updater)
	raw, err := json.Marshal(map[string]any{
		"event_id": "event-action-1", "host_id": hostID.String(),
		"event_type": "agent_guard_action_status", "severity": "info",
		"event_data": map[string]any{
			"schema": agentGuardActionStatusSchema, "action_id": actionID.String(),
			"command_id": "AG-GUARD-" + actionID.String(), "action": model.AgentGuardActionFreezeExecutionUnit,
			"status": model.AgentGuardActionStatusSuccess, "instance_id": instanceID.String(),
			"execution_unit_id": unitID.String(), "method": "cgroup_v2", "executed": true,
			"state_changed": true, "error_message": "must-not-be-broadcast",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.HandleKafkaMessage(context.Background(), nil, raw); err != nil {
		t.Fatalf("HandleKafkaMessage: %v", err)
	}
	if updater.report.HostID != hostID.String() || updater.report.ActionID != actionID.String() ||
		!updater.report.Executed || !updater.report.StateChanged {
		t.Fatalf("unvalidated status report: %#v", updater.report)
	}
	if len(broadcaster.messages) != 1 || broadcaster.messages[0].Type != "agent_guard.action_updated" {
		t.Fatalf("unexpected messages: %#v", broadcaster.messages)
	}
	encoded, _ := json.Marshal(broadcaster.messages[0])
	if stringContains(string(encoded), "must-not-be-broadcast") ||
		!stringContains(string(encoded), actionID.String()) {
		t.Fatalf("unsafe or incomplete action summary: %s", encoded)
	}
}

func stringContains(value, needle string) bool {
	for index := 0; index+len(needle) <= len(value); index++ {
		if value[index:index+len(needle)] == needle {
			return true
		}
	}
	return false
}
