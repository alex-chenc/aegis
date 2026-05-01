package grpc_server

import (
	"fmt"
	"testing"
	"time"

	"server/internal/model"
	"server/internal/repository"
	pb "server/pkg/api/v1"
	"server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() {
	if logger.Logger == nil {
		logger.Logger, _ = zap.NewDevelopment()
	}
}

func setupAutoActionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.Exec(`CREATE TABLE block_policies (
		id TEXT PRIMARY KEY,
		mitre_id TEXT NOT NULL UNIQUE,
		mitre_name TEXT,
		enabled INTEGER NOT NULL DEFAULT 1,
		auto_block INTEGER NOT NULL DEFAULT 0,
		auto_dispose INTEGER NOT NULL DEFAULT 0,
		action TEXT NOT NULL DEFAULT 'kill_process',
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("failed to create block_policies: %v", err)
	}

	if err := db.Exec(`CREATE TABLE alerts (
		id TEXT PRIMARY KEY,
		alert_id TEXT NOT NULL UNIQUE,
		host_id TEXT NOT NULL,
		pid INTEGER NOT NULL,
		ppid INTEGER DEFAULT 0,
		command_line TEXT,
		process_tree TEXT,
		mitre_id TEXT NOT NULL,
		mitre_name TEXT,
		severity TEXT NOT NULL DEFAULT 'medium',
		description TEXT,
		llm_summary TEXT,
		dedupe_key TEXT NOT NULL,
		hit_count INTEGER DEFAULT 1,
		auto_blocked INTEGER DEFAULT 0,
		manual_blocked INTEGER DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'pending',
		judgment_source TEXT DEFAULT 'system',
		block_status TEXT,
		block_message TEXT,
		auto_dispose INTEGER DEFAULT 0,
		llm_disposal_strategy TEXT,
		rule_id TEXT,
		rule_title TEXT,
		first_seen_at DATETIME,
		last_seen_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("failed to create alerts: %v", err)
	}

	return db
}

func seedPolicyForTest(t *testing.T, db *gorm.DB, mitreID string, enabled, autoBlock, autoDispose bool) {
	t.Helper()
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
		"INSERT INTO block_policies (id, mitre_id, enabled, auto_block, auto_dispose, action, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))",
		uuid.New().String(), mitreID, e, ab, ad, "kill_process",
	).Error; err != nil {
		t.Fatalf("failed to seed policy: %v", err)
	}
}

func createTestAlert(db *gorm.DB, mitreID string) *model.Alert {
	alert := &model.Alert{
		ID:        uuid.New(),
		AlertID:   "ALT-" + uuid.New().String()[:8],
		HostID:    uuid.New(),
		PID:       100,
		MitreID:   mitreID,
		Severity:  "high",
		DedupeKey: "test-dedupe-" + uuid.New().String()[:8],
		HitCount:  1,
		Status:    "pending",
	}
	db.Create(alert)
	return alert
}

func TestCheckAutoActions_AutoBlockWhenEnabled(t *testing.T) {
	db := setupAutoActionTestDB(t)
	seedPolicyForTest(t, db, "T1059.004", true, true, false)
	alert := createTestAlert(db, "T1059.004")

	s := &GRPCServer{
		blockPolicyRepo: repository.NewBlockPolicyRepository(db),
		alertRepo:       repository.NewAlertRepository(db),
	}
	stream := &captureAgentStream{}
	s.agentConnections.Store(alert.HostID, &AgentConnection{
		HostID: alert.HostID,
		Stream: stream,
		Ctx:    t.Context(),
		Inbox:  make(chan *pb.CommandExecute, 1),
	})

	s.checkAutoActions(alert)

	if !alert.AutoBlocked {
		t.Fatal("expected AutoBlocked=true")
	}
	if alert.BlockStatus == nil || *alert.BlockStatus != "blocking" {
		t.Fatal("expected BlockStatus=blocking")
	}
	if len(stream.sent) != 1 || stream.sent[0].GetBlock() == nil {
		t.Fatal("expected auto-block command to be sent")
	}
}

func TestCheckAutoActions_NoAutoBlockWhenDisabled(t *testing.T) {
	db := setupAutoActionTestDB(t)
	seedPolicyForTest(t, db, "T1059.004", true, false, false)
	alert := createTestAlert(db, "T1059.004")

	s := &GRPCServer{
		blockPolicyRepo: repository.NewBlockPolicyRepository(db),
		alertRepo:       repository.NewAlertRepository(db),
	}

	s.checkAutoActions(alert)

	if alert.AutoBlocked {
		t.Fatal("expected AutoBlocked=false when AutoBlock disabled")
	}
}

func TestCheckAutoActions_AutoDisposeWhenEnabled(t *testing.T) {
	db := setupAutoActionTestDB(t)
	seedPolicyForTest(t, db, "T1059.004", true, false, true)
	alert := createTestAlert(db, "T1059.004")

	s := &GRPCServer{
		blockPolicyRepo: repository.NewBlockPolicyRepository(db),
		alertRepo:       repository.NewAlertRepository(db),
	}

	s.checkAutoActions(alert)

	if !alert.AutoDispose {
		t.Fatal("expected AutoDispose=true")
	}
	if alert.Status != "resolved" {
		t.Fatalf("expected Status=resolved, got %s", alert.Status)
	}
}

func TestCheckAutoActions_BothAutoBlockAndAutoDispose(t *testing.T) {
	db := setupAutoActionTestDB(t)
	seedPolicyForTest(t, db, "T1059.004", true, true, true)
	alert := createTestAlert(db, "T1059.004")

	s := &GRPCServer{
		blockPolicyRepo: repository.NewBlockPolicyRepository(db),
		alertRepo:       repository.NewAlertRepository(db),
	}

	s.checkAutoActions(alert)

	if !alert.AutoBlocked {
		t.Fatal("expected AutoBlocked=true")
	}
	if !alert.AutoDispose {
		t.Fatal("expected AutoDispose=true")
	}
	if alert.Status != "resolved" {
		t.Fatalf("expected Status=resolved, got %s", alert.Status)
	}
}

func TestCheckAutoActions_NoOpWhenPolicyDisabled(t *testing.T) {
	db := setupAutoActionTestDB(t)
	seedPolicyForTest(t, db, "T1059.004", false, true, true)
	alert := createTestAlert(db, "T1059.004")

	s := &GRPCServer{
		blockPolicyRepo: repository.NewBlockPolicyRepository(db),
		alertRepo:       repository.NewAlertRepository(db),
	}

	s.checkAutoActions(alert)

	if alert.AutoBlocked {
		t.Fatal("expected AutoBlocked=false when policy disabled")
	}
	if alert.AutoDispose {
		t.Fatal("expected AutoDispose=false when policy disabled")
	}
}
