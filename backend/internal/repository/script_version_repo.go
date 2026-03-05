package repository

import (
	"database/sql"

	"baseline-system/internal/model"
)

type ScriptVersionRepository struct {
	db *sql.DB
}

func NewScriptVersionRepository(db *sql.DB) *ScriptVersionRepository {
	return &ScriptVersionRepository{db: db}
}

func (r *ScriptVersionRepository) Create(version *model.ScriptVersion) error {
	query := `
		INSERT INTO script_versions (rule_id, script_type, version, script_content, generation_source,
			llm_prompt_used, llm_response_raw, minio_object_name, is_current)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
	`
	return r.db.QueryRow(
		query,
		version.RuleID, version.ScriptType, version.Version, version.ScriptContent,
		version.GenerationSource, version.LLMPromptUsed, version.LLMResponseRaw,
		version.MinioObjectName, version.IsCurrent,
	).Scan(&version.ID, &version.CreatedAt)
}

func (r *ScriptVersionRepository) FindByRuleAndType(ruleID, scriptType string) ([]model.ScriptVersion, error) {
	query := `
		SELECT id, rule_id, script_type, version, script_content, generation_source,
		       llm_prompt_used, llm_response_raw, minio_object_name, is_current, created_at
		FROM script_versions
		WHERE rule_id = $1 AND script_type = $2
		ORDER BY version DESC
	`
	rows, err := r.db.Query(query, ruleID, scriptType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []model.ScriptVersion
	for rows.Next() {
		var v model.ScriptVersion
		if err := rows.Scan(
			&v.ID, &v.RuleID, &v.ScriptType, &v.Version, &v.ScriptContent, &v.GenerationSource,
			&v.LLMPromptUsed, &v.LLMResponseRaw, &v.MinioObjectName, &v.IsCurrent, &v.CreatedAt,
		); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func (r *ScriptVersionRepository) SetCurrentVersion(versionID string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var ruleID string
	var scriptType string
	err = tx.QueryRow(`SELECT rule_id, script_type FROM script_versions WHERE id = $1`, versionID).Scan(&ruleID, &scriptType)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`UPDATE script_versions SET is_current = false WHERE rule_id = $1 AND script_type = $2`, ruleID, scriptType)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`UPDATE script_versions SET is_current = true WHERE id = $1`, versionID)
	if err != nil {
		return err
	}

	return tx.Commit()
}
