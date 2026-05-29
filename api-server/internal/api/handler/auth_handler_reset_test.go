package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func createResetKey(t *testing.T, db *gorm.DB, resetKey string) {
	t.Helper()
	resetKeyJSON, _ := json.Marshal(resetKey)
	err := db.Create(&model.SystemConfig{
		Category:    "auth",
		ConfigKey:   "password_reset_key",
		ConfigValue: resetKeyJSON,
		Description: "Password reset key",
	}).Error
	if err != nil {
		t.Fatalf("failed to create reset key: %v", err)
	}
}

func newTestAuthRouterWithReset(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuthUser{}, &model.AuthSession{}); err != nil {
		t.Fatalf("failed to migrate auth tables: %v", err)
	}
	// Create system_configs table manually for SQLite compatibility
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS system_configs (
		id TEXT PRIMARY KEY,
		config_key TEXT NOT NULL UNIQUE,
		config_value TEXT NOT NULL,
		description TEXT,
		category TEXT NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("failed to create system_configs table: %v", err)
	}

	authSvc := service.NewAuthService(repository.NewAuthRepository(db), nil)
	authHandler := NewAuthHandler(authSvc, nil)

	router := gin.New()
	group := router.Group("/api/v1/auth")
	authHandler.RegisterRoutes(group)
	return router, db
}

func setupAdminWithPassword(t *testing.T, router *gin.Engine) {
	t.Helper()

	// Bootstrap
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap-login", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected bootstrap 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var bootstrap struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &bootstrap); err != nil {
		t.Fatalf("failed to decode bootstrap response: %v", err)
	}

	// Set credentials
	body := bytes.NewBufferString(`{"username":"admin","new_password":"OldPassword123!","confirm_password":"OldPassword123!"}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-credentials", body)
	req.Header.Set("Authorization", "Bearer "+bootstrap.Token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected change credentials 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestResetPasswordHandlerSuccess(t *testing.T) {
	router, db := newTestAuthRouterWithReset(t)
	setupAdminWithPassword(t, router)

	// Set reset key
	resetKey := "test-reset-key-12345"
	createResetKey(t, db, resetKey)

	// Reset password
	body := bytes.NewBufferString(`{"reset_key":"test-reset-key-12345","new_password":"Admin@123","confirm_password":"Admin@123"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected reset password 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Token               string `json:"token"`
		Username            string `json:"username"`
		ForcePasswordChange bool   `json:"force_password_change"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode reset response: %v", err)
	}
	if result.Token == "" {
		t.Fatalf("expected token in response")
	}
	if result.Username != "admin" {
		t.Fatalf("expected username 'admin', got %q", result.Username)
	}

	// Verify login with new password
	loginBody := bytes.NewBufferString(`{"username":"admin","password":"Admin@123"}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", loginBody)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected login with new password 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestResetPasswordHandlerInvalidKey(t *testing.T) {
	router, _ := newTestAuthRouterWithReset(t)
	setupAdminWithPassword(t, router)

	body := bytes.NewBufferString(`{"reset_key":"wrong-key","new_password":"Admin@123","confirm_password":"Admin@123"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected reset password 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestResetPasswordHandlerMismatchedPasswords(t *testing.T) {
	router, db := newTestAuthRouterWithReset(t)
	setupAdminWithPassword(t, router)

	resetKey := "test-reset-key-12345"
	createResetKey(t, db, resetKey)

	body := bytes.NewBufferString(`{"reset_key":"test-reset-key-12345","new_password":"Admin@123","confirm_password":"DifferentPassword123!"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected reset password 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestResetPasswordHandlerShortPassword(t *testing.T) {
	router, db := newTestAuthRouterWithReset(t)
	setupAdminWithPassword(t, router)

	resetKey := "test-reset-key-12345"
	createResetKey(t, db, resetKey)

	body := bytes.NewBufferString(`{"reset_key":"test-reset-key-12345","new_password":"short","confirm_password":"short"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected reset password 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestResetPasswordHandlerInvalidRequest(t *testing.T) {
	router, _ := newTestAuthRouterWithReset(t)

	body := bytes.NewBufferString(`{invalid json}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected reset password 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
