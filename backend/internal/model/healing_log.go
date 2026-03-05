package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

type AttemptDetail struct {
	Attempt         int       `json:"attempt"`
	ScriptVersionID string    `json:"script_version_id"`
	ErrorInput      string    `json:"error_input"`
	LLMFixSummary   string    `json:"llm_fix_summary"`
	ResultExitCode  int       `json:"result_exit_code"`
	ResultStderr    string    `json:"result_stderr"`
	Timestamp       time.Time `json:"timestamp"`
}

type AttemptsDetail []AttemptDetail

func (a AttemptsDetail) Value() (driver.Value, error) {
	return json.Marshal(a)
}

func (a *AttemptsDetail) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, a)
}

type HealingLog struct {
	ID                   string         `json:"id"`
	OriginalTaskID       string         `json:"original_task_id"`
	RuleID               string         `json:"rule_id"`
	HostID               string         `json:"host_id"`
	ScriptType           string         `json:"script_type"`
	TriggerError         string         `json:"trigger_error"`
	TriggerExitCode      int            `json:"trigger_exit_code"`
	TotalAttempts        int            `json:"total_attempts"`
	MaxAttempts          int            `json:"max_attempts"`
	Status               string         `json:"status"`
	FinalScriptVersionID *string        `json:"final_script_version_id"`
	AttemptsDetail       AttemptsDetail `json:"attempts_detail"`
	StartedAt            time.Time      `json:"started_at"`
	FinishedAt           *time.Time     `json:"finished_at"`
	CreatedAt            time.Time      `json:"created_at"`
}
