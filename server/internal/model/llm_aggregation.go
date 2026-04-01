package model

import (
	"time"

	"github.com/google/uuid"
)

type LLMAggregation struct {
	ID               uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AggregationID    string     `gorm:"type:varchar(64);uniqueIndex;not null" json:"aggregation_id"`
	StartTime        time.Time  `gorm:"not null" json:"start_time"`
	EndTime          time.Time  `gorm:"not null" json:"end_time"`
	HostIDs          []string   `gorm:"type:text[]" json:"host_ids"`
	EventCount       int        `gorm:"default:0" json:"event_count"`
	AlertCount       int        `gorm:"default:0" json:"alert_count"`
	AIJudgedCount    int        `gorm:"default:0" json:"ai_judged_count"`
	AutoDisposeCount int        `gorm:"default:0" json:"auto_dispose_count"`
	LLMResponse      string     `gorm:"type:text" json:"llm_response"`
	Status           string     `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	Error            string     `gorm:"type:text" json:"error"`
	CreatedAt        time.Time  `gorm:"default:now()" json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at"`
}

func (LLMAggregation) TableName() string { return "llm_aggregations" }
