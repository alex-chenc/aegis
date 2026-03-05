package model

import "time"

type TaskLog struct {
	ID            string     `json:"id"`
	TaskGroupID   string     `json:"task_group_id"`
	RuleID        string     `json:"rule_id"`
	HostID        string     `json:"host_id"`
	TaskType      string     `json:"task_type"`
	Status        string     `json:"status"`
	ScriptContent *string    `json:"script_content"`
	ScriptVersion *int       `json:"script_version"`
	Stdout        *string    `json:"stdout"`
	Stderr        *string    `json:"stderr"`
	ExitCode      *int       `json:"exit_code"`
	HealingID     *string    `json:"healing_id"`
	StartedAt     *time.Time `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`
	CreatedAt     time.Time  `json:"created_at"`
}
