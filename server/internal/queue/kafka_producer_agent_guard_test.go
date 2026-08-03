package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type captureKafkaMessageWriter struct {
	messages []kafka.Message
}

func (w *captureKafkaMessageWriter) WriteMessages(_ context.Context, messages ...kafka.Message) error {
	w.messages = append(w.messages, messages...)
	return nil
}

func (w *captureKafkaMessageWriter) Close() error {
	return nil
}

func TestSendRawEventWithContextUsesInstanceKeyAndIdempotencyHeaders(t *testing.T) {
	writer := &captureKafkaMessageWriter{}
	producer := &KafkaProducer{
		writers: map[string]kafkaMessageWriter{
			"aegis.security.events": writer,
		},
		logger: zap.NewNop(),
	}
	metadata := SecurityEventMetadata{
		PartitionKey:  "host-1:instance-1",
		EventID:       "event-1",
		HostID:        "host-1",
		InstanceID:    "instance-1",
		HostBootID:    "boot-1",
		AgentSequence: "42",
		EventType:     "agent_behavior",
		Schema:        "aegis.agent_behavior.v1",
	}
	event := struct {
		EventID       string `json:"event_id"`
		EventDataJSON string `json:"event_data_json"`
	}{
		EventID:       "event-1",
		EventDataJSON: `{"actor":{"argv":["--token=do-not-log"]},"resource":{"identity":"/secret/path"}}`,
	}

	if err := producer.SendRawEventWithContext(context.Background(), metadata, event); err != nil {
		t.Fatalf("SendRawEventWithContext: %v", err)
	}
	if len(writer.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(writer.messages))
	}
	message := writer.messages[0]
	if got := string(message.Key); got != metadata.PartitionKey {
		t.Fatalf("message key = %q, want %q", got, metadata.PartitionKey)
	}
	headers := kafkaHeaderMap(message.Headers)
	for key, want := range map[string]string{
		"aegis-event-id":       metadata.EventID,
		"aegis-host-id":        metadata.HostID,
		"aegis-instance-id":    metadata.InstanceID,
		"aegis-host-boot-id":   metadata.HostBootID,
		"aegis-agent-sequence": metadata.AgentSequence,
		"aegis-event-type":     metadata.EventType,
		"aegis-schema":         metadata.Schema,
	} {
		if got := headers[key]; got != want {
			t.Errorf("header %q = %q, want %q", key, got, want)
		}
	}
	for _, header := range message.Headers {
		if strings.Contains(string(header.Value), "do-not-log") ||
			strings.Contains(string(header.Value), "/secret/path") {
			t.Fatalf("sensitive event payload leaked into Kafka header %q", header.Key)
		}
	}
	if !strings.Contains(string(message.Value), `"event_id":"event-1"`) {
		t.Fatalf("event payload was not preserved: %s", message.Value)
	}
}

func kafkaHeaderMap(headers []kafka.Header) map[string]string {
	values := make(map[string]string, len(headers))
	for _, header := range headers {
		values[header.Key] = string(header.Value)
	}
	return values
}
