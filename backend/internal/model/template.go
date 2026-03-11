package model

import (
	"time"

	"github.com/google/uuid"
)

type Template struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name              string    `gorm:"type:varchar(255);not null" json:"name"`
	DisplayName       string    `gorm:"type:varchar(255);not null" json:"display_name"`
	FileType          string    `gorm:"type:varchar(20);not null" json:"file_type"`
	FileMD5           string    `gorm:"type:varchar(32);index" json:"file_md5"`
	MinioObjectName   string    `gorm:"type:varchar(255);not null" json:"minio_object_name"`
	LLMPromptTemplate *string   `gorm:"type:text" json:"llm_prompt_template"`
	Status            string    `gorm:"type:varchar(20);not null;default:'parsing'" json:"status"`
	ErrorMessage      *string   `gorm:"type:text" json:"error_message"`
	RuleCount         int       `gorm:"default:0" json:"rule_count"`
	CreatedAt         time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Template) TableName() string {
	return "templates"
}
