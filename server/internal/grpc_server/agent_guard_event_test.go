package grpc_server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"server/internal/queue"
	pb "server/pkg/api/v1"
	"server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type capturedAgentGuardEvent struct {
	metadata queue.SecurityEventMetadata
	event    *pb.RuntimeEvent
}

type captureRuntimeEventProducer struct {
	events []capturedAgentGuardEvent
	err    error
}

func (p *captureRuntimeEventProducer) SendRawEventWithContext(
	_ context.Context,
	metadata queue.SecurityEventMetadata,
	value interface{},
) error {
	if p.err != nil {
		return p.err
	}
	event, ok := value.(*pb.RuntimeEvent)
	if !ok {
		return errors.New("unexpected event type")
	}
	p.events = append(p.events, capturedAgentGuardEvent{metadata: metadata, event: event})
	return nil
}

func TestReportEventForwardsAgentGuardEnvelopeWithStableKafkaContext(t *testing.T) {
	hostID := uuid.NewString()
	instanceID := uuid.NewString()
	hostBootID := uuid.NewString()
	producer := &captureRuntimeEventProducer{}
	server := &GRPCServer{kafkaProducer: producer}
	payload := `{
		"schema":"aegis.agent_behavior.v1",
		"event_id":"payload-event-id",
		"host_boot_id":"` + hostBootID + `",
		"agent_sequence":42,
		"agent":{"instance_id":"` + instanceID + `"},
		"actor":{"argv":["--token=do-not-log"]},
		"resource":{"identity":"/secret/path"}
	}`

	response, err := server.ReportEvent(context.Background(), &pb.ReportEventRequest{
		HostId: hostID,
		Events: []*pb.RuntimeEvent{{
			EventType:     "agent_behavior",
			EventDataJson: payload,
			MatchedRuleId: "AGB-BUILTIN-001",
		}},
	})
	if err != nil {
		t.Fatalf("ReportEvent: %v", err)
	}
	if !response.Success || response.ReceivedCount != 1 {
		t.Fatalf("response = %#v, want one accepted event", response)
	}
	if len(producer.events) != 1 {
		t.Fatalf("forwarded events = %d, want 1", len(producer.events))
	}
	forwarded := producer.events[0]
	if _, err := uuid.Parse(forwarded.event.EventId); err != nil {
		t.Fatalf("generated event ID %q is not a UUID: %v", forwarded.event.EventId, err)
	}
	if forwarded.event.HostId != hostID {
		t.Fatalf("forwarded host ID = %q, want %q", forwarded.event.HostId, hostID)
	}
	if forwarded.event.EventDataJson != payload {
		t.Fatal("Server changed Agent Guard event_data_json")
	}
	if forwarded.metadata.PartitionKey != hostID+":"+instanceID {
		t.Fatalf("partition key = %q", forwarded.metadata.PartitionKey)
	}
	if forwarded.metadata.EventID != forwarded.event.EventId ||
		forwarded.metadata.HostBootID != hostBootID ||
		forwarded.metadata.AgentSequence != "42" ||
		forwarded.metadata.Schema != "aegis.agent_behavior.v1" {
		t.Fatalf("unexpected idempotency metadata: %#v", forwarded.metadata)
	}
}

func TestReportEventForwardsAppliedAndRejectedStatusEventsWithStablePartition(t *testing.T) {
	hostID := uuid.NewString()
	producer := &captureRuntimeEventProducer{}
	server := &GRPCServer{kafkaProducer: producer}

	for _, status := range []string{"applied", "rejected"} {
		payload := `{"schema":"aegis.agent_guard.v1","bundle_version":7,"status":"` + status + `"}`
		response, err := server.ReportEvent(context.Background(), &pb.ReportEventRequest{
			HostId: hostID,
			Events: []*pb.RuntimeEvent{{
				EventId:       uuid.NewString(),
				EventType:     "agent_guard_config_status",
				EventDataJson: payload,
			}},
		})
		if err != nil || !response.Success || response.ReceivedCount != 1 {
			t.Fatalf("%s status ReportEvent response=%#v err=%v", status, response, err)
		}
		forwarded := producer.events[len(producer.events)-1]
		if forwarded.event.EventDataJson != payload {
			t.Fatalf("%s status payload changed during forwarding", status)
		}
		if got := forwarded.metadata.PartitionKey; got != hostID {
			t.Fatalf("%s status partition key = %q, want host ID", status, got)
		}
	}
}

func TestReportEventReportsKafkaFailureWithoutAcknowledgingTheEvent(t *testing.T) {
	hostID := uuid.NewString()
	producer := &captureRuntimeEventProducer{err: errors.New("kafka unavailable")}
	server := &GRPCServer{kafkaProducer: producer}
	response, err := server.ReportEvent(context.Background(), &pb.ReportEventRequest{
		HostId: hostID,
		Events: []*pb.RuntimeEvent{{
			EventId:       uuid.NewString(),
			EventType:     "agent_guard_health",
			EventDataJson: `not-json-but-must-be-forwarded`,
		}},
	})
	if err != nil {
		t.Fatalf("ReportEvent returned transport error: %v", err)
	}
	if response.Success || response.ReceivedCount != 0 {
		t.Fatalf("Kafka failure acknowledged as success: %#v", response)
	}
}

func TestAgentGuardEventsNeverEnterLegacyServerAlertPolicy(t *testing.T) {
	for _, eventType := range []string{
		"agent_behavior",
		"agent_guard_config_status",
		"agent_sandbox_violation",
		"agent_guard_action_status",
	} {
		event := &pb.RuntimeEvent{EventType: eventType, MatchedRuleId: "AGB-BUILTIN-001"}
		if shouldCreateLegacyAlert(event) {
			t.Errorf("%s entered legacy Server alert/auto-action path", eventType)
		}
	}
	if !shouldCreateLegacyAlert(&pb.RuntimeEvent{
		EventType:     "process_exec",
		MatchedRuleId: "legacy-sigma-rule",
	}) {
		t.Fatal("legacy matched event no longer enters existing alert path")
	}
}

func TestAgentGuardTransportLogsExcludeBundleAndEvidencePayloads(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	previousLogger := logger.Logger
	logger.Logger = zap.New(core)
	t.Cleanup(func() {
		logger.Logger = previousLogger
	})

	hostID := uuid.New()
	instanceID := uuid.NewString()
	server := &GRPCServer{kafkaProducer: &captureRuntimeEventProducer{}}
	impl := &APIServerToServerImpl{grpcServer: server}
	bundle := strings.TrimSuffix(
		agentGuardBundlePayload(hostID, 12, agentGuardTestDigest(12)),
		"}",
	) + `,"opaque":"bundle-secret-do-not-log"}`
	if response, err := impl.SyncAgentConfig(context.Background(), &pb.SyncAgentConfigRequest{
		HostId: hostID.String(),
		Configs: []*pb.AgentConfig{{
			ConfigType: "agent_guard_bundle",
			ConfigJson: bundle,
		}},
	}); err != nil || !response.Success {
		t.Fatalf("SyncAgentConfig response=%#v err=%v", response, err)
	}

	evidence := `{
		"schema":"aegis.agent_behavior.v1",
		"agent":{"instance_id":"` + instanceID + `"},
		"actor":{"argv":["--token=evidence-secret-do-not-log"]},
		"resource":{"identity":"/private/evidence/path"}
	}`
	if response, err := server.ReportEvent(context.Background(), &pb.ReportEventRequest{
		HostId: hostID.String(),
		Events: []*pb.RuntimeEvent{{
			EventId:       uuid.NewString(),
			EventType:     "agent_behavior",
			EventDataJson: evidence,
		}},
	}); err != nil || !response.Success {
		t.Fatalf("ReportEvent response=%#v err=%v", response, err)
	}

	for _, entry := range observed.All() {
		fields, _ := json.Marshal(entry.ContextMap())
		logged := entry.Message + string(fields)
		for _, forbidden := range []string{
			"bundle-secret-do-not-log",
			"evidence-secret-do-not-log",
			"/private/evidence/path",
		} {
			if strings.Contains(logged, forbidden) {
				t.Fatalf("sensitive payload leaked to logs: %s", logged)
			}
		}
	}
}
