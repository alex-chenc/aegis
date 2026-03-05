package model

import "time"

type ScriptVersion struct {
	ID               string    `json:"id"`
	RuleID           string    `json:"rule_id"`
	ScriptType       string    `json:"script_type"`
	Version          int       `json:"version"`
	ScriptContent    string    `json:"script_content"`
	GenerationSource string    `json:"generation_source"`
	LLMPromptUsed    *string   `json:"llm_prompt_used"`
	LLMResponseRaw   *string   `json:"llm_response_raw"`
	MinioObjectName  *string   `json:"minio_object_name"`
	IsCurrent        bool      `json:"is_current"`
	CreatedAt        time.Time `json:"created_at"`
}
