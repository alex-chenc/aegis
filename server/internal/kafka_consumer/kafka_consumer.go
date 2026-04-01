package kafka_consumer

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// MessageHandler is a function that processes a consumed message
type MessageHandler func(ctx context.Context, key, value []byte) error

// KafkaConsumer reads messages from a Kafka topic
type KafkaConsumer struct {
	reader  *kafka.Reader
	handler MessageHandler
	logger  *zap.Logger
}

// NewKafkaConsumer creates a new consumer for the specified topic
func NewKafkaConsumer(brokers []string, topic, groupID string, handler MessageHandler, logger *zap.Logger) *KafkaConsumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       10e3, // 10KB
		MaxBytes:       10e6, // 10MB
		CommitInterval: time.Second,
		StartOffset:    kafka.LastOffset, // Start from latest
	})

	return &KafkaConsumer{
		reader:  reader,
		handler: handler,
		logger:  logger,
	}
}

// Start begins consuming messages until context is cancelled
func (c *KafkaConsumer) Start(ctx context.Context) error {
	c.logger.Info("starting consumer",
		zap.String("topic", c.reader.Config().Topic),
		zap.String("group", c.reader.Config().GroupID),
	)

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("consumer stopping due to context cancellation")
			return ctx.Err()
		default:
			msg, err := c.reader.ReadMessage(ctx)
			if err != nil {
				// Check if context was cancelled
				if ctx.Err() != nil {
					return nil // Graceful shutdown
				}
				c.logger.Error("failed to read message", zap.Error(err))
				continue
			}

			// Process message
			c.logger.Debug("message received",
				zap.String("topic", msg.Topic),
				zap.String("key", string(msg.Key)),
				zap.Int("value_size", len(msg.Value)),
			)

			if err := c.handler(ctx, msg.Key, msg.Value); err != nil {
				c.logger.Error("handler error",
					zap.String("topic", msg.Topic),
					zap.String("key", string(msg.Key)),
					zap.Error(err),
				)
				// Continue processing next message (don't block on errors)
			}
		}
	}
}

// Close closes the consumer reader
func (c *KafkaConsumer) Close() error {
	c.logger.Info("closing consumer", zap.String("topic", c.reader.Config().Topic))
	return c.reader.Close()
}
