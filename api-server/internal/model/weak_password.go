package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// =====================================================
// Task Related Models
// =====================================================

// WeakPasswordScanTask 弱密码扫描任务主表
type WeakPasswordScanTask struct {
	ID                   uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name                 string         `gorm:"type:text;not null" json:"name"`
	TriggerSource        string         `gorm:"type:varchar(32);default:'manual'" json:"trigger_source"`
	Status               string         `gorm:"type:varchar(32);default:'pending'" json:"status"`
	Progress             int            `gorm:"default:0" json:"progress"`
	CurrentStage         string         `gorm:"type:varchar(64)" json:"current_stage"`
	ScopeJSON            datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"scope_json"`
	DictionaryPolicyJSON datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"dictionary_policy_json"`
	AIPolicyJSON         datatypes.JSON `gorm:"column:ai_policy_json;type:jsonb;default:'{}'" json:"ai_policy_json"`
	TotalHosts           int            `gorm:"default:0" json:"total_hosts"`
	TotalApplications    int            `gorm:"default:0" json:"total_applications"`
	MatchedFindings      int            `gorm:"default:0" json:"matched_findings"`
	FailedApplications   int            `gorm:"default:0" json:"failed_applications"`
	CreatedBy            *uuid.UUID     `gorm:"type:uuid" json:"created_by"`
	StartedAt            *time.Time     `json:"started_at"`
	FinishedAt           *time.Time     `json:"finished_at"`
	CreatedAt            time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt            time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (WeakPasswordScanTask) TableName() string {
	return "weak_password_scan_tasks"
}

// WeakPasswordAssetAppAnalysis 应用资产分析批次
type WeakPasswordAssetAppAnalysis struct {
	ID                    uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskID                *uuid.UUID     `gorm:"type:uuid" json:"task_id"`
	ScopeJSON             datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"scope_json"`
	Status                string         `gorm:"type:varchar(32);default:'pending'" json:"status"`
	ApplicationAssetCount int            `gorm:"default:0" json:"application_asset_count"`
	CandidateCount        int            `gorm:"default:0" json:"candidate_count"`
	ErrorCode             string         `gorm:"type:varchar(64)" json:"error_code"`
	ErrorMessage          string         `gorm:"type:text" json:"error_message"`
	LLMModel              string         `gorm:"type:varchar(128)" json:"llm_model"`
	PromptSummary         string         `gorm:"type:text" json:"prompt_summary"`
	CreatedBy             *uuid.UUID     `gorm:"type:uuid" json:"created_by"`
	StartedAt             *time.Time     `json:"started_at"`
	FinishedAt            *time.Time     `json:"finished_at"`
	CreatedAt             time.Time      `gorm:"autoCreateTime" json:"created_at"`
}

func (WeakPasswordAssetAppAnalysis) TableName() string {
	return "weak_password_asset_app_analyses"
}

// WeakPasswordCandidateApplication AI 分析出的候选应用
type WeakPasswordCandidateApplication struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AnalysisID         uuid.UUID      `gorm:"type:uuid;not null" json:"analysis_id"`
	HostID             uuid.UUID      `gorm:"type:uuid;not null" json:"host_id"`
	AssetID            *uuid.UUID     `gorm:"type:uuid" json:"asset_id"`
	ApplicationName    string         `gorm:"type:varchar(255);not null" json:"application_name"`
	ApplicationType    string         `gorm:"type:varchar(64);not null" json:"application_type"`
	ApplicationVersion string         `gorm:"type:varchar(128)" json:"application_version"`
	ProfileID          string         `gorm:"type:varchar(128)" json:"profile_id"`
	Confidence         float64        `gorm:"default:0" json:"confidence"`
	CredentialTypes    datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"credential_types"`
	CandidatePathsJSON datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"candidate_paths_json"`
	ExtractorPlanJSON  datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"extractor_plan_json"`
	AssetEvidenceJSON  datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"asset_evidence_json"`
	AIReason           string         `gorm:"type:text" json:"ai_reason"`
	Status             string         `gorm:"type:varchar(32);default:'candidate'" json:"status"`
	IgnoredBy          *uuid.UUID     `gorm:"type:uuid" json:"ignored_by"`
	IgnoredAt          *time.Time     `json:"ignored_at"`
	CreatedAt          time.Time      `gorm:"autoCreateTime" json:"created_at"`
}

func (WeakPasswordCandidateApplication) TableName() string {
	return "weak_password_candidate_applications"
}

// WeakPasswordCollectionPlan 采集计划
type WeakPasswordCollectionPlan struct {
	ID                     uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskID                 uuid.UUID      `gorm:"type:uuid;not null" json:"task_id"`
	HostID                 uuid.UUID      `gorm:"type:uuid;not null" json:"host_id"`
	CandidateApplicationID *uuid.UUID     `gorm:"type:uuid" json:"candidate_application_id"`
	PlanJSON               datatypes.JSON `gorm:"type:jsonb;not null" json:"plan_json"`
	LLMAnalysisJSON        datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"llm_analysis_json"`
	Status                 string         `gorm:"type:varchar(32);default:'pending'" json:"status"`
	CreatedAt              time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt              time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (WeakPasswordCollectionPlan) TableName() string {
	return "weak_password_collection_plans"
}

// WeakPasswordScanHost 任务维度主机状态
type WeakPasswordScanHost struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskID             uuid.UUID  `gorm:"type:uuid;not null" json:"task_id"`
	HostID             uuid.UUID  `gorm:"type:uuid;not null" json:"host_id"`
	Status             string     `gorm:"type:varchar(32);default:'pending'" json:"status"`
	AgentStatus        string     `gorm:"type:varchar(32);default:'unknown'" json:"agent_status"`
	Progress           int        `gorm:"default:0" json:"progress"`
	CurrentStage       string     `gorm:"type:varchar(64)" json:"current_stage"`
	CollectedRecords   int        `gorm:"default:0" json:"collected_records"`
	MatchedFindings    int        `gorm:"default:0" json:"matched_findings"`
	FailedApplications int        `gorm:"default:0" json:"failed_applications"`
	ErrorCode          string     `gorm:"type:varchar(64)" json:"error_code"`
	ErrorMessage       string     `gorm:"type:text" json:"error_message"`
	StartedAt          *time.Time `json:"started_at"`
	FinishedAt         *time.Time `json:"finished_at"`
	CreatedAt          time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt          time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (WeakPasswordScanHost) TableName() string {
	return "weak_password_scan_hosts"
}

// WeakPasswordScanApplication 单应用检查状态
type WeakPasswordScanApplication struct {
	ID                     uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskID                 uuid.UUID      `gorm:"type:uuid;not null" json:"task_id"`
	ScanHostID             uuid.UUID      `gorm:"type:uuid;not null" json:"scan_host_id"`
	HostID                 uuid.UUID      `gorm:"type:uuid;not null" json:"host_id"`
	AssetID                *uuid.UUID     `gorm:"type:uuid" json:"asset_id"`
	CandidateApplicationID *uuid.UUID     `gorm:"type:uuid" json:"candidate_application_id"`
	ApplicationName        string         `gorm:"type:varchar(255);not null" json:"application_name"`
	ApplicationType        string         `gorm:"type:varchar(64);not null" json:"application_type"`
	ProfileID              string         `gorm:"type:varchar(128)" json:"profile_id"`
	Status                 string         `gorm:"type:varchar(32);default:'pending'" json:"status"`
	Progress               int            `gorm:"default:0" json:"progress"`
	CurrentStage           string         `gorm:"type:varchar(64)" json:"current_stage"`
	AgentToolCallCount     int            `gorm:"default:0" json:"agent_tool_call_count"`
	MaxAgentToolCalls      int            `gorm:"default:10" json:"max_agent_tool_calls"`
	CollectedRecords       int            `gorm:"default:0" json:"collected_records"`
	MatchedFindings        int            `gorm:"default:0" json:"matched_findings"`
	AttemptedPathsJSON     datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"attempted_paths_json"`
	ErrorCode              string         `gorm:"type:varchar(64)" json:"error_code"`
	ErrorMessage           string         `gorm:"type:text" json:"error_message"`
	StartedAt              *time.Time     `json:"started_at"`
	FinishedAt             *time.Time     `json:"finished_at"`
	CreatedAt              time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt              time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (WeakPasswordScanApplication) TableName() string {
	return "weak_password_scan_applications"
}

// WeakPasswordAgentToolCall Agent 工具调用记录
type WeakPasswordAgentToolCall struct {
	ID                   uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskID               uuid.UUID      `gorm:"type:uuid;not null" json:"task_id"`
	ScanApplicationID    *uuid.UUID     `gorm:"type:uuid" json:"scan_application_id"`
	HostID               uuid.UUID      `gorm:"type:uuid;not null" json:"host_id"`
	CallID               string         `gorm:"type:varchar(255);not null" json:"call_id"`
	ToolName             string         `gorm:"type:varchar(128);not null" json:"tool_name"`
	ArgumentsSummaryJSON datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"arguments_summary_json"`
	ResultSummaryJSON    datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"result_summary_json"`
	Status               string         `gorm:"type:varchar(32);default:'pending'" json:"status"`
	ErrorCode            string         `gorm:"type:varchar(64)" json:"error_code"`
	ErrorMessage         string         `gorm:"type:text" json:"error_message"`
	ExecutionTimeMs      int64          `gorm:"default:0" json:"execution_time_ms"`
	CreatedAt            time.Time      `gorm:"autoCreateTime" json:"created_at"`
}

func (WeakPasswordAgentToolCall) TableName() string {
	return "weak_password_agent_tool_calls"
}

// =====================================================
// Dictionary Related Models
// =====================================================

// WeakPasswordDictionary 字典元数据
type WeakPasswordDictionary struct {
	ID                   uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name                 string         `gorm:"type:varchar(255);not null" json:"name"`
	DictionaryType       string         `gorm:"type:varchar(32);not null" json:"dictionary_type"`
	Status               string         `gorm:"type:varchar(32);default:'enabled'" json:"status"`
	EntryCount           int            `gorm:"default:0" json:"entry_count"`
	Source               string         `gorm:"type:varchar(64);not null" json:"source"`
	Categories           datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"categories"`
	GenerationPolicyJSON datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"generation_policy_json"`
	PromptSummary        string         `gorm:"type:text" json:"prompt_summary"`
	LLMModel             string         `gorm:"type:varchar(128)" json:"llm_model"`
	CreatedBy            *uuid.UUID     `gorm:"type:uuid" json:"created_by"`
	CreatedAt            time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt            time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (WeakPasswordDictionary) TableName() string {
	return "weak_password_dictionaries"
}

// WeakPasswordDictionaryEntry 字典条目
type WeakPasswordDictionaryEntry struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	DictionaryID  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_wp_dict_entries_hash" json:"dictionary_id"`
	Candidate     string    `gorm:"type:text;not null" json:"candidate"`
	CandidateHash string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_wp_dict_entries_hash" json:"candidate_hash"`
	Category      string    `gorm:"type:varchar(64)" json:"category"`
	RuleSource    string    `gorm:"type:varchar(128)" json:"rule_source"`
	RiskLevel     string    `gorm:"type:varchar(32)" json:"risk_level"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (WeakPasswordDictionaryEntry) TableName() string {
	return "weak_password_dictionary_entries"
}

// =====================================================
// Match and Finding Related Models
// =====================================================

// WeakPasswordMatchBatch 匹配批次
type WeakPasswordMatchBatch struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskID            uuid.UUID      `gorm:"type:uuid;not null" json:"task_id"`
	ScanApplicationID *uuid.UUID     `gorm:"type:uuid" json:"scan_application_id"`
	BatchType         string         `gorm:"type:varchar(32);not null" json:"batch_type"`
	Status            string         `gorm:"type:varchar(32);default:'pending'" json:"status"`
	CredentialType    string         `gorm:"type:varchar(32)" json:"credential_type"`
	CandidateCount    int            `gorm:"default:0" json:"candidate_count"`
	RecordCount       int            `gorm:"default:0" json:"record_count"`
	LLMModel          string         `gorm:"type:varchar(128)" json:"llm_model"`
	PromptSummary     string         `gorm:"type:text" json:"prompt_summary"`
	ResultSummaryJSON datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"result_summary_json"`
	ErrorCode         string         `gorm:"type:varchar(64)" json:"error_code"`
	ErrorMessage      string         `gorm:"type:text" json:"error_message"`
	StartedAt         *time.Time     `json:"started_at"`
	FinishedAt        *time.Time     `json:"finished_at"`
	CreatedAt         time.Time      `gorm:"autoCreateTime" json:"created_at"`
}

func (WeakPasswordMatchBatch) TableName() string {
	return "weak_password_match_batches"
}

// WeakPasswordFinding 命中结果
type WeakPasswordFinding struct {
	ID                       uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskID                   uuid.UUID      `gorm:"type:uuid;not null" json:"task_id"`
	ScanApplicationID        *uuid.UUID     `gorm:"type:uuid" json:"scan_application_id"`
	HostID                   uuid.UUID      `gorm:"type:uuid;not null" json:"host_id"`
	AssetID                  *uuid.UUID     `gorm:"type:uuid" json:"asset_id"`
	ApplicationName          string         `gorm:"type:varchar(255);not null" json:"application_name"`
	ApplicationType          string         `gorm:"type:varchar(64);not null" json:"application_type"`
	Account                  string         `gorm:"type:varchar(255);not null" json:"account"`
	CredentialType           string         `gorm:"type:varchar(32);not null" json:"credential_type"`
	MatchStatus              string         `gorm:"type:varchar(32);not null" json:"match_status"`
	MatchedPasswordMask      string         `gorm:"type:varchar(128)" json:"matched_password_mask"`
	MatchedPasswordEncrypted []byte         `gorm:"type:bytea" json:"-"`
	MatchSource              string         `gorm:"type:varchar(64);not null" json:"match_source"`
	MatchRule                string         `gorm:"type:varchar(128);not null" json:"match_rule"`
	DictionaryID             *uuid.UUID     `gorm:"type:uuid" json:"dictionary_id"`
	Confidence               float64        `gorm:"default:0" json:"confidence"`
	SourcePath               string         `gorm:"type:text" json:"source_path"`
	FieldPath                string         `gorm:"type:varchar(255)" json:"field_path"`
	EvidenceJSON             datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"evidence_json"`
	AIReason                 string         `gorm:"type:text" json:"ai_reason"`
	FixedAt                  *time.Time     `json:"fixed_at"`
	FalsePositiveAt          *time.Time     `json:"false_positive_at"`
	RiskAcceptedAt           *time.Time     `json:"risk_accepted_at"`
	CreatedAt                time.Time      `gorm:"autoCreateTime" json:"created_at"`
}

func (WeakPasswordFinding) TableName() string {
	return "weak_password_findings"
}

// WeakPasswordCollectionError 采集错误
type WeakPasswordCollectionError struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskID             uuid.UUID      `gorm:"type:uuid;not null" json:"task_id"`
	ScanApplicationID  *uuid.UUID     `gorm:"type:uuid" json:"scan_application_id"`
	HostID             uuid.UUID      `gorm:"type:uuid;not null" json:"host_id"`
	ApplicationName    string         `gorm:"type:varchar(255)" json:"application_name"`
	SourcePath         string         `gorm:"type:text" json:"source_path"`
	ErrorCode          string         `gorm:"type:varchar(64);not null" json:"error_code"`
	ErrorMessage       string         `gorm:"type:text" json:"error_message"`
	AgentToolCallCount int            `gorm:"default:0" json:"agent_tool_call_count"`
	AttemptedPathsJSON datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"attempted_paths_json"`
	RepairTraceJSON    datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"repair_trace_json"`
	FinalStatus        string         `gorm:"type:varchar(32);not null" json:"final_status"`
	CreatedAt          time.Time      `gorm:"autoCreateTime" json:"created_at"`
}

func (WeakPasswordCollectionError) TableName() string {
	return "weak_password_collection_errors"
}

// WeakPasswordAIReport AI 分析报告
type WeakPasswordAIReport struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TaskID            uuid.UUID      `gorm:"type:uuid;not null" json:"task_id"`
	ScanApplicationID *uuid.UUID     `gorm:"type:uuid" json:"scan_application_id"`
	ReportType        string         `gorm:"type:varchar(64);not null" json:"report_type"`
	LLMModel          string         `gorm:"type:varchar(128)" json:"llm_model"`
	PromptSummary     string         `gorm:"type:text" json:"prompt_summary"`
	ReportJSON        datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"report_json"`
	CreatedAt         time.Time      `gorm:"autoCreateTime" json:"created_at"`
}

func (WeakPasswordAIReport) TableName() string {
	return "weak_password_ai_reports"
}

// WeakPasswordRevealAudit 明文查看审计
type WeakPasswordRevealAudit struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	FindingID   uuid.UUID  `gorm:"type:uuid;not null" json:"finding_id"`
	RequesterID uuid.UUID  `gorm:"type:uuid;not null" json:"requester_id"`
	ApproverID  *uuid.UUID `gorm:"type:uuid" json:"approver_id"`
	Status      string     `gorm:"type:varchar(32);default:'pending'" json:"status"`
	Reason      string     `gorm:"type:text" json:"reason"`
	Watermark   string     `gorm:"type:varchar(255)" json:"watermark"`
	RevealedAt  *time.Time `json:"revealed_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

func (WeakPasswordRevealAudit) TableName() string {
	return "weak_password_reveal_audits"
}

// =====================================================
// Request/Response Models
// =====================================================

// AnalyzeAssetApplicationsRequest 一键分析应用资产请求
type AnalyzeAssetApplicationsRequest struct {
	Scope struct {
		HostIDs          []string `json:"host_ids"`
		HostGroupIDs     []string `json:"host_group_ids"`
		ApplicationTypes []string `json:"application_types"`
		OnlineAgentsOnly bool     `json:"online_agents_only"`
	} `json:"scope" binding:"required"`
}

// CreateTaskByApplicationRequest 针对单个应用创建任务请求
type CreateTaskByApplicationRequest struct {
	CandidateApplicationID string                       `json:"candidate_application_id" binding:"required"`
	DictionaryPolicy       WeakPasswordDictionaryPolicy `json:"dictionary_policy"`
	AIPolicy               WeakPasswordAIPolicy         `json:"ai_policy"`
}

// CreateTasksByApplicationsRequest 针对多个应用创建弱密码检查任务
type CreateTasksByApplicationsRequest struct {
	CandidateApplicationIDs []string                     `json:"candidate_application_ids" binding:"required"`
	DictionaryPolicy        WeakPasswordDictionaryPolicy `json:"dictionary_policy"`
	AIPolicy                WeakPasswordAIPolicy         `json:"ai_policy"`
}

// WeakPasswordDictionaryPolicy 字典选择策略。
// Hybrid/Fuzzy 为历史兼容字段，当前前端不再展示，也不参与匹配扩展。
type WeakPasswordDictionaryPolicy struct {
	UseDefault1000 bool     `json:"use_default_1000"`
	DictionaryIDs  []string `json:"dictionary_ids"`
	UseAIGenerated bool     `json:"use_ai_generated"`
	Hybrid         bool     `json:"hybrid,omitempty"`
	Fuzzy          bool     `json:"fuzzy,omitempty"`
}

// WeakPasswordAIPolicy AI 辅助策略。
// EncryptedPasswordLLMMatch 为历史兼容字段，密码类型由服务端/LLM 自行判断。
type WeakPasswordAIPolicy struct {
	RepairCollectionErrors    bool `json:"repair_collection_errors"`
	EncryptedPasswordLLMMatch bool `json:"encrypted_password_llm_match,omitempty"`
	DetectionRounds           int  `json:"detection_rounds,omitempty"`
	MaxAgentToolCallsPerApp   int  `json:"max_agent_tool_calls_per_app"`
}

// AIGenerateDictionaryRequest AI 生成字典请求
type AIGenerateDictionaryRequest struct {
	NaturalLanguage        string   `json:"natural_language"`
	Target                 string   `json:"target"`
	ApplicationType        string   `json:"application_type"`
	OrganizationKeywords   []string `json:"organization_keywords"`
	AccountKeywords        []string `json:"account_keywords"`
	Count                  int      `json:"count"`
	Rules                  []string `json:"rules"`
	DeduplicateWithDefault bool     `json:"deduplicate_with_default"`
}

// RevealFindingRequest 查看完整命中密码请求
type RevealFindingRequest struct {
	Password string `json:"password" binding:"required"`
}

// TaskProgressResponse 任务进度响应
type TaskProgressResponse struct {
	TaskID             string `json:"task_id"`
	Status             string `json:"status"`
	Progress           int    `json:"progress"`
	CurrentStage       string `json:"current_stage"`
	CurrentHostID      string `json:"current_host_id"`
	CurrentApplication string `json:"current_application"`
	AgentToolCallCount int    `json:"agent_tool_call_count"`
	MaxAgentToolCalls  int    `json:"max_agent_tool_calls"`
	LastAgentTool      string `json:"last_agent_tool"`
	LastErrorCode      string `json:"last_error_code"`
	Message            string `json:"message"`
}

// TaskStatus constants
const (
	TaskStatusPending               = "pending"
	TaskStatusAnalyzingAssets       = "analyzing_assets"
	TaskStatusCollectingCredentials = "collecting_credentials"
	TaskStatusRepairingCollection   = "repairing_collection"
	TaskStatusMatching              = "matching"
	TaskStatusCompleted             = "completed"
	TaskStatusPartialFailed         = "partial_failed"
	TaskStatusFailed                = "failed"
	TaskStatusCancelled             = "cancelled"
)

// ApplicationStatus constants
const (
	AppStatusCandidate  = "candidate"
	AppStatusPlanned    = "planned"
	AppStatusCollecting = "collecting"
	AppStatusRepairing  = "repairing"
	AppStatusMatching   = "matching"
	AppStatusMatched    = "matched"
	AppStatusNoMatch    = "no_match"
	AppStatusFailed     = "failed"
	AppStatusIgnored    = "ignored"
)

// ErrorCode constants
const (
	ErrCodeNoApplicationAssets      = "no_application_assets"
	ErrCodeAgentNotConnected        = "agent_not_connected"
	ErrCodeAgentCallbackUnavailable = "agent_callback_unavailable"
	ErrCodePermissionDenied         = "permission_denied"
	ErrCodeFileNotFound             = "file_not_found"
	ErrCodeFieldNotFound            = "field_not_found"
	ErrCodeFileTooLarge             = "file_too_large"
	ErrCodeConfigDiscoveryFailed    = "config_discovery_failed"
	ErrCodeLLMMatchVerifyFailed     = "llm_match_verify_failed"
	ErrCodeUnsupportedFormat        = "unsupported_credential_format"
)

// MatchStatus constants
const (
	MatchStatusConfirmed              = "confirmed"
	MatchStatusAIInferredNeedsConfirm = "ai_inferred_needs_confirm"
	MatchStatusVerifyFailed           = "verify_failed"
	MatchStatusFalsePositive          = "false_positive"
	MatchStatusFixed                  = "fixed"
	MatchStatusRiskAccepted           = "risk_accepted"
)

// CredentialType constants
const (
	CredTypePlaintext     = "plaintext"
	CredTypeHash          = "hash"
	CredTypeSaltedHash    = "salted_hash"
	CredTypeEncryptedBlob = "encrypted_blob"
	CredTypeAuthString    = "auth_string"
	CredTypeUnknown       = "unknown"
)

// DictionaryType constants
const (
	DictTypeDefault1000 = "default_1000"
	DictTypeUploaded    = "uploaded"
	DictTypeAIGenerated = "ai_generated"
	DictTypeTaskTemp    = "task_temp"
)
