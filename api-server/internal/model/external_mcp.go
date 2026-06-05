package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// ExternalMCPSource 外接 MCP 数据源
type ExternalMCPSource struct {
	ID                 uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SourceID           string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"source_id"`
	Name               string         `gorm:"type:varchar(120);not null" json:"name"`
	SourceType         string         `gorm:"type:varchar(40);not null" json:"source_type"`
	Transport          string         `gorm:"type:varchar(40);not null;default:'streamable_http'" json:"transport"`
	EndpointURL        string         `gorm:"type:text;not null" json:"endpoint_url"`
	AuthType           string         `gorm:"type:varchar(40);not null;default:'none'" json:"auth_type"`
	CredentialRef      string         `gorm:"type:varchar(255)" json:"credential_ref,omitempty"`
	Enabled            bool           `gorm:"not null;default:true" json:"enabled"`
	Description        string         `gorm:"type:text" json:"description"`
	AllowedToolNames   datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"allowed_tool_names"`
	SchemaCache        datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"schema_cache"`
	QueryLimits        datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"query_limits"`
	DataClassification string         `gorm:"type:varchar(40);not null;default:'internal'" json:"data_classification"`
	LastTestStatus     string         `gorm:"type:varchar(40)" json:"last_test_status"`
	LastTestError      string         `gorm:"type:text" json:"last_test_error,omitempty"`
	LastTestAt         *time.Time     `json:"last_test_at,omitempty"`
	CreatedBy          string         `gorm:"type:varchar(100)" json:"created_by"`
	UpdatedBy          string         `gorm:"type:varchar(100)" json:"updated_by"`
	CreatedAt          time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt          time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (ExternalMCPSource) TableName() string {
	return "external_mcp_sources"
}

// ExternalMCPQueryLog 外接 MCP 查询日志
type ExternalMCPQueryLog struct {
	ID             uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	QueryID        string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"query_id"`
	SessionID      string         `gorm:"type:varchar(100)" json:"session_id"`
	RunID          string         `gorm:"type:varchar(100)" json:"run_id"`
	ToolCallID     string         `gorm:"type:varchar(100)" json:"tool_call_id"`
	SourceID       string         `gorm:"type:varchar(100);not null" json:"source_id"`
	SourceName     string         `gorm:"type:varchar(120)" json:"source_name"`
	QueryGoal      string         `gorm:"type:text;not null" json:"query_goal"`
	RequestSummary string         `gorm:"type:text" json:"request_summary"`
	RedactedRequest datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"redacted_request"`
	ResultCount    int            `gorm:"not null;default:0" json:"result_count"`
	ResultDigest   string         `gorm:"type:text" json:"result_digest"`
	Status         string         `gorm:"type:varchar(40);not null" json:"status"`
	ErrorMessage   string         `gorm:"type:text" json:"error_message,omitempty"`
	DurationMs     int            `gorm:"not null;default:0" json:"duration_ms"`
	CreatedBy      string         `gorm:"type:varchar(100)" json:"created_by"`
	CreatedAt      time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (ExternalMCPQueryLog) TableName() string {
	return "external_mcp_query_logs"
}

// MCP source type constants
const (
	MCPSourceTypeSIEM         = "siem"
	MCPSourceTypeCMDB         = "cmdb"
	MCPSourceTypeEDR          = "edr"
	MCPSourceTypeTicket       = "ticket"
	MCPSourceTypeThreatIntel  = "threat_intel"
	MCPSourceTypeLogWarehouse = "log_warehouse"
	MCPSourceTypeCustom       = "custom"
)

// MCP transport constants
const (
	MCPTransportStreamableHTTP = "streamable_http"
	MCPTransportSSE            = "sse"
	MCPTransportStdio          = "stdio"
)

// MCP auth type constants
const (
	MCPAuthTypeNone   = "none"
	MCPAuthTypeAPIKey = "api_key"
	MCPAuthTypeBearer = "bearer"
	MCPAuthTypeBasic  = "basic"
	MCPAuthTypeOAuth2 = "oauth2"
)

// MCP query status constants
const (
	MCPQueryStatusSuccess = "success"
	MCPQueryStatusFailed  = "failed"
	MCPQueryStatusTimeout = "timeout"
)

// MCPConnectionTestResult MCP 连接测试结果
type MCPConnectionTestResult struct {
	SourceID  string `json:"source_id"`
	Success   bool   `json:"success"`
	LatencyMs int    `json:"latency_ms"`
	ToolCount int    `json:"tool_count"`
	Message   string `json:"message"`
}

// MCPSchemaSyncResult MCP Schema 同步结果
type MCPSchemaSyncResult struct {
	SourceID      string   `json:"source_id"`
	SchemaVersion string   `json:"schema_version"`
	ToolCount     int      `json:"tool_count"`
	Fields        []string `json:"fields"`
}
