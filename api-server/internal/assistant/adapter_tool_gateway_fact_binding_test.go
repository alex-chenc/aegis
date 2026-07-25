package assistant

import "testing"

func TestResolveEntityIDFromFactsBindsUniqueSingularID(t *testing.T) {
	outcomes := []capturedStepOutcome{{
		facts: []map[string]interface{}{{
			"kind": "host_resolved",
			"id":   "cf18f7f7-5b45-46e2-9889-160dddc4ee30",
		}},
	}}

	got, ok := resolveEntityIDFromFacts(outcomes, "host_id")
	if !ok || got != "cf18f7f7-5b45-46e2-9889-160dddc4ee30" {
		t.Fatalf("singular host_id binding = %q, %v", got, ok)
	}
}

func TestResolveEntityIDFromFactsRejectsAmbiguousSingularID(t *testing.T) {
	outcomes := []capturedStepOutcome{{
		facts: []map[string]interface{}{
			{"kind": "host_resolved", "id": "host-1"},
			{"kind": "host_resolved", "id": "host-2"},
		},
	}}

	if got, ok := resolveEntityIDFromFacts(outcomes, "host_id"); ok {
		t.Fatalf("ambiguous singular ID must not bind, got %q", got)
	}
}
