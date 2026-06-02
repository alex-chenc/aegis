package repository

import (
	"context"
	"strings"

	"api-server/internal/model"

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
	RuleCount int    `json:"rule_count"`
}

func (r *BlockPolicyRepository) ListPaginatedWithRuleTitle(page, pageSize int, query string) ([]BlockPolicyWithRuleTitle, int64, error) {
	var results []BlockPolicyWithRuleTitle
	var total int64

	db := r.db.Table("block_policies")
	if query != "" {
		search := "%" + query + "%"
		db = db.Where(`block_policies.mitre_id ILIKE ?
			OR block_policies.mitre_name ILIKE ?
			OR EXISTS (
				SELECT 1 FROM sigma_rules
				WHERE sigma_rules.mitre_id = block_policies.mitre_id
				AND (sigma_rules.source IS NULL OR sigma_rules.source != 'detection_package')
				AND (sigma_rules.title ILIKE ? OR sigma_rules.rule_id ILIKE ?)
			)`, search, search, search, search)
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	queryDB := r.db.Table("block_policies").
		Select(`block_policies.*, 
			(
				SELECT title FROM sigma_rules
				WHERE sigma_rules.mitre_id = block_policies.mitre_id
				AND (sigma_rules.source IS NULL OR sigma_rules.source != 'detection_package')
				ORDER BY CASE WHEN sigma_rules.source = 'detection_package_correlation' THEN 0 ELSE 1 END,
					sigma_rules.created_at DESC
				LIMIT 1
			) as rule_title,
			(
				SELECT COUNT(*) FROM sigma_rules
				WHERE sigma_rules.mitre_id = block_policies.mitre_id
				AND (sigma_rules.source IS NULL OR sigma_rules.source != 'detection_package')
			) as rule_count`)
	if query != "" {
		search := "%" + query + "%"
		queryDB = queryDB.Where(`block_policies.mitre_id ILIKE ?
			OR block_policies.mitre_name ILIKE ?
			OR EXISTS (
				SELECT 1 FROM sigma_rules
				WHERE sigma_rules.mitre_id = block_policies.mitre_id
				AND (sigma_rules.source IS NULL OR sigma_rules.source != 'detection_package')
				AND (sigma_rules.title ILIKE ? OR sigma_rules.rule_id ILIKE ?)
			)`, search, search, search, search)
	}
	err := queryDB.
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

func (r *BlockPolicyRepository) DeleteExceptMitreIDs(mitreIDs []string) (int64, error) {
	var result *gorm.DB
	if len(mitreIDs) == 0 {
		result = r.db.Where("1 = 1").Delete(&model.BlockPolicy{})
	} else {
		result = r.db.Where("mitre_id NOT IN ?", mitreIDs).Delete(&model.BlockPolicy{})
	}
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
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
