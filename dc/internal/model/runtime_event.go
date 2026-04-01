package model

import (
	"time"

	"github.com/google/uuid"
)

// RuntimeEvent represents a security event from an agent
type RuntimeEvent struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	EventID       string    `gorm:"uniqueIndex;not null" json:"event_id"`
	HostID        uuid.UUID `gorm:"index;not null" json:"host_id"`
	EventType     string    `gorm:"index" json:"event_type"`
	EventData     string    `gorm:"type:text" json:"event_data"`
	MatchedRuleID string    `gorm:"index" json:"matched_rule_id"`
	MitreID       string    `gorm:"index" json:"mitre_id"`
	Severity      string    `gorm:"index" json:"severity"`
	PID           int       `json:"pid"`
	CommandLine   string    `gorm:"type:text" json:"command_line"`
	ProcessName   string    `json:"process_name"`
	Timestamp     int64     `json:"timestamp"`
	Aggregated    bool      `gorm:"default:false" json:"aggregated"`
	CreatedAt     time.Time `json:"created_at"`
}

// TableName returns the table name for RuntimeEvent
func (RuntimeEvent) TableName() string {
	return "runtime_events"
}

// LLMAnalysisResult represents the result of LLM analysis
type LLMAnalysisResult struct {
	Summary         string `json:"summary"`
	Severity        string `json:"severity"`
	MitreTechnique  string `json:"mitre_technique"`
	MatchedRule     string `json:"matched_rule"`
	AnalysisDetails string `json:"analysis_details"`
}
