package repository

import (
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WeakPasswordRepository struct {
	db *gorm.DB
}

func NewWeakPasswordRepository(db *gorm.DB) *WeakPasswordRepository {
	return &WeakPasswordRepository{db: db}
}

func (r *WeakPasswordRepository) DB() *gorm.DB {
	return r.db
}

type WeakPasswordApplicationAssetFilter struct {
	HostIDs          []uuid.UUID
	ApplicationTypes []string
	Keyword          string
	OnlineAgentsOnly bool
	Page             int
	PageSize         int
}

func (r *WeakPasswordRepository) ListApplicationAssets(filter WeakPasswordApplicationAssetFilter) ([]model.HostApplicationAsset, int64, error) {
	var assets []model.HostApplicationAsset
	var total int64
	q := r.db.Model(&model.HostApplicationAsset{}).Where("host_application_assets.status != ?", "deleted")
	if len(filter.HostIDs) > 0 {
		q = q.Where("host_application_assets.host_id IN ?", filter.HostIDs)
	}
	if len(filter.ApplicationTypes) > 0 {
		q = q.Where("(host_application_assets.category IN ? OR host_application_assets.name IN ?)", filter.ApplicationTypes, filter.ApplicationTypes)
	}
	if filter.Keyword != "" {
		like := "%" + filter.Keyword + "%"
		q = q.Where("(host_application_assets.name ILIKE ? OR host_application_assets.display_name ILIKE ? OR host_application_assets.hostname ILIKE ? OR host_application_assets.ip_address ILIKE ?)", like, like, like, like)
	}
	if filter.OnlineAgentsOnly {
		q = q.Joins("JOIN hosts h ON h.id = host_application_assets.host_id").
			Where("h.last_heartbeat_at > ?", time.Now().Add(-2*time.Minute))
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 200
	}
	err := q.Order("collected_at DESC").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&assets).Error
	return assets, total, err
}

func (r *WeakPasswordRepository) CreateAnalysisWithCandidates(analysis *model.WeakPasswordAssetAppAnalysis, candidates []model.WeakPasswordCandidateApplication) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(analysis).Error; err != nil {
			return err
		}
		if len(candidates) > 0 {
			// Use the same conflict target as the database constraint so repeated
			// analyses refresh the latest candidate row for the same collected asset.
			for i := range candidates {
				if err := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "host_id"}, {Name: "asset_id"}, {Name: "application_type"}},
					DoUpdates: clause.AssignmentColumns([]string{"analysis_id", "confidence", "candidate_paths_json", "extractor_plan_json", "asset_evidence_json", "ai_reason", "status"}),
				}).Create(&candidates[i]).Error; err != nil {
					return err
				}
				q := tx.Where("host_id = ? AND application_type = ?", candidates[i].HostID, candidates[i].ApplicationType)
				if candidates[i].AssetID == nil {
					q = q.Where("asset_id IS NULL")
				} else {
					q = q.Where("asset_id = ?", *candidates[i].AssetID)
				}
				var persisted model.WeakPasswordCandidateApplication
				if err := q.First(&persisted).Error; err != nil {
					return err
				}
				candidates[i] = persisted
			}
		}
		return nil
	})
}

func (r *WeakPasswordRepository) ListCandidateApplications(analysisID *uuid.UUID, hostID *uuid.UUID, applicationType, confidence string, page, pageSize int) ([]model.WeakPasswordCandidateApplication, int64, error) {
	var items []model.WeakPasswordCandidateApplication
	var total int64
	q := r.db.Model(&model.WeakPasswordCandidateApplication{})

	// If no analysis_id specified, use the latest analysis
	if analysisID == nil {
		var latestAnalysis model.WeakPasswordAssetAppAnalysis
		if err := r.db.Order("created_at DESC").First(&latestAnalysis).Error; err == nil {
			analysisID = &latestAnalysis.ID
		}
	}

	if analysisID != nil {
		q = q.Where("analysis_id = ?", *analysisID)
	}
	if hostID != nil {
		q = q.Where("host_id = ?", *hostID)
	}
	if applicationType != "" {
		q = q.Where("application_type = ?", applicationType)
	}
	switch confidence {
	case "high":
		q = q.Where("confidence >= ?", 0.8)
	case "medium":
		q = q.Where("confidence >= ? AND confidence < ?", 0.5, 0.8)
	case "low":
		q = q.Where("confidence < ?", 0.5)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	return items, total, err
}

func (r *WeakPasswordRepository) GetCandidateApplication(id uuid.UUID) (*model.WeakPasswordCandidateApplication, error) {
	var candidate model.WeakPasswordCandidateApplication
	if err := r.db.Where("id = ?", id).First(&candidate).Error; err != nil {
		return nil, err
	}
	return &candidate, nil
}

func (r *WeakPasswordRepository) CreateTaskBundle(task *model.WeakPasswordScanTask, host *model.WeakPasswordScanHost, app *model.WeakPasswordScanApplication, plan *model.WeakPasswordCollectionPlan) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		if err := tx.Create(host).Error; err != nil {
			return err
		}
		if err := tx.Create(app).Error; err != nil {
			return err
		}
		if err := tx.Create(plan).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *WeakPasswordRepository) GetTask(id uuid.UUID) (*model.WeakPasswordScanTask, error) {
	var task model.WeakPasswordScanTask
	if err := r.db.Where("id = ?", id).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *WeakPasswordRepository) ListTasks(page, pageSize int, status string) ([]model.WeakPasswordScanTask, int64, error) {
	var tasks []model.WeakPasswordScanTask
	var total int64
	q := r.db.Model(&model.WeakPasswordScanTask{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&tasks).Error
	return tasks, total, err
}

func (r *WeakPasswordRepository) GetScanApplicationByTask(taskID uuid.UUID) (*model.WeakPasswordScanApplication, error) {
	var app model.WeakPasswordScanApplication
	if err := r.db.Where("task_id = ?", taskID).Order("created_at ASC").First(&app).Error; err != nil {
		return nil, err
	}
	return &app, nil
}

func (r *WeakPasswordRepository) ListScanApplicationsByCandidateIDs(candidateIDs []uuid.UUID) ([]model.WeakPasswordScanApplication, error) {
	var apps []model.WeakPasswordScanApplication
	if len(candidateIDs) == 0 {
		return apps, nil
	}
	err := r.db.
		Where("candidate_application_id IN ?", candidateIDs).
		Order("updated_at DESC, created_at DESC").
		Find(&apps).Error
	return apps, err
}

func (r *WeakPasswordRepository) ListScanHosts(taskID uuid.UUID) ([]model.WeakPasswordScanHost, error) {
	var hosts []model.WeakPasswordScanHost
	err := r.db.Where("task_id = ?", taskID).Order("created_at ASC").Find(&hosts).Error
	return hosts, err
}

type WeakPasswordScanHostWithInfo struct {
	model.WeakPasswordScanHost
	Hostname  string `json:"hostname"`
	IPAddress string `json:"ip_address"`
}

func (r *WeakPasswordRepository) ListScanHostsWithInfo(taskID uuid.UUID, page, pageSize int) ([]WeakPasswordScanHostWithInfo, int64, error) {
	var hosts []WeakPasswordScanHostWithInfo
	var total int64
	q := r.db.Table("weak_password_scan_hosts AS scan_hosts").
		Select("scan_hosts.*, hosts.hostname AS hostname, hosts.ip_address AS ip_address").
		Joins("LEFT JOIN hosts ON hosts.id = scan_hosts.host_id").
		Where("scan_hosts.task_id = ?", taskID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	err := q.
		Order("scan_hosts.created_at ASC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&hosts).Error
	return hosts, total, err
}

func (r *WeakPasswordRepository) ListFindings(taskID uuid.UUID, page, pageSize int) ([]model.WeakPasswordFinding, int64, error) {
	var findings []model.WeakPasswordFinding
	var total int64
	q := r.db.Model(&model.WeakPasswordFinding{}).Where("task_id = ?", taskID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&findings).Error
	return findings, total, err
}

func (r *WeakPasswordRepository) ListFindingsByScanApplicationIDs(scanApplicationIDs []uuid.UUID) ([]model.WeakPasswordFinding, error) {
	var findings []model.WeakPasswordFinding
	if len(scanApplicationIDs) == 0 {
		return findings, nil
	}
	err := r.db.
		Where("scan_application_id IN ?", scanApplicationIDs).
		Order("created_at DESC").
		Find(&findings).Error
	return findings, err
}

func (r *WeakPasswordRepository) GetFinding(id uuid.UUID) (*model.WeakPasswordFinding, error) {
	var finding model.WeakPasswordFinding
	if err := r.db.Where("id = ?", id).First(&finding).Error; err != nil {
		return nil, err
	}
	return &finding, nil
}

func (r *WeakPasswordRepository) ListCollectionErrors(taskID uuid.UUID, page, pageSize int) ([]model.WeakPasswordCollectionError, int64, error) {
	var errors []model.WeakPasswordCollectionError
	var total int64
	q := r.db.Model(&model.WeakPasswordCollectionError{}).Where("task_id = ?", taskID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&errors).Error
	return errors, total, err
}

func (r *WeakPasswordRepository) UpdateTask(task *model.WeakPasswordScanTask) error {
	return r.db.Save(task).Error
}

func (r *WeakPasswordRepository) DeleteTask(taskID uuid.UUID) error {
	return r.db.Delete(&model.WeakPasswordScanTask{}, "id = ?", taskID).Error
}

func (r *WeakPasswordRepository) UpdateScanHost(host *model.WeakPasswordScanHost) error {
	return r.db.Save(host).Error
}

func (r *WeakPasswordRepository) UpdateScanApplication(app *model.WeakPasswordScanApplication) error {
	return r.db.Save(app).Error
}

func (r *WeakPasswordRepository) CreateToolCall(call *model.WeakPasswordAgentToolCall) error {
	return r.db.Create(call).Error
}

func (r *WeakPasswordRepository) UpdateToolCall(call *model.WeakPasswordAgentToolCall) error {
	return r.db.Save(call).Error
}

func (r *WeakPasswordRepository) LastToolCall(taskID uuid.UUID) (*model.WeakPasswordAgentToolCall, error) {
	var call model.WeakPasswordAgentToolCall
	if err := r.db.Where("task_id = ?", taskID).Order("created_at DESC").First(&call).Error; err != nil {
		return nil, err
	}
	return &call, nil
}

func (r *WeakPasswordRepository) ListToolCalls(taskID uuid.UUID, page, pageSize int) ([]model.WeakPasswordAgentToolCall, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	return r.ListToolCallsByOffset(taskID, (page-1)*pageSize, pageSize)
}

func (r *WeakPasswordRepository) ListToolCallsByOffset(taskID uuid.UUID, offset, limit int) ([]model.WeakPasswordAgentToolCall, int64, error) {
	var calls []model.WeakPasswordAgentToolCall
	var total int64
	q := r.db.Model(&model.WeakPasswordAgentToolCall{}).Where("task_id = ?", taskID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 10
	}
	err := q.Order("created_at ASC").Offset(offset).Limit(limit).Find(&calls).Error
	return calls, total, err
}

func (r *WeakPasswordRepository) CreateCollectionError(errRecord *model.WeakPasswordCollectionError) error {
	return r.db.Create(errRecord).Error
}

func (r *WeakPasswordRepository) CreateFindings(findings []model.WeakPasswordFinding) error {
	if len(findings) == 0 {
		return nil
	}
	// Use upsert to avoid duplicate findings based on (task_id, host_id, source_path, field_path, account)
	for i := range findings {
		if err := r.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "task_id"}, {Name: "host_id"}, {Name: "source_path"}, {Name: "field_path"}, {Name: "account"}},
			DoUpdates: clause.AssignmentColumns([]string{"match_status", "matched_password_mask", "matched_password_encrypted", "match_source", "match_rule", "confidence", "ai_reason"}),
		}).Create(&findings[i]).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *WeakPasswordRepository) GetDefaultDictionary() (*model.WeakPasswordDictionary, error) {
	var dict model.WeakPasswordDictionary
	if err := r.db.Where("dictionary_type = ?", model.DictTypeDefault1000).First(&dict).Error; err != nil {
		return nil, err
	}
	return &dict, nil
}

func (r *WeakPasswordRepository) UpsertDictionary(dict *model.WeakPasswordDictionary) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "status", "entry_count", "categories", "generation_policy_json", "prompt_summary", "llm_model", "updated_at"}),
	}).Create(dict).Error
}

func (r *WeakPasswordRepository) CreateDictionary(dict *model.WeakPasswordDictionary, entries []model.WeakPasswordDictionaryEntry) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(dict).Error; err != nil {
			return err
		}
		if len(entries) > 0 {
			if err := tx.Create(&entries).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *WeakPasswordRepository) UpsertDictionaryEntries(entries []model.WeakPasswordDictionaryEntry) error {
	if len(entries) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "dictionary_id"}, {Name: "candidate_hash"}},
		DoNothing: true,
	}).Create(&entries).Error
}

func (r *WeakPasswordRepository) CountDictionaryEntries(dictionaryID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.Model(&model.WeakPasswordDictionaryEntry{}).Where("dictionary_id = ?", dictionaryID).Count(&count).Error
	return count, err
}

func (r *WeakPasswordRepository) ListDictionaryEntries(dictionaryIDs []uuid.UUID) ([]model.WeakPasswordDictionaryEntry, error) {
	var entries []model.WeakPasswordDictionaryEntry
	q := r.db.Model(&model.WeakPasswordDictionaryEntry{})
	if len(dictionaryIDs) > 0 {
		q = q.Where("dictionary_id IN ?", dictionaryIDs)
	}
	err := q.Find(&entries).Error
	return entries, err
}

func (r *WeakPasswordRepository) ListDictionaryEntriesPaged(dictionaryID uuid.UUID, page, pageSize int) ([]model.WeakPasswordDictionaryEntry, int64, error) {
	var entries []model.WeakPasswordDictionaryEntry
	var total int64
	q := r.db.Model(&model.WeakPasswordDictionaryEntry{}).Where("dictionary_id = ?", dictionaryID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	err := q.Order("created_at ASC, candidate ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&entries).Error
	return entries, total, err
}

func (r *WeakPasswordRepository) ListDictionaries(page, pageSize int) ([]model.WeakPasswordDictionary, int64, error) {
	var dictionaries []model.WeakPasswordDictionary
	var total int64
	q := r.db.Model(&model.WeakPasswordDictionary{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&dictionaries).Error
	return dictionaries, total, err
}

func (r *WeakPasswordRepository) ListDictionariesByTypes(types []string) ([]model.WeakPasswordDictionary, error) {
	var dictionaries []model.WeakPasswordDictionary
	if len(types) == 0 {
		return dictionaries, nil
	}
	err := r.db.
		Where("dictionary_type IN ? AND status = ?", types, "enabled").
		Order("created_at DESC").
		Find(&dictionaries).Error
	return dictionaries, err
}

func (r *WeakPasswordRepository) CreateRevealAudit(audit *model.WeakPasswordRevealAudit) error {
	return r.db.Create(audit).Error
}
