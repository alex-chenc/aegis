package sessionaudit

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type Consumer struct {
	reader    *kafka.Reader
	projector *Projector
	logger    *zap.Logger
}

func NewConsumer(brokers []string, groupID string, projector *Projector, logger *zap.Logger) *Consumer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Consumer{reader: kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, Topic: Topic, GroupID: groupID + "-agent-sessions", MinBytes: 1, MaxBytes: 10 << 20, MaxWait: time.Second, CommitInterval: time.Second}), projector: projector, logger: logger}
}
func (c *Consumer) Start(ctx context.Context) error {
	for {
		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.logger.Warn("agent_session_projection_read_failed", zap.Error(err))
			continue
		}
		if err := c.projector.Project(ctx, msg.Value); err != nil {
			c.logger.Warn("agent_session_projection_failed", zap.Error(err))
		} else {
			c.logger.Debug("agent_session_projection_succeeded", zap.Int("bytes", len(msg.Value)))
		}
	}
}
func (c *Consumer) Close() error { return c.reader.Close() }
