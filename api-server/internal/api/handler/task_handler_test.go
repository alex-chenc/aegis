package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/internal/service"
	"api-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTaskHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

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
			stdout TEXT NULL,
			stderr TEXT NULL,
			exit_code INTEGER NULL,
			started_at DATETIME NULL,
			finished_at DATETIME NULL,
			created_at DATETIME NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("failed to create task_logs table: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE hosts (
			id TEXT PRIMARY KEY,
			ip_address TEXT NOT NULL,
			hostname TEXT NOT NULL,
			os_type TEXT NOT NULL,
			agent_version TEXT NOT NULL,
			last_heartbeat_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("failed to create hosts table: %v", err)
	}

	return db
}

func TestGetTaskLogs_NilRuleID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	if logger.Logger == nil {
		err := logger.Init(&logger.Config{
			Level:      "error",
			MaxSize:    10,
			MaxBackups: 1,
			MaxAge:     1,
			Compress:   false,
		})
		if err != nil {
			t.Fatalf("failed to init logger: %v", err)
		}
	}

	db := setupTaskHandlerTestDB(t)
	taskLogRepo := repository.NewTaskLogRepository(db)
	hostRepo := repository.NewHostRepository(db)
	taskService := service.NewTaskService(taskLogRepo, hostRepo, nil, nil, nil, nil)

	taskGroupID := uuid.New()
	hostID := uuid.New()
	vulnID := uuid.New()
	taskID := uuid.New()

	insertSQL := `
		INSERT INTO task_logs (
			id, task_group_id, rule_id, host_id, vulnerability_id, task_type, status, created_at
		) VALUES (?, ?, NULL, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`
	if err := db.Exec(insertSQL, taskID.String(), taskGroupID.String(), hostID.String(), vulnID.String(), "check", "success").Error; err != nil {
		t.Fatalf("failed to insert task log: %v", err)
	}

	handler := &TaskHandler{
		taskService: taskService,
		taskLogRepo: taskLogRepo,
	}

	router := gin.Default()
	router.GET("/tasks/:id/logs", handler.GetTaskLogs)

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+taskGroupID.String()+"/logs", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestGetTaskLogs_AuditBlocked(t *testing.T) {
	gin.SetMode(gin.TestMode)

	if logger.Logger == nil {
		err := logger.Init(&logger.Config{
			Level:      "error",
			MaxSize:    10,
			MaxBackups: 1,
			MaxAge:     1,
			Compress:   false,
		})
		if err != nil {
			t.Fatalf("failed to init logger: %v", err)
		}
	}

	db := setupTaskHandlerTestDB(t)

	// Create script_audit_log table
	if err := db.Exec(`
		CREATE TABLE script_audit_log (
			id TEXT PRIMARY KEY,
			task_id TEXT,
			rule_id TEXT,
			script_type TEXT,
			script_content TEXT,
			audit_source TEXT,
			attempt INTEGER,
			passed BOOLEAN,
			risk_level TEXT,
			blacklist_hits TEXT,
			ai_analysis TEXT,
			error_msg TEXT,
			duration_ms INTEGER,
			created_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("failed to create script_audit_log table: %v", err)
	}

	taskLogRepo := repository.NewTaskLogRepository(db)
	hostRepo := repository.NewHostRepository(db)
	auditLogRepo := repository.NewAuditLogRepo(db)
	taskService := service.NewTaskService(taskLogRepo, hostRepo, nil, nil, nil, nil)

	taskGroupID := uuid.New()
	hostID := uuid.New()
	taskID := uuid.New()

	// Insert AUDIT_BLOCKED task
	insertSQL := `
		INSERT INTO task_logs (
			id, task_group_id, rule_id, host_id, task_type, status, stderr, created_at
		) VALUES (?, ?, NULL, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`
	blockReason := "脚本存在恶意命令，下发已阻止。\n命中规则：\n  1. [critical] curl管道执行 (第5行, 匹配: curl | bash)"
	if err := db.Exec(insertSQL, taskID.String(), taskGroupID.String(), hostID.String(), "check", "AUDIT_BLOCKED", blockReason).Error; err != nil {
		t.Fatalf("failed to insert task log: %v", err)
	}

	// Insert audit log with blacklist hits
	hits := []map[string]interface{}{
		{"rule_name": "curl管道执行", "severity": "critical", "line_number": 5, "matched_text": "curl | bash"},
	}
	hitsJSON, _ := json.Marshal(hits)
	auditLog := &model.ScriptAuditLog{
		TaskID:        taskID.String(),
		ScriptType:    "all",
		AuditSource:   "dispatch",
		Attempt:       1,
		Passed:        false,
		RiskLevel:     "critical",
		BlacklistHits: hitsJSON,
	}
	if err := auditLogRepo.Create(auditLog); err != nil {
		t.Fatalf("failed to create audit log: %v", err)
	}

	handler := &TaskHandler{
		taskService:  taskService,
		taskLogRepo:  taskLogRepo,
		auditLogRepo: auditLogRepo,
	}

	router := gin.Default()
	router.GET("/tasks/:id/logs", handler.GetTaskLogs)

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+taskGroupID.String()+"/logs", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	// Parse response and verify audit_info is populated
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	data, ok := resp["data"].([]interface{})
	if !ok || len(data) == 0 {
		t.Fatalf("expected data array with at least 1 item")
	}
	task := data[0].(map[string]interface{})
	auditInfo, ok := task["audit_info"]
	if !ok || auditInfo == nil {
		t.Fatalf("expected audit_info to be populated for AUDIT_BLOCKED task")
	}
	auditMap := auditInfo.(map[string]interface{})
	if auditMap["error_message"] != blockReason {
		t.Errorf("expected error_message to be block reason, got %v", auditMap["error_message"])
	}
	hitRules, ok := auditMap["hit_rules"].([]interface{})
	if !ok || len(hitRules) == 0 {
		t.Fatalf("expected hit_rules to have at least 1 entry")
	}
	hitRule := hitRules[0].(map[string]interface{})
	if hitRule["rule_name"] != "curl管道执行" {
		t.Errorf("expected rule_name 'curl管道执行', got %v", hitRule["rule_name"])
	}
	if hitRule["severity"] != "critical" {
		t.Errorf("expected severity 'critical', got %v", hitRule["severity"])
	}
}
