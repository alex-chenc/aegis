package repository

import (
	"dc/internal/model"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AlertRepository struct {
	db *gorm.DB
}

func NewAlertRepository(db *gorm.DB) *AlertRepository {
	return &AlertRepository{db: db}
}

func (r *AlertRepository) Create(alert *model.Alert) error {
	return r.db.Create(alert).Error
}

func (r *AlertRepository) FindByID(id uuid.UUID) (*model.Alert, error) {
	var alert model.Alert
	if err := r.db.First(&alert, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &alert, nil
}

func (r *AlertRepository) FindByDedupeKey(dedupeKey string) (*model.Alert, error) {
	var alert model.Alert
	if err := r.db.First(&alert, "dedupe_key = ?", dedupeKey).Error; err != nil {
		return nil, err
	}
	return &alert, nil
}

func (r *AlertRepository) Update(alert *model.Alert) error {
	return r.db.Save(alert).Error
}

func (r *AlertRepository) FindPending(limit int) ([]model.Alert, error) {
	var alerts []model.Alert
	if err := r.db.Where("status = ?", "pending").Limit(limit).Find(&alerts).Error; err != nil {
		return nil, err
	}
	return alerts, nil
}

func (r *AlertRepository) UpdateStatus(alertID string, status string) error {
	return r.db.Model(&model.Alert{}).Where("alert_id = ?", alertID).Update("status", status).Error
}

func (r *AlertRepository) SetResolved(alertID string) error {
	now := time.Now()
	return r.db.Model(&model.Alert{}).Where("alert_id = ?", alertID).Updates(map[string]interface{}{
		"status":      "resolved",
		"resolved_at": now,
	}).Error
}

func (r *AlertRepository) CountByStatus(status string) (int64, error) {
	var count int64
	if err := r.db.Model(&model.Alert{}).Where("status = ?", status).Count(&count).Error; err != nil {
		logger := zap.L()
		logger.Error("Failed to count alerts by status", zap.Error(err), zap.String("status", status))
		return 0, err
	}
	return count, nil
}