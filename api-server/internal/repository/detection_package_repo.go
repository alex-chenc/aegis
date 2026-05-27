package repository

import (
	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DetectionPackageRepo struct {
	db *gorm.DB
}

func NewDetectionPackageRepo(db *gorm.DB) *DetectionPackageRepo {
	return &DetectionPackageRepo{db: db}
}

func (r *DetectionPackageRepo) CreateDraft(draft *model.DetectionPackageDraft) error {
	return r.db.Create(draft).Error
}

func (r *DetectionPackageRepo) UpdateDraft(draft *model.DetectionPackageDraft) error {
	return r.db.Save(draft).Error
}

func (r *DetectionPackageRepo) GetDraftByPackageID(packageID string) (*model.DetectionPackageDraft, error) {
	var draft model.DetectionPackageDraft
	err := r.db.Where("package_id = ?", packageID).First(&draft).Error
	return &draft, err
}

func (r *DetectionPackageRepo) GetDraftByID(id uuid.UUID) (*model.DetectionPackageDraft, error) {
	var draft model.DetectionPackageDraft
	err := r.db.Where("id = ?", id).First(&draft).Error
	return &draft, err
}

func (r *DetectionPackageRepo) DeleteDraftByPackageID(packageID string) error {
	return r.db.Where("package_id = ?", packageID).Delete(&model.DetectionPackageDraft{}).Error
}

func (r *DetectionPackageRepo) CreatePackage(pkg *model.DetectionPackage) error {
	return r.db.Create(pkg).Error
}

func (r *DetectionPackageRepo) UpdatePackage(pkg *model.DetectionPackage) error {
	return r.db.Save(pkg).Error
}

func (r *DetectionPackageRepo) GetPackage(packageID, version string) (*model.DetectionPackage, error) {
	var pkg model.DetectionPackage
	err := r.db.Where("package_id = ? AND version = ?", packageID, version).First(&pkg).Error
	return &pkg, err
}

func (r *DetectionPackageRepo) GetLatestPackage(packageID string) (*model.DetectionPackage, error) {
	var pkg model.DetectionPackage
	err := r.db.Where("package_id = ?", packageID).Order("created_at DESC").First(&pkg).Error
	return &pkg, err
}

func (r *DetectionPackageRepo) ListPackages(page, pageSize int, status, search string) ([]model.DetectionPackage, int64, error) {
	var packages []model.DetectionPackage
	var total int64
	query := r.db.Model(&model.DetectionPackage{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if search != "" {
		query = query.Where("package_id LIKE ? OR title LIKE ? OR cve_ids::text LIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	query.Count(&total)
	err := query.Order("updated_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&packages).Error
	return packages, total, err
}

func (r *DetectionPackageRepo) ListDrafts(page, pageSize int, status, search string) ([]model.DetectionPackageDraft, int64, error) {
	var drafts []model.DetectionPackageDraft
	var total int64
	query := r.db.Model(&model.DetectionPackageDraft{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if search != "" {
		query = query.Where("package_id LIKE ? OR title LIKE ? OR cve_ids::text LIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	query.Count(&total)
	err := query.Order("updated_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&drafts).Error
	return drafts, total, err
}

func (r *DetectionPackageRepo) GetEnabledPackage(packageID string) (*model.DetectionPackage, error) {
	var pkg model.DetectionPackage
	err := r.db.Where("package_id = ? AND status = ?", packageID, "enabled").First(&pkg).Error
	return &pkg, err
}

func (r *DetectionPackageRepo) GetPackageByVersion(packageID, version string) (*model.DetectionPackage, error) {
	var pkg model.DetectionPackage
	err := r.db.Where("package_id = ? AND version = ?", packageID, version).First(&pkg).Error
	return &pkg, err
}

func (r *DetectionPackageRepo) DisableOtherVersions(packageID, currentVersion string) error {
	return r.db.Model(&model.DetectionPackage{}).
		Where("package_id = ? AND version != ? AND status = ?", packageID, currentVersion, "enabled").
		Update("status", "disabled").Error
}

func (r *DetectionPackageRepo) CreateBuild(build *model.DetectionPackageBuild) error {
	return r.db.Create(build).Error
}

func (r *DetectionPackageRepo) UpdateBuild(build *model.DetectionPackageBuild) error {
	return r.db.Save(build).Error
}

func (r *DetectionPackageRepo) GetBuild(id uuid.UUID) (*model.DetectionPackageBuild, error) {
	var build model.DetectionPackageBuild
	err := r.db.Where("id = ?", id).First(&build).Error
	return &build, err
}

func (r *DetectionPackageRepo) GetLatestBuild(packageID string) (*model.DetectionPackageBuild, error) {
	var build model.DetectionPackageBuild
	err := r.db.Where("package_id = ?", packageID).Order("created_at DESC").First(&build).Error
	return &build, err
}

func (r *DetectionPackageRepo) UpsertHostStatus(status *model.DetectionPackageHostStatus) error {
	return r.db.Where("package_id = ? AND version = ? AND host_id = ?", status.PackageID, status.Version, status.HostID).
		Assign(status).FirstOrCreate(status).Error
}

func (r *DetectionPackageRepo) ListHostStatus(packageID, version string, page, pageSize int) ([]model.DetectionPackageHostStatus, int64, error) {
	var statuses []model.DetectionPackageHostStatus
	var total int64
	query := r.db.Model(&model.DetectionPackageHostStatus{}).Where("package_id = ?", packageID)
	if version != "" {
		query = query.Where("version = ?", version)
	}
	query.Count(&total)
	err := query.Order("updated_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&statuses).Error
	return statuses, total, err
}

func (r *DetectionPackageRepo) CountHostStatus(packageID, version string) (total, active, failed int64, err error) {
	query := r.db.Model(&model.DetectionPackageHostStatus{}).Where("package_id = ? AND version = ?", packageID, version)
	err = query.Count(&total).Error
	if err != nil {
		return
	}
	err = query.Where("status = ?", "active").Count(&active).Error
	if err != nil {
		return
	}
	err = query.Where("status IN ?", []string{"load_failed", "signature_failed", "blocked_by_hook_allowlist"}).Count(&failed).Error
	return
}

func (r *DetectionPackageRepo) CreateOperation(op *model.DetectionPackageOperation) error {
	return r.db.Create(op).Error
}

func (r *DetectionPackageRepo) GetActiveAllowlist() (*model.EBPFHookAllowlistConfig, error) {
	var config model.EBPFHookAllowlistConfig
	err := r.db.Where("activated_at IS NOT NULL").Order("activated_at DESC").First(&config).Error
	return &config, err
}

func (r *DetectionPackageRepo) CreateAllowlist(config *model.EBPFHookAllowlistConfig) error {
	return r.db.Create(config).Error
}

func (r *DetectionPackageRepo) CreateCorrelationRule(rule *model.CorrelationRule) error {
	return r.db.Create(rule).Error
}

func (r *DetectionPackageRepo) ListCorrelationRules(packageID, version string) ([]model.CorrelationRule, error) {
	var rules []model.CorrelationRule
	err := r.db.Where("package_id = ? AND package_version = ?", packageID, version).Find(&rules).Error
	return rules, err
}

func (r *DetectionPackageRepo) ListAlertsByPackageID(packageID string, page, pageSize int) ([]model.RuntimeEvent, int64, error) {
	var events []model.RuntimeEvent
	var total int64
	query := r.db.Model(&model.RuntimeEvent{}).Where("event_type = ? AND matched_rule_id LIKE ?", "correlation_alert", packageID+"%")
	query.Count(&total)
	err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&events).Error
	return events, total, err
}

func (r *DetectionPackageRepo) ListAllowlistHistory(page, pageSize int) ([]model.EBPFHookAllowlistConfig, int64, error) {
	var configs []model.EBPFHookAllowlistConfig
	var total int64
	query := r.db.Model(&model.EBPFHookAllowlistConfig{})
	query.Count(&total)
	err := query.Order("version DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&configs).Error
	return configs, total, err
}
