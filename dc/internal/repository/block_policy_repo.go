package repository

import (
	"dc/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BlockPolicyRepository struct {
	db *gorm.DB
}

func NewBlockPolicyRepository(db *gorm.DB) *BlockPolicyRepository {
	return &BlockPolicyRepository{db: db}
}

func (r *BlockPolicyRepository) Create(policy *model.BlockPolicy) error {
	return r.db.Create(policy).Error
}

func (r *BlockPolicyRepository) FindByID(id uuid.UUID) (*model.BlockPolicy, error) {
	var policy model.BlockPolicy
	if err := r.db.First(&policy, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &policy, nil
}

func (r *BlockPolicyRepository) FindByMitreID(mitreID string) (*model.BlockPolicy, error) {
	var policy model.BlockPolicy
	if err := r.db.First(&policy, "mitre_id = ?", mitreID).Error; err != nil {
		return nil, err
	}
	return &policy, nil
}

func (r *BlockPolicyRepository) Update(policy *model.BlockPolicy) error {
	return r.db.Save(policy).Error
}

func (r *BlockPolicyRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&model.BlockPolicy{}, "id = ?", id).Error
}

func (r *BlockPolicyRepository) FindAll() ([]model.BlockPolicy, error) {
	var policies []model.BlockPolicy
	if err := r.db.Find(&policies).Error; err != nil {
		return nil, err
	}
	return policies, nil
}

func (r *BlockPolicyRepository) FindEnabled() ([]model.BlockPolicy, error) {
	var policies []model.BlockPolicy
	if err := r.db.Where("enabled = ?", true).Find(&policies).Error; err != nil {
		return nil, err
	}
	return policies, nil
}