package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const (
	AgentGuardCoverageFullEnforcement              = "full_enforcement"
	AgentGuardCoverageBehaviorMonitorEscapeEnforce = "behavior_monitor_escape_enforce"
	AgentGuardCoverageMonitorOnly                  = "monitor_only"
	AgentGuardCoverageNoIsolation                  = "no_isolation"
	AgentGuardCoverageRemoteUnobservable           = "remote_unobservable"
	AgentGuardCoverageUnsupportedProfile           = "unsupported_profile"
	AgentGuardCoverageDegraded                     = "degraded"

	AgentGuardRuleKeySensitiveDirectory  = "AGB-BUILTIN-001"
	AgentGuardRuleKeyExternalNetwork     = "AGB-BUILTIN-002"
	AgentGuardRuleKeyFileCreation        = "AGB-BUILTIN-003"
	AgentGuardRuleKeySensitiveCommand    = "AGB-BUILTIN-004"
	AgentGuardRuleKeyPrivilegeEscalation = "AGB-BUILTIN-005"

	AgentGuardProfileKeyCodexLinux      = "codex-linux"
	AgentGuardProfileKeyOpenClawLinux   = "openclaw-linux"
	AgentGuardProfileKeyHermesLinux     = "hermes-linux"
	AgentGuardProfileKeyClaudeCodeLinux = "claude-code-linux"
	AgentGuardProfileKeyOpenCodeLinux   = "opencode-linux"
	AgentGuardProfileKeyGeminiCLILinux  = "gemini-cli-linux"

	AgentGuardSessionSourceAgentOfficial = "agent_official"
	AgentGuardSessionSourceAdapterHook   = "adapter_hook"
	AgentGuardSessionSourceAegisWrapper  = "aegis_wrapper"

	AgentGuardActionFreezeExecutionUnit   = "freeze_execution_unit"
	AgentGuardActionHoldExecutionUnit     = "hold_execution_unit"
	AgentGuardActionResumeExecutionUnit   = "resume_execution_unit"
	AgentGuardActionKillExecutionUnit     = "kill_execution_unit"
	AgentGuardActionKillAgentInstance     = "kill_agent_instance"
	AgentGuardActionSourceManual          = "manual"
	AgentGuardActionStatusPending         = "pending"
	AgentGuardActionStatusDispatching     = "dispatching"
	AgentGuardActionStatusRunning         = "running"
	AgentGuardActionStatusSuccess         = "success"
	AgentGuardActionStatusFailed          = "failed"
	AgentGuardActionStatusExpired         = "expired"
	AgentGuardActionStatusCancelled       = "cancelled"
	AgentGuardAnalysisStatusPending       = "pending"
	AgentGuardAnalysisStatusRunning       = "running"
	AgentGuardAnalysisStatusSucceeded     = "succeeded"
	AgentGuardAnalysisStatusFailed        = "failed"
	AgentGuardAnalysisStatusInvalidOutput = "invalid_output"
	AgentGuardAnalysisStatusInconclusive  = "inconclusive"
	AgentGuardAnalysisStatusCancelled     = "cancelled"
)

// AgentGuardAdapterProfile is a versioned, immutable product recognition and
// isolation expectation manifest.
type AgentGuardAdapterProfile struct {
	ID                   uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ProfileKey           string         `gorm:"type:varchar(128);not null;uniqueIndex:uq_agent_guard_profile_version" json:"profile_key"`
	ProfileVersion       int64          `gorm:"not null;uniqueIndex:uq_agent_guard_profile_version" json:"profile_version"`
	AgentType            string         `gorm:"type:varchar(64);not null;index" json:"agent_type"`
	DisplayName          string         `gorm:"type:varchar(255);not null" json:"display_name"`
	Source               string         `gorm:"type:varchar(32);not null;default:builtin" json:"source"`
	SandboxFamily        string         `gorm:"type:varchar(32);not null" json:"sandbox_family"`
	ControllerMatch      datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"controller_match"`
	WorkerMatch          datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"worker_match"`
	BackendDetectors     datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"backend_detectors"`
	IsolationExpectation datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"isolation_expectation"`
	DefaultEscapeRules   datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"default_escape_rules"`
	Digest               string         `gorm:"type:varchar(80);not null" json:"digest"`
	Enabled              bool           `gorm:"not null;default:true" json:"enabled"`
	CreatedBy            string         `gorm:"type:varchar(100)" json:"created_by,omitempty"`
	CreatedAt            time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt            time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (AgentGuardAdapterProfile) TableName() string {
	return "agent_guard_adapter_profiles"
}

// AgentBehaviorRuleDefinition stores immutable, versioned behavior rule
// definitions. Administrators override these definitions only through policy.
type AgentBehaviorRuleDefinition struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RuleKey           string         `gorm:"type:varchar(128);not null;uniqueIndex:uq_agent_behavior_rule_version" json:"rule_key"`
	RuleVersion       int64          `gorm:"not null;uniqueIndex:uq_agent_behavior_rule_version" json:"rule_version"`
	Name              string         `gorm:"type:varchar(255);not null" json:"name"`
	Description       string         `gorm:"type:text" json:"description,omitempty"`
	Source            string         `gorm:"type:varchar(24);not null;default:builtin" json:"source"`
	Engine            string         `gorm:"type:varchar(32);not null" json:"engine"`
	Categories        datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"categories"`
	DefaultEnabled    bool           `gorm:"not null;default:true" json:"default_enabled"`
	DefaultSeverity   string         `gorm:"type:varchar(20);not null" json:"default_severity"`
	DefaultAction     string         `gorm:"type:varchar(40);not null" json:"default_action"`
	RecommendedAction string         `gorm:"type:varchar(40);not null" json:"recommended_action"`
	ParametersSchema  datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"parameters_schema"`
	DefaultParameters datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"default_parameters"`
	RequiredEvidence  datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"required_evidence"`
	AllowConditions   datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"allow_conditions"`
	MITRE             datatypes.JSON `gorm:"column:mitre;type:jsonb;not null;default:'[]'" json:"mitre"`
	Immutable         bool           `gorm:"not null;default:true" json:"immutable"`
	Digest            string         `gorm:"type:varchar(80);not null" json:"digest"`
	CreatedAt         time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (AgentBehaviorRuleDefinition) TableName() string {
	return "agent_behavior_rule_definitions"
}

type AgentGuardPolicy struct {
	ID                   uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PolicyKey            string         `gorm:"type:varchar(128);not null;uniqueIndex:uq_agent_guard_policy_version" json:"policy_key"`
	Version              int64          `gorm:"not null;uniqueIndex:uq_agent_guard_policy_version" json:"version"`
	Name                 string         `gorm:"type:varchar(255);not null" json:"name"`
	Description          string         `gorm:"type:text" json:"description,omitempty"`
	Status               string         `gorm:"type:varchar(32);not null;default:draft;index" json:"status"`
	Priority             int            `gorm:"not null;default:100" json:"priority"`
	Targets              datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"targets"`
	CollectionPolicy     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"collection_policy"`
	BuiltinRuleOverrides datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"builtin_rule_overrides"`
	AtomicRules          datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"atomic_rules"`
	CorrelationRules     datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"correlation_rules"`
	AnalysisPolicy       datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"analysis_policy"`
	EscapeRules          datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"escape_rules"`
	FreezeTimeoutSeconds int            `gorm:"not null;default:300" json:"freeze_timeout_seconds"`
	CompiledPreview      datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"compiled_preview"`
	Digest               string         `gorm:"type:varchar(80)" json:"digest,omitempty"`
	CreatedBy            string         `gorm:"type:varchar(100);not null" json:"created_by"`
	PublishedBy          string         `gorm:"type:varchar(100)" json:"published_by,omitempty"`
	PublishedAt          *time.Time     `json:"published_at,omitempty"`
	DisabledBy           string         `gorm:"type:varchar(100)" json:"disabled_by,omitempty"`
	DisabledAt           *time.Time     `json:"disabled_at,omitempty"`
	CreatedAt            time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt            time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (AgentGuardPolicy) TableName() string {
	return "agent_guard_policies"
}

type AgentGuardPolicyDelivery struct {
	ID                  uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	HostID              uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_agent_guard_delivery;index" json:"host_id"`
	BundleVersion       int64          `gorm:"not null;uniqueIndex:uq_agent_guard_delivery" json:"bundle_version"`
	BundleDigest        string         `gorm:"type:varchar(80);not null" json:"bundle_digest"`
	PolicyVersions      datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"policy_versions"`
	ProfileVersions     datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"profile_versions"`
	BuiltinRuleVersions datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"builtin_rule_versions"`
	Status              string         `gorm:"type:varchar(32);not null;default:pending;index" json:"status"`
	CapabilitySnapshot  datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"capability_snapshot"`
	CoverageLevel       string         `gorm:"type:varchar(40)" json:"coverage_level,omitempty"`
	ErrorCode           string         `gorm:"type:varchar(100)" json:"error_code,omitempty"`
	ErrorMessage        string         `gorm:"type:text" json:"error_message,omitempty"`
	GeneratedAt         time.Time      `gorm:"not null;default:now()" json:"generated_at"`
	DispatchedAt        *time.Time     `json:"dispatched_at,omitempty"`
	ReceivedAt          *time.Time     `json:"received_at,omitempty"`
	AppliedAt           *time.Time     `json:"applied_at,omitempty"`
	LastReportedAt      *time.Time     `json:"last_reported_at,omitempty"`
	CreatedAt           time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt           time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (AgentGuardPolicyDelivery) TableName() string {
	return "agent_guard_policy_deliveries"
}

type AgentRuntimeInstance struct {
	ID                   uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	HostID               uuid.UUID      `gorm:"type:uuid;not null;index" json:"host_id"`
	AssetID              *uuid.UUID     `gorm:"type:uuid;index" json:"asset_id,omitempty"`
	AdapterProfileID     *uuid.UUID     `gorm:"type:uuid" json:"adapter_profile_id,omitempty"`
	ProfileKey           string         `gorm:"type:varchar(128);not null" json:"profile_key"`
	ProfileVersion       int64          `gorm:"not null" json:"profile_version"`
	AgentType            string         `gorm:"type:varchar(64);not null;index" json:"agent_type"`
	DisplayName          string         `gorm:"type:varchar(255)" json:"display_name,omitempty"`
	ControllerPID        int            `gorm:"column:controller_pid;not null;uniqueIndex:uq_agent_runtime_process" json:"controller_pid"`
	ControllerStartTicks string         `gorm:"type:numeric(20,0);not null;uniqueIndex:uq_agent_runtime_process" json:"controller_start_ticks"`
	ControllerExe        string         `gorm:"type:text" json:"controller_exe,omitempty"`
	ControllerCmdline    string         `gorm:"type:text" json:"controller_cmdline,omitempty"`
	RunUID               *int           `gorm:"column:run_uid" json:"run_uid,omitempty"`
	RunUser              string         `gorm:"type:varchar(255)" json:"run_user,omitempty"`
	DetectionConfidence  string         `gorm:"type:varchar(32);not null;default:candidate" json:"detection_confidence"`
	Status               string         `gorm:"type:varchar(32);not null;default:running;index" json:"status"`
	CoverageLevel        string         `gorm:"type:varchar(40);not null;default:monitor_only;index" json:"coverage_level"`
	CoverageReasons      datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"coverage_reasons"`
	Metadata             datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	FirstSeenAt          time.Time      `gorm:"not null" json:"first_seen_at"`
	LastSeenAt           time.Time      `gorm:"not null;index" json:"last_seen_at"`
	StoppedAt            *time.Time     `json:"stopped_at,omitempty"`
	CreatedAt            time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt            time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (AgentRuntimeInstance) TableName() string {
	return "agent_runtime_instances"
}

type AgentExecutionUnit struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	HostID            uuid.UUID      `gorm:"type:uuid;not null;index" json:"host_id"`
	InstanceID        uuid.UUID      `gorm:"type:uuid;not null;index" json:"instance_id"`
	UnitType          string         `gorm:"type:varchar(40);not null" json:"unit_type"`
	Fingerprint       string         `gorm:"type:varchar(160);not null;uniqueIndex:uq_agent_execution_unit_fingerprint" json:"fingerprint"`
	RootPID           *int           `gorm:"column:root_pid" json:"root_pid,omitempty"`
	RootStartTicks    string         `gorm:"type:numeric(20,0)" json:"root_start_ticks,omitempty"`
	CgroupID          string         `gorm:"type:varchar(32);index" json:"cgroup_id,omitempty"`
	CgroupPath        string         `gorm:"type:text" json:"cgroup_path,omitempty"`
	ContainerID       string         `gorm:"type:varchar(128);index" json:"container_id,omitempty"`
	ContainerRuntime  string         `gorm:"type:varchar(64)" json:"container_runtime,omitempty"`
	RemoteBackend     string         `gorm:"type:varchar(64)" json:"remote_backend,omitempty"`
	RemoteExecutionID string         `gorm:"type:varchar(255)" json:"remote_execution_id,omitempty"`
	RemoteHostRef     string         `gorm:"type:varchar(255)" json:"remote_host_ref,omitempty"`
	IsolationBaseline datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"isolation_baseline"`
	IsolationActual   datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"isolation_actual"`
	IsolationDiff     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"isolation_diff"`
	CoverageLevel     string         `gorm:"type:varchar(40);not null;index" json:"coverage_level"`
	CoverageReasons   datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"coverage_reasons"`
	Status            string         `gorm:"type:varchar(32);not null;default:observed;index" json:"status"`
	FirstSeenAt       time.Time      `gorm:"not null" json:"first_seen_at"`
	LastSeenAt        time.Time      `gorm:"not null;index" json:"last_seen_at"`
	FrozenAt          *time.Time     `json:"frozen_at,omitempty"`
	StoppedAt         *time.Time     `json:"stopped_at,omitempty"`
	CreatedAt         time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (AgentExecutionUnit) TableName() string {
	return "agent_execution_units"
}

type AgentBehaviorSession struct {
	ID                   uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	HostID               uuid.UUID      `gorm:"type:uuid;not null;index" json:"host_id"`
	InstanceID           uuid.UUID      `gorm:"type:uuid;not null;index" json:"instance_id"`
	ExecutionUnitID      *uuid.UUID     `gorm:"type:uuid;index" json:"execution_unit_id,omitempty"`
	ExternalSessionID    string         `gorm:"type:varchar(255)" json:"external_session_id,omitempty"`
	Source               string         `gorm:"type:varchar(32);not null" json:"source"`
	Confidence           string         `gorm:"type:varchar(20);not null" json:"confidence"`
	CorrelationTokenHash string         `gorm:"type:varchar(80)" json:"correlation_token_hash,omitempty"`
	Status               string         `gorm:"type:varchar(24);not null;default:active" json:"status"`
	BehaviorCount        int64          `gorm:"not null;default:0" json:"behavior_count"`
	FindingCount         int64          `gorm:"not null;default:0" json:"finding_count"`
	Completeness         datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"completeness"`
	StartedAt            time.Time      `gorm:"not null;index" json:"started_at"`
	LastSeenAt           time.Time      `gorm:"not null" json:"last_seen_at"`
	EndedAt              *time.Time     `json:"ended_at,omitempty"`
	CreatedAt            time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt            time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (AgentBehaviorSession) TableName() string {
	return "agent_behavior_sessions"
}

type AgentBehaviorEvent struct {
	ID                     uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RawEventID             string         `gorm:"type:varchar(100);not null;uniqueIndex" json:"raw_event_id"`
	HostID                 uuid.UUID      `gorm:"type:uuid;not null;index" json:"host_id"`
	HostBootID             string         `gorm:"type:varchar(100);not null" json:"host_boot_id"`
	AgentSequence          int64          `gorm:"not null" json:"agent_sequence"`
	InstanceID             *uuid.UUID     `gorm:"type:uuid;index" json:"instance_id,omitempty"`
	SessionID              *uuid.UUID     `gorm:"type:uuid;index" json:"session_id,omitempty"`
	ExecutionUnitID        *uuid.UUID     `gorm:"type:uuid;index" json:"execution_unit_id,omitempty"`
	PolicyID               *uuid.UUID     `gorm:"type:uuid;index" json:"policy_id,omitempty"`
	PolicyVersion          *int64         `json:"policy_version,omitempty"`
	RuleID                 string         `gorm:"type:varchar(100)" json:"rule_id,omitempty"`
	SchemaVersion          string         `gorm:"type:varchar(64);not null;default:aegis.agent.behavior.v1" json:"schema_version"`
	CorrelationID          string         `gorm:"type:varchar(100)" json:"correlation_id,omitempty"`
	ParentEventID          string         `gorm:"type:varchar(100)" json:"parent_event_id,omitempty"`
	AgentType              string         `gorm:"type:varchar(64)" json:"agent_type,omitempty"`
	ProfileKey             string         `gorm:"type:varchar(128)" json:"profile_key,omitempty"`
	ProfileVersion         *int64         `json:"profile_version,omitempty"`
	Category               string         `gorm:"type:varchar(32);not null;index" json:"category"`
	Operation              string         `gorm:"type:varchar(64);not null;index" json:"operation"`
	Outcome                string         `gorm:"type:varchar(24);not null" json:"outcome"`
	Errno                  *int           `json:"errno,omitempty"`
	Decision               string         `gorm:"type:varchar(40);not null;default:audit;index" json:"decision"`
	Severity               string         `gorm:"type:varchar(20);not null;default:info;index" json:"severity"`
	PID                    *int           `gorm:"column:pid" json:"pid,omitempty"`
	PPID                   *int           `gorm:"column:ppid" json:"ppid,omitempty"`
	ProcessStartTicks      string         `gorm:"type:numeric(20,0)" json:"process_start_ticks,omitempty"`
	ProcessName            string         `gorm:"type:varchar(255)" json:"process_name,omitempty"`
	ProcessExe             string         `gorm:"type:text" json:"process_exe,omitempty"`
	CommandArgv            datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"command_argv"`
	CommandCwd             string         `gorm:"type:text" json:"command_cwd,omitempty"`
	CommandVisibility      string         `gorm:"type:varchar(24);not null;default:complete" json:"command_visibility"`
	ProcessChain           datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"process_chain"`
	ResourceType           string         `gorm:"type:varchar(32)" json:"resource_type,omitempty"`
	ResourceIdentity       string         `gorm:"type:text" json:"resource_identity,omitempty"`
	ResourceIdentityHash   string         `gorm:"type:varchar(80);index" json:"resource_identity_hash,omitempty"`
	ResourceClassification string         `gorm:"type:varchar(64)" json:"resource_classification,omitempty"`
	Resource               datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"resource"`
	Isolation              datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"isolation"`
	Collection             datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"collection"`
	Evidence               datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"evidence"`
	OccurredAt             time.Time      `gorm:"not null;index" json:"occurred_at"`
	OccurredMonotonicNS    string         `gorm:"type:numeric(20,0)" json:"occurred_monotonic_ns,omitempty"`
	ReceivedAt             time.Time      `gorm:"not null;default:now()" json:"received_at"`
	CreatedAt              time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (AgentBehaviorEvent) TableName() string {
	return "agent_behavior_events"
}

type AgentSecurityFinding struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	FindingKey        string         `gorm:"type:varchar(255);not null;uniqueIndex" json:"finding_key"`
	HostID            uuid.UUID      `gorm:"type:uuid;not null;index" json:"host_id"`
	InstanceID        *uuid.UUID     `gorm:"type:uuid;index" json:"instance_id,omitempty"`
	SessionID         *uuid.UUID     `gorm:"type:uuid;index" json:"session_id,omitempty"`
	ExecutionUnitID   *uuid.UUID     `gorm:"type:uuid;index" json:"execution_unit_id,omitempty"`
	PolicyID          *uuid.UUID     `gorm:"type:uuid;index" json:"policy_id,omitempty"`
	PolicyVersion     *int64         `json:"policy_version,omitempty"`
	Title             string         `gorm:"type:varchar(500);not null" json:"title"`
	Severity          string         `gorm:"type:varchar(20);not null;index" json:"severity"`
	Verdict           string         `gorm:"type:varchar(24);not null;default:suspicious;index" json:"verdict"`
	Confidence        float64        `gorm:"type:numeric(5,4);not null;default:0" json:"confidence"`
	Status            string         `gorm:"type:varchar(24);not null;default:open;index" json:"status"`
	DecisionSources   datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"decision_sources"`
	RuleHits          datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"rule_hits"`
	EvidenceEventIDs  datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"evidence_event_ids"`
	EvidenceGraph     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"evidence_graph"`
	AttackStages      datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"attack_stages"`
	Summary           string         `gorm:"type:text" json:"summary,omitempty"`
	RecommendedAction string         `gorm:"type:varchar(64)" json:"recommended_action,omitempty"`
	LatestAnalysisID  *uuid.UUID     `gorm:"type:uuid" json:"latest_analysis_id,omitempty"`
	HandledBy         string         `gorm:"type:varchar(100)" json:"handled_by,omitempty"`
	HandledNote       string         `gorm:"type:text" json:"handled_note,omitempty"`
	HandledAt         *time.Time     `json:"handled_at,omitempty"`
	FirstObservedAt   time.Time      `gorm:"not null" json:"first_observed_at"`
	LastObservedAt    time.Time      `gorm:"not null;index" json:"last_observed_at"`
	CreatedAt         time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (AgentSecurityFinding) TableName() string {
	return "agent_security_findings"
}

type AgentSecurityAnalysisRun struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	FindingID         uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_agent_security_analysis_attempt;index" json:"finding_id"`
	Attempt           int            `gorm:"not null;uniqueIndex:uq_agent_security_analysis_attempt" json:"attempt"`
	Status            string         `gorm:"type:varchar(24);not null;default:pending;index" json:"status"`
	Provider          string         `gorm:"type:varchar(64)" json:"provider,omitempty"`
	Model             string         `gorm:"type:varchar(128)" json:"model,omitempty"`
	PromptVersion     string         `gorm:"type:varchar(64);not null" json:"prompt_version"`
	InputDigest       string         `gorm:"type:varchar(80);not null;index" json:"input_digest"`
	EvidenceEventIDs  datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"evidence_event_ids"`
	EvidenceSummary   datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"evidence_summary"`
	Output            datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"output"`
	Verdict           string         `gorm:"type:varchar(24)" json:"verdict,omitempty"`
	AttackProbability *float64       `gorm:"type:numeric(5,4)" json:"attack_probability,omitempty"`
	Confidence        *float64       `gorm:"type:numeric(5,4)" json:"confidence,omitempty"`
	ErrorCode         string         `gorm:"type:varchar(100)" json:"error_code,omitempty"`
	ErrorMessage      string         `gorm:"type:text" json:"error_message,omitempty"`
	RequestedBy       string         `gorm:"type:varchar(100)" json:"requested_by,omitempty"`
	QueuedAt          time.Time      `gorm:"not null" json:"queued_at"`
	StartedAt         *time.Time     `json:"started_at,omitempty"`
	CompletedAt       *time.Time     `json:"completed_at,omitempty"`
	CreatedAt         time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (AgentSecurityAnalysisRun) TableName() string {
	return "agent_security_analysis_runs"
}

type AgentGuardAction struct {
	ID                     uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	CommandID              string         `gorm:"type:varchar(100);uniqueIndex" json:"command_id,omitempty"`
	TriggerBehaviorEventID *uuid.UUID     `gorm:"type:uuid;index" json:"trigger_behavior_event_id,omitempty"`
	TriggerFindingID       *uuid.UUID     `gorm:"type:uuid;index" json:"trigger_finding_id,omitempty"`
	HostID                 uuid.UUID      `gorm:"type:uuid;not null;index" json:"host_id"`
	InstanceID             *uuid.UUID     `gorm:"type:uuid;index" json:"instance_id,omitempty"`
	ExecutionUnitID        *uuid.UUID     `gorm:"type:uuid;index" json:"execution_unit_id,omitempty"`
	Action                 string         `gorm:"type:varchar(40);not null" json:"action"`
	Source                 string         `gorm:"type:varchar(32);not null" json:"source"`
	Status                 string         `gorm:"type:varchar(32);not null;default:pending;index" json:"status"`
	Reason                 string         `gorm:"type:text;not null" json:"reason"`
	RequestedBy            string         `gorm:"type:varchar(100)" json:"requested_by,omitempty"`
	HoldRequested          bool           `gorm:"not null;default:false" json:"hold_requested"`
	FreezeTimeoutSeconds   *int           `json:"freeze_timeout_seconds,omitempty"`
	Result                 datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"result"`
	ErrorCode              string         `gorm:"type:varchar(100)" json:"error_code,omitempty"`
	ErrorMessage           string         `gorm:"type:text" json:"error_message,omitempty"`
	RequestedAt            time.Time      `gorm:"not null;index" json:"requested_at"`
	DispatchedAt           *time.Time     `json:"dispatched_at,omitempty"`
	CompletedAt            *time.Time     `json:"completed_at,omitempty"`
	ExpiresAt              *time.Time     `json:"expires_at,omitempty"`
	CreatedAt              time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt              time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (AgentGuardAction) TableName() string {
	return "agent_guard_actions"
}

// AgentGuardPolicyDraftRequest is the HTTP/service input contract. Repository
// code stores the normalized JSON form in AgentGuardPolicy.
type AgentGuardPolicyDraftRequest struct {
	PolicyKey            string                          `json:"policy_key"`
	Name                 string                          `json:"name"`
	Description          string                          `json:"description,omitempty"`
	Priority             int                             `json:"priority"`
	Targets              AgentGuardPolicyTargets         `json:"targets"`
	Collection           AgentGuardCollectionPolicy      `json:"collection"`
	BuiltinRuleOverrides []AgentGuardBuiltinRuleOverride `json:"builtin_rule_overrides"`
	AtomicRules          []AgentGuardAtomicRule          `json:"atomic_rules"`
	CorrelationRules     []AgentGuardCorrelationRule     `json:"correlation_rules"`
	Analysis             AgentGuardAnalysisPolicy        `json:"analysis"`
	EscapeRules          []AgentGuardEscapeRule          `json:"escape_rules"`
	FreezeTimeoutSeconds int                             `json:"freeze_timeout_seconds"`
}

type AgentGuardPolicyTargets struct {
	HostIDs      []string `json:"host_ids"`
	HostGroupIDs []string `json:"host_group_ids"`
	AgentTypes   []string `json:"agent_types"`
	ProfileKeys  []string `json:"profile_keys,omitempty"`
}

type AgentGuardCollectionPolicy struct {
	Categories         []string       `json:"categories"`
	CommandArgv        string         `json:"command_argv"`
	FileContent        string         `json:"file_content"`
	NetworkContent     string         `json:"network_content"`
	Aggregation        map[string]int `json:"aggregation"`
	ToolAdapterEnabled bool           `json:"tool_adapter_enabled"`
}

type AgentGuardBuiltinRuleOverride struct {
	RuleKey          string           `json:"rule_key"`
	RuleVersion      int64            `json:"rule_version"`
	Enabled          bool             `json:"enabled"`
	SeverityOverride string           `json:"severity_override,omitempty"`
	ActionOverride   string           `json:"action_override,omitempty"`
	Parameters       map[string]any   `json:"parameters"`
	Exceptions       []map[string]any `json:"exceptions"`
}

type AgentGuardAtomicRule struct {
	RuleID     string           `json:"rule_id"`
	Rule       string           `json:"rule"`
	Resource   map[string]any   `json:"resource"`
	Operations []string         `json:"operations"`
	Action     string           `json:"action"`
	Severity   string           `json:"severity"`
	Parameters map[string]any   `json:"parameters,omitempty"`
	Exceptions []map[string]any `json:"exceptions,omitempty"`
}

type AgentGuardCorrelationRule struct {
	RuleID           string   `json:"rule_id"`
	WindowSeconds    int      `json:"window_seconds"`
	Action           string   `json:"action"`
	Severity         string   `json:"severity"`
	GroupKeys        []string `json:"group_keys,omitempty"`
	RequiredEvidence []string `json:"required_evidence,omitempty"`
}

type AgentGuardAnalysisPolicy struct {
	Enabled               bool     `json:"enabled"`
	TriggerSeverities     []string `json:"trigger_severities"`
	AIOnlyActionCeiling   string   `json:"ai_only_action_ceiling"`
	EvidenceWindowSeconds int      `json:"evidence_window_seconds"`
}

type AgentGuardEscapeRule struct {
	RuleID     string         `json:"rule_id"`
	Rule       string         `json:"rule"`
	Parameters map[string]any `json:"parameters"`
	Action     string         `json:"action"`
	Severity   string         `json:"severity"`
	Enabled    bool           `json:"enabled"`
}

// AgentGuardPolicyDraftUpdate contains only fields that may change while a
// policy remains in draft. Policy key, version and status are intentionally
// absent.
type AgentGuardPolicyDraftUpdate struct {
	Name                 string
	Description          string
	Priority             int
	Targets              datatypes.JSON
	CollectionPolicy     datatypes.JSON
	BuiltinRuleOverrides datatypes.JSON
	AtomicRules          datatypes.JSON
	CorrelationRules     datatypes.JSON
	AnalysisPolicy       datatypes.JSON
	EscapeRules          datatypes.JSON
	FreezeTimeoutSeconds int
	CompiledPreview      datatypes.JSON
	Digest               string
}

type AgentGuardPolicyValidationIssue struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type AgentGuardPolicyValidationPreview struct {
	Valid             bool                              `json:"valid"`
	Digest            string                            `json:"digest,omitempty"`
	CompiledPreview   map[string]any                    `json:"compiled_preview,omitempty"`
	DefinitionDigests map[string]string                 `json:"definition_digests,omitempty"`
	Errors            []AgentGuardPolicyValidationIssue `json:"errors"`
	Warnings          []AgentGuardPolicyValidationIssue `json:"warnings"`
}

type AgentGuardPageQuery struct {
	Page     int `form:"page" json:"page"`
	PageSize int `form:"page_size" json:"page_size"`
}

type AgentGuardProfileQuery struct {
	AgentGuardPageQuery
	AgentType string `form:"agent_type" json:"agent_type"`
	Source    string `form:"source" json:"source"`
	Enabled   *bool  `form:"enabled" json:"enabled"`
	Keyword   string `form:"keyword" json:"keyword"`
}

type AgentBehaviorRuleQuery struct {
	AgentGuardPageQuery
	Source   string `form:"source" json:"source"`
	Engine   string `form:"engine" json:"engine"`
	Category string `form:"category" json:"category"`
	Keyword  string `form:"keyword" json:"keyword"`
}

type AgentGuardPolicyQuery struct {
	AgentGuardPageQuery
	Status  string `form:"status" json:"status"`
	Keyword string `form:"keyword" json:"keyword"`
}

type AgentGuardDeliveryQuery struct {
	AgentGuardPageQuery
	HostID string `form:"host_id" json:"host_id"`
	Status string `form:"status" json:"status"`
}

type AgentGuardAgentQuery struct {
	AgentGuardPageQuery
	HostIDs          []string `form:"host_ids" json:"host_ids"`
	AgentTypes       []string `form:"agent_types" json:"agent_types"`
	RuntimeStatus    string   `form:"runtime_status" json:"runtime_status"`
	Coverage         string   `form:"coverage" json:"coverage"`
	IsolationType    string   `form:"isolation_type" json:"isolation_type"`
	HasHighRisk      *bool    `form:"has_high_risk" json:"has_high_risk"`
	HasEscapeFinding *bool    `form:"has_escape_finding" json:"has_escape_finding"`
	Keyword          string   `form:"keyword" json:"keyword"`
}

type AgentRuntimeInstanceQuery struct {
	AgentGuardPageQuery
	AgentScopeKey string   `form:"agent_scope_key" json:"agent_scope_key"`
	HostID        string   `form:"host_id" json:"host_id"`
	AssetIDs      []string `form:"asset_ids" json:"asset_ids"`
	AgentTypes    []string `form:"agent_types" json:"agent_types"`
	InstanceIDs   []string `form:"instance_ids" json:"instance_ids"`
	ProfileKey    string   `form:"profile_key" json:"profile_key"`
	Status        string   `form:"status" json:"status"`
	Coverage      string   `form:"coverage" json:"coverage"`
	IsolationType string   `form:"isolation_type" json:"isolation_type"`
	ContainerID   string   `form:"container_id" json:"container_id"`
	StartTime     *time.Time
	EndTime       *time.Time
}

type AgentBehaviorSessionQuery struct {
	AgentGuardPageQuery
	InstanceID      string `form:"instance_id" json:"instance_id"`
	ExecutionUnitID string `form:"execution_unit_id" json:"execution_unit_id"`
	Status          string `form:"status" json:"status"`
	Source          string `form:"source" json:"source"`
}

type AgentExecutionUnitQuery struct {
	AgentGuardPageQuery
	HostID      string `form:"host_id" json:"host_id"`
	InstanceID  string `form:"instance_id" json:"instance_id"`
	UnitType    string `form:"unit_type" json:"unit_type"`
	Status      string `form:"status" json:"status"`
	Coverage    string `form:"coverage" json:"coverage"`
	ContainerID string `form:"container_id" json:"container_id"`
}

type AgentBehaviorEventQuery struct {
	AgentGuardPageQuery
	HostID                 string `form:"host_id" json:"host_id"`
	AgentType              string `form:"agent_type" json:"agent_type"`
	InstanceID             string `form:"instance_id" json:"instance_id"`
	SessionID              string `form:"session_id" json:"session_id"`
	ExecutionUnitID        string `form:"execution_unit_id" json:"execution_unit_id"`
	Category               string `form:"category" json:"category"`
	Operation              string `form:"operation" json:"operation"`
	Outcome                string `form:"outcome" json:"outcome"`
	ResourceType           string `form:"resource_type" json:"resource_type"`
	ResourceClassification string `form:"resource_classification" json:"resource_classification"`
	Decision               string `form:"decision" json:"decision"`
	Severity               string `form:"severity" json:"severity"`
	RuleID                 string `form:"rule_id" json:"rule_id"`
	PolicyID               string `form:"policy_id" json:"policy_id"`
	ResourceKeyword        string `form:"resource_keyword" json:"resource_keyword"`
	StartTime              *time.Time
	EndTime                *time.Time
}

type AgentSecurityFindingQuery struct {
	AgentGuardPageQuery
	AgentScopeKey   string   `form:"agent_scope_key" json:"agent_scope_key"`
	AssetID         string   `form:"asset_id" json:"asset_id"`
	HostID          string   `form:"host_id" json:"host_id"`
	AgentType       string   `form:"agent_type" json:"agent_type"`
	ProfileKey      string   `form:"profile_key" json:"profile_key"`
	InstanceID      string   `form:"instance_id" json:"instance_id"`
	InstanceIDs     []string `form:"instance_ids" json:"instance_ids"`
	SessionID       string   `form:"session_id" json:"session_id"`
	ExecutionUnitID string   `form:"execution_unit_id" json:"execution_unit_id"`
	Severity        string   `form:"severity" json:"severity"`
	Verdict         string   `form:"verdict" json:"verdict"`
	Status          string   `form:"status" json:"status"`
	ConfidenceMin   *float64 `form:"confidence_min" json:"confidence_min"`
	RuleID          string   `form:"rule_id" json:"rule_id"`
	AnalysisStatus  string   `form:"analysis_status" json:"analysis_status"`
	Handled         *bool    `form:"handled" json:"handled"`
	StartTime       *time.Time
	EndTime         *time.Time
}

type AgentSecurityAnalysisQuery struct {
	AgentGuardPageQuery
	FindingID string `form:"finding_id" json:"finding_id"`
	Status    string `form:"status" json:"status"`
}

type AgentGuardActionQuery struct {
	AgentGuardPageQuery
	HostID          string `form:"host_id" json:"host_id"`
	InstanceID      string `form:"instance_id" json:"instance_id"`
	ExecutionUnitID string `form:"execution_unit_id" json:"execution_unit_id"`
	Action          string `form:"action" json:"action"`
	Status          string `form:"status" json:"status"`
	Source          string `form:"source" json:"source"`
	StartTime       *time.Time
	EndTime         *time.Time
}

type AgentGuardHostSummary struct {
	ID       uuid.UUID `json:"id"`
	Hostname string    `json:"hostname"`
	IP       string    `json:"ip"`
}

// AgentGuardAgentSummary is intentionally a narrow outer-list DTO. It must
// never grow command line, path, address, isolation baseline or evidence fields.
type AgentGuardAgentSummary struct {
	AgentScopeKey        string                `json:"agent_scope_key"`
	AssetID              *uuid.UUID            `json:"asset_id,omitempty"`
	Host                 AgentGuardHostSummary `json:"host"`
	AgentType            string                `json:"agent_type"`
	DisplayName          string                `json:"display_name"`
	ProfileKey           string                `json:"profile_key,omitempty"`
	RunningInstanceCount int                   `json:"running_instance_count"`
	ControllerPIDs       []int                 `json:"controller_pids"`
	RuntimeStatus        string                `json:"runtime_status"`
	IsolationTypes       []string              `json:"isolation_types"`
	CoverageLevel        string                `json:"coverage_level"`
	CoverageReasons      []string              `json:"coverage_reasons"`
	HighRiskFindingCount int64                 `json:"high_risk_finding_count"`
	EscapeFindingCount   int64                 `json:"escape_finding_count"`
	ActionStatus         string                `json:"action_status"`
	LastSeenAt           *time.Time            `json:"last_seen_at,omitempty"`
	ScopeIdentity        string                `json:"-"`
}

type AgentGuardOverview struct {
	RunningInstances   int64            `json:"running_instances"`
	ExecutionUnits     int64            `json:"execution_units"`
	Coverage           map[string]int64 `json:"coverage"`
	Behaviors24h       map[string]int64 `json:"behaviors_24h"`
	Findings24h        map[string]int64 `json:"findings_24h"`
	BuiltinRuleHits24h map[string]int64 `json:"builtin_rule_hits_24h"`
	PolicyHosts        map[string]int64 `json:"policy_hosts"`
}

type AgentGuardCoverageSummary struct {
	HostID          uuid.UUID `json:"host_id"`
	AgentType       string    `json:"agent_type"`
	ProfileKey      string    `json:"profile_key"`
	IsolationType   string    `json:"isolation_type"`
	CoverageLevel   string    `json:"coverage_level"`
	CoverageReasons []string  `json:"coverage_reasons"`
	InstanceCount   int64     `json:"instance_count"`
	UnitCount       int64     `json:"unit_count"`
}

type AgentGuardHostStatus struct {
	HostID             uuid.UUID                 `json:"host_id"`
	CapabilitySnapshot json.RawMessage           `json:"capability"`
	CoverageLevel      string                    `json:"coverage_level"`
	CoverageReasons    []string                  `json:"coverage_reasons"`
	LatestDelivery     *AgentGuardPolicyDelivery `json:"latest_delivery,omitempty"`
	ErrorCode          string                    `json:"error_code,omitempty"`
	ErrorMessage       string                    `json:"error_message,omitempty"`
}
