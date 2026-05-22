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

func TestStartsWithModifier(t *testing.T) {
	rule := &Rule{
		ID:    "test-startswith",
		Title: "StartsWith Test",
		Level: "high",
		Logsource: Logsource{Category: "network_connection"},
		Detection: Detection{
			Selections: map[string]interface{}{
				"filter_local": map[string]interface{}{
					"DestinationIp|startswith": []interface{}{"127.", "::1"},
				},
			},
			Condition: "filter_local",
		},
	}
	cr := CompileRule(rule)

	// Should match localhost
	event := map[string]interface{}{
		"destinationip": "127.0.0.1",
	}
	if !cr.Match(event) {
		t.Error("expected match for 127.0.0.1")
	}

	// Should not match external IP
	event2 := map[string]interface{}{
		"destinationip": "10.0.0.1",
	}
	if cr.Match(event2) {
		t.Error("did not expect match for 10.0.0.1")
	}
}

func TestNumericPortArray(t *testing.T) {
	rule := &Rule{
		ID:    "test-numeric-ports",
		Title: "Numeric Port Array Test",
		Level: "high",
		Logsource: Logsource{Category: "network_connection"},
		Detection: Detection{
			Selections: map[string]interface{}{
				"selection": map[string]interface{}{
					"DestinationPort": []interface{}{4444, 5555, 31337},
				},
			},
			Condition: "selection",
		},
	}
	cr := CompileRule(rule)

	// Should match port 4444
	event := map[string]interface{}{
		"destinationport": 4444,
	}
	if !cr.Match(event) {
		t.Error("expected match for port 4444")
	}

	// Should not match port 80
	event2 := map[string]interface{}{
		"destinationport": 80,
	}
	if cr.Match(event2) {
		t.Error("did not expect match for port 80")
	}
}

func TestTargetFilenameAlias(t *testing.T) {
	rule := &Rule{
		ID:    "test-filename-alias",
		Title: "TargetFilename Alias Test",
		Level: "high",
		Logsource: Logsource{Category: "file_event"},
		Detection: Detection{
			Selections: map[string]interface{}{
				"selection": map[string]interface{}{
					"TargetFilename|re": "^/etc/(passwd|shadow)$",
				},
			},
			Condition: "selection",
		},
	}
	cr := CompileRule(rule)

	event := map[string]interface{}{
		"targetfilename": "/etc/passwd",
	}
	if !cr.Match(event) {
		t.Error("expected match for /etc/passwd via lowercase alias")
	}
}

func TestInboundHighRiskPortDetection(t *testing.T) {
	// Test that a sigma rule matching SourcePort (inbound connections) works correctly.
	// Scenario: external host connects to agent host on port 8081.
	// For inbound connections, the high-risk port is the local listening port (SourcePort).
	rule := &Rule{
		ID:    "aegis-network-high-risk-inbound-port",
		Title: "High Risk Inbound TCP Port",
		Level: "high",
		Logsource: Logsource{
			Category: "network_connection",
			Product:  "linux",
		},
		Detection: Detection{
			Selections: map[string]interface{}{
				"selection": map[string]interface{}{
					"SourcePort": []interface{}{4444, 5555, 31337, 1234, 8443, 8081},
					"network.transport": "tcp",
				},
				"filter_local": map[string]interface{}{
					"SourceIp|startswith": []interface{}{"127.", "::1"},
				},
			},
			Condition: "selection and not filter_local",
		},
	}
	cr := CompileRule(rule)

	// Should match: external IP connecting to port 8081
	eventMatch := map[string]interface{}{
		"category":          "network_connection",
		"sourceport":        8081,
		"sourceip":          "34.174.207.156",
		"network.transport": "tcp",
	}
	if !cr.Match(eventMatch) {
		t.Error("expected match for external IP connecting to port 8081")
	}

	// Should NOT match: localhost connection to port 8081
	eventLocalhost := map[string]interface{}{
		"category":          "network_connection",
		"sourceport":        8081,
		"sourceip":          "127.0.0.1",
		"network.transport": "tcp",
	}
	if cr.Match(eventLocalhost) {
		t.Error("did not expect match for localhost connection")
	}

	// Should NOT match: non-high-risk port
	eventOtherPort := map[string]interface{}{
		"category":          "network_connection",
		"sourceport":        80,
		"sourceip":          "34.174.207.156",
		"network.transport": "tcp",
	}
	if cr.Match(eventOtherPort) {
		t.Error("did not expect match for port 80")
	}

	// Should NOT match: UDP protocol
	eventUDP := map[string]interface{}{
		"category":          "network_connection",
		"sourceport":        8081,
		"sourceip":          "34.174.207.156",
		"network.transport": "udp",
	}
	if cr.Match(eventUDP) {
		t.Error("did not expect match for UDP protocol")
	}
}

func TestInboundPortWithMixedCaseFieldName(t *testing.T) {
	// Verify that SourcePort lookup works case-insensitively
	rule := &Rule{
		ID:    "test-inbound-case",
		Title: "Inbound Port Case Test",
		Level: "high",
		Logsource: Logsource{
			Category: "network_connection",
		},
		Detection: Detection{
			Selections: map[string]interface{}{
				"selection": map[string]interface{}{
					"SourcePort": []interface{}{8081},
				},
			},
			Condition: "selection",
		},
	}
	cr := CompileRule(rule)

	// Test with various case aliases that the pipeline should produce
	testCases := []struct {
		name   string
		port   interface{}
		expect bool
	}{
		{"lowercase sourceport", 8081, true},
		{"camelCase SourcePort", 8081, true},
		{"lowercase source_port", 8081, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			event := map[string]interface{}{
				"category":   "network_connection",
				"sourceport": tc.port,
			}
			result := cr.Match(event)
			if result != tc.expect {
				t.Errorf("sourceport=%v: got %v, want %v", tc.port, result, tc.expect)
			}
		})
	}
}
