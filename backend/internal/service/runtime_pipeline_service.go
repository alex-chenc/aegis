package service

import (
	"context"
	"encoding/json"
	"time"

	"aegis-system/internal/model"
	"aegis-system/internal/pipeline"
	"aegis-system/internal/queue"
	"aegis-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// LLMAnalysisServiceInterface defines the interface for LLM analysis service
type LLMAnalysisServiceInterface interface {
	Analyze(ctx context.Context, window *pipeline.HostWindow) (*pipeline.LLMAnalysisOutput, error)
}

// WebSocketServiceInterface defines the interface for WebSocket service
type WebSocketServiceInterface interface {
	BroadcastAlert(alert *model.Alert)
}

// RuntimePipelineService orchestrates the event processing pipeline
type RuntimePipelineService struct {
	consumer     *queue.KafkaConsumer
	aggregator   *pipeline.HostWindowAggregator
	llmService   LLMAnalysisServiceInterface
	alertService *AlertService
	wsService    WebSocketServiceInterface
	logger       *zap.Logger
}

// NewRuntimePipelineService creates a new pipeline service
func NewRuntimePipelineService(
	brokers []string,
	groupID string,
	llmService LLMAnalysisServiceInterface,
	alertService *AlertService,
	wsService WebSocketServiceInterface,
) *RuntimePipelineService {
	s := &RuntimePipelineService{
		llmService:   llmService,
		alertService: alertService,
		wsService:    wsService,
		logger:       logger.Logger,
	}

	// Create aggregator with 2-minute window
	s.aggregator = pipeline.NewHostWindowAggregator(2*time.Minute, s.onWindowFlush)

	// Create consumer for raw-events topic
	s.consumer = queue.NewKafkaConsumer(
		brokers,
		"raw-events",
		groupID,
		s.handleRawEvent,
		s.logger,
	)

	return s
}

// Start begins the pipeline service
func (s *RuntimePipelineService) Start(ctx context.Context) error {
	s.logger.Info("starting runtime pipeline service")

	// Start aggregator ticker (check every 10 seconds)
	go s.aggregator.StartTicker(ctx, 10*time.Second)

	// Start consuming from Kafka
	return s.consumer.Start(ctx)
}

// handleRawEvent processes a single raw event from Kafka
func (s *RuntimePipelineService) handleRawEvent(ctx context.Context, key, value []byte) error {
	var event pipeline.RuntimeEvent
	if err := json.Unmarshal(value, &event); err != nil {
		s.logger.Error("failed to unmarshal event",
			zap.String("key", string(key)),
			zap.Error(err),
		)
		return err
	}

	hostID := string(key)
	s.aggregator.AddEvent(hostID, event)

	s.logger.Debug("event added to window",
		zap.String("host_id", hostID),
		zap.String("event_type", event.EventType),
		zap.Int("pid", event.PID),
	)

	return nil
}

// onWindowFlush is called when a host window expires
func (s *RuntimePipelineService) onWindowFlush(window *pipeline.HostWindow) {
	ctx := context.Background()

	s.logger.Info("processing window",
		zap.String("host_id", window.HostID),
		zap.Int("event_count", len(window.Events)),
	)

	if len(window.Events) == 0 {
		return
	}

	// Call LLM analysis
	result, err := s.llmService.Analyze(ctx, window)
	if err != nil {
		s.logger.Error("LLM analysis failed",
			zap.String("host_id", window.HostID),
			zap.Error(err),
		)
		return
	}

	// Process alerts
	for _, alert := range result.Alerts {
		hostUUID, err := uuid.Parse(window.HostID)
		if err != nil {
			s.logger.Error("invalid host ID",
				zap.String("host_id", window.HostID),
				zap.Error(err),
			)
			continue
		}

		createdAlert, err := s.alertService.UpsertByDedupe(
			hostUUID,
			alert.PID,
			alert.RuleID,
			alert.RuleTitle,
			alert.MitreID,
			alert.MitreName,
			alert.Severity,
			alert.Description,
		)
		if err != nil {
			s.logger.Error("failed to create alert",
				zap.String("host_id", window.HostID),
				zap.Error(err),
			)
			continue
		}

		// Check if auto-block is enabled
		if err := s.alertService.CheckAndAutoBlock(createdAlert); err != nil {
			s.logger.Error("auto-block check failed",
				zap.String("alert_id", createdAlert.AlertID),
				zap.Error(err),
			)
		}

		// Broadcast alert via WebSocket
		s.wsService.BroadcastAlert(createdAlert)

		s.logger.Info("alert created",
			zap.String("alert_id", createdAlert.AlertID),
			zap.String("mitre_id", alert.MitreID),
			zap.String("severity", alert.Severity),
		)
	}

	// Log tool calls (actual execution will be in LLMAnalysisService)
	for _, toolCall := range result.ToolCalls {
		s.logger.Info("tool call requested",
			zap.String("host_id", window.HostID),
			zap.String("tool", toolCall.Tool),
			zap.String("reason", toolCall.Reason),
		)
	}

	// Log rule adjustments
	for _, adj := range result.RuleAdjustments {
		s.logger.Info("rule adjustment suggested",
			zap.String("host_id", window.HostID),
			zap.String("rule_id", adj.RuleID),
			zap.String("action", adj.Action),
			zap.String("reason", adj.Reason),
		)
	}
}

// GetStats returns pipeline statistics
func (s *RuntimePipelineService) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"active_windows": s.aggregator.GetWindowCount(),
		"total_events":   s.aggregator.GetEventCount(),
	}
}

// Close shuts down the pipeline service
func (s *RuntimePipelineService) Close() error {
	s.logger.Info("shutting down runtime pipeline service")
	return s.consumer.Close()
}
