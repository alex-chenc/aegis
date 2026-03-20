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
	writers map[string]*kafka.Writer
	logger  *zap.Logger
}

// NewKafkaProducer creates a new Kafka producer with writers for all required topics
func NewKafkaProducer(brokers []string, logger *zap.Logger) *KafkaProducer {
	topics := []string{"raw-events", "analysis-results", "block-commands", "rule-updates", "tool-calls"}
	writers := make(map[string]*kafka.Writer)

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

// SendRawEvent sends a raw event from Agent to raw-events topic
func (p *KafkaProducer) SendRawEvent(ctx context.Context, hostID string, event interface{}) error {
	return p.SendMessage(ctx, "raw-events", hostID, event)
}

// SendBlockCommand sends a block command to block-commands topic
func (p *KafkaProducer) SendBlockCommand(ctx context.Context, hostID string, command interface{}) error {
	return p.SendMessage(ctx, "block-commands", hostID, command)
}

// SendRuleUpdate sends a rule update to rule-updates topic (broadcast, no key partitioning)
func (p *KafkaProducer) SendRuleUpdate(ctx context.Context, ruleID string, update interface{}) error {
	return p.SendMessage(ctx, "rule-updates", ruleID, update)
}

// SendToolCall sends a tool call log to tool-calls topic
func (p *KafkaProducer) SendToolCall(ctx context.Context, hostID string, call interface{}) error {
	return p.SendMessage(ctx, "tool-calls", hostID, call)
}

// SendAnalysisResult sends an LLM analysis result to analysis-results topic
func (p *KafkaProducer) SendAnalysisResult(ctx context.Context, hostID string, result interface{}) error {
	return p.SendMessage(ctx, "analysis-results", hostID, result)
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
