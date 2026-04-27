package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
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
)

type AuthService struct {
	repo       *repository.AuthRepository
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

type AuthContext struct {
	UserID              string
	Username            string
	ForcePasswordChange bool
}

func NewAuthService(repo *repository.AuthRepository) *AuthService {
	return &AuthService{
		repo:       repo,
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

func (s *AuthService) Login(username, password string) (*AuthSessionResult, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, ErrInvalidCredentials
	}

	user, err := s.repo.FindUserByUsername(username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if user.PasswordHash == "" {
		return nil, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
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
