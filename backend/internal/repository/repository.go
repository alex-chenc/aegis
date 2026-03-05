package repository

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ai-benchmark/backend/internal/models"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetDB() *gorm.DB {
	return r.db
}

func (r *Repository) CreateHost(host *models.Host) error {
	if host.ID == uuid.Nil {
		host.ID = uuid.New()
	}
	return r.db.Create(host).Error
}

func (r *Repository) GetHostByID(id uuid.UUID) (*models.Host, error) {
	var host models.Host
	err := r.db.First(&host, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &host, nil
}

func (r *Repository) GetHosts(params models.HostListParams) (*models.HostResponse, error) {
	var hosts []models.Host
	var total int64

	query := r.db.Model(&models.Host{})

	if params.Search != "" {
		query = query.Where("hostname ILIKE ? OR ip_address ILIKE ?", "%"+params.Search+"%", "%"+params.Search+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	offset := (params.Page - 1) * params.PageSize
	if offset < 0 {
		offset = 0
	}
	if params.PageSize <= 0 {
		params.PageSize = 10
	}

	if err := query.Offset(offset).Limit(params.PageSize).Order("last_heartbeat_at DESC").Find(&hosts).Error; err != nil {
		return nil, err
	}

	return &models.HostResponse{
		Total: int(total),
		Items: hosts,
	}, nil
}

func (r *Repository) UpdateHost(id uuid.UUID, updates map[string]interface{}) error {
	return r.db.Model(&models.Host{}).Where("id = ?", id).Updates(updates).Error
}

func (r *Repository) UpsertHost(host *models.Host) error {
	return r.db.Where("id = ?", host.ID).Assign(host).FirstOrCreate(host).Error
}

func (r *Repository) DeleteHost(id uuid.UUID) error {
	return r.db.Delete(&models.Host{}, "id = ?", id).Error
}

func (r *Repository) CreateTemplate(template *models.Template) error {
	if template.ID == uuid.Nil {
		template.ID = uuid.New()
	}
	return r.db.Create(template).Error
}

func (r *Repository) GetTemplateByID(id uuid.UUID) (*models.Template, error) {
	var template models.Template
	err := r.db.Preload("BaselineRules").First(&template, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &template, nil
}

func (r *Repository) GetTemplates(page, pageSize int) ([]models.Template, int64, error) {
	var templates []models.Template
	var total int64

	if err := r.db.Model(&models.Template{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	if err := r.db.Preload("BaselineRules").Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&templates).Error; err != nil {
		return nil, 0, err
	}

	return templates, total, nil
}

func (r *Repository) DeleteTemplate(id uuid.UUID) error {
	return r.db.Delete(&models.Template{}, "id = ?", id).Error
}

func (r *Repository) CreateBaselineRule(rule *models.BaselineRule) error {
	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}
	return r.db.Create(rule).Error
}

func (r *Repository) GetBaselineRulesByTemplateID(templateID uuid.UUID) ([]models.BaselineRule, error) {
	var rules []models.BaselineRule
	err := r.db.Where("template_id = ?", templateID).Order("created_at ASC").Find(&rules).Error
	return rules, err
}

func (r *Repository) GetBaselineRuleByID(id uuid.UUID) (*models.BaselineRule, error) {
	var rule models.BaselineRule
	err := r.db.First(&rule, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *Repository) UpdateBaselineRule(id uuid.UUID, updates map[string]interface{}) error {
	return r.db.Model(&models.BaselineRule{}).Where("id = ?", id).Updates(updates).Error
}

func (r *Repository) CreateTaskLog(taskLog *models.TaskLog) error {
	if taskLog.ID == uuid.Nil {
		taskLog.ID = uuid.New()
	}
	return r.db.Create(taskLog).Error
}

func (r *Repository) GetTaskLogByID(id uuid.UUID) (*models.TaskLog, error) {
	var taskLog models.TaskLog
	err := r.db.First(&taskLog, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &taskLog, nil
}

func (r *Repository) GetTaskLogs(page, pageSize int, hostID, ruleID *uuid.UUID) ([]models.TaskLog, int64, error) {
	var taskLogs []models.TaskLog
	var total int64

	query := r.db.Model(&models.TaskLog{})

	if hostID != nil {
		query = query.Where("host_id = ?", hostID)
	}
	if ruleID != nil {
		query = query.Where("rule_id = ?", ruleID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	if err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&taskLogs).Error; err != nil {
		return nil, 0, err
	}

	return taskLogs, total, nil
}

func (r *Repository) UpdateTaskLog(id uuid.UUID, updates map[string]interface{}) error {
	return r.db.Model(&models.TaskLog{}).Where("id = ?", id).Updates(updates).Error
}

func (r *Repository) CleanupOldTaskLogs(days int) (int64, error) {
	result := r.db.Where("created_at < ?", time.Now().AddDate(0, 0, -days)).Delete(&models.TaskLog{})
	return result.RowsAffected, result.Error
}
