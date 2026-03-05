package repository

import (
	"context"
	"database/sql"

	"baseline-system/internal/model"
)

type RuleRepository struct {
	db *sql.DB
}

func NewRuleRepository(db *sql.DB) *RuleRepository {
	return &RuleRepository{db: db}
}

func (r *RuleRepository) BatchCreate(rules []*model.BaselineRule) error {
	ctx := context.Background()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO baseline_rules (template_id, title, check_content, fix_content)
		VALUES ($1, $2, $3, $4)
		RETURNING id, script_status, created_at, updated_at
	`

	for _, rule := range rules {
		err := tx.QueryRow(
			query,
			rule.TemplateID, rule.Title, rule.CheckContent, rule.FixContent,
		).Scan(&rule.ID, &rule.ScriptStatus, &rule.CreatedAt, &rule.UpdatedAt)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *RuleRepository) FindByTemplateID(templateID string) ([]model.BaselineRule, error) {
	query := `
		SELECT id, template_id, title, check_content, fix_content,
		       generated_check_script, generated_fix_script,
		       check_script_version, fix_script_version, script_status,
		       created_at, updated_at
		FROM baseline_rules WHERE template_id = $1
		ORDER BY created_at
	`
	rows, err := r.db.Query(query, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []model.BaselineRule
	for rows.Next() {
		var r model.BaselineRule
		if err := rows.Scan(
			&r.ID, &r.TemplateID, &r.Title, &r.CheckContent, &r.FixContent,
			&r.GeneratedCheckScript, &r.GeneratedFixScript,
			&r.CheckScriptVersion, &r.FixScriptVersion, &r.ScriptStatus,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (r *RuleRepository) FindByID(id string) (*model.BaselineRule, error) {
	query := `
		SELECT id, template_id, title, check_content, fix_content,
		       generated_check_script, generated_fix_script,
		       check_script_version, fix_script_version, script_status,
		       created_at, updated_at
		FROM baseline_rules WHERE id = $1
	`
	var rule model.BaselineRule
	err := r.db.QueryRow(query, id).Scan(
		&rule.ID, &rule.TemplateID, &rule.Title, &rule.CheckContent, &rule.FixContent,
		&rule.GeneratedCheckScript, &rule.GeneratedFixScript,
		&rule.CheckScriptVersion, &rule.FixScriptVersion, &rule.ScriptStatus,
		&rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *RuleRepository) UpdateScript(ruleID, scriptType, scriptContent string, version int) error {
	var query string
	if scriptType == "CHECK" {
		query = `
			UPDATE baseline_rules SET generated_check_script = $1, check_script_version = $2,
				script_status = 'ready', updated_at = NOW()
			WHERE id = $3
		`
	} else {
		query = `
			UPDATE baseline_rules SET generated_fix_script = $1, fix_script_version = $2,
				script_status = 'ready', updated_at = NOW()
			WHERE id = $3
		`
	}
	_, err := r.db.Exec(query, scriptContent, version, ruleID)
	return err
}

func (r *RuleRepository) UpdateScriptStatus(ruleID, status string) error {
	query := `UPDATE baseline_rules SET script_status = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(query, status, ruleID)
	return err
}
