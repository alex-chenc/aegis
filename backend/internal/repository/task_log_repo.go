package repository

import (
	"database/sql"
	"time"

	"baseline-system/internal/model"
)

type TaskLogRepository struct {
	db *sql.DB
}

func NewTaskLogRepository(db *sql.DB) *TaskLogRepository {
	return &TaskLogRepository{db: db}
}

func (r *TaskLogRepository) Create(log *model.TaskLog) error {
	query := `
		INSERT INTO task_logs (task_group_id, rule_id, host_id, task_type, status, script_content, script_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`
	return r.db.QueryRow(
		query,
		log.TaskGroupID, log.RuleID, log.HostID, log.TaskType, log.Status,
		log.ScriptContent, log.ScriptVersion,
	).Scan(&log.ID, &log.CreatedAt)
}

func (r *TaskLogRepository) UpdateResult(id string, stdout, stderr *string, exitCode *int, status string, finishedAt time.Time) error {
	query := `
		UPDATE task_logs SET stdout = $1, stderr = $2, exit_code = $3, status = $4, finished_at = $5
		WHERE id = $6
	`
	_, err := r.db.Exec(query, stdout, stderr, exitCode, status, finishedAt, id)
	return err
}

func (r *TaskLogRepository) FindByGroupID(groupID string) ([]model.TaskLog, error) {
	query := `
		SELECT id, task_group_id, rule_id, host_id, task_type, status,
		       script_content, script_version, stdout, stderr, exit_code, healing_id,
		       started_at, finished_at, created_at
		FROM task_logs WHERE task_group_id = $1
		ORDER BY created_at
	`
	rows, err := r.db.Query(query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []model.TaskLog
	for rows.Next() {
		var l model.TaskLog
		if err := rows.Scan(
			&l.ID, &l.TaskGroupID, &l.RuleID, &l.HostID, &l.TaskType, &l.Status,
			&l.ScriptContent, &l.ScriptVersion, &l.Stdout, &l.Stderr, &l.ExitCode, &l.HealingID,
			&l.StartedAt, &l.FinishedAt, &l.CreatedAt,
		); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (r *TaskLogRepository) FindByID(id string) (*model.TaskLog, error) {
	query := `
		SELECT id, task_group_id, rule_id, host_id, task_type, status,
		       script_content, script_version, stdout, stderr, exit_code, healing_id,
		       started_at, finished_at, created_at
		FROM task_logs WHERE id = $1
	`
	var l model.TaskLog
	err := r.db.QueryRow(query, id).Scan(
		&l.ID, &l.TaskGroupID, &l.RuleID, &l.HostID, &l.TaskType, &l.Status,
		&l.ScriptContent, &l.ScriptVersion, &l.Stdout, &l.Stderr, &l.ExitCode, &l.HealingID,
		&l.StartedAt, &l.FinishedAt, &l.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &l, nil
}
