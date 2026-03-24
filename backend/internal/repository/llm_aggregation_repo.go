package repository

import (
	"aegis-system/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type LLMAggregationRepository struct {
	db *gorm.DB
}

func NewLLMAggregationRepository(db *gorm.DB) *LLMAggregationRepository {
	return &LLMAggregationRepository{db: db}
}

func (r *LLMAggregationRepository) Create(agg *model.LLMAggregation) error {
	return r.db.Create(agg).Error
}

func (r *LLMAggregationRepository) FindByID(id string) (*model.LLMAggregation, error) {
	var agg model.LLMAggregation
	err := r.db.Where("aggregation_id = ?", id).First(&agg).Error
	if err != nil {
		return nil, err
	}
	return &agg, nil
}

func (r *LLMAggregationRepository) Update(agg *model.LLMAggregation) error {
	return r.db.Save(agg).Error
}

func (r *LLMAggregationRepository) UpdateStatus(id uuid.UUID, status string, errMsg string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if errMsg != "" {
		updates["error"] = errMsg
	}
	return r.db.Model(&model.LLMAggregation{}).Where("id = ?", id).Updates(updates).Error
}

func (r *LLMAggregationRepository) List(page, pageSize int) ([]model.LLMAggregation, int64, error) {
	var aggs []model.LLMAggregation
	var total int64

	if err := r.db.Model(&model.LLMAggregation{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&aggs).Error
	return aggs, total, err
}
