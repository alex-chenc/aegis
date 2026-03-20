package queue

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestNewKafkaConsumer(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	brokers := []string{"localhost:9092"}

	handler := func(ctx context.Context, key, value []byte) error {
		return nil
	}

	consumer := NewKafkaConsumer(brokers, "test-topic", "test-group", handler, logger)
	if consumer == nil {
		t.Fatal("expected non-nil consumer")
	}

	// Verify reader config
	if consumer.reader.Config().Topic != "test-topic" {
		t.Errorf("expected topic 'test-topic', got '%s'", consumer.reader.Config().Topic)
	}
	if consumer.reader.Config().GroupID != "test-group" {
		t.Errorf("expected group 'test-group', got '%s'", consumer.reader.Config().GroupID)
	}

	// Clean up
	consumer.Close()
}
