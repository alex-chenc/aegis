package repository

import (
	"fmt"
	"strings"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/datatypes"
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

func (r *DetectionPackageRepo) DeletePackage(packageID string) error {
	return r.db.Where("package_id = ?", packageID).Delete(&model.DetectionPackage{}).Error
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
		query = query.Where("status IN ?", detectionPackageStatusAliases(status))
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
		query = query.Where("status IN ?", detectionPackageStatusAliases(status))
	}
	if search != "" {
		query = query.Where("package_id LIKE ? OR title LIKE ? OR cve_ids::text LIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	query.Count(&total)
	err := query.Order("updated_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&drafts).Error
	return drafts, total, err
}

// ListPackagesUnified queries both detection_packages and detection_package_drafts with UNION ALL
// for correct pagination across both tables.
func (r *DetectionPackageRepo) ListPackagesUnified(page, pageSize int, status, search string) ([]model.DetectionPackage, int64, error) {
	var packages []model.DetectionPackage
	var total int64

	// Build the published packages subquery
	publishedSelect := `SELECT id, package_id, version, title, description, cve_ids, status,
		package_size, created_at, updated_at FROM detection_packages`
	draftSelect := `SELECT id, package_id, target_version AS version, title, description, cve_ids, status,
		0 AS package_size, created_at, updated_at FROM detection_package_drafts`

	var publishedWhere []string
	var draftWhere []string
	var publishedArgs []interface{}
	var draftArgs []interface{}

	if status != "" {
		statusClause, statusArgs := statusWhereClause(status)
		publishedWhere = append(publishedWhere, statusClause)
		publishedArgs = append(publishedArgs, statusArgs...)
		draftWhere = append(draftWhere, statusClause)
		draftArgs = append(draftArgs, statusArgs...)
	}

	// Search filter
	if search != "" {
		searchPattern := "%" + search + "%"
		publishedWhere = append(publishedWhere, "(package_id LIKE ? OR title LIKE ? OR cve_ids::text LIKE ?)")
		publishedArgs = append(publishedArgs, searchPattern, searchPattern, searchPattern)
		draftWhere = append(draftWhere, "(package_id LIKE ? OR title LIKE ? OR cve_ids::text LIKE ?)")
		draftArgs = append(draftArgs, searchPattern, searchPattern, searchPattern)
	}

	// Build WHERE clauses
	publishedWhereStr := ""
	if len(publishedWhere) > 0 {
		publishedWhereStr = " WHERE " + strings.Join(publishedWhere, " AND ")
	}
	draftWhereStr := ""
	if len(draftWhere) > 0 {
		draftWhereStr = " WHERE " + strings.Join(draftWhere, " AND ")
	}

	publishedSQL := publishedSelect + publishedWhereStr
	draftSQL := draftSelect + draftWhereStr

	// Count total
	countSQL := "SELECT COUNT(*) FROM (" + publishedSQL + " UNION ALL " + draftSQL + ") AS combined"
	allArgs := append(publishedArgs, draftArgs...)
	if err := r.db.Raw(countSQL, allArgs...).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count packages: %w", err)
	}

	// Query with pagination
	querySQL := "SELECT * FROM (" + publishedSQL + " UNION ALL " + draftSQL + ") AS combined ORDER BY updated_at DESC LIMIT ? OFFSET ?"
	queryArgs := append(allArgs, pageSize, (page-1)*pageSize)
	if err := r.db.Raw(querySQL, queryArgs...).Scan(&packages).Error; err != nil {
		return nil, 0, fmt.Errorf("list packages: %w", err)
	}

	return packages, total, nil
}

func detectionPackageStatusAliases(status string) []string {
	switch status {
	case "built":
		return []string{"built", "build_success"}
	case "build_success":
		return []string{"build_success", "built"}
	default:
		return []string{status}
	}
}

func statusWhereClause(status string) (string, []interface{}) {
	aliases := detectionPackageStatusAliases(status)
	if len(aliases) == 1 {
		return "status = ?", []interface{}{aliases[0]}
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(aliases)), ",")
	args := make([]interface{}, 0, len(aliases))
	for _, alias := range aliases {
		args = append(args, alias)
	}
	return "status IN (" + placeholders + ")", args
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
	// Use sql.DB directly to avoid GORM issues in goroutines
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}

	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
	args = append(args, build.Status)
	argIdx++

	if build.ErrorMessage != "" {
		setClauses = append(setClauses, fmt.Sprintf("error_message = $%d", argIdx))
		args = append(args, build.ErrorMessage)
		argIdx++
	}
	if build.BuilderDigest != "" {
		setClauses = append(setClauses, fmt.Sprintf("builder_digest = $%d", argIdx))
		args = append(args, build.BuilderDigest)
		argIdx++
	}
	if build.ClangVersion != "" {
		setClauses = append(setClauses, fmt.Sprintf("clang_version = $%d", argIdx))
		args = append(args, build.ClangVersion)
		argIdx++
	}
	if build.StartedAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("started_at = $%d", argIdx))
		args = append(args, build.StartedAt)
		argIdx++
	}
	if build.FinishedAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("finished_at = $%d", argIdx))
		args = append(args, build.FinishedAt)
		argIdx++
	}
	if build.DurationMs > 0 {
		setClauses = append(setClauses, fmt.Sprintf("duration_ms = $%d", argIdx))
		args = append(args, build.DurationMs)
		argIdx++
	}
	if build.UnsignedPackageObjectKey != "" {
		setClauses = append(setClauses, fmt.Sprintf("unsigned_package_object_key = $%d", argIdx))
		args = append(args, build.UnsignedPackageObjectKey)
		argIdx++
	}
	if build.UnsignedPackageSHA256 != "" {
		setClauses = append(setClauses, fmt.Sprintf("unsigned_package_sha256 = $%d", argIdx))
		args = append(args, build.UnsignedPackageSHA256)
		argIdx++
	}
	if build.UnsignedPackageSize > 0 {
		setClauses = append(setClauses, fmt.Sprintf("unsigned_package_size = $%d", argIdx))
		args = append(args, build.UnsignedPackageSize)
		argIdx++
	}
	if build.BuildLogObjectKey != "" {
		setClauses = append(setClauses, fmt.Sprintf("build_log_object_key = $%d", argIdx))
		args = append(args, build.BuildLogObjectKey)
		argIdx++
	}
	if len(build.HookSummary) > 0 && string(build.HookSummary) != "[]" && string(build.HookSummary) != "{}" {
		setClauses = append(setClauses, fmt.Sprintf("hook_summary = $%d", argIdx))
		args = append(args, string(build.HookSummary))
		argIdx++
	}
	if len(build.ArtifactSummary) > 0 && string(build.ArtifactSummary) != "[]" && string(build.ArtifactSummary) != "{}" {
		setClauses = append(setClauses, fmt.Sprintf("artifact_summary = $%d", argIdx))
		args = append(args, string(build.ArtifactSummary))
		argIdx++
	}
	if build.BuildLog != "" {
		setClauses = append(setClauses, fmt.Sprintf("build_log = $%d", argIdx))
		args = append(args, build.BuildLog)
		argIdx++
	}
	if len(build.EventSchema) > 0 && string(build.EventSchema) != "{}" && string(build.EventSchema) != "[]" {
		setClauses = append(setClauses, fmt.Sprintf("event_schema = $%d", argIdx))
		args = append(args, string(build.EventSchema))
		argIdx++
	}
	if build.ReviewedBy != nil && *build.ReviewedBy != "" {
		setClauses = append(setClauses, fmt.Sprintf("reviewed_by = $%d", argIdx))
		args = append(args, *build.ReviewedBy)
		argIdx++
	}
	if build.ReviewedAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("reviewed_at = $%d", argIdx))
		args = append(args, build.ReviewedAt)
		argIdx++
	}
	if build.ReviewComment != nil && *build.ReviewComment != "" {
		setClauses = append(setClauses, fmt.Sprintf("review_comment = $%d", argIdx))
		args = append(args, *build.ReviewComment)
		argIdx++
	}

	query := "UPDATE detection_package_builds SET " + strings.Join(setClauses, ", ") + fmt.Sprintf(" WHERE id = $%d", argIdx)
	args = append(args, build.ID)

	result, err := sqlDB.Exec(query, args...)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		fmt.Printf("[WARN] UpdateBuild: no rows affected for build %s\n", build.ID)
	}
	return nil
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
	if status.MetricsJSON == nil {
		status.MetricsJSON = datatypes.JSON([]byte("{}"))
	}
	var existing model.DetectionPackageHostStatus
	err := r.db.Where("package_id = ? AND version = ? AND host_id = ?", status.PackageID, status.Version, status.HostID).First(&existing).Error
	if err != nil {
		return r.db.Create(status).Error
	}
	return r.db.Model(&existing).Updates(map[string]interface{}{
		"hostname":        status.Hostname,
		"status":          status.Status,
		"active_artifact": status.ActiveArtifact,
		"error_message":   status.ErrorMessage,
		"last_reported_at": status.LastReportedAt,
	}).Error
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
	query := r.db.Model(&model.RuntimeEvent{}).Where("event_type = ? AND matched_rule_id LIKE ?", "correlation_alert", "%"+packageID+"%")
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
