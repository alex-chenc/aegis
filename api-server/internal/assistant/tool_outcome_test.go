package assistant

import (
	"context"
	"testing"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

func TestNormalizeToolOutcomeKeepsAsyncTriggerNonTerminal(t *testing.T) {
	tool := &ToolSpec{
		Capability: "generate_example",
		ExecutionContract: ToolExecutionContract{
			Mode:                 ToolExecutionAsynchronous,
			CompletionCapability: "get_example_status",
		},
		ResultContract: ToolResultContract{
			AcceptedOnSuccess:  true,
			OperationRefFields: []string{"job_id"},
		},
	}

	outcome := normalizeToolOutcome(tool, map[string]interface{}{"job_id": "job-1"})
	if outcome.Terminal || outcome.OperationStatus != agentruntime.OperationAccepted {
		t.Fatalf("unexpected async trigger outcome: %#v", outcome)
	}
	if len(outcome.SatisfiesCapabilities) != 0 {
		t.Fatalf("accepted operation must not satisfy capabilities: %#v", outcome.SatisfiesCapabilities)
	}
}

// TestNormalizeToolOutcomeExtractsSynchronousProducerRef verifies that a
// synchronous producer (e.g. Asset.Collection.Trigger) that returns a durable
// reference on terminal success exposes it in OperationRef so a downstream step
// can deterministically bind it as a previous_step argument.
func TestNormalizeToolOutcomeExtractsSynchronousProducerRef(t *testing.T) {
	tool := &ToolSpec{
		Capability: "trigger_asset_collection",
		ResultContract: ToolResultContract{
			OperationRefFields: []string{"task_id"},
		},
	}
	outcome := normalizeToolOutcome(tool, map[string]interface{}{"task_id": "T-123", "status": "done"})
	if !outcome.Terminal || outcome.OperationStatus != agentruntime.OperationSucceeded {
		t.Fatalf("expected terminal success, got %#v", outcome)
	}
	if outcome.OperationRef["task_id"] != "T-123" {
		t.Fatalf("expected OperationRef to carry task_id=T-123, got %#v", outcome.OperationRef)
	}
}

func TestNormalizeToolOutcomeUsesTerminalStatusEvidence(t *testing.T) {
	tool := &ToolSpec{
		Capability: "get_example_status",
		ResultContract: ToolResultContract{
			OperationStatusField:  "status",
			SuccessValues:         []string{"generated"},
			PendingValues:         []string{"generating"},
			FailureValues:         []string{"failed"},
			ArtifactRefFields:     []string{"artifact_id", "artifact_type"},
			SatisfiesCapabilities: []string{"generate_example"},
		},
	}

	running := normalizeToolOutcome(tool, map[string]interface{}{"status": "generating"})
	if running.Terminal || running.OperationStatus != agentruntime.OperationRunning {
		t.Fatalf("unexpected running outcome: %#v", running)
	}

	generated := normalizeToolOutcome(tool, map[string]interface{}{
		"status":        "generated",
		"artifact_id":   "artifact-1",
		"artifact_type": "poc",
	})
	if !generated.Terminal || generated.OperationStatus != agentruntime.OperationSucceeded {
		t.Fatalf("unexpected generated outcome: %#v", generated)
	}
	if !containsDecisionString(generated.SatisfiesCapabilities, "generate_example") {
		t.Fatalf("terminal status did not satisfy trigger capability: %#v", generated.SatisfiesCapabilities)
	}
}

func TestNormalizeToolOutcomeRejectsMissingDeclaredStatusEvidence(t *testing.T) {
	tool := &ToolSpec{
		Capability: "get_example_status",
		ResultContract: ToolResultContract{
			OperationStatusField: "status",
			SuccessValues:        []string{"completed"},
			PendingValues:        []string{"running"},
			FailureValues:        []string{"failed"},
		},
	}

	outcome := normalizeToolOutcome(tool, map[string]interface{}{"job_id": "job-1"})
	if !outcome.Terminal || outcome.OperationStatus != agentruntime.OperationFailed {
		t.Fatalf("missing declared status must not become business success: %#v", outcome)
	}
	if len(outcome.SatisfiesCapabilities) != 0 {
		t.Fatalf("missing status must not satisfy capability: %#v", outcome.SatisfiesCapabilities)
	}
}

func TestNormalizeToolOutcomeRejectsMissingRequiredSideEffect(t *testing.T) {
	tool := &ToolSpec{
		Capability: "execute_example",
		ResultContract: ToolResultContract{
			SideEffectRefFields: []string{"task_group_id"},
		},
	}

	outcome := normalizeToolOutcome(tool, map[string]interface{}{"status": "ok"})
	if outcome.OperationStatus != agentruntime.OperationFailed {
		t.Fatalf("missing task-group evidence must be a business failure: %#v", outcome)
	}
	if len(outcome.SatisfiesCapabilities) != 0 {
		t.Fatalf("missing side effect must not satisfy capability: %#v", outcome.SatisfiesCapabilities)
	}
}

func TestNormalizeToolOutcomeExtractsOnlineHostFacts(t *testing.T) {
	tool := &ToolSpec{
		Capability: "list_hosts",
		ResultContract: ToolResultContract{
			FactBindings: []ToolFactBinding{{
				Kind:       "host_online",
				ItemsField: "data",
				IDField:    "id",
				StateField: "agent_status",
				StateValue: "online",
			}},
		},
	}

	outcome := normalizeToolOutcome(tool, map[string]interface{}{
		"status": "all",
		"data": []interface{}{
			map[string]interface{}{"id": "host-1", "agent_status": "online"},
			map[string]interface{}{"id": "host-2", "agent_status": "offline"},
		},
	})
	if len(outcome.Facts) != 1 || outcome.Facts[0]["id"] != "host-1" {
		t.Fatalf("online host facts = %#v", outcome.Facts)
	}
}

func TestNormalizeToolOutcomeExtractsFactsFromTypedHandlerSlice(t *testing.T) {
	tool := &ToolSpec{
		Capability: "list_hosts",
		ResultContract: ToolResultContract{
			FactBindings: []ToolFactBinding{{
				Kind:       "host_online",
				ItemsField: "data",
				IDField:    "id",
				StateField: "agent_status",
				StateValue: "online",
			}},
		},
	}

	outcome := normalizeToolOutcome(tool, map[string]interface{}{
		"data": []map[string]interface{}{
			{"id": "host-typed", "agent_status": "online"},
		},
	})
	if len(outcome.Facts) != 1 || outcome.Facts[0]["id"] != "host-typed" {
		t.Fatalf("typed handler facts = %#v", outcome.Facts)
	}
}

func TestToolDispatcherPersistsBusinessOutcomeSeparatelyFromTransportStatus(t *testing.T) {
	registry := NewToolRegistry()
	if err := registry.Register(&ToolSpec{
		Name:               "Example.Async.Start",
		Capability:         "start_example_async",
		Risk:               ToolRiskMedium,
		DefaultWhitelisted: true,
		Enabled:            true,
		ExecutionContract: ToolExecutionContract{
			Mode:                 ToolExecutionAsynchronous,
			CompletionCapability: "get_example_async_status",
		},
		ResultContract: ToolResultContract{
			AcceptedOnSuccess:  true,
			OperationRefFields: []string{"job_id"},
		},
		Handler: func(context.Context, map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"job_id": "job-1"}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	dispatcher, repo := newTestToolDispatcher(t, registry)
	result, err := dispatcher.Dispatch(context.Background(), DispatchRequest{
		SessionID: "session-1",
		MessageID: "message-1",
		RunID:     "run-1",
		CallID:    "call-1",
		ToolName:  "Example.Async.Start",
		Args:      map[string]interface{}{},
		Operator:  "tester",
		Approved:  true,
	})
	if err != nil || !result.Success {
		t.Fatalf("dispatch result=%#v err=%v", result, err)
	}
	if len(repo.calls) != 1 {
		t.Fatalf("persisted calls=%d", len(repo.calls))
	}
	call := repo.calls[0]
	if call.Status != "success" {
		t.Fatalf("transport status=%q", call.Status)
	}
	if call.OperationStatus != string(agentruntime.OperationAccepted) {
		t.Fatalf("operation status=%q", call.OperationStatus)
	}
	if call.OperationTerminal == nil || *call.OperationTerminal {
		t.Fatalf("operation terminal=%v", call.OperationTerminal)
	}
}
