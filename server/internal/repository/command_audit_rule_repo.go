package repository

import (
	"server/internal/model"

	"gorm.io/gorm"
)

type CommandAuditRuleRepo struct {
	db *gorm.DB
}

func NewCommandAuditRuleRepo(db *gorm.DB) *CommandAuditRuleRepo {
	return &CommandAuditRuleRepo{db: db}
}

func (r *CommandAuditRuleRepo) FindAllEnabled() ([]model.CommandAuditRule, error) {
	var rules []model.CommandAuditRule
	if err := r.db.Where("is_enabled = true").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *CommandAuditRuleRepo) FindAll() ([]model.CommandAuditRule, error) {
	var rules []model.CommandAuditRule
	if err := r.db.Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}
