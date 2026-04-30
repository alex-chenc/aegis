package service

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRuleGenerationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	if logger.Logger == nil {
		if err := logger.Init(&logger.Config{Level: "error", MaxSize: 10, MaxBackups: 1, MaxAge: 1, Compress: false}); err != nil {
			t.Fatalf("failed to init logger: %v", err)
		}
	}

	dsn := fmt.Sprintf("file:%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	statements := []string{
		`CREATE TABLE ai_rule_config (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			enabled BOOLEAN NOT NULL DEFAULT 0,
			mode TEXT NOT NULL DEFAULT 'suggest',
			thresholds TEXT NOT NULL,
			conservatism REAL NOT NULL DEFAULT 0.5,
			require_approval BOOLEAN NOT NULL DEFAULT 1,
			auto_activate_after_approval BOOLEAN NOT NULL DEFAULT 0,
			activation_delay_hours INTEGER NOT NULL DEFAULT 24,
			notify_on_generation BOOLEAN NOT NULL DEFAULT 1,
			notify_on_approval BOOLEAN NOT NULL DEFAULT 1,
			notification_targets TEXT NOT NULL DEFAULT '[]',
			rules_generated_count INTEGER NOT NULL DEFAULT 0,
			rules_approved_count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME,
			created_by TEXT,
			updated_by TEXT
		)`,
		`CREATE TABLE sigma_rules (
			id TEXT PRIMARY KEY,
			rule_id TEXT NOT NULL UNIQUE,
			title TEXT,
			description TEXT,
			content TEXT NOT NULL,
			status TEXT NOT NULL,
			mitre_id TEXT,
			severity TEXT,
			generated_by TEXT NOT NULL,
			version TEXT NOT NULL,
			created_at DATETIME,
			activated_at DATETIME,
			updated_at DATETIME,
			source TEXT,
			file_name TEXT,
			file_hash TEXT,
			file_size INTEGER,
			parsed_at DATETIME,
			parse_error TEXT,
			ai_generated BOOLEAN,
			parent_rule_id TEXT,
			generation_prompt TEXT,
			generation_context TEXT,
			approved_by TEXT,
			approved_at DATETIME,
			dispatch_hosts TEXT,
			dispatch_status TEXT
		)`,
		`CREATE TABLE alerts (
			id TEXT PRIMARY KEY,
			alert_id TEXT NOT NULL UNIQUE,
			host_id TEXT NOT NULL,
			pid INTEGER NOT NULL,
			ppid INTEGER,
			command_line TEXT,
			process_tree TEXT,
			mitre_id TEXT NOT NULL,
			mitre_name TEXT,
			severity TEXT NOT NULL,
			description TEXT,
			llm_summary TEXT,
			dedupe_key TEXT NOT NULL,
			hit_count INTEGER,
			auto_blocked BOOLEAN,
			manual_blocked BOOLEAN,
			status TEXT NOT NULL,
			judgment_source TEXT,
			block_status TEXT,
			block_message TEXT,
			auto_dispose BOOLEAN,
			llm_disposal_strategy TEXT,
			rule_id TEXT,
			rule_title TEXT,
			first_seen_at DATETIME,
			last_seen_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("failed to create test schema: %v", err)
		}
	}

	return db
}

func seedAIConfig(t *testing.T, db *gorm.DB, enabled bool, count int, hours int) {
	t.Helper()

	config := &model.AIConfig{
		ID:                  uuid.New(),
		Name:                "default",
		Description:         "test config",
		Enabled:             enabled,
		Mode:                "suggest",
		Thresholds:          `{"high_frequency_count":` + strconv.Itoa(count) + `,"high_frequency_hours":` + strconv.Itoa(hours) + `}`,
		Conservatism:        0.1,
		RequireApproval:     true,
		NotificationTargets: `[]`,
	}
	if err := db.Create(config).Error; err != nil {
		t.Fatalf("failed to seed ai config: %v", err)
	}
}

func seedRuleAndAlerts(t *testing.T, db *gorm.DB, ruleID string, mitreID string, createdAt time.Time, count int) {
	t.Helper()

	rule := &model.SigmaRule{
		ID:          uuid.New(),
		RuleID:      ruleID,
		Title:       "Broad shell rule",
		Content:     "title: Broad shell rule\ndetection:\n  condition: selection\n",
		Status:      "active",
		MitreID:     mitreID,
		Severity:    "high",
		GeneratedBy: "manual",
		Version:     "1.0",
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}
	if err := db.Create(rule).Error; err != nil {
		t.Fatalf("failed to seed sigma rule: %v", err)
	}

	hostID := uuid.New()
	for i := 0; i < count; i++ {
		alert := &model.Alert{
			ID:          uuid.New(),
			AlertID:     ruleID + "-alert-" + strconv.Itoa(i),
			HostID:      hostID,
			PID:         100 + i,
			MitreID:     mitreID,
			Severity:    "high",
			DedupeKey:   ruleID + "-dedupe-" + strconv.Itoa(i),
			Status:      "pending",
			RuleID:      ruleID,
			CreatedAt:   createdAt,
			FirstSeenAt: createdAt,
			LastSeenAt:  createdAt,
			UpdatedAt:   createdAt,
		}
		if err := db.Create(alert).Error; err != nil {
			t.Fatalf("failed to seed alert: %v", err)
		}
	}
}

func newRuleGenerationTestService(db *gorm.DB) *RuleGenerationService {
	configRepo := repository.NewAIRuleConfigRepository(db)
	return &RuleGenerationService{
		configService: NewAIRuleConfigService(configRepo),
		sigmaRuleRepo: repository.NewSigmaRuleRepository(db),
		alertRepo:     repository.NewAlertRepository(db),
		sampleSize:    10,
		stopCh:        make(chan struct{}),
	}
}

func TestRuleGenerationCollectConfiguredTriggerStatsUsesConfiguredHours(t *testing.T) {
	db := setupRuleGenerationTestDB(t)
	now := time.Now()
	createdAt := now.Add(-2 * time.Hour)
	seedAIConfig(t, db, true, 2, 3)
	seedRuleAndAlerts(t, db, "rule-configured-hours", "T1059.004", createdAt, 2)

	service := newRuleGenerationTestService(db)
	stats, err := service.collectConfiguredTriggerStats(now)
	if err != nil {
		t.Fatalf("expected configured trigger stats, got error: %v", err)
	}

	if len(stats) != 1 {
		t.Fatalf("expected one rule over threshold in configured 3h window, got %d", len(stats))
	}
	if stats[0].RuleID != "rule-configured-hours" {
		t.Fatalf("expected rule-configured-hours, got %s", stats[0].RuleID)
	}
	if stats[0].AlertCount != 2 {
		t.Fatalf("expected alert count 2, got %d", stats[0].AlertCount)
	}
	if stats[0].TimeWindow != "3h" {
		t.Fatalf("expected time window 3h, got %s", stats[0].TimeWindow)
	}
}

func TestRuleGenerationCollectConfiguredTriggerStatsSkipsDisabledConfig(t *testing.T) {
	db := setupRuleGenerationTestDB(t)
	now := time.Now()
	seedAIConfig(t, db, false, 1, 24)
	seedRuleAndAlerts(t, db, "rule-disabled", "T1059.004", now.Add(-time.Hour), 1)

	service := newRuleGenerationTestService(db)
	stats, err := service.collectConfiguredTriggerStats(now)
	if err != nil {
		t.Fatalf("expected disabled config to skip without error, got %v", err)
	}
	if len(stats) != 0 {
		t.Fatalf("expected no stats while config disabled, got %d", len(stats))
	}
}

func TestRuleGenerationApplyRuleAdjustmentPersistsTightenedExperimentalRule(t *testing.T) {
	db := setupRuleGenerationTestDB(t)
	now := time.Now()
	seedAIConfig(t, db, true, 1, 1)
	seedRuleAndAlerts(t, db, "rule-tighten", "T1059.004", now, 1)

	service := newRuleGenerationTestService(db)
	rule, err := service.sigmaRuleRepo.FindByRuleID("rule-tighten")
	if err != nil {
		t.Fatalf("failed to find seeded rule: %v", err)
	}
	rule.Content = "title: Broad shell rule\ndetection:\n  selection:\n    CommandLine|contains: bash\n  selection_process:\n    Image|endswith: /bash\n  filter_known_admin:\n    CommandLine|contains: uptime\n  condition: selection\n"
	if err := service.sigmaRuleRepo.Update(rule); err != nil {
		t.Fatalf("failed to update seeded rule content: %v", err)
	}

	err = service.applyRuleAdjustment(rule, RuleAdjustment{
		RuleID:          "rule-tighten",
		Action:          "tighten",
		AddConditions:   []string{"selection_process"},
		ExcludePatterns: []string{"filter_known_admin"},
	}, repository.RuleTriggerStats{RuleID: "rule-tighten", MitreID: "T1059.004", AlertCount: 1, TimeWindow: "1h"})
	if err != nil {
		t.Fatalf("expected rule adjustment to persist, got error: %v", err)
	}

	updated, err := service.sigmaRuleRepo.FindByRuleID("rule-tighten")
	if err != nil {
		t.Fatalf("failed to reload adjusted rule: %v", err)
	}
	if updated.Status != "experimental" {
		t.Fatalf("expected status experimental, got %s", updated.Status)
	}
	if updated.Version != "1.1" {
		t.Fatalf("expected version 1.1, got %s", updated.Version)
	}
	if !strings.Contains(updated.Content, "selection and selection_process and not filter_known_admin") {
		t.Fatalf("expected tightened condition in content, got:\n%s", updated.Content)
	}
}

func TestRuleGenerationApplyRuleAdjustmentIgnoresInvalidSeverityChange(t *testing.T) {
	db := setupRuleGenerationTestDB(t)
	now := time.Now()
	seedAIConfig(t, db, true, 1, 1)
	seedRuleAndAlerts(t, db, "rule-invalid-severity", "T1059.004", now, 1)

	service := newRuleGenerationTestService(db)
	rule, err := service.sigmaRuleRepo.FindByRuleID("rule-invalid-severity")
	if err != nil {
		t.Fatalf("failed to find seeded rule: %v", err)
	}
	rule.Content = "title: Broad shell rule\ndetection:\n  selection:\n    CommandLine|contains: bash\n  selection_process:\n    Image|endswith: /bash\n  filter_known_admin:\n    CommandLine|contains: uptime\n  condition: selection\n"
	if err := service.sigmaRuleRepo.Update(rule); err != nil {
		t.Fatalf("failed to update seeded rule content: %v", err)
	}

	err = service.applyRuleAdjustment(rule, RuleAdjustment{
		RuleID:          "rule-invalid-severity",
		Action:          "tighten",
		AddConditions:   []string{"selection_process"},
		SeverityChange:  "降低到 medium，当前更像批量误报",
		ExcludePatterns: []string{"filter_known_admin"},
	}, repository.RuleTriggerStats{RuleID: "rule-invalid-severity", MitreID: "T1059.004", AlertCount: 1, TimeWindow: "1h"})
	if err != nil {
		t.Fatalf("expected invalid severity text to be ignored, got error: %v", err)
	}

	updated, err := service.sigmaRuleRepo.FindByRuleID("rule-invalid-severity")
	if err != nil {
		t.Fatalf("failed to reload adjusted rule: %v", err)
	}
	if updated.Severity != "high" {
		t.Fatalf("expected severity to remain high, got %s", updated.Severity)
	}
}

func TestRuleGenerationConservativePolicyRequiresHigherConfidence(t *testing.T) {
	if threshold := falsePositiveConfidenceThreshold(0.0); threshold != 0.9 {
		t.Fatalf("expected strict conservative confidence threshold 0.9, got %.2f", threshold)
	}
	if threshold := falsePositiveConfidenceThreshold(1.0); threshold != 0.7 {
		t.Fatalf("expected aggressive confidence threshold 0.7, got %.2f", threshold)
	}
}

func TestRuleGenerationConservativeCooldownSkipsRecentlyUpdatedRule(t *testing.T) {
	now := time.Now()
	rule := &model.SigmaRule{
		RuleID:    "rule-cooldown",
		UpdatedAt: now.Add(-time.Hour),
	}

	if !shouldSkipRuleForCooldown(rule, 0.0, now) {
		t.Fatalf("expected conservative mode to skip rule updated inside cooldown")
	}

	rule.UpdatedAt = now.Add(-25 * time.Hour)
	if shouldSkipRuleForCooldown(rule, 0.0, now) {
		t.Fatalf("expected conservative mode to allow rule after cooldown")
	}
}

func TestRuleGenerationApplyRuleAdjustmentRejectsUndefinedSelector(t *testing.T) {
	db := setupRuleGenerationTestDB(t)
	now := time.Now()
	seedAIConfig(t, db, true, 1, 1)
	seedRuleAndAlerts(t, db, "rule-undefined-selector", "T1059.004", now, 1)

	service := newRuleGenerationTestService(db)
	rule, err := service.sigmaRuleRepo.FindByRuleID("rule-undefined-selector")
	if err != nil {
		t.Fatalf("failed to find seeded rule: %v", err)
	}

	err = service.applyRuleAdjustment(rule, RuleAdjustment{
		RuleID:        "rule-undefined-selector",
		Action:        "tighten",
		AddConditions: []string{"selection_missing"},
	}, repository.RuleTriggerStats{RuleID: "rule-undefined-selector", MitreID: "T1059.004", AlertCount: 1, TimeWindow: "1h"})
	if err == nil {
		t.Fatalf("expected undefined selector to reject rule tightening")
	}
}

func TestRuleGenerationApplyRuleAdjustmentRejectsNaturalLanguageCondition(t *testing.T) {
	db := setupRuleGenerationTestDB(t)
	now := time.Now()
	seedAIConfig(t, db, true, 1, 1)
	seedRuleAndAlerts(t, db, "rule-natural-language", "T1059.004", now, 1)

	service := newRuleGenerationTestService(db)
	rule, err := service.sigmaRuleRepo.FindByRuleID("rule-natural-language")
	if err != nil {
		t.Fatalf("failed to find seeded rule: %v", err)
	}

	err = service.applyRuleAdjustment(rule, RuleAdjustment{
		RuleID:        "rule-natural-language",
		Action:        "tighten",
		AddConditions: []string{"CommandLine contains 'curl'"},
	}, repository.RuleTriggerStats{RuleID: "rule-natural-language", MitreID: "T1059.004", AlertCount: 1, TimeWindow: "1h"})
	if err == nil {
		t.Fatalf("expected natural-language condition to reject rule tightening")
	}
}
