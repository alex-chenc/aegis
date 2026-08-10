package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// KafkaProducer manages Kafka writers for different topics
type KafkaProducer struct {
	writers map[string]kafkaMessageWriter
	logger  *zap.Logger
}

type kafkaMessageWriter interface {
	WriteMessages(context.Context, ...kafka.Message) error
	Close() error
}

// SecurityEventMetadata carries only routing and idempotency context. It must
// never contain command arguments, paths, addresses, or evidence payloads.
type SecurityEventMetadata struct {
	PartitionKey  string
	EventID       string
	HostID        string
	InstanceID    string
	HostBootID    string
	AgentSequence string
	EventType     string
	Schema        string
}

// NewKafkaProducer creates a new Kafka producer with writers for all required topics
func NewKafkaProducer(brokers []string, logger *zap.Logger) *KafkaProducer {
	topics := []string{"aegis.security.events", "aegis.block.commands", "aegis.rule.updates", "aegis.agent.sessions.v1"}
	writers := make(map[string]kafkaMessageWriter)

	for _, topic := range topics {
		writers[topic] = &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        topic,
			Balancer:     &kafka.Hash{}, // Partition by key for ordering
			BatchTimeout: 10 * time.Millisecond,
			BatchSize:    100,
			Async:        false, // Synchronous for reliability
		}
	}

	return &KafkaProducer{writers: writers, logger: logger}
}

// SendMessage sends a message to the specified topic with a key for partitioning
func (p *KafkaProducer) SendMessage(ctx context.Context, topic, key string, value interface{}) error {
	return p.sendMessage(ctx, topic, key, value, nil)
}

func (p *KafkaProducer) sendMessage(
	ctx context.Context,
	topic string,
	key string,
	value interface{},
	headers []kafka.Header,
) error {
	writer, ok := p.writers[topic]
	if !ok {
		return fmt.Errorf("unknown topic: %s", topic)
	}

	// Marshal value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Write message
	err = writer.WriteMessages(ctx, kafka.Message{
		Key:     []byte(key),
		Value:   data,
		Headers: headers,
		Time:    time.Now(),
	})

	if err != nil {
		p.logger.Error("failed to write message",
			zap.String("topic", topic),
			zap.String("key", key),
			zap.Error(err),
		)
		return fmt.Errorf("failed to write message to topic %s: %w", topic, err)
	}

	p.logger.Debug("message sent",
		zap.String("topic", topic),
		zap.String("key", key),
	)

	return nil
}

// SendRawEvent sends a raw event from Agent to aegis.security.events topic
func (p *KafkaProducer) SendRawEvent(ctx context.Context, hostID string, event interface{}) error {
	return p.SendMessage(ctx, "aegis.security.events", hostID, event)
}

// SendRawEventWithContext writes an event without changing its value schema.
// Kafka headers provide consumers with stable replay context while remaining
// backward compatible with consumers that only read the protobuf JSON value.
func (p *KafkaProducer) SendRawEventWithContext(
	ctx context.Context,
	metadata SecurityEventMetadata,
	event interface{},
) error {
	key := metadata.PartitionKey
	if key == "" {
		key = metadata.HostID
	}
	headers := make([]kafka.Header, 0, 7)
	appendHeader := func(name string, value string) {
		if value != "" {
			headers = append(headers, kafka.Header{Key: name, Value: []byte(value)})
		}
	}
	appendHeader("aegis-event-id", metadata.EventID)
	appendHeader("aegis-host-id", metadata.HostID)
	appendHeader("aegis-instance-id", metadata.InstanceID)
	appendHeader("aegis-host-boot-id", metadata.HostBootID)
	appendHeader("aegis-agent-sequence", metadata.AgentSequence)
	appendHeader("aegis-event-type", metadata.EventType)
	appendHeader("aegis-schema", metadata.Schema)
	return p.sendMessage(ctx, "aegis.security.events", key, event, headers)
}

// SendBlockCommand sends a block command to aegis.block.commands topic
func (p *KafkaProducer) SendBlockCommand(ctx context.Context, hostID string, command interface{}) error {
	return p.SendMessage(ctx, "aegis.block.commands", hostID, command)
}

// SendRuleUpdate sends a rule update to aegis.rule.updates topic (broadcast, no key partitioning)
func (p *KafkaProducer) SendRuleUpdate(ctx context.Context, ruleID string, update interface{}) error {
	return p.SendMessage(ctx, "aegis.rule.updates", ruleID, update)
}

// Close closes all Kafka writers
func (p *KafkaProducer) Close() error {
	var lastErr error
	for topic, writer := range p.writers {
		if err := writer.Close(); err != nil {
			p.logger.Error("failed to close writer",
				zap.String("topic", topic),
				zap.Error(err),
			)
			lastErr = err
		}
	}
	return lastErr
}
