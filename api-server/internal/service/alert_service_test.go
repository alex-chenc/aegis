package service

import (
	"fmt"
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAlertServiceTestDB(t *testing.T) *gorm.DB {
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
		`CREATE TABLE block_policies (
			id TEXT PRIMARY KEY,
			mitre_id TEXT NOT NULL UNIQUE,
			mitre_name TEXT,
			enabled BOOLEAN NOT NULL DEFAULT 1,
			auto_block BOOLEAN NOT NULL DEFAULT 0,
			auto_dispose BOOLEAN NOT NULL DEFAULT 0,
			action TEXT NOT NULL DEFAULT 'kill_process',
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE block_records (
			id TEXT PRIMARY KEY,
			block_id TEXT NOT NULL UNIQUE,
			alert_id TEXT,
			host_id TEXT NOT NULL,
			action TEXT NOT NULL,
			target TEXT,
			reason TEXT,
			issued_by TEXT,
			success BOOLEAN,
			message TEXT,
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

func newAlertTestService(db *gorm.DB) *AlertService {
	return NewAlertService(
		repository.NewAlertRepository(db),
		repository.NewBlockPolicyRepository(db),
		repository.NewBlockRepository(db),
		nil,
	)
}

func seedBlockPolicy(t *testing.T, db *gorm.DB, mitreID string, enabled, autoBlock, autoDispose bool) {
	t.Helper()
	// Use raw SQL to avoid GORM zero-value bug: GORM skips bool:false fields,
	// letting SQLite DEFAULT apply, which may not match the intended value.
	now := time.Now().Format("2006-01-02 15:04:05")
	e, ab, ad := 0, 0, 0
	if enabled {
		e = 1
	}
	if autoBlock {
		ab = 1
	}
	if autoDispose {
		ad = 1
	}
	if err := db.Exec(
		"INSERT INTO block_policies (id, mitre_id, enabled, auto_block, auto_dispose, action, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		uuid.New().String(), mitreID, e, ab, ad, "kill_process", now, now,
	).Error; err != nil {
		t.Fatalf("failed to seed block policy: %v", err)
	}
}

func seedAlertForTest(t *testing.T, db *gorm.DB, alertID, mitreID, status string) *model.Alert {
	t.Helper()
	alert := &model.Alert{
		ID:          uuid.New(),
		AlertID:     alertID,
		HostID:      uuid.New(),
		PID:         100,
		MitreID:     mitreID,
		Severity:    "high",
		DedupeKey:   alertID + "-dedupe",
		Status:      status,
		HitCount:    1,
		FirstSeenAt: time.Now(),
		LastSeenAt:  time.Now(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.Create(alert).Error; err != nil {
		t.Fatalf("failed to seed alert: %v", err)
	}
	return alert
}

func TestCheckAndAutoDispose_ResolvesAlertWhenEnabled(t *testing.T) {
	db := setupAlertServiceTestDB(t)
	seedBlockPolicy(t, db, "T1059.004", true, false, true)
	alert := seedAlertForTest(t, db, "ALT-001", "T1059.004", "pending")

	svc := newAlertTestService(db)
	if err := svc.CheckAndAutoDispose(alert); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if alert.Status != StatusResolved {
		t.Fatalf("expected status resolved, got %s", alert.Status)
	}
	if !alert.AutoDispose {
		t.Fatal("expected AutoDispose=true")
	}
}

func TestCheckAndAutoDispose_NoOpWhenPolicyDisabled(t *testing.T) {
	db := setupAlertServiceTestDB(t)
	seedBlockPolicy(t, db, "T1059.004", false, false, true)
	alert := seedAlertForTest(t, db, "ALT-002", "T1059.004", "pending")

	svc := newAlertTestService(db)
	if err := svc.CheckAndAutoDispose(alert); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if alert.Status != "pending" {
		t.Fatalf("expected status to remain pending, got %s", alert.Status)
	}
}

func TestCheckAndAutoDispose_NoOpWhenAutoDisposeDisabled(t *testing.T) {
	db := setupAlertServiceTestDB(t)
	seedBlockPolicy(t, db, "T1059.004", true, false, false)
	alert := seedAlertForTest(t, db, "ALT-003", "T1059.004", "pending")

	svc := newAlertTestService(db)
	if err := svc.CheckAndAutoDispose(alert); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if alert.Status != "pending" {
		t.Fatalf("expected status to remain pending, got %s", alert.Status)
	}
}

func TestCheckAndAutoBlock_BlocksWhenEnabled(t *testing.T) {
	db := setupAlertServiceTestDB(t)
	seedBlockPolicy(t, db, "T1059.004", true, true, false)
	alert := seedAlertForTest(t, db, "ALT-004", "T1059.004", "pending")

	svc := newAlertTestService(db)
	if err := svc.CheckAndAutoBlock(alert); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !alert.AutoBlocked {
		t.Fatal("expected AutoBlocked=true")
	}
	if alert.BlockStatus == nil || *alert.BlockStatus != BlockBlocking {
		t.Fatal("expected BlockStatus=blocking")
	}
}

func TestCheckAndAutoBlock_NoOpWhenAutoBlockDisabled(t *testing.T) {
	db := setupAlertServiceTestDB(t)
	seedBlockPolicy(t, db, "T1059.004", true, false, false)
	alert := seedAlertForTest(t, db, "ALT-005", "T1059.004", "pending")

	svc := newAlertTestService(db)
	if err := svc.CheckAndAutoBlock(alert); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if alert.AutoBlocked {
		t.Fatal("expected AutoBlocked=false when AutoBlock policy is disabled")
	}
}

func TestUpsertByDedupe_CreatesNewAlert(t *testing.T) {
	db := setupAlertServiceTestDB(t)
	svc := newAlertTestService(db)

	alert, err := svc.UpsertByDedupe(uuid.New(), 100, "rule-1", "Test Rule", "T1059.004", "Unix Shell", "high", "test alert")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if alert.HitCount != 1 {
		t.Fatalf("expected HitCount=1, got %d", alert.HitCount)
	}
	if alert.Status != StatusPending {
		t.Fatalf("expected status pending, got %s", alert.Status)
	}
}

func TestUpsertByDedupe_IncrementsHitCount(t *testing.T) {
	db := setupAlertServiceTestDB(t)
	svc := newAlertTestService(db)

	hostID := uuid.New()
	alert1, err := svc.UpsertByDedupe(hostID, 100, "rule-1", "Test Rule", "T1059.004", "Unix Shell", "high", "test alert")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	alert2, err := svc.UpsertByDedupe(hostID, 100, "rule-1", "Test Rule", "T1059.004", "Unix Shell", "high", "test alert")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if alert1.ID != alert2.ID {
		t.Fatal("expected same alert to be returned (dedup)")
	}
	if alert2.HitCount != 2 {
		t.Fatalf("expected HitCount=2, got %d", alert2.HitCount)
	}
}
