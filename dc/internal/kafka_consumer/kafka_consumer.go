package kafka_consumer

import (
	"context"
	"encoding/json"
	"time"

	"dc/config"
	"dc/internal/alert_generator"
	"dc/internal/aggregator"
	"dc/internal/event_handler"
	"dc/internal/llm_analyzer"
	"dc/internal/repository"
	"dc/pkg/logger"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type KafkaConsumer struct {
	reader       *kafka.Reader
	eventHandler *event_handler.EventHandler
	logger       *zap.Logger
}

func NewKafkaConsumer(
	cfg *config.KafkaConfig,
	runtimeEventRepo *repository.RuntimeEventRepository,
	llmAnalyzer *llm_analyzer.LLMAnalyzer,
	alertGen *alert_generator.AlertGenerator,
	aggregator *aggregator.Aggregator,
) (*KafkaConsumer, error) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        cfg.GroupID,
		Topic:          cfg.Topic,
		MinBytes:       10e3, // 10KB
		MaxBytes:       10e6, // 10MB
		MaxWait:        1 * time.Second,
		CommitInterval: time.Second,
	})

	logger.Info("Kafka consumer created",
		zap.Strings("brokers", cfg.Brokers),
		zap.String("topic", cfg.Topic),
		zap.String("group_id", cfg.GroupID),
	)

	return &KafkaConsumer{
		reader:       reader,
		eventHandler: event_handler.NewEventHandler(runtimeEventRepo, llmAnalyzer, alertGen, aggregator),
		logger:       logger.Get(),
	}, nil
}

func (c *KafkaConsumer) Start(ctx context.Context) error {
	c.logger.Info("Starting Kafka consumer...")

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Kafka consumer stopping due to context cancellation")
			return ctx.Err()
		default:
		}

		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.logger.Error("Failed to read message", zap.Error(err))
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			c.logger.Error("Failed to unmarshal event",
				zap.Error(err),
				zap.ByteString("value", msg.Value),
			)
			continue
		}

		if err := c.eventHandler.Handle(event); err != nil {
			c.logger.Error("Failed to handle event", zap.Error(err))
		}
	}
}

func (c *KafkaConsumer) Close() error {
	c.logger.Info("Closing Kafka consumer")
	return c.reader.Close()
}