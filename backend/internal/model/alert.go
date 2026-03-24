package model

import (
	"time"

	"github.com/google/uuid"
)

type Alert struct {
	ID            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AlertID       string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"alert_id"`
	HostID        uuid.UUID `gorm:"type:uuid;not null;index" json:"host_id"`
	PID           int       `gorm:"column:pid;not null" json:"pid"`
	MitreID       string    `gorm:"type:varchar(20);not null;index" json:"mitre_id"`
	MitreName     string    `gorm:"type:varchar(100)" json:"mitre_name"`
	Severity      string    `gorm:"type:varchar(20);not null;default:'medium'" json:"severity"`
	Description   string    `gorm:"type:text" json:"description"`
	LLMSummary    string    `gorm:"type:text;column:llm_summary" json:"llm_summary"`
	DedupeKey     string    `gorm:"type:varchar(256);not null;index" json:"dedupe_key"`
	HitCount      int       `gorm:"not null;default:1" json:"hit_count"`
	AutoBlocked   bool      `gorm:"not null;default:false" json:"auto_blocked"`
	ManualBlocked bool      `gorm:"not null;default:false" json:"manual_blocked"`
	Status        string    `gorm:"type:varchar(20);not null;default:'active';index" json:"status"`
	FirstSeenAt   time.Time `gorm:"default:now()" json:"first_seen_at"`
	LastSeenAt    time.Time `gorm:"default:now()" json:"last_seen_at"`
	CreatedAt     time.Time `gorm:"default:now()" json:"created_at"`
	UpdatedAt     time.Time `gorm:"default:now()" json:"updated_at"`
}

func (Alert) TableName() string { return "alerts" }
