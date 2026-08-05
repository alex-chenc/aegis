package event_handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"dc/internal/aggregator"
	"dc/internal/alert_generator"
	"dc/internal/llm_analyzer"
	"dc/internal/model"
	"dc/internal/pipeline"
	"dc/internal/repository"
	"dc/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type EventHandler struct {
	logger            *zap.Logger
	repo              RuntimeEventWriter
	llmAnalyzer       *llm_analyzer.LLMAnalyzer
	alertGen          LegacyAlertGenerator
	aggregator        *aggregator.Aggregator
	behaviorRepo      AgentBehaviorWriter
	stateRepo         AgentGuardStateWriter
	projectionEnabled bool
	notifier          AgentBehaviorNotifier
	ruleProcessor     AgentGuardRuleProcessor
	ruleOptions       pipeline.AgentRuleProcessingOptions
}

type RuntimeEventWriter interface {
	CreateWithContext(context.Context, *model.RuntimeEvent) error
}

type LegacyAlertGenerator interface {
	GenerateAlert(*model.RuntimeEvent) *model.Alert
}

type AgentBehaviorWriter interface {
	CreateWithContext(context.Context, *model.AgentBehaviorEvent) (bool, error)
}

type AgentGuardStateWriter interface {
	UpsertWithContext(context.Context, *model.AgentGuardStateProjection) (bool, error)
}

type AgentBehaviorUpdate struct {
	Type            string     `json:"type"`
	EventID         string     `json:"event_id"`
	HostID          uuid.UUID  `json:"host_id"`
	InstanceID      *uuid.UUID `json:"instance_id,omitempty"`
	SessionID       *uuid.UUID `json:"session_id,omitempty"`
	ExecutionUnitID *uuid.UUID `json:"execution_unit_id,omitempty"`
}

type AgentBehaviorNotifier interface {
	BehaviorCreated(context.Context, AgentBehaviorUpdate) error
}

type AgentGuardStateUpdate struct {
	Type          string    `json:"type"`
	HostID        uuid.UUID `json:"host_id"`
	ObjectID      uuid.UUID `json:"object_id,omitempty"`
	BundleVersion int64     `json:"bundle_version,omitempty"`
	Action        string    `json:"action,omitempty"`
	Status        string    `json:"status,omitempty"`
}

type AgentGuardStateNotifier interface {
	StateUpdated(context.Context, AgentGuardStateUpdate) error
}

type AgentFindingUpdate struct {
	Type      string    `json:"type"`
	FindingID uuid.UUID `json:"finding_id"`
	Created   bool      `json:"created"`
	Severity  string    `json:"severity"`
}

type AgentGuardFindingNotifier interface {
	FindingUpdated(context.Context, AgentFindingUpdate) error
}

type AgentGuardRuleProcessor interface {
	ProcessBehavior(
		context.Context,
		*model.AgentBehaviorEvent,
		pipeline.AgentRuleProcessingOptions,
	) (pipeline.AgentRuleProcessingResult, error)
	ProcessGuardEvent(
		context.Context,
		*model.RuntimeEvent,
		pipeline.AgentRuleProcessingOptions,
	) (pipeline.AgentRuleProcessingResult, error)
}

func NewEventHandler(
	repo *repository.RuntimeEventRepository,
	llmAnalyzer *llm_analyzer.LLMAnalyzer,
	alertGen *alert_generator.AlertGenerator,
	aggregator *aggregator.Aggregator,
) *EventHandler {
	return NewEventHandlerWithAgentGuard(repo, llmAnalyzer, alertGen, aggregator, nil, nil, false, nil)
}

func NewEventHandlerWithAgentGuard(
	repo RuntimeEventWriter,
	llmAnalyzer *llm_analyzer.LLMAnalyzer,
	alertGen LegacyAlertGenerator,
	aggregator *aggregator.Aggregator,
	behaviorRepo AgentBehaviorWriter,
	stateRepo AgentGuardStateWriter,
	projectionEnabled bool,
	notifier AgentBehaviorNotifier,
) *EventHandler {
	return NewEventHandlerWithAgentGuardRules(
		repo,
		llmAnalyzer,
		alertGen,
		aggregator,
		behaviorRepo,
		stateRepo,
		projectionEnabled,
		notifier,
		nil,
		pipeline.AgentRuleProcessingOptions{},
	)
}

func NewEventHandlerWithAgentGuardRules(
	repo RuntimeEventWriter,
	llmAnalyzer *llm_analyzer.LLMAnalyzer,
	alertGen LegacyAlertGenerator,
	aggregator *aggregator.Aggregator,
	behaviorRepo AgentBehaviorWriter,
	stateRepo AgentGuardStateWriter,
	projectionEnabled bool,
	notifier AgentBehaviorNotifier,
	ruleProcessor AgentGuardRuleProcessor,
	ruleOptions pipeline.AgentRuleProcessingOptions,
) *EventHandler {
	eventLogger := logger.Get()
	if eventLogger == nil {
		eventLogger = zap.NewNop()
	}
	ruleOptions.FindingsEnabled = ruleOptions.RulesEnabled && ruleOptions.FindingsEnabled
	ruleOptions.AlertsEnabled = ruleOptions.FindingsEnabled && ruleOptions.AlertsEnabled
	ruleOptions.ActionFlags.ActionEnabled = ruleOptions.FindingsEnabled && ruleOptions.ActionFlags.ActionEnabled
	ruleOptions.ActionFlags.DenyEnabled = ruleOptions.ActionFlags.ActionEnabled && ruleOptions.ActionFlags.DenyEnabled
	ruleOptions.ActionFlags.FreezeEnabled = ruleOptions.ActionFlags.ActionEnabled && ruleOptions.ActionFlags.FreezeEnabled
	ruleOptions.ActionFlags.PublishEnabled = ruleOptions.ActionFlags.ActionEnabled && ruleOptions.ActionFlags.PublishEnabled
	return &EventHandler{
		logger:            eventLogger,
		repo:              repo,
		llmAnalyzer:       llmAnalyzer,
		alertGen:          alertGen,
		aggregator:        aggregator,
		behaviorRepo:      behaviorRepo,
		stateRepo:         stateRepo,
		projectionEnabled: projectionEnabled,
		notifier:          notifier,
		ruleProcessor:     ruleProcessor,
		ruleOptions:       ruleOptions,
	}
}

func (h *EventHandler) Handle(event map[string]interface{}) error {
	h.logger.Debug("Handling event",
		zap.String("event_id", stringField(event, "event_id")),
		zap.String("event_type", stringField(event, "event_type")),
	)

	// Extract relevant fields from the event map
	eventID := stringField(event, "event_id")
	eventType := stringField(event, "event_type")
	agentGuardEvent := isAgentGuardEventType(eventType)
	if eventID == "" {
		if agentGuardEvent {
			return pipeline.ErrAgentBehaviorInvalidContract
		}
		eventID = uuid.New().String()
	}

	hostIDStr := stringField(event, "host_id")
	hostID, err := uuid.Parse(hostIDStr)
	if err != nil {
		if agentGuardEvent {
			return pipeline.ErrAgentBehaviorInvalidContract
		}
		hostID = uuid.New()
	}

	eventData := stringField(event, "event_data_json")
	if eventData == "" {
		eventData = stringField(event, "event_data")
	}
	if agentGuardEvent {
		eventData = pipeline.SanitizeAgentGuardEventData(eventData)
	} else {
		eventData = normalizeEventDataJSON(eventData)
	}
	matchedRuleID := stringField(event, "matched_rule_id")
	mitreID := stringField(event, "mitre_id")
	severity := stringField(event, "severity")
	commandLine := stringField(event, "command_line")
	processName := stringField(event, "process_name")
	if agentGuardEvent {
		// P1 Agent Guard facts cannot inherit legacy Sigma rule metadata.
		// Rules/findings/alerts remain behind their later-stage gates.
		matchedRuleID = ""
		mitreID = ""
		commandLine = pipeline.RedactSummary(commandLine)
		processName = pipeline.RedactSummary(processName)
	}

	// Create RuntimeEvent model
	runtimeEvent := &model.RuntimeEvent{
		EventID:       eventID,
		HostID:        hostID,
		EventType:     eventType,
		EventData:     eventData,
		MatchedRuleID: matchedRuleID,
		MitreID:       mitreID,
		Severity:      severity,
		PID:           int(int64Field(event, "pid")),
		CommandLine:   commandLine,
		ProcessName:   processName,
		Timestamp:     int64Field(event, "timestamp"),
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

	if agentGuardEvent {
		return h.handleAgentGuardEvent(ctx, runtimeEvent)
	}

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

func (h *EventHandler) handleAgentGuardEvent(ctx context.Context, runtimeEvent *model.RuntimeEvent) error {
	if !h.projectionEnabled {
		return nil
	}
	switch runtimeEvent.EventType {
	case "agent_behavior":
		return h.handleAgentBehavior(ctx, runtimeEvent)
	case "agent_sandbox_violation", "agent_isolation_drift":
		return h.handleAgentGuardRuleEvent(ctx, runtimeEvent)
	default:
		return h.handleAgentGuardState(ctx, runtimeEvent)
	}
}

func (h *EventHandler) handleAgentBehavior(ctx context.Context, runtimeEvent *model.RuntimeEvent) error {
	if h.behaviorRepo == nil {
		return nil
	}
	projected, err := pipeline.NormalizeAgentBehavior(
		runtimeEvent.EventID,
		runtimeEvent.HostID,
		runtimeEvent.EventData,
	)
	if err != nil {
		h.logger.Warn("agent_behavior_projection_rejected",
			zap.String("event_id", runtimeEvent.EventID),
			zap.String("host_id", runtimeEvent.HostID.String()),
			zap.String("error_code", "agent_behavior_invalid_contract"),
			zap.Error(err),
		)
		return err
	}
	// Keep DC projection lossless with respect to the signed Agent fact. Tool
	// command classification and rule matching belong to api-server; this
	// service must not derive a second tool verdict while persisting the event.
	inserted, err := h.behaviorRepo.CreateWithContext(ctx, projected)
	if err != nil {
		errorCode := "agent_behavior_projection_write_failed"
		if errors.Is(err, repository.ErrAgentGuardStateDependencyMissing) {
			errorCode = "agent_behavior_state_dependency_missing"
		}
		h.logger.Error("agent_behavior_projection_failed",
			zap.String("event_id", runtimeEvent.EventID),
			zap.String("host_id", runtimeEvent.HostID.String()),
			zap.String("category", projected.Category),
			zap.String("operation", projected.Operation),
			zap.String("error_code", errorCode),
			zap.Error(err),
		)
		return err
	}
	if !inserted {
		h.logger.Debug("agent_behavior_projection_replayed",
			zap.String("event_id", runtimeEvent.EventID),
			zap.String("host_id", runtimeEvent.HostID.String()),
		)
	} else {
		h.logger.Info("agent_behavior_projected",
			zap.String("event_id", runtimeEvent.EventID),
			zap.String("host_id", runtimeEvent.HostID.String()),
			zap.String("category", projected.Category),
			zap.String("operation", projected.Operation),
			zap.String("decision", projected.Decision),
			zap.String("visibility", projected.CommandVisibility),
		)
	}
	if inserted && h.notifier != nil {
		update := AgentBehaviorUpdate{
			Type:            "agent_guard.behavior_created",
			EventID:         projected.RawEventID,
			HostID:          projected.HostID,
			InstanceID:      projected.InstanceID,
			SessionID:       projected.SessionID,
			ExecutionUnitID: projected.ExecutionUnitID,
		}
		if err := h.notifier.BehaviorCreated(ctx, update); err != nil {
			h.logger.Warn("agent_behavior_notification_failed",
				zap.String("event_id", runtimeEvent.EventID),
				zap.Error(err),
			)
		}
	}
	// Agent Guard tool rules are evaluated by api-server from the signed
	// upper-layer tool command/input event. DC is only the behavior projection
	// boundary; eBPF/process facts remain enrichment evidence and must not
	// create an independent rule hit here.
	h.logger.Debug("agent_behavior_rule_evaluation_delegated",
		zap.String("event_id", runtimeEvent.EventID),
		zap.String("host_id", runtimeEvent.HostID.String()),
		zap.String("category", projected.Category),
		zap.String("operation", projected.Operation),
		zap.String("rule_owner", "api-server"),
	)
	return nil
}

func (h *EventHandler) handleAgentGuardRuleEvent(ctx context.Context, runtimeEvent *model.RuntimeEvent) error {
	if h.ruleProcessor == nil || !h.ruleOptions.RulesEnabled {
		return nil
	}
	result, err := h.ruleProcessor.ProcessGuardEvent(ctx, runtimeEvent, h.ruleOptions)
	if err != nil {
		h.logger.Warn("agent_guard_escape_processing_failed",
			zap.String("event_id", runtimeEvent.EventID),
			zap.String("host_id", runtimeEvent.HostID.String()),
			zap.String("event_type", runtimeEvent.EventType),
			zap.String("error_code", "agent_guard_escape_invalid_or_unpersisted_evidence"),
			zap.Error(err),
		)
		return err
	}
	h.logger.Info("agent_guard_escape_evaluated",
		zap.String("event_id", runtimeEvent.EventID),
		zap.String("host_id", runtimeEvent.HostID.String()),
		zap.String("event_type", runtimeEvent.EventType),
		zap.Int("rule_hit_count", result.HitCount),
		zap.Int("finding_update_count", len(result.FindingUpdates)),
		zap.Int("action_update_count", len(result.ActionUpdates)),
	)
	h.notifyFindingUpdates(ctx, runtimeEvent, result.FindingUpdates)
	h.notifyActionUpdates(ctx, runtimeEvent, result.ActionUpdates)
	return nil
}

func (h *EventHandler) notifyActionUpdates(
	ctx context.Context,
	runtimeEvent *model.RuntimeEvent,
	updates []pipeline.AgentGuardActionUpdate,
) {
	notifier, ok := h.notifier.(AgentGuardStateNotifier)
	if !ok {
		return
	}
	for _, update := range updates {
		if err := notifier.StateUpdated(ctx, AgentGuardStateUpdate{
			Type: "agent_guard.action_updated", HostID: runtimeEvent.HostID,
			ObjectID: update.ActionID, Action: update.Action, Status: update.Status,
		}); err != nil {
			h.logger.Warn("agent_guard_action_notification_failed",
				zap.String("event_id", runtimeEvent.EventID),
				zap.String("action_id", update.ActionID.String()),
				zap.String("action", update.Action),
				zap.String("status", update.Status),
				zap.Error(err))
		}
	}
}

func (h *EventHandler) notifyFindingUpdates(
	ctx context.Context,
	runtimeEvent *model.RuntimeEvent,
	updates []pipeline.AgentFindingUpdate,
) {
	notifier, ok := h.notifier.(AgentGuardFindingNotifier)
	if !ok {
		return
	}
	for _, update := range updates {
		if err := notifier.FindingUpdated(ctx, AgentFindingUpdate{
			Type:      "agent_guard.finding_updated",
			FindingID: update.FindingID,
			Created:   update.Created,
			Severity:  update.Severity,
		}); err != nil {
			h.logger.Warn("agent_guard_finding_notification_failed",
				zap.String("event_id", runtimeEvent.EventID),
				zap.String("finding_id", update.FindingID.String()),
				zap.Error(err),
			)
		}
	}
}

func (h *EventHandler) handleAgentGuardState(ctx context.Context, runtimeEvent *model.RuntimeEvent) error {
	if h.stateRepo == nil || !isProjectedAgentGuardState(runtimeEvent.EventType) {
		return nil
	}
	projected, err := pipeline.NormalizeAgentGuardState(runtimeEvent.EventType, runtimeEvent.HostID, runtimeEvent.EventData)
	if err != nil {
		h.logger.Warn("agent_guard_state_parse_failed",
			zap.String("event_id", runtimeEvent.EventID),
			zap.String("host_id", runtimeEvent.HostID.String()),
			zap.String("event_type", runtimeEvent.EventType),
			zap.String("error_code", "agent_guard_state_invalid_contract"),
			zap.Error(err),
		)
		return err
	}
	changed, err := h.stateRepo.UpsertWithContext(ctx, projected)
	if err != nil {
		errorCode := "agent_guard_state_projection_write_failed"
		if errors.Is(err, repository.ErrAgentGuardStateDependencyMissing) {
			errorCode = "agent_guard_state_dependency_missing"
		} else if errors.Is(err, repository.ErrAgentGuardDeliveryStateMissing) {
			errorCode = "agent_guard_delivery_state_missing"
		} else if errors.Is(err, repository.ErrAgentGuardActionStateMissing) {
			errorCode = "agent_guard_action_state_missing"
		}
		fields := []zap.Field{
			zap.String("event_id", runtimeEvent.EventID),
			zap.String("host_id", runtimeEvent.HostID.String()),
			zap.String("event_type", runtimeEvent.EventType),
			zap.String("error_code", errorCode),
			zap.Error(err),
		}
		fields = appendStateIdentity(fields, projected)
		h.logger.Error("agent_guard_state_projection_failed", fields...)
		return err
	}
	if !changed {
		return nil
	}
	fields := []zap.Field{
		zap.String("event_id", runtimeEvent.EventID),
		zap.String("host_id", runtimeEvent.HostID.String()),
		zap.String("event_type", runtimeEvent.EventType),
	}
	fields = appendStateIdentity(fields, projected)
	h.logger.Info("agent_guard_state_projected", fields...)
	stateNotifier, ok := h.notifier.(AgentGuardStateNotifier)
	if ok {
		update := AgentGuardStateUpdate{
			Type:     stateUpdateType(runtimeEvent.EventType),
			HostID:   runtimeEvent.HostID,
			ObjectID: projected.ObjectID,
		}
		if projected.Delivery != nil {
			update.BundleVersion = projected.Delivery.BundleVersion
		}
		if projected.Action != nil {
			update.Action = projected.Action.Action
			update.Status = projected.Action.Status
		}
		if err := stateNotifier.StateUpdated(ctx, update); err != nil {
			h.logger.Warn("agent_guard_state_notification_failed",
				zap.String("event_id", runtimeEvent.EventID),
				zap.Error(err),
			)
		}
	}
	return nil
}

func appendStateIdentity(fields []zap.Field, projection *model.AgentGuardStateProjection) []zap.Field {
	if projection.Delivery != nil {
		return append(fields, zap.Int64("bundle_version", projection.Delivery.BundleVersion))
	}
	return append(fields, zap.String("object_id", projection.ObjectID.String()))
}

func isAgentGuardEventType(eventType string) bool {
	switch eventType {
	case "agent_guard_config_status",
		"agent_instance_started", "agent_instance_updated", "agent_instance_stopped",
		"agent_execution_unit_started", "agent_execution_unit_updated", "agent_execution_unit_stopped",
		"agent_behavior_session_started", "agent_behavior_session_updated", "agent_behavior_session_stopped",
		"agent_behavior", "agent_sandbox_violation", "agent_isolation_drift",
		"agent_guard_action_status", "agent_guard_health":
		return true
	default:
		return false
	}
}

func isProjectedAgentGuardState(eventType string) bool {
	switch eventType {
	case "agent_guard_config_status",
		"agent_instance_started", "agent_instance_updated", "agent_instance_stopped",
		"agent_execution_unit_started", "agent_execution_unit_updated", "agent_execution_unit_stopped",
		"agent_behavior_session_started", "agent_behavior_session_updated", "agent_behavior_session_stopped",
		"agent_guard_action_status":
		return true
	default:
		return false
	}
}

func stateUpdateType(eventType string) string {
	switch {
	case eventType == "agent_guard_config_status":
		return "agent_guard.delivery_updated"
	case eventType == "agent_guard_action_status":
		return "agent_guard.action_updated"
	case strings.HasPrefix(eventType, "agent_instance_"):
		return "agent_guard.instance_updated"
	default:
		return "agent_guard.agent_summary_updated"
	}
}

func stringField(event map[string]interface{}, key string) string {
	value, _ := event[key].(string)
	return value
}

func int64Field(event map[string]interface{}, key string) int64 {
	switch value := event[key].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	default:
		return 0
	}
}

func normalizeEventDataJSON(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !json.Valid([]byte(value)) {
		return "{}"
	}
	return value
}
