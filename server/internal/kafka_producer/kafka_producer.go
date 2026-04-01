package kafka_producer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"server/config"
)

// KafkaProducer manages Kafka writers for different topics
type KafkaProducer struct {
	writers map[string]*kafka.Writer
	logger  *zap.Logger
}

// NewKafkaProducer creates a new Kafka producer with writers for all required topics
func NewKafkaProducer(cfg *config.KafkaConfig) (*KafkaProducer, error) {
	logger := zap.L()
	topics := []string{"aegis.security.events", "aegis.block.commands", "aegis.rule.updates"}
	writers := make(map[string]*kafka.Writer)

	for _, topic := range topics {
		writers[topic] = &kafka.Writer{
			Addr:         kafka.TCP(cfg.Brokers...),
			Topic:        topic,
			Balancer:     &kafka.Hash{},
			BatchTimeout: 10 * time.Millisecond,
			BatchSize:    100,
			Async:        false,
		}
	}

	return &KafkaProducer{writers: writers, logger: logger}, nil
}

// SendMessage sends a message to the specified topic with a key for partitioning
func (p *KafkaProducer) SendMessage(ctx context.Context, topic, key string, value interface{}) error {
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
		Key:   []byte(key),
		Value: data,
		Time:  time.Now(),
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

// SendSecurityEvent sends a security event to aegis.security.events topic
func (p *KafkaProducer) SendSecurityEvent(ctx context.Context, hostID string, event interface{}) error {
	return p.SendMessage(ctx, "aegis.security.events", hostID, event)
}

// SendBlockCommand sends a block command to aegis.block.commands topic
func (p *KafkaProducer) SendBlockCommand(ctx context.Context, hostID string, command interface{}) error {
	return p.SendMessage(ctx, "aegis.block.commands", hostID, command)
}

// SendRuleUpdate sends a rule update to aegis.rule.updates topic
func (p *KafkaProducer) SendRuleUpdate(ctx context.Context, hostID string, update interface{}) error {
	return p.SendMessage(ctx, "aegis.rule.updates", hostID, update)
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
