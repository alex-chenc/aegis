package model

import (
	"time"

	"github.com/google/uuid"
)

type BlockPolicy struct {
	ID           uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	MitreID      string    `gorm:"type:varchar(20);uniqueIndex;not null" json:"mitre_id"`
	MitreName    string    `gorm:"type:varchar(100)" json:"mitre_name"`
	Enabled      bool      `gorm:"not null;default:true" json:"enabled"`
	AutoBlock    bool      `gorm:"not null;default:false" json:"auto_block"`
	AIAutoBlock  bool      `gorm:"not null;default:false;column:ai_auto_block" json:"ai_auto_block"`
	AutoDispose  bool      `gorm:"not null;default:false" json:"auto_dispose"`
	Action       string    `gorm:"type:varchar(50);not null;default:'kill_process'" json:"action"`
	CreatedAt    time.Time `gorm:"default:now()" json:"created_at"`
	UpdatedAt    time.Time `gorm:"default:now()" json:"updated_at"`
}

func (BlockPolicy) TableName() string { return "block_policies" }
