package model

import "time"

type BaselineRule struct {
	ID                   string    `json:"id"`
	TemplateID           string    `json:"template_id"`
	Title                string    `json:"title"`
	CheckContent         string    `json:"check_content"`
	FixContent           string    `json:"fix_content"`
	GeneratedCheckScript *string   `json:"generated_check_script"`
	GeneratedFixScript   *string   `json:"generated_fix_script"`
	CheckScriptVersion   int       `json:"check_script_version"`
	FixScriptVersion     int       `json:"fix_script_version"`
	ScriptStatus         string    `json:"script_status"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}
