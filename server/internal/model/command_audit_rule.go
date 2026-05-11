package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// StringArray is a custom type for JSONB string arrays
type StringArray []string

func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return "[]", nil
	}
	return json.Marshal(a)
}

func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = []string{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("StringArray.Scan: expected []byte, got %T", value)
	}
	return json.Unmarshal(bytes, a)
}

type CommandAuditRule struct {
	ID        uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name      string      `gorm:"type:varchar(200);not null" json:"name"`
	RuleType  string      `gorm:"type:varchar(20);not null;default:hard_block" json:"rule_type"`
	MatchType string      `gorm:"type:varchar(20);not null;default:regex" json:"match_type"`
	Pattern   string      `gorm:"type:text;not null" json:"pattern"`
	Category  string      `gorm:"type:varchar(50);not null;default:system" json:"category"`
	Severity  string      `gorm:"type:varchar(20);not null;default:high" json:"severity"`
	AppliesTo StringArray `gorm:"type:jsonb;not null;default:'[\"all\"]'" json:"applies_to"`
	IsEnabled bool        `gorm:"not null;default:true" json:"is_enabled"`
	CreatedAt time.Time   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time   `gorm:"autoUpdateTime" json:"updated_at"`
}

func (CommandAuditRule) TableName() string {
	return "command_audit_rules"
}
