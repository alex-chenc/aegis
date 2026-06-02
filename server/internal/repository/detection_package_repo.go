package repository

import (
	"encoding/json"
	"fmt"
	"server/internal/model"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DetectionPackageRepository struct {
	db *gorm.DB
}

func NewDetectionPackageRepository(db *gorm.DB) *DetectionPackageRepository {
	return &DetectionPackageRepository{db: db}
}

func (r *DetectionPackageRepository) ListEnabled() ([]model.DetectionPackage, error) {
	var packages []model.DetectionPackage
	err := r.db.Where("status = ?", "enabled").Order("updated_at DESC").Find(&packages).Error
	return packages, err
}

func (r *DetectionPackageRepository) GetActiveAllowlistPayload() (string, error) {
	var row struct {
		Version    int64
		ConfigJSON string
	}
	if err := r.db.Table("ebpf_hook_allowlist_configs").
		Select("version, config_json::text AS config_json").
		Where("activated_at IS NOT NULL").
		Order("activated_at DESC").
		Limit(1).
		Scan(&row).Error; err != nil {
		return "", err
	}
	if row.ConfigJSON == "" {
		return "", fmt.Errorf("active allowlist not found")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(row.ConfigJSON), &payload); err != nil || payload == nil {
		payload = map[string]interface{}{}
	}
	if _, exists := payload["version"]; !exists && row.Version > 0 {
		payload["version"] = row.Version
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return row.ConfigJSON, nil
	}
	return string(out), nil
}

func (r *DetectionPackageRepository) UpsertHostStatus(packageID, version string, hostID uuid.UUID, hostname, status string) error {
	now := time.Now()
	hostStatus := &model.DetectionPackageHostStatus{
		PackageID:      packageID,
		Version:        version,
		HostID:         hostID,
		Hostname:       hostname,
		Status:         status,
		UpdatedAt:      now,
		LastReportedAt: &now,
	}
	if err := r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "package_id"},
			{Name: "version"},
			{Name: "host_id"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"hostname": hostname,
			"status": gorm.Expr(
				"CASE WHEN detection_package_host_status.status = 'active' AND EXCLUDED.status = 'installing' THEN detection_package_host_status.status ELSE EXCLUDED.status END",
			),
			"updated_at":       now,
			"last_reported_at": now,
		}),
	}).Create(hostStatus).Error; err != nil {
		return err
	}

	return r.db.Where("package_id = ? AND host_id = ? AND version <> ?", packageID, hostID, version).
		Delete(&model.DetectionPackageHostStatus{}).Error
}
