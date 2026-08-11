package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const (
	MCPPlatformServerDiscoveredCandidate = "discovered_candidate"
	MCPPlatformServerDraft               = "draft"
	MCPPlatformServerValidating          = "validating"
	MCPPlatformServerReviewRequired      = "review_required"
	MCPPlatformServerApproved            = "approved"
	MCPPlatformServerPublished           = "published"
	MCPPlatformServerSuspended           = "suspended"
	MCPPlatformServerRetired             = "retired"
	MCPPlatformServerDriftDetected       = "drift_detected"
	MCPPlatformServerQuarantined         = "quarantined"

	MCPPlatformJobCreated            = "created"
	MCPPlatformJobValidatingEndpoint = "validating_endpoint"
	MCPPlatformJobAwaitingAuth       = "awaiting_auth"
	MCPPlatformJobAuthenticating     = "authenticating"
	MCPPlatformJobDiscovering        = "discovering"
	MCPPlatformJobValidatingTools    = "validating_tools"
	MCPPlatformJobSecurityScanning   = "security_scanning"
	MCPPlatformJobClassifying        = "classifying"
	MCPPlatformJobBuildingRelease    = "building_release"
	MCPPlatformJobAwaitingApproval   = "awaiting_approval"
	MCPPlatformJobPublishing         = "publishing"
	MCPPlatformJobActive             = "active"
	MCPPlatformJobFailed             = "failed"
	MCPPlatformJobCancelled          = "cancelled"

	MCPPlatformTransportStreamableHTTP = "streamable_http"
	MCPPlatformTransportLegacySSE      = "legacy_sse"
	MCPPlatformAuthNone                = "none"
	MCPPlatformAuthOAuth2              = "oauth2"
	MCPPlatformAuthBearer              = "bearer"
	MCPPlatformAuthAPIKey              = "api_key"

	MCPPlatformRiskL1 = "l1"
	MCPPlatformRiskL2 = "l2"
	MCPPlatformRiskL3 = "l3"
	MCPPlatformRiskL4 = "l4"

	MCPPlatformApprovalAdmission = "admission"
	MCPPlatformApprovalRelease   = "release"
	MCPPlatformApprovalRuntime   = "runtime"
	MCPPlatformApprovalPending   = "pending"
	MCPPlatformApprovalApproved  = "approved"
	MCPPlatformApprovalRejected  = "rejected"
	MCPPlatformApprovalCancelled = "cancelled"
	MCPPlatformApprovalExpired   = "expired"
)

// MCPServer is the logical identity of a remote MCP server. The endpoint is
// never returned in API JSON; EndpointURL is retained only for the upstream
// worker and must be protected by the database/secret manager in production.
type MCPServer struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ServerKey          string     `gorm:"type:varchar(100);not null;uniqueIndex" json:"server_key"`
	DisplayName        string     `gorm:"type:varchar(160);not null" json:"display_name"`
	OwnerTeamID        *uuid.UUID `gorm:"type:uuid;index" json:"owner_team_id,omitempty"`
	OwnerUserID        string     `gorm:"type:varchar(100);not null;index" json:"owner_user_id"`
	Environment        string     `gorm:"type:varchar(32);not null;index" json:"environment"`
	Transport          string     `gorm:"type:varchar(32);not null" json:"transport"`
	EndpointURL        string     `gorm:"type:text;not null" json:"-"`
	EndpointDisplay    string     `gorm:"type:varchar(255);not null" json:"endpoint_display"`
	CredentialRef      string     `gorm:"type:varchar(255)" json:"-"`
	AuthType           string     `gorm:"type:varchar(32);not null" json:"auth_type"`
	ProtocolVersion    string     `gorm:"type:varchar(64)" json:"protocol_version,omitempty"`
	RiskTier           string     `gorm:"type:varchar(8);not null;default:l2;index" json:"risk_tier"`
	LifecycleStatus    string     `gorm:"type:varchar(40);not null;index" json:"lifecycle_status"`
	ActiveRevisionID   *uuid.UUID `gorm:"type:uuid" json:"active_revision_id,omitempty"`
	ToolCount          int        `gorm:"not null;default:0" json:"tool_count"`
	PublishedToolCount int        `gorm:"not null;default:0" json:"published_tool_count"`
	LastHealthStatus   string     `gorm:"type:varchar(32)" json:"last_health_status,omitempty"`
	LastErrorCode      string     `gorm:"type:varchar(100)" json:"last_error_code,omitempty"`
	LastErrorMessage   string     `gorm:"type:text" json:"last_error_message,omitempty"`
	LastSyncedAt       *time.Time `json:"last_synced_at,omitempty"`
	CreatedBy          string     `gorm:"type:varchar(100);not null" json:"created_by"`
	UpdatedBy          string     `gorm:"type:varchar(100);not null" json:"updated_by"`
	CreatedAt          time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt          time.Time  `gorm:"not null;default:now()" json:"updated_at"`
}

func (MCPServer) TableName() string { return "mcp_servers" }

type MCPOnboardingJob struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	IdempotencyKey  string     `gorm:"type:varchar(128);not null;uniqueIndex" json:"-"`
	ServerID        *uuid.UUID `gorm:"type:uuid;index" json:"server_id,omitempty"`
	DisplayName     string     `gorm:"type:varchar(160);not null" json:"display_name"`
	EndpointURL     string     `gorm:"type:text;not null" json:"-"`
	EndpointDisplay string     `gorm:"type:varchar(255);not null" json:"endpoint_display"`
	CredentialRef   string     `gorm:"type:varchar(255)" json:"-"`
	AuthType        string     `gorm:"type:varchar(32);not null" json:"auth_type"`
	OwnerTeamID     *uuid.UUID `gorm:"type:uuid" json:"owner_team_id,omitempty"`
	OwnerUserID     string     `gorm:"type:varchar(100);not null;index" json:"owner_user_id"`
	Environment     string     `gorm:"type:varchar(32);not null" json:"environment"`
	TargetCatalogID *uuid.UUID `gorm:"type:uuid" json:"target_catalog_id,omitempty"`
	PublishPolicy   string     `gorm:"type:varchar(32);not null" json:"publish_policy"`
	Status          string     `gorm:"type:varchar(40);not null;index" json:"status"`
	Step            string     `gorm:"type:varchar(40);not null" json:"step"`
	Attempt         int        `gorm:"not null;default:0" json:"attempt"`
	ErrorCode       string     `gorm:"type:varchar(100)" json:"error_code,omitempty"`
	ErrorMessage    string     `gorm:"type:text" json:"error_message,omitempty"`
	RevisionID      *uuid.UUID `gorm:"type:uuid" json:"revision_id,omitempty"`
	CreatedBy       string     `gorm:"type:varchar(100);not null" json:"created_by"`
	CreatedAt       time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"not null;default:now()" json:"updated_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

func (MCPOnboardingJob) TableName() string { return "mcp_onboarding_jobs" }

type MCPServerRevision struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ServerID        uuid.UUID      `gorm:"type:uuid;not null;index" json:"server_id"`
	RevisionNo      int64          `gorm:"not null" json:"revision_no"`
	ProtocolVersion string         `gorm:"type:varchar(64)" json:"protocol_version,omitempty"`
	Capabilities    datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"capabilities"`
	ToolsSnapshot   datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"tools_snapshot"`
	Digest          string         `gorm:"type:varchar(80);not null" json:"digest"`
	Status          string         `gorm:"type:varchar(40);not null;index" json:"status"`
	DiscoveryError  string         `gorm:"type:text" json:"discovery_error,omitempty"`
	CreatedBy       string         `gorm:"type:varchar(100);not null" json:"created_by"`
	CreatedAt       time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (MCPServerRevision) TableName() string { return "mcp_server_revisions" }

type MCPToolRevision struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ServerRevisionID uuid.UUID      `gorm:"type:uuid;not null;index" json:"server_revision_id"`
	UpstreamName     string         `gorm:"type:varchar(255);not null" json:"upstream_name"`
	Alias            string         `gorm:"type:varchar(255);not null;index" json:"alias"`
	Title            string         `gorm:"type:varchar(255)" json:"title,omitempty"`
	Description      string         `gorm:"type:text" json:"description,omitempty"`
	InputSchema      datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"input_schema"`
	OutputSchema     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"output_schema"`
	VerifiedMetadata datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"verified_metadata"`
	RiskTier         string         `gorm:"type:varchar(8);not null;default:l2;index" json:"risk_tier"`
	Status           string         `gorm:"type:varchar(32);not null;default:discovered;index" json:"status"`
	CreatedAt        time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (MCPToolRevision) TableName() string { return "mcp_tool_revisions" }

type MCPAdmissionReview struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SubjectType   string         `gorm:"type:varchar(32);not null" json:"subject_type"`
	SubjectID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"subject_id"`
	SubjectDigest string         `gorm:"type:varchar(80);not null" json:"subject_digest"`
	RequiredRoles datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"required_roles"`
	Quorum        int            `gorm:"not null;default:1" json:"quorum"`
	Decision      string         `gorm:"type:varchar(32);not null;default:pending" json:"decision"`
	ReviewerID    string         `gorm:"type:varchar(100)" json:"reviewer_id,omitempty"`
	Reason        string         `gorm:"type:text" json:"reason,omitempty"`
	EvidenceRefs  datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"evidence_refs"`
	ExpiresAt     *time.Time     `json:"expires_at,omitempty"`
	CreatedAt     time.Time      `gorm:"not null;default:now()" json:"created_at"`
	DecidedAt     *time.Time     `json:"decided_at,omitempty"`
}

func (MCPAdmissionReview) TableName() string { return "mcp_admission_reviews" }

type MCPCatalog struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CatalogKey  string    `gorm:"type:varchar(100);not null;uniqueIndex" json:"catalog_key"`
	DisplayName string    `gorm:"type:varchar(160);not null" json:"display_name"`
	Status      string    `gorm:"type:varchar(32);not null;default:active" json:"status"`
	CreatedBy   string    `gorm:"type:varchar(100);not null" json:"created_by"`
	CreatedAt   time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

func (MCPCatalog) TableName() string { return "mcp_catalogs" }

type MCPCatalogRelease struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CatalogID        uuid.UUID      `gorm:"type:uuid;not null;index" json:"catalog_id"`
	ServerRevisionID *uuid.UUID     `gorm:"type:uuid;index" json:"server_revision_id,omitempty"`
	ReleaseNo        int64          `gorm:"not null" json:"release_no"`
	Manifest         datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"manifest"`
	ManifestDigest   string         `gorm:"type:varchar(80);not null" json:"manifest_digest"`
	Status           string         `gorm:"type:varchar(32);not null;index" json:"status"`
	CreatedBy        string         `gorm:"type:varchar(100);not null" json:"created_by"`
	CreatedAt        time.Time      `gorm:"not null;default:now()" json:"created_at"`
	PublishedAt      *time.Time     `json:"published_at,omitempty"`
}

func (MCPCatalogRelease) TableName() string { return "mcp_catalog_releases" }

// MCPCatalogReleaseTool is the immutable mapping exposed by a release. The
// Gateway must resolve aliases only through this table; it must never accept a
// client-supplied upstream name or endpoint.
type MCPCatalogReleaseTool struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ReleaseID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"release_id"`
	ToolRevisionID uuid.UUID      `gorm:"type:uuid;not null;index" json:"tool_revision_id"`
	ExposedName    string         `gorm:"type:varchar(255);not null" json:"exposed_name"`
	Title          string         `gorm:"type:varchar(255)" json:"title,omitempty"`
	Description    string         `gorm:"type:text" json:"description,omitempty"`
	InputSchema    datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"input_schema"`
	OutputSchema   datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"output_schema"`
	ApprovalMode   string         `gorm:"type:varchar(32);not null;default:none" json:"approval_mode"`
	RateLimit      datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"rate_limit"`
	Resource       datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"resource"`
	Status         string         `gorm:"type:varchar(32);not null;default:staged" json:"status"`
	DisplayOrder   int            `gorm:"not null;default:0" json:"display_order"`
	CreatedAt      time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (MCPCatalogReleaseTool) TableName() string { return "mcp_catalog_release_tools" }

type MCPClient struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ClientKey   string    `gorm:"type:varchar(100);not null;uniqueIndex" json:"client_key"`
	DisplayName string    `gorm:"type:varchar(160);not null" json:"display_name"`
	ClientType  string    `gorm:"type:varchar(32);not null" json:"client_type"`
	Status      string    `gorm:"type:varchar(32);not null;index" json:"status"`
	CreatedBy   string    `gorm:"type:varchar(100);not null" json:"created_by"`
	CreatedAt   time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

func (MCPClient) TableName() string { return "mcp_clients" }

type MCPClientGrant struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ClientID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"client_id"`
	CatalogID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"catalog_id"`
	ToolAllowlist datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"tool_allowlist"`
	ResourceScope datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"resource_scope"`
	Status        string         `gorm:"type:varchar(32);not null;index" json:"status"`
	ExpiresAt     *time.Time     `json:"expires_at,omitempty"`
	CreatedBy     string         `gorm:"type:varchar(100);not null" json:"created_by"`
	CreatedAt     time.Time      `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (MCPClientGrant) TableName() string { return "mcp_client_grants" }

// MCPClientCredential stores only a digest of the one-time credential shown
// to an AI Agent. The raw token must never be persisted or returned again.
type MCPClientCredential struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ClientID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"client_id"`
	TokenPrefix string     `gorm:"type:varchar(32);not null;uniqueIndex" json:"token_prefix"`
	TokenHash   string     `gorm:"type:varchar(64);not null;uniqueIndex" json:"-"`
	Status      string     `gorm:"type:varchar(32);not null;index" json:"status"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedBy   string     `gorm:"type:varchar(100);not null" json:"created_by"`
	CreatedAt   time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"not null;default:now()" json:"updated_at"`
}

func (MCPClientCredential) TableName() string { return "mcp_client_credentials" }

type MCPPolicySet struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PolicyKey   string    `gorm:"type:varchar(128);not null;uniqueIndex" json:"policy_key"`
	DisplayName string    `gorm:"type:varchar(160);not null" json:"display_name"`
	Status      string    `gorm:"type:varchar(32);not null;default:draft" json:"status"`
	CreatedBy   string    `gorm:"type:varchar(100);not null" json:"created_by"`
	CreatedAt   time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

func (MCPPolicySet) TableName() string { return "mcp_policy_sets" }

type MCPPolicyVersion struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PolicySetID       uuid.UUID      `gorm:"type:uuid;not null;index" json:"policy_set_id"`
	Version           int64          `gorm:"not null" json:"version"`
	LanguageVersion   string         `gorm:"type:varchar(32);not null" json:"language_version"`
	Source            datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"source"`
	CompiledBundleRef string         `gorm:"type:varchar(512)" json:"compiled_bundle_ref,omitempty"`
	Digest            string         `gorm:"type:varchar(80);not null" json:"digest"`
	TestReport        datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"test_report"`
	Status            string         `gorm:"type:varchar(32);not null;default:draft" json:"status"`
	Signature         string         `gorm:"type:text" json:"signature,omitempty"`
	SigningKeyID      string         `gorm:"type:varchar(128)" json:"signing_key_id,omitempty"`
	CreatedBy         string         `gorm:"type:varchar(100);not null" json:"created_by"`
	CreatedAt         time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (MCPPolicyVersion) TableName() string { return "mcp_policy_versions" }

type MCPApprovalRequest struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ApprovalType   string     `gorm:"type:varchar(32);not null;index" json:"approval_type"`
	SubjectType    string     `gorm:"type:varchar(32);not null" json:"subject_type"`
	SubjectID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"subject_id"`
	RequestedBy    string     `gorm:"type:varchar(100);not null" json:"requested_by"`
	Status         string     `gorm:"type:varchar(32);not null;index" json:"status"`
	RequestDigest  string     `gorm:"type:varchar(80)" json:"request_digest,omitempty"`
	Reason         string     `gorm:"type:text" json:"reason,omitempty"`
	DecisionReason string     `gorm:"type:text" json:"decision_reason,omitempty"`
	DecidedBy      string     `gorm:"type:varchar(100)" json:"decided_by,omitempty"`
	CreatedAt      time.Time  `gorm:"not null;default:now()" json:"created_at"`
	DecidedAt      *time.Time `json:"decided_at,omitempty"`
}

func (MCPApprovalRequest) TableName() string { return "mcp_approval_requests" }

type MCPInvocation struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ClientID         *uuid.UUID `gorm:"type:uuid;index" json:"client_id,omitempty"`
	CatalogReleaseID *uuid.UUID `gorm:"type:uuid;index" json:"catalog_release_id,omitempty"`
	ToolRevisionID   *uuid.UUID `gorm:"type:uuid;index" json:"tool_revision_id,omitempty"`
	UserID           string     `gorm:"type:varchar(100);index" json:"user_id,omitempty"`
	ToolAlias        string     `gorm:"type:varchar(255);not null" json:"tool_alias"`
	Status           string     `gorm:"type:varchar(32);not null;index" json:"status"`
	PolicyDecision   string     `gorm:"type:varchar(32)" json:"policy_decision,omitempty"`
	RuleStatus       string     `gorm:"type:varchar(32)" json:"rule_status,omitempty"`
	AIStatus         string     `gorm:"type:varchar(32)" json:"ai_status,omitempty"`
	RequestDigest    string     `gorm:"type:varchar(80)" json:"request_digest,omitempty"`
	ResultDigest     string     `gorm:"type:varchar(80)" json:"result_digest,omitempty"`
	CreatedAt        time.Time  `gorm:"not null;default:now();index" json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

func (MCPInvocation) TableName() string { return "mcp_invocations" }

type MCPSecurityVerdict struct {
	ID                    uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	InvocationID          uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex" json:"invocation_id"`
	DeterministicSeverity string         `gorm:"type:varchar(16);not null" json:"deterministic_severity"`
	AIVerdict             string         `gorm:"type:varchar(32)" json:"ai_verdict,omitempty"`
	OverallRisk           string         `gorm:"type:varchar(16);not null" json:"overall_risk"`
	Evidence              datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"evidence"`
	UpdatedAt             time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

func (MCPSecurityVerdict) TableName() string { return "mcp_security_verdicts" }

type MCPInvocationEvent struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	InvocationID uuid.UUID      `gorm:"type:uuid;not null;index" json:"invocation_id"`
	EventSeq     int64          `gorm:"not null" json:"event_seq"`
	EventType    string         `gorm:"type:varchar(48);not null" json:"event_type"`
	Status       string         `gorm:"type:varchar(32);not null" json:"status"`
	TraceID      string         `gorm:"type:varchar(128)" json:"trace_id,omitempty"`
	Digest       string         `gorm:"type:varchar(80)" json:"digest,omitempty"`
	Metadata     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	CreatedAt    time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (MCPInvocationEvent) TableName() string { return "mcp_invocation_events" }

type MCPInvocationPayloadRef struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	InvocationID   uuid.UUID `gorm:"type:uuid;not null;index" json:"invocation_id"`
	Stage          string    `gorm:"type:varchar(48);not null" json:"stage"`
	ObjectRef      string    `gorm:"type:varchar(512);not null" json:"object_ref"`
	Digest         string    `gorm:"type:varchar(80);not null" json:"digest"`
	SizeBytes      int64     `gorm:"not null;default:0" json:"size_bytes"`
	Classification string    `gorm:"type:varchar(32)" json:"classification,omitempty"`
	Status         string    `gorm:"type:varchar(32);not null" json:"status"`
	CreatedAt      time.Time `gorm:"not null;default:now()" json:"created_at"`
}

func (MCPInvocationPayloadRef) TableName() string { return "mcp_invocation_payload_refs" }

type MCPAuditOutbox struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	EventID       uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex" json:"event_id"`
	Topic         string         `gorm:"type:varchar(255);not null" json:"topic"`
	Payload       datatypes.JSON `gorm:"type:jsonb;not null" json:"payload"`
	Status        string         `gorm:"type:varchar(32);not null;default:pending" json:"status"`
	Attempt       int            `gorm:"not null;default:0" json:"attempt"`
	NextAttemptAt *time.Time     `json:"next_attempt_at,omitempty"`
	CreatedAt     time.Time      `gorm:"not null;default:now()" json:"created_at"`
	PublishedAt   *time.Time     `json:"published_at,omitempty"`
}

func (MCPAuditOutbox) TableName() string { return "mcp_audit_outbox" }

type MCPRuleDefinition struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RuleKey    string         `gorm:"type:varchar(128);not null" json:"rule_key"`
	Version    int64          `gorm:"not null" json:"version"`
	Name       string         `gorm:"type:varchar(255);not null" json:"name"`
	Phase      string         `gorm:"type:varchar(16);not null" json:"phase"`
	Severity   string         `gorm:"type:varchar(16);not null" json:"severity"`
	Definition datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"definition"`
	Digest     string         `gorm:"type:varchar(80);not null" json:"digest"`
	Enabled    bool           `gorm:"not null;default:true" json:"enabled"`
	CreatedAt  time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (MCPRuleDefinition) TableName() string { return "mcp_rule_definitions" }

type MCPRuleHit struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	InvocationID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"invocation_id"`
	RuleDefinitionID uuid.UUID      `gorm:"type:uuid;not null;index" json:"rule_definition_id"`
	Severity         string         `gorm:"type:varchar(16);not null" json:"severity"`
	Phase            string         `gorm:"type:varchar(16);not null" json:"phase"`
	Evidence         datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"evidence"`
	CreatedAt        time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

func (MCPRuleHit) TableName() string { return "mcp_rule_hits" }

type MCPAIAnalysisRun struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	InvocationID uuid.UUID  `gorm:"type:uuid;not null;index" json:"invocation_id"`
	ActivityID   *uuid.UUID `gorm:"type:uuid" json:"activity_id,omitempty"`
	Status       string     `gorm:"type:varchar(32);not null" json:"status"`
	Model        string     `gorm:"type:varchar(160)" json:"model,omitempty"`
	Verdict      string     `gorm:"type:varchar(32)" json:"verdict,omitempty"`
	ErrorCode    string     `gorm:"type:varchar(100)" json:"error_code,omitempty"`
	Attempts     int        `gorm:"not null;default:0" json:"attempts"`
	LeaseUntil   *time.Time `json:"lease_until,omitempty"`
	CreatedAt    time.Time  `gorm:"not null;default:now()" json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

func (MCPAIAnalysisRun) TableName() string { return "mcp_ai_analysis_runs" }

type MCPAIAnalysisChunk struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RunID        uuid.UUID      `gorm:"type:uuid;not null;index" json:"run_id"`
	ChunkNo      int            `gorm:"not null" json:"chunk_no"`
	InputDigest  string         `gorm:"type:varchar(80);not null" json:"input_digest"`
	Status       string         `gorm:"type:varchar(32);not null" json:"status"`
	Output       datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"output"`
	InputTokens  *int           `json:"input_tokens,omitempty"`
	OutputTokens *int           `json:"output_tokens,omitempty"`
	CreatedAt    time.Time      `gorm:"not null;default:now()" json:"created_at"`
	CompletedAt  *time.Time     `json:"completed_at,omitempty"`
}

func (MCPAIAnalysisChunk) TableName() string { return "mcp_ai_analysis_chunks" }
