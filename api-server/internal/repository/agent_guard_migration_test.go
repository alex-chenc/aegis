package repository

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"api-server/internal/model"
)

const (
	ruleManifestDelimiter    = "$agent_guard_rules$"
	profileManifestDelimiter = "$agent_guard_profiles$"
)

func TestAgentGuardMigrationDefinesCompleteSchema(t *testing.T) {
	content := readAgentGuardMigration(t)

	requiredTables := []string{
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
	for _, table := range requiredTables {
		fragment := "CREATE TABLE IF NOT EXISTS " + table
		if !strings.Contains(content, fragment) {
			t.Errorf("migration missing %q", fragment)
		}
	}
	if count := strings.Count(content, "CREATE TABLE IF NOT EXISTS "); count != len(requiredTables) {
		t.Fatalf("Agent Guard migration table count = %d, want exactly %d", count, len(requiredTables))
	}

	requiredFragments := []string{
		"UNIQUE (profile_key, profile_version)",
		"chk_agent_guard_profile_digest",
		"UNIQUE (rule_key, rule_version)",
		"chk_agent_behavior_rule_digest",
		"UNIQUE (policy_key, version)",
		"UNIQUE (host_id, bundle_version)",
		"UNIQUE (host_id, controller_pid, controller_start_ticks)",
		"UNIQUE (host_id, fingerprint)",
		"raw_event_id",
		"finding_key",
		"UNIQUE (finding_id, attempt)",
		"command_id",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_agent_behavior_events_host_sequence",
		"CREATE INDEX IF NOT EXISTS idx_agent_behavior_events_instance_time",
		"CREATE INDEX IF NOT EXISTS idx_agent_behavior_events_session_time",
		"CREATE INDEX IF NOT EXISTS idx_agent_behavior_events_unit_time",
		"CREATE INDEX IF NOT EXISTS idx_agent_behavior_events_category_time",
		"CREATE INDEX IF NOT EXISTS idx_agent_security_findings_status",
		"CREATE INDEX IF NOT EXISTS idx_agent_guard_actions_status",
		"chk_agent_behavior_session_token_hash",
		"correlation_token_hash ~ '^sha256:[0-9a-f]{64}$'",
		"fk_agent_security_findings_latest_analysis",
		"IF NOT EXISTS",
		"'freeze_execution_unit'",
		"'resume_execution_unit'",
		"'pending'",
		"'running'",
		"'succeeded'",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(content, fragment) {
			t.Errorf("migration missing required schema fragment %q", fragment)
		}
	}

	if strings.Contains(strings.ToUpper(content), "DROP TABLE") {
		t.Fatal("migration must preserve Agent Guard audit tables during rollback/re-execution")
	}
}

func TestAgentGuardMigrationSeedsMatchBuiltinManifest(t *testing.T) {
	content := readAgentGuardMigration(t)

	var sqlRules []model.AgentBehaviorRuleDefinition
	decodeMigrationManifest(t, content, ruleManifestDelimiter, &sqlRules)
	codeRules := model.BuiltinAgentBehaviorRuleManifest()
	if len(sqlRules) != len(codeRules) {
		t.Fatalf("SQL rule seed count = %d, code manifest count = %d", len(sqlRules), len(codeRules))
	}
	assertRuleManifestMatches(t, sqlRules, codeRules)

	var sqlProfiles []model.AgentGuardAdapterProfile
	decodeMigrationManifest(t, content, profileManifestDelimiter, &sqlProfiles)
	codeProfiles := model.BuiltinAgentGuardProfileManifest()
	baseProfiles := make([]model.AgentGuardAdapterProfile, 0, len(codeProfiles))
	for _, profile := range codeProfiles {
		if profile.ProfileKey != model.AgentGuardProfileKeyZcodeLinux {
			baseProfiles = append(baseProfiles, profile)
		}
	}
	if len(sqlProfiles) != len(baseProfiles) {
		t.Fatalf("SQL profile seed count = %d, base code manifest count = %d", len(sqlProfiles), len(baseProfiles))
	}
	assertProfileManifestMatches(t, sqlProfiles, baseProfiles)
	followUp := readZcodeProfileMigration(t)
	if !strings.Contains(followUp, `'zcode-linux'`) ||
		!strings.Contains(followUp, `sha256:bcb65be77f138f3f0f5d6de4ac2d017b43876f9cd98a0d0a7c55bd0f8dd5389c`) {
		t.Fatal("Zcode follow-up migration does not contain the immutable profile seed")
	}

	if err := model.VerifyBuiltinAgentGuardManifest(); err != nil {
		t.Fatalf("built-in Agent Guard manifest is invalid: %v", err)
	}
}

func TestAgentGuardMigrationIsTextuallyIdempotent(t *testing.T) {
	content := readAgentGuardMigration(t)

	if strings.Count(content, "ON CONFLICT (rule_key, rule_version) DO NOTHING") != 1 {
		t.Error("rule seed must use one stable idempotent conflict strategy")
	}
	if strings.Count(content, "ON CONFLICT (profile_key, profile_version) DO NOTHING") != 1 {
		t.Error("profile seed must be idempotent without overwriting immutable definitions or digests")
	}
	profileSeed := content[strings.Index(content, "INSERT INTO agent_guard_adapter_profiles"):]
	if strings.Contains(profileSeed, "DO UPDATE") || strings.Contains(profileSeed, "digest = EXCLUDED.digest") {
		t.Error("profile seed must never silently synchronize over an existing immutable profile")
	}
	if !strings.Contains(content, "duplicate_object") {
		t.Error("latest_analysis foreign key creation must tolerate repeated execution")
	}
}

func readAgentGuardMigration(t *testing.T) string {
	t.Helper()
	repositoryDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get repository directory: %v", err)
	}
	path := filepath.Clean(filepath.Join(repositoryDir, "..", "..", "..", "migrations", "029_v6.2_agent_guard.sql"))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Agent Guard migration: %v", err)
	}
	return string(content)
}

func readZcodeProfileMigration(t *testing.T) string {
	t.Helper()
	repositoryDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get repository directory: %v", err)
	}
	path := filepath.Clean(filepath.Join(repositoryDir, "..", "..", "..", "migrations", "030_v6.2_zcode_agent_guard_profile.sql"))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Zcode profile migration: %v", err)
	}
	return string(content)
}

func decodeMigrationManifest(t *testing.T, content, delimiter string, destination any) {
	t.Helper()
	first := strings.Index(content, delimiter)
	if first < 0 {
		t.Fatalf("migration missing manifest delimiter %q", delimiter)
	}
	start := first + len(delimiter)
	endOffset := strings.Index(content[start:], delimiter)
	if endOffset < 0 {
		t.Fatalf("migration missing closing manifest delimiter %q", delimiter)
	}
	payload := strings.TrimSpace(content[start : start+endOffset])
	if err := json.Unmarshal([]byte(payload), destination); err != nil {
		t.Fatalf("decode %s manifest: %v", delimiter, err)
	}
}

func assertRuleManifestMatches(t *testing.T, sqlRules, codeRules []model.AgentBehaviorRuleDefinition) {
	t.Helper()
	codeByKey := make(map[string]model.AgentBehaviorRuleDefinition, len(codeRules))
	for _, rule := range codeRules {
		codeByKey[rule.RuleKey] = rule
	}
	for _, sqlRule := range sqlRules {
		codeRule, ok := codeByKey[sqlRule.RuleKey]
		if !ok {
			t.Errorf("SQL contains unknown built-in rule %q", sqlRule.RuleKey)
			continue
		}
		if sqlRule.ID != codeRule.ID || sqlRule.RuleVersion != codeRule.RuleVersion || sqlRule.Digest != codeRule.Digest {
			t.Errorf("SQL/code identity mismatch for %s: SQL id/version/digest=%s/%d/%s code=%s/%d/%s",
				sqlRule.RuleKey, sqlRule.ID, sqlRule.RuleVersion, sqlRule.Digest,
				codeRule.ID, codeRule.RuleVersion, codeRule.Digest)
		}
		digest, err := model.CalculateAgentBehaviorRuleDigest(sqlRule)
		if err != nil {
			t.Errorf("calculate SQL rule digest for %s: %v", sqlRule.RuleKey, err)
			continue
		}
		if digest != sqlRule.Digest {
			t.Errorf("SQL rule %s digest = %s, calculated %s", sqlRule.RuleKey, sqlRule.Digest, digest)
		}
	}
}

func assertProfileManifestMatches(t *testing.T, sqlProfiles, codeProfiles []model.AgentGuardAdapterProfile) {
	t.Helper()
	codeByKey := make(map[string]model.AgentGuardAdapterProfile, len(codeProfiles))
	for _, profile := range codeProfiles {
		codeByKey[profile.ProfileKey] = profile
	}
	for _, sqlProfile := range sqlProfiles {
		codeProfile, ok := codeByKey[sqlProfile.ProfileKey]
		if !ok {
			t.Errorf("SQL contains unknown built-in profile %q", sqlProfile.ProfileKey)
			continue
		}
		if sqlProfile.ID != codeProfile.ID || sqlProfile.ProfileVersion != codeProfile.ProfileVersion || sqlProfile.Digest != codeProfile.Digest {
			t.Errorf("SQL/code identity mismatch for %s: SQL id/version/digest=%s/%d/%s code=%s/%d/%s",
				sqlProfile.ProfileKey, sqlProfile.ID, sqlProfile.ProfileVersion, sqlProfile.Digest,
				codeProfile.ID, codeProfile.ProfileVersion, codeProfile.Digest)
		}
		digest, err := model.CalculateAgentGuardProfileDigest(sqlProfile)
		if err != nil {
			t.Errorf("calculate SQL profile digest for %s: %v", sqlProfile.ProfileKey, err)
			continue
		}
		if digest != sqlProfile.Digest {
			t.Errorf("SQL profile %s digest = %s, calculated %s", sqlProfile.ProfileKey, sqlProfile.Digest, digest)
		}
	}
}
