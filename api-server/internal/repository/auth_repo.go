package repository

import (
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AuthRepository struct {
	db *gorm.DB
}

func NewAuthRepository(db *gorm.DB) *AuthRepository {
	return &AuthRepository{db: db}
}

func (r *AuthRepository) GetUser() (*model.AuthUser, error) {
	var user model.AuthUser
	if err := r.db.Order("created_at ASC").First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *AuthRepository) CreateUser(user *model.AuthUser) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	return r.db.Create(user).Error
}

func (r *AuthRepository) UpdateUser(user *model.AuthUser) error {
	return r.db.Save(user).Error
}

func (r *AuthRepository) FindUserByUsername(username string) (*model.AuthUser, error) {
	var user model.AuthUser
	if err := r.db.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *AuthRepository) CreateSession(session *model.AuthSession) error {
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}
	return r.db.Create(session).Error
}

func (r *AuthRepository) FindSessionByTokenHash(tokenHash string, now time.Time) (*model.AuthSession, error) {
	var session model.AuthSession
	err := r.db.Preload("User").
		Where("token_hash = ? AND expires_at > ?", tokenHash, now).
		First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *AuthRepository) DeleteSessionByTokenHash(tokenHash string) error {
	return r.db.Where("token_hash = ?", tokenHash).Delete(&model.AuthSession{}).Error
}

func (r *AuthRepository) DeleteExpiredSessions(now time.Time) error {
	return r.db.Where("expires_at <= ?", now).Delete(&model.AuthSession{}).Error
}
