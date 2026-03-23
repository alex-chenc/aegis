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

	"aegis-system/internal/repository"
	"aegis-system/pkg/logger"

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
