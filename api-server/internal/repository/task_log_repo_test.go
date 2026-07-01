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

func uuidPtr(id uuid.UUID) *uuid.UUID {
	return &id
}

func setupTaskLogRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	if logger.Logger == nil {
		if err := logger.Init(&logger.Config{Level: "error", MaxSize: 10, MaxBackups: 1, MaxAge: 1}); err != nil {
			t.Fatalf("failed to init logger: %v", err)
		}
	}

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
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
			max_rounds INTEGER NOT NULL DEFAULT 3,
			auto_verify BOOLEAN NOT NULL DEFAULT 0,
			verify_round INTEGER NOT NULL DEFAULT 0,
			stdout TEXT NULL,
			stderr TEXT NULL,
			exit_code INTEGER NULL,
			healing_id TEXT NULL,
			started_at DATETIME NULL,
			finished_at DATETIME NULL,
			created_at DATETIME NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("failed to create task_logs table: %v", err)
	}

	return db
}

func TestTaskLogRepository_UpdateForRedispatch(t *testing.T) {
	db := setupTaskLogRepoTestDB(t)
	repo := NewTaskLogRepository(db)

	originalScript := "echo old"
	stdout := "old stdout"
	stderr := "old stderr"
	exitCode := 1
	startedAt := time.Now().Add(-10 * time.Minute)
	finishedAt := time.Now().Add(-5 * time.Minute)
	scriptVersion := 1

	task := &model.TaskLog{
		ID:            uuid.New(),
		TaskGroupID:   uuid.New(),
		RuleID:        uuidPtr(uuid.New()),
		HostID:        uuid.New(),
		TaskType:      "fix",
		Status:        "failed",
		ScriptContent: &originalScript,
		ScriptVersion: &scriptVersion,
		Stdout:        &stdout,
		Stderr:        &stderr,
		ExitCode:      &exitCode,
		StartedAt:     &startedAt,
		FinishedAt:    &finishedAt,
		CreatedAt:     time.Now().Add(-20 * time.Minute),
	}

	if err := repo.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	if err := repo.UpdateForRedispatch(task.ID, "echo healed", 3); err != nil {
		t.Fatalf("UpdateForRedispatch failed: %v", err)
	}

	updated, err := repo.FindByID(task.ID)
	if err != nil {
		t.Fatalf("failed to reload updated task: %v", err)
	}

	if updated.Status != "PENDING" {
		t.Fatalf("expected status PENDING, got %s", updated.Status)
	}

	if updated.ScriptContent == nil || *updated.ScriptContent != "echo healed" {
		t.Fatalf("expected script_content to be updated to healed script")
	}

	if updated.ScriptVersion == nil || *updated.ScriptVersion != 3 {
		t.Fatalf("expected script_version to be 3")
	}

	if updated.Stdout != nil || updated.Stderr != nil || updated.ExitCode != nil {
		t.Fatalf("expected stdout/stderr/exit_code to be reset to nil")
	}

	if updated.FinishedAt != nil {
		t.Fatalf("expected finished_at to be reset to nil")
	}

	if updated.StartedAt == nil {
		t.Fatalf("expected started_at to be set")
	}
}

func TestTaskLogRepository_FindAutoVerifyTerminalTasks(t *testing.T) {
	db := setupTaskLogRepoTestDB(t)
	repo := NewTaskLogRepository(db)

	now := time.Now()
	exitCode := 1
	tasks := []*model.TaskLog{
		{
			ID:          uuid.New(),
			TaskGroupID: uuid.New(),
			RuleID:      uuidPtr(uuid.New()),
			HostID:      uuid.New(),
			TaskType:    "CHECK",
			Status:      "SUCCESS",
			ExitCode:    &exitCode,
			MaxRounds:   3,
			AutoVerify:  true,
			StartedAt:   &now,
			FinishedAt:  &now,
			CreatedAt:   now,
		},
		{
			ID:          uuid.New(),
			TaskGroupID: uuid.New(),
			RuleID:      uuidPtr(uuid.New()),
			HostID:      uuid.New(),
			TaskType:    "CHECK",
			Status:      "RUNNING",
			MaxRounds:   3,
			AutoVerify:  true,
			StartedAt:   &now,
			CreatedAt:   now,
		},
		{
			ID:          uuid.New(),
			TaskGroupID: uuid.New(),
			RuleID:      uuidPtr(uuid.New()),
			HostID:      uuid.New(),
			TaskType:    "CHECK",
			Status:      "SUCCESS",
			ExitCode:    &exitCode,
			MaxRounds:   3,
			AutoVerify:  false,
			StartedAt:   &now,
			FinishedAt:  &now,
			CreatedAt:   now,
		},
	}
	for _, task := range tasks {
		if err := repo.Create(task); err != nil {
			t.Fatalf("failed to create task: %v", err)
		}
	}

	found, err := repo.FindAutoVerifyTerminalTasks(100)
	if err != nil {
		t.Fatalf("FindAutoVerifyTerminalTasks failed: %v", err)
	}
	if len(found) != 1 || found[0].ID != tasks[0].ID {
		t.Fatalf("expected only terminal auto-verify task, got %#v", found)
	}
}

func TestTaskLogRepository_HasAutoVerifyFollowup(t *testing.T) {
	db := setupTaskLogRepoTestDB(t)
	repo := NewTaskLogRepository(db)

	groupID := uuid.New()
	ruleID := uuid.New()
	hostID := uuid.New()
	now := time.Now()
	task := &model.TaskLog{
		ID:          uuid.New(),
		TaskGroupID: groupID,
		RuleID:      &ruleID,
		HostID:      hostID,
		TaskType:    "FIX",
		Status:      "PENDING",
		MaxRounds:   3,
		AutoVerify:  true,
		VerifyRound: 2,
		StartedAt:   &now,
		CreatedAt:   now,
	}
	if err := repo.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	exists, err := repo.HasAutoVerifyFollowup(groupID, &ruleID, hostID, "FIX", 2)
	if err != nil {
		t.Fatalf("HasAutoVerifyFollowup failed: %v", err)
	}
	if !exists {
		t.Fatal("expected followup to exist")
	}

	exists, err = repo.HasAutoVerifyFollowup(groupID, &ruleID, hostID, "CHECK", 2)
	if err != nil {
		t.Fatalf("HasAutoVerifyFollowup failed: %v", err)
	}
	if exists {
		t.Fatal("did not expect CHECK followup to exist")
	}
}
