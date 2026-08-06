package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestBuiltinAgentGuardManifestDigests(t *testing.T) {
	for _, rule := range BuiltinAgentBehaviorRuleManifest() {
		calculated, err := CalculateAgentBehaviorRuleDigest(rule)
		if err != nil {
			t.Fatalf("calculate %s digest: %v", rule.RuleKey, err)
		}
		if calculated != rule.Digest {
			t.Errorf("%s digest = %s, want %s", rule.RuleKey, rule.Digest, calculated)
		}
	}
	for _, profile := range BuiltinAgentGuardProfileManifest() {
		calculated, err := CalculateAgentGuardProfileDigest(profile)
		if err != nil {
			t.Fatalf("calculate %s digest: %v", profile.ProfileKey, err)
		}
		if calculated != profile.Digest {
			t.Errorf("%s digest = %s, want %s", profile.ProfileKey, profile.Digest, calculated)
		}
	}
	if err := VerifyBuiltinAgentGuardManifest(); err != nil {
		t.Fatalf("VerifyBuiltinAgentGuardManifest: %v", err)
	}
}

func TestP4BuiltinAgentProfilesAreStableAndRemoteFailsClosed(t *testing.T) {
	expected := map[string]struct {
		id         string
		agentType  string
		digest     string
		executable string
		configPath string
	}{
		AgentGuardProfileKeyClaudeCodeLinux: {
			id: AgentGuardProfileIDClaudeCodeLinux, agentType: "claude-code", executable: "claude",
			configPath: ".claude/settings.json", digest: "sha256:94eb603baadec817c6e03857064fbe809aa5da42d612d9dd7e8b486f66cb63a7",
		},
		AgentGuardProfileKeyOpenCodeLinux: {
			id: AgentGuardProfileIDOpenCodeLinux, agentType: "opencode", executable: "opencode",
			configPath: ".config/opencode/opencode.json", digest: "sha256:b0fff61d935a97de75d5e90c658248201117608d256cd9be3f9a30d1ee3a34c2",
		},
		AgentGuardProfileKeyGeminiCLILinux: {
			id: AgentGuardProfileIDGeminiCLILinux, agentType: "gemini-cli", executable: "gemini",
			configPath: ".gemini/settings.json", digest: "sha256:300f72f233925ac36203a8a6d6ad4d8aa3247b93cf03474e8cede761315f5f66",
		},
	}

	seen := make(map[string]bool, len(expected))
	for _, profile := range BuiltinAgentGuardProfileManifest() {
		want, ok := expected[profile.ProfileKey]
		if !ok {
			continue
		}
		seen[profile.ProfileKey] = true
		if profile.ID.String() != want.id || profile.ProfileVersion != 1 || profile.AgentType != want.agentType ||
			profile.Source != "builtin" || profile.SandboxFamily != "local_process_tree" ||
			profile.Digest != want.digest || !profile.Enabled {
			t.Errorf("unstable P4 profile identity for %s: %#v", profile.ProfileKey, profile)
		}
		controller := string(profile.ControllerMatch)
		if !strings.Contains(controller, `"exe_basenames":["`+want.executable+`"]`) ||
			!strings.Contains(controller, `"config_paths":["`+want.configPath+`"]`) {
			t.Errorf("profile %s has incomplete controller match: %s", profile.ProfileKey, controller)
		}
		remote := string(profile.IsolationExpectation)
		if !strings.Contains(remote, `"family":"remote_sandbox"`) ||
			!strings.Contains(remote, `"coverage_without_sensor":"remote_unobservable"`) {
			t.Errorf("profile %s does not fail closed for remote execution: %s", profile.ProfileKey, remote)
		}
		encoded, err := json.Marshal(profile)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"agent_official", "adapter_hook", "aegis_wrapper", "trusted_proof"} {
			if strings.Contains(string(encoded), forbidden) {
				t.Errorf("profile %s incorrectly infers tool trust from process recognition: %s", profile.ProfileKey, encoded)
			}
		}
	}
	if len(seen) != len(expected) {
		t.Fatalf("P4 profile coverage = %#v, want all %d profiles", seen, len(expected))
	}
}

func TestAgentGuardAgentSummaryCannotLeakEvidence(t *testing.T) {
	summaryType := reflect.TypeOf(AgentGuardAgentSummary{})
	forbiddenTokens := []string{
		"cmdline",
		"command",
		"path",
		"address",
		"destination",
		"baseline",
		"evidence",
		"analysis",
	}
	for i := 0; i < summaryType.NumField(); i++ {
		field := summaryType.Field(i)
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		normalized := strings.ToLower(field.Name + " " + jsonName)
		for _, forbidden := range forbiddenTokens {
			if strings.Contains(normalized, forbidden) {
				t.Errorf("outer Agent summary field %s may leak forbidden %q data", field.Name, forbidden)
			}
		}
	}

	encoded, err := json.Marshal(AgentGuardAgentSummary{})
	if err != nil {
		t.Fatalf("marshal AgentGuardAgentSummary: %v", err)
	}
	for _, forbidden := range forbiddenTokens {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Errorf("outer Agent summary JSON leaks forbidden key %q: %s", forbidden, encoded)
		}
	}
}

func TestAgentGuardModelsUseExpectedTables(t *testing.T) {
	tableNames := []string{
		(AgentGuardAdapterProfile{}).TableName(),
		(AgentBehaviorRuleDefinition{}).TableName(),
		(AgentGuardPolicy{}).TableName(),
		(AgentGuardPolicyDelivery{}).TableName(),
		(AgentRuntimeInstance{}).TableName(),
		(AgentExecutionUnit{}).TableName(),
		(AgentBehaviorSession{}).TableName(),
		(AgentBehaviorEvent{}).TableName(),
		(AgentSecurityFinding{}).TableName(),
		(AgentSecurityAnalysisRun{}).TableName(),
		(AgentGuardAction{}).TableName(),
	}
	expected := []string{
		"agent_guard_adapter_profiles",
		"agent_behavior_rule_definitions",
		"agent_guard_policies",
		"agent_guard_policy_deliveries",
		"agent_runtime_instances",
		"agent_execution_units",
		"agent_behavior_sessions",
		"agent_behavior_events",
		"agent_security_findings",
		"agent_security_analysis_runs",
		"agent_guard_actions",
	}
	if !reflect.DeepEqual(tableNames, expected) {
		t.Fatalf("Agent Guard table names = %#v, want %#v", tableNames, expected)
	}
}
