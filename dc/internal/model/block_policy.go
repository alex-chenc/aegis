package model

import (
	"time"
)

// BlockPolicy represents a blocking policy for MITRE techniques
type BlockPolicy struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	MitreID     string    `gorm:"uniqueIndex;not null" json:"mitre_id"`
	MitreName   string    `json:"mitre_name"`
	Description string    `gorm:"type:text" json:"description"`
	Action      string    `json:"action"` // block, alert, log
	Enabled     bool      `gorm:"default:false" json:"enabled"`
	AutoDispose bool      `gorm:"default:false" json:"auto_dispose"`
	Priority    int       `json:"priority"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName returns the table name for BlockPolicy
func (BlockPolicy) TableName() string {
	return "block_policies"
}
