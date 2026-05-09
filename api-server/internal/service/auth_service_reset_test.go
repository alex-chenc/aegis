package service

import (
	"encoding/json"
	"testing"

	"api-server/internal/model"
	"api-server/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestAuthServiceWithResetKey(t *testing.T) (*AuthService, *gorm.DB) {
	t.Helper()

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

	repo := repository.NewAuthRepository(db)
	svc := NewAuthService(repo, nil)
	return svc, db
}

func TestResetPasswordWithValidKey(t *testing.T) {
	svc, db := newTestAuthServiceWithResetKey(t)

	// Create admin user with a password
	session, err := svc.BootstrapLogin()
	if err != nil {
		t.Fatalf("expected bootstrap login to succeed: %v", err)
	}
	_, err = svc.ChangeCredentials(session.Token, ChangeCredentialsInput{
		Username:        "admin",
		NewPassword:     "OldPassword123!",
		ConfirmPassword: "OldPassword123!",
	})
	if err != nil {
		t.Fatalf("expected credential change to succeed: %v", err)
	}

	// Set reset key in system_configs
	resetKey := "test-reset-key-12345"
	resetKeyJSON, _ := json.Marshal(resetKey)
	err = db.Create(&model.SystemConfig{
		Category:    "auth",
		ConfigKey:   "password_reset_key",
		ConfigValue: resetKeyJSON,
		Description: "Password reset key",
	}).Error
	if err != nil {
		t.Fatalf("failed to create reset key: %v", err)
	}

	// Reset password
	result, err := svc.ResetPassword(resetKey, "Admin@123", "Admin@123")
	if err != nil {
		t.Fatalf("expected password reset to succeed: %v", err)
	}
	if result.Username != "admin" {
		t.Fatalf("expected username 'admin', got %q", result.Username)
	}
	if result.Token == "" {
		t.Fatalf("expected token to be returned")
	}
	if result.ForcePasswordChange {
		t.Fatalf("expected force_password_change to be false")
	}

	// Verify new password works
	login, err := svc.Login("admin", "Admin@123")
	if err != nil {
		t.Fatalf("expected login with new password to succeed: %v", err)
	}
	if login.Token == "" {
		t.Fatalf("expected login token")
	}

	// Verify old password no longer works
	_, err = svc.Login("admin", "OldPassword123!")
	if err != ErrInvalidCredentials {
		t.Fatalf("expected old password to fail, got %v", err)
	}
}

func TestResetPasswordWithInvalidKey(t *testing.T) {
	svc, _ := newTestAuthServiceWithResetKey(t)

	// Create admin user
	session, err := svc.BootstrapLogin()
	if err != nil {
		t.Fatalf("expected bootstrap login to succeed: %v", err)
	}
	_, err = svc.ChangeCredentials(session.Token, ChangeCredentialsInput{
		Username:        "admin",
		NewPassword:     "OldPassword123!",
		ConfirmPassword: "OldPassword123!",
	})
	if err != nil {
		t.Fatalf("expected credential change to succeed: %v", err)
	}

	// Try reset with invalid key
	_, err = svc.ResetPassword("invalid-key", "Admin@123", "Admin@123")
	if err != ErrInvalidToken {
		t.Fatalf("expected invalid token error, got %v", err)
	}
}

func TestResetPasswordWithMismatchedPasswords(t *testing.T) {
	svc, db := newTestAuthServiceWithResetKey(t)

	// Create admin user
	session, err := svc.BootstrapLogin()
	if err != nil {
		t.Fatalf("expected bootstrap login to succeed: %v", err)
	}
	_, err = svc.ChangeCredentials(session.Token, ChangeCredentialsInput{
		Username:        "admin",
		NewPassword:     "OldPassword123!",
		ConfirmPassword: "OldPassword123!",
	})
	if err != nil {
		t.Fatalf("expected credential change to succeed: %v", err)
	}

	// Set reset key
	resetKey := "test-reset-key-12345"
	resetKeyJSON, _ := json.Marshal(resetKey)
	err = db.Create(&model.SystemConfig{
		Category:    "auth",
		ConfigKey:   "password_reset_key",
		ConfigValue: resetKeyJSON,
	}).Error
	if err != nil {
		t.Fatalf("failed to create reset key: %v", err)
	}

	// Try reset with mismatched passwords
	_, err = svc.ResetPassword(resetKey, "Admin@123", "DifferentPassword123!")
	if err != ErrValidation {
		t.Fatalf("expected validation error for mismatched passwords, got %v", err)
	}
}

func TestResetPasswordWithShortPassword(t *testing.T) {
	svc, db := newTestAuthServiceWithResetKey(t)

	// Create admin user
	session, err := svc.BootstrapLogin()
	if err != nil {
		t.Fatalf("expected bootstrap login to succeed: %v", err)
	}
	_, err = svc.ChangeCredentials(session.Token, ChangeCredentialsInput{
		Username:        "admin",
		NewPassword:     "OldPassword123!",
		ConfirmPassword: "OldPassword123!",
	})
	if err != nil {
		t.Fatalf("expected credential change to succeed: %v", err)
	}

	// Set reset key
	resetKey := "test-reset-key-12345"
	resetKeyJSON, _ := json.Marshal(resetKey)
	err = db.Create(&model.SystemConfig{
		Category:    "auth",
		ConfigKey:   "password_reset_key",
		ConfigValue: resetKeyJSON,
	}).Error
	if err != nil {
		t.Fatalf("failed to create reset key: %v", err)
	}

	// Try reset with short password
	_, err = svc.ResetPassword(resetKey, "short", "short")
	if err != ErrValidation {
		t.Fatalf("expected validation error for short password, got %v", err)
	}
}

func TestResetPasswordKeyChangesAfterUse(t *testing.T) {
	svc, db := newTestAuthServiceWithResetKey(t)

	// Create admin user
	session, err := svc.BootstrapLogin()
	if err != nil {
		t.Fatalf("expected bootstrap login to succeed: %v", err)
	}
	_, err = svc.ChangeCredentials(session.Token, ChangeCredentialsInput{
		Username:        "admin",
		NewPassword:     "OldPassword123!",
		ConfirmPassword: "OldPassword123!",
	})
	if err != nil {
		t.Fatalf("expected credential change to succeed: %v", err)
	}

	// Set reset key
	resetKey := "test-reset-key-12345"
	resetKeyJSON, _ := json.Marshal(resetKey)
	err = db.Create(&model.SystemConfig{
		Category:    "auth",
		ConfigKey:   "password_reset_key",
		ConfigValue: resetKeyJSON,
	}).Error
	if err != nil {
		t.Fatalf("failed to create reset key: %v", err)
	}

	// Reset password
	_, err = svc.ResetPassword(resetKey, "Admin@123", "Admin@123")
	if err != nil {
		t.Fatalf("expected password reset to succeed: %v", err)
	}

	// Try to reuse the same key
	_, err = svc.ResetPassword(resetKey, "AnotherPassword123!", "AnotherPassword123!")
	if err != ErrInvalidToken {
		t.Fatalf("expected reused key to fail, got %v", err)
	}
}

func TestResetPasswordWhenNoAdminExists(t *testing.T) {
	svc, db := newTestAuthServiceWithResetKey(t)

	// Set reset key without creating admin user
	resetKey := "test-reset-key-12345"
	resetKeyJSON, _ := json.Marshal(resetKey)
	err := db.Create(&model.SystemConfig{
		Category:    "auth",
		ConfigKey:   "password_reset_key",
		ConfigValue: resetKeyJSON,
	}).Error
	if err != nil {
		t.Fatalf("failed to create reset key: %v", err)
	}

	// Try reset when no admin exists
	_, err = svc.ResetPassword(resetKey, "Admin@123", "Admin@123")
	if err != ErrInvalidCredentials {
		t.Fatalf("expected invalid credentials error when no admin exists, got %v", err)
	}
}
