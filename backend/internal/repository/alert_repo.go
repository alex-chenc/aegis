package repository

import (
	"fmt"

	"aegis-system/internal/model"

	"gorm.io/gorm"
)

type AlertRepository struct {
	db *gorm.DB
}

func NewAlertRepository(db *gorm.DB) *AlertRepository {
	return &AlertRepository{db: db}
}

func (r *AlertRepository) FindByDedupeKey(dedupeKey string) (*model.Alert, error) {
	var alert model.Alert
	err := r.db.Where("dedupe_key = ?", dedupeKey).First(&alert).Error
	if err != nil {
		return nil, err
	}

	return &alert, nil
}

func (r *AlertRepository) Create(alert *model.Alert) error {
	return r.db.Create(alert).Error
}

func (r *AlertRepository) Update(alert *model.Alert) error {
	return r.db.Save(alert).Error
}

func (r *AlertRepository) FindByID(id string) (*model.Alert, error) {
	var alert model.Alert
	err := r.db.Where("alert_id = ?", id).First(&alert).Error
	if err != nil {
		return nil, err
	}

	return &alert, nil
}

func (r *AlertRepository) List(page, pageSize int, filters map[string]interface{}) ([]model.Alert, int64, error) {
	var (
		alerts []model.Alert
		total  int64
	)

	query := r.db.Model(&model.Alert{})
	for key, val := range filters {
		if val != nil && val != "" {
			query = query.Where(key+" = ?", val)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&alerts).Error; err != nil {
		return nil, 0, err
	}

	return alerts, total, nil
}

func (r *AlertRepository) Resolve(alertID string) error {
	return r.db.Model(&model.Alert{}).
		Where("alert_id = ?", alertID).
		Updates(map[string]interface{}{"status": "resolved", "updated_at": gorm.Expr("NOW()")}).Error
}

func (r *AlertRepository) MarkAutoBlocked(alertID string) error {
	return r.db.Model(&model.Alert{}).
		Where("alert_id = ?", alertID).
		Updates(map[string]interface{}{"auto_blocked": true, "updated_at": gorm.Expr("NOW()")}).Error
}

func (r *AlertRepository) MarkManualBlocked(alertID string) error {
	return r.db.Model(&model.Alert{}).
		Where("alert_id = ?", alertID).
		Updates(map[string]interface{}{"manual_blocked": true, "updated_at": gorm.Expr("NOW()")}).Error
}

func (r *AlertRepository) GetTodayCount() (int64, error) {
	var count int64
	err := r.db.Model(&model.Alert{}).Where("created_at >= CURRENT_DATE").Count(&count).Error
	return count, err
}

func (r *AlertRepository) GetAffectedHostCount() (int64, error) {
	var count int64
	err := r.db.Model(&model.Alert{}).Where("status = ?", "active").Distinct("host_id").Count(&count).Error
	return count, err
}

func (r *AlertRepository) GetTrend(hours int) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	interval := fmt.Sprintf("%d hours", hours)
	err := r.db.Raw(`
		SELECT date_trunc('hour', created_at) as time_bucket, count(*) as count
		FROM alerts
		WHERE created_at >= NOW() - CAST(? AS interval)
		GROUP BY time_bucket
		ORDER BY time_bucket
	`, interval).Scan(&results).Error
	return results, err
}
