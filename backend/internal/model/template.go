package model

import "time"

type Template struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	FileType          string    `json:"file_type"`
	MinioObjectName   string    `json:"minio_object_name"`
	LLMPromptTemplate *string   `json:"llm_prompt_template"`
	Status            string    `json:"status"`
	ErrorMessage      *string   `json:"error_message"`
	RuleCount         int       `json:"rule_count"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
