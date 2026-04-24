package model

import (
	"time"

	"github.com/google/uuid"
)

type LLMConfig struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	APIKeyEncrypted string     `gorm:"type:text;not null" json:"-"`
	APIKeyMasked    string     `gorm:"type:varchar(50);not null" json:"api_key_masked"`
	Provider        string     `gorm:"type:varchar(50);not null;default:'custom'" json:"provider"`
	BaseURL         string     `gorm:"type:varchar(500);not null" json:"base_url"`
	ModelName       string     `gorm:"type:varchar(100);not null;default:'qwen-plus'" json:"model_name"`
	IsActive        bool       `gorm:"not null;default:true;index" json:"is_active"`
	LastTestStatus  *string    `gorm:"type:varchar(20)" json:"last_test_status"`
	LastTestAt      *time.Time `json:"last_test_at"`
	CreatedAt       time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (LLMConfig) TableName() string {
	return "llm_configs"
}
