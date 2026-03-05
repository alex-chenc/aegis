package repository

import (
	"database/sql"
	"time"

	"baseline-system/internal/model"
)

type HealingLogRepository struct {
	db *sql.DB
}

func NewHealingLogRepository(db *sql.DB) *HealingLogRepository {
	return &HealingLogRepository{db: db}
}

func (r *HealingLogRepository) Create(log *model.HealingLog) error {
	query := `
		INSERT INTO healing_logs (original_task_id, rule_id, host_id, script_type, trigger_error,
			trigger_exit_code, total_attempts, max_attempts, status, started_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at
	`
	return r.db.QueryRow(
		query,
		log.OriginalTaskID, log.RuleID, log.HostID, log.ScriptType, log.TriggerError,
		log.TriggerExitCode, log.TotalAttempts, log.MaxAttempts, log.Status, log.StartedAt,
	).Scan(&log.ID, &log.CreatedAt)
}

func (r *HealingLogRepository) Update(log *model.HealingLog) error {
	query := `
		UPDATE healing_logs SET
			total_attempts = $1, status = $2, final_script_version_id = $3,
			attempts_detail = $4, finished_at = $5
		WHERE id = $6
	`
	_, err := r.db.Exec(
		query,
		log.TotalAttempts, log.Status, log.FinalScriptVersionID,
		log.AttemptsDetail, log.FinishedAt, log.ID,
	)
	return err
}

func (r *HealingLogRepository) FindByID(id string) (*model.HealingLog, error) {
	query := `
		SELECT id, original_task_id, rule_id, host_id, script_type, trigger_error,
		       trigger_exit_code, total_attempts, max_attempts, status, final_script_version_id,
		       attempts_detail, started_at, finished_at, created_at
		FROM healing_logs WHERE id = $1
	`
	var log model.HealingLog
	err := r.db.QueryRow(query, id).Scan(
		&log.ID, &log.OriginalTaskID, &log.RuleID, &log.HostID, &log.ScriptType, &log.TriggerError,
		&log.TriggerExitCode, &log.TotalAttempts, &log.MaxAttempts, &log.Status, &log.FinalScriptVersionID,
		&log.AttemptsDetail, &log.StartedAt, &log.FinishedAt, &log.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *HealingLogRepository) FindByOriginalTaskID(taskID string) (*model.HealingLog, error) {
	query := `
		SELECT id, original_task_id, rule_id, host_id, script_type, trigger_error,
		       trigger_exit_code, total_attempts, max_attempts, status, final_script_version_id,
		       attempts_detail, started_at, finished_at, created_at
		FROM healing_logs WHERE original_task_id = $1
	`
	var log model.HealingLog
	err := r.db.QueryRow(query, taskID).Scan(
		&log.ID, &log.OriginalTaskID, &log.RuleID, &log.HostID, &log.ScriptType, &log.TriggerError,
		&log.TriggerExitCode, &log.TotalAttempts, &log.MaxAttempts, &log.Status, &log.FinalScriptVersionID,
		&log.AttemptsDetail, &log.StartedAt, &log.FinishedAt, &log.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *HealingLogRepository) IncrementAttempts(id string) error {
	query := `UPDATE healing_logs SET total_attempts = total_attempts + 1 WHERE id = $1`
	_, err := r.db.Exec(query, id)
	return err
}

func (r *HealingLogRepository) MarkCompleted(id string, scriptVersionID string) error {
	now := time.Now()
	query := `
		UPDATE healing_logs SET status = 'healed', final_script_version_id = $1, finished_at = $2
		WHERE id = $3
	`
	_, err := r.db.Exec(query, scriptVersionID, now, id)
	return err
}

func (r *HealingLogRepository) MarkFailed(id string) error {
	now := time.Now()
	query := `UPDATE healing_logs SET status = 'failed', finished_at = $1 WHERE id = $2`
	_, err := r.db.Exec(query, now, id)
	return err
}
