package model

import (
	"time"

	"github.com/google/uuid"
)

type BlockRecord struct {
	ID        uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BlockID   string     `gorm:"type:varchar(64);uniqueIndex;not null" json:"block_id"`
	AlertID   *uuid.UUID `gorm:"type:uuid" json:"alert_id"`
	HostID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"host_id"`
	Action    string     `gorm:"type:varchar(50);not null;default:'kill_process'" json:"action"`
	Target    string     `gorm:"type:varchar(255)" json:"target"`
	Success   bool       `gorm:"not null;default:false" json:"success"`
	Message   string     `gorm:"type:text" json:"message"`
	IssuedBy  string     `gorm:"type:varchar(20);not null;default:'llm'" json:"issued_by"`
	CreatedAt time.Time  `gorm:"default:now()" json:"created_at"`
}

func (BlockRecord) TableName() string { return "block_records" }
