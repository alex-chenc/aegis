package service

import (
	"testing"

	"api-server/internal/model"
	"api-server/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestAuthService(t *testing.T) *AuthService {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuthUser{}, &model.AuthSession{}); err != nil {
		t.Fatalf("failed to migrate auth tables: %v", err)
	}

	return NewAuthService(repository.NewAuthRepository(db))
}

func TestAuthServiceBootstrapLoginRequiresCredentialChange(t *testing.T) {
	svc := newTestAuthService(t)

	status, err := svc.GetStatus()
	if err != nil {
		t.Fatalf("expected status to load: %v", err)
	}
	if status.Initialized {
		t.Fatalf("expected fresh database to be uninitialized")
	}

	session, err := svc.BootstrapLogin()
	if err != nil {
		t.Fatalf("expected bootstrap login to succeed: %v", err)
	}
	if session.Token == "" {
		t.Fatalf("expected bootstrap login to return token")
	}
	if !session.ForcePasswordChange {
		t.Fatalf("expected bootstrap session to force password change")
	}

	retrySession, err := svc.BootstrapLogin()
	if err != nil {
		t.Fatalf("expected bootstrap login to remain available before initialization: %v", err)
	}
	if retrySession.Token == "" || !retrySession.ForcePasswordChange {
		t.Fatalf("expected retry bootstrap session to force password change, got %+v", retrySession)
	}
}

func TestAuthServiceBootstrapUnavailableAfterCredentialChange(t *testing.T) {
	svc := newTestAuthService(t)

	session, err := svc.BootstrapLogin()
	if err != nil {
		t.Fatalf("expected bootstrap login to succeed: %v", err)
	}
	if _, err := svc.ChangeCredentials(session.Token, ChangeCredentialsInput{
		Username:        "security-admin",
		NewPassword:     "StrongerPassword123!",
		ConfirmPassword: "StrongerPassword123!",
	}); err != nil {
		t.Fatalf("expected credential change to succeed: %v", err)
	}

	if _, err := svc.BootstrapLogin(); err != ErrBootstrapUnavailable {
		t.Fatalf("expected bootstrap login to be unavailable after initialization, got %v", err)
	}
}

func TestAuthServiceChangeCredentialsEnablesPasswordLogin(t *testing.T) {
	svc := newTestAuthService(t)

	session, err := svc.BootstrapLogin()
	if err != nil {
		t.Fatalf("expected bootstrap login to succeed: %v", err)
	}

	updated, err := svc.ChangeCredentials(session.Token, ChangeCredentialsInput{
		Username:        "security-admin",
		NewPassword:     "StrongerPassword123!",
		ConfirmPassword: "StrongerPassword123!",
	})
	if err != nil {
		t.Fatalf("expected credential change to succeed: %v", err)
	}
	if updated.ForcePasswordChange {
		t.Fatalf("expected credential change to clear force password flag")
	}
	if updated.Username != "security-admin" {
		t.Fatalf("expected username to change, got %q", updated.Username)
	}
	if _, err := svc.ValidateToken(session.Token); err != ErrInvalidToken {
		t.Fatalf("expected bootstrap token to be revoked after credential change, got %v", err)
	}

	login, err := svc.Login("security-admin", "StrongerPassword123!")
	if err != nil {
		t.Fatalf("expected password login to succeed: %v", err)
	}
	if login.Token == "" || login.ForcePasswordChange {
		t.Fatalf("expected normal login token without force change, got %+v", login)
	}

	if _, err := svc.Login("security-admin", "wrong-password"); err != ErrInvalidCredentials {
		t.Fatalf("expected invalid credentials for wrong password, got %v", err)
	}
}

func TestAuthServiceForcedSessionCannotAccessBusinessAPIs(t *testing.T) {
	svc := newTestAuthService(t)

	session, err := svc.BootstrapLogin()
	if err != nil {
		t.Fatalf("expected bootstrap login to succeed: %v", err)
	}

	authCtx, err := svc.ValidateToken(session.Token)
	if err != nil {
		t.Fatalf("expected token to validate: %v", err)
	}
	if !authCtx.ForcePasswordChange {
		t.Fatalf("expected forced password change context")
	}
	if authCtx.CanAccessBusinessAPI() {
		t.Fatalf("expected forced password change session to be blocked from business APIs")
	}
}
