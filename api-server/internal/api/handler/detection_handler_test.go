package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"api-server/internal/repository"
	"api-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupDetectionHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE sigma_rules (
			id TEXT PRIMARY KEY NOT NULL DEFAULT (lower(hex(randomblob(16)))),
			rule_id TEXT NOT NULL UNIQUE,
			title TEXT,
			description TEXT,
			content TEXT NOT NULL,
			status TEXT NOT NULL,
			mitre_id TEXT,
			severity TEXT,
			generated_by TEXT NOT NULL,
			version TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			activated_at DATETIME,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`).Error; err != nil {
		t.Fatalf("failed to create sigma_rules table: %v", err)
	}

	return db
}

func newImportRulesMultipartRequest(t *testing.T, yamlContent string) *http.Request {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", "rules.yml")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}

	if _, err := part.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write yaml content: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req
}

func TestImportRules_NoFileUploaded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if logger.Logger == nil {
		if err := logger.Init(&logger.Config{Level: "error", MaxSize: 10, MaxBackups: 1, MaxAge: 1, Compress: false}); err != nil {
			t.Fatalf("failed to init logger: %v", err)
		}
	}

	h := &DetectionHandler{}
	r := gin.New()
	r.POST("/import", h.ImportRules)

	req := httptest.NewRequest(http.MethodPost, "/import", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestImportRules_MissingRuleID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if logger.Logger == nil {
		if err := logger.Init(&logger.Config{Level: "error", MaxSize: 10, MaxBackups: 1, MaxAge: 1, Compress: false}); err != nil {
			t.Fatalf("failed to init logger: %v", err)
		}
	}

	db := setupDetectionHandlerTestDB(t)
	h := &DetectionHandler{sigmaRuleRepo: repository.NewSigmaRuleRepository(db)}
	r := gin.New()
	r.POST("/import", h.ImportRules)

	req := newImportRulesMultipartRequest(t, `title: Missing ID Rule\nlevel: high\n`)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestImportRules_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if logger.Logger == nil {
		if err := logger.Init(&logger.Config{Level: "error", MaxSize: 10, MaxBackups: 1, MaxAge: 1, Compress: false}); err != nil {
			t.Fatalf("failed to init logger: %v", err)
		}
	}

	db := setupDetectionHandlerTestDB(t)
	h := &DetectionHandler{sigmaRuleRepo: repository.NewSigmaRuleRepository(db)}
	r := gin.New()
	r.POST("/import", h.ImportRules)

	yamlContent := `title: First Rule
id: rule-1
description: first desc
level: medium
tags:
  - attack.t1059
---
title: Second Rule
id: rule-2
description: second desc
level: high
tags:
  - windows
`

	req := newImportRulesMultipartRequest(t, yamlContent)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if int(resp["total"].(float64)) != 2 {
		t.Fatalf("expected total=2, got %v", resp["total"])
	}
	if int(resp["imported"].(float64)) != 2 {
		t.Fatalf("expected imported=2, got %v", resp["imported"])
	}

	var count int64
	if err := db.Table("sigma_rules").Count(&count).Error; err != nil {
		t.Fatalf("failed to count rules: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rules in db, got %d", count)
	}
}

func setupListAlertsTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:list-alerts-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	for _, stmt := range []string{
		`CREATE TABLE hosts (
			id TEXT PRIMARY KEY,
			ip_address TEXT NOT NULL,
			hostname TEXT NOT NULL,
			os_type TEXT NOT NULL,
			agent_version TEXT NOT NULL,
			last_heartbeat_at DATETIME NOT NULL,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE sigma_rules (
			id TEXT PRIMARY KEY,
			rule_id TEXT NOT NULL,
			title TEXT,
			description TEXT,
			content TEXT NOT NULL,
			status TEXT NOT NULL,
			mitre_id TEXT,
			severity TEXT,
			generated_by TEXT NOT NULL,
			version TEXT NOT NULL,
			created_at DATETIME,
			activated_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE alerts (
			id TEXT PRIMARY KEY,
			alert_id TEXT UNIQUE NOT NULL,
			host_id TEXT NOT NULL,
			pid INTEGER NOT NULL,
			ppid INTEGER DEFAULT 0,
			command_line TEXT,
			process_tree TEXT,
			mitre_id TEXT NOT NULL,
			mitre_name TEXT,
			severity TEXT NOT NULL,
			description TEXT,
			llm_summary TEXT,
			dedupe_key TEXT NOT NULL,
			hit_count INTEGER NOT NULL DEFAULT 1,
			auto_blocked BOOLEAN NOT NULL DEFAULT FALSE,
			manual_blocked BOOLEAN NOT NULL DEFAULT FALSE,
			status TEXT NOT NULL,
			judgment_source TEXT,
			block_status TEXT,
			block_message TEXT,
			auto_dispose BOOLEAN NOT NULL DEFAULT FALSE,
			llm_disposal_strategy TEXT,
			rule_id TEXT,
			rule_title TEXT,
			first_seen_at DATETIME,
			last_seen_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("failed to create test table: %v", err)
		}
	}

	now := time.Date(2026, 4, 28, 1, 0, 0, 0, time.UTC)
	inserts := []string{
		`INSERT INTO hosts (id, ip_address, hostname, os_type, agent_version, last_heartbeat_at, created_at, updated_at)
		 VALUES ('11111111-1111-1111-1111-111111111111', '10.0.0.1', 'host-a', 'linux', 'test', ?, ?, ?)`,
		`INSERT INTO hosts (id, ip_address, hostname, os_type, agent_version, last_heartbeat_at, created_at, updated_at)
		 VALUES ('22222222-2222-2222-2222-222222222222', '10.0.0.2', 'host-b', 'linux', 'test', ?, ?, ?)`,
		`INSERT INTO hosts (id, ip_address, hostname, os_type, agent_version, last_heartbeat_at, created_at, updated_at)
		 VALUES ('33333333-3333-3333-3333-333333333333', '10.0.0.3', 'host-c', 'linux', 'test', ?, ?, ?)`,
	}
	for _, stmt := range inserts {
		if err := db.Exec(stmt, now, now, now).Error; err != nil {
			t.Fatalf("failed to insert host: %v", err)
		}
	}

	alerts := []struct {
		id       string
		alertID  string
		hostID   string
		lastSeen time.Time
	}{
		{"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "ALT-1", "11111111-1111-1111-1111-111111111111", time.Date(2026, 4, 28, 1, 5, 0, 0, time.UTC)},
		{"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "ALT-2", "22222222-2222-2222-2222-222222222222", time.Date(2026, 4, 28, 1, 30, 0, 0, time.UTC)},
		{"cccccccc-cccc-cccc-cccc-cccccccccccc", "ALT-3", "33333333-3333-3333-3333-333333333333", time.Date(2026, 4, 28, 3, 0, 0, 0, time.UTC)},
	}
	for _, alert := range alerts {
		if err := db.Exec(`INSERT INTO alerts (
			id, alert_id, host_id, pid, mitre_id, mitre_name, severity, description, dedupe_key,
			status, judgment_source, rule_title, first_seen_at, last_seen_at, created_at, updated_at
		) VALUES (?, ?, ?, 100, 'T1059', 'Command and Scripting Interpreter', 'high', 'test alert', ?, 'pending', 'system', 'Test Rule', ?, ?, ?, ?)`,
			alert.id, alert.alertID, alert.hostID, alert.alertID, alert.lastSeen, alert.lastSeen, alert.lastSeen, alert.lastSeen,
		).Error; err != nil {
			t.Fatalf("failed to insert alert: %v", err)
		}
	}

	return db
}

func TestListAlerts_FiltersByHostnamesAndTimeRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if logger.Logger == nil {
		if err := logger.Init(&logger.Config{Level: "error", MaxSize: 10, MaxBackups: 1, MaxAge: 1, Compress: false}); err != nil {
			t.Fatalf("failed to init logger: %v", err)
		}
	}

	db := setupListAlertsTestDB(t)
	h := &DetectionHandler{alertRepo: repository.NewAlertRepository(db)}
	r := gin.New()
	r.GET("/alerts", h.ListAlerts)

	req := httptest.NewRequest(http.MethodGet, "/alerts?page=1&pageSize=200&hostnames=host-a,host-b&start_time=2026-04-28T01:00:00Z&end_time=2026-04-28T02:00:00Z", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Data []struct {
				AlertID  string `json:"alert_id"`
				Hostname string `json:"hostname"`
			} `json:"data"`
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Data.Total != 2 {
		t.Fatalf("expected total=2, got %d", resp.Data.Total)
	}
	if len(resp.Data.Data) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(resp.Data.Data))
	}

	seen := map[string]string{}
	for _, alert := range resp.Data.Data {
		seen[alert.AlertID] = alert.Hostname
	}
	if seen["ALT-1"] != "host-a" || seen["ALT-2"] != "host-b" {
		t.Fatalf("expected ALT-1/ALT-2 for host-a/host-b, got %#v", seen)
	}
	if _, ok := seen["ALT-3"]; ok {
		t.Fatalf("did not expect ALT-3 outside filters, got %#v", seen)
	}
}
