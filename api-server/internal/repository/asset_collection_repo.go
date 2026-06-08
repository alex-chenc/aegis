package repository

import (
	"fmt"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AssetCollectionRepository 资产采集仓库
type AssetCollectionRepository struct {
	db *gorm.DB
}

// NewAssetCollectionRepository 创建资产采集仓库
func NewAssetCollectionRepository(db *gorm.DB) *AssetCollectionRepository {
	return &AssetCollectionRepository{db: db}
}

// GetConfig 获取采集配置
func (r *AssetCollectionRepository) GetConfig() (*model.AssetCollectionConfig, error) {
	var config model.AssetCollectionConfig
	err := r.db.First(&config).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			nextRun := time.Now().Add(12 * time.Hour)
			config = model.AssetCollectionConfig{
				ID:            uuid.New(),
				Enabled:       true,
				IntervalHours: 12,
				CollectTypes:  []byte(`["process","application_analysis"]`),
				Scope:         "all_hosts",
				NextRunAt:     &nextRun,
			}
			if createErr := r.db.Create(&config).Error; createErr != nil {
				return nil, createErr
			}
			return &config, nil
		}
		return nil, err
	}
	return &config, nil
}

// UpdateConfig 更新采集配置
func (r *AssetCollectionRepository) UpdateConfig(config *model.AssetCollectionConfig) error {
	return r.db.Save(config).Error
}

// CreateTask 创建采集任务
func (r *AssetCollectionRepository) CreateTask(task *model.AssetCollectionTask) error {
	return r.db.Create(task).Error
}

// GetTask 获取采集任务
func (r *AssetCollectionRepository) GetTask(id uuid.UUID) (*model.AssetCollectionTask, error) {
	var task model.AssetCollectionTask
	err := r.db.Where("id = ?", id).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// UpdateTask 更新采集任务
func (r *AssetCollectionRepository) UpdateTask(task *model.AssetCollectionTask) error {
	return r.db.Save(task).Error
}

// ListTasks 列出采集任务
func (r *AssetCollectionRepository) ListTasks(page, pageSize int, status string) ([]model.AssetCollectionTask, int64, error) {
	var tasks []model.AssetCollectionTask
	var total int64

	query := r.db.Model(&model.AssetCollectionTask{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&tasks).Error

	return tasks, total, err
}

// CreateTaskHost 创建任务主机记录
func (r *AssetCollectionRepository) CreateTaskHost(taskHost *model.AssetCollectionTaskHost) error {
	return r.db.Create(taskHost).Error
}

// UpdateTaskHost 更新任务主机记录
func (r *AssetCollectionRepository) UpdateTaskHost(taskHost *model.AssetCollectionTaskHost) error {
	return r.db.Save(taskHost).Error
}

// GetTaskHosts 获取任务的所有主机
func (r *AssetCollectionRepository) GetTaskHosts(taskID uuid.UUID) ([]model.AssetCollectionTaskHost, error) {
	var hosts []model.AssetCollectionTaskHost
	err := r.db.Where("task_id = ?", taskID).Find(&hosts).Error
	return hosts, err
}

// UpsertSoftwareAsset Upsert 软件资产
func (r *AssetCollectionRepository) UpsertSoftwareAsset(asset *model.HostSoftwareAsset) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "host_id"}, {Name: "package_manager"}, {Name: "fingerprint"}},
		DoUpdates: clause.AssignmentColumns([]string{"hostname", "ip_address", "group_name", "os_type", "version", "release", "epoch", "architecture", "source_name", "vendor", "license", "install_paths", "file_count", "package_metadata", "last_modified_at", "last_seen_at", "collected_at", "updated_at"}),
	}).Create(asset).Error
}

// UpsertApplicationAsset Upsert 应用资产
func (r *AssetCollectionRepository) UpsertApplicationAsset(asset *model.HostApplicationAsset) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "host_id"}, {Name: "fingerprint"}},
		DoUpdates: clause.AssignmentColumns([]string{"hostname", "ip_address", "group_name", "os_type", "category", "name", "display_name", "version", "version_source", "install_path", "start_path", "config_paths", "site_paths", "domains", "listen_ports", "run_user", "runtime_name", "runtime_version", "framework_name", "framework_version", "related_pids", "related_packages", "ai_confidence", "ai_evidence", "ai_raw_output", "last_seen_at", "collected_at", "updated_at"}),
	}).Create(asset).Error
}

// CreateProcessSnapshot 创建进程快照
func (r *AssetCollectionRepository) CreateProcessSnapshot(snapshot *model.HostProcessSnapshot) error {
	return r.db.Create(snapshot).Error
}

// CreateToolCall 创建工具调用记录
func (r *AssetCollectionRepository) CreateToolCall(toolCall *model.HostApplicationToolCall) error {
	return r.db.Create(toolCall).Error
}

// GetSoftwareAssets 获取软件资产列表
func (r *AssetCollectionRepository) GetSoftwareAssets(query model.SoftwareAssetQuery) ([]model.HostSoftwareAsset, int64, error) {
	var assets []model.HostSoftwareAsset
	var total int64

	q := r.db.Model(&model.HostSoftwareAsset{}).Where("status = ?", "active")

	if query.Keyword != "" {
		q = q.Where("(name ILIKE ? OR version ILIKE ? OR hostname ILIKE ? OR ip_address ILIKE ?)",
			"%"+query.Keyword+"%", "%"+query.Keyword+"%", "%"+query.Keyword+"%", "%"+query.Keyword+"%")
	}
	if query.HostID != "" {
		q = q.Where("host_id = ?", query.HostID)
	}
	if query.OSType != "" {
		q = q.Where("os_type = ?", query.OSType)
	}
	if query.PackageManager != "" {
		q = q.Where("package_manager = ?", query.PackageManager)
	}
	if query.Status != "" {
		q = q.Where("status = ?", query.Status)
	}

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	err := q.Order("collected_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&assets).Error

	return assets, total, err
}

// GetApplicationAssets 获取应用资产列表
func (r *AssetCollectionRepository) GetApplicationAssets(query model.ApplicationAssetQuery) ([]model.HostApplicationAsset, int64, error) {
	var assets []model.HostApplicationAsset
	var total int64

	q := r.db.Model(&model.HostApplicationAsset{}).Where("status != ?", "deleted")

	if query.Category != "" {
		q = q.Where("category = ?", query.Category)
	}
	if query.Keyword != "" {
		q = q.Where("(name ILIKE ? OR display_name ILIKE ? OR hostname ILIKE ? OR ip_address ILIKE ?)",
			"%"+query.Keyword+"%", "%"+query.Keyword+"%", "%"+query.Keyword+"%", "%"+query.Keyword+"%")
	}
	if query.HostID != "" {
		q = q.Where("host_id = ?", query.HostID)
	}
	if query.MinConfidence > 0 {
		q = q.Where("ai_confidence >= ?", query.MinConfidence)
	}
	if query.ReviewStatus != "" {
		q = q.Where("review_status = ?", query.ReviewStatus)
	}
	if query.Status != "" {
		q = q.Where("status = ?", query.Status)
	}

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	err := q.Order("collected_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&assets).Error

	return assets, total, err
}

// GetApplicationAsset 获取应用资产详情
func (r *AssetCollectionRepository) GetApplicationAsset(id uuid.UUID) (*model.HostApplicationAsset, error) {
	var asset model.HostApplicationAsset
	err := r.db.Where("id = ?", id).First(&asset).Error
	if err != nil {
		return nil, err
	}
	return &asset, nil
}

// UpdateApplicationAsset 更新应用资产
func (r *AssetCollectionRepository) UpdateApplicationAsset(asset *model.HostApplicationAsset) error {
	return r.db.Save(asset).Error
}

// GetToolCallsByApplication 获取应用的工具调用记录
func (r *AssetCollectionRepository) GetToolCallsByApplication(appID uuid.UUID) ([]model.HostApplicationToolCall, error) {
	var calls []model.HostApplicationToolCall
	err := r.db.Where("application_id = ?", appID).Order("created_at DESC").Find(&calls).Error
	return calls, err
}

// GetSoftwareAssetsByHost 获取主机的软件资产
func (r *AssetCollectionRepository) GetSoftwareAssetsByHost(hostID uuid.UUID) ([]model.HostSoftwareAsset, error) {
	var assets []model.HostSoftwareAsset
	err := r.db.Where("host_id = ? AND status = ?", hostID, "active").Find(&assets).Error
	return assets, err
}

// GetApplicationAssetsByHost 获取主机的应用资产
func (r *AssetCollectionRepository) GetApplicationAssetsByHost(hostID uuid.UUID) ([]model.HostApplicationAsset, error) {
	var assets []model.HostApplicationAsset
	err := r.db.Where("host_id = ? AND status IN ?", hostID, []string{"active", "needs_review"}).Find(&assets).Error
	return assets, err
}

// GetSummary 获取资产概览
func (r *AssetCollectionRepository) GetSummary() (*model.AssetSummary, error) {
	summary := &model.AssetSummary{}

	// 软件资产数量
	r.db.Model(&model.HostSoftwareAsset{}).Where("status = ?", "active").Count(&summary.SoftwareCount)

	// 应用资产数量
	r.db.Model(&model.HostApplicationAsset{}).Where("status IN ?", []string{"active", "needs_review"}).Count(&summary.ApplicationCount)

	// 各分类数量
	r.db.Model(&model.HostApplicationAsset{}).Where("category = ? AND status != ?", "database", "deleted").Count(&summary.DatabaseCount)
	r.db.Model(&model.HostApplicationAsset{}).Where("category = ? AND status != ?", "web_service", "deleted").Count(&summary.WebServiceCount)
	r.db.Model(&model.HostApplicationAsset{}).Where("category = ? AND status != ?", "web_framework", "deleted").Count(&summary.WebFrameworkCount)
	r.db.Model(&model.HostApplicationAsset{}).Where("category = ? AND status != ?", "web_site", "deleted").Count(&summary.WebSiteCount)

	// AI 资产分类数量
	r.db.Model(&model.HostApplicationAsset{}).Where("category = ? AND status != ?", "llm_service", "deleted").Count(&summary.LLMServiceCount)
	r.db.Model(&model.HostApplicationAsset{}).Where("category = ? AND status != ?", "ai_agent", "deleted").Count(&summary.AIAgentCount)
	r.db.Model(&model.HostApplicationAsset{}).Where("category = ? AND status != ?", "mcp_server", "deleted").Count(&summary.MCPServerCount)

	// 待复核数量
	r.db.Model(&model.HostApplicationAsset{}).Where("review_status = ?", "pending").Count(&summary.NeedsReviewCount)

	// 最近采集时间
	var lastTask model.AssetCollectionTask
	if err := r.db.Where("status = ?", "completed").Order("finished_at DESC").First(&lastTask).Error; err == nil {
		summary.LastCollectionAt = lastTask.FinishedAt
	}

	return summary, nil
}

// GetLatestSoftwareByHost 获取主机最新的软件资产时间
func (r *AssetCollectionRepository) GetLatestSoftwareByHost(hostID uuid.UUID) (*time.Time, error) {
	var asset model.HostSoftwareAsset
	err := r.db.Where("host_id = ? AND status = ?", hostID, "active").
		Order("collected_at DESC").
		First(&asset).Error
	if err != nil {
		return nil, err
	}
	return &asset.CollectedAt, nil
}

// ApplicationAssetWithHost 带主机信息的应用资产
type ApplicationAssetWithHost struct {
	ID          uuid.UUID  `json:"id"`
	HostID      uuid.UUID  `json:"host_id"`
	Hostname    string     `json:"hostname"`
	IPAddress   string     `json:"ip_address"`
	OsType      string     `json:"os_type"`
	Name        string     `json:"name"`
	DisplayName string     `json:"display_name"`
	Version     string     `json:"version"`
	Category    string     `json:"category"`
	ListenPorts string     `json:"listen_ports"`
	RunUser     string     `json:"run_user"`
	CollectedAt time.Time  `json:"collected_at"`
}

// SearchApplicationAssets 按应用名搜索哪些主机安装了该应用（JOIN hosts 表）
func (r *AssetCollectionRepository) SearchApplicationAssets(appName string, page, pageSize int) ([]ApplicationAssetWithHost, int64, error) {
	if appName == "" {
		return nil, 0, fmt.Errorf("app_name is required")
	}

	var total int64
	query := r.db.Model(&model.HostApplicationAsset{}).
		Where("status = ? AND (name ILIKE ? OR display_name ILIKE ?)", "active", "%"+appName+"%", "%"+appName+"%")
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count application assets: %w", err)
	}

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	var results []ApplicationAssetWithHost
	offset := (page - 1) * pageSize
	err := r.db.Table("host_application_assets a").
		Select(`a.id, a.host_id, h.hostname, h.ip_address, h.os_type,
			a.name, a.display_name, a.version, a.category,
			a.listen_ports::text, a.run_user, a.collected_at`).
		Joins("LEFT JOIN hosts h ON h.id = a.host_id").
		Where("a.status = ? AND (a.name ILIKE ? OR a.display_name ILIKE ?)", "active", "%"+appName+"%", "%"+appName+"%").
		Order("h.hostname, a.name").
		Offset(offset).Limit(pageSize).
		Scan(&results).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to search application assets: %w", err)
	}

	return results, total, nil
}
