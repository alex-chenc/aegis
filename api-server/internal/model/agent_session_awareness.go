package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const (
	AgentSessionSourceClaude = "claude-code"
	AgentSessionSourceCodex  = "codex"
	AgentSessionModeStatic   = "static_scan"
	AgentSessionStateActive  = "active_inferred"
	AgentSessionStateIdle    = "idle_inferred"
	AgentSessionStateDone    = "complete"
	AgentSessionStateUnknown = "unknown"
	AgentSessionRiskLow      = "low"
	AgentSessionRiskMedium   = "medium"
	AgentSessionRiskHigh     = "high"
	AgentSessionRiskCritical = "critical"
)

// AgentConversationSession is the redacted session projection. It deliberately
// has no source_path field: source locations are represented by project_digest.
type AgentConversationSession struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	HostID                uuid.UUID  `gorm:"type:uuid;not null;index" json:"host_id"`
	AgentType             string     `gorm:"type:varchar(32);not null;index" json:"agent_type"`
	SourceMode            string     `gorm:"type:varchar(24);not null;default:static_scan" json:"source_mode"`
	SourceSubjectUID      int64      `gorm:"not null" json:"source_subject_uid"`
	ExternalSessionID     string     `gorm:"type:varchar(255);not null" json:"external_session_id"`
	ProjectDigest         string     `gorm:"type:varchar(80)" json:"project_digest,omitempty"`
	Title                 string     `gorm:"type:text" json:"title,omitempty"`
	Model                 string     `gorm:"type:varchar(128)" json:"model,omitempty"`
	State                 string     `gorm:"type:varchar(32);not null;default:unknown" json:"state"`
	FirstSeenAt           *time.Time `json:"first_seen_at,omitempty"`
	LastSeenAt            *time.Time `json:"last_seen_at,omitempty"`
	LastItemAt            *time.Time `json:"last_item_at,omitempty"`
	ItemCount             int64      `gorm:"not null;default:0" json:"item_count"`
	PromptCount           int64      `gorm:"not null;default:0" json:"prompt_count"`
	AssistantCount        int64      `gorm:"not null;default:0" json:"assistant_count"`
	ToolCallCount         int64      `gorm:"not null;default:0" json:"tool_call_count"`
	EstimatedInputTokens  int64      `gorm:"not null;default:0" json:"estimated_input_tokens"`
	EstimatedOutputTokens int64      `gorm:"not null;default:0" json:"estimated_output_tokens"`
	EstimatedTotalTokens  int64      `gorm:"not null;default:0" json:"estimated_total_tokens"`
	TokenEstimateMethod   string     `gorm:"type:varchar(32);not null;default:chars_div_4" json:"token_estimate_method"`
	RiskLevel             string     `gorm:"type:varchar(16);not null;default:unknown" json:"risk_level"`
	RuleHitCount          int64      `gorm:"not null;default:0" json:"rule_hit_count"`
	AIRiskScore           *float64   `gorm:"type:numeric(5,2)" json:"ai_risk_score,omitempty"`
	LastSequence          int64      `gorm:"not null;default:-1" json:"last_sequence"`
	LastCollectedAt       *time.Time `json:"last_collected_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func (AgentConversationSession) TableName() string { return "agent_conversation_sessions" }

type AgentConversationItem struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SessionID        uuid.UUID      `gorm:"type:uuid;not null;index" json:"session_id"`
	ItemID           string         `gorm:"type:varchar(128);not null" json:"item_id"`
	Sequence         int64          `gorm:"not null" json:"sequence"`
	ItemType         string         `gorm:"type:varchar(32);not null" json:"item_type"`
	Role             string         `gorm:"type:varchar(32)" json:"role,omitempty"`
	OccurredAt       *time.Time     `json:"occurred_at,omitempty"`
	ContentDigest    string         `gorm:"type:varchar(80)" json:"content_digest,omitempty"`
	ContentRedacted  string         `gorm:"type:text" json:"content_redacted,omitempty"`
	NormalizedJSON   datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"normalized_json"`
	Visibility       string         `gorm:"type:varchar(16);not null;default:normal" json:"visibility"`
	RedactionApplied bool           `gorm:"not null;default:false" json:"redaction_applied"`
	InputTokens      *int64         `json:"input_tokens,omitempty"`
	OutputTokens     *int64         `json:"output_tokens,omitempty"`
	TotalTokens      *int64         `json:"total_tokens,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

func (AgentConversationItem) TableName() string { return "agent_conversation_items" }

type AgentSessionRuleDefinition struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RuleKey     string         `gorm:"type:varchar(128);not null;index" json:"rule_key"`
	Version     int64          `gorm:"not null" json:"version"`
	Name        string         `gorm:"type:varchar(255);not null" json:"name"`
	Category    string         `gorm:"type:varchar(64);not null" json:"category"`
	Severity    string         `gorm:"type:varchar(16);not null" json:"severity"`
	Enabled     bool           `gorm:"not null;default:true" json:"enabled"`
	Pattern     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"pattern"`
	Description string         `gorm:"type:text" json:"description,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

func (AgentSessionRuleDefinition) TableName() string { return "agent_session_rule_definitions" }

type AgentSessionRuleHit struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SessionID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"session_id"`
	ItemID          *uuid.UUID `gorm:"type:uuid" json:"item_id,omitempty"`
	RuleID          *uuid.UUID `gorm:"type:uuid" json:"rule_id,omitempty"`
	RuleKey         string     `gorm:"type:varchar(128);not null" json:"rule_key"`
	Severity        string     `gorm:"type:varchar(16);not null" json:"severity"`
	Category        string     `gorm:"type:varchar(64);not null" json:"category"`
	EvidenceDigest  string     `gorm:"type:varchar(80)" json:"evidence_digest,omitempty"`
	EvidenceExcerpt string     `gorm:"type:text" json:"evidence_excerpt,omitempty"`
	Status          string     `gorm:"type:varchar(24);not null;default:open" json:"status"`
	// ItemSequence is derived from the item relation for API responses.
	ItemSequence *int64    `gorm:"-" json:"item_sequence,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func (AgentSessionRuleHit) TableName() string { return "agent_session_rule_hits" }

type AgentSessionAIRun struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SessionID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"session_id"`
	Provider      string     `gorm:"type:varchar(64);not null" json:"provider"`
	Model         string     `gorm:"type:varchar(128);not null" json:"model"`
	PromptVersion string     `gorm:"type:varchar(64);not null" json:"prompt_version"`
	Status        string     `gorm:"type:varchar(24);not null;default:queued" json:"status"`
	ChunkCount    int        `gorm:"not null;default:0" json:"chunk_count"`
	RiskScore     *float64   `gorm:"type:numeric(5,2)" json:"risk_score,omitempty"`
	Summary       string     `gorm:"type:text" json:"summary,omitempty"`
	ErrorCode     string     `gorm:"type:varchar(64)" json:"error_code,omitempty"`
	ErrorMessage  string     `gorm:"type:text" json:"error_message,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

func (AgentSessionAIRun) TableName() string { return "agent_session_ai_runs" }

type AgentSessionAIChunk struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RunID              uuid.UUID      `gorm:"type:uuid;not null;index" json:"run_id"`
	ChunkIndex         int            `gorm:"not null" json:"chunk_index"`
	ItemStartSequence  int64          `gorm:"not null" json:"item_start_sequence"`
	ItemEndSequence    int64          `gorm:"not null" json:"item_end_sequence"`
	InputTokenEstimate int64          `gorm:"not null;default:0" json:"input_token_estimate"`
	OutputJSON         datatypes.JSON `gorm:"type:jsonb" json:"output_json,omitempty"`
	Status             string         `gorm:"type:varchar(24);not null;default:queued" json:"status"`
	CreatedAt          time.Time      `json:"created_at"`
}

func (AgentSessionAIChunk) TableName() string { return "agent_session_ai_chunks" }
