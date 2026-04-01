package repository

import (
	"server/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RuntimeEventRepository struct {
	db *gorm.DB
}

func NewRuntimeEventRepository(db *gorm.DB) *RuntimeEventRepository {
	return &RuntimeEventRepository{db: db}
}

func (r *RuntimeEventRepository) Create(event *model.RuntimeEvent) error {
	return r.db.Create(event).Error
}

func (r *RuntimeEventRepository) FindUnaggregated(startTime, endTime int64, hostIDs []string) ([]model.RuntimeEvent, error) {
	var events []model.RuntimeEvent
	query := r.db.Where("aggregated = ? AND timestamp >= ? AND timestamp <= ?", false, startTime, endTime)
	if len(hostIDs) > 0 {
		query = query.Where("host_id IN ?", hostIDs)
	}
	err := query.Order("timestamp ASC").Find(&events).Error
	return events, err
}

func (r *RuntimeEventRepository) MarkAggregated(eventIDs []uuid.UUID) error {
	return r.db.Model(&model.RuntimeEvent{}).
		Where("id IN ?", eventIDs).
		Update("aggregated", true).Error
}

func (r *RuntimeEventRepository) DeleteOlderThan(timestamp int64) error {
	return r.db.Where("timestamp < ? AND aggregated = ?", timestamp, true).
		Delete(&model.RuntimeEvent{}).Error
}
