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

func setupAIAutoBlockTestDB(t *testing.T) *gorm.DB {
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
		`CREATE TABLE hosts (
			id TEXT PRIMARY KEY,
			hostname TEXT,
			ip_address TEXT,
			os_type TEXT,
			agent_version TEXT,
			status TEXT,
			last_heartbeat_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE sigma_rules (
			id TEXT PRIMARY KEY,
			rule_id TEXT NOT NULL UNIQUE,
			title TEXT,
			description TEXT,
			content TEXT,
			status TEXT,
			mitre_id TEXT,
			severity TEXT,
			generated_by TEXT,
			version TEXT,
			created_at DATETIME,
			activated_at DATETIME
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
		`CREATE TABLE block_policies (
			id TEXT PRIMARY KEY,
			mitre_id TEXT NOT NULL UNIQUE,
			mitre_name TEXT,
			enabled BOOLEAN NOT NULL DEFAULT 1,
			auto_block BOOLEAN NOT NULL DEFAULT 0,
			ai_auto_block BOOLEAN NOT NULL DEFAULT 0,
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

func newAIAutoBlockTestService(db *gorm.DB) *AIAutoBlockService {
	return NewAIAutoBlockService(
		repository.NewAlertRepository(db),
		repository.NewBlockPolicyRepository(db),
		repository.NewBlockRepository(db),
		nil, // no gRPC client in tests
	)
}

func seedAIAutoBlockPolicy(t *testing.T, db *gorm.DB, mitreID string, enabled, autoBlock, aiAutoBlock bool) {
	t.Helper()
	now := time.Now().Format("2006-01-02 15:04:05")
	e, ab, aib := 0, 0, 0
	if enabled {
		e = 1
	}
	if autoBlock {
		ab = 1
	}
	if aiAutoBlock {
		aib = 1
	}
	if err := db.Exec(
		"INSERT INTO block_policies (id, mitre_id, enabled, auto_block, ai_auto_block, action, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		uuid.New().String(), mitreID, e, ab, aib, "kill_process", now, now,
	).Error; err != nil {
		t.Fatalf("failed to seed block policy: %v", err)
	}
}

func seedAIAutoBlockHost(t *testing.T, db *gorm.DB, hostID string) {
	t.Helper()
	db.Exec("INSERT INTO hosts (id, hostname, ip_address, os_type, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		hostID, "test-host", "192.168.1.1", "linux", "online", time.Now(), time.Now())
}

func seedAIAutoBlockAlert(t *testing.T, db *gorm.DB, alertID, mitreID, hostID string) *model.Alert {
	t.Helper()
	alert := &model.Alert{
		ID:          uuid.New(),
		AlertID:     alertID,
		HostID:      uuid.MustParse(hostID),
		PID:         100,
		MitreID:     mitreID,
		Severity:    "high",
		DedupeKey:   alertID + "-dedupe",
		Status:      "pending",
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

func TestAIAutoBlock_Execute_SkipsNonConfirmThreat(t *testing.T) {
	db := setupAIAutoBlockTestDB(t)
	seedAIAutoBlockPolicy(t, db, "T1059.004", true, false, true)
	hostID := uuid.New().String()
	seedAIAutoBlockHost(t, db, hostID)
	_ = seedAIAutoBlockAlert(t, db, "ALT-001", "T1059.004", hostID)

	svc := newAIAutoBlockTestService(db)
	conclusions := []AlertConclusion{
		{AlertID: "ALT-001", Action: "mark_false_positive", Summary: "false positive"},
	}

	payload := svc.Execute(conclusions, nil)
	if payload.Triggered {
		t.Fatal("expected triggered=false for non-confirm_threat conclusions")
	}
	if payload.Summary.Total != 0 {
		t.Fatalf("expected total=0, got %d", payload.Summary.Total)
	}
}

func TestAIAutoBlock_Execute_ConfirmThreatBlocksWhenPolicyEnabled(t *testing.T) {
	db := setupAIAutoBlockTestDB(t)
	hostID := uuid.New().String()
	seedAIAutoBlockHost(t, db, hostID)
	seedAIAutoBlockPolicy(t, db, "T1059.004", true, false, true)
	alert := seedAIAutoBlockAlert(t, db, "ALT-002", "T1059.004", hostID)

	svc := newAIAutoBlockTestService(db)
	conclusions := []AlertConclusion{
		{AlertID: "ALT-002", Action: "confirm_threat", Summary: "confirmed threat"},
	}
	alertIDToUUID := map[string]uuid.UUID{
		"ALT-002": alert.ID,
	}

	payload := svc.Execute(conclusions, alertIDToUUID)
	if !payload.Triggered {
		t.Fatal("expected triggered=true")
	}
	if payload.Summary.Total != 1 {
		t.Fatalf("expected total=1, got %d", payload.Summary.Total)
	}
	// No gRPC client, so it should fail
	if payload.Results[0].Status != "failed" {
		t.Fatalf("expected failed status (no gRPC client), got %s", payload.Results[0].Status)
	}
	if payload.Results[0].IssuedBy != "ai_auto" {
		t.Fatalf("expected issued_by=ai_auto, got %s", payload.Results[0].IssuedBy)
	}
}

func TestAIAutoBlock_Execute_SkipsWhenPolicyDisabled(t *testing.T) {
	db := setupAIAutoBlockTestDB(t)
	hostID := uuid.New().String()
	seedAIAutoBlockHost(t, db, hostID)
	seedAIAutoBlockPolicy(t, db, "T1059.004", false, false, true)
	alert := seedAIAutoBlockAlert(t, db, "ALT-003", "T1059.004", hostID)

	svc := newAIAutoBlockTestService(db)
	conclusions := []AlertConclusion{
		{AlertID: "ALT-003", Action: "confirm_threat", Summary: "confirmed threat"},
	}
	alertIDToUUID := map[string]uuid.UUID{
		"ALT-003": alert.ID,
	}

	payload := svc.Execute(conclusions, alertIDToUUID)
	if payload.Results[0].Status != "skipped" {
		t.Fatalf("expected skipped, got %s", payload.Results[0].Status)
	}
	if payload.Summary.Skipped != 1 {
		t.Fatalf("expected skipped=1, got %d", payload.Summary.Skipped)
	}
}

func TestAIAutoBlock_Execute_SkipsWhenAIAutoBlockDisabled(t *testing.T) {
	db := setupAIAutoBlockTestDB(t)
	hostID := uuid.New().String()
	seedAIAutoBlockHost(t, db, hostID)
	seedAIAutoBlockPolicy(t, db, "T1059.004", true, false, false)
	alert := seedAIAutoBlockAlert(t, db, "ALT-004", "T1059.004", hostID)

	svc := newAIAutoBlockTestService(db)
	conclusions := []AlertConclusion{
		{AlertID: "ALT-004", Action: "confirm_threat", Summary: "confirmed threat"},
	}
	alertIDToUUID := map[string]uuid.UUID{
		"ALT-004": alert.ID,
	}

	payload := svc.Execute(conclusions, alertIDToUUID)
	if payload.Results[0].Status != "skipped" {
		t.Fatalf("expected skipped, got %s", payload.Results[0].Status)
	}
}

func TestAIAutoBlock_Execute_SkipsWhenAutoBlockEnabled(t *testing.T) {
	db := setupAIAutoBlockTestDB(t)
	hostID := uuid.New().String()
	seedAIAutoBlockHost(t, db, hostID)
	seedAIAutoBlockPolicy(t, db, "T1059.004", true, true, false)
	alert := seedAIAutoBlockAlert(t, db, "ALT-005", "T1059.004", hostID)

	svc := newAIAutoBlockTestService(db)
	conclusions := []AlertConclusion{
		{AlertID: "ALT-005", Action: "confirm_threat", Summary: "confirmed threat"},
	}
	alertIDToUUID := map[string]uuid.UUID{
		"ALT-005": alert.ID,
	}

	payload := svc.Execute(conclusions, alertIDToUUID)
	if payload.Results[0].Status != "skipped" {
		t.Fatalf("expected skipped (auto_block enabled), got %s", payload.Results[0].Status)
	}
}

func TestAIAutoBlock_Execute_SkipsWhenBlockRecordExists(t *testing.T) {
	db := setupAIAutoBlockTestDB(t)
	hostID := uuid.New().String()
	seedAIAutoBlockHost(t, db, hostID)
	seedAIAutoBlockPolicy(t, db, "T1059.004", true, false, true)
	alert := seedAIAutoBlockAlert(t, db, "ALT-006", "T1059.004", hostID)

	// Seed an existing block record using GORM model instead of raw SQL
	blockRecord := &model.BlockRecord{
		BlockID:  "BLK-EXISTING",
		AlertID:  &alert.ID,
		HostID:   alert.HostID,
		Action:   "kill_process",
		Target:   "100",
		Success:  true,
		Message:  "阻断成功",
		IssuedBy: "manual",
	}
	if err := db.Create(blockRecord).Error; err != nil {
		t.Fatalf("failed to seed block record: %v", err)
	}

	svc := newAIAutoBlockTestService(db)
	conclusions := []AlertConclusion{
		{AlertID: "ALT-006", Action: "confirm_threat", Summary: "confirmed threat"},
	}
	alertIDToUUID := map[string]uuid.UUID{
		"ALT-006": alert.ID,
	}

	payload := svc.Execute(conclusions, alertIDToUUID)
	if payload.Results[0].Status != "skipped" {
		t.Fatalf("expected skipped (existing block record), got %s", payload.Results[0].Status)
	}
	if payload.Results[0].ExistingBlockID != "BLK-EXISTING" {
		t.Fatalf("expected existing_block_id=BLK-EXISTING, got %s", payload.Results[0].ExistingBlockID)
	}
}

func TestAIAutoBlock_Execute_SkipsWhenAlertNotFound(t *testing.T) {
	db := setupAIAutoBlockTestDB(t)
	seedAIAutoBlockPolicy(t, db, "T1059.004", true, false, true)

	svc := newAIAutoBlockTestService(db)
	conclusions := []AlertConclusion{
		{AlertID: "ALT-NONEXIST", Action: "confirm_threat", Summary: "confirmed threat"},
	}
	alertIDToUUID := map[string]uuid.UUID{
		"ALT-NONEXIST": uuid.New(),
	}

	payload := svc.Execute(conclusions, alertIDToUUID)
	if payload.Results[0].Status != "skipped" {
		t.Fatalf("expected skipped, got %s", payload.Results[0].Status)
	}
}

func TestAIAutoBlock_Execute_HandlesMultipleConclusions(t *testing.T) {
	db := setupAIAutoBlockTestDB(t)
	hostID := uuid.New().String()
	seedAIAutoBlockHost(t, db, hostID)
	seedAIAutoBlockPolicy(t, db, "T1059.004", true, false, true)
	alert1 := seedAIAutoBlockAlert(t, db, "ALT-007", "T1059.004", hostID)
	alert2 := seedAIAutoBlockAlert(t, db, "ALT-008", "T1059.004", hostID)

	svc := newAIAutoBlockTestService(db)
	conclusions := []AlertConclusion{
		{AlertID: "ALT-007", Action: "confirm_threat", Summary: "threat 1"},
		{AlertID: "ALT-008", Action: "confirm_threat", Summary: "threat 2"},
		{AlertID: "ALT-009", Action: "mark_false_positive", Summary: "false positive"},
	}
	alertIDToUUID := map[string]uuid.UUID{
		"ALT-007": alert1.ID,
		"ALT-008": alert2.ID,
	}

	payload := svc.Execute(conclusions, alertIDToUUID)
	if payload.Summary.Total != 2 {
		t.Fatalf("expected total=2 (only confirm_threat), got %d", payload.Summary.Total)
	}
	if len(payload.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(payload.Results))
	}
}

func TestAIAutoBlock_TargetResolution(t *testing.T) {
	alert := &model.Alert{
		PID:         4242,
		CommandLine: "/tmp/test-file",
	}

	target, err := aiAutoBlockTargetForAlert(alert, "kill_process")
	if err != nil {
		t.Fatalf("kill_process failed: %v", err)
	}
	if target != "4242" {
		t.Fatalf("expected 4242, got %s", target)
	}

	target, err = aiAutoBlockTargetForAlert(alert, "quarantine_file")
	if err != nil {
		t.Fatalf("quarantine_file failed: %v", err)
	}
	if target != "/tmp/test-file" {
		t.Fatalf("expected /tmp/test-file, got %s", target)
	}

	alert.CommandLine = "203.0.113.25"
	target, err = aiAutoBlockTargetForAlert(alert, "block_connection")
	if err != nil {
		t.Fatalf("block_connection failed: %v", err)
	}
	if target != "203.0.113.25" {
		t.Fatalf("expected 203.0.113.25, got %s", target)
	}
}

func TestAIAutoBlock_TargetResolution_MissingTargets(t *testing.T) {
	alert := &model.Alert{PID: 4242}

	if _, err := aiAutoBlockTargetForAlert(alert, "quarantine_file"); err == nil {
		t.Fatal("expected missing quarantine_file target to fail")
	}
	if _, err := aiAutoBlockTargetForAlert(alert, "block_connection"); err == nil {
		t.Fatal("expected missing block_connection target to fail")
	}
}
