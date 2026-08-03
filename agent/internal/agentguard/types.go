package agentguard

import "time"

const (
	BundleSchemaV1   = "aegis.agent_guard.bundle.v1"
	BehaviorSchemaV1 = "aegis.agent_behavior.v1"
	GuardSchemaV1    = "aegis.agent_guard.v1"
)

type Confidence string

const (
	ConfidenceConfirmed    Confidence = "confirmed"
	ConfidenceProbable     Confidence = "probable"
	ConfidenceInferred     Confidence = "inferred"
	ConfidenceCandidate    Confidence = "candidate"
	ConfidenceAmbiguous    Confidence = "ambiguous"
	ConfidenceUnattributed Confidence = "unattributed"
)

type IsolationType string

const (
	IsolationLocalProcessTree      IsolationType = "local_process_tree"
	IsolationLinuxNamespace        IsolationType = "linux_namespace"
	IsolationOCIContainer          IsolationType = "oci_container"
	IsolationRemoteSandbox         IsolationType = "remote_sandbox"
	IsolationWholeProcessContainer IsolationType = "whole_process_container"
)

type CoverageLevel string

const (
	CoverageFullEnforcement    CoverageLevel = "full_enforcement"
	CoverageMonitorOnly        CoverageLevel = "monitor_only"
	CoverageNoIsolation        CoverageLevel = "no_isolation"
	CoverageRemoteUnobservable CoverageLevel = "remote_unobservable"
	CoverageDegraded           CoverageLevel = "degraded"
)

type ProcessIdentity struct {
	PID        uint32 `json:"pid"`
	StartTicks uint64 `json:"start_ticks"`
}

func (p ProcessIdentity) Valid() bool {
	return p.PID > 0 && p.StartTicks > 0
}

type ProcessSnapshot struct {
	Identity         ProcessIdentity `json:"identity"`
	PPID             uint32          `json:"ppid"`
	ParentStartTicks uint64          `json:"parent_start_ticks,omitempty"`
	Exe              string          `json:"exe"`
	Argv             []string        `json:"argv,omitempty"`
	CWD              string          `json:"cwd,omitempty"`
	UID              uint32          `json:"uid"`
	GID              uint32          `json:"gid"`
	CgroupPath       string          `json:"cgroup_path,omitempty"`
	ContainerID      string          `json:"container_id,omitempty"`
	ConfigEvidence   []string        `json:"config_evidence,omitempty"`
	KnownParent      bool            `json:"known_parent,omitempty"`
	KnownHelper      bool            `json:"known_helper,omitempty"`
	ContainerLabel   bool            `json:"container_label,omitempty"`
}

type AdapterProfile struct {
	ProfileKey           string             `json:"profile_key"`
	ProfileVersion       int64              `json:"profile_version"`
	AgentType            string             `json:"agent_type"`
	DisplayName          string             `json:"display_name"`
	SandboxFamily        IsolationType      `json:"sandbox_family"`
	ControllerMatch      []ProcessMatchRule `json:"controller_match"`
	WorkerMatch          []ProcessMatchRule `json:"worker_match"`
	BackendDetectors     []BackendDetector  `json:"backend_detectors"`
	IsolationExpectation map[string]any     `json:"isolation_expectation"`
	DefaultEscapeRules   []string           `json:"default_escape_rules"`
	Digest               string             `json:"digest"`
}

type ProcessMatchRule struct {
	ExeBasenames             []string `json:"exe_basenames,omitempty"`
	CmdlineTokens            []string `json:"cmdline_tokens,omitempty"`
	ConfigPaths              []string `json:"config_paths,omitempty"`
	EvidenceWeight           int      `json:"evidence_weight,omitempty"`
	AncestorBasenames        []string `json:"ancestor_basenames,omitempty"`
	AncestorCmdlineTokens    []string `json:"ancestor_cmdline_tokens,omitempty"`
	ContainerLabels          []string `json:"container_labels,omitempty"`
	BackendRequired          string   `json:"backend_required,omitempty"`
	ForkDescendant           bool     `json:"fork_descendant,omitempty"`
	NamespaceHelper          bool     `json:"namespace_helper,omitempty"`
	RequiredNamespaceChanges []string `json:"required_namespace_changes,omitempty"`
}

type BackendDetector struct {
	Backend string   `json:"backend"`
	Signals []string `json:"signals"`
}

type ProfileMatch struct {
	Profile    *AdapterProfile
	Confidence Confidence
	Evidence   []string
}

type RuntimeInstance struct {
	InstanceID         string          `json:"instance_id"`
	HostID             string          `json:"host_id"`
	AssetID            string          `json:"asset_id,omitempty"`
	ProfileKey         string          `json:"profile_key"`
	ProfileVersion     int64           `json:"profile_version"`
	AgentType          string          `json:"agent_type"`
	DisplayName        string          `json:"display_name"`
	Controller         ProcessIdentity `json:"controller"`
	ControllerExe      string          `json:"controller_exe,omitempty"`
	RunUID             uint32          `json:"run_uid"`
	Confidence         Confidence      `json:"confidence"`
	Status             string          `json:"status"`
	Coverage           CoverageLevel   `json:"coverage"`
	LaunchedByInstance string          `json:"launched_by_instance,omitempty"`
	FirstSeenAt        time.Time       `json:"first_seen_at"`
	LastSeenAt         time.Time       `json:"last_seen_at"`
}

type BehaviorSession struct {
	SessionID            string     `json:"session_id"`
	InstanceID           string     `json:"instance_id"`
	Source               string     `json:"source"`
	Confidence           Confidence `json:"confidence"`
	CorrelationTokenHash string     `json:"correlation_token_hash,omitempty"`
	Status               string     `json:"status"`
	FirstSeenAt          time.Time  `json:"first_seen_at"`
	LastSeenAt           time.Time  `json:"last_seen_at"`
}

type ExecutionUnit struct {
	UnitID            string            `json:"unit_id"`
	InstanceID        string            `json:"instance_id"`
	SessionID         string            `json:"session_id"`
	Type              IsolationType     `json:"type"`
	RootProcess       ProcessIdentity   `json:"root_process"`
	CgroupPath        string            `json:"cgroup_path,omitempty"`
	ContainerID       string            `json:"container_id,omitempty"`
	ContainerRuntime  string            `json:"container_runtime,omitempty"`
	RemoteExecutionID string            `json:"remote_execution_id,omitempty"`
	Coverage          CoverageLevel     `json:"coverage"`
	Capabilities      GuardCapabilities `json:"capabilities"`
	IsolationBaseline IsolationState    `json:"isolation_baseline"`
	IsolationActual   IsolationState    `json:"isolation_actual"`
	IsolationDiff     IsolationDiff     `json:"isolation_diff"`
	Completeness      string            `json:"completeness"`
	Status            string            `json:"status"`
	FirstSeenAt       time.Time         `json:"first_seen_at"`
	LastSeenAt        time.Time         `json:"last_seen_at"`
}

type GuardSubject struct {
	InstanceID string     `json:"instance_id"`
	SessionID  string     `json:"session_id"`
	UnitID     string     `json:"unit_id"`
	Confidence Confidence `json:"confidence"`
}

type ContainerCgroup struct {
	Version     int    `json:"version"`
	Runtime     string `json:"runtime"`
	ContainerID string `json:"container_id"`
	Path        string `json:"path"`
}

type Category string

const (
	CategoryProcess   Category = "process"
	CategoryFile      Category = "file"
	CategoryNetwork   Category = "network"
	CategoryIdentity  Category = "identity"
	CategoryKernel    Category = "kernel"
	CategoryIsolation Category = "isolation"
	CategoryIPC       Category = "ipc"
	CategoryControl   Category = "control"
	CategoryTool      Category = "tool"
)

type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailed  Outcome = "failed"
	OutcomeUnknown Outcome = "unknown"
)

type Decision string

const (
	DecisionAudit                  Decision = "audit"
	DecisionAlert                  Decision = "alert"
	DecisionWouldDeny              Decision = "would_deny"
	DecisionEnforcementUnavailable Decision = "enforcement_unavailable"
	DecisionDeny                   Decision = "deny"
	DecisionDenyAndFreeze          Decision = "deny_and_freeze"
)

type Actor struct {
	PID        uint32   `json:"pid"`
	StartTicks uint64   `json:"start_ticks"`
	PPID       uint32   `json:"ppid"`
	Exe        string   `json:"exe,omitempty"`
	Argv       []string `json:"argv,omitempty"`
	CWD        string   `json:"cwd,omitempty"`
	UID        uint32   `json:"uid"`
	GID        uint32   `json:"gid"`
}

type Resource struct {
	Type           string         `json:"type"`
	Identity       string         `json:"identity"`
	Classification string         `json:"classification,omitempty"`
	Attributes     map[string]any `json:"attributes,omitempty"`
}

type CollectionEvidence struct {
	Source              string   `json:"source"`
	Sensor              string   `json:"sensor"`
	Visibility          string   `json:"visibility"`
	TruncatedFields     []string `json:"truncated_fields"`
	LostEventsSinceLast uint64   `json:"lost_events_since_last"`
	AggregatedCount     uint64   `json:"aggregated_count,omitempty"`
	CoverageLevel       string   `json:"coverage_level,omitempty"`
	CoverageReasons     []string `json:"coverage_reasons,omitempty"`
	Completeness        string   `json:"completeness,omitempty"`
}

type BehaviorEvent struct {
	Schema                string             `json:"schema"`
	EventID               string             `json:"event_id"`
	EventType             string             `json:"event_type"`
	HostID                string             `json:"host_id"`
	HostBootID            string             `json:"host_boot_id"`
	AgentSequence         uint64             `json:"agent_sequence"`
	InstanceID            string             `json:"instance_id"`
	ExecutionUnitID       string             `json:"execution_unit_id"`
	SessionID             string             `json:"session_id"`
	CorrelationID         string             `json:"correlation_id,omitempty"`
	ParentEventID         string             `json:"parent_event_id,omitempty"`
	AgentType             string             `json:"agent_type,omitempty"`
	ProfileKey            string             `json:"profile_key,omitempty"`
	ProfileVersion        int64              `json:"profile_version,omitempty"`
	OccurredAt            time.Time          `json:"occurred_at"`
	OccurredMonotonicNS   uint64             `json:"occurred_monotonic_ns"`
	Category              Category           `json:"category"`
	Operation             string             `json:"operation"`
	Outcome               Outcome            `json:"outcome"`
	Errno                 int32              `json:"errno"`
	Actor                 Actor              `json:"actor"`
	Resource              Resource           `json:"resource"`
	AttributionConfidence Confidence         `json:"attribution_confidence"`
	Decision              Decision           `json:"decision"`
	Severity              string             `json:"severity"`
	RuleID                string             `json:"rule_id,omitempty"`
	Isolation             map[string]any     `json:"isolation,omitempty"`
	Evidence              map[string]any     `json:"evidence,omitempty"`
	Collection            CollectionEvidence `json:"collection"`
}

type RawBehavior struct {
	EventID             string
	OccurredAt          time.Time
	OccurredMonotonicNS uint64
	Category            Category
	Operation           string
	Outcome             Outcome
	Errno               int32
	Process             ProcessSnapshot
	Argv                []string
	Resource            Resource
	Source              string
	Sensor              string
	Visibility          string
	LostEvents          uint64
	EventType           string
	SessionID           string
	CorrelationID       string
	ParentEventID       string
	Decision            Decision
	Severity            string
	RuleID              string
	Isolation           map[string]any
	Evidence            map[string]any
}

type EvidenceAvailability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type CapabilityState struct {
	Visible     bool   `json:"visible"`
	Inheritable string `json:"inheritable,omitempty"`
	Permitted   string `json:"permitted,omitempty"`
	Effective   string `json:"effective,omitempty"`
	Bounding    string `json:"bounding,omitempty"`
	Ambient     string `json:"ambient,omitempty"`
}

type IsolationState struct {
	NamespaceInodes  map[string]uint64               `json:"namespace_inodes"`
	CgroupPath       string                          `json:"cgroup_path,omitempty"`
	CgroupVersion    int                             `json:"cgroup_version,omitempty"`
	ContainerID      string                          `json:"container_id,omitempty"`
	ContainerRuntime string                          `json:"container_runtime,omitempty"`
	RootMount        string                          `json:"root_mount,omitempty"`
	MountInfoDigest  string                          `json:"mount_info_digest,omitempty"`
	MountCount       int                             `json:"mount_count,omitempty"`
	MountPropagation []string                        `json:"mount_propagation"`
	Capabilities     CapabilityState                 `json:"capabilities"`
	NoNewPrivileges  *bool                           `json:"no_new_privs"`
	SeccompMode      *int                            `json:"seccomp_mode"`
	Availability     map[string]EvidenceAvailability `json:"availability"`
	CapturedAt       time.Time                       `json:"captured_at"`
}

type StateDifference struct {
	Before any `json:"before"`
	After  any `json:"after"`
}

type IsolationDiff struct {
	StateChanged bool                       `json:"state_changed"`
	Changes      map[string]StateDifference `json:"changes"`
	Unavailable  []string                   `json:"unavailable"`
}

type GuardCapabilities struct {
	KernelRelease   string   `json:"kernel_release"`
	BTF             bool     `json:"btf"`
	RingBuffer      bool     `json:"ring_buffer"`
	PerfBuffer      bool     `json:"perf_buffer"`
	BPFLSM          bool     `json:"bpf_lsm"`
	CgroupVersion   int      `json:"cgroup_version"`
	CgroupFreeze    bool     `json:"cgroup_freeze"`
	Pidfd           bool     `json:"pidfd"`
	NamespaceRead   bool     `json:"namespace_read"`
	MountInfoRead   bool     `json:"mountinfo_read"`
	SupportedHooks  []string `json:"supported_hooks"`
	DegradedReasons []string `json:"degraded_reasons"`
}

type GuardAttempt struct {
	EventID          string
	Category         Category
	Operation        string
	Target           string
	SecondaryTarget  string
	TargetPID        uint32
	TargetNamespace  string
	ReturnCode       int64
	BeforeUID        uint32
	BeforeUIDVisible bool
	Baseline         IsolationState
	Actual           IsolationState
	EvidenceEventIDs []string
}

type SandboxViolation struct {
	Rule             string         `json:"rule"`
	Operation        string         `json:"operation"`
	Target           string         `json:"target,omitempty"`
	Baseline         IsolationState `json:"baseline"`
	Actual           IsolationState `json:"actual"`
	Diff             IsolationDiff  `json:"diff"`
	StateChanged     bool           `json:"state_changed"`
	ReturnCode       int64          `json:"return_code"`
	Decision         Decision       `json:"decision"`
	Severity         string         `json:"severity"`
	EvidenceEventIDs []string       `json:"evidence_event_ids"`
}
