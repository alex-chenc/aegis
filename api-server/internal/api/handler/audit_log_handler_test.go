package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAuditLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}

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
			created_at DATETIME NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("failed to create script_audit_log table: %v", err)
	}

	return db
}

func seedAuditLogs(t *testing.T, db *gorm.DB, count int) []uuid.UUID {
	t.Helper()
	var ids []uuid.UUID
	for i := 0; i < count; i++ {
		id := uuid.New()
		log := model.ScriptAuditLog{
			ID:            id,
			TaskID:        "task-" + uuid.New().String(),
			ScriptType:    "check",
			ScriptContent: "#!/bin/bash\necho test",
			AuditSource:   "blacklist",
			Attempt:       1,
			Passed:        true,
			RiskLevel:     "safe",
			DurationMs:    100,
		}
		if err := db.Create(&log).Error; err != nil {
			t.Fatalf("failed to seed audit log: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func initTestLogger(t *testing.T) {
	t.Helper()
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
}

func TestDeleteLogs_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initTestLogger(t)

	db := setupAuditLogTestDB(t)
	repo := repository.NewAuditLogRepo(db)
	handler := NewAuditLogHandler(repo)

	ids := seedAuditLogs(t, db, 3)

	body, _ := json.Marshal(gin.H{"ids": []string{ids[0].String(), ids[1].String(), ids[2].String()}})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/settings/audit-logs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	c, r := gin.CreateTestContext(w)
	r.DELETE("/api/v1/settings/audit-logs", handler.DeleteLogs)
	c.Request = req
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp["data"].(map[string]interface{})
	if data["deleted"] != float64(3) {
		t.Fatalf("expected deleted=3, got %v", data["deleted"])
	}
}

func TestDeleteLogs_EmptyIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initTestLogger(t)

	db := setupAuditLogTestDB(t)
	repo := repository.NewAuditLogRepo(db)
	handler := NewAuditLogHandler(repo)

	body, _ := json.Marshal(gin.H{"ids": []string{}})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/settings/audit-logs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	c, r := gin.CreateTestContext(w)
	r.DELETE("/api/v1/settings/audit-logs", handler.DeleteLogs)
	c.Request = req
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteLogs_InvalidUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initTestLogger(t)

	db := setupAuditLogTestDB(t)
	repo := repository.NewAuditLogRepo(db)
	handler := NewAuditLogHandler(repo)

	body, _ := json.Marshal(gin.H{"ids": []string{"not-a-uuid"}})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/settings/audit-logs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	c, r := gin.CreateTestContext(w)
	r.DELETE("/api/v1/settings/audit-logs", handler.DeleteLogs)
	c.Request = req
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteLogs_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initTestLogger(t)

	db := setupAuditLogTestDB(t)
	repo := repository.NewAuditLogRepo(db)
	handler := NewAuditLogHandler(repo)

	randomID := uuid.New()
	body, _ := json.Marshal(gin.H{"ids": []string{randomID.String()}})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/settings/audit-logs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	c, r := gin.CreateTestContext(w)
	r.DELETE("/api/v1/settings/audit-logs", handler.DeleteLogs)
	c.Request = req
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp["data"].(map[string]interface{})
	if data["deleted"] != float64(0) {
		t.Fatalf("expected deleted=0, got %v", data["deleted"])
	}
}

func TestDeleteLogs_PartialMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initTestLogger(t)

	db := setupAuditLogTestDB(t)
	repo := repository.NewAuditLogRepo(db)
	handler := NewAuditLogHandler(repo)

	ids := seedAuditLogs(t, db, 1)
	nonExistentID := uuid.New()

	body, _ := json.Marshal(gin.H{"ids": []string{ids[0].String(), nonExistentID.String()}})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/settings/audit-logs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	c, r := gin.CreateTestContext(w)
	r.DELETE("/api/v1/settings/audit-logs", handler.DeleteLogs)
	c.Request = req
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp["data"].(map[string]interface{})
	if data["deleted"] != float64(1) {
		t.Fatalf("expected deleted=1, got %v", data["deleted"])
	}
}
