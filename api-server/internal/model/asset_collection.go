package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// AssetCollectionConfig 资产周期采集配置
type AssetCollectionConfig struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Enabled       bool           `gorm:"default:true" json:"enabled"`
	IntervalHours int            `gorm:"default:12" json:"interval_hours"`
	CollectTypes  datatypes.JSON `gorm:"type:jsonb;default:'[\"process\",\"application_analysis\"]'" json:"collect_types"`
	Scope         string         `gorm:"type:varchar(32);default:'all_hosts'" json:"scope"`
	NextRunAt     *time.Time     `json:"next_run_at"`
	LastRunAt     *time.Time     `json:"last_run_at"`
	UpdatedBy     string         `json:"updated_by"`
	CreatedAt     time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (AssetCollectionConfig) TableName() string {
	return "asset_collection_configs"
}

// AssetCollectionTask 采集任务
type AssetCollectionTask struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskType      string         `gorm:"type:varchar(32);default:'full'" json:"task_type"`
	TriggerSource string         `gorm:"type:varchar(32);default:'manual'" json:"trigger_source"`
	Scope         string         `gorm:"type:varchar(32);default:'hosts'" json:"scope"`
	HostFilter    datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"host_filter"`
	CollectTypes  datatypes.JSON `gorm:"type:jsonb;default:'[\"process\",\"application_analysis\"]'" json:"collect_types"`
	Status        string         `gorm:"type:varchar(32);default:'collecting'" json:"status"`
	TotalHosts    int            `gorm:"default:0" json:"total_hosts"`
	SuccessHosts  int            `gorm:"default:0" json:"success_hosts"`
	FailedHosts   int            `gorm:"default:0" json:"failed_hosts"`
	CurrentStage  string         `json:"current_stage"`
	ErrorMessage  string         `json:"error_message"`
	RequestedBy   string         `json:"requested_by"`
	StartedAt     *time.Time     `json:"started_at"`
	FinishedAt    *time.Time     `json:"finished_at"`
	CreatedAt     time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (AssetCollectionTask) TableName() string {
	return "asset_collection_tasks"
}

// AssetCollectionTaskHost 任务主机执行明细
type AssetCollectionTaskHost struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskID            uuid.UUID  `gorm:"type:uuid" json:"task_id"`
	HostID            uuid.UUID  `gorm:"type:uuid" json:"host_id"`
	Hostname          string     `json:"hostname"`
	IPAddress         string     `json:"ip_address"`
	Status            string     `gorm:"type:varchar(32);default:'collecting'" json:"status"`
	CollectStartedAt  *time.Time `json:"collect_started_at"`
	CollectFinishedAt *time.Time `json:"collect_finished_at"`
	SoftwareCount     int        `gorm:"default:0" json:"software_count"`
	ProcessCount      int        `gorm:"default:0" json:"process_count"`
	ApplicationCount  int        `gorm:"default:0" json:"application_count"`
	ErrorMessage      string     `json:"error_message"`
	RawSnapshotID     *uuid.UUID `json:"raw_snapshot_id"`
	CreatedAt         time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (AssetCollectionTaskHost) TableName() string {
	return "asset_collection_task_hosts"
}

// HostSoftwareAsset 主机软件资产
type HostSoftwareAsset struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	HostID          uuid.UUID      `gorm:"type:uuid" json:"host_id"`
	Hostname        string         `json:"hostname"`
	IPAddress       string         `json:"ip_address"`
	GroupName       string         `gorm:"default:'默认分组'" json:"group_name"`
	OSType          string         `json:"os_type"`
	PackageManager  string         `json:"package_manager"`
	Name            string         `json:"name"`
	Version         string         `json:"version"`
	Release         string         `json:"release"`
	Epoch           string         `json:"epoch"`
	Architecture    string         `json:"architecture"`
	SourceName      string         `json:"source_name"`
	Vendor          string         `json:"vendor"`
	License         string         `json:"license"`
	InstallPaths    datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"install_paths"`
	FileCount       int            `gorm:"default:0" json:"file_count"`
	PackageMetadata datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"package_metadata"`
	Fingerprint     string         `json:"fingerprint"`
	Status          string         `gorm:"type:varchar(32);default:'active'" json:"status"`
	LastModifiedAt  *time.Time     `json:"last_modified_at"`
	FirstSeenAt     time.Time      `gorm:"autoCreateTime" json:"first_seen_at"`
	LastSeenAt      time.Time      `json:"last_seen_at"`
	CollectedAt     time.Time      `json:"collected_at"`
	CreatedAt       time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (HostSoftwareAsset) TableName() string {
	return "host_software_assets"
}

// HostProcessSnapshot 主机进程快照
type HostProcessSnapshot struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskID           *uuid.UUID     `gorm:"type:uuid" json:"task_id"`
	HostID           uuid.UUID      `gorm:"type:uuid" json:"host_id"`
	Hostname         string         `json:"hostname"`
	IPAddress        string         `json:"ip_address"`
	ProcessCount     int            `gorm:"default:0" json:"process_count"`
	ListenPortCount  int            `gorm:"default:0" json:"listen_port_count"`
	SnapshotHash     string         `json:"snapshot_hash"`
	SnapshotJSON     datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"snapshot_json"`
	RedactionSummary datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"redaction_summary"`
	CollectedAt      time.Time      `gorm:"autoCreateTime" json:"collected_at"`
	CreatedAt        time.Time      `gorm:"autoCreateTime" json:"created_at"`
}

func (HostProcessSnapshot) TableName() string {
	return "host_process_snapshots"
}

// HostApplicationAsset 主机应用资产
type HostApplicationAsset struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	HostID           uuid.UUID      `gorm:"type:uuid" json:"host_id"`
	Hostname         string         `json:"hostname"`
	IPAddress        string         `json:"ip_address"`
	GroupName        string         `gorm:"default:'默认分组'" json:"group_name"`
	OSType           string         `json:"os_type"`
	Category         string         `gorm:"type:varchar(32);default:'unknown'" json:"category"`
	Name             string         `json:"name"`
	DisplayName      string         `json:"display_name"`
	Version          string         `json:"version"`
	VersionSource    string         `json:"version_source"`
	InstallPath      string         `json:"install_path"`
	StartPath        string         `json:"start_path"`
	ConfigPaths      datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"config_paths"`
	SitePaths        datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"site_paths"`
	Domains          datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"domains"`
	ListenPorts      datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"listen_ports"`
	RunUser          string         `json:"run_user"`
	RuntimeName      string         `json:"runtime_name"`
	RuntimeVersion   string         `json:"runtime_version"`
	FrameworkName    string         `json:"framework_name"`
	FrameworkVersion string         `json:"framework_version"`
	RelatedPIDs      datatypes.JSON `gorm:"column:related_pids;type:jsonb;default:'[]'" json:"related_pids"`
	RelatedPackages  datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"related_packages"`
	AIConfidence     float64        `gorm:"type:numeric(4,3);default:0" json:"ai_confidence"`
	AIEvidence       datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"ai_evidence"`
	AIRawOutput      datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"ai_raw_output"`
	ManualOverrides  datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"manual_overrides"`
	ReviewStatus     string         `gorm:"type:varchar(32);default:'auto'" json:"review_status"`
	Status           string         `gorm:"type:varchar(32);default:'active'" json:"status"`
	Fingerprint      string         `json:"fingerprint"`
	FirstSeenAt      time.Time      `gorm:"autoCreateTime" json:"first_seen_at"`
	LastSeenAt       time.Time      `json:"last_seen_at"`
	CollectedAt      time.Time      `json:"collected_at"`
	CreatedAt        time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (HostApplicationAsset) TableName() string {
	return "host_application_assets"
}

// HostApplicationToolCall 工具调用记录
type HostApplicationToolCall struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskID          *uuid.UUID     `gorm:"type:uuid" json:"task_id"`
	ApplicationID   *uuid.UUID     `gorm:"type:uuid" json:"application_id"`
	HostID          uuid.UUID      `gorm:"type:uuid" json:"host_id"`
	CallID          string         `gorm:"uniqueIndex" json:"call_id"`
	ToolName        string         `json:"tool_name"`
	ArgumentsJSON   datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"arguments_json"`
	ResultJSON      datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"result_json"`
	Success         bool           `gorm:"default:false" json:"success"`
	ErrorMessage    string         `json:"error_message"`
	ExecutionTimeMs int64          `gorm:"default:0" json:"execution_time_ms"`
	CreatedAt       time.Time      `gorm:"autoCreateTime" json:"created_at"`
}

func (HostApplicationToolCall) TableName() string {
	return "host_application_tool_calls"
}

// TriggerAssetCollectionRequest 触发采集请求
type TriggerAssetCollectionRequest struct {
	Scope   string   `json:"scope" binding:"required"`
	HostIDs []string `json:"host_ids"`
	Types   []string `json:"types" binding:"required"`
	Force   bool     `json:"force"`
}

// AssetSummary 资产概览
type AssetSummary struct {
	SoftwareCount     int64      `json:"software_count"`
	ApplicationCount  int64      `json:"application_count"`
	DatabaseCount     int64      `json:"database_count"`
	WebServiceCount   int64      `json:"web_service_count"`
	WebFrameworkCount int64      `json:"web_framework_count"`
	WebSiteCount      int64      `json:"web_site_count"`
	LLMServiceCount   int64      `json:"llm_service_count"`
	AIAgentCount      int64      `json:"ai_agent_count"`
	MCPServerCount    int64      `json:"mcp_server_count"`
	NeedsReviewCount  int64      `json:"needs_review_count"`
	LastCollectionAt  *time.Time `json:"last_collection_at"`
}

// SoftwareAssetQuery 软件资产查询参数
type SoftwareAssetQuery struct {
	Page           int    `form:"page"`
	PageSize       int    `form:"page_size"`
	Keyword        string `form:"keyword"`
	HostID         string `form:"host_id"`
	GroupID        string `form:"group_id"`
	OSType         string `form:"os_type"`
	PackageManager string `form:"package_manager"`
	Status         string `form:"status"`
	StartTime      string `form:"start_time"`
	EndTime        string `form:"end_time"`
}

// ApplicationAssetQuery 应用资产查询参数
type ApplicationAssetQuery struct {
	Page          int     `form:"page"`
	PageSize      int     `form:"page_size"`
	Keyword       string  `form:"keyword"`
	HostID        string  `form:"host_id"`
	GroupID       string  `form:"group_id"`
	Category      string  `form:"category"`
	MinConfidence float64 `form:"min_confidence"`
	ReviewStatus  string  `form:"review_status"`
	Status        string  `form:"status"`
}

// CollectionTaskQuery 采集任务查询参数
type CollectionTaskQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Status   string `form:"status"`
}

// ApplicationReviewPayload 人工复核请求
type ApplicationReviewPayload struct {
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	Version      string   `json:"version"`
	InstallPath  string   `json:"install_path"`
	ConfigPaths  []string `json:"config_paths"`
	ReviewStatus string   `json:"review_status"`
}
