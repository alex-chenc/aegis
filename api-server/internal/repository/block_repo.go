package repository

import (
	"api-server/internal/model"

	"gorm.io/gorm"
)

type BlockRepository struct {
	db *gorm.DB
}

func NewBlockRepository(db *gorm.DB) *BlockRepository {
	return &BlockRepository{db: db}
}

func (r *BlockRepository) Create(record *model.BlockRecord) error {
	return r.db.Create(record).Error
}

func (r *BlockRepository) List(page, pageSize int, filters map[string]interface{}) ([]model.BlockRecord, int64, error) {
	var (
		records []model.BlockRecord
		total   int64
	)

	query := r.db.Model(&model.BlockRecord{})
	for key, val := range filters {
		if val != nil && val != "" {
			query = query.Where(key+" = ?", val)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records).Error; err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

func (r *BlockRepository) GetTodayCount() (int64, error) {
	var count int64
	err := r.db.Model(&model.BlockRecord{}).Where("created_at >= CURRENT_DATE").Count(&count).Error
	return count, err
}
