package model

import (
	"time"

	"github.com/google/uuid"
)

type TaskLog struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskGroupID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"task_group_id"`
	RuleID          *uuid.UUID `gorm:"type:uuid;index" json:"rule_id"`
	HostID          uuid.UUID  `gorm:"type:uuid;not null;index" json:"host_id"`
	VulnerabilityID *uuid.UUID `gorm:"type:uuid;index" json:"vulnerability_id"`
	TaskType        string     `gorm:"type:varchar(20);not null" json:"task_type"`
	Status          string     `gorm:"type:varchar(20);not null;index" json:"status"`
	ScriptContent   *string    `gorm:"type:text" json:"script_content"`
	ScriptVersion   *int       `json:"script_version"`
	AttemptNo       int        `gorm:"not null;default:1" json:"attempt_no"`
	MaxRounds       int        `gorm:"not null;default:1" json:"max_rounds"`
	Stdout          *string    `gorm:"type:text" json:"stdout"`
	Stderr          *string    `gorm:"type:text" json:"stderr"`
	ExitCode        *int       `json:"exit_code"`
	HealingID       *uuid.UUID `gorm:"type:uuid" json:"healing_id"`
	StartedAt       *time.Time `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at"`
	CreatedAt       time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

func (TaskLog) TableName() string {
	return "task_logs"
}
