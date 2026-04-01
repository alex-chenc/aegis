package queue

import (
	"testing"

	"go.uber.org/zap"
)

func TestNewKafkaProducer(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	brokers := []string{"localhost:9092"}

	producer := NewKafkaProducer(brokers, logger)
	if producer == nil {
		t.Fatal("expected non-nil producer")
	}

	// Verify all topics have writers
	expectedTopics := []string{"aegis.security.events", "aegis.block.commands", "aegis.rule.updates"}
	for _, topic := range expectedTopics {
		if _, ok := producer.writers[topic]; !ok {
			t.Errorf("missing writer for topic: %s", topic)
		}
	}

	// Clean up
	producer.Close()
}
