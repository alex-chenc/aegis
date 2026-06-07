package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// AssistantSession 智能体会话
type AssistantSession struct {
	ID            uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SessionID     string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"session_id"`
	Title         string         `gorm:"type:varchar(255);not null" json:"title"`
	TaskType      string         `gorm:"type:varchar(40);not null;default:'explanation'" json:"task_type"`
	ModeSource    string         `gorm:"type:varchar(40);not null;default:'assistant'" json:"mode_source"`
	Status        string         `gorm:"type:varchar(32);not null;default:'active'" json:"status"`
	CreatedBy     string         `gorm:"type:varchar(100)" json:"created_by"`
	MessageCount  int            `gorm:"not null;default:0" json:"message_count"`
	ToolCallCount int            `gorm:"not null;default:0" json:"tool_call_count"`
	ApprovalCount int            `gorm:"not null;default:0" json:"approval_count"`
	LastMessageAt *time.Time     `json:"last_message_at,omitempty"`
	Metadata      datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	CreatedAt     time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (AssistantSession) TableName() string {
	return "assistant_sessions"
}

// AssistantMessage 智能体消息
type AssistantMessage struct {
	ID          uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SessionID   string         `gorm:"type:varchar(100);index;not null" json:"session_id"`
	MessageID   string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"message_id"`
	Role        string         `gorm:"type:varchar(20);not null" json:"role"`
	Content     string         `gorm:"type:text" json:"content"`
	Thinking    datatypes.JSON `gorm:"type:jsonb" json:"thinking,omitempty"` // JSON 数组，每个元素是一个思考步骤
	Plan        datatypes.JSON `gorm:"type:jsonb" json:"plan,omitempty"`
	ToolCalls   datatypes.JSON `gorm:"type:jsonb" json:"tool_calls,omitempty"`
	Approvals   datatypes.JSON `gorm:"type:jsonb" json:"approvals,omitempty"`
	ResultCards datatypes.JSON `gorm:"type:jsonb" json:"result_cards,omitempty"`
	CreatedAt   time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (AssistantMessage) TableName() string {
	return "assistant_messages"
}

// AssistantContextRef 上下文引用
type AssistantContextRef struct {
	ID         uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SessionID  string         `gorm:"type:varchar(100);index;not null" json:"session_id"`
	ObjectType string         `gorm:"type:varchar(40);not null" json:"object_type"`
	ObjectID   string         `gorm:"type:varchar(160);not null" json:"object_id"`
	Title      string         `gorm:"type:varchar(255)" json:"title"`
	Summary    string         `gorm:"type:text" json:"summary,omitempty"`
	RoutePath  string         `gorm:"type:varchar(255)" json:"route_path,omitempty"`
	Snapshot   datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"snapshot"`
	CreatedAt  time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (AssistantContextRef) TableName() string {
	return "assistant_context_refs"
}

// AssistantToolCall 工具调用记录
type AssistantToolCall struct {
	ID            uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SessionID     string         `gorm:"type:varchar(100);index;not null" json:"session_id"`
	MessageID     string         `gorm:"type:varchar(100);index" json:"message_id"`
	CallID        string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"call_id"`
	ToolName      string         `gorm:"type:varchar(120);not null" json:"tool_name"`
	Domain        string         `gorm:"type:varchar(40);not null" json:"domain"`
	RiskLevel     string         `gorm:"type:varchar(20);not null" json:"risk_level"`
	Status        string         `gorm:"type:varchar(32);not null" json:"status"`
	Args          datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"args"`
	ArgsSummary   string         `gorm:"type:text" json:"args_summary"`
	Result        datatypes.JSON `gorm:"type:jsonb" json:"result,omitempty"`
	ResultSummary string         `gorm:"type:text" json:"result_summary,omitempty"`
	ErrorMessage  string         `gorm:"type:text" json:"error_message,omitempty"`
	DurationMs    int64          `json:"duration_ms"`
	CreatedAt     time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (AssistantToolCall) TableName() string {
	return "assistant_tool_calls"
}

// AssistantApproval 审批记录
type AssistantApproval struct {
	ID            uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	ApprovalID    string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"approval_id"`
	SessionID     string         `gorm:"type:varchar(100);index;not null" json:"session_id"`
	ToolCallID    string         `gorm:"type:varchar(100);index;not null" json:"tool_call_id"`
	ToolName      string         `gorm:"type:varchar(120);not null" json:"tool_name"`
	RiskLevel     string         `gorm:"type:varchar(20);not null" json:"risk_level"`
	Title         string         `gorm:"type:varchar(255);not null" json:"title"`
	ImpactSummary string         `gorm:"type:text" json:"impact_summary"`
	ParamsPreview datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"params_preview"`
	RollbackHint  string         `gorm:"type:text" json:"rollback_hint"`
	Status        string         `gorm:"type:varchar(32);not null;default:'pending'" json:"status"`
	RequestedBy   string         `gorm:"type:varchar(100)" json:"requested_by"`
	ReviewedBy    string         `gorm:"type:varchar(100)" json:"reviewed_by"`
	ReviewComment string         `gorm:"type:text" json:"review_comment"`
	ExpiresAt     *time.Time     `json:"expires_at,omitempty"`
	CreatedAt     time.Time      `gorm:"not null;default:now()" json:"created_at"`
	ReviewedAt    *time.Time     `json:"reviewed_at,omitempty"`
}

func (AssistantApproval) TableName() string {
	return "assistant_approvals"
}

// AssistantToolSelection 工具选择记录
type AssistantToolSelection struct {
	ID             uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SessionID      string         `gorm:"type:varchar(100);index;not null" json:"session_id"`
	RunID          string         `gorm:"type:varchar(100);index;not null" json:"run_id"`
	MessageID      string         `gorm:"type:varchar(100)" json:"message_id"`
	Stage          string         `gorm:"type:varchar(32);not null;default:'initial'" json:"stage"`
	Query          string         `gorm:"type:text;not null" json:"query"`
	Intent         datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"intent"`
	SelectedTools  datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"selected_tools"`
	CandidateTools datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"candidate_tools"`
	MaxTools       int            `gorm:"not null;default:24" json:"max_tools"`
	CreatedAt      time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (AssistantToolSelection) TableName() string {
	return "assistant_tool_selections"
}

// AssistantToolPolicy 工具策略
type AssistantToolPolicy struct {
	ID                uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	ToolName          string    `gorm:"type:varchar(160);uniqueIndex;not null" json:"tool_name"`
	Domain            string    `gorm:"type:varchar(40);not null" json:"domain"`
	Operation         string    `gorm:"type:varchar(40);not null" json:"operation"`
	RiskLevel         string    `gorm:"type:varchar(20);not null" json:"risk_level"`
	Description       string    `gorm:"type:text" json:"description"`
	ArgsSummary       string    `gorm:"type:text" json:"args_summary"`
	DefaultWhitelisted bool     `gorm:"not null;default:false" json:"default_whitelisted"`
	Whitelisted       bool      `gorm:"not null;default:false" json:"whitelisted"`
	Enabled           bool      `gorm:"not null;default:true" json:"enabled"`
	Source            string    `gorm:"type:varchar(32);not null;default:'builtin'" json:"source"`
	UpdatedBy         string    `gorm:"type:varchar(100)" json:"updated_by"`
	CreatedAt         time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt         time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

func (AssistantToolPolicy) TableName() string {
	return "assistant_tool_policies"
}

// AssistantMemory 智能体记忆
type AssistantMemory struct {
	ID         uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SessionID  string         `gorm:"type:varchar(100);index;not null" json:"session_id"`
	MemoryType string         `gorm:"type:varchar(40);not null" json:"memory_type"`
	Content    string         `gorm:"type:text;not null" json:"content"`
	Metadata   datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	CreatedAt  time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt  time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (AssistantMemory) TableName() string {
	return "assistant_memory"
}

// Session status constants
const (
	SessionStatusActive            = "active"
	SessionStatusRunning           = "running"
	SessionStatusWaitingApproval   = "waiting_approval"
	SessionStatusCompleted         = "completed"
	SessionStatusCancelled         = "cancelled"
	SessionStatusFailed            = "failed"
)

// Task type constants
const (
	TaskTypeInvestigation          = "investigation"
	TaskTypeHostAttackInvestigation = "host_attack_investigation"
	TaskTypeOperations             = "operations"
	TaskTypeGeneration             = "generation"
	TaskTypeRemediation            = "remediation"
	TaskTypeConfiguration          = "configuration"
	TaskTypeExplanation            = "explanation"
)

// Risk level constants
const (
	RiskReadonly = "readonly"
	RiskLow      = "low"
	RiskMedium   = "medium"
	RiskHigh     = "high"
	RiskCritical = "critical"
)

// Approval status constants
const (
	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusRejected = "rejected"
	ApprovalStatusExpired  = "expired"
	ApprovalStatusExecuted = "executed"
	ApprovalStatusFailed   = "failed"
)

// Tool approval mode constants
const (
	ApprovalModeRequestApproval = "request_approval"
	ApprovalModeWhitelist       = "whitelist"
	ApprovalModeFullAccess      = "full_access"
)

// Tool call status constants
const (
	ToolCallStatusRunning         = "running"
	ToolCallStatusSuccess         = "success"
	ToolCallStatusFailed          = "failed"
	ToolCallStatusApprovalRequired = "approval_required"
	ToolCallStatusRejected        = "rejected"
)
