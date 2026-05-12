package model

import (
	"time"

	"github.com/google/uuid"
)

// AgentExecution — 单次agent-runtime执行记录
type AgentExecution struct {
	ID              uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	SessionID       string    `gorm:"type:varchar(100);index;not null"`
	TaskID          string    `gorm:"type:varchar(100);uniqueIndex;not null"`
	Status          string    `gorm:"type:varchar(20)"`  // completed/failed/interrupted/limited
	ExitReason      string    `gorm:"type:varchar(50)"`
	FinalAnswer     string    `gorm:"type:text"`
	InitialPlan     JSONB     `gorm:"type:jsonb"`
	FinalPlan       JSONB     `gorm:"type:jsonb"`
	Completion      JSONB     `gorm:"type:jsonb"`
	Metrics         JSONB     `gorm:"type:jsonb"`
	StartedAt       time.Time
	EndedAt         time.Time
	TotalDurationMs int64
	CreatedAt       time.Time
}

// AgentStepExecution — 步骤执行详情
type AgentStepExecution struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	ExecutionID uuid.UUID `gorm:"type:uuid;index;not null"`
	TaskID      string    `gorm:"type:varchar(100);index"`
	StepID      string    `gorm:"type:varchar(50)"`
	Attempt     int
	Status      string    `gorm:"type:varchar(20)"` // completed/failed/skipped
	Result      string    `gorm:"type:text"`
	Evidence    JSONB     `gorm:"type:jsonb"`
	Error       JSONB     `gorm:"type:jsonb"`
	ReactTurns  JSONB     `gorm:"type:jsonb"`
	StartedAt   time.Time
	EndedAt     time.Time
	DurationMs  int64
	CreatedAt   time.Time
}

// AgentReflection — 反思记录
type AgentReflection struct {
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	ExecutionID    uuid.UUID `gorm:"type:uuid;index;not null"`
	TaskID         string    `gorm:"type:varchar(100);index"`
	StepID         string    `gorm:"type:varchar(50)"`
	ReflectionID   string    `gorm:"type:varchar(100)"`
	Trigger        string    `gorm:"type:varchar(50)"`
	RootCause      string    `gorm:"type:text"`
	Impact         string    `gorm:"type:text"`
	Recoverable    bool
	Recommendation string    `gorm:"type:varchar(50)"`
	DisableTools   JSONB     `gorm:"type:jsonb"`
	CorrectionHint string    `gorm:"type:text"`
	ReusableLesson string    `gorm:"type:text"`
	CreatedAt      time.Time
}

// AgentAudit — 审计记录
type AgentAudit struct {
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	ExecutionID    uuid.UUID `gorm:"type:uuid;index;not null"`
	TaskID         string    `gorm:"type:varchar(100);index"`
	AuditID        string    `gorm:"type:varchar(100)"`
	Trigger        string    `gorm:"type:varchar(50)"`
	Drifted        bool
	RiskLevel      string    `gorm:"type:varchar(20)"`
	Findings       JSONB     `gorm:"type:jsonb"`
	Decision       string    `gorm:"type:varchar(50)"`
	CorrectionHint string    `gorm:"type:text"`
	ShouldExit     bool
	ExitReason     string    `gorm:"type:varchar(50)"`
	CreatedAt      time.Time
}

// AgentCorrection — 计划纠正记录
type AgentCorrection struct {
	ID               uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	ExecutionID      uuid.UUID `gorm:"type:uuid;index;not null"`
	TaskID           string    `gorm:"type:varchar(100);index"`
	CorrectionID     string    `gorm:"type:varchar(100)"`
	Trigger          string    `gorm:"type:varchar(50)"`
	FromPlanVersion  int
	ToPlanVersion    int
	Reason           string    `gorm:"type:text"`
	Actions          JSONB     `gorm:"type:jsonb"`
	Valid            bool
	ValidationErrors JSONB     `gorm:"type:jsonb"`
	CreatedAt        time.Time
}

// AgentToolCallRecord — 工具调用详情
type AgentToolCallRecord struct {
	ID            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	ExecutionID   uuid.UUID `gorm:"type:uuid;index;not null"`
	TaskID        string    `gorm:"type:varchar(100);index"`
	StepID        string    `gorm:"type:varchar(50)"`
	CallID        string    `gorm:"type:varchar(100)"`
	ToolName      string    `gorm:"type:varchar(100)"`
	Reason        string    `gorm:"type:text"`
	ArgsSummary   string    `gorm:"type:text"`
	Status        string    `gorm:"type:varchar(20)"`
	ResultSummary string    `gorm:"type:text"`
	ErrorMessage  string    `gorm:"type:text"`
	RiskLevel     string    `gorm:"type:varchar(20)"`
	DurationMs    int64
	StartedAt     time.Time
	EndedAt       time.Time
	CreatedAt     time.Time
}

// AgentModelError — 模型调用错误
type AgentModelError struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	ExecutionID uuid.UUID `gorm:"type:uuid;index;not null"`
	TaskID      string    `gorm:"type:varchar(100);index"`
	StepID      string    `gorm:"type:varchar(50)"`
	CallID      string    `gorm:"type:varchar(100)"`
	Purpose     string    `gorm:"type:varchar(20)"`
	ErrorKind   string    `gorm:"type:varchar(50)"`
	Message     string    `gorm:"type:text"`
	Recoverable bool
	Model       string    `gorm:"type:varchar(100)"`
	TokensUsed  int
	LatencyMs   int64
	OccurredAt  time.Time
	CreatedAt   time.Time
}
