package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type SystemConfig struct {
	ID          uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ConfigKey   string          `gorm:"type:varchar(200);uniqueIndex;not null" json:"config_key"`
	ConfigValue json.RawMessage `gorm:"type:jsonb;not null" json:"config_value"`
	Description string          `gorm:"type:text" json:"description"`
	Category    string          `gorm:"type:varchar(50);not null;index" json:"category"`
	CreatedAt   time.Time       `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time       `gorm:"autoUpdateTime" json:"updated_at"`
}

func (SystemConfig) TableName() string {
	return "system_configs"
}

// CommandAuditSettings 命令审计配置
type CommandAuditSettings struct {
	BlacklistEnabled bool `json:"blacklist_enabled"`
	AIEnabled        bool `json:"ai_enabled"`
	MaxRetry         int  `json:"max_retry"`
	DispatchCheck    bool `json:"dispatch_check"`
	AgentCheck       bool `json:"agent_check"`
}

func DefaultCommandAuditSettings() CommandAuditSettings {
	return CommandAuditSettings{
		BlacklistEnabled: true,
		AIEnabled:        true,
		MaxRetry:         3,
		DispatchCheck:    true,
		AgentCheck:       true,
	}
}
