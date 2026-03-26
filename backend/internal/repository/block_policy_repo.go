package repository

import (
	"context"
	"strings"

	"aegis-system/internal/model"

	"gorm.io/gorm"
)

type BlockPolicyRepository struct {
	db *gorm.DB
}

func NewBlockPolicyRepository(db *gorm.DB) *BlockPolicyRepository {
	return &BlockPolicyRepository{db: db}
}

func (r *BlockPolicyRepository) List() ([]model.BlockPolicy, error) {
	var policies []model.BlockPolicy
	err := r.db.Order("mitre_id").Find(&policies).Error
	return policies, err
}

func (r *BlockPolicyRepository) ListPaginated(page, pageSize int, query string) ([]model.BlockPolicy, int64, error) {
	var policies []model.BlockPolicy
	var total int64

	db := r.db.Model(&model.BlockPolicy{})
	if query != "" {
		db = db.Where("mitre_id ILIKE ? OR mitre_name ILIKE ?", "%"+query+"%", "%"+query+"%")
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := db.Order("mitre_id").Offset(offset).Limit(pageSize).Find(&policies).Error
	return policies, total, err
}

type BlockPolicyWithRuleTitle struct {
	model.BlockPolicy
	RuleTitle string `json:"rule_title"`
}

func (r *BlockPolicyRepository) ListPaginatedWithRuleTitle(page, pageSize int, query string) ([]BlockPolicyWithRuleTitle, int64, error) {
	var results []BlockPolicyWithRuleTitle
	var total int64

	db := r.db.Model(&model.BlockPolicy{})
	if query != "" {
		db = db.Where("mitre_id ILIKE ? OR mitre_name ILIKE ?", "%"+query+"%", "%"+query+"%")
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := r.db.Table("block_policies").
		Select(`block_policies.*, 
			(SELECT title FROM sigma_rules WHERE sigma_rules.mitre_id = block_policies.mitre_id LIMIT 1) as rule_title`).
		Where("mitre_id ILIKE ? OR mitre_name ILIKE ?", "%"+query+"%", "%"+query+"%").
		Order("mitre_id").
		Offset(offset).
		Limit(pageSize).
		Find(&results).Error
	return results, total, err
}

func (r *BlockPolicyRepository) FindByMitreID(mitreID string) (*model.BlockPolicy, error) {
	var policy model.BlockPolicy
	err := r.db.Where("mitre_id = ?", mitreID).First(&policy).Error
	if err != nil {
		return nil, err
	}

	return &policy, nil
}

func (r *BlockPolicyRepository) Update(mitreID string, updates map[string]interface{}) error {
	updates["updated_at"] = gorm.Expr("NOW()")
	return r.db.Model(&model.BlockPolicy{}).Where("mitre_id = ?", mitreID).Updates(updates).Error
}

func (r *BlockPolicyRepository) Create(policy *model.BlockPolicy) error {
	return r.db.Create(policy).Error
}

func (r *BlockPolicyRepository) CreateBatch(policies []model.BlockPolicy) error {
	return r.db.CreateInBatches(policies, 50).Error
}

func (r *BlockPolicyRepository) DeleteByMitreID(mitreID string) (bool, error) {
	result := r.db.Where("mitre_id = ?", mitreID).Delete(&model.BlockPolicy{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *BlockPolicyRepository) NormalizeMitreIDs(ctx context.Context) (int, error) {
	var policies []model.BlockPolicy
	if err := r.db.Where("mitre_id IS NOT NULL AND mitre_id != ''").Find(&policies).Error; err != nil {
		return 0, err
	}

	updated := 0
	for _, policy := range policies {
		upperID := strings.ToUpper(policy.MitreID)
		if !strings.HasPrefix(upperID, "T") {
			upperID = "T" + upperID
		}
		if upperID != policy.MitreID {
			if err := r.db.Model(&model.BlockPolicy{}).Where("id = ?", policy.ID).Update("mitre_id", upperID).Error; err != nil {
				continue
			}
			updated++
		}
	}
	return updated, nil
}
