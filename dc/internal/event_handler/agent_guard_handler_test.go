package event_handler

import (
	"context"
	"strings"
	"testing"

	"dc/internal/model"
	"dc/internal/pipeline"
	"dc/internal/repository"
	"dc/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type fakeRuntimeWriter struct {
	events map[string]*model.RuntimeEvent
}

func (f *fakeRuntimeWriter) CreateWithContext(_ context.Context, event *model.RuntimeEvent) error {
	if f.events == nil {
		f.events = make(map[string]*model.RuntimeEvent)
	}
	if _, exists := f.events[event.EventID]; !exists {
		f.events[event.EventID] = event
	}
	return nil
}

type fakeBehaviorWriter struct {
	events map[string]*model.AgentBehaviorEvent
	err    error
}

func (f *fakeBehaviorWriter) CreateWithContext(_ context.Context, event *model.AgentBehaviorEvent) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if f.events == nil {
		f.events = make(map[string]*model.AgentBehaviorEvent)
	}
	if _, exists := f.events[event.RawEventID]; exists {
		return false, nil
	}
	f.events[event.RawEventID] = event
	return true, nil
}

type fakeBehaviorNotifier struct {
	updates  []AgentBehaviorUpdate
	states   []AgentGuardStateUpdate
	findings []AgentFindingUpdate
}

type fakeRuleProcessor struct {
	behaviorCalls int
	guardCalls    int
	options       []pipeline.AgentRuleProcessingOptions
}

func (f *fakeRuleProcessor) ProcessBehavior(
	_ context.Context,
	_ *model.AgentBehaviorEvent,
	options pipeline.AgentRuleProcessingOptions,
) (pipeline.AgentRuleProcessingResult, error) {
	f.behaviorCalls++
	f.options = append(f.options, options)
	if f.behaviorCalls > 1 {
		return pipeline.AgentRuleProcessingResult{HitCount: 1}, nil
	}
	return pipeline.AgentRuleProcessingResult{
		HitCount: 1,
		FindingUpdates: []pipeline.AgentFindingUpdate{{
			FindingID: uuid.MustParse("62000000-0000-4000-8000-000000000001"),
			Created:   true, Changed: true, Severity: "high",
		}},
	}, nil
}

func (f *fakeRuleProcessor) ProcessGuardEvent(
	_ context.Context,
	_ *model.RuntimeEvent,
	options pipeline.AgentRuleProcessingOptions,
) (pipeline.AgentRuleProcessingResult, error) {
	f.guardCalls++
	f.options = append(f.options, options)
	return pipeline.AgentRuleProcessingResult{HitCount: 1}, nil
}

type fakeStateWriter struct {
	kinds []string
	seen  map[string]bool
}

func (f *fakeStateWriter) UpsertWithContext(_ context.Context, projection *model.AgentGuardStateProjection) (bool, error) {
	if f.seen == nil {
		f.seen = make(map[string]bool)
	}
	key := projection.EventType + ":" + projection.ObjectID.String()
	if f.seen[key] {
		return false, nil
	}
	f.seen[key] = true
	switch {
	case projection.Instance != nil:
		f.kinds = append(f.kinds, "instance")
	case projection.Unit != nil:
		f.kinds = append(f.kinds, "unit")
	case projection.Session != nil:
		f.kinds = append(f.kinds, "session")
	case projection.Delivery != nil:
		f.kinds = append(f.kinds, "delivery")
	case projection.Action != nil:
		f.kinds = append(f.kinds, "action")
	}
	return true, nil
}

type fakeAlertGenerator struct {
	calls int
}

func (f *fakeAlertGenerator) GenerateAlert(_ *model.RuntimeEvent) *model.Alert {
	f.calls++
	return &model.Alert{}
}

func (f *fakeBehaviorNotifier) BehaviorCreated(_ context.Context, update AgentBehaviorUpdate) error {
	f.updates = append(f.updates, update)
	return nil
}

func (f *fakeBehaviorNotifier) StateUpdated(_ context.Context, update AgentGuardStateUpdate) error {
	f.states = append(f.states, update)
	return nil
}

func (f *fakeBehaviorNotifier) FindingUpdated(_ context.Context, update AgentFindingUpdate) error {
	f.findings = append(f.findings, update)
	return nil
}

func TestAgentBehaviorProjectionIsFlaggedAndReplaySafe(t *testing.T) {
	logger.Logger = zap.NewNop()
	logger.Sugar = logger.Logger.Sugar()
	hostID := uuid.New()
	eventID := uuid.NewString()
	instanceID := uuid.NewString()
	unitID := uuid.NewString()
	sessionID := uuid.NewString()
	eventData := `{"schema":"aegis.agent_behavior.v1","event_id":"` + eventID + `","host_id":"` + hostID.String() + `","host_boot_id":"boot-1","agent_sequence":7,"instance_id":"` + instanceID + `","execution_unit_id":"` + unitID + `","session_id":"` + sessionID + `","occurred_at":"2026-07-30T10:00:00Z","occurred_monotonic_ns":987654321012345678,"category":"process","operation":"exec","outcome":"success","actor":{"pid":10,"start_ticks":99,"argv":["bash","-lc","echo ok"]},"attribution_confidence":"confirmed","collection":{"source":"ebpf","sensor":"execve","visibility":"complete","lost_events_since_last":0}}`
	input := map[string]interface{}{
		"event_id":        eventID,
		"host_id":         hostID.String(),
		"event_type":      "agent_behavior",
		"event_data_json": eventData,
		"severity":        "info",
	}

	raw := &fakeRuntimeWriter{}
	behavior := &fakeBehaviorWriter{}
	notifier := &fakeBehaviorNotifier{}
	alerts := &fakeAlertGenerator{}
	input["matched_rule_id"] = "must-not-run-in-p1"
	input["severity"] = "critical"
	disabled := NewEventHandlerWithAgentGuard(raw, nil, alerts, nil, behavior, nil, false, notifier)
	if err := disabled.Handle(input); err != nil {
		t.Fatalf("disabled Handle: %v", err)
	}
	if len(raw.events) != 1 || len(behavior.events) != 0 || len(notifier.updates) != 0 {
		t.Fatalf("disabled counts raw=%d behavior=%d updates=%d", len(raw.events), len(behavior.events), len(notifier.updates))
	}

	enabled := NewEventHandlerWithAgentGuard(raw, nil, alerts, nil, behavior, nil, true, notifier)
	if err := enabled.Handle(input); err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	if err := enabled.Handle(input); err != nil {
		t.Fatalf("replay Handle: %v", err)
	}
	if len(raw.events) != 1 || len(behavior.events) != 1 || len(notifier.updates) != 1 {
		t.Fatalf("enabled counts raw=%d behavior=%d updates=%d", len(raw.events), len(behavior.events), len(notifier.updates))
	}
	if alerts.calls != 0 {
		t.Fatalf("ordinary Agent Behavior invoked legacy alert generator %d times", alerts.calls)
	}
	if raw.events[eventID].MatchedRuleID != "" {
		t.Fatalf("P1 raw behavior retained legacy rule metadata: %q", raw.events[eventID].MatchedRuleID)
	}
	if notifier.updates[0].Type != "agent_guard.behavior_created" {
		t.Fatalf("update type = %q", notifier.updates[0].Type)
	}
}

func TestLegacyRuntimeEventPathIsUnaffected(t *testing.T) {
	logger.Logger = zap.NewNop()
	logger.Sugar = logger.Logger.Sugar()
	raw := &fakeRuntimeWriter{}
	alerts := &fakeAlertGenerator{}
	handler := NewEventHandlerWithAgentGuard(raw, nil, alerts, nil, nil, nil, true, nil)
	eventID := uuid.NewString()
	if err := handler.Handle(map[string]interface{}{
		"event_id":        eventID,
		"host_id":         uuid.NewString(),
		"event_type":      "process_exec",
		"event_data":      `{"process":"bash"}`,
		"matched_rule_id": "legacy-rule",
		"severity":        "medium",
		"pid":             float64(42),
		"timestamp":       float64(1_722_336_000_000),
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if alerts.calls != 1 || raw.events[eventID].MatchedRuleID != "legacy-rule" ||
		raw.events[eventID].PID != 42 || raw.events[eventID].Timestamp != 1_722_336_000_000 {
		t.Fatalf("legacy event changed: alerts=%d raw=%#v", alerts.calls, raw.events[eventID])
	}
}

func TestInvalidAgentBehaviorKeepsSanitizedRawButRejectsProjection(t *testing.T) {
	logger.Logger = zap.NewNop()
	logger.Sugar = logger.Logger.Sugar()
	raw := &fakeRuntimeWriter{}
	behavior := &fakeBehaviorWriter{}
	handler := NewEventHandlerWithAgentGuard(raw, nil, nil, nil, behavior, nil, true, nil)
	eventID := uuid.NewString()
	err := handler.Handle(map[string]interface{}{
		"event_id":        eventID,
		"host_id":         uuid.NewString(),
		"event_type":      "agent_behavior",
		"event_data_json": `{"schema":"aegis.agent_behavior.v2","authorization":"secret"}`,
		"command_line":    "curl --token secret",
	})
	if err == nil {
		t.Fatal("expected projection validation error")
	}
	if len(raw.events) != 1 || len(behavior.events) != 0 {
		t.Fatalf("counts raw=%d behavior=%d", len(raw.events), len(behavior.events))
	}
	if raw.events[eventID].CommandLine == "curl --token secret" {
		t.Fatal("raw summary command line was not redacted")
	}
	if strings.Contains(raw.events[eventID].EventData, "secret") {
		t.Fatalf("raw event data leaked secret: %s", raw.events[eventID].EventData)
	}
}

func TestAgentGuardLifecycleProjectsInDependencyOrder(t *testing.T) {
	logger.Logger = zap.NewNop()
	logger.Sugar = logger.Logger.Sugar()
	hostID := uuid.New()
	instanceID := uuid.NewString()
	unitID := uuid.NewString()
	sessionID := uuid.NewString()
	raw := &fakeRuntimeWriter{}
	state := &fakeStateWriter{}
	notifier := &fakeBehaviorNotifier{}
	handler := NewEventHandlerWithAgentGuard(raw, nil, nil, nil, nil, state, true, notifier)
	events := []map[string]interface{}{
		{
			"event_id": uuid.NewString(), "host_id": hostID.String(), "event_type": "agent_instance_started",
			"event_data_json": `{"schema":"aegis.agent_guard.v1","instance_id":"` + instanceID + `","profile_key":"codex-linux","profile_version":1,"agent_type":"codex","controller_pid":10,"controller_start_ticks":99,"detection_confidence":"confirmed","status":"running","coverage_level":"monitor_only","coverage_reasons":[],"first_seen_at":"2026-07-30T10:00:00Z","last_seen_at":"2026-07-30T10:00:01Z"}`,
		},
		{
			"event_id": uuid.NewString(), "host_id": hostID.String(), "event_type": "agent_execution_unit_started",
			"event_data_json": `{"schema":"aegis.agent_guard.v1","execution_unit_id":"` + unitID + `","instance_id":"` + instanceID + `","unit_type":"local_process_tree","fingerprint":"unit","root_pid":10,"root_start_ticks":99,"coverage_level":"no_isolation","coverage_reasons":[],"status":"observed","first_seen_at":"2026-07-30T10:00:00Z","last_seen_at":"2026-07-30T10:00:01Z"}`,
		},
		{
			"event_id": uuid.NewString(), "host_id": hostID.String(), "event_type": "agent_behavior_session_started",
			"event_data_json": `{"schema":"aegis.agent_guard.v1","session_id":"` + sessionID + `","instance_id":"` + instanceID + `","execution_unit_id":"` + unitID + `","source":"activity_window","confidence":"inferred","status":"active","completeness":{},"started_at":"2026-07-30T10:00:00Z","last_seen_at":"2026-07-30T10:00:01Z"}`,
		},
	}
	for _, event := range events {
		if err := handler.Handle(event); err != nil {
			t.Fatalf("Handle(%s): %v", event["event_type"], err)
		}
	}
	for _, event := range events {
		if err := handler.Handle(event); err != nil {
			t.Fatalf("replay Handle(%s): %v", event["event_type"], err)
		}
	}
	if strings.Join(state.kinds, ",") != "instance,unit,session" {
		t.Fatalf("projection order = %v", state.kinds)
	}
	if len(raw.events) != 3 || len(notifier.states) != 3 {
		t.Fatalf("raw=%d state updates=%d", len(raw.events), len(notifier.states))
	}
}

func TestBehaviorMissingLifecycleStateKeepsRaw(t *testing.T) {
	logger.Logger = zap.NewNop()
	logger.Sugar = logger.Logger.Sugar()
	raw := &fakeRuntimeWriter{}
	behavior := &fakeBehaviorWriter{err: repository.ErrAgentGuardStateDependencyMissing}
	handler := NewEventHandlerWithAgentGuard(raw, nil, nil, nil, behavior, nil, true, nil)
	hostID := uuid.New()
	eventID := uuid.NewString()
	err := handler.Handle(map[string]interface{}{
		"event_id": eventID, "host_id": hostID.String(), "event_type": "agent_behavior",
		"event_data_json": `{"schema":"aegis.agent_behavior.v1","event_id":"` + eventID + `","host_id":"` + hostID.String() + `","host_boot_id":"boot","agent_sequence":1,"instance_id":"` + uuid.NewString() + `","execution_unit_id":"` + uuid.NewString() + `","session_id":"` + uuid.NewString() + `","occurred_at":"2026-07-30T10:00:00Z","occurred_monotonic_ns":1,"category":"process","operation":"exec","outcome":"success","actor":{"pid":10,"start_ticks":99},"attribution_confidence":"confirmed","collection":{"source":"ebpf","sensor":"execve","lost_events_since_last":0}}`,
	})
	if err == nil || len(raw.events) != 1 || len(behavior.events) != 0 {
		t.Fatalf("err=%v raw=%d behavior=%d", err, len(raw.events), len(behavior.events))
	}
}

func TestAgentGuardConfigStatusProjectsDeliveryUpdate(t *testing.T) {
	logger.Logger = zap.NewNop()
	logger.Sugar = logger.Logger.Sugar()
	raw := &fakeRuntimeWriter{}
	state := &fakeStateWriter{}
	notifier := &fakeBehaviorNotifier{}
	handler := NewEventHandlerWithAgentGuard(raw, nil, nil, nil, nil, state, true, notifier)
	err := handler.Handle(map[string]interface{}{
		"event_id":   uuid.NewString(),
		"host_id":    uuid.NewString(),
		"event_type": "agent_guard_config_status",
		"event_data_json": `{"schema":"aegis.agent_guard.v1","status":"applied","bundle_version":7,` +
			`"digest":"sha256:test","occurred_at":"2026-07-30T10:00:00Z"}`,
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if strings.Join(state.kinds, ",") != "delivery" || len(notifier.states) != 1 ||
		notifier.states[0].Type != "agent_guard.delivery_updated" ||
		notifier.states[0].BundleVersion != 7 {
		t.Fatalf("state=%v notifier=%#v", state.kinds, notifier.states)
	}
}

func TestAgentGuardActionStatusProjectsRealTerminalResult(t *testing.T) {
	logger.Logger = zap.NewNop()
	logger.Sugar = logger.Logger.Sugar()
	raw := &fakeRuntimeWriter{}
	state := &fakeStateWriter{}
	notifier := &fakeBehaviorNotifier{}
	handler := NewEventHandlerWithAgentGuard(raw, nil, nil, nil, nil, state, true, notifier)
	actionID := uuid.NewString()
	err := handler.Handle(map[string]interface{}{
		"event_id": uuid.NewString(), "host_id": uuid.NewString(),
		"event_type": "agent_guard_action_status",
		"event_data_json": `{"schema":"aegis.agent_guard.v1","action_id":"` + actionID +
			`","command_id":"AG-GUARD-` + actionID + `","execution_unit_id":"` + uuid.NewString() +
			`","action":"freeze_execution_unit","status":"failed",` +
			`"error_code":"freezer_unavailable","error_message":"freezer unavailable",` +
			`"result":{"state_changed":false},"occurred_at":"2026-07-30T10:00:00Z"}`,
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if strings.Join(state.kinds, ",") != "action" || len(notifier.states) != 1 ||
		notifier.states[0].Type != "agent_guard.action_updated" {
		t.Fatalf("state=%v notifier=%#v", state.kinds, notifier.states)
	}
}

func TestAgentGuardBehaviorRulesAreDelegatedToAPIServer(t *testing.T) {
	logger.Logger = zap.NewNop()
	logger.Sugar = logger.Logger.Sugar()
	hostID := uuid.New()
	eventID := uuid.NewString()
	instanceID := uuid.NewString()
	unitID := uuid.NewString()
	sessionID := uuid.NewString()
	raw := &fakeRuntimeWriter{}
	behavior := &fakeBehaviorWriter{}
	notifier := &fakeBehaviorNotifier{}
	rules := &fakeRuleProcessor{}
	handler := NewEventHandlerWithAgentGuardRules(
		raw, nil, nil, nil, behavior, nil, true, notifier, rules,
		pipeline.AgentRuleProcessingOptions{
			RulesEnabled: true, FindingsEnabled: false, AlertsEnabled: true,
		},
	)
	input := map[string]interface{}{
		"event_id": eventID, "host_id": hostID.String(), "event_type": "agent_behavior",
		"event_data_json": `{"schema":"aegis.agent_behavior.v1","event_id":"` + eventID +
			`","host_id":"` + hostID.String() + `","host_boot_id":"boot","agent_sequence":1,` +
			`"instance_id":"` + instanceID + `","execution_unit_id":"` + unitID +
			`","session_id":"` + sessionID + `","occurred_at":"2026-07-30T10:00:00Z",` +
			`"occurred_monotonic_ns":1,"category":"file","operation":"read_observed","outcome":"success",` +
			`"actor":{"pid":10,"ppid":1,"start_ticks":99},"resource":{"type":"file","identity":"/etc/shadow",` +
			`"attributes":{"resolved_path":"/etc/shadow"}},"attribution_confidence":"confirmed",` +
			`"collection":{"source":"ebpf","sensor":"file_open","lost_events_since_last":0}}`,
	}
	if err := handler.Handle(input); err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	if err := handler.Handle(input); err != nil {
		t.Fatalf("replay Handle: %v", err)
	}
	if rules.behaviorCalls != 0 {
		t.Fatalf("DC evaluated behavior rules %d times; rule ownership belongs to api-server", rules.behaviorCalls)
	}
	if len(notifier.updates) != 1 || len(notifier.findings) != 0 {
		t.Fatalf("behavior updates=%d finding updates=%d", len(notifier.updates), len(notifier.findings))
	}
}
