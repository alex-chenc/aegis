package middleware

import (
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

func newMiddlewareTestAuthService(t *testing.T) *service.AuthService {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuthUser{}, &model.AuthSession{}); err != nil {
		t.Fatalf("failed to migrate auth tables: %v", err)
	}
	return service.NewAuthService(repository.NewAuthRepository(db))
}

func TestAuthRequiredRejectsBusinessAPIWithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authSvc := newMiddlewareTestAuthService(t)
	router := gin.New()
	router.Use(AuthRequired(authSvc))
	router.GET("/api/v1/hosts", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthRequiredRejectsForcedPasswordSessionForBusinessAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authSvc := newMiddlewareTestAuthService(t)
	session, err := authSvc.BootstrapLogin()
	if err != nil {
		t.Fatalf("expected bootstrap login: %v", err)
	}

	router := gin.New()
	router.Use(AuthRequired(authSvc))
	router.GET("/api/v1/hosts", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	req.Header.Set("Authorization", "Bearer "+session.Token)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for forced password session, got %d: %s", rec.Code, rec.Body.String())
	}
}
