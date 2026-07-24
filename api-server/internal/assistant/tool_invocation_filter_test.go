package assistant

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestToolInvocationFilterCanonicalizesLiveHostSelector(t *testing.T) {
	registry := newInvocationFilterTestRegistry(t)
	filters := NewToolInvocationFilterChain(registry, zap.NewNop())

	prepared, err := filters.Prepare(context.Background(), ToolInvocationFilterRequest{
		SessionID: "session-1",
		RunID:     "run-1",
		StepID:    "step-1",
		Phase:     ToolInvocationPhaseCandidate,
		ToolName:  "Host.Resolve",
		Args:      map[string]interface{}{"selector": "live"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := prepared.Args["selector"]; exists {
		t.Fatalf("legacy selector survived preparation: %#v", prepared.Args)
	}
	if prepared.Args["target_scope"] != "all_online_hosts" || prepared.Args["require_online"] != true {
		t.Fatalf("live selector was not canonicalized: %#v", prepared.Args)
	}
	if !prepared.Modified {
		t.Fatalf("expected preparation to record a safe modification: %#v", prepared)
	}
}

func TestToolInvocationFilterBlocksUnknownArgs(t *testing.T) {
	registry := newInvocationFilterTestRegistry(t)
	filters := NewToolInvocationFilterChain(registry, zap.NewNop())

	_, err := filters.Prepare(context.Background(), ToolInvocationFilterRequest{
		Phase:    ToolInvocationPhaseCandidate,
		ToolName: "Host.Resolve",
		Args:     map[string]interface{}{"get_schema": true},
	})
	if err == nil {
		t.Fatal("expected unknown argument to be blocked")
	}
}

func TestToolInvocationFilterDoesNotMutateApprovalResume(t *testing.T) {
	registry := newInvocationFilterTestRegistry(t)
	filters := NewToolInvocationFilterChain(registry, zap.NewNop())

	_, err := filters.Prepare(context.Background(), ToolInvocationFilterRequest{
		Phase:    ToolInvocationPhaseApprovalResume,
		ToolName: "Host.Resolve",
		Args:     map[string]interface{}{"selector": "live"},
	})
	if err == nil {
		t.Fatal("approval resume must validate persisted args without modifying them")
	}
}

func newInvocationFilterTestRegistry(t *testing.T) *ToolRegistry {
	t.Helper()
	registry := NewToolRegistry()
	if err := registry.Register(&ToolSpec{
		Name:      "Host.Resolve",
		Domain:    DomainHost,
		Operation: OpGet,
		Risk:      ToolRiskReadonly,
		Enabled:   true,
		ExposurePolicy: ToolExposurePolicy{
			Exposure:       ToolExposurePrimary,
			Discoverable:   true,
			DirectCallable: true,
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_selectors": map[string]interface{}{
					"type":     "array",
					"minItems": 1,
					"items":    map[string]interface{}{"type": "string"},
				},
				"target_scope": map[string]interface{}{
					"type": "string",
					"enum": []interface{}{"all_online_hosts"},
				},
				"require_online": map[string]interface{}{"type": "boolean"},
			},
			"additionalProperties": false,
		},
		Handler: func(context.Context, map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"ok": true}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	return registry
}
