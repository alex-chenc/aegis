package service

import (
	"strings"
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSigmaRuleUploadAllowsDistinctRulesForSameMitre(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:sigma-upload-duplicate-mitre?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE sigma_rules (
		id TEXT PRIMARY KEY,
		rule_id TEXT NOT NULL UNIQUE,
		title TEXT,
		content TEXT NOT NULL,
		status TEXT NOT NULL,
		mitre_id TEXT,
		severity TEXT,
		file_hash TEXT,
		description TEXT,
		generated_by TEXT,
		version TEXT,
		activated_at DATETIME,
		source TEXT,
		file_name TEXT,
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
		dispatch_status TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatal(err)
	}
	repo := repository.NewSigmaRuleRepository(db)
	now := time.Now()
	existing := &model.SigmaRule{
		ID:          uuid.New(),
		RuleID:      "persisted-rule-id",
		Title:       "Existing T1059 rule",
		Content:     "title: existing",
		Status:      "pending",
		MitreID:     "T1059",
		GeneratedBy: "upload",
		Version:     "1.0",
		CreatedAt:   now,
		UpdatedAt:   now,
		FileHash:    "different-file-hash",
	}
	if err := db.Exec(
		"INSERT INTO sigma_rules (id, rule_id, title, content, status, mitre_id, severity, file_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		existing.ID.String(), existing.RuleID, existing.Title, existing.Content, existing.Status, existing.MitreID, existing.Severity, existing.FileHash, now, now,
	).Error; err != nil {
		t.Fatal(err)
	}

	content := `title: Uploaded SSH shell rule
id: yaml-rule-id
tags:
  - attack.t1059
logsource:
  category: process_creation
detection:
  selection:
    Image: /usr/bin/ssh
  condition: selection
level: high
`
	result, err := NewSigmaRuleUploadService(repo, nil).UploadRules(strings.NewReader(content), "rule.yml", int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.Success || result.ParsedCount != 1 || result.SkippedCount != 0 || len(result.Rules) != 1 {
		t.Fatalf("unexpected import result: %#v", result)
	}
	if result.Rules[0].RuleID != "yaml-rule-id" {
		t.Fatalf("import result rule_id = %q, want uploaded rule ID", result.Rules[0].RuleID)
	}
	if _, err := repo.FindByRuleID("yaml-rule-id"); err != nil {
		t.Fatalf("uploaded rule with shared MITRE ID was not persisted: %v", err)
	}
}
