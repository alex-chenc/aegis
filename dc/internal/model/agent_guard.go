package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AgentBehaviorEvent is the immutable, query-oriented projection of one
// aegis.agent_behavior.v1 runtime event. Raw RuntimeEvent persistence remains
// independent so Kafka replay can retry either write safely.
type AgentBehaviorEvent struct {
	ID                     uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	RawEventID             string          `gorm:"column:raw_event_id;uniqueIndex;not null" json:"raw_event_id"`
	HostID                 uuid.UUID       `gorm:"type:uuid;index;not null" json:"host_id"`
	HostBootID             string          `gorm:"column:host_boot_id;not null" json:"host_boot_id"`
	AgentSequence          int64           `gorm:"column:agent_sequence;not null" json:"agent_sequence"`
	InstanceID             *uuid.UUID      `gorm:"type:uuid" json:"instance_id,omitempty"`
	SessionID              *uuid.UUID      `gorm:"type:uuid" json:"session_id,omitempty"`
	ExecutionUnitID        *uuid.UUID      `gorm:"type:uuid" json:"execution_unit_id,omitempty"`
	PolicyID               *uuid.UUID      `gorm:"type:uuid" json:"policy_id,omitempty"`
	PolicyVersion          *int64          `json:"policy_version,omitempty"`
	RuleID                 string          `json:"rule_id,omitempty"`
	SchemaVersion          string          `gorm:"column:schema_version;not null" json:"schema_version"`
	CorrelationID          string          `json:"correlation_id,omitempty"`
	ParentEventID          string          `json:"parent_event_id,omitempty"`
	AgentType              string          `json:"agent_type,omitempty"`
	ProfileKey             string          `json:"profile_key,omitempty"`
	ProfileVersion         *int64          `json:"profile_version,omitempty"`
	Category               string          `gorm:"not null" json:"category"`
	Operation              string          `gorm:"not null" json:"operation"`
	Outcome                string          `gorm:"not null" json:"outcome"`
	Errno                  *int            `json:"errno,omitempty"`
	Decision               string          `gorm:"not null" json:"decision"`
	Severity               string          `gorm:"not null" json:"severity"`
	PID                    int             `gorm:"column:pid" json:"pid,omitempty"`
	PPID                   int             `gorm:"column:ppid" json:"ppid,omitempty"`
	ProcessStartTicks      string          `gorm:"column:process_start_ticks" json:"process_start_ticks,omitempty"`
	ProcessName            string          `json:"process_name,omitempty"`
	ProcessExe             string          `json:"process_exe,omitempty"`
	CommandArgv            json.RawMessage `gorm:"column:command_argv;type:jsonb;not null" json:"command_argv"`
	CommandCWD             string          `json:"command_cwd,omitempty"`
	CommandVisibility      string          `gorm:"not null" json:"command_visibility"`
	ProcessChain           json.RawMessage `gorm:"type:jsonb;not null" json:"process_chain"`
	ResourceType           string          `json:"resource_type,omitempty"`
	ResourceIdentity       string          `json:"resource_identity,omitempty"`
	ResourceIdentityHash   string          `json:"resource_identity_hash,omitempty"`
	ResourceClassification string          `json:"resource_classification,omitempty"`
	Resource               json.RawMessage `gorm:"type:jsonb;not null" json:"resource"`
	Isolation              json.RawMessage `gorm:"type:jsonb;not null" json:"isolation"`
	Collection             json.RawMessage `gorm:"type:jsonb;not null" json:"collection"`
	Evidence               json.RawMessage `gorm:"type:jsonb;not null" json:"evidence"`
	OccurredAt             time.Time       `gorm:"not null" json:"occurred_at"`
	OccurredMonotonicNS    *int64          `gorm:"column:occurred_monotonic_ns" json:"occurred_monotonic_ns,omitempty"`
	ReceivedAt             time.Time       `gorm:"not null" json:"received_at"`
	CreatedAt              time.Time       `json:"created_at"`
	AggregatedCount        int64           `gorm:"-" json:"-"`
	LostEventsSinceLast    int64           `gorm:"-" json:"-"`
	HasTruncatedFields     bool            `gorm:"-" json:"-"`
	Completeness           json.RawMessage `gorm:"-" json:"-"`
}

func (AgentBehaviorEvent) TableName() string {
	return "agent_behavior_events"
}

type AgentRuntimeInstance struct {
	ID                   uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	HostID               uuid.UUID       `gorm:"type:uuid;not null" json:"host_id"`
	AssetID              *uuid.UUID      `gorm:"type:uuid" json:"asset_id,omitempty"`
	ProfileKey           string          `gorm:"not null" json:"profile_key"`
	ProfileVersion       int64           `gorm:"not null" json:"profile_version"`
	AgentType            string          `gorm:"not null" json:"agent_type"`
	DisplayName          string          `json:"display_name,omitempty"`
	ControllerPID        int             `gorm:"column:controller_pid;not null" json:"controller_pid"`
	ControllerStartTicks string          `gorm:"column:controller_start_ticks;not null" json:"controller_start_ticks"`
	ControllerExe        string          `json:"controller_exe,omitempty"`
	RunUID               *int            `gorm:"column:run_uid" json:"run_uid,omitempty"`
	DetectionConfidence  string          `gorm:"not null" json:"detection_confidence"`
	Status               string          `gorm:"not null" json:"status"`
	CoverageLevel        string          `gorm:"not null" json:"coverage_level"`
	CoverageReasons      json.RawMessage `gorm:"type:jsonb;not null" json:"coverage_reasons"`
	Metadata             json.RawMessage `gorm:"type:jsonb;not null" json:"metadata"`
	FirstSeenAt          time.Time       `gorm:"not null" json:"first_seen_at"`
	LastSeenAt           time.Time       `gorm:"not null" json:"last_seen_at"`
	StoppedAt            *time.Time      `json:"stopped_at,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

func (AgentRuntimeInstance) TableName() string {
	return "agent_runtime_instances"
}

type AgentExecutionUnit struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	HostID            uuid.UUID       `gorm:"type:uuid;not null" json:"host_id"`
	InstanceID        uuid.UUID       `gorm:"type:uuid;not null" json:"instance_id"`
	UnitType          string          `gorm:"not null" json:"unit_type"`
	Fingerprint       string          `gorm:"not null" json:"fingerprint"`
	RootPID           *int            `gorm:"column:root_pid" json:"root_pid,omitempty"`
	RootStartTicks    *string         `gorm:"column:root_start_ticks" json:"root_start_ticks,omitempty"`
	CgroupPath        string          `json:"cgroup_path,omitempty"`
	ContainerID       string          `json:"container_id,omitempty"`
	ContainerRuntime  string          `json:"container_runtime,omitempty"`
	RemoteBackend     string          `json:"remote_backend,omitempty"`
	RemoteExecutionID string          `json:"remote_execution_id,omitempty"`
	RemoteHostRef     string          `json:"remote_host_ref,omitempty"`
	CoverageLevel     string          `gorm:"not null" json:"coverage_level"`
	CoverageReasons   json.RawMessage `gorm:"type:jsonb;not null" json:"coverage_reasons"`
	IsolationBaseline json.RawMessage `gorm:"type:jsonb;not null" json:"isolation_baseline"`
	IsolationActual   json.RawMessage `gorm:"type:jsonb;not null" json:"isolation_actual"`
	IsolationDiff     json.RawMessage `gorm:"type:jsonb;not null" json:"isolation_diff"`
	Status            string          `gorm:"not null" json:"status"`
	FirstSeenAt       time.Time       `gorm:"not null" json:"first_seen_at"`
	LastSeenAt        time.Time       `gorm:"not null" json:"last_seen_at"`
	StoppedAt         *time.Time      `json:"stopped_at,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

func (AgentExecutionUnit) TableName() string {
	return "agent_execution_units"
}

type AgentBehaviorSession struct {
	ID                   uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	HostID               uuid.UUID       `gorm:"type:uuid;not null" json:"host_id"`
	InstanceID           uuid.UUID       `gorm:"type:uuid;not null" json:"instance_id"`
	ExecutionUnitID      *uuid.UUID      `gorm:"type:uuid" json:"execution_unit_id,omitempty"`
	ExternalSessionID    *string         `json:"external_session_id,omitempty"`
	Source               string          `gorm:"not null" json:"source"`
	Confidence           string          `gorm:"not null" json:"confidence"`
	CorrelationTokenHash *string         `json:"correlation_token_hash,omitempty"`
	Status               string          `gorm:"not null" json:"status"`
	BehaviorCount        int64           `gorm:"not null" json:"behavior_count"`
	FindingCount         int64           `gorm:"not null" json:"finding_count"`
	Completeness         json.RawMessage `gorm:"type:jsonb;not null" json:"completeness"`
	StartedAt            time.Time       `gorm:"not null" json:"started_at"`
	LastSeenAt           time.Time       `gorm:"not null" json:"last_seen_at"`
	EndedAt              *time.Time      `json:"ended_at,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

func (AgentBehaviorSession) TableName() string {
	return "agent_behavior_sessions"
}

type AgentGuardStateProjection struct {
	EventType string
	ObjectID  uuid.UUID
	Instance  *AgentRuntimeInstance
	Unit      *AgentExecutionUnit
	Session   *AgentBehaviorSession
	Delivery  *AgentGuardDeliveryStatus
	Action    *AgentGuardAction
}

type AgentGuardDeliveryStatus struct {
	HostID         uuid.UUID
	BundleVersion  int64
	BundleDigest   string
	Status         string
	ErrorCode      string
	LastReportedAt time.Time
}

type AgentGuardAction struct {
	ID                   uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	CommandID            string          `gorm:"column:command_id;type:varchar(100);uniqueIndex" json:"command_id,omitempty"`
	TriggerFindingID     *uuid.UUID      `gorm:"type:uuid;index" json:"trigger_finding_id,omitempty"`
	HostID               uuid.UUID       `gorm:"type:uuid;not null;index" json:"host_id"`
	InstanceID           *uuid.UUID      `gorm:"type:uuid;index" json:"instance_id,omitempty"`
	ExecutionUnitID      *uuid.UUID      `gorm:"type:uuid;index" json:"execution_unit_id,omitempty"`
	Action               string          `gorm:"type:varchar(40);not null" json:"action"`
	Source               string          `gorm:"type:varchar(32);not null" json:"source"`
	Status               string          `gorm:"type:varchar(32);not null;index" json:"status"`
	Reason               string          `gorm:"type:text;not null" json:"reason"`
	RequestedBy          string          `gorm:"type:varchar(100)" json:"requested_by,omitempty"`
	HoldRequested        bool            `gorm:"not null" json:"hold_requested"`
	FreezeTimeoutSeconds *int            `json:"freeze_timeout_seconds,omitempty"`
	Result               json.RawMessage `gorm:"type:jsonb;not null" json:"result"`
	ErrorCode            string          `gorm:"type:varchar(100)" json:"error_code,omitempty"`
	ErrorMessage         string          `gorm:"type:text" json:"error_message,omitempty"`
	RequestedAt          time.Time       `gorm:"not null" json:"requested_at"`
	DispatchedAt         *time.Time      `json:"dispatched_at,omitempty"`
	CompletedAt          *time.Time      `json:"completed_at,omitempty"`
	ExpiresAt            *time.Time      `json:"expires_at,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

func (AgentGuardAction) TableName() string {
	return "agent_guard_actions"
}

type AgentGuardPolicy struct {
	ID                   uuid.UUID       `gorm:"type:uuid;primaryKey"`
	Version              int64           `gorm:"not null"`
	Status               string          `gorm:"type:varchar(32);not null"`
	CorrelationRules     json.RawMessage `gorm:"type:jsonb;not null"`
	AtomicRules          json.RawMessage `gorm:"type:jsonb;not null"`
	EscapeRules          json.RawMessage `gorm:"type:jsonb;not null"`
	FreezeTimeoutSeconds int             `gorm:"not null"`
	PublishedAt          *time.Time
}

func (AgentGuardPolicy) TableName() string {
	return "agent_guard_policies"
}

type AgentSecurityFinding struct {
	ID                  uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	FindingKey          string          `gorm:"type:varchar(255);not null;uniqueIndex" json:"finding_key"`
	HostID              uuid.UUID       `gorm:"type:uuid;not null;index" json:"host_id"`
	InstanceID          *uuid.UUID      `gorm:"type:uuid;index" json:"instance_id,omitempty"`
	SessionID           *uuid.UUID      `gorm:"type:uuid;index" json:"session_id,omitempty"`
	ExecutionUnitID     *uuid.UUID      `gorm:"type:uuid;index" json:"execution_unit_id,omitempty"`
	PolicyID            *uuid.UUID      `gorm:"type:uuid;index" json:"policy_id,omitempty"`
	PolicyVersion       *int64          `json:"policy_version,omitempty"`
	Title               string          `gorm:"type:varchar(500);not null" json:"title"`
	Severity            string          `gorm:"type:varchar(20);not null;index" json:"severity"`
	Verdict             string          `gorm:"type:varchar(24);not null" json:"verdict"`
	Confidence          float64         `gorm:"type:numeric(5,4);not null" json:"confidence"`
	Status              string          `gorm:"type:varchar(24);not null;index" json:"status"`
	DecisionSources     json.RawMessage `gorm:"type:jsonb;not null" json:"decision_sources"`
	RuleHits            json.RawMessage `gorm:"type:jsonb;not null" json:"rule_hits"`
	EvidenceEventIDs    json.RawMessage `gorm:"type:jsonb;not null" json:"evidence_event_ids"`
	EvidenceGraph       json.RawMessage `gorm:"type:jsonb;not null" json:"evidence_graph"`
	AttackStages        json.RawMessage `gorm:"type:jsonb;not null" json:"attack_stages"`
	Summary             string          `gorm:"type:text" json:"summary,omitempty"`
	RecommendedAction   string          `gorm:"type:varchar(64)" json:"recommended_action,omitempty"`
	FirstObservedAt     time.Time       `gorm:"not null" json:"first_observed_at"`
	LastObservedAt      time.Time       `gorm:"not null;index" json:"last_observed_at"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	EvidenceSourceTable string          `gorm:"-" json:"-"`
}

func (AgentSecurityFinding) TableName() string {
	return "agent_security_findings"
}
