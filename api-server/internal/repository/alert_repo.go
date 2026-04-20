package repository

import (
	"context"
	"strings"
	"time"

	"api-server/internal/model"

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
	type AlertWithHost struct {
		model.Alert
		Hostname  string `json:"hostname"`
		RuleTitle string `json:"rule_title"`
	}

	var result AlertWithHost
	err := r.db.Table("alerts").
		Select(`alerts.*, 
			hosts.hostname, 
			COALESCE(
				NULLIF(alerts.rule_title, ''),
				(SELECT title FROM sigma_rules WHERE LOWER(mitre_id) = LOWER(alerts.mitre_id) LIMIT 1),
				alerts.mitre_id
			) as rule_title`).
		Joins("LEFT JOIN hosts ON alerts.host_id = hosts.id").
		Where("alerts.alert_id = ?", id).
		First(&result).Error
	if err != nil {
		return nil, err
	}

	alert := result.Alert
	alert.Hostname = result.Hostname
	if result.RuleTitle != "" {
		alert.RuleTitle = result.RuleTitle
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
	err := r.db.Model(&model.Alert{}).Where("status = ?", "pending").Distinct("host_id").Count(&count).Error
	return count, err
}

func (r *AlertRepository) GetTrend(hours int) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	err := r.db.Raw(`
		SELECT 
			time_bucket as time_bucket,
			COALESCE(alert_counts.count, 0) as count
		FROM (
			SELECT generate_series(
				date_trunc('hour', NOW() - make_interval(hours := ?)),
				date_trunc('hour', NOW()),
				interval '1 hour'
			) as time_bucket
		) hours
		LEFT JOIN (
			SELECT date_trunc('hour', created_at) as alert_hour, count(*) as count
			FROM alerts
			WHERE created_at >= NOW() - make_interval(hours := ?)
			GROUP BY alert_hour
		) alert_counts ON hours.time_bucket = alert_counts.alert_hour
		ORDER BY time_bucket
	`, hours, hours).Scan(&results).Error
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

func (r *AlertRepository) MarkAIJudged(alertID string, llmSummary string) error {
	updates := map[string]interface{}{
		"judgment_source": "ai",
		"updated_at":      gorm.Expr("NOW()"),
	}
	if llmSummary != "" {
		updates["llm_summary"] = llmSummary
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

func (r *AlertRepository) UpdateLLMSummary(alertID string, summary string) error {
	return r.db.Model(&model.Alert{}).
		Where("alert_id = ?", alertID).
		Updates(map[string]interface{}{
			"llm_summary": summary,
			"updated_at":  gorm.Expr("NOW()"),
		}).Error
}

func (r *AlertRepository) DeleteByIDs(alertIDs []string) error {
	if len(alertIDs) == 0 {
		return nil
	}
	return r.db.Where("alert_id IN ?", alertIDs).Delete(&model.Alert{}).Error
}

// FindPendingByTimeRange 查询指定时间范围内的pending状态告警
// 用于AI降噪功能，根据用户选择的时间范围筛选告警
func (r *AlertRepository) FindPendingByTimeRange(startTime, endTime time.Time, hostIDs []string) ([]model.Alert, error) {
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
		Joins("LEFT JOIN hosts ON alerts.host_id = hosts.id").
		Where("alerts.status = ?", "pending").
		Where("alerts.created_at >= ? AND alerts.created_at <= ?", startTime, endTime)

	if len(hostIDs) > 0 {
		query = query.Where("alerts.host_id IN ?", hostIDs)
	}

	err := query.Order("alerts.created_at DESC").Find(&alertsWithHost).Error
	if err != nil {
		return nil, err
	}

	alerts := make([]model.Alert, len(alertsWithHost))
	for i, a := range alertsWithHost {
		alerts[i] = a.Alert
		alerts[i].Hostname = a.Hostname
		if a.RuleTitle != "" {
			alerts[i].RuleTitle = a.RuleTitle
		} else {
			alerts[i].RuleTitle = a.Alert.MitreName
		}
	}

	return alerts, nil
}

type RuleTriggerStats struct {
	RuleID     string
	RuleTitle  string
	MitreID    string
	AlertCount int
	TimeWindow string
	Alerts     []model.Alert
}

func (r *AlertRepository) GetRuleTriggerStats(startTime, endTime time.Time, minCount int, sampleSize int) ([]RuleTriggerStats, error) {
	type RuleCount struct {
		RuleID     string
		MitreID    string
		AlertCount int
	}

	var ruleCounts []RuleCount
	err := r.db.Table("alerts").
		Select("rule_id, mitre_id, COUNT(*) as alert_count").
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Where("rule_id IS NOT NULL AND rule_id != ''").
		Group("rule_id, mitre_id").
		Having("COUNT(*) >= ?", minCount).
		Order("alert_count DESC").
		Find(&ruleCounts).Error
	if err != nil {
		return nil, err
	}

	var results []RuleTriggerStats
	for _, rc := range ruleCounts {
		var alerts []model.Alert
		err := r.db.Where("rule_id = ? AND created_at >= ? AND created_at <= ?", rc.RuleID, startTime, endTime).
			Order("created_at DESC").
			Limit(sampleSize).
			Find(&alerts).Error
		if err != nil {
			continue
		}

		var ruleTitle string
		var sigmaRule model.SigmaRule
		if err := r.db.Where("rule_id = ?", rc.RuleID).First(&sigmaRule).Error; err == nil {
			ruleTitle = sigmaRule.Title
		}

		results = append(results, RuleTriggerStats{
			RuleID:     rc.RuleID,
			RuleTitle:  ruleTitle,
			MitreID:    rc.MitreID,
			AlertCount: rc.AlertCount,
			Alerts:     alerts,
		})
	}

	return results, nil
}

func (r *AlertRepository) CountByRuleID(ruleID string) (int, error) {
	var count int64
	err := r.db.Model(&model.Alert{}).Where("rule_id = ?", ruleID).Count(&count).Error
	return int(count), err
}

func (r *AlertRepository) DeleteByRuleID(ruleID string) (int, error) {
	result := r.db.Where("rule_id = ?", ruleID).Delete(&model.Alert{})
	return int(result.RowsAffected), result.Error
}

// DeleteByMitreID deletes all alerts associated with a MITRE ID
func (r *AlertRepository) DeleteByMitreID(mitreID string) (int, error) {
	result := r.db.Where("mitre_id = ?", mitreID).Delete(&model.Alert{})
	return int(result.RowsAffected), result.Error
}

func (r *AlertRepository) NormalizeMitreIDs(ctx context.Context) (int, error) {
	var alerts []model.Alert
	if err := r.db.Where("mitre_id IS NOT NULL AND mitre_id != ''").Find(&alerts).Error; err != nil {
		return 0, err
	}

	updated := 0
	for _, alert := range alerts {
		upperID := strings.ToUpper(alert.MitreID)
		if !strings.HasPrefix(upperID, "T") {
			upperID = "T" + upperID
		}
		if upperID != alert.MitreID {
			if err := r.db.Model(&model.Alert{}).Where("id = ?", alert.ID).Update("mitre_id", upperID).Error; err != nil {
				continue
			}
			updated++
		}
	}
	return updated, nil
}

// CountByMitreIDInTimeRange 统计指定时间范围内同一MITRE ID的告警数
func (r *AlertRepository) CountByMitreIDInTimeRange(mitreID string, hours int) (int64, error) {
	var count int64
	err := r.db.Model(&model.Alert{}).
		Where("mitre_id = ?", mitreID).
		Where("created_at >= NOW() - INTERVAL '1 hour' * ?", hours).
		Count(&count).Error
	return count, err
}

// GetAlertIDsByMitreIDInTimeRange 获取指定时间范围内同一MITRE ID的告警ID列表
func (r *AlertRepository) GetAlertIDsByMitreIDInTimeRange(mitreID string, hours int, limit int) ([]model.Alert, error) {
	var alerts []model.Alert
	err := r.db.Where("mitre_id = ?", mitreID).
		Where("created_at >= NOW() - INTERVAL '1 hour' * ?", hours).
		Order("created_at DESC").
		Limit(limit).
		Find(&alerts).Error
	return alerts, err
}

// FindByAlertIDs 根据告警ID列表获取告警
func (r *AlertRepository) FindByAlertIDs(alertIDs []string) ([]model.Alert, error) {
	if len(alertIDs) == 0 {
		return []model.Alert{}, nil
	}

	type AlertWithHost struct {
		model.Alert
		Hostname  string `json:"hostname"`
		RuleTitle string `json:"rule_title"`
	}

	var alertsWithHost []AlertWithHost
	err := r.db.Table("alerts").
		Select(`alerts.*,
			hosts.hostname,
			COALESCE(
				NULLIF(alerts.rule_title, ''),
				(SELECT title FROM sigma_rules WHERE LOWER(mitre_id) = LOWER(alerts.mitre_id) LIMIT 1),
				alerts.mitre_name
			) as rule_title`).
		Joins("LEFT JOIN hosts ON alerts.host_id = hosts.id").
		Where("alerts.alert_id IN ?", alertIDs).
		Find(&alertsWithHost).Error
	if err != nil {
		return nil, err
	}

	alerts := make([]model.Alert, len(alertsWithHost))
	for i, a := range alertsWithHost {
		alerts[i] = a.Alert
		alerts[i].Hostname = a.Hostname
		if a.RuleTitle != "" {
			alerts[i].RuleTitle = a.RuleTitle
		} else {
			alerts[i].RuleTitle = a.Alert.MitreName
		}
	}

	return alerts, nil
}
