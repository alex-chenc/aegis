package grpc_server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	pb "server/pkg/api/v1"
	"server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestValidateAgentGuardBlockCommandRejectsLegacyWildcardAndHostTargets(t *testing.T) {
	hostID := uuid.New()
	unitID := uuid.New()
	tests := []struct {
		name   string
		action string
		target string
		wantOK bool
	}{
		{name: "freeze unit", action: "freeze_execution_unit", target: unitID.String(), wantOK: true},
		{name: "resume unit", action: "resume_execution_unit", target: unitID.String(), wantOK: true},
		{name: "hold unit", action: "hold_execution_unit", target: unitID.String(), wantOK: true},
		{name: "kill unit", action: "kill_execution_unit", target: unitID.String(), wantOK: true},
		{name: "kill instance", action: "kill_agent_instance", target: unitID.String(), wantOK: true},
		{name: "legacy pid", action: "kill_process", target: "1234"},
		{name: "legacy path", action: "quarantine_file", target: "/tmp/file"},
		{name: "wildcard", action: "freeze_execution_unit", target: "*"},
		{name: "json target forbidden", action: "freeze_execution_unit", target: `{"execution_unit_id":"` + unitID.String() + `"}`},
		{name: "host level forbidden", action: "freeze_execution_unit", target: hostID.String()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateAgentGuardBlockCommand(hostID, &pb.BlockCommand{
				CommandId: "AG-GUARD-" + uuid.NewString(), HostId: hostID.String(),
				Action: test.action, Target: test.target, Reason: "published policy authorized",
			})
			if test.wantOK && err != nil {
				t.Fatalf("ValidateAgentGuardBlockCommand: %v", err)
			}
			if !test.wantOK && err == nil {
				t.Fatal("expected validation rejection")
			}
		})
	}
}

func TestAgentGuardBlockKafkaHandlerValidatesPartitionAndPreservesAgentFailure(t *testing.T) {
	hostID := uuid.New()
	unitID := uuid.New()
	reason := "freeze unavailable: cgroup v2 freezer is not delegated"
	producer := &captureRuntimeEventProducer{}
	server := &GRPCServer{kafkaProducer: producer}
	server.agentConnections.Store(hostID, &AgentConnection{
		HostID: hostID, CallbackClient: &failingAgentClient{reason: reason}, Ctx: context.Background(),
	})
	command := map[string]string{
		"command_id": "AG-GUARD-" + uuid.NewString(), "host_id": hostID.String(),
		"action": "freeze_execution_unit", "target": unitID.String(), "reason": "correlation policy",
	}
	payload, _ := json.Marshal(command)
	err := server.HandleAgentGuardBlockMessage(context.Background(), []byte(hostID.String()), payload)
	if err == nil || err.Error() != reason {
		t.Fatalf("failure reason=%v, want exact %q", err, reason)
	}
	if len(producer.events) != 1 || producer.events[0].event.EventType != "agent_guard_action_status" {
		t.Fatalf("failure status events=%#v", producer.events)
	}
	var status map[string]any
	if json.Unmarshal([]byte(producer.events[0].event.EventDataJson), &status) != nil ||
		status["status"] != "failed" || status["error_message"] != reason ||
		status["execution_unit_id"] != unitID.String() {
		t.Fatalf("failure status=%v", status)
	}
	if err := server.HandleAgentGuardBlockMessage(context.Background(), []byte(uuid.NewString()), payload); err == nil {
		t.Fatal("partition/host mismatch was accepted")
	}
}

func TestValidateAgentGuardBlockCommandRequiresActionIdentityCommand(t *testing.T) {
	hostID := uuid.New()
	err := ValidateAgentGuardBlockCommand(hostID, &pb.BlockCommand{
		CommandId: "legacy-command", HostId: hostID.String(),
		Action: "freeze_execution_unit", Target: uuid.NewString(),
	})
	if err == nil {
		t.Fatal("accepted command_id that cannot correlate an Agent action status")
	}
}

func TestExecuteBlockCommandLogsIdentifiersButNotTargetReasonOrPayload(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	previousLogger := logger.Logger
	logger.Logger = zap.New(core)
	t.Cleanup(func() { logger.Logger = previousLogger })

	hostID := uuid.New()
	unitID := uuid.New()
	server := &GRPCServer{}
	server.agentConnections.Store(hostID, &AgentConnection{
		HostID: hostID, CallbackClient: &successAgentClient{}, Ctx: context.Background(),
	})
	impl := &APIServerToServerImpl{grpcServer: server}
	reason := "evidence-secret-must-not-log"
	response, err := impl.ExecuteBlockCommand(context.Background(), &pb.ExecuteBlockCommandRequest{
		CommandId: "AG-GUARD-" + uuid.NewString(), HostId: hostID.String(),
		Action: "freeze_execution_unit", Target: unitID.String(), Reason: reason,
	})
	if err != nil || !response.Success {
		t.Fatalf("response=%#v err=%v", response, err)
	}
	for _, entry := range observed.All() {
		encoded, _ := json.Marshal(entry.ContextMap())
		text := entry.Message + string(encoded)
		if strings.Contains(text, unitID.String()) || strings.Contains(text, reason) {
			t.Fatalf("sensitive target/reason leaked into logs: %s", text)
		}
	}
}
