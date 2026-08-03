package grpc_server

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	pb "server/pkg/api/v1"

	"github.com/google/uuid"
)

func TestAPIServerBundleAndAgentStatusCrossContract(t *testing.T) {
	bundlePath := requireContractPath(t, "AEGIS_AGENT_GUARD_CONTRACT_BUNDLE")
	bundlePayload, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("read api-server bundle fixture: %v", err)
	}
	var envelope struct {
		HostID        string `json:"host_id"`
		BundleVersion int64  `json:"bundle_version"`
		Digest        string `json:"digest"`
	}
	if err := json.Unmarshal(bundlePayload, &envelope); err != nil {
		t.Fatalf("decode api-server bundle fixture: %v", err)
	}
	hostID := uuid.MustParse(envelope.HostID)
	server := &GRPCServer{}
	snapshot, err := server.cacheAgentGuardBundle(hostID, &pb.ConfigSync{
		ConfigType: agentGuardBundleConfigType,
		Action:     "full_sync",
		Payload:    string(bundlePayload),
	})
	if err != nil {
		t.Fatalf("Server rejected api-server bundle fixture: %v", err)
	}
	if snapshot.Version != envelope.BundleVersion ||
		snapshot.Digest != envelope.Digest ||
		snapshot.Config.Payload != string(bundlePayload) {
		t.Fatalf("Server changed bundle contract: %#v", snapshot)
	}

	for _, fixture := range []struct {
		name string
		key  string
	}{
		{name: "applied", key: "AEGIS_AGENT_GUARD_APPLIED_STATUS"},
		{name: "rejected", key: "AEGIS_AGENT_GUARD_REJECTED_STATUS"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			raw, err := os.ReadFile(requireContractPath(t, fixture.key))
			if err != nil {
				t.Fatalf("read Agent status fixture: %v", err)
			}
			producer := &captureRuntimeEventProducer{}
			server.kafkaProducer = producer
			response, err := server.ReportEvent(context.Background(), &pb.ReportEventRequest{
				HostId: envelope.HostID,
				Events: []*pb.RuntimeEvent{{
					EventId:       uuid.NewString(),
					EventType:     "agent_guard_config_status",
					EventDataJson: string(raw),
				}},
			})
			if err != nil || !response.Success || response.ReceivedCount != 1 {
				t.Fatalf("ReportEvent response=%#v err=%v", response, err)
			}
			if len(producer.events) != 1 ||
				producer.events[0].event.EventDataJson != string(raw) ||
				producer.events[0].metadata.PartitionKey != envelope.HostID ||
				producer.events[0].metadata.Schema != "aegis.agent_guard.v1" {
				t.Fatalf("Server changed Agent status contract: %#v", producer.events)
			}
		})
	}
}

func requireContractPath(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Skipf("%s is not set", key)
	}
	return value
}
