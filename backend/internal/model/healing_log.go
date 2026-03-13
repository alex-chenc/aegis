package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type AttemptDetail struct {
	Attempt         int       `json:"attempt"`
	ScriptVersionID string    `json:"script_version_id"`
	ErrorInput      string    `json:"error_input"`
	LLMFixSummary   string    `json:"llm_fix_summary"`
	ResultExitCode  int       `json:"result_exit_code"`
	ResultStderr    string    `json:"result_stderr"`
	Timestamp       time.Time `json:"timestamp"`
}

type AttemptsDetail []AttemptDetail

func (a AttemptsDetail) Value() (driver.Value, error) {
	return json.Marshal(a)
}

func (a *AttemptsDetail) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, a)
}

type HealingLog struct {
	ID                   uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OriginalTaskID       uuid.UUID      `gorm:"type:uuid;not null;index" json:"original_task_id"`
	RuleID               *uuid.UUID     `gorm:"type:uuid;index" json:"rule_id"`
	HostID               uuid.UUID      `gorm:"type:uuid;not null;index" json:"host_id"`
	VulnerabilityID      *uuid.UUID     `gorm:"type:uuid;index" json:"vulnerability_id"`
	ScriptType           string         `gorm:"type:varchar(20);not null" json:"script_type"`
	TriggerError         string         `gorm:"type:text;not null" json:"trigger_error"`
	TriggerExitCode      int            `gorm:"not null" json:"trigger_exit_code"`
	TotalAttempts        int            `gorm:"not null;default:0" json:"total_attempts"`
	MaxAttempts          int            `gorm:"not null;default:3" json:"max_attempts"`
	Status               string         `gorm:"type:varchar(20);not null;default:'healing';index" json:"status"`
	FinalScriptVersionID *uuid.UUID     `gorm:"type:uuid" json:"final_script_version_id"`
	AttemptsDetail       AttemptsDetail `gorm:"type:jsonb" json:"attempts_detail"`
	UserSuggestion       string         `gorm:"type:text" json:"user_suggestion"`
	LastError            string         `gorm:"type:text" json:"last_error"`
	StartedAt            time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"started_at"`
	FinishedAt           *time.Time     `json:"finished_at"`
	CreatedAt            time.Time      `gorm:"autoCreateTime" json:"created_at"`
}

func (HealingLog) TableName() string {
	return "self_healing_logs"
}
