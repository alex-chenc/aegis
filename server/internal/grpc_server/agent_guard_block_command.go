package grpc_server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"server/internal/queue"
	pb "server/pkg/api/v1"
	"server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	ErrAgentGuardBlockCommandInvalid = errors.New("invalid agent guard block command")
	ErrAgentGuardBlockTargetInvalid  = errors.New("invalid agent guard block target")
)

var allowedAgentGuardBlockActions = map[string]struct{}{
	"freeze_execution_unit": {},
	"resume_execution_unit": {},
	"hold_execution_unit":   {},
	"kill_execution_unit":   {},
	"kill_agent_instance":   {},
}

// ValidateAgentGuardBlockCommand is the Server-side trust boundary for P3
// actions. target is exactly one execution unit or instance UUID; PID, path,
// JSON, wildcard, and host-level targets are intentionally rejected.
func ValidateAgentGuardBlockCommand(hostID uuid.UUID, command *pb.BlockCommand) error {
	if command == nil || hostID == uuid.Nil {
		return ErrAgentGuardBlockCommandInvalid
	}
	commandHostID, err := uuid.Parse(strings.TrimSpace(command.HostId))
	if err != nil || commandHostID != hostID {
		return fmt.Errorf("%w: host mismatch", ErrAgentGuardBlockCommandInvalid)
	}
	commandID := strings.TrimSpace(command.CommandId)
	if commandID == "" || len(commandID) > 100 || strings.ContainsAny(commandID, "*?[]{}") {
		return fmt.Errorf("%w: command_id", ErrAgentGuardBlockCommandInvalid)
	}
	if _, err := agentGuardActionIDFromCommand(commandID); err != nil {
		return fmt.Errorf("%w: command_id identity", ErrAgentGuardBlockCommandInvalid)
	}
	if _, ok := allowedAgentGuardBlockActions[command.Action]; !ok {
		return fmt.Errorf("%w: unsupported action", ErrAgentGuardBlockCommandInvalid)
	}
	targetID, err := uuid.Parse(strings.TrimSpace(command.Target))
	if err != nil || targetID == uuid.Nil || targetID == hostID {
		return ErrAgentGuardBlockTargetInvalid
	}
	if len(command.Reason) > 2048 {
		return fmt.Errorf("%w: reason too large", ErrAgentGuardBlockCommandInvalid)
	}
	return nil
}

// HandleAgentGuardBlockMessage validates the Kafka partition key and strict
// command schema before forwarding to the connected host. Agent error text is
// returned unchanged to the caller but is not written to Server logs.
func (s *GRPCServer) HandleAgentGuardBlockMessage(ctx context.Context, key, value []byte) error {
	hostID, err := uuid.Parse(strings.TrimSpace(string(key)))
	if err != nil {
		return fmt.Errorf("%w: partition host", ErrAgentGuardBlockCommandInvalid)
	}
	var command pb.BlockCommand
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&command); err != nil {
		return fmt.Errorf("%w: JSON schema", ErrAgentGuardBlockCommandInvalid)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%w: trailing JSON", ErrAgentGuardBlockCommandInvalid)
	}
	if err := ValidateAgentGuardBlockCommand(hostID, &command); err != nil {
		logger.Warn("agent_guard_block_command_rejected",
			zap.String("command_id", command.CommandId),
			zap.String("host_id", hostID.String()),
			zap.String("action", command.Action),
			zap.String("error_code", "invalid_agent_guard_block_command"))
		return err
	}
	if err := s.SendBlockCommand(hostID, &command); err != nil {
		if reportErr := s.reportAgentGuardForwardFailure(ctx, hostID, &command, err); reportErr != nil {
			logger.Warn("agent_guard_block_failure_report_failed",
				zap.String("command_id", command.CommandId),
				zap.String("host_id", hostID.String()),
				zap.String("action", command.Action),
				zap.String("error_code", "agent_guard_failure_status_publish_failed"))
		}
		logger.Warn("agent_guard_block_command_forward_failed",
			zap.String("command_id", command.CommandId),
			zap.String("host_id", hostID.String()),
			zap.String("action", command.Action),
			zap.String("error_code", "agent_command_failed"))
		return err
	}
	logger.Info("agent_guard_block_command_forwarded",
		zap.String("command_id", command.CommandId),
		zap.String("host_id", hostID.String()),
		zap.String("action", command.Action))
	return nil
}

func agentGuardActionIDFromCommand(commandID string) (uuid.UUID, error) {
	if !strings.HasPrefix(commandID, "AG-GUARD-") {
		return uuid.Nil, ErrAgentGuardBlockCommandInvalid
	}
	actionID, err := uuid.Parse(strings.TrimPrefix(commandID, "AG-GUARD-"))
	if err != nil || actionID == uuid.Nil {
		return uuid.Nil, ErrAgentGuardBlockCommandInvalid
	}
	return actionID, nil
}

func (s *GRPCServer) reportAgentGuardForwardFailure(
	ctx context.Context,
	hostID uuid.UUID,
	command *pb.BlockCommand,
	forwardErr error,
) error {
	if s == nil || s.kafkaProducer == nil || command == nil || forwardErr == nil {
		return nil
	}
	actionID, err := agentGuardActionIDFromCommand(command.CommandId)
	if err != nil {
		return err
	}
	eventID := uuid.NewString()
	body := map[string]any{
		"schema":        "aegis.agent_guard.v1",
		"action_id":     actionID.String(),
		"command_id":    command.CommandId,
		"action":        command.Action,
		"status":        "failed",
		"result":        map[string]any{"executed": false, "state_changed": false, "transport": "server"},
		"error_code":    "agent_guard_action_forward_failed",
		"error_message": truncateAgentGuardFailure(forwardErr.Error(), 1000),
		"occurred_at":   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if command.Action == "kill_agent_instance" {
		body["instance_id"] = command.Target
	} else {
		body["execution_unit_id"] = command.Target
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	event := &pb.RuntimeEvent{
		EventId: eventID, HostId: hostID.String(), EventType: "agent_guard_action_status",
		EventDataJson: string(payload), Severity: "high", Timestamp: time.Now().UnixMilli(),
	}
	return s.kafkaProducer.SendRawEventWithContext(ctx, queue.SecurityEventMetadata{
		PartitionKey: hostID.String(), EventID: eventID, HostID: hostID.String(),
		EventType: "agent_guard_action_status", Schema: "aegis.agent_guard.v1",
	}, event)
}

func truncateAgentGuardFailure(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
