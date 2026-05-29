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

func newTestAuthRouter(t *testing.T) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuthUser{}, &model.AuthSession{}, &model.RolePermission{}); err != nil {
		t.Fatalf("failed to migrate auth tables: %v", err)
	}

	roleRepo := repository.NewRoleRepo(db)
	authSvc := service.NewAuthService(repository.NewAuthRepository(db), nil)
	authHandler := NewAuthHandler(authSvc, roleRepo)

	router := gin.New()
	group := router.Group("/api/v1/auth")
	authHandler.RegisterRoutes(group)
	return router
}

func TestAuthHandlerBootstrapChangeCredentialsAndLogin(t *testing.T) {
	router := newTestAuthRouter(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap-login", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected bootstrap 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var bootstrap struct {
		Token               string `json:"token"`
		ForcePasswordChange bool   `json:"force_password_change"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &bootstrap); err != nil {
		t.Fatalf("failed to decode bootstrap response: %v", err)
	}
	if bootstrap.Token == "" || !bootstrap.ForcePasswordChange {
		t.Fatalf("unexpected bootstrap response: %+v", bootstrap)
	}

	body := bytes.NewBufferString(`{"username":"security-admin","new_password":"StrongerPassword123!","confirm_password":"StrongerPassword123!"}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-credentials", body)
	req.Header.Set("Authorization", "Bearer "+bootstrap.Token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected change credentials 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body = bytes.NewBufferString(`{"username":"security-admin","password":"StrongerPassword123!"}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthHandlerLoginAutoAssignsAdminRole(t *testing.T) {
	router := newTestAuthRouter(t)

	// Bootstrap and change credentials to create a user
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

	body := bytes.NewBufferString(`{"username":"security-admin","new_password":"StrongerPassword123!","confirm_password":"StrongerPassword123!"}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-credentials", body)
	req.Header.Set("Authorization", "Bearer "+bootstrap.Token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected change credentials 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Login — should auto-assign admin role since no role record exists
	body = bytes.NewBufferString(`{"username":"security-admin","password":"StrongerPassword123!"}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var loginResp struct {
		Token string `json:"token"`
		Role  string `json:"role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("failed to decode login response: %v", err)
	}
	if loginResp.Role != "admin" {
		t.Fatalf("expected role 'admin', got '%s'", loginResp.Role)
	}

	// Verify /me also returns admin role
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp.Token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected me 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var meResp struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &meResp); err != nil {
		t.Fatalf("failed to decode me response: %v", err)
	}
	if meResp.Role != "admin" {
		t.Fatalf("expected /me role 'admin', got '%s'", meResp.Role)
	}
}

func TestAuthHandlerBootstrapUnavailableAfterInitialization(t *testing.T) {
	router := newTestAuthRouter(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap-login", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected first bootstrap 200, got %d", rec.Code)
	}

	var bootstrap struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &bootstrap); err != nil {
		t.Fatalf("failed to decode bootstrap response: %v", err)
	}

	body := bytes.NewBufferString(`{"username":"security-admin","new_password":"StrongerPassword123!","confirm_password":"StrongerPassword123!"}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-credentials", body)
	req.Header.Set("Authorization", "Bearer "+bootstrap.Token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected change credentials 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap-login", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected bootstrap after initialization 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
