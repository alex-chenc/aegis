package repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSigmaRuleRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	if err := db.Exec(`CREATE TABLE sigma_rules (
		id TEXT PRIMARY KEY,
		rule_id TEXT NOT NULL UNIQUE,
		title TEXT,
		content TEXT NOT NULL,
		status TEXT NOT NULL,
		activated_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("failed to create sigma_rules: %v", err)
	}

	return db
}

func TestSigmaRuleRepositoryGetActiveAndExperimentalIncludesNewExperimentalRules(t *testing.T) {
	db := setupSigmaRuleRepoTestDB(t)
	repo := NewSigmaRuleRepository(db)
	now := time.Now()

	rules := []struct {
		ruleID      string
		title       string
		content     string
		status      string
		activatedAt *time.Time
	}{
		{ruleID: "active-rule", title: "active", content: "title: active", status: "active", activatedAt: &now},
		{ruleID: "experimental-rule", title: "experimental", content: "title: experimental", status: "experimental", activatedAt: &now},
		{ruleID: "pending-rule", title: "pending", content: "title: pending", status: "pending"},
		{ruleID: "disabled-rule", title: "disabled", content: "title: disabled", status: "disabled"},
	}
	for _, rule := range rules {
		if err := db.Exec(
			"INSERT INTO sigma_rules (id, rule_id, title, content, status, activated_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			uuid.New().String(), rule.ruleID, rule.title, rule.content, rule.status, rule.activatedAt, now, now,
		).Error; err != nil {
			t.Fatalf("failed to seed rule %s: %v", rule.ruleID, err)
		}
	}

	got, err := repo.GetActiveAndExperimental()
	if err != nil {
		t.Fatalf("GetActiveAndExperimental failed: %v", err)
	}

	seen := map[string]bool{}
	for _, rule := range got {
		seen[rule.RuleID] = true
	}

	if !seen["active-rule"] {
		t.Fatal("expected active rule to be returned")
	}
	if !seen["experimental-rule"] {
		t.Fatal("expected newly experimental rule to be returned")
	}
	if seen["pending-rule"] {
		t.Fatal("did not expect pending rule to be returned")
	}
	if seen["disabled-rule"] {
		t.Fatal("did not expect disabled rule to be returned")
	}
}
