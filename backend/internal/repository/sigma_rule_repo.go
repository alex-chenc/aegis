package repository

import (
	"aegis-system/internal/model"

	"gorm.io/gorm"
)

type SigmaRuleRepository struct {
	db *gorm.DB
}

func NewSigmaRuleRepository(db *gorm.DB) *SigmaRuleRepository {
	return &SigmaRuleRepository{db: db}
}

func (r *SigmaRuleRepository) Create(rule *model.SigmaRule) error {
	return r.db.Create(rule).Error
}

func (r *SigmaRuleRepository) Update(rule *model.SigmaRule) error {
	return r.db.Save(rule).Error
}

func (r *SigmaRuleRepository) FindByID(ruleID string) (*model.SigmaRule, error) {
	var rule model.SigmaRule
	err := r.db.Where("rule_id = ?", ruleID).First(&rule).Error
	if err != nil {
		return nil, err
	}

	return &rule, nil
}

func (r *SigmaRuleRepository) List(page, pageSize int, filters map[string]interface{}) ([]model.SigmaRule, int64, error) {
	var (
		rules []model.SigmaRule
		total int64
	)

	query := r.db.Model(&model.SigmaRule{})
	for key, val := range filters {
		if val != nil && val != "" {
			query = query.Where(key+" = ?", val)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rules).Error; err != nil {
		return nil, 0, err
	}

	return rules, total, nil
}

func (r *SigmaRuleRepository) UpdateStatus(ruleID, status string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": gorm.Expr("NOW()"),
	}
	if status == "active" {
		updates["activated_at"] = gorm.Expr("NOW()")
	}

	return r.db.Model(&model.SigmaRule{}).Where("rule_id = ?", ruleID).Updates(updates).Error
}

func (r *SigmaRuleRepository) GetActiveAndExperimental() ([]model.SigmaRule, error) {
	var rules []model.SigmaRule
	err := r.db.Where("status IN ?", []string{"active", "experimental"}).Find(&rules).Error
	return rules, err
}

func (r *SigmaRuleRepository) GetActiveCount() (int64, error) {
	var count int64
	err := r.db.Model(&model.SigmaRule{}).Where("status IN ?", []string{"active", "experimental"}).Count(&count).Error
	return count, err
}
