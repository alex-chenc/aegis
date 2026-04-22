package repository

import (
	"api-server/internal/model"
)

// FindByIDs 根据内部UUID列表获取告警
func (r *AlertRepository) FindByIDs(ids []string) ([]model.Alert, error) {
	if len(ids) == 0 {
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
		Where("alerts.id IN ?", ids).
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
