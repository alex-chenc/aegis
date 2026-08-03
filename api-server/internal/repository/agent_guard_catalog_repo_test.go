package repository

import (
	"context"
	"errors"
	"testing"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAgentGuardCatalogRepositoryListsAndVerifiesBuiltinManifest(t *testing.T) {
	db := setupAgentGuardCatalogTestDB(t)
	ctx := context.Background()
	repo := NewAgentGuardCatalogRepository(db)

	for _, profile := range model.BuiltinAgentGuardProfileManifest() {
		if err := db.Create(&profile).Error; err != nil {
			t.Fatalf("seed profile %s: %v", profile.ProfileKey, err)
		}
	}
	for _, rule := range model.BuiltinAgentBehaviorRuleManifest() {
		if err := db.Create(&rule).Error; err != nil {
			t.Fatalf("seed rule %s: %v", rule.RuleKey, err)
		}
	}

	profiles, total, err := repo.ListProfiles(ctx, model.AgentGuardProfileQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 2},
		AgentType:           "codex",
	})
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if total != 1 || len(profiles) != 1 || profiles[0].ProfileKey != model.AgentGuardProfileKeyCodexLinux {
		t.Fatalf("unexpected codex profiles: total=%d profiles=%#v", total, profiles)
	}
	profiles, total, err = repo.ListProfiles(ctx, model.AgentGuardProfileQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 20},
		AgentType:           "claude-code",
		Source:              "builtin",
	})
	if err != nil {
		t.Fatalf("ListProfiles P4 catalog: %v", err)
	}
	if total != 1 || len(profiles) != 1 || profiles[0].ProfileKey != model.AgentGuardProfileKeyClaudeCodeLinux ||
		profiles[0].Digest != "sha256:e4158634ff61db23c9fa930507e5d91bb79840e94508e7ec9d4d5cd76f0e01e1" {
		t.Fatalf("unexpected Claude Code catalog result: total=%d profiles=%#v", total, profiles)
	}

	rules, total, err := repo.ListRules(ctx, model.AgentBehaviorRuleQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 3},
		Source:              "builtin",
	})
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if total != 5 || len(rules) != 3 {
		t.Fatalf("unexpected rule page: total=%d len=%d", total, len(rules))
	}

	rule, err := repo.GetRule(ctx, model.AgentGuardRuleKeyFileCreation, 1)
	if err != nil {
		t.Fatalf("GetRule: %v", err)
	}
	if rule.Name != "文件生成" {
		t.Fatalf("rule name = %q, want 文件生成", rule.Name)
	}

	if err := repo.VerifyBuiltinManifest(ctx); err != nil {
		t.Fatalf("VerifyBuiltinManifest: %v", err)
	}

	if err := db.Model(&model.AgentGuardAdapterProfile{}).
		Where("profile_key = ?", model.AgentGuardProfileKeyClaudeCodeLinux).
		Update("digest", "sha256:tampered").Error; err != nil {
		t.Fatalf("tamper profile digest: %v", err)
	}
	err = repo.VerifyBuiltinManifest(ctx)
	if !errors.Is(err, ErrAgentGuardBuiltinDigestMismatch) {
		t.Fatalf("VerifyBuiltinManifest profile error = %v, want ErrAgentGuardBuiltinDigestMismatch", err)
	}
	if err := db.Model(&model.AgentGuardAdapterProfile{}).
		Where("profile_key = ?", model.AgentGuardProfileKeyClaudeCodeLinux).
		Update("digest", model.BuiltinAgentGuardProfileManifest()[3].Digest).Error; err != nil {
		t.Fatalf("restore profile digest: %v", err)
	}

	if err := db.Model(&model.AgentBehaviorRuleDefinition{}).
		Where("rule_key = ?", model.AgentGuardRuleKeySensitiveCommand).
		Update("digest", "sha256:tampered").Error; err != nil {
		t.Fatalf("tamper rule digest: %v", err)
	}
	err = repo.VerifyBuiltinManifest(ctx)
	if !errors.Is(err, ErrAgentGuardBuiltinDigestMismatch) {
		t.Fatalf("VerifyBuiltinManifest error = %v, want ErrAgentGuardBuiltinDigestMismatch", err)
	}
}

func TestAgentGuardCatalogRepositoryNotFoundErrors(t *testing.T) {
	db := setupAgentGuardCatalogTestDB(t)
	repo := NewAgentGuardCatalogRepository(db)
	ctx := context.Background()

	if _, err := repo.GetProfile(ctx, uuid.New()); !errors.Is(err, ErrAgentGuardProfileNotFound) {
		t.Fatalf("GetProfile error = %v, want ErrAgentGuardProfileNotFound", err)
	}
	if _, err := repo.GetRule(ctx, "AGB-MISSING", 0); !errors.Is(err, ErrAgentGuardRuleNotFound) {
		t.Fatalf("GetRule error = %v, want ErrAgentGuardRuleNotFound", err)
	}
}

func setupAgentGuardCatalogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	statements := []string{
		`CREATE TABLE agent_guard_adapter_profiles (
			id TEXT PRIMARY KEY,
			profile_key TEXT NOT NULL,
			profile_version INTEGER NOT NULL,
			agent_type TEXT NOT NULL,
			display_name TEXT NOT NULL,
			source TEXT NOT NULL,
			sandbox_family TEXT NOT NULL,
			controller_match TEXT NOT NULL,
			worker_match TEXT NOT NULL,
			backend_detectors TEXT NOT NULL,
			isolation_expectation TEXT NOT NULL,
			default_escape_rules TEXT NOT NULL,
			digest TEXT NOT NULL,
			enabled BOOLEAN NOT NULL,
			created_by TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			UNIQUE(profile_key, profile_version)
		)`,
		`CREATE TABLE agent_behavior_rule_definitions (
			id TEXT PRIMARY KEY,
			rule_key TEXT NOT NULL,
			rule_version INTEGER NOT NULL,
			name TEXT NOT NULL,
			description TEXT,
			source TEXT NOT NULL,
			engine TEXT NOT NULL,
			categories TEXT NOT NULL,
			default_enabled BOOLEAN NOT NULL,
			default_severity TEXT NOT NULL,
			default_action TEXT NOT NULL,
			recommended_action TEXT NOT NULL,
			parameters_schema TEXT NOT NULL,
			default_parameters TEXT NOT NULL,
			required_evidence TEXT NOT NULL,
			allow_conditions TEXT NOT NULL,
			mitre TEXT NOT NULL,
			immutable BOOLEAN NOT NULL,
			digest TEXT NOT NULL,
			created_at DATETIME,
			updated_at DATETIME,
			UNIQUE(rule_key, rule_version)
		)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create catalog test schema: %v", err)
		}
	}
	return db
}
