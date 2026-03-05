package model

import "time"

type LLMConfig struct {
	ID              string     `json:"id"`
	APIKeyEncrypted string     `json:"-"`
	APIKeyMasked    string     `json:"api_key_masked"`
	BaseURL         string     `json:"base_url"`
	ModelName       string     `json:"model_name"`
	IsActive        bool       `json:"is_active"`
	LastTestStatus  *string    `json:"last_test_status"`
	LastTestAt      *time.Time `json:"last_test_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
