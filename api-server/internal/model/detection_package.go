package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type DetectionPackageDraft struct {
	ID                uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PackageID         string         `gorm:"type:varchar(160);uniqueIndex;not null" json:"package_id"`
	TargetVersion     string         `gorm:"type:varchar(32);not null" json:"target_version"`
	Title             string         `gorm:"type:varchar(255);not null" json:"title"`
	Description       string         `gorm:"type:text" json:"description"`
	CVEIDs            datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"cve_ids"`
	AIGenerated       bool           `gorm:"not null;default:false" json:"ai_generated"`
	AIGenerationInput datatypes.JSON `gorm:"type:jsonb" json:"ai_generation_input"`
	HookPlanYAML      string         `gorm:"type:text" json:"hook_plan_yaml"`
	EBPFSource        string         `gorm:"type:text" json:"ebpf_source"`
	SigmaRulesYAML    string         `gorm:"type:text" json:"sigma_rules_yaml"`
	CorrelationYAML   string         `gorm:"type:text" json:"correlation_yaml"`
	BuildParams       datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"build_params"`
	Status            string         `gorm:"type:varchar(32);not null;default:'draft'" json:"status"`
	LastBuildID       *uuid.UUID     `gorm:"type:uuid" json:"last_build_id"`
	CreatedBy         string         `gorm:"type:varchar(100)" json:"created_by"`
	UpdatedBy         string         `gorm:"type:varchar(100)" json:"updated_by"`
	CreatedAt         time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (DetectionPackageDraft) TableName() string { return "detection_package_drafts" }

type DetectionPackage struct {
	ID                 uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PackageID          string         `gorm:"type:varchar(160);not null;uniqueIndex:idx_pkg_version" json:"package_id"`
	Version            string         `gorm:"type:varchar(32);not null;uniqueIndex:idx_pkg_version" json:"version"`
	Title              string         `gorm:"type:varchar(255);not null" json:"title"`
	Description        string         `gorm:"type:text" json:"description"`
	CVEIDs             datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"cve_ids"`
	Status             string         `gorm:"type:varchar(32);not null;default:'built'" json:"status"`
	PackageObjectKey   string         `gorm:"type:text" json:"package_object_key"`
	SignatureObjectKey string         `gorm:"type:text" json:"signature_object_key"`
	PackageSize        int64          `gorm:"not null;default:0" json:"package_size"`
	PackageSHA256      string         `gorm:"type:varchar(64)" json:"package_sha256"`
	SignedBy           string         `gorm:"type:varchar(100)" json:"signed_by"`
	SignedAt           *time.Time     `json:"signed_at"`
	EnabledAt          *time.Time     `json:"enabled_at"`
	DisabledAt         *time.Time     `json:"disabled_at"`
	BuildID            *uuid.UUID     `gorm:"type:uuid" json:"build_id"`
	BuilderImage       string         `gorm:"type:varchar(255)" json:"builder_image"`
	BuilderDigest      string         `gorm:"type:varchar(128)" json:"builder_digest"`
	ManifestJSON       datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"manifest_json"`
	HookSummary        datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"hook_summary"`
	EventSchema        datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"event_schema"`
	LimitsJSON         datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"limits_json"`
	CreatedAt          time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt          time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (DetectionPackage) TableName() string { return "detection_packages" }

type DetectionPackageBuild struct {
	ID                       uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	DraftID                  *uuid.UUID     `gorm:"type:uuid" json:"draft_id"`
	PackageID                string         `gorm:"type:varchar(160);not null" json:"package_id"`
	Version                  string         `gorm:"type:varchar(32);not null" json:"version"`
	Status                   string         `gorm:"type:varchar(32);not null;default:'pending'" json:"status"`
	BuilderImage             string         `gorm:"type:varchar(255);not null" json:"builder_image"`
	BuilderDigest            string         `gorm:"type:varchar(128)" json:"builder_digest"`
	ClangVersion             string         `gorm:"type:varchar(100)" json:"clang_version"`
	StartedAt                *time.Time     `json:"started_at"`
	FinishedAt               *time.Time     `json:"finished_at"`
	DurationMs               int64          `json:"duration_ms"`
	ArtifactSummary          datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"artifact_summary"`
	HookSummary              datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"hook_summary"`
	EventSchema              datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"event_schema"`
	UnsignedPackageObjectKey string         `gorm:"type:text" json:"unsigned_package_object_key"`
	UnsignedPackageSHA256    string         `gorm:"type:varchar(64)" json:"unsigned_package_sha256"`
	UnsignedPackageSize      int64          `gorm:"not null;default:0" json:"unsigned_package_size"`
	BuildLogObjectKey        string         `gorm:"type:text" json:"build_log_object_key"`
	BuildLog                 string         `gorm:"type:text" json:"build_log"`
	ErrorMessage             string         `gorm:"type:text" json:"error_message"`
	CreatedBy                string         `gorm:"type:varchar(100)" json:"created_by"`
	CreatedAt                time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (DetectionPackageBuild) TableName() string { return "detection_package_builds" }

type DetectionPackageHostStatus struct {
	ID                uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PackageID         string         `gorm:"type:varchar(160);not null;uniqueIndex:idx_pkg_host" json:"package_id"`
	Version           string         `gorm:"type:varchar(32);not null;uniqueIndex:idx_pkg_host" json:"version"`
	HostID            uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_pkg_host" json:"host_id"`
	Hostname          string         `gorm:"type:varchar(255)" json:"hostname"`
	Status            string         `gorm:"type:varchar(64);not null;default:'pending'" json:"status"`
	PluginStatus      string         `gorm:"type:varchar(64)" json:"plugin_status"`
	SigmaStatus       string         `gorm:"type:varchar(64)" json:"sigma_status"`
	CorrelationStatus string         `gorm:"type:varchar(64)" json:"correlation_status"`
	ActiveArtifact    string         `gorm:"type:varchar(16)" json:"active_artifact"`
	LoadedHooks       datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"loaded_hooks"`
	KernelRelease     string         `gorm:"type:varchar(128)" json:"kernel_release"`
	Arch              string         `gorm:"type:varchar(32)" json:"arch"`
	ErrorMessage      string         `gorm:"type:text" json:"error_message"`
	MetricsJSON       datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"metrics_json"`
	InstalledAt       *time.Time     `json:"installed_at"`
	UpdatedAt         time.Time      `gorm:"not null;default:now()" json:"updated_at"`
	LastReportedAt    *time.Time     `json:"last_reported_at"`
}

func (DetectionPackageHostStatus) TableName() string { return "detection_package_host_status" }

type DetectionPackageOperation struct {
	ID           uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PackageID    string         `gorm:"type:varchar(160)" json:"package_id"`
	Version      string         `gorm:"type:varchar(32)" json:"version"`
	Operation    string         `gorm:"type:varchar(64);not null" json:"operation"`
	Operator     string         `gorm:"type:varchar(100)" json:"operator"`
	RequestJSON  datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"request_json"`
	ResultJSON   datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"result_json"`
	Success      bool           `gorm:"not null;default:true" json:"success"`
	ErrorMessage string         `gorm:"type:text" json:"error_message"`
	CreatedAt    time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (DetectionPackageOperation) TableName() string { return "detection_package_operations" }

type EBPFHookAllowlistConfig struct {
	ID          uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Version     int64          `gorm:"type:bigint;uniqueIndex;autoIncrement" json:"version"`
	ConfigJSON  datatypes.JSON `gorm:"type:jsonb;not null" json:"config_json"`
	Description string         `gorm:"type:text" json:"description"`
	UpdatedBy   string         `gorm:"type:varchar(100)" json:"updated_by"`
	CreatedAt   time.Time      `gorm:"not null;default:now()" json:"created_at"`
	ActivatedAt *time.Time     `json:"activated_at"`
}

func (EBPFHookAllowlistConfig) TableName() string { return "ebpf_hook_allowlist_configs" }

type CorrelationRule struct {
	ID             uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PackageID      string         `gorm:"type:varchar(160);not null;uniqueIndex:idx_corr_rule" json:"package_id"`
	PackageVersion string         `gorm:"type:varchar(32);not null;uniqueIndex:idx_corr_rule" json:"package_version"`
	RuleID         string         `gorm:"type:varchar(220);not null;uniqueIndex:idx_corr_rule" json:"rule_id"`
	Title          string         `gorm:"type:varchar(255)" json:"title"`
	Severity       string         `gorm:"type:varchar(32)" json:"severity"`
	ByKey          string         `gorm:"type:varchar(32)" json:"by_key"`
	WindowSeconds  int            `json:"window_seconds"`
	Ordered        bool           `gorm:"not null;default:true" json:"ordered"`
	SequenceJSON   datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"sequence_json"`
	Content        string         `gorm:"type:text;not null" json:"content"`
	CreatedAt      time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (CorrelationRule) TableName() string { return "correlation_rules" }
