package repository

import (
	"testing"
	"time"

	"server/internal/model"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func uuidPtr(id uuid.UUID) *uuid.UUID {
	return &id
}

func setupTaskLogRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	if err := db.AutoMigrate(&model.TaskLog{}); err != nil {
		t.Fatalf("failed to migrate task_logs: %v", err)
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

	if updated.Status != "pending" {
		t.Fatalf("expected status pending, got %s", updated.Status)
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
