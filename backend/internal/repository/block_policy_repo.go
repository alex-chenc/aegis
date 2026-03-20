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
