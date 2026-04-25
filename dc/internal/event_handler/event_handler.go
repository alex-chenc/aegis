package event_handler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"dc/internal/aggregator"
	"dc/internal/alert_generator"
	"dc/internal/llm_analyzer"
	"dc/internal/model"
	"dc/internal/repository"
	"dc/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type EventHandler struct {
	logger      *zap.Logger
	repo        *repository.RuntimeEventRepository
	llmAnalyzer *llm_analyzer.LLMAnalyzer
	alertGen    *alert_generator.AlertGenerator
	aggregator  *aggregator.Aggregator
}

func NewEventHandler(
	repo *repository.RuntimeEventRepository,
	llmAnalyzer *llm_analyzer.LLMAnalyzer,
	alertGen *alert_generator.AlertGenerator,
	aggregator *aggregator.Aggregator,
) *EventHandler {
	return &EventHandler{
		logger:      logger.Get(),
		repo:        repo,
		llmAnalyzer: llmAnalyzer,
		alertGen:    alertGen,
		aggregator:  aggregator,
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
	eventData = normalizeEventDataJSON(eventData)
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

	// Add to aggregator for batch processing
	if h.aggregator != nil {
		h.aggregator.AddEvent(runtimeEvent)
	}

	// Generate alert if matched rule exists
	if h.alertGen != nil && matchedRuleID != "" {
		alert := h.alertGen.GenerateAlert(runtimeEvent)
		if alert != nil {
			h.logger.Info("Alert generated",
				zap.String("alert_id", alert.AlertID),
				zap.String("severity", alert.Severity),
			)
		}
	}

	// Trigger LLM analysis for critical/high severity events
	if h.llmAnalyzer != nil && (severity == "critical" || severity == "high") {
		go func() {
			analysisCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := h.llmAnalyzer.AnalyzeEvents(analysisCtx, []*model.RuntimeEvent{runtimeEvent})
			if err != nil {
				h.logger.Error("LLM analysis failed",
					zap.String("event_id", eventID),
					zap.Error(err),
				)
				return
			}

			h.logger.Info("LLM analysis completed",
				zap.String("event_id", eventID),
				zap.String("summary", result.Summary),
			)
			// TODO: Store analysis result if there's a storage mechanism
		}()
	}

	return nil
}

func normalizeEventDataJSON(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !json.Valid([]byte(value)) {
		return "{}"
	}
	return value
}
