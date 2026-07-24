package assistant

import "testing"

func TestWorkflowRegistryMatchesBaselineWithoutReturningEveryModule(t *testing.T) {
	matches := NewWorkflowRegistry().Match(IntentResult{
		Domains:     []string{"baseline"},
		Operations:  []string{"execute"},
		ObjectTypes: []string{"baseline_template", "host"},
	})
	if len(matches) == 0 || len(matches) >= len(builtinWorkflowSpecs()) {
		t.Fatalf("workflow matches were not scoped: %#v", matches)
	}
	found := false
	for _, match := range matches {
		if match.ID == "baseline_compliance" {
			found = true
		}
	}
	if !found {
		t.Fatalf("baseline workflow missing: %#v", matches)
	}
}

func TestIntentBreakdownRejectsInventedWorkflowID(t *testing.T) {
	breakdown := &IntentBreakdown{
		Goal:          "Run a baseline check.",
		Scope:         IntentScope{Kind: "all"},
		RiskHint:      "high",
		WorkflowIDs:   []string{"invented_workflow"},
		Confidence:    0.9,
		Objects:       []IntentObject{},
		MissingInfo:   []MissingInfo{},
		Parameters:    IntentParameters{},
		RequiresWrite: true,
	}
	if err := validateIntentBreakdownAgainstCatalog(breakdown, nil, []WorkflowSpec{{ID: "baseline_compliance"}}); err == nil {
		t.Fatal("expected an invented workflow ID to be rejected")
	}
}
