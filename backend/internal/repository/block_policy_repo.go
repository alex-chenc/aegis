package repository

import (
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
