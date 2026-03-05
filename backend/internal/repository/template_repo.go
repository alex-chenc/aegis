package repository

import (
	"database/sql"

	"baseline-system/internal/model"
)

type TemplateRepository struct {
	db *sql.DB
}

func NewTemplateRepository(db *sql.DB) *TemplateRepository {
	return &TemplateRepository{db: db}
}

func (r *TemplateRepository) Create(template *model.Template) error {
	query := `
		INSERT INTO templates (name, file_type, minio_object_name, llm_prompt_template, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRow(
		query,
		template.Name, template.FileType, template.MinioObjectName,
		template.LLMPromptTemplate, template.Status,
	).Scan(&template.ID, &template.CreatedAt, &template.UpdatedAt)
}

func (r *TemplateRepository) FindAll(page, pageSize int) ([]model.Template, error) {
	offset := (page - 1) * pageSize
	query := `
		SELECT id, name, file_type, minio_object_name, llm_prompt_template,
		       status, error_message, rule_count, created_at, updated_at
		FROM templates
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := r.db.Query(query, pageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []model.Template
	for rows.Next() {
		var t model.Template
		if err := rows.Scan(
			&t.ID, &t.Name, &t.FileType, &t.MinioObjectName, &t.LLMPromptTemplate,
			&t.Status, &t.ErrorMessage, &t.RuleCount, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

func (r *TemplateRepository) FindByID(id string) (*model.Template, error) {
	query := `
		SELECT id, name, file_type, minio_object_name, llm_prompt_template,
		       status, error_message, rule_count, created_at, updated_at
		FROM templates WHERE id = $1
	`
	var t model.Template
	err := r.db.QueryRow(query, id).Scan(
		&t.ID, &t.Name, &t.FileType, &t.MinioObjectName, &t.LLMPromptTemplate,
		&t.Status, &t.ErrorMessage, &t.RuleCount, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TemplateRepository) UpdateStatus(id, status string, errorMessage *string, ruleCount int) error {
	query := `
		UPDATE templates SET status = $1, error_message = $2, rule_count = $3, updated_at = NOW()
		WHERE id = $4
	`
	_, err := r.db.Exec(query, status, errorMessage, ruleCount, id)
	return err
}

func (r *TemplateRepository) Delete(id string) error {
	query := `DELETE FROM templates WHERE id = $1`
	_, err := r.db.Exec(query, id)
	return err
}
