package sigma

import (
	"testing"
)

func TestExtractMitreID(t *testing.T) {
	tests := []struct {
		name     string
		tags     []string
		expected string
	}{
		{"lowercase t888", []string{"attack.t888"}, "T888"},
		{"uppercase T888", []string{"attack.T888"}, "T888"},
		{"sub-technique lowercase", []string{"attack.t1059.004"}, "T1059.004"},
		{"sub-technique uppercase", []string{"attack.T1059.004"}, "T1059.004"},
		{"mixed case", []string{"attack.T888"}, "T888"},
		{"no mitre tag", []string{"host-based", "linux"}, ""},
		{"empty tags", []string{}, ""},
		{"mitre among other tags", []string{"host-based", "attack.t1053.003", "linux"}, "T1053.003"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMitreID(tt.tags)
			if result != tt.expected {
				t.Errorf("extractMitreID(%v) = %q, want %q", tt.tags, result, tt.expected)
			}
		})
	}
}

func TestCompileRuleMitreIDNormalization(t *testing.T) {
	rule := &Rule{
		ID:     "reverse-shell-detect",
		Tags:   []string{"attack.t888"},
		Level:  "high",
		Logsource: Logsource{Category: "process_creation"},
		Detection: Detection{
			Selections: map[string]interface{}{
				"selection": map[string]interface{}{
					"commandline": "bash -i",
				},
			},
			Condition: "selection",
		},
	}

	compiled := CompileRule(rule)
	if compiled.MitreID != "T888" {
		t.Errorf("MitreID = %q, want %q", compiled.MitreID, "T888")
	}
	if compiled.Severity != "high" {
		t.Errorf("Severity = %q, want %q", compiled.Severity, "high")
	}
}

func TestCompiledRuleMatchRequiresAllConditionSelectors(t *testing.T) {
	rule := &Rule{
		ID: "rule-and-condition",
		Logsource: Logsource{
			Category: "process_creation",
		},
		Detection: Detection{
			Selections: map[string]interface{}{
				"selection_image": map[string]interface{}{
					"image": "bash",
				},
				"selection_command": map[string]interface{}{
					"commandline": "-c",
				},
			},
			Condition: "selection_image and selection_command",
		},
	}

	compiled := CompileRule(rule)

	if compiled.Match(map[string]interface{}{"category": "process_creation", "image": "/bin/bash"}) {
		t.Fatalf("expected event matching only one selector to be rejected")
	}
	if !compiled.Match(map[string]interface{}{"category": "process_creation", "image": "/bin/bash", "commandline": "bash -c id"}) {
		t.Fatalf("expected event matching both selectors to be accepted")
	}
}

func TestCompiledRuleMatchSupportsFilterExclusion(t *testing.T) {
	rule := &Rule{
		ID: "rule-filter-condition",
		Logsource: Logsource{
			Category: "process_creation",
		},
		Detection: Detection{
			Selections: map[string]interface{}{
				"selection": map[string]interface{}{
					"commandline": "curl",
				},
				"filter_known_healthcheck": map[string]interface{}{
					"commandline": "localhost/health",
				},
			},
			Condition: "selection and not 1 of filter_*",
		},
	}

	compiled := CompileRule(rule)

	if compiled.Match(map[string]interface{}{"category": "process_creation", "commandline": "curl localhost/health"}) {
		t.Fatalf("expected event matching a filter selector to be rejected")
	}
	if !compiled.Match(map[string]interface{}{"category": "process_creation", "commandline": "curl http://example.invalid/payload.sh"}) {
		t.Fatalf("expected event matching selection without filter to be accepted")
	}
}

func TestBlockStrategyHostTestRuleMatchesOnlyDedicatedCommand(t *testing.T) {
	loader := NewLoader("testdata")
	if err := loader.LoadFromDisk(); err != nil {
		t.Fatalf("failed to load test rules: %v", err)
	}

	matches := loader.MatchAll(map[string]interface{}{
		"category":    "process_creation",
		"commandline": "/bin/sh -c 'echo aegis-block-strategy-test'",
	})
	if len(matches) != 1 {
		t.Fatalf("expected one dedicated block strategy rule match, got %d", len(matches))
	}
	if matches[0].ID != "aegis-block-strategy-host-test" {
		t.Fatalf("expected dedicated rule id, got %s", matches[0].ID)
	}

	if matches := loader.MatchAll(map[string]interface{}{
		"category":    "process_creation",
		"commandline": "/bin/sh -c 'echo normal-command'",
	}); len(matches) != 0 {
		t.Fatalf("expected normal command not to match, got %d matches", len(matches))
	}
}
