package model

import (
	"time"

	"github.com/google/uuid"
)

type SigmaRule struct {
	ID          uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	RuleID      string     `gorm:"type:varchar(128);uniqueIndex;not null" json:"rule_id"`
	Title       string     `gorm:"type:varchar(256)" json:"title"`
	Description string     `gorm:"type:text" json:"description"`
	Content     string     `gorm:"type:text;not null" json:"content"`
	Status      string     `gorm:"type:varchar(20);not null;default:'pending';index" json:"status"`
	MitreID     string     `gorm:"type:varchar(20);index" json:"mitre_id"`
	Severity    string     `gorm:"type:varchar(20)" json:"severity"`
	GeneratedBy string     `gorm:"type:varchar(20);not null;default:'llm'" json:"generated_by"`
	Version     string     `gorm:"type:varchar(20);not null;default:'1.0'" json:"version"`
	CreatedAt   time.Time  `gorm:"default:now()" json:"created_at"`
	ActivatedAt *time.Time `json:"activated_at"`
	UpdatedAt   time.Time  `gorm:"default:now()" json:"updated_at"`
}

func (SigmaRule) TableName() string { return "sigma_rules" }
