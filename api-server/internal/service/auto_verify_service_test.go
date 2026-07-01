package service

import (
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAutoVerifyServiceTest(t *testing.T) (*repository.TaskLogRepository, *repository.RuleRepository, *gorm.DB) {
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
		CREATE TABLE aegis_rules (
			id TEXT PRIMARY KEY,
			template_id TEXT NOT NULL,
			title TEXT NOT NULL,
			check_content TEXT NOT NULL,
			fix_content TEXT NOT NULL,
			generated_check_script TEXT NULL,
			generated_fix_script TEXT NULL,
			check_script_version INTEGER DEFAULT 0,
			fix_script_version INTEGER DEFAULT 0,
			check_script_status TEXT DEFAULT 'pending',
			fix_script_status TEXT DEFAULT 'pending',
			check_script_error TEXT NULL,
			fix_script_error TEXT NULL,
			script_status TEXT DEFAULT 'pending',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("failed to create aegis_rules table: %v", err)
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

	return repository.NewTaskLogRepository(db), repository.NewRuleRepository(db), db
}

func createAutoVerifyRule(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	checkScript := "exit 0"
	fixScript := "exit 0"
	rule := &model.AegisRule{
		ID:                   uuid.New(),
		TemplateID:           uuid.New(),
		Title:                "baseline rule",
		CheckContent:         "check",
		FixContent:           "fix",
		GeneratedCheckScript: &checkScript,
		GeneratedFixScript:   &fixScript,
		CheckScriptStatus:    "generated",
		FixScriptStatus:      "generated",
		ScriptStatus:         "ready",
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
	if err := db.Create(rule).Error; err != nil {
		t.Fatalf("failed to create rule: %v", err)
	}
	return rule.ID
}

func TestAutoVerifyCheckFailureCreatesFixWithOriginalMaxRounds(t *testing.T) {
	taskRepo, ruleRepo, db := setupAutoVerifyServiceTest(t)
	ruleID := createAutoVerifyRule(t, db)
	groupID := uuid.New()
	hostID := uuid.New()
	exitCode := 1
	now := time.Now()
	task := &model.TaskLog{
		ID:          uuid.New(),
		TaskGroupID: groupID,
		RuleID:      &ruleID,
		HostID:      hostID,
		TaskType:    "CHECK",
		Status:      "SUCCESS",
		ExitCode:    &exitCode,
		MaxRounds:   4,
		AutoVerify:  true,
		VerifyRound: 0,
		StartedAt:   &now,
		FinishedAt:  &now,
		CreatedAt:   now,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	svc := NewAutoVerifyService(taskRepo, ruleRepo, nil)
	if !svc.HandleTaskResult(task, "SUCCESS", 1) {
		t.Fatal("expected check failure to be handled")
	}
	if !svc.HandleTaskResult(task, "SUCCESS", 1) {
		t.Fatal("expected duplicate handling to be idempotent")
	}

	var followups []model.TaskLog
	if err := db.Where("task_group_id = ? AND task_type = ?", groupID, "FIX").Find(&followups).Error; err != nil {
		t.Fatalf("failed to query followups: %v", err)
	}
	if len(followups) != 1 {
		t.Fatalf("expected exactly one FIX followup, got %d", len(followups))
	}
	if followups[0].MaxRounds != 4 {
		t.Fatalf("expected max_rounds 4, got %d", followups[0].MaxRounds)
	}
	if followups[0].VerifyRound != 1 {
		t.Fatalf("expected verify_round 1, got %d", followups[0].VerifyRound)
	}
}

func TestAutoVerifyFixSuccessCreatesCheckWithOriginalMaxRounds(t *testing.T) {
	taskRepo, ruleRepo, db := setupAutoVerifyServiceTest(t)
	ruleID := createAutoVerifyRule(t, db)
	groupID := uuid.New()
	hostID := uuid.New()
	exitCode := 0
	now := time.Now()
	task := &model.TaskLog{
		ID:          uuid.New(),
		TaskGroupID: groupID,
		RuleID:      &ruleID,
		HostID:      hostID,
		TaskType:    "FIX",
		Status:      "SUCCESS",
		ExitCode:    &exitCode,
		MaxRounds:   5,
		AutoVerify:  true,
		VerifyRound: 2,
		StartedAt:   &now,
		FinishedAt:  &now,
		CreatedAt:   now,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	svc := NewAutoVerifyService(taskRepo, ruleRepo, nil)
	if !svc.HandleTaskResult(task, "SUCCESS", 0) {
		t.Fatal("expected fix success to be handled")
	}

	var followup model.TaskLog
	if err := db.Where("task_group_id = ? AND task_type = ?", groupID, "CHECK").First(&followup).Error; err != nil {
		t.Fatalf("expected CHECK followup: %v", err)
	}
	if followup.MaxRounds != 5 {
		t.Fatalf("expected max_rounds 5, got %d", followup.MaxRounds)
	}
	if followup.VerifyRound != 2 {
		t.Fatalf("expected verify_round 2, got %d", followup.VerifyRound)
	}
}

func TestAutoVerifyFixFailureDoesNotRepeatOldFixScript(t *testing.T) {
	taskRepo, ruleRepo, db := setupAutoVerifyServiceTest(t)
	ruleID := createAutoVerifyRule(t, db)
	exitCode := 1
	now := time.Now()
	task := &model.TaskLog{
		ID:          uuid.New(),
		TaskGroupID: uuid.New(),
		RuleID:      &ruleID,
		HostID:      uuid.New(),
		TaskType:    "FIX",
		Status:      "FAILED",
		ExitCode:    &exitCode,
		MaxRounds:   3,
		AutoVerify:  true,
		VerifyRound: 1,
		StartedAt:   &now,
		FinishedAt:  &now,
		CreatedAt:   now,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	svc := NewAutoVerifyService(taskRepo, ruleRepo, nil)
	if !svc.HandleTaskResult(task, "FAILED", 1) {
		t.Fatal("expected fix failure to be handled")
	}

	var count int64
	if err := db.Model(&model.TaskLog{}).Where("task_group_id = ?", task.TaskGroupID).Count(&count).Error; err != nil {
		t.Fatalf("failed to count tasks: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected no duplicate FIX task, got %d tasks", count)
	}
}

func TestAutoVerifyScannerHandlesPersistedTerminalTaskOnce(t *testing.T) {
	taskRepo, ruleRepo, db := setupAutoVerifyServiceTest(t)
	ruleID := createAutoVerifyRule(t, db)
	groupID := uuid.New()
	exitCode := 1
	now := time.Now()
	task := &model.TaskLog{
		ID:          uuid.New(),
		TaskGroupID: groupID,
		RuleID:      &ruleID,
		HostID:      uuid.New(),
		TaskType:    "CHECK",
		Status:      "SUCCESS",
		ExitCode:    &exitCode,
		MaxRounds:   3,
		AutoVerify:  true,
		VerifyRound: 0,
		StartedAt:   &now,
		FinishedAt:  &now,
		CreatedAt:   now,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	svc := NewAutoVerifyService(taskRepo, ruleRepo, nil)
	svc.scanCompletedTaskResults()
	svc.scanCompletedTaskResults()

	var followups []model.TaskLog
	if err := db.Where("task_group_id = ? AND task_type = ?", groupID, "FIX").Find(&followups).Error; err != nil {
		t.Fatalf("failed to query followups: %v", err)
	}
	if len(followups) != 1 {
		t.Fatalf("expected scanner to create one FIX followup, got %d", len(followups))
	}
}
