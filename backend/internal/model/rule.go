package model

import (
	"time"

	"github.com/google/uuid"
)

type BaselineRule struct {
	ID                   uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TemplateID           uuid.UUID `gorm:"type:uuid;not null;index" json:"template_id"`
	Title                string    `gorm:"type:varchar(255);not null" json:"title"`
	CheckContent         string    `gorm:"type:text;not null" json:"check_content"`
	FixContent           string    `gorm:"type:text;not null" json:"fix_content"`
	GeneratedCheckScript *string   `gorm:"type:text" json:"generated_check_script"`
	GeneratedFixScript   *string   `gorm:"type:text" json:"generated_fix_script"`
	CheckScriptVersion   int       `gorm:"default:0" json:"check_script_version"`
	FixScriptVersion     int       `gorm:"default:0" json:"fix_script_version"`
	ScriptStatus         string    `gorm:"type:varchar(20);default:'pending'" json:"script_status"`
	CreatedAt            time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt            time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (BaselineRule) TableName() string {
	return "baseline_rules"
}
