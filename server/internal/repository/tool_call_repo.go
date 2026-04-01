package repository

import (
	"server/internal/model"

	"gorm.io/gorm"
)

type ToolCallRepository struct {
	db *gorm.DB
}

func NewToolCallRepository(db *gorm.DB) *ToolCallRepository {
	return &ToolCallRepository{db: db}
}

func (r *ToolCallRepository) Create(call *model.ToolCall) error {
	return r.db.Create(call).Error
}

func (r *ToolCallRepository) Update(call *model.ToolCall) error {
	return r.db.Save(call).Error
}

func (r *ToolCallRepository) FindByCallID(callID string) (*model.ToolCall, error) {
	var call model.ToolCall
	err := r.db.Where("call_id = ?", callID).First(&call).Error
	if err != nil {
		return nil, err
	}

	return &call, nil
}

func (r *ToolCallRepository) List(page, pageSize int, filters map[string]interface{}) ([]model.ToolCall, int64, error) {
	var (
		calls []model.ToolCall
		total int64
	)

	query := r.db.Model(&model.ToolCall{})
	for key, val := range filters {
		if val != nil && val != "" {
			query = query.Where(key+" = ?", val)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&calls).Error; err != nil {
		return nil, 0, err
	}

	return calls, total, nil
}
