package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrBootstrapUnavailable = errors.New("bootstrap login is unavailable")
	ErrInvalidCredentials   = errors.New("invalid credentials")
	ErrInvalidToken         = errors.New("invalid token")
	ErrValidation           = errors.New("validation failed")
	ErrLoginRateLimited     = errors.New("login rate limited")
)

type AuthService struct {
	repo       *repository.AuthRepository
	redis      interface {
		IncrementLoginFail(username string) (int, error)
		GetLoginFailCount(username string) (int, error)
		GetLoginFailTTL(username string) (time.Duration, error)
		ClearLoginFail(username string) error
	}
	sessionTTL time.Duration
}

type AuthStatus struct {
	Initialized bool `json:"initialized"`
}

type AuthSessionResult struct {
	Token               string `json:"token"`
	Username            string `json:"username"`
	ForcePasswordChange bool   `json:"force_password_change"`
}

type ChangeCredentialsInput struct {
	Username        string
	NewPassword     string
	ConfirmPassword string
}

type ChangePasswordInput struct {
	CurrentPassword string
	NewPassword     string
	ConfirmPassword string
}

type AuthContext struct {
	UserID              string
	Username            string
	ForcePasswordChange bool
}

func NewAuthService(repo *repository.AuthRepository, redis interface {
	IncrementLoginFail(username string) (int, error)
	GetLoginFailCount(username string) (int, error)
	GetLoginFailTTL(username string) (time.Duration, error)
	ClearLoginFail(username string) error
}) *AuthService {
	return &AuthService{
		repo:       repo,
		redis:      redis,
		sessionTTL: 24 * time.Hour,
	}
}

func (s *AuthService) GetStatus() (*AuthStatus, error) {
	user, err := s.repo.GetUser()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &AuthStatus{Initialized: false}, nil
	}
	if err != nil {
		return nil, err
	}
	return &AuthStatus{Initialized: user.PasswordHash != "" && !user.ForcePasswordChange}, nil
}

func (s *AuthService) BootstrapLogin() (*AuthSessionResult, error) {
	user, err := s.repo.GetUser()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		user = &model.AuthUser{
			Username:            "admin",
			ForcePasswordChange: true,
		}
		if err := s.repo.CreateUser(user); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	if user.PasswordHash != "" || !user.ForcePasswordChange {
		return nil, ErrBootstrapUnavailable
	}

	return s.createSession(user)
}

type LoginRateLimitError struct {
	Remaining time.Duration
}

func (e *LoginRateLimitError) Error() string {
	return "login rate limited"
}

func (s *AuthService) Login(username, password string) (*AuthSessionResult, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, ErrInvalidCredentials
	}

	// Check rate limit
	if s.redis != nil {
		count, _ := s.redis.GetLoginFailCount(username)
		if count >= 3 {
			ttl, _ := s.redis.GetLoginFailTTL(username)
			return nil, &LoginRateLimitError{Remaining: ttl}
		}
	}

	user, err := s.repo.FindUserByUsername(username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if s.redis != nil {
				s.redis.IncrementLoginFail(username)
			}
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if user.PasswordHash == "" {
		if s.redis != nil {
			s.redis.IncrementLoginFail(username)
		}
		return nil, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		if s.redis != nil {
			s.redis.IncrementLoginFail(username)
		}
		return nil, ErrInvalidCredentials
	}

	// Clear fail counter on successful login
	if s.redis != nil {
		s.redis.ClearLoginFail(username)
	}

	now := time.Now()
	user.LastLoginAt = &now
	if err := s.repo.UpdateUser(user); err != nil {
		return nil, err
	}

	return s.createSession(user)
}

func (s *AuthService) ChangeCredentials(token string, input ChangeCredentialsInput) (*AuthSessionResult, error) {
	_, session, err := s.validateTokenWithSession(token)
	if err != nil {
		return nil, err
	}

	username := strings.TrimSpace(input.Username)
	if username == "" || len(username) > 64 {
		return nil, ErrValidation
	}
	if input.NewPassword != input.ConfirmPassword {
		return nil, ErrValidation
	}
	if len(input.NewPassword) < 8 {
		return nil, ErrValidation
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &session.User
	user.Username = username
	user.PasswordHash = string(hash)
	user.ForcePasswordChange = false
	now := time.Now()
	user.LastLoginAt = &now
	if err := s.repo.UpdateUser(user); err != nil {
		return nil, err
	}

	if err := s.repo.DeleteSessionByTokenHash(hashToken(token)); err != nil {
		return nil, err
	}
	return s.createSession(user)
}

func (s *AuthService) ValidateToken(token string) (*AuthContext, error) {
	authCtx, _, err := s.validateTokenWithSession(token)
	return authCtx, err
}

func (s *AuthService) Logout(token string) error {
	if token == "" {
		return ErrInvalidToken
	}
	return s.repo.DeleteSessionByTokenHash(hashToken(token))
}

func (s *AuthService) ResetPassword(resetKey, newPassword, confirmPassword string) (*AuthSessionResult, error) {
	storedKey, err := s.repo.GetPasswordResetKey()
	if err != nil {
		return nil, ErrInvalidToken
	}
	if subtle.ConstantTimeCompare([]byte(resetKey), []byte(storedKey)) != 1 {
		return nil, ErrInvalidToken
	}

	if newPassword != confirmPassword {
		return nil, ErrValidation
	}
	if len(newPassword) < 8 {
		return nil, ErrValidation
	}

	user, err := s.repo.GetUser()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user.PasswordHash = string(hash)
	user.ForcePasswordChange = false
	now := time.Now()
	user.LastLoginAt = &now
	if err := s.repo.UpdateUser(user); err != nil {
		return nil, err
	}

	// Invalidate all existing sessions for this user
	if err := s.repo.DeleteSessionsByUserID(user.ID); err != nil {
		return nil, err
	}

	newResetKey, err := randomToken()
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdatePasswordResetKey(newResetKey); err != nil {
		return nil, err
	}

	return s.createSession(user)
}

func (s *AuthService) ChangePassword(token string, input ChangePasswordInput) (*AuthSessionResult, error) {
	authCtx, session, err := s.validateTokenWithSession(token)
	if err != nil {
		return nil, err
	}

	if input.CurrentPassword == "" || input.NewPassword == "" {
		return nil, ErrValidation
	}
	if input.NewPassword != input.ConfirmPassword {
		return nil, ErrValidation
	}
	if len(input.NewPassword) < 8 {
		return nil, ErrValidation
	}
	if !regexp.MustCompile(`[a-zA-Z]`).MatchString(input.NewPassword) || !regexp.MustCompile(`[0-9]`).MatchString(input.NewPassword) {
		return nil, ErrValidation
	}
	if input.CurrentPassword == input.NewPassword {
		return nil, ErrValidation
	}

	user, err := s.repo.FindUserByUsername(authCtx.Username)
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.CurrentPassword)); err != nil {
		return nil, ErrInvalidCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user.PasswordHash = string(hash)
	user.ForcePasswordChange = false
	if err := s.repo.UpdateUser(user); err != nil {
		return nil, err
	}

	if err := s.repo.DeleteSessionsByUserID(session.UserID); err != nil {
		return nil, err
	}

	return s.createSession(user)
}

func (c *AuthContext) CanAccessBusinessAPI() bool {
	return !c.ForcePasswordChange
}

func (s *AuthService) validateTokenWithSession(token string) (*AuthContext, *model.AuthSession, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil, ErrInvalidToken
	}
	session, err := s.repo.FindSessionByTokenHash(hashToken(token), time.Now())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrInvalidToken
		}
		return nil, nil, err
	}

	return &AuthContext{
		UserID:              session.User.ID.String(),
		Username:            session.User.Username,
		ForcePasswordChange: session.User.ForcePasswordChange,
	}, session, nil
}

func (s *AuthService) createSession(user *model.AuthUser) (*AuthSessionResult, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	session := &model.AuthSession{
		UserID:    user.ID,
		TokenHash: hashToken(token),
		ExpiresAt: time.Now().Add(s.sessionTTL),
	}
	if err := s.repo.CreateSession(session); err != nil {
		return nil, err
	}

	return &AuthSessionResult{
		Token:               token,
		Username:            user.Username,
		ForcePasswordChange: user.ForcePasswordChange,
	}, nil
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
