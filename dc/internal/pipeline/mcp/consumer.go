package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const Topic = "aegis.mcp.invocations.v1"

type Consumer struct {
	reader *kafka.Reader
	db     *gorm.DB
	log    *zap.Logger
}

func NewConsumer(brokers []string, groupID string, db *gorm.DB, logger *zap.Logger) *Consumer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Consumer{reader: kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, Topic: Topic, GroupID: groupID + "-mcp", MinBytes: 1, MaxBytes: 2 << 20, MaxWait: time.Second, CommitInterval: time.Second}), db: db, log: logger}
}

func (c *Consumer) Start(ctx context.Context) error {
	for {
		message, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.log.Warn("mcp_invocation_event_read_failed", zap.Error(err))
			continue
		}
		if err := c.project(ctx, message.Value); err != nil {
			c.log.Warn("mcp_invocation_event_project_failed", zap.Error(err))
		}
	}
}

func (c *Consumer) project(ctx context.Context, payload []byte) error {
	var event InvocationEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode invocation event: %w", err)
	}
	if event.Schema != "aegis.mcp.invocation.v1" {
		return fmt.Errorf("unsupported invocation schema")
	}
	if _, err := uuid.Parse(event.InvocationID); err != nil {
		return fmt.Errorf("invalid invocation id")
	}
	verdict := Analyze(event)
	hits, _ := json.Marshal(verdict.Hits)
	evidence := []byte(`{"source":"dc_mcp_rules","hits":` + string(hits) + `}`)
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE mcp_invocations SET rule_status=?, completed_at=COALESCE(completed_at, now()) WHERE id=?`, verdict.DeterministicSeverity, event.InvocationID).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO mcp_security_verdicts (id, invocation_id, deterministic_severity, ai_verdict, overall_risk, evidence, updated_at) VALUES (?, ?, ?, ?, ?, ?::jsonb, now()) ON CONFLICT (invocation_id) DO UPDATE SET deterministic_severity=EXCLUDED.deterministic_severity, overall_risk=EXCLUDED.overall_risk, evidence=EXCLUDED.evidence, updated_at=now()`, uuid.New(), event.InvocationID, verdict.DeterministicSeverity, "pending", verdict.OverallRisk, string(evidence)).Error
	})
}

func (c *Consumer) Close() error { return c.reader.Close() }
