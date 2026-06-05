package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// AssistantInvestigationReport 攻击研判报告
type AssistantInvestigationReport struct {
	ID              uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	InvestigationID string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"investigation_id"`
	SessionID       string         `gorm:"type:varchar(100);index;not null" json:"session_id"`
	RunID           string         `gorm:"type:varchar(100)" json:"run_id"`
	HostID          string         `gorm:"type:varchar(160);not null" json:"host_id"`
	TaskType        string         `gorm:"type:varchar(60);not null;default:'host_attack_investigation'" json:"task_type"`
	Verdict         string         `gorm:"type:varchar(40);not null" json:"verdict"`
	Score           int            `gorm:"not null;default:0" json:"score"`
	Confidence      float64        `gorm:"type:numeric(5,4);not null;default:0" json:"confidence"`
	TimeRange       datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"time_range"`
	SourceCoverage  datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"source_coverage"`
	EntryCandidates datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"entry_candidates"`
	AttackTimeline  datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"attack_timeline"`
	AttackPath      datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"attack_path"`
	ImpactScope     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"impact_scope"`
	MissingEvidence datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"missing_evidence"`
	ReportMarkdown  string         `gorm:"type:text;not null;default:''" json:"report_markdown"`
	CreatedBy       string         `gorm:"type:varchar(100)" json:"created_by"`
	CreatedAt       time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (AssistantInvestigationReport) TableName() string {
	return "assistant_investigation_reports"
}

// AssistantInvestigationEvidence 攻击研判证据
type AssistantInvestigationEvidence struct {
	ID              uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	InvestigationID string         `gorm:"type:varchar(100);index;not null" json:"investigation_id"`
	EvidenceID      string         `gorm:"type:varchar(100);not null" json:"evidence_id"`
	SourceType      string         `gorm:"type:varchar(60);not null" json:"source_type"`
	SourceName      string         `gorm:"type:varchar(120);not null" json:"source_name"`
	ObjectType      string         `gorm:"type:varchar(60);not null" json:"object_type"`
	ObjectID        string         `gorm:"type:varchar(160)" json:"object_id"`
	HostID          string         `gorm:"type:varchar(160)" json:"host_id"`
	EventTime       *time.Time     `json:"event_time,omitempty"`
	Severity        string         `gorm:"type:varchar(40)" json:"severity"`
	MITREID         string         `gorm:"type:varchar(40)" json:"mitre_id"`
	Title           string         `gorm:"type:varchar(255);not null" json:"title"`
	Summary         string         `gorm:"type:text;not null;default:''" json:"summary"`
	Normalized      datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"normalized"`
	Supports        datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"supports"`
	Confidence      float64        `gorm:"type:numeric(5,4);not null;default:0" json:"confidence"`
	IsExternal      bool           `gorm:"not null;default:false" json:"is_external"`
	IsTruncated     bool           `gorm:"not null;default:false" json:"is_truncated"`
	CreatedAt       time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (AssistantInvestigationEvidence) TableName() string {
	return "assistant_investigation_evidence"
}

// Investigation verdict constants
const (
	VerdictConfirmedCompromised = "confirmed_compromised"
	VerdictSuspicious           = "suspicious"
	VerdictLikelyBenign         = "likely_benign"
	VerdictInsufficientEvidence = "insufficient_evidence"
)

// HostAttackInvestigationInput 主机攻击研判输入
type HostAttackInvestigationInput struct {
	SessionID          string                 `json:"session_id"`
	RunID              string                 `json:"run_id"`
	UserID             string                 `json:"user_id"`
	UserMessage        string                 `json:"user_message"`
	HostID             string                 `json:"host_id"`
	Hostname           string                 `json:"hostname,omitempty"`
	IPs                []string               `json:"ips,omitempty"`
	AlertIDs           []string               `json:"alert_ids,omitempty"`
	CVEIDs             []string               `json:"cve_ids,omitempty"`
	TimeRange          InvestigationTimeRange `json:"time_range"`
	IncludeAgentLive   bool                   `json:"include_agent_live"`
	IncludeExternalMCP bool                   `json:"include_external_mcp"`
	MCPSourceIDs       []string               `json:"mcp_source_ids,omitempty"`
	MaxEvidenceItems   int                    `json:"max_evidence_items"`
	Metadata           map[string]any         `json:"metadata,omitempty"`
}

// InvestigationTimeRange 研判时间范围
type InvestigationTimeRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// HostAttackInvestigationResult 主机攻击研判结果
type HostAttackInvestigationResult struct {
	InvestigationID      string                   `json:"investigation_id"`
	Host                 HostSnapshot             `json:"host"`
	TimeRange            InvestigationTimeRange   `json:"time_range"`
	CompromiseAssessment CompromiseAssessment     `json:"compromise_assessment"`
	EntryPointCandidates []EntryPointCandidate    `json:"entry_point_candidates"`
	AttackTimeline       AttackTimeline           `json:"attack_timeline"`
	AttackPath           AttackPathGraph          `json:"attack_path"`
	EvidenceMatrix       EvidenceMatrix           `json:"evidence_matrix"`
	MITRETechniques      []MITRETechniqueEvidence `json:"mitre_techniques"`
	ImpactScope          ImpactScope              `json:"impact_scope"`
	RecommendedActions   []RecommendedAction      `json:"recommended_actions"`
	MissingEvidence      []MissingEvidence        `json:"missing_evidence"`
	SourceCoverage       SourceCoverage           `json:"source_coverage"`
	ReportMarkdown       string                   `json:"report_markdown"`
	CreatedAt            time.Time                `json:"created_at"`
}

// HostSnapshot 主机快照
type HostSnapshot struct {
	HostID    string   `json:"host_id"`
	Hostname  string   `json:"hostname"`
	IPs       []string `json:"ips"`
	OS        string   `json:"os"`
	AgentStatus string `json:"agent_status"`
}

// CompromiseAssessment 被攻击评估
type CompromiseAssessment struct {
	Verdict        string   `json:"verdict"`
	Score          int      `json:"score"`
	Confidence     float64  `json:"confidence"`
	Summary        string   `json:"summary"`
	KeyReasons     []string `json:"key_reasons"`
	Contradictions []string `json:"contradictions,omitempty"`
}

// EntryPointCandidate 入口候选
type EntryPointCandidate struct {
	CandidateID        string    `json:"candidate_id"`
	EntryType          string    `json:"entry_type"`
	Title              string    `json:"title"`
	Score              int       `json:"score"`
	Confidence         float64   `json:"confidence"`
	FirstSeenAt        *time.Time `json:"first_seen_at,omitempty"`
	EvidenceIDs        []string  `json:"evidence_ids"`
	CounterEvidenceIDs []string  `json:"counter_evidence_ids,omitempty"`
	RelatedCVEIDs      []string  `json:"related_cve_ids,omitempty"`
	RelatedBaselineIDs []string  `json:"related_baseline_ids,omitempty"`
	Explanation        string    `json:"explanation"`
}

// AttackTimeline 攻击时间线
type AttackTimeline struct {
	Events []AttackTimelineEvent `json:"events"`
}

// AttackTimelineEvent 攻击时间线事件
type AttackTimelineEvent struct {
	EventID     string    `json:"event_id"`
	Time        time.Time `json:"time"`
	Phase       string    `json:"phase"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	EvidenceIDs []string  `json:"evidence_ids"`
	Confidence  float64   `json:"confidence"`
}

// AttackPathGraph 攻击路径图
type AttackPathGraph struct {
	Nodes []AttackPathNode `json:"nodes"`
	Edges []AttackPathEdge `json:"edges"`
}

// AttackPathNode 攻击路径节点
type AttackPathNode struct {
	NodeID      string   `json:"node_id"`
	NodeType    string   `json:"node_type"`
	Label       string   `json:"label"`
	RiskLevel   string   `json:"risk_level"`
	EvidenceIDs []string `json:"evidence_ids"`
}

// AttackPathEdge 攻击路径边
type AttackPathEdge struct {
	From        string   `json:"from"`
	To          string   `json:"to"`
	Relation    string   `json:"relation"`
	EvidenceIDs []string `json:"evidence_ids"`
	Confidence  float64  `json:"confidence"`
}

// EvidenceItem 证据项
type EvidenceItem struct {
	EvidenceID  string         `json:"evidence_id"`
	SourceType  string         `json:"source_type"`
	SourceName  string         `json:"source_name"`
	ObjectType  string         `json:"object_type"`
	ObjectID    string         `json:"object_id"`
	HostID      string         `json:"host_id"`
	Timestamp   *time.Time     `json:"timestamp,omitempty"`
	Severity    string         `json:"severity"`
	MITREID     string         `json:"mitre_id,omitempty"`
	Title       string         `json:"title"`
	Summary     string         `json:"summary"`
	RawSummary  string         `json:"raw_summary,omitempty"`
	Normalized  map[string]any `json:"normalized"`
	Supports    []string       `json:"supports"`
	Confidence  float64        `json:"confidence"`
	IsExternal  bool           `json:"is_external"`
	IsTruncated bool           `json:"is_truncated"`
}

// EvidenceMatrix 证据矩阵
type EvidenceMatrix struct {
	Items       []EvidenceItem       `json:"items"`
	ByPhase     map[string][]string   `json:"by_phase"`
	BySource    map[string][]string   `json:"by_source"`
	ByMITRE     map[string][]string   `json:"by_mitre"`
	KeyEvidence []string              `json:"key_evidence"`
}

// MITRETechniqueEvidence MITRE 技术证据
type MITRETechniqueEvidence struct {
	TechniqueID string   `json:"technique_id"`
	Name        string   `json:"name"`
	EvidenceIDs []string `json:"evidence_ids"`
}

// ImpactScope 影响范围
type ImpactScope struct {
	AffectedHosts   []string `json:"affected_hosts"`
	AffectedUsers   []string `json:"affected_users"`
	AffectedProcesses []string `json:"affected_processes"`
	AffectedFiles   []string `json:"affected_files"`
	AffectedNetwork []string `json:"affected_network"`
}

// RecommendedAction 建议动作
type RecommendedAction struct {
	ActionType  string `json:"action_type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	RiskLevel   string `json:"risk_level"`
	ToolName    string `json:"tool_name,omitempty"`
}

// MissingEvidence 缺失证据
type MissingEvidence struct {
	SourceType     string `json:"source_type"`
	Description    string `json:"description"`
	SuggestedAction string `json:"suggested_action"`
}

// SourceCoverage 数据源覆盖
type SourceCoverage struct {
	AegisInternal  bool     `json:"aegis_internal"`
	AgentLive      bool     `json:"agent_live"`
	ExternalMCP    bool     `json:"external_mcp"`
	MCPSourceNames []string `json:"mcp_source_names,omitempty"`
}
