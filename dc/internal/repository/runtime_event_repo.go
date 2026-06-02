package repository

import (
	"context"
	"dc/internal/model"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RuntimeEventRepository struct {
	db *gorm.DB
}

func NewRuntimeEventRepository(db *gorm.DB) *RuntimeEventRepository {
	return &RuntimeEventRepository{db: db}
}

func (r *RuntimeEventRepository) Create(event *model.RuntimeEvent) error {
	return r.db.Clauses(ignoreDuplicateEventID()).Create(event).Error
}

func (r *RuntimeEventRepository) CreateWithContext(ctx context.Context, event *model.RuntimeEvent) error {
	return r.db.WithContext(ctx).Clauses(ignoreDuplicateEventID()).Create(event).Error
}

func (r *RuntimeEventRepository) CreateBatch(events []*model.RuntimeEvent) error {
	return r.db.Clauses(ignoreDuplicateEventID()).CreateInBatches(events, 100).Error
}

func ignoreDuplicateEventID() clause.OnConflict {
	return clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_id"}},
		DoNothing: true,
	}
}

func (r *RuntimeEventRepository) FindByID(id uuid.UUID) (*model.RuntimeEvent, error) {
	var event model.RuntimeEvent
	if err := r.db.First(&event, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *RuntimeEventRepository) FindByHostID(hostID uuid.UUID, limit int) ([]model.RuntimeEvent, error) {
	var events []model.RuntimeEvent
	if err := r.db.Where("host_id = ?", hostID).Order("created_at DESC").Limit(limit).Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

func (r *RuntimeEventRepository) FindByTimeRange(start, end time.Time, limit int) ([]model.RuntimeEvent, error) {
	var events []model.RuntimeEvent
	if err := r.db.Where("created_at BETWEEN ? AND ?", start, end).Order("created_at DESC").Limit(limit).Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

func (r *RuntimeEventRepository) DeleteOlderThan(threshold time.Time) error {
	return r.db.Where("created_at < ?", threshold).Delete(&model.RuntimeEvent{}).Error
}

func (r *RuntimeEventRepository) Count() (int64, error) {
	var count int64
	if err := r.db.Model(&model.RuntimeEvent{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
