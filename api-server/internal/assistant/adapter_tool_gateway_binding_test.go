package assistant

import (
	"context"
	"strings"
	"testing"

	agentruntime "github.com/alex-chenc/agent-runtime"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// registerBindingTestTools registers a producer/consumer pair used to verify
// deterministic previous_step argument binding. The producer returns a task_id
// reference; the consumer requires task_id and declares it as a previous_step
// argument (left unbound at decision time, exactly like Operation.Get /
// Asset.Collection.Get / Vulnerability.Scan.Status).
func registerBindingTestTools(t *testing.T, registry *ToolRegistry, consumerTaskID *string) {
	t.Helper()
	if err := registry.Register(&ToolSpec{
		Name:               "Example.Produce",
		Domain:             DomainSystem,
		Operation:          OpExecute,
		Capability:         "produce_task",
		Description:        "Produce a task and return its reference.",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		Idempotent:         true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		ResultContract: ToolResultContract{OperationRefFields: []string{"task_id"}},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"task_id": "T-123", "status": "done"}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(&ToolSpec{
		Name:               "Example.Consume",
		Domain:             DomainSystem,
		Operation:          OpGet,
		Capability:         "get_task",
		Description:        "Get a task by id.",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		Idempotent:         true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_id": map[string]interface{}{"type": "string"},
			},
			"required": []string{"task_id"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			if consumerTaskID != nil {
				*consumerTaskID, _ = args["task_id"].(string)
			}
			return map[string]interface{}{"task_id": args["task_id"], "status": "ok"}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func bindingTestPlan() *ToolExecutionPlan {
	return &ToolExecutionPlan{
		Goal:            "produce then consume",
		DecisionTraceID: "td_binding_test",
		Steps: []ToolPlanStep{
			{
				StepID:     "authorized_01",
				ToolName:   "Example.Produce",
				Capability: "produce_task",
				Risk:       string(ToolRiskReadonly),
				ArgSources: map[string]ArgSource{},
			},
			{
				StepID:   "authorized_02",
				ToolName: "Example.Consume",
				Args:     map[string]interface{}{}, // task_id unbound at decision time
				ArgSources: map[string]ArgSource{
					"task_id": {SourceType: "previous_step", SourceRef: "task"},
				},
				Risk: string(ToolRiskReadonly),
			},
		},
	}
}

// TestAssistantToolGatewayBindsPreviousStepArgFromPriorOutcome verifies that a
// downstream step's previous_step argument is deterministically bound from the
// producer's captured outcome, overriding any model-invented value.
func TestAssistantToolGatewayBindsPreviousStepArgFromPriorOutcome(t *testing.T) {
	registry := NewToolRegistry()
	var consumerTaskID string
	registerBindingTestTools(t, registry, &consumerTaskID)

	dispatcher, _ := newTestToolDispatcher(t, registry)
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher:        dispatcher,
		SessionID:         "sess-binding",
		MessageID:         "msg-1",
		RunID:             "run-1",
		RequireMappedPlan: true,
		ExecutionPlan:     bindingTestPlan(),
		Logger:            zap.NewNop(),
	})

	// Step 1: producer returns task_id and the gateway captures its reference.
	if _, err := gateway.Call(context.Background(), agentruntime.ToolRequest{
		CallID:   "call-produce",
		StepID:   "authorized_01",
		ToolName: "Example.Produce",
		Args:     map[string]interface{}{},
	}); err != nil {
		t.Fatalf("producer call failed: %v", err)
	}

	gateway.outcomesMu.Lock()
	captured, ok := gateway.priorStepOutcomes["authorized_01"]
	gateway.outcomesMu.Unlock()
	if !ok || captured.operationRef["task_id"] != "T-123" {
		t.Fatalf("expected producer outcome to capture task_id=T-123, got %#v", captured)
	}

	// Step 2: the model attempts an invented task_id; the backend must override
	// it with the deterministic value from the producer outcome.
	resp, err := gateway.Call(context.Background(), agentruntime.ToolRequest{
		CallID:   "call-consume",
		StepID:   "authorized_02",
		ToolName: "Example.Consume",
		Args:     map[string]interface{}{"task_id": "MODEL_INVENTED"},
	})
	if err != nil {
		t.Fatalf("consumer call failed: %v", err)
	}
	if resp.Status != agentruntime.ToolCallSuccess {
		t.Fatalf("expected success, got %s: %s", resp.Status, resp.ErrorMessage)
	}
	if consumerTaskID != "T-123" {
		t.Fatalf("expected previous_step bound task_id T-123, got %q", consumerTaskID)
	}
}

// TestAssistantToolGatewaySkipsStepWhenPreviousStepArgUnresolvable verifies
// that a step whose required previous_step argument cannot be resolved (its
// producer did not run) is skipped rather than dispatched with a model-invented
// or missing value.
func TestAssistantToolGatewaySkipsStepWhenPreviousStepArgUnresolvable(t *testing.T) {
	registry := NewToolRegistry()
	var consumerTaskID string
	registerBindingTestTools(t, registry, &consumerTaskID)

	dispatcher, _ := newTestToolDispatcher(t, registry)
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher:        dispatcher,
		SessionID:         "sess-skip",
		MessageID:         "msg-1",
		RunID:             "run-1",
		RequireMappedPlan: true,
		ExecutionPlan:     bindingTestPlan(),
		Logger:            zap.NewNop(),
	})

	// Call the consumer WITHOUT first running its producer. The required
	// previous_step task_id is unresolvable, so the step must be skipped.
	_, err := gateway.Call(context.Background(), agentruntime.ToolRequest{
		CallID:   "call-consume-skip",
		StepID:   "authorized_02",
		ToolName: "Example.Consume",
		Args:     map[string]interface{}{"task_id": "MODEL_INVENTED"},
	})
	if err == nil {
		t.Fatalf("expected skip error for unresolvable previous_step argument")
	}
	if !strings.Contains(err.Error(), "skipped") || !strings.Contains(err.Error(), "task_id") {
		t.Fatalf("expected skip error mentioning task_id, got %v", err)
	}
	if consumerTaskID != "" {
		t.Fatalf("consumer handler must not run when step is skipped, got task_id=%q", consumerTaskID)
	}
}

// TestAssistantToolGatewayDoesNotSkipWhenPreviousStepArgOptional confirms an
// optional previous_step argument that cannot be resolved does not skip the
// step (only required previous_step args gate execution).
func TestAssistantToolGatewayDoesNotSkipWhenPreviousStepArgOptional(t *testing.T) {
	registry := NewToolRegistry()
	if err := registry.Register(&ToolSpec{
		Name:               "Example.OptionalConsumer",
		Domain:             DomainSystem,
		Operation:          OpGet,
		Capability:         "get_optional",
		Description:        "Get with optional previous_step arg.",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		Idempotent:         true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"hint": map[string]interface{}{"type": "string"}},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"ok": true}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	plan := &ToolExecutionPlan{
		Goal:            "optional consumer",
		DecisionTraceID: "td_optional_" + uuid.New().String()[:8],
		Steps: []ToolPlanStep{{
			StepID:     "authorized_01",
			ToolName:   "Example.OptionalConsumer",
			Capability: "get_optional",
			Risk:       string(ToolRiskReadonly),
			ArgSources: map[string]ArgSource{
				"hint": {SourceType: "previous_step", SourceRef: "hint"},
			},
		}},
	}
	dispatcher, _ := newTestToolDispatcher(t, registry)
	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher:        dispatcher,
		SessionID:         "sess-optional",
		MessageID:         "msg-1",
		RunID:             "run-1",
		RequireMappedPlan: true,
		ExecutionPlan:     plan,
		Logger:            zap.NewNop(),
	})

	resp, err := gateway.Call(context.Background(), agentruntime.ToolRequest{
		CallID:   "call-optional",
		StepID:   "authorized_01",
		ToolName: "Example.OptionalConsumer",
		Args:     map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("optional previous_step arg must not skip the step: %v", err)
	}
	if resp.Status != agentruntime.ToolCallSuccess {
		t.Fatalf("expected success, got %s: %s", resp.Status, resp.ErrorMessage)
	}
}
