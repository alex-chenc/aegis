package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

const agentGuardBlockTopic = "aegis.block.commands"

var ErrAgentGuardActionPublishContract = errors.New("invalid Agent Guard action publish contract")

type agentActionMessageWriter interface {
	WriteMessages(context.Context, ...kafka.Message) error
	Close() error
}

// KafkaAgentActionPublisher is the asynchronous DC-to-Server transport. It
// accepts only the deterministic, UUID-scoped freeze command produced by the
// eligibility pipeline; synchronous deny remains Agent-local.
type KafkaAgentActionPublisher struct {
	writer agentActionMessageWriter
}

func NewKafkaAgentActionPublisher(brokers []string) *KafkaAgentActionPublisher {
	return &KafkaAgentActionPublisher{writer: &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        agentGuardBlockTopic,
		Balancer:     &kafka.Hash{},
		BatchTimeout: 10 * time.Millisecond,
		BatchSize:    100,
		Async:        false,
	}}
}

func (p *KafkaAgentActionPublisher) PublishAgentGuardAction(
	ctx context.Context,
	command AgentGuardBlockCommand,
) error {
	if p == nil || p.writer == nil {
		return fmt.Errorf("%w: publisher unavailable", ErrAgentGuardActionPublishContract)
	}
	hostID, hostErr := uuid.Parse(strings.TrimSpace(command.HostID))
	targetID, targetErr := uuid.Parse(strings.TrimSpace(command.Target))
	actionID, commandErr := actionIDFromCommand(command.CommandID)
	if hostErr != nil || hostID == uuid.Nil || targetErr != nil || targetID == uuid.Nil ||
		targetID == hostID || commandErr != nil || command.Action != "freeze_execution_unit" ||
		len(command.Reason) > 2048 {
		return ErrAgentGuardActionPublishContract
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("marshal Agent Guard action: %w", err)
	}
	if err := p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(hostID.String()),
		Value: payload,
		Headers: []kafka.Header{
			{Key: "aegis-action-id", Value: []byte(actionID.String())},
			{Key: "aegis-command-id", Value: []byte(command.CommandID)},
		},
	}); err != nil {
		return fmt.Errorf("publish Agent Guard action: %w", err)
	}
	return nil
}

func (p *KafkaAgentActionPublisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}

func actionIDFromCommand(commandID string) (uuid.UUID, error) {
	if !strings.HasPrefix(commandID, "AG-GUARD-") {
		return uuid.Nil, ErrAgentGuardActionPublishContract
	}
	id, err := uuid.Parse(strings.TrimPrefix(commandID, "AG-GUARD-"))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, ErrAgentGuardActionPublishContract
	}
	return id, nil
}
