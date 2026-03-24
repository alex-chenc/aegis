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

	countQuery := r.db.Model(&model.Alert{})
	for key, val := range filters {
		if val != nil && val != "" {
			countQuery = countQuery.Where(key+" = ?", val)
		}
	}

	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	type AlertWithHost struct {
		model.Alert
		Hostname  string `json:"hostname"`
		RuleTitle string `json:"rule_title"`
	}

	var alertsWithHost []AlertWithHost
	query := r.db.Table("alerts").
		Select(`alerts.*, 
			hosts.hostname, 
			COALESCE(
				NULLIF(alerts.rule_title, ''),
				(SELECT title FROM sigma_rules WHERE LOWER(mitre_id) = LOWER(alerts.mitre_id) LIMIT 1),
				alerts.mitre_name
			) as rule_title`).
		Joins("LEFT JOIN hosts ON alerts.host_id = hosts.id")

	for key, val := range filters {
		if val != nil && val != "" {
			query = query.Where("alerts."+key+" = ?", val)
		}
	}

	err := query.Order("alerts.created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&alertsWithHost).Error

	if err != nil {
		return nil, 0, err
	}

	alerts = make([]model.Alert, len(alertsWithHost))
	for i, a := range alertsWithHost {
		alerts[i] = a.Alert
		alerts[i].Hostname = a.Hostname
		if a.RuleTitle != "" {
			alerts[i].RuleTitle = a.RuleTitle
		} else {
			alerts[i].RuleTitle = a.Alert.MitreName
		}
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

func (r *AlertRepository) UpdateBlockStatus(alertID string, status string, message string) error {
	updates := map[string]interface{}{
		"block_status": status,
		"updated_at":   gorm.Expr("NOW()"),
	}
	if message != "" {
		updates["block_message"] = message
	}
	if status == "success" {
		updates["status"] = "resolved"
	}
	return r.db.Model(&model.Alert{}).
		Where("alert_id = ?", alertID).
		Updates(updates).Error
}

func (r *AlertRepository) MarkAIJudged(alertID string, disposalStrategy string) error {
	updates := map[string]interface{}{
		"judgment_source": "ai",
		"updated_at":      gorm.Expr("NOW()"),
	}
	if disposalStrategy != "" {
		updates["llm_disposal_strategy"] = disposalStrategy
	}
	return r.db.Model(&model.Alert{}).
		Where("alert_id = ?", alertID).
		Updates(updates).Error
}

func (r *AlertRepository) FindByMitreID(mitreID string) ([]model.Alert, error) {
	var alerts []model.Alert
	err := r.db.Where("mitre_id = ?", mitreID).Order("created_at DESC").Find(&alerts).Error
	return alerts, err
}

func (r *AlertRepository) GetActiveCount() (int64, error) {
	var count int64
	err := r.db.Model(&model.Alert{}).Where("status = ?", "pending").Count(&count).Error
	return count, err
}

func (r *AlertRepository) GetCountByJudgmentSource(source string) (int64, error) {
	var count int64
	err := r.db.Model(&model.Alert{}).Where("judgment_source = ?", source).Count(&count).Error
	return count, err
}

func (r *AlertRepository) GetCountByMitreID(mitreID string) (int64, error) {
	var count int64
	err := r.db.Model(&model.Alert{}).Where("mitre_id = ? OR mitre_id LIKE ?", mitreID, mitreID+".%").Count(&count).Error
	return count, err
}
