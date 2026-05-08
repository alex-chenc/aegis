package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ScriptAuditLog struct {
	ID            uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskID        string          `gorm:"type:varchar(100);index" json:"task_id"`
	RuleID        string          `gorm:"type:varchar(100)" json:"rule_id"`
	ScriptType    string          `gorm:"type:varchar(50)" json:"script_type"`
	ScriptContent string          `gorm:"type:text" json:"script_content"`
	AuditSource   string          `gorm:"type:varchar(20)" json:"audit_source"`
	Attempt       int             `json:"attempt"`
	Passed        bool            `json:"passed"`
	RiskLevel     string          `gorm:"type:varchar(20)" json:"risk_level"`
	BlacklistHits json.RawMessage `gorm:"type:jsonb" json:"blacklist_hits"`
	AIAnalysis    json.RawMessage `gorm:"type:jsonb" json:"ai_analysis"`
	ErrorMsg      string          `gorm:"type:text" json:"error_msg"`
	DurationMs    int64           `gorm:"type:bigint" json:"duration_ms"`
	CreatedAt     time.Time       `gorm:"autoCreateTime;index" json:"created_at"`
}

func (ScriptAuditLog) TableName() string {
	return "script_audit_log"
}
