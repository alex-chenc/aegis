package model

import (
	"time"

	"github.com/google/uuid"
)

type RuntimeEvent struct {
	ID            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	EventID       string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"event_id"`
	HostID        uuid.UUID `gorm:"type:uuid;not null;index" json:"host_id"`
	EventType     string    `gorm:"type:varchar(32);not null;index" json:"event_type"`
	EventData     string    `gorm:"type:jsonb;not null" json:"event_data"`
	MatchedRuleID string    `gorm:"type:varchar(128)" json:"matched_rule_id"`
	RuleTitle     string    `gorm:"type:varchar(255)" json:"rule_title"`
	MitreID       string    `gorm:"type:varchar(20)" json:"mitre_id"`
	Severity      string    `gorm:"type:varchar(16)" json:"severity"`
	PID           int       `gorm:"column:pid" json:"pid"`
	CommandLine   string    `gorm:"type:text" json:"command_line"`
	Timestamp     int64     `gorm:"not null;index" json:"timestamp"`
	CreatedAt     time.Time `gorm:"default:now()" json:"created_at"`
	Aggregated    bool      `gorm:"default:false;index" json:"aggregated"`
}

func (RuntimeEvent) TableName() string { return "runtime_events" }
