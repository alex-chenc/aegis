package model

import (
	"time"

	"github.com/google/uuid"
)

type AuthUser struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	Username            string     `gorm:"not null;uniqueIndex;size:64" json:"username"`
	PasswordHash        string     `gorm:"not null;type:text;default:''" json:"-"`
	ForcePasswordChange bool       `gorm:"not null;default:true;index" json:"force_password_change"`
	LastLoginAt         *time.Time `json:"last_login_at,omitempty"`
	CreatedAt           time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt           time.Time  `gorm:"not null" json:"updated_at"`
}

func (AuthUser) TableName() string {
	return "auth_users"
}

type AuthSession struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	User      AuthUser  `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	TokenHash string    `gorm:"not null;uniqueIndex;size:64" json:"-"`
	ExpiresAt time.Time `gorm:"not null;index" json:"expires_at"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
}

func (AuthSession) TableName() string {
	return "auth_sessions"
}
