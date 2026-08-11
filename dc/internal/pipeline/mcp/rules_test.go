package mcp

import "testing"

func TestAnalyzeKeepsSecretEvidenceRedacted(t *testing.T) {
	verdict := Analyze(InvocationEvent{ToolAlias: "search", RuleHints: []string{"secret_marker"}, Status: "succeeded"})
	if verdict.DeterministicSeverity != "high" || verdict.OverallRisk != "high" {
		t.Fatalf("expected high deterministic floor: %#v", verdict)
	}
	if len(verdict.Hits) != 1 || verdict.Hits[0].Evidence != "redacted" {
		t.Fatalf("secret evidence leaked or missing: %#v", verdict.Hits)
	}
}

func TestAnalyzeTreatsPromptInjectionAsUntrusted(t *testing.T) {
	verdict := Analyze(InvocationEvent{ToolAlias: "read", RuleHints: []string{"prompt_injection_marker"}})
	if verdict.OverallRisk != "high" || len(verdict.Hits) != 1 {
		t.Fatalf("expected injection finding: %#v", verdict)
	}
}

func TestAnalyzePolicyDenialHasAuditVerdict(t *testing.T) {
	verdict := Analyze(InvocationEvent{ToolAlias: "write", PolicyDecision: "deny", Status: "denied"})
	if verdict.DeterministicSeverity != "medium" || len(verdict.Hits) != 1 {
		t.Fatalf("expected medium denial finding: %#v", verdict)
	}
}
