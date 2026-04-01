package event_handler

import (
	"context"

	"dc/internal/model"
	"dc/internal/repository"
	"dc/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type EventHandler struct {
	logger *zap.Logger
	repo   *repository.RuntimeEventRepository
}

func NewEventHandler(repo *repository.RuntimeEventRepository) *EventHandler {
	return &EventHandler{
		logger: logger.Get(),
		repo:   repo,
	}
}

func (h *EventHandler) Handle(event map[string]interface{}) error {
	h.logger.Debug("Handling event",
		zap.Any("event", event),
	)

	// Extract relevant fields from the event map
	eventID, _ := event["event_id"].(string)
	if eventID == "" {
		eventID = uuid.New().String()
	}

	hostIDStr, _ := event["host_id"].(string)
	hostID, err := uuid.Parse(hostIDStr)
	if err != nil {
		hostID = uuid.New()
	}

	eventType, _ := event["event_type"].(string)
	eventData, _ := event["event_data"].(string)
	matchedRuleID, _ := event["matched_rule_id"].(string)
	mitreID, _ := event["mitre_id"].(string)
	severity, _ := event["severity"].(string)
	commandLine, _ := event["command_line"].(string)
	processName, _ := event["process_name"].(string)

	// Create RuntimeEvent model
	runtimeEvent := &model.RuntimeEvent{
		EventID:       eventID,
		HostID:        hostID,
		EventType:     eventType,
		EventData:     eventData,
		MatchedRuleID: matchedRuleID,
		MitreID:       mitreID,
		Severity:      severity,
		CommandLine:   commandLine,
		ProcessName:   processName,
	}

	// Persist to database
	ctx := context.Background()
	if err := h.repo.CreateWithContext(ctx, runtimeEvent); err != nil {
		h.logger.Error("Failed to persist runtime event",
			zap.String("event_id", eventID),
			zap.Error(err),
		)
		return err
	}

	h.logger.Info("Event persisted",
		zap.String("event_id", runtimeEvent.EventID),
		zap.String("host_id", runtimeEvent.HostID.String()),
		zap.String("event_type", runtimeEvent.EventType),
	)

	return nil
}