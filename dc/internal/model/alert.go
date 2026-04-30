package model

import (
	"time"

	"github.com/google/uuid"
)

// Alert represents a security alert
type Alert struct {
	ID              uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AlertID        string    `gorm:"uniqueIndex;not null" json:"alert_id"`
	HostID         uuid.UUID `gorm:"index;not null" json:"host_id"`
	PID            int       `json:"pid"`
	PPID           int       `json:"ppid"`
	CommandLine    string    `gorm:"type:text" json:"command_line"`
	ProcessTree    string    `gorm:"type:text" json:"process_tree"`
	MitreID        string    `gorm:"index" json:"mitre_id"`
	MitreName      string    `json:"mitre_name"`
	Severity       string    `gorm:"index" json:"severity"`
	Description    string    `gorm:"type:text" json:"description"`
	DedupeKey      string    `gorm:"index" json:"dedupe_key"`
	HitCount       int       `json:"hit_count"`
	Status         string    `gorm:"index;default:pending" json:"status"`
	AutoBlocked    bool      `json:"auto_blocked"`
	ManualBlocked  bool      `gorm:"not null;default:false" json:"manual_blocked"`
	AutoDispose    bool      `json:"auto_dispose"`
	BlockStatus    *string   `json:"block_status,omitempty"`
	BlockMessage   *string   `json:"block_message,omitempty"`
	JudgmentSource string    `json:"judgment_source"`
	RuleID         string    `json:"rule_id"`
	FirstSeenAt    time.Time `json:"first_seen_at"`
	LastSeenAt     time.Time `json:"last_seen_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	ProcessName    string    `json:"process_name"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TableName returns the table name for Alert
func (Alert) TableName() string {
	return "alerts"
}
