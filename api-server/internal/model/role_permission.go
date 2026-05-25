package model

import "time"

type RolePermission struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID    string    `json:"user_id" gorm:"column:user_id;uniqueIndex;not null"`
	Role      string    `json:"role" gorm:"column:role;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (RolePermission) TableName() string {
	return "role_permissions"
}

const (
	RoleSecurityAnalyst   = "security_analyst"
	RoleSecurityDeveloper = "security_developer"
	RoleAdmin             = "admin"
)
