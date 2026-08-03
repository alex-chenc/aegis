package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

type captureAgentActionWriter struct {
	messages []kafka.Message
	err      error
}

func (w *captureAgentActionWriter) WriteMessages(_ context.Context, messages ...kafka.Message) error {
	w.messages = append(w.messages, messages...)
	return w.err
}

func (w *captureAgentActionWriter) Close() error { return nil }

func TestKafkaAgentActionPublisherUsesHostPartitionAndStableActionID(t *testing.T) {
	hostID, unitID, actionID := uuid.New(), uuid.New(), uuid.New()
	writer := &captureAgentActionWriter{}
	publisher := &KafkaAgentActionPublisher{writer: writer}
	command := AgentGuardBlockCommand{
		CommandID: "AG-GUARD-" + actionID.String(), HostID: hostID.String(),
		Action: "freeze_execution_unit", Target: unitID.String(), Reason: "published policy",
	}
	if err := publisher.PublishAgentGuardAction(context.Background(), command); err != nil {
		t.Fatalf("PublishAgentGuardAction: %v", err)
	}
	if len(writer.messages) != 1 || string(writer.messages[0].Key) != hostID.String() {
		t.Fatalf("messages=%#v", writer.messages)
	}
	var decoded AgentGuardBlockCommand
	if json.Unmarshal(writer.messages[0].Value, &decoded) != nil || decoded != command {
		t.Fatalf("decoded command=%#v", decoded)
	}
	if len(writer.messages[0].Headers) != 2 ||
		string(writer.messages[0].Headers[0].Value) != actionID.String() {
		t.Fatalf("headers=%#v", writer.messages[0].Headers)
	}
}

func TestKafkaAgentActionPublisherRejectsNonUUIDAndPreservesWriteFailure(t *testing.T) {
	hostID, unitID, actionID := uuid.New(), uuid.New(), uuid.New()
	writer := &captureAgentActionWriter{}
	publisher := &KafkaAgentActionPublisher{writer: writer}
	for _, command := range []AgentGuardBlockCommand{
		{CommandID: "AG-GUARD-" + actionID.String(), HostID: hostID.String(), Action: "deny", Target: unitID.String()},
		{CommandID: "AG-GUARD-" + actionID.String(), HostID: hostID.String(), Action: "freeze_execution_unit", Target: "*"},
		{CommandID: "other-" + actionID.String(), HostID: hostID.String(), Action: "freeze_execution_unit", Target: unitID.String()},
	} {
		if err := publisher.PublishAgentGuardAction(context.Background(), command); !errors.Is(err, ErrAgentGuardActionPublishContract) {
			t.Fatalf("command=%#v err=%v", command, err)
		}
	}
	writer.err = errors.New("broker unavailable")
	err := publisher.PublishAgentGuardAction(context.Background(), AgentGuardBlockCommand{
		CommandID: "AG-GUARD-" + actionID.String(), HostID: hostID.String(),
		Action: "freeze_execution_unit", Target: unitID.String(),
	})
	if err == nil || !errors.Is(err, writer.err) {
		t.Fatalf("write failure=%v", err)
	}
}
