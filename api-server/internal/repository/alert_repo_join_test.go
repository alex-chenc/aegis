package repository

import (
	"fmt"
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAlertJoinTestDB(t *testing.T) *gorm.DB {
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
		`CREATE TABLE hosts (
			id TEXT PRIMARY KEY,
			hostname TEXT,
			ip_address TEXT,
			os_type TEXT,
			agent_version TEXT,
			status TEXT,
			last_heartbeat DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE sigma_rules (
			id TEXT PRIMARY KEY,
			rule_id TEXT,
			title TEXT,
			description TEXT,
			content TEXT,
			status TEXT DEFAULT 'pending',
			mitre_id TEXT,
			severity TEXT,
			generated_by TEXT DEFAULT 'llm',
			version TEXT DEFAULT '1.0',
			created_at DATETIME,
			activated_at DATETIME,
			updated_at DATETIME,
			source TEXT DEFAULT 'upload',
			file_name TEXT,
			file_hash TEXT,
			file_size INTEGER DEFAULT 0,
			parsed_at DATETIME,
			parse_error TEXT,
			ai_generated BOOLEAN DEFAULT 0,
			parent_rule_id TEXT,
			generation_prompt TEXT,
			generation_context TEXT,
			approved_by TEXT,
			approved_at DATETIME,
			dispatch_hosts TEXT,
			dispatch_status TEXT DEFAULT 'pending'
		)`,
		`CREATE TABLE runtime_events (
			id TEXT PRIMARY KEY,
			event_id TEXT NOT NULL UNIQUE,
			host_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			event_data TEXT NOT NULL,
			matched_rule_id TEXT,
			rule_title TEXT,
			mitre_id TEXT,
			severity TEXT,
			pid INTEGER,
			command_line TEXT,
			timestamp INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			aggregated BOOLEAN DEFAULT FALSE
		)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("failed to create test schema: %v", err)
		}
	}

	return db
}

func TestAlertRepoFindByIDDerivesProcessCountFromCorrelationEvidence(t *testing.T) {
	db := setupAlertJoinTestDB(t)
	repo := NewAlertRepository(db)

	hostID := uuid.New()
	ruleID := "pkg.copyfail_chain"
	alertID := "ALT-process-count"
	alert := &model.Alert{
		ID:        uuid.New(),
		AlertID:   alertID,
		HostID:    hostID,
		PID:       4321,
		MitreID:   "T1068",
		Severity:  "critical",
		DedupeKey: "test:4321:pkg.copyfail_chain",
		HitCount:  1,
		Status:    "pending",
		RuleID:    ruleID,
	}
	if err := db.Create(alert).Error; err != nil {
		t.Fatalf("failed to seed alert: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO runtime_events (id, event_id, host_id, event_type, event_data, matched_rule_id, mitre_id, severity, pid, timestamp)
		VALUES (?, ?, ?, 'correlation_alert', ?, ?, 'T1068', 'critical', ?, ?)
	`, uuid.New().String(), "EVT-process-count", hostID.String(),
		`[{"rule_id":"pkg.socket","pid":4321},{"rule_id":"pkg.bind","pid":4321},{"rule_id":"pkg.splice","pid":9876}]`,
		ruleID, 4321, time.Now().UnixMilli()).Error; err != nil {
		t.Fatalf("failed to seed runtime event: %v", err)
	}

	result, err := repo.FindByID(alertID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if result.ProcessCount != 2 {
		t.Fatalf("ProcessCount = %d, want 2", result.ProcessCount)
	}
}

func TestAlertRepoJoinResolvesRuleTitleByRuleID(t *testing.T) {
	db := setupAlertJoinTestDB(t)
	repo := NewAlertRepository(db)

	// Seed a sigma rule with rule_id and title
	ruleID := "test-rule-001"
	ruleTitle := "反弹Shell命令行检测"
	sigmaRule := &model.SigmaRule{
		ID:      uuid.New(),
		RuleID:  ruleID,
		Title:   ruleTitle,
		MitreID: "T888",
	}
	if err := db.Create(sigmaRule).Error; err != nil {
		t.Fatalf("failed to seed sigma rule: %v", err)
	}

	// Seed an alert with the same rule_id but empty rule_title
	alertID := "ALT-test01"
	alert := &model.Alert{
		ID:        uuid.New(),
		AlertID:   alertID,
		HostID:    uuid.New(),
		PID:       12345,
		MitreID:   "T888",
		Severity:  "high",
		DedupeKey: "test:12345:T888",
		HitCount:  1,
		Status:    "pending",
		RuleID:    ruleID,
		RuleTitle: "", // empty - should be resolved from sigma_rules
	}
	if err := db.Create(alert).Error; err != nil {
		t.Fatalf("failed to seed alert: %v", err)
	}

	// Query the alert - rule_title should be resolved from sigma_rules via JOIN
	result, err := repo.FindByID(alertID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}

	if result.RuleTitle != ruleTitle {
		t.Errorf("expected rule_title %q, got %q", ruleTitle, result.RuleTitle)
	}
}

func TestAlertRepoJoinResolvesRuleTitleFromAlertWhenSet(t *testing.T) {
	db := setupAlertJoinTestDB(t)
	repo := NewAlertRepository(db)

	// Seed a sigma rule
	ruleID := "test-rule-002"
	sigmaRule := &model.SigmaRule{
		ID:      uuid.New(),
		RuleID:  ruleID,
		Title:   "Sigma Rule Title",
		MitreID: "T1059",
	}
	if err := db.Create(sigmaRule).Error; err != nil {
		t.Fatalf("failed to seed sigma rule: %v", err)
	}

	// Seed an alert with its own rule_title (should take priority)
	alertID := "ALT-test02"
	alertTitle := "Alert Own Title"
	alert := &model.Alert{
		ID:        uuid.New(),
		AlertID:   alertID,
		HostID:    uuid.New(),
		PID:       12345,
		MitreID:   "T1059",
		Severity:  "high",
		DedupeKey: "test:12345:T1059",
		HitCount:  1,
		Status:    "pending",
		RuleID:    ruleID,
		RuleTitle: alertTitle,
	}
	if err := db.Create(alert).Error; err != nil {
		t.Fatalf("failed to seed alert: %v", err)
	}

	// Query the alert - rule_title should be the alert's own value
	result, err := repo.FindByID(alertID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}

	if result.RuleTitle != alertTitle {
		t.Errorf("expected rule_title %q, got %q", alertTitle, result.RuleTitle)
	}
}

func TestAlertRepoJoinFallsBackToMitreNameWhenNoSigmaRule(t *testing.T) {
	db := setupAlertJoinTestDB(t)
	repo := NewAlertRepository(db)

	// Seed an alert with no matching sigma rule, but with mitre_name
	alertID := "ALT-test03"
	mitreName := "反弹Shell命令行检测"
	alert := &model.Alert{
		ID:        uuid.New(),
		AlertID:   alertID,
		HostID:    uuid.New(),
		PID:       12345,
		MitreID:   "T888",
		MitreName: mitreName,
		Severity:  "high",
		DedupeKey: "test:12345:T888",
		HitCount:  1,
		Status:    "pending",
		RuleID:    "nonexistent-rule",
		RuleTitle: "",
	}
	if err := db.Create(alert).Error; err != nil {
		t.Fatalf("failed to seed alert: %v", err)
	}

	// Query the alert - rule_title should fall back to mitre_name
	result, err := repo.FindByID(alertID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}

	if result.RuleTitle != mitreName {
		t.Errorf("expected rule_title %q, got %q", mitreName, result.RuleTitle)
	}
}
