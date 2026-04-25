package model

import (
	"time"

	"github.com/google/uuid"
)

type ImageModelConfig struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	APIKeyEncrypted string     `gorm:"type:text;not null" json:"-"`
	APIKeyMasked    string     `gorm:"type:varchar(50);not null" json:"api_key_masked"`
	Provider        string     `gorm:"type:varchar(50);not null;default:'minimax'" json:"provider"`
	BaseURL         string     `gorm:"type:varchar(500);not null;default:'https://api.minimax.io/v1'" json:"base_url"`
	ModelName       string     `gorm:"type:varchar(100);not null;default:'image-01'" json:"model_name"`
	IsActive        bool       `gorm:"not null;default:true;index" json:"is_active"`
	LastTestStatus  *string    `gorm:"type:varchar(20)" json:"last_test_status"`
	LastTestAt      *time.Time `json:"last_test_at"`
	CreatedAt       time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (ImageModelConfig) TableName() string {
	return "image_model_configs"
}
