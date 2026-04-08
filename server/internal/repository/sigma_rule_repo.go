package repository

import (
	"context"
	"strings"
	"time"

	"server/internal/model"

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

func (r *SigmaRuleRepository) ExistsByMitreID(mitreID string) (bool, error) {
	var count int64
	err := r.db.Model(&model.SigmaRule{}).Where("mitre_id = ?", mitreID).Count(&count).Error
	return count > 0, err
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

	if searchQuery, ok := filters["query"].(string); ok && searchQuery != "" {
		searchPattern := "%" + searchQuery + "%"
		query = query.Where(
			"title ILIKE ? OR rule_id ILIKE ? OR mitre_id ILIKE ?",
			searchPattern, searchPattern, searchPattern,
		)
	}

	for key, val := range filters {
		if key == "query" {
			continue
		}
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
	// Set activated_at for both active and experimental status
	// This allows tracking when the rule entered experimental status (24h silent period)
	// and when it was promoted to active
	if status == "active" || status == "experimental" {
		updates["activated_at"] = gorm.Expr("NOW()")
	}

	return r.db.Model(&model.SigmaRule{}).Where("rule_id = ?", ruleID).Updates(updates).Error
}

func (r *SigmaRuleRepository) GetActiveAndExperimental() ([]model.SigmaRule, error) {
	var rules []model.SigmaRule
	// Only return experimental rules that have passed the 24-hour silent period
	// Experimental rules need to wait 24 hours after being set to experimental before being deployed
	err := r.db.Where(
		"status = ? OR (status = ? AND activated_at IS NOT NULL AND activated_at <= ?)",
		"active",
		"experimental",
		time.Now().Add(-24*time.Hour),
	).Find(&rules).Error
	return rules, err
}

func (r *SigmaRuleRepository) GetActiveCount() (int64, error) {
	var count int64
	err := r.db.Model(&model.SigmaRule{}).Where("status IN ?", []string{"active", "experimental"}).Count(&count).Error
	return count, err
}

func (r *SigmaRuleRepository) GetExperimentalRules() ([]model.SigmaRule, error) {
	var rules []model.SigmaRule
	err := r.db.Where("status = ?", "experimental").Find(&rules).Error
	return rules, err
}

func (r *SigmaRuleRepository) FindByRuleID(ruleID string) (*model.SigmaRule, error) {
	var rule model.SigmaRule
	err := r.db.Where("rule_id = ?", ruleID).First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *SigmaRuleRepository) DeleteByRuleID(ruleID string) error {
	return r.db.Where("rule_id = ?", ruleID).Delete(&model.SigmaRule{}).Error
}

func (r *SigmaRuleRepository) NormalizeMitreIDs(ctx context.Context) (int, error) {
	var rules []model.SigmaRule
	if err := r.db.Where("mitre_id IS NOT NULL AND mitre_id != ''").Find(&rules).Error; err != nil {
		return 0, err
	}

	updated := 0
	for _, rule := range rules {
		upperID := strings.ToUpper(rule.MitreID)
		if !strings.HasPrefix(upperID, "T") {
			upperID = "T" + upperID
		}
		if upperID != rule.MitreID {
			if err := r.db.Model(&model.SigmaRule{}).Where("id = ?", rule.ID).Update("mitre_id", upperID).Error; err != nil {
				continue
			}
			updated++
		}
	}
	return updated, nil
}
