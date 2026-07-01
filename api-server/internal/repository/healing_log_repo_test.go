package repository

import (
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupHealingLogRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	if logger.Logger == nil {
		if err := logger.Init(&logger.Config{Level: "error"}); err != nil {
			t.Fatalf("failed to init logger: %v", err)
		}
	}

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE self_healing_logs (
			id TEXT PRIMARY KEY,
			original_task_id TEXT NOT NULL,
			rule_id TEXT NULL,
			host_id TEXT NOT NULL,
			vulnerability_id TEXT NULL,
			script_type TEXT NOT NULL,
			trigger_error TEXT NOT NULL,
			trigger_exit_code INTEGER NOT NULL,
			total_attempts INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 3,
			status TEXT NOT NULL DEFAULT 'healing',
			final_script_version_id TEXT NULL,
			attempts_detail TEXT NULL,
			user_suggestion TEXT NULL,
			last_error TEXT NULL,
			started_at DATETIME NOT NULL,
			finished_at DATETIME NULL,
			created_at DATETIME NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("failed to create self_healing_logs table: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE task_logs (
			id TEXT PRIMARY KEY,
			task_group_id TEXT NOT NULL,
			rule_id TEXT NULL,
			host_id TEXT NOT NULL,
			vulnerability_id TEXT NULL,
			task_type TEXT NOT NULL,
			status TEXT NOT NULL,
			script_content TEXT NULL,
			script_version INTEGER NULL,
			attempt_no INTEGER NOT NULL DEFAULT 1,
			max_rounds INTEGER NOT NULL DEFAULT 1,
			auto_verify BOOLEAN NOT NULL DEFAULT 0,
			verify_round INTEGER NOT NULL DEFAULT 0,
			stdout TEXT NULL,
			stderr TEXT NULL,
			exit_code INTEGER NULL,
			healing_id TEXT NULL REFERENCES self_healing_logs(id),
			started_at DATETIME NULL,
			finished_at DATETIME NULL,
			created_at DATETIME NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("failed to create task_logs table: %v", err)
	}

	return db
}

func TestHealingLogRepository_DeleteByOriginalTaskIDsClearsTaskReferences(t *testing.T) {
	db := setupHealingLogRepoTestDB(t)
	repo := NewHealingLogRepository(db)

	originalTaskID := uuid.New()
	ruleID := uuid.New()
	hostID := uuid.New()
	healingID := uuid.New()
	now := time.Now()
	healingLog := &model.HealingLog{
		ID:              healingID,
		OriginalTaskID:  originalTaskID,
		RuleID:          &ruleID,
		HostID:          hostID,
		ScriptType:      "FIX",
		TriggerError:    "failed",
		TriggerExitCode: 1,
		TotalAttempts:   1,
		MaxAttempts:     3,
		Status:          "failed",
		AttemptsDetail:  make(model.AttemptsDetail, 0),
		StartedAt:       now,
		CreatedAt:       now,
	}
	if err := repo.Create(healingLog); err != nil {
		t.Fatalf("failed to create healing log: %v", err)
	}

	script := "echo healed"
	task := &model.TaskLog{
		ID:            uuid.New(),
		TaskGroupID:   uuid.New(),
		RuleID:        &ruleID,
		HostID:        hostID,
		TaskType:      "FIX",
		Status:        "SUCCESS",
		ScriptContent: &script,
		AttemptNo:     2,
		MaxRounds:     10,
		HealingID:     &healingID,
		CreatedAt:     now,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("failed to create task log: %v", err)
	}

	if err := repo.DeleteByOriginalTaskIDs([]uuid.UUID{originalTaskID}); err != nil {
		t.Fatalf("DeleteByOriginalTaskIDs failed: %v", err)
	}

	var count int64
	if err := db.Model(&model.HealingLog{}).Where("id = ?", healingID).Count(&count).Error; err != nil {
		t.Fatalf("failed to count healing logs: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected healing log to be deleted, got %d", count)
	}

	var updated model.TaskLog
	if err := db.First(&updated, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("failed to reload task log: %v", err)
	}
	if updated.HealingID != nil {
		t.Fatalf("expected task healing_id to be cleared")
	}
}
