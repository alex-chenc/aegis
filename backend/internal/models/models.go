package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Base contains common columns for all tables.
type Base struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate will set a UUID rather than numeric ID.
func (base *Base) BeforeCreate(tx *gorm.DB) error {
	if base.ID == uuid.Nil {
		base.ID = uuid.New()
	}
	return nil
}

type Template struct {
	Base
	Name              string         `gorm:"type:varchar(255);not null" json:"name"`
	FileType          string         `gorm:"type:varchar(20);not null" json:"file_type"`
	MinioObjectName   string         `gorm:"type:varchar(255);not null" json:"minio_object_name"`
	LLMPromptTemplate string         `gorm:"type:text" json:"llm_prompt_template"`
	BaselineRules     []BaselineRule `gorm:"foreignKey:TemplateID;constraint:OnDelete:CASCADE;" json:"baseline_rules,omitempty"`
}

type BaselineRule struct {
	Base
	TemplateID           uuid.UUID `gorm:"type:uuid;not null;index" json:"template_id"`
	Title                string    `gorm:"type:varchar(255);not null" json:"title"`
	CheckContent         string    `gorm:"type:text;not null" json:"check_content"`
	FixContent           string    `gorm:"type:text;not null" json:"fix_content"`
	GeneratedCheckScript string    `gorm:"type:text" json:"generated_check_script"`
	GeneratedFixScript   string    `gorm:"type:text" json:"generated_fix_script"`
}

type TaskLog struct {
	Base
	RuleID     uuid.UUID `gorm:"type:uuid;not null;index" json:"rule_id"`
	HostID     uuid.UUID `gorm:"type:uuid;not null;index" json:"host_id"`
	TaskType   string    `gorm:"type:varchar(20);not null" json:"task_type"` // CHECK or FIX
	Status     string    `gorm:"type:varchar(20);not null" json:"status"`    // SUCCESS, FAILED, TIMEOUT
	Stdout     string    `gorm:"type:text" json:"stdout"`
	Stderr     string    `gorm:"type:text" json:"stderr"`
	ExitCode   int       `json:"exit_code"`
	StartedAt  time.Time `gorm:"not null" json:"started_at"`
	FinishedAt time.Time `gorm:"not null" json:"finished_at"`
}
