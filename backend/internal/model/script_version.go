package model

import (
	"time"

	"github.com/google/uuid"
)

type ScriptVersion struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RuleID           uuid.UUID `gorm:"type:uuid;not null;index" json:"rule_id"`
	ScriptType       string    `gorm:"type:varchar(10);not null" json:"script_type"`
	Version          int       `gorm:"not null" json:"version"`
	ScriptContent    string    `gorm:"type:text;not null" json:"script_content"`
	GenerationSource string    `gorm:"type:varchar(20);not null" json:"generation_source"`
	LLMPromptUsed    *string   `gorm:"type:text" json:"llm_prompt_used"`
	LLMResponseRaw   *string   `gorm:"type:text" json:"llm_response_raw"`
	MinioObjectName  *string   `gorm:"type:varchar(255)" json:"minio_object_name"`
	IsCurrent        bool      `gorm:"not null;default:true" json:"is_current"`
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (ScriptVersion) TableName() string {
	return "script_versions"
}
