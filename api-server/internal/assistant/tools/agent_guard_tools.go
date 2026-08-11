package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/service"
	"github.com/google/uuid"
)

// AgentGuardQueryForTools is the narrow, read-oriented facade consumed by
// Assistant. Keeping this interface separate from the HTTP handler prevents
// policy JSON, controller internals and Gin request concerns from leaking into
// model-facing tools.
type AgentGuardQueryForTools interface {
	GetOverview(context.Context) (*model.AgentGuardOverview, error)
	ListAgents(context.Context, model.AgentGuardAgentQuery) ([]model.AgentGuardAgentSummary, int64, error)
	ListInstances(context.Context, model.AgentRuntimeInstanceQuery) ([]model.AgentRuntimeInstance, int64, error)
	ListSessions(context.Context, model.AgentBehaviorSessionQuery) ([]model.AgentBehaviorSession, int64, error)
	ListExecutionUnits(context.Context, model.AgentExecutionUnitQuery) ([]model.AgentExecutionUnit, int64, error)
	ListBehaviors(context.Context, model.AgentBehaviorEventQuery) ([]model.AgentBehaviorEvent, int64, error)
	GetInstance(context.Context, uuid.UUID) (*model.AgentRuntimeInstance, error)
	GetSession(context.Context, uuid.UUID) (*model.AgentBehaviorSession, error)
	GetExecutionUnit(context.Context, uuid.UUID) (*model.AgentExecutionUnit, error)
	GetBehavior(context.Context, string) (*model.AgentBehaviorEvent, error)
	GetRuntimeEvent(context.Context, string) (*model.RuntimeEvent, error)
	GetFinding(context.Context, uuid.UUID) (*model.AgentSecurityFinding, error)
	GetAnalysis(context.Context, uuid.UUID) (*model.AgentSecurityAnalysisRun, error)
	GetAction(context.Context, uuid.UUID) (*model.AgentGuardAction, error)
	ListFindings(context.Context, model.AgentSecurityFindingQuery) ([]model.AgentSecurityFinding, int64, error)
	ListActions(context.Context, model.AgentGuardActionQuery) ([]model.AgentGuardAction, int64, error)
	DeleteSessions(context.Context, []uuid.UUID) error
}

type AgentGuardCatalogForTools interface {
	ListProfiles(context.Context, model.AgentGuardProfileQuery) ([]model.AgentGuardAdapterProfile, int64, error)
	ListRules(context.Context, model.AgentBehaviorRuleQuery) ([]model.AgentBehaviorRuleDefinition, int64, error)
}

type AgentGuardAnalysisForTools interface {
	Request(context.Context, uuid.UUID, string) (*model.AgentSecurityAnalysisRun, error)
}

type AgentGuardActionsForTools interface {
	RequestExecutionUnit(context.Context, uuid.UUID, string, service.AgentGuardManualActionRequest, string) (*model.AgentGuardAction, error)
	RequestInstanceKill(context.Context, uuid.UUID, service.AgentGuardManualActionRequest, string) (*model.AgentGuardAction, error)
}

type AgentGuardRuntimeSettingsForTools interface {
	Get(context.Context, uuid.UUID) (*model.AgentGuardRuntimeSettings, error)
	Update(context.Context, model.AgentGuardRuntimeSettings, string) (*model.AgentGuardRuntimeSettings, error)
}

type AgentGuardConfigScannerForTools interface {
	Scan(context.Context, string) (*service.AgentConfigScanResult, error)
}

type AgentConversationForTools interface {
	List(context.Context, *uuid.UUID, string, string, int, int) (service.AgentSessionListResult, error)
	Detail(context.Context, uuid.UUID, bool) (*model.AgentConversationSession, []model.AgentConversationItem, error)
	RequestCollection(context.Context, uuid.UUID, string) (service.AgentSessionCollectionResult, error)
	Analyze(context.Context, uuid.UUID) (service.AgentSessionAIResult, error)
	GetAIAnalysis(context.Context, uuid.UUID) (service.AgentSessionAIResult, error)
}

// agentConversationQueryResult deliberately puts pagination metadata before
// items. Tool results can be context-truncated by the model runtime; placing
// total/page information first ensures the model can still distinguish a
// complete page from a partial page and request the next page deterministically.
type agentConversationQueryResult struct {
	Page          int                               `json:"page"`
	PageSize      int                               `json:"page_size"`
	ReturnedCount int                               `json:"returned_count"`
	Total         int64                             `json:"total"`
	TotalPages    int                               `json:"total_pages"`
	HasNextPage   bool                              `json:"has_next_page"`
	HasPrevious   bool                              `json:"has_previous_page"`
	RiskSummary   agentConversationRiskSummary      `json:"risk_summary"`
	Items         []agentConversationSessionSummary `json:"items"`
}

// agentConversationRiskSummary keeps the exact high-risk objects ahead of the
// full item list. The runtime may truncate long tool results, so final
// conclusions must not lose the affected session IDs and their evidence.
// Risk category names are deliberately not inferred from aggregate counters.
type agentConversationRiskSummary struct {
	HighRiskCount       int                           `json:"high_risk_count"`
	ActiveHighRiskCount int                           `json:"active_high_risk_count"`
	UnknownRiskCount    int                           `json:"unknown_risk_count"`
	RiskTypesAvailable  bool                          `json:"risk_types_available"`
	HighRiskSessions    []agentConversationRiskDigest `json:"high_risk_sessions"`
}

type agentConversationRiskDigest struct {
	ID           uuid.UUID `json:"id"`
	State        string    `json:"state"`
	RuleHitCount int64     `json:"rule_hit_count"`
	ItemCount    int64     `json:"item_count"`
}

// agentConversationSessionSummary is intentionally smaller than the storage
// model. Query results should identify and rank sessions, not carry unrelated
// token counters, timestamps or persistence fields into the model context.
type agentConversationSessionSummary struct {
	ID           uuid.UUID  `json:"id"`
	HostID       uuid.UUID  `json:"host_id"`
	AgentType    string     `json:"agent_type"`
	ExternalID   string     `json:"external_session_id"`
	Title        string     `json:"title,omitempty"`
	Model        string     `json:"model,omitempty"`
	State        string     `json:"state"`
	RiskLevel    string     `json:"risk_level"`
	RuleHitCount int64      `json:"rule_hit_count"`
	ItemCount    int64      `json:"item_count"`
	FirstSeenAt  *time.Time `json:"first_seen_at,omitempty"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
}

type AgentGuardToolDeps struct {
	Query         AgentGuardQueryForTools
	Catalog       AgentGuardCatalogForTools
	Analysis      AgentGuardAnalysisForTools
	Actions       AgentGuardActionsForTools
	Runtime       AgentGuardRuntimeSettingsForTools
	ConfigScanner AgentGuardConfigScannerForTools
	Conversations AgentConversationForTools
}

// RegisterAgentGuardTools exposes semantic posture/evidence operations. The
// policy compiler, policy repository and raw configuration fields are not
// registered here: they remain behind the admin APIs and deterministic
// service-side decision paths.
func RegisterAgentGuardTools(registry *assistant.ToolRegistry, deps AgentGuardToolDeps) error {
	if registry == nil {
		return fmt.Errorf("assistant tool registry is not configured")
	}
	if err := registry.Register(&assistant.ToolSpec{
		Name: "AgentGuard.Posture.Assess", Domain: assistant.DomainAgentGuard, Operation: assistant.OpGet,
		Capability:       "assess_agent_guard_posture",
		Description:      "Assess Agent Guard posture with coverage, runtime counts, recent finding counts, and bounded agent summaries.",
		ModelDescription: "Use this first when the user asks whether smart-agent protection is healthy or where coverage is degraded.",
		Tags:             []string{"v6.3", "agent_guard", "posture", "high_level"}, ObjectTypes: []string{"agent_guard_posture"},
		Risk: assistant.ToolRiskReadonly, AutoCallable: true, Idempotent: true, DefaultWhitelisted: true, Enabled: true,
		ExposurePolicy: primaryExposure(220), DefaultTimeout: 15 * time.Second,
		ArgsSchema: emptyObjectSchema(), Handler: func(ctx context.Context, _ map[string]interface{}) (interface{}, error) {
			if deps.Query == nil {
				return nil, fmt.Errorf("agent guard query service is not configured")
			}
			overview, err := deps.Query.GetOverview(ctx)
			if err != nil {
				return nil, err
			}
			agents, total, err := deps.Query.ListAgents(ctx, model.AgentGuardAgentQuery{AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 50}})
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"overview": overview, "agents": agents, "agents_total": total, "policy_details_hidden": true}, nil
		},
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name: "AgentGuard.Scope.Investigate", Domain: assistant.DomainAgentGuard, Operation: assistant.OpGet,
		Capability:       "investigate_agent_guard_scope",
		Description:      "Investigate one exact Agent Guard instance, session, execution unit, behavior event, finding, analysis, or action by stable identifier.",
		ModelDescription: "Use an exact identifier returned by Agent Guard posture or another evidence tool; the server resolves ownership and returns a bounded semantic record.",
		Tags:             []string{"v6.3", "agent_guard", "evidence", "scope"}, ObjectTypes: []string{"agent_guard_scope"},
		Risk: assistant.ToolRiskReadonly, AutoCallable: true, Idempotent: true, DefaultWhitelisted: true, Enabled: true,
		ExposurePolicy: primaryExposure(210), DefaultTimeout: 15 * time.Second,
		ArgsSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{
			"scope_type": map[string]interface{}{"type": "string", "enum": []interface{}{"instance", "session", "execution_unit", "behavior", "runtime_event", "finding", "analysis", "action"}},
			"scope_id":   map[string]interface{}{"type": "string", "description": "Exact UUID, raw event ID, or runtime event ID returned by Aegis."},
		}, "required": []string{"scope_type", "scope_id"}, "additionalProperties": false},
		Handler: makeAgentGuardScopeHandler(deps.Query),
	}); err != nil {
		return err
	}
	if err := registry.Register(&assistant.ToolSpec{
		Name: "AgentGuard.Evidence.List", Domain: assistant.DomainAgentGuard, Operation: assistant.OpList, Capability: "list_agent_guard_evidence",
		Description:      "List bounded Agent Guard evidence summaries by exact host, instance, session, execution unit, severity, and status filters.",
		ModelDescription: "Use this to enumerate evidence before selecting an exact scope ID; raw policy and isolation implementation fields are not returned.",
		Tags:             []string{"v6.3", "agent_guard", "evidence", "list"}, ObjectTypes: []string{"instance", "session", "execution_unit", "behavior", "finding", "action"}, Risk: assistant.ToolRiskReadonly, AutoCallable: true, Idempotent: true, DefaultWhitelisted: true, Enabled: true, ExposurePolicy: contextualExposure(190), DefaultTimeout: 20 * time.Second,
		ArgsSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"kind": map[string]interface{}{"type": "string", "enum": []interface{}{"instances", "sessions", "execution_units", "behaviors", "findings", "actions"}}, "host_id": map[string]interface{}{"type": "string", "format": "uuid"}, "instance_id": map[string]interface{}{"type": "string", "format": "uuid"}, "session_id": map[string]interface{}{"type": "string", "format": "uuid"}, "execution_unit_id": map[string]interface{}{"type": "string", "format": "uuid"}, "severity": map[string]interface{}{"type": "string"}, "status": map[string]interface{}{"type": "string"}, "page": map[string]interface{}{"type": "integer", "minimum": 1}, "page_size": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 50}}, "required": []string{"kind"}, "additionalProperties": false},
		Handler:    makeAgentGuardEvidenceListHandler(deps.Query),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name: "AgentGuard.Configuration.Assess", Domain: assistant.DomainAgentGuard, Operation: assistant.OpGet,
		Capability:       "assess_agent_guard_configuration",
		Description:      "Run a bounded static configuration assessment for one host and return finding summaries without raw file contents or secret values.",
		ModelDescription: "Use this to assess an agent configuration; raw paths, configuration content, hook commands, and secret values are intentionally hidden.",
		Tags:             []string{"v6.3", "agent_guard", "configuration", "redaction"}, ObjectTypes: []string{"host", "configuration_assessment"},
		Risk: assistant.ToolRiskLow, AutoCallable: true, Idempotent: false, DefaultWhitelisted: true, Enabled: true,
		ExposurePolicy: primaryExposure(205), DefaultTimeout: 60 * time.Second,
		ArgsSchema: uuidObjectSchema("host_id", "Exact host UUID to scan."),
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			if deps.ConfigScanner == nil {
				return nil, fmt.Errorf("agent guard configuration scanner is not configured")
			}
			hostID := getStringArg(args, "host_id", "")
			if _, err := uuid.Parse(hostID); err != nil {
				return nil, fmt.Errorf("host_id must be a UUID")
			}
			result, err := deps.ConfigScanner.Scan(ctx, hostID)
			if err != nil {
				return nil, err
			}
			return summarizeAgentConfigScan(result), nil
		},
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name: "AgentGuard.Catalog.List", Domain: assistant.DomainAgentGuard, Operation: assistant.OpList,
		Capability:       "list_agent_guard_catalog",
		Description:      "List enabled Agent Guard adapter and behavior catalog summaries needed to explain coverage and evidence semantics.",
		ModelDescription: "Use this for supported agent types, coverage capabilities, and semantic rule names; policy graphs and implementation parameters are hidden.",
		Tags:             []string{"v6.3", "agent_guard", "catalog"}, ObjectTypes: []string{"agent_guard_profile", "agent_guard_rule"},
		Risk: assistant.ToolRiskReadonly, AutoCallable: true, Idempotent: true, DefaultWhitelisted: true, Enabled: true,
		ExposurePolicy: contextualExposure(150), DefaultTimeout: 15 * time.Second,
		ArgsSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{
			"kind":    map[string]interface{}{"type": "string", "enum": []interface{}{"profiles", "rules"}},
			"keyword": map[string]interface{}{"type": "string"}, "page_size": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 50},
		}, "required": []string{"kind"}, "additionalProperties": false},
		Handler: makeAgentGuardCatalogHandler(deps.Catalog),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name: "AgentGuard.Finding.Analyze", Domain: assistant.DomainAgentGuard, Operation: assistant.OpGenerate,
		Capability:       "analyze_agent_guard_finding",
		Description:      "Queue bounded evidence analysis for one exact Agent Guard finding and return the durable analysis reference.",
		ModelDescription: "Use this when a finding needs evidence-based analysis; analysis does not authorize a control action.",
		Tags:             []string{"v6.3", "agent_guard", "analysis"}, ObjectTypes: []string{"finding", "analysis"},
		Risk: assistant.ToolRiskMedium, AutoCallable: false, RequiresApproval: true, Idempotent: true, DefaultWhitelisted: false, Enabled: true,
		ExposurePolicy: primaryExposure(200), DefaultTimeout: 15 * time.Second,
		ExecutionContract: assistant.ToolExecutionContract{Mode: assistant.ToolExecutionAsynchronous, CompletionCapability: "get_agent_guard_analysis"},
		ResultContract:    assistant.ToolResultContract{OperationStatusField: "status", PendingValues: []string{"pending", "running"}, SuccessValues: []string{"succeeded", "inconclusive"}, FailureValues: []string{"failed", "invalid_output", "cancelled"}, OperationRefFields: []string{"analysis_id"}},
		ArgsSchema:        uuidObjectSchema("finding_id", "Exact Agent Guard finding UUID."),
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			if deps.Analysis == nil {
				return nil, fmt.Errorf("agent guard analysis service is not configured")
			}
			findingID, err := parseUUIDArg(args, "finding_id")
			if err != nil {
				return nil, err
			}
			run, err := deps.Analysis.Request(ctx, findingID, agentGuardOperator(ctx))
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"analysis_id": run.ID.String(), "finding_id": run.FindingID.String(), "status": run.Status}, nil
		},
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name: "AgentGuard.Analysis.Get", Domain: assistant.DomainAgentGuard, Operation: assistant.OpGet,
		Capability: "get_agent_guard_analysis", Description: "Get the durable status and bounded result of an Agent Guard finding analysis.",
		ModelDescription: "Use the analysis ID returned by AgentGuard.Finding.Analyze to poll or inspect terminal evidence.",
		Tags:             []string{"v6.3", "agent_guard", "analysis", "status"}, ObjectTypes: []string{"analysis"}, Risk: assistant.ToolRiskReadonly,
		AutoCallable: true, Idempotent: true, DefaultWhitelisted: true, Enabled: true, ExposurePolicy: companionExposure(), DefaultTimeout: 15 * time.Second,
		ArgsSchema: uuidObjectSchema("analysis_id", "Exact analysis UUID."), Handler: makeAgentGuardAnalysisGetHandler(deps.Query),
	}); err != nil {
		return err
	}

	if err := registerAgentGuardActionTools(registry, deps); err != nil {
		return err
	}
	if err := registerAgentConversationTools(registry, deps); err != nil {
		return err
	}
	return registerAgentGuardRuntimeTools(registry, deps)
}

func registerAgentGuardActionTools(registry *assistant.ToolRegistry, deps AgentGuardToolDeps) error {
	for _, item := range []struct{ name, capability, action string }{
		{"AgentGuard.ExecutionUnit.Freeze", "freeze_agent_guard_execution_unit", model.AgentGuardActionFreezeExecutionUnit},
		{"AgentGuard.ExecutionUnit.Resume", "resume_agent_guard_execution_unit", model.AgentGuardActionResumeExecutionUnit},
		{"AgentGuard.ExecutionUnit.Kill", "kill_agent_guard_execution_unit", model.AgentGuardActionKillExecutionUnit},
	} {
		action := item
		if err := registry.Register(&assistant.ToolSpec{
			Name: action.name, Domain: assistant.DomainAgentGuard, Operation: assistant.OpExecute, Capability: action.capability,
			Description:      fmt.Sprintf("Request the explicit Agent Guard %s action for one exact execution unit after backend ownership and coverage checks.", action.action),
			ModelDescription: "This is a controlled high-risk action. It requires an exact execution unit ID, a reason, approval, and server-side safety checks.",
			Tags:             []string{"v6.3", "agent_guard", "action", "approval"}, ObjectTypes: []string{"execution_unit", "action"}, Risk: assistant.ToolRiskHigh,
			AutoCallable: false, RequiresApproval: true, Idempotent: true, DefaultWhitelisted: false, Enabled: true, ExposurePolicy: primaryExposure(100), DefaultTimeout: 30 * time.Second,
			ArgsSchema: actionArgsSchema("execution_unit_id"), Handler: makeAgentGuardUnitActionHandler(deps.Actions, action.action),
		}); err != nil {
			return err
		}
	}
	if err := registry.Register(&assistant.ToolSpec{
		Name: "AgentGuard.Instance.Kill", Domain: assistant.DomainAgentGuard, Operation: assistant.OpExecute, Capability: "kill_agent_guard_instance",
		Description: "Request termination of one exact Agent Guard runtime instance after backend ownership, coverage, and online-agent checks.", ModelDescription: "Controlled high-risk action requiring an exact runtime instance ID, a reason, and approval.",
		Tags: []string{"v6.3", "agent_guard", "action", "approval"}, ObjectTypes: []string{"instance", "action"}, Risk: assistant.ToolRiskCritical, AutoCallable: false, RequiresApproval: true, Idempotent: true, DefaultWhitelisted: false, Enabled: true, ExposurePolicy: primaryExposure(90), DefaultTimeout: 30 * time.Second,
		ArgsSchema: actionArgsSchema("instance_id"), Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			if deps.Actions == nil {
				return nil, fmt.Errorf("agent guard action service is not configured")
			}
			id, err := parseUUIDArg(args, "instance_id")
			if err != nil {
				return nil, err
			}
			action, err := deps.Actions.RequestInstanceKill(ctx, id, service.AgentGuardManualActionRequest{Reason: getStringArg(args, "reason", ""), Hold: getBoolArg(args, "hold", false)}, agentGuardOperator(ctx))
			if err != nil {
				return nil, err
			}
			return summarizeAgentGuardAction(action), nil
		},
	}); err != nil {
		return err
	}
	return registry.Register(&assistant.ToolSpec{
		Name: "AgentGuard.Action.Get", Domain: assistant.DomainAgentGuard, Operation: assistant.OpGet, Capability: "get_agent_guard_action",
		Description: "Get the bounded status and outcome of a requested Agent Guard action without exposing control policy internals.", ModelDescription: "Use an action ID returned by a controlled action tool to poll dispatch and completion.", Tags: []string{"v6.3", "agent_guard", "action", "status"}, ObjectTypes: []string{"action"}, Risk: assistant.ToolRiskReadonly, AutoCallable: true, Idempotent: true, DefaultWhitelisted: true, Enabled: true, ExposurePolicy: companionExposure(), DefaultTimeout: 15 * time.Second,
		ArgsSchema: uuidObjectSchema("action_id", "Exact action UUID."), Handler: makeAgentGuardActionGetHandler(deps.Query),
	})
}

func registerAgentConversationTools(registry *assistant.ToolRegistry, deps AgentGuardToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name: "AgentConversation.Query", Domain: assistant.DomainAgentGuard, Operation: assistant.OpList, Capability: "query_agent_conversations",
		Description: "List redacted Claude Code and Codex conversation session summaries with bounded host, risk, and pagination filters.", ModelDescription: "Use this to find conversation session IDs; content is redacted by the server and source paths are not returned.", Tags: []string{"v6.3", "agent_guard", "conversation"}, ObjectTypes: []string{"conversation_session"}, Risk: assistant.ToolRiskReadonly, AutoCallable: true, Idempotent: true, DefaultWhitelisted: true, Enabled: true, ExposurePolicy: primaryExposure(180), DefaultTimeout: 15 * time.Second,
		ArgsSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"host_id": map[string]interface{}{"type": "string", "format": "uuid"}, "agent_type": map[string]interface{}{"type": "string", "enum": []interface{}{model.AgentSessionSourceClaude, model.AgentSessionSourceCodex}}, "risk": map[string]interface{}{"type": "string"}, "page": map[string]interface{}{"type": "integer", "minimum": 1, "description": "1-based page number."}, "page_size": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 50, "default": 20, "description": "Items per page; the result always returns total_pages and has_next_page before items."}}, "additionalProperties": false},
		Handler:    makeConversationQueryHandler(deps.Conversations),
	}); err != nil {
		return err
	}
	if err := registry.Register(&assistant.ToolSpec{
		Name: "AgentConversation.Get", Domain: assistant.DomainAgentGuard, Operation: assistant.OpGet, Capability: "get_agent_conversation_content",
		Description: "Get one redacted conversation session and optionally its bounded redacted messages; raw source files and unredacted content are never returned.", ModelDescription: "Use an exact conversation session ID returned by AgentConversation.Query.", Tags: []string{"v6.3", "agent_guard", "conversation", "redaction"}, ObjectTypes: []string{"conversation_session", "conversation_item"}, Risk: assistant.ToolRiskReadonly, AutoCallable: true, Idempotent: true, DefaultWhitelisted: true, Enabled: true, ExposurePolicy: contextualExposure(130), DefaultTimeout: 15 * time.Second,
		ArgsSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"session_id": map[string]interface{}{"type": "string", "format": "uuid"}, "include_content": map[string]interface{}{"type": "boolean"}}, "required": []string{"session_id"}, "additionalProperties": false}, Handler: makeConversationGetHandler(deps.Conversations),
	}); err != nil {
		return err
	}
	if err := registry.Register(&assistant.ToolSpec{
		Name: "AgentConversation.Collect", Domain: assistant.DomainAgentGuard, Operation: assistant.OpDispatch, Capability: "collect_agent_conversations",
		Description: "Request a bounded static redacted conversation collection for one host and supported agent type.", ModelDescription: "This dispatch is explicit and auditable; it does not install a hook or authorize arbitrary command execution.", Tags: []string{"v6.3", "agent_guard", "conversation", "collection"}, ObjectTypes: []string{"host", "conversation_session"}, Risk: assistant.ToolRiskMedium, AutoCallable: false, RequiresApproval: true, Idempotent: true, DefaultWhitelisted: false, Enabled: true, ExposurePolicy: primaryExposure(120), DefaultTimeout: 30 * time.Second,
		ArgsSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"host_id": map[string]interface{}{"type": "string", "format": "uuid"}, "agent_type": map[string]interface{}{"type": "string", "enum": []interface{}{model.AgentSessionSourceClaude, model.AgentSessionSourceCodex}}}, "required": []string{"host_id", "agent_type"}, "additionalProperties": false}, Handler: makeConversationCollectHandler(deps.Conversations),
	}); err != nil {
		return err
	}
	if err := registry.Register(&assistant.ToolSpec{
		Name: "AgentConversation.Analyze", Domain: assistant.DomainAgentGuard, Operation: assistant.OpGenerate, Capability: "analyze_agent_conversation",
		Description: "Analyze one redacted conversation session for prompt injection, jailbreak, secret exposure, unsafe tool intent, and social engineering.", ModelDescription: "Use this for evidence analysis only; the result never authorizes an Agent Guard control action.", Tags: []string{"v6.3", "agent_guard", "conversation", "analysis"}, ObjectTypes: []string{"conversation_session", "conversation_analysis"}, Risk: assistant.ToolRiskMedium, AutoCallable: false, RequiresApproval: true, Idempotent: false, DefaultWhitelisted: false, Enabled: true, ExposurePolicy: primaryExposure(110), DefaultTimeout: 120 * time.Second,
		ArgsSchema: uuidObjectSchema("session_id", "Exact conversation session UUID."), Handler: makeConversationAnalyzeHandler(deps.Conversations),
	}); err != nil {
		return err
	}
	if err := registry.Register(&assistant.ToolSpec{
		Name: "AgentGuard.Session.Delete", Domain: assistant.DomainAgentGuard, Operation: assistant.OpDelete, Capability: "delete_agent_guard_behavior_sessions",
		Description:      "Delete explicitly selected Agent Guard behavior sessions and their owned evidence records through the transactional service boundary.",
		ModelDescription: "This is irreversible evidence deletion. It requires explicit session UUIDs, approval, and must never be inferred from a natural-language scope.",
		Tags:             []string{"v6.3", "agent_guard", "session", "destructive", "approval"}, ObjectTypes: []string{"session", "finding", "behavior"},
		Risk: assistant.ToolRiskCritical, AutoCallable: false, RequiresApproval: true, Idempotent: true, DefaultWhitelisted: false, Enabled: true, ExposurePolicy: primaryExposure(70), DefaultTimeout: 30 * time.Second,
		ArgsSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"session_ids": map[string]interface{}{"type": "array", "minItems": 1, "maxItems": 50, "items": map[string]interface{}{"type": "string", "format": "uuid"}}}, "required": []string{"session_ids"}, "additionalProperties": false},
		Handler:    makeAgentGuardSessionDeleteHandler(deps.Query),
	}); err != nil {
		return err
	}
	return registry.Register(&assistant.ToolSpec{
		Name: "AgentConversation.Analysis.Get", Domain: assistant.DomainAgentGuard, Operation: assistant.OpGet, Capability: "get_agent_conversation_analysis", Description: "Get the latest persisted redacted analysis result for a conversation session.", ModelDescription: "Use this to read a previously requested conversation analysis without re-running the model.", Tags: []string{"v6.3", "agent_guard", "conversation", "analysis"}, ObjectTypes: []string{"conversation_analysis"}, Risk: assistant.ToolRiskReadonly, AutoCallable: true, Idempotent: true, DefaultWhitelisted: true, Enabled: true, ExposurePolicy: companionExposure(), DefaultTimeout: 15 * time.Second,
		ArgsSchema: uuidObjectSchema("session_id", "Exact conversation session UUID."), Handler: makeConversationAIGetHandler(deps.Conversations),
	})
}

func registerAgentGuardRuntimeTools(registry *assistant.ToolRegistry, deps AgentGuardToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{Name: "AgentGuard.RuntimeSettings.Get", Domain: assistant.DomainAgentGuard, Operation: assistant.OpGet, Capability: "get_agent_guard_runtime_settings", Description: "Get normalized Agent Guard runtime setting status for one host without exposing policy source or secret values.", ModelDescription: "Use this to inspect whether supported adapters and hooks are enabled or pending reconnect.", Tags: []string{"v6.3", "agent_guard", "settings"}, ObjectTypes: []string{"host", "runtime_settings"}, Risk: assistant.ToolRiskReadonly, AutoCallable: true, Idempotent: true, DefaultWhitelisted: true, Enabled: true, ExposurePolicy: contextualExposure(100), DefaultTimeout: 15 * time.Second, ArgsSchema: uuidObjectSchema("host_id", "Exact host UUID."), Handler: makeRuntimeGetHandler(deps.Runtime)}); err != nil {
		return err
	}
	return registry.Register(&assistant.ToolSpec{Name: "AgentGuard.RuntimeSettings.Update", Domain: assistant.DomainAgentGuard, Operation: assistant.OpUpdate, Capability: "update_agent_guard_runtime_settings", Description: "Update normalized Agent Guard runtime adapter and hook switches for one host through the audited settings service.", ModelDescription: "Controlled high-risk settings change. It accepts only explicit boolean switches and supported agent hook entries; policy definitions remain hidden.", Tags: []string{"v6.3", "agent_guard", "settings", "approval"}, ObjectTypes: []string{"host", "runtime_settings"}, Risk: assistant.ToolRiskHigh, AutoCallable: false, RequiresApproval: true, Idempotent: true, DefaultWhitelisted: false, Enabled: true, ExposurePolicy: primaryExposure(80), DefaultTimeout: 30 * time.Second, ArgsSchema: runtimeSettingsSchema(), Handler: makeRuntimeUpdateHandler(deps.Runtime)})
}

func primaryExposure(priority int) assistant.ToolExposurePolicy {
	return assistant.ToolExposurePolicy{Exposure: assistant.ToolExposurePrimary, Discoverable: true, DirectCallable: true, CatalogPriority: priority}
}
func contextualExposure(priority int) assistant.ToolExposurePolicy {
	return assistant.ToolExposurePolicy{Exposure: assistant.ToolExposureContextual, Discoverable: true, DirectCallable: true, CatalogPriority: priority}
}
func companionExposure() assistant.ToolExposurePolicy {
	return assistant.ToolExposurePolicy{Exposure: assistant.ToolExposureCompanion, Discoverable: false, DirectCallable: true}
}
func emptyObjectSchema() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}, "additionalProperties": false}
}
func uuidObjectSchema(name, description string) map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{name: map[string]interface{}{"type": "string", "format": "uuid", "description": description}}, "required": []string{name}, "additionalProperties": false}
}
func actionArgsSchema(idName string) map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{idName: map[string]interface{}{"type": "string", "format": "uuid"}, "reason": map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 1000}, "hold": map[string]interface{}{"type": "boolean"}}, "required": []string{idName, "reason"}, "additionalProperties": false}
}
func runtimeSettingsSchema() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{"host_id": map[string]interface{}{"type": "string", "format": "uuid"}, "tool_adapter_enabled": map[string]interface{}{"type": "boolean"}, "session_hook_enabled": map[string]interface{}{"type": "boolean"}, "behavior_policy_enabled": map[string]interface{}{"type": "boolean"}, "escape_policy_enabled": map[string]interface{}{"type": "boolean"}, "injections": map[string]interface{}{"type": "array", "maxItems": 10, "items": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"agent_type": map[string]interface{}{"type": "string"}, "behavior_enabled": map[string]interface{}{"type": "boolean"}, "escape_enabled": map[string]interface{}{"type": "boolean"}}, "required": []string{"agent_type"}, "additionalProperties": false}}}, "required": []string{"host_id", "tool_adapter_enabled", "session_hook_enabled", "behavior_policy_enabled", "escape_policy_enabled"}, "additionalProperties": false}
}

func makeAgentGuardScopeHandler(query AgentGuardQueryForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if query == nil {
			return nil, fmt.Errorf("agent guard query service is not configured")
		}
		kind, id := strings.ToLower(strings.TrimSpace(getStringArg(args, "scope_type", ""))), strings.TrimSpace(getStringArg(args, "scope_id", ""))
		if kind == "" || id == "" {
			return nil, fmt.Errorf("scope_type and scope_id are required")
		}
		switch kind {
		case "instance":
			scopeID, err := parseScopeUUID(id, kind)
			if err != nil {
				return nil, err
			}
			v, err := query.GetInstance(ctx, scopeID)
			return wrapScope(v, err, "instance")
		case "session":
			scopeID, err := parseScopeUUID(id, kind)
			if err != nil {
				return nil, err
			}
			v, err := query.GetSession(ctx, scopeID)
			return wrapScope(v, err, "session")
		case "execution_unit":
			scopeID, err := parseScopeUUID(id, kind)
			if err != nil {
				return nil, err
			}
			v, err := query.GetExecutionUnit(ctx, scopeID)
			return wrapScope(v, err, "execution_unit")
		case "behavior":
			v, err := query.GetBehavior(ctx, id)
			return wrapBehavior(v, err)
		case "runtime_event":
			v, err := query.GetRuntimeEvent(ctx, id)
			return wrapRuntimeEvent(v, err)
		case "finding":
			scopeID, err := parseScopeUUID(id, kind)
			if err != nil {
				return nil, err
			}
			v, err := query.GetFinding(ctx, scopeID)
			return wrapFinding(v, err)
		case "analysis":
			scopeID, err := parseScopeUUID(id, kind)
			if err != nil {
				return nil, err
			}
			v, err := query.GetAnalysis(ctx, scopeID)
			return wrapAnalysis(v, err)
		case "action":
			scopeID, err := parseScopeUUID(id, kind)
			if err != nil {
				return nil, err
			}
			v, err := query.GetAction(ctx, scopeID)
			return wrapAction(v, err)
		default:
			return nil, fmt.Errorf("unsupported scope_type %q", kind)
		}
	}
}

func makeAgentGuardEvidenceListHandler(query AgentGuardQueryForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if query == nil {
			return nil, fmt.Errorf("agent guard query service is not configured")
		}
		kind := strings.ToLower(strings.TrimSpace(getStringArg(args, "kind", "")))
		page, pageSize := getIntArg(args, "page", 1), getIntArg(args, "page_size", 20)
		if page < 1 || pageSize < 1 || pageSize > 50 {
			return nil, fmt.Errorf("page and page_size are out of range")
		}
		idFilter := func(name string) string { return getStringArg(args, name, "") }
		switch kind {
		case "instances":
			items, total, err := query.ListInstances(ctx, model.AgentRuntimeInstanceQuery{AgentGuardPageQuery: model.AgentGuardPageQuery{Page: page, PageSize: pageSize}, HostID: idFilter("host_id"), InstanceIDs: optionalStringSlice(idFilter("instance_id")), Status: getStringArg(args, "status", "")})
			if err != nil {
				return nil, err
			}
			out := make([]interface{}, 0, len(items))
			for i := range items {
				out = append(out, map[string]interface{}{"id": items[i].ID, "host_id": items[i].HostID, "agent_type": items[i].AgentType, "profile_key": items[i].ProfileKey, "display_name": items[i].DisplayName, "status": items[i].Status, "coverage_level": items[i].CoverageLevel, "coverage_reasons": decodeStringArray(items[i].CoverageReasons), "last_seen_at": items[i].LastSeenAt})
			}
			return map[string]interface{}{"kind": kind, "items": out, "total": total, "page": page, "page_size": pageSize}, nil
		case "sessions":
			items, total, err := query.ListSessions(ctx, model.AgentBehaviorSessionQuery{AgentGuardPageQuery: model.AgentGuardPageQuery{Page: page, PageSize: pageSize}, InstanceID: idFilter("instance_id"), ExecutionUnitID: idFilter("execution_unit_id"), Status: getStringArg(args, "status", "")})
			if err != nil {
				return nil, err
			}
			out := make([]interface{}, 0, len(items))
			for i := range items {
				out = append(out, map[string]interface{}{"id": items[i].ID, "host_id": items[i].HostID, "instance_id": items[i].InstanceID, "execution_unit_id": items[i].ExecutionUnitID, "external_session_id": items[i].ExternalSessionID, "source": items[i].Source, "confidence": items[i].Confidence, "status": items[i].Status, "behavior_count": items[i].BehaviorCount, "finding_count": items[i].FindingCount, "last_seen_at": items[i].LastSeenAt})
			}
			return map[string]interface{}{"kind": kind, "items": out, "total": total, "page": page, "page_size": pageSize}, nil
		case "execution_units":
			items, total, err := query.ListExecutionUnits(ctx, model.AgentExecutionUnitQuery{AgentGuardPageQuery: model.AgentGuardPageQuery{Page: page, PageSize: pageSize}, HostID: idFilter("host_id"), InstanceID: idFilter("instance_id"), Status: getStringArg(args, "status", "")})
			if err != nil {
				return nil, err
			}
			out := make([]interface{}, 0, len(items))
			for i := range items {
				out = append(out, map[string]interface{}{"id": items[i].ID, "host_id": items[i].HostID, "instance_id": items[i].InstanceID, "unit_type": items[i].UnitType, "status": items[i].Status, "coverage_level": items[i].CoverageLevel, "coverage_reasons": decodeStringArray(items[i].CoverageReasons), "last_seen_at": items[i].LastSeenAt, "frozen_at": items[i].FrozenAt})
			}
			return map[string]interface{}{"kind": kind, "items": out, "total": total, "page": page, "page_size": pageSize}, nil
		case "behaviors":
			items, total, err := query.ListBehaviors(ctx, model.AgentBehaviorEventQuery{AgentGuardPageQuery: model.AgentGuardPageQuery{Page: page, PageSize: pageSize}, HostID: idFilter("host_id"), InstanceID: idFilter("instance_id"), SessionID: idFilter("session_id"), ExecutionUnitID: idFilter("execution_unit_id"), Severity: getStringArg(args, "severity", ""), Decision: getStringArg(args, "status", "")})
			if err != nil {
				return nil, err
			}
			out := make([]interface{}, 0, len(items))
			for i := range items {
				out = append(out, map[string]interface{}{"id": items[i].ID, "raw_event_id": items[i].RawEventID, "host_id": items[i].HostID, "instance_id": items[i].InstanceID, "session_id": items[i].SessionID, "execution_unit_id": items[i].ExecutionUnitID, "category": items[i].Category, "operation": items[i].Operation, "outcome": items[i].Outcome, "decision": items[i].Decision, "severity": items[i].Severity, "occurred_at": items[i].OccurredAt})
			}
			return map[string]interface{}{"kind": kind, "items": out, "total": total, "page": page, "page_size": pageSize}, nil
		case "findings":
			items, total, err := query.ListFindings(ctx, model.AgentSecurityFindingQuery{AgentGuardPageQuery: model.AgentGuardPageQuery{Page: page, PageSize: pageSize}, HostID: idFilter("host_id"), InstanceID: idFilter("instance_id"), SessionID: idFilter("session_id"), ExecutionUnitID: idFilter("execution_unit_id"), Severity: getStringArg(args, "severity", ""), Status: getStringArg(args, "status", "")})
			if err != nil {
				return nil, err
			}
			out := make([]interface{}, 0, len(items))
			for i := range items {
				out = append(out, summarizeAgentGuardFinding(&items[i]))
			}
			return map[string]interface{}{"kind": kind, "items": out, "total": total, "page": page, "page_size": pageSize}, nil
		case "actions":
			items, total, err := query.ListActions(ctx, model.AgentGuardActionQuery{AgentGuardPageQuery: model.AgentGuardPageQuery{Page: page, PageSize: pageSize}, HostID: idFilter("host_id"), InstanceID: idFilter("instance_id"), ExecutionUnitID: idFilter("execution_unit_id"), Status: getStringArg(args, "status", "")})
			if err != nil {
				return nil, err
			}
			out := make([]interface{}, 0, len(items))
			for i := range items {
				out = append(out, summarizeAgentGuardAction(&items[i]))
			}
			return map[string]interface{}{"kind": kind, "items": out, "total": total, "page": page, "page_size": pageSize}, nil
		default:
			return nil, fmt.Errorf("kind must be instances, sessions, execution_units, behaviors, findings, or actions")
		}
	}
}

func optionalStringSlice(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{strings.TrimSpace(value)}
}

func makeAgentGuardCatalogHandler(catalog AgentGuardCatalogForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if catalog == nil {
			return nil, fmt.Errorf("agent guard catalog service is not configured")
		}
		pageSize := getIntArg(args, "page_size", 20)
		if pageSize < 1 || pageSize > 50 {
			return nil, fmt.Errorf("page_size must be between 1 and 50")
		}
		keyword := getStringArg(args, "keyword", "")
		if getStringArg(args, "kind", "") == "profiles" {
			enabled := true
			items, total, err := catalog.ListProfiles(ctx, model.AgentGuardProfileQuery{AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: pageSize}, Enabled: &enabled, Keyword: keyword})
			if err != nil {
				return nil, err
			}
			out := make([]map[string]interface{}, 0, len(items))
			for _, p := range items {
				out = append(out, map[string]interface{}{"profile_key": p.ProfileKey, "version": p.ProfileVersion, "agent_type": p.AgentType, "display_name": p.DisplayName, "sandbox_family": p.SandboxFamily, "enabled": p.Enabled})
			}
			return map[string]interface{}{"kind": "profiles", "items": out, "total": total, "policy_details_hidden": true}, nil
		}
		if getStringArg(args, "kind", "") == "rules" {
			items, total, err := catalog.ListRules(ctx, model.AgentBehaviorRuleQuery{AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: pageSize}, Keyword: keyword})
			if err != nil {
				return nil, err
			}
			out := make([]map[string]interface{}, 0, len(items))
			for _, r := range items {
				out = append(out, map[string]interface{}{"rule_key": r.RuleKey, "version": r.RuleVersion, "name": r.Name, "description": r.Description, "engine": r.Engine, "categories": decodeStringArray(r.Categories), "default_severity": r.DefaultSeverity, "recommended_action": r.RecommendedAction, "immutable": r.Immutable})
			}
			return map[string]interface{}{"kind": "rules", "items": out, "total": total, "policy_details_hidden": true}, nil
		}
		return nil, fmt.Errorf("kind must be profiles or rules")
	}
}

func makeAgentGuardUnitActionHandler(actions AgentGuardActionsForTools, action string) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if actions == nil {
			return nil, fmt.Errorf("agent guard action service is not configured")
		}
		id, err := parseUUIDArg(args, "execution_unit_id")
		if err != nil {
			return nil, err
		}
		result, err := actions.RequestExecutionUnit(ctx, id, action, service.AgentGuardManualActionRequest{Reason: getStringArg(args, "reason", ""), Hold: getBoolArg(args, "hold", false)}, agentGuardOperator(ctx))
		if err != nil {
			return nil, err
		}
		return summarizeAgentGuardAction(result), nil
	}
}
func makeAgentGuardAnalysisGetHandler(query AgentGuardQueryForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if query == nil {
			return nil, fmt.Errorf("agent guard query service is not configured")
		}
		id, err := parseUUIDArg(args, "analysis_id")
		if err != nil {
			return nil, err
		}
		v, err := query.GetAnalysis(ctx, id)
		return wrapAnalysis(v, err)
	}
}
func makeAgentGuardActionGetHandler(query AgentGuardQueryForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if query == nil {
			return nil, fmt.Errorf("agent guard query service is not configured")
		}
		id, err := parseUUIDArg(args, "action_id")
		if err != nil {
			return nil, err
		}
		v, err := query.GetAction(ctx, id)
		return wrapAction(v, err)
	}
}

func makeConversationQueryHandler(s AgentConversationForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if s == nil {
			return nil, fmt.Errorf("agent conversation service is not configured")
		}
		var hostID *uuid.UUID
		if raw := getStringArg(args, "host_id", ""); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				return nil, fmt.Errorf("host_id must be a UUID")
			}
			hostID = &id
		}
		page, size := getIntArg(args, "page", 1), getIntArg(args, "page_size", 20)
		if page < 1 || size < 1 || size > 50 {
			return nil, fmt.Errorf("page and page_size are out of range")
		}
		result, err := s.List(ctx, hostID, getStringArg(args, "agent_type", ""), getStringArg(args, "risk", ""), page, size)
		if err != nil {
			return nil, err
		}
		totalPages := 0
		if result.Total > 0 {
			totalPages = int((result.Total + int64(size) - 1) / int64(size))
		}
		items := make([]agentConversationSessionSummary, 0, len(result.Items))
		riskSummary := agentConversationRiskSummary{
			RiskTypesAvailable: false,
			HighRiskSessions:   make([]agentConversationRiskDigest, 0),
		}
		for _, session := range result.Items {
			items = append(items, agentConversationSessionSummary{
				ID:           session.ID,
				HostID:       session.HostID,
				AgentType:    session.AgentType,
				ExternalID:   session.ExternalSessionID,
				Title:        session.Title,
				Model:        session.Model,
				State:        session.State,
				RiskLevel:    session.RiskLevel,
				RuleHitCount: session.RuleHitCount,
				ItemCount:    session.ItemCount,
				FirstSeenAt:  session.FirstSeenAt,
				LastSeenAt:   session.LastSeenAt,
			})
			switch strings.ToLower(strings.TrimSpace(session.RiskLevel)) {
			case model.AgentSessionRiskHigh:
				riskSummary.HighRiskCount++
				if strings.EqualFold(session.State, model.AgentSessionStateActive) {
					riskSummary.ActiveHighRiskCount++
				}
				riskSummary.HighRiskSessions = append(riskSummary.HighRiskSessions, agentConversationRiskDigest{
					ID:           session.ID,
					State:        session.State,
					RuleHitCount: session.RuleHitCount,
					ItemCount:    session.ItemCount,
				})
			case "", "unknown":
				riskSummary.UnknownRiskCount++
			}
		}
		return agentConversationQueryResult{
			Page:          page,
			PageSize:      size,
			ReturnedCount: len(items),
			Total:         result.Total,
			TotalPages:    totalPages,
			HasNextPage:   totalPages > page,
			HasPrevious:   page > 1,
			RiskSummary:   riskSummary,
			Items:         items,
		}, nil
	}
}
func makeConversationGetHandler(s AgentConversationForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if s == nil {
			return nil, fmt.Errorf("agent conversation service is not configured")
		}
		id, err := parseUUIDArg(args, "session_id")
		if err != nil {
			return nil, err
		}
		session, items, err := s.Detail(ctx, id, getBoolArg(args, "include_content", false))
		if err != nil {
			return nil, err
		}
		out := map[string]interface{}{"session": session, "content_redacted": true}
		if items != nil {
			bounded := make([]map[string]interface{}, 0, len(items))
			for _, item := range items {
				content := item.ContentRedacted
				if len(content) > 4096 {
					content = content[:4096]
				}
				bounded = append(bounded, map[string]interface{}{"sequence": item.Sequence, "item_type": item.ItemType, "role": item.Role, "occurred_at": item.OccurredAt, "content": content, "visibility": item.Visibility, "redaction_applied": item.RedactionApplied})
			}
			out["items"] = bounded
		}
		return out, nil
	}
}
func makeConversationCollectHandler(s AgentConversationForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if s == nil {
			return nil, fmt.Errorf("agent conversation service is not configured")
		}
		host, err := parseUUIDArg(args, "host_id")
		if err != nil {
			return nil, err
		}
		agentType := getStringArg(args, "agent_type", "")
		return s.RequestCollection(ctx, host, agentType)
	}
}
func makeConversationAnalyzeHandler(s AgentConversationForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if s == nil {
			return nil, fmt.Errorf("agent conversation service is not configured")
		}
		id, err := parseUUIDArg(args, "session_id")
		if err != nil {
			return nil, err
		}
		return s.Analyze(ctx, id)
	}
}
func makeConversationAIGetHandler(s AgentConversationForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if s == nil {
			return nil, fmt.Errorf("agent conversation service is not configured")
		}
		id, err := parseUUIDArg(args, "session_id")
		if err != nil {
			return nil, err
		}
		return s.GetAIAnalysis(ctx, id)
	}
}

func makeAgentGuardSessionDeleteHandler(query AgentGuardQueryForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if query == nil {
			return nil, fmt.Errorf("agent guard query service is not configured")
		}
		ids, err := parseUUIDSliceArg(args, "session_ids")
		if err != nil {
			return nil, err
		}
		if err := query.DeleteSessions(ctx, ids); err != nil {
			return nil, err
		}
		return map[string]interface{}{"deleted_session_ids": ids, "deleted_count": len(ids)}, nil
	}
}
func makeRuntimeGetHandler(s AgentGuardRuntimeSettingsForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if s == nil {
			return nil, fmt.Errorf("agent guard runtime settings service is not configured")
		}
		id, err := parseUUIDArg(args, "host_id")
		if err != nil {
			return nil, err
		}
		return s.Get(ctx, id)
	}
}
func makeRuntimeUpdateHandler(s AgentGuardRuntimeSettingsForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if s == nil {
			return nil, fmt.Errorf("agent guard runtime settings service is not configured")
		}
		host, err := parseUUIDArg(args, "host_id")
		if err != nil {
			return nil, err
		}
		settings := model.DefaultAgentGuardRuntimeSettings(host.String())
		settings.ToolAdapterEnabled = getBoolArg(args, "tool_adapter_enabled", settings.ToolAdapterEnabled)
		settings.SessionHookEnabled = getBoolArg(args, "session_hook_enabled", settings.SessionHookEnabled)
		settings.BehaviorPolicyEnabled = getBoolArg(args, "behavior_policy_enabled", settings.BehaviorPolicyEnabled)
		settings.EscapePolicyEnabled = getBoolArg(args, "escape_policy_enabled", settings.EscapePolicyEnabled)
		if raw, ok := args["injections"].([]interface{}); ok {
			settings.Injections = make([]model.AgentGuardHookInjection, 0, len(raw))
			for _, value := range raw {
				item, ok := value.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("injections must contain objects")
				}
				settings.Injections = append(settings.Injections, model.AgentGuardHookInjection{AgentType: getStringArg(item, "agent_type", ""), BehaviorEnabled: getBoolArg(item, "behavior_enabled", false), EscapeEnabled: getBoolArg(item, "escape_enabled", false)})
			}
		}
		updated, err := s.Update(ctx, settings, agentGuardOperator(ctx))
		if err != nil {
			return nil, err
		}
		return updated, nil
	}
}

func parseUUIDArg(args map[string]interface{}, name string) (uuid.UUID, error) {
	raw := strings.TrimSpace(getStringArg(args, name, ""))
	if raw == "" {
		return uuid.Nil, fmt.Errorf("%s is required", name)
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s must be a UUID", name)
	}
	return id, nil
}

func parseUUIDSliceArg(args map[string]interface{}, name string) ([]uuid.UUID, error) {
	raw, ok := args[name].([]interface{})
	if !ok || len(raw) == 0 {
		return nil, fmt.Errorf("%s must contain at least one UUID", name)
	}
	ids := make([]uuid.UUID, 0, len(raw))
	seen := make(map[uuid.UUID]struct{}, len(raw))
	for _, value := range raw {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%s must contain only UUID strings", name)
		}
		id, err := uuid.Parse(strings.TrimSpace(text))
		if err != nil {
			return nil, fmt.Errorf("%s contains an invalid UUID", name)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}
func parseScopeUUID(raw, kind string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("scope_id must be a UUID for %s", kind)
	}
	return id, nil
}
func agentGuardOperator(ctx context.Context) string {
	if invocation, ok := assistant.ToolInvocationFromContext(ctx); ok && strings.TrimSpace(invocation.Operator) != "" {
		return strings.TrimSpace(invocation.Operator)
	}
	return "assistant"
}
func wrapScope(v interface{}, err error, kind string) (interface{}, error) {
	if err != nil {
		return nil, err
	}
	var scope interface{}
	switch value := v.(type) {
	case *model.AgentRuntimeInstance:
		scope = map[string]interface{}{"id": value.ID, "host_id": value.HostID, "agent_type": value.AgentType, "profile_key": value.ProfileKey, "display_name": value.DisplayName, "detection_confidence": value.DetectionConfidence, "status": value.Status, "coverage_level": value.CoverageLevel, "coverage_reasons": decodeStringArray(value.CoverageReasons), "last_seen_at": value.LastSeenAt}
	case *model.AgentBehaviorSession:
		scope = map[string]interface{}{"id": value.ID, "host_id": value.HostID, "instance_id": value.InstanceID, "execution_unit_id": value.ExecutionUnitID, "external_session_id": value.ExternalSessionID, "source": value.Source, "confidence": value.Confidence, "status": value.Status, "behavior_count": value.BehaviorCount, "finding_count": value.FindingCount, "started_at": value.StartedAt, "last_seen_at": value.LastSeenAt, "ended_at": value.EndedAt}
	case *model.AgentExecutionUnit:
		scope = map[string]interface{}{"id": value.ID, "host_id": value.HostID, "instance_id": value.InstanceID, "unit_type": value.UnitType, "status": value.Status, "coverage_level": value.CoverageLevel, "coverage_reasons": decodeStringArray(value.CoverageReasons), "container_runtime": value.ContainerRuntime, "first_seen_at": value.FirstSeenAt, "last_seen_at": value.LastSeenAt, "frozen_at": value.FrozenAt, "stopped_at": value.StoppedAt}
	default:
		scope = v
	}
	return map[string]interface{}{"scope_type": kind, "scope": scope}, nil
}
func wrapBehavior(v *model.AgentBehaviorEvent, err error) (interface{}, error) {
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"scope_type": "behavior", "scope": map[string]interface{}{"id": v.ID, "raw_event_id": v.RawEventID, "host_id": v.HostID, "instance_id": v.InstanceID, "session_id": v.SessionID, "execution_unit_id": v.ExecutionUnitID, "category": v.Category, "operation": v.Operation, "outcome": v.Outcome, "decision": v.Decision, "severity": v.Severity, "occurred_at": v.OccurredAt}}, nil
}
func wrapRuntimeEvent(v *model.RuntimeEvent, err error) (interface{}, error) {
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"scope_type": "runtime_event", "scope": map[string]interface{}{"event_id": v.EventID, "host_id": v.HostID, "event_type": v.EventType, "severity": v.Severity, "occurred_at": v.Timestamp}}, nil
}
func wrapFinding(v *model.AgentSecurityFinding, err error) (interface{}, error) {
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"scope_type": "finding", "scope": summarizeAgentGuardFinding(v)}, nil
}
func wrapAnalysis(v *model.AgentSecurityAnalysisRun, err error) (interface{}, error) {
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"scope_type": "analysis", "scope": map[string]interface{}{"analysis_id": v.ID, "finding_id": v.FindingID, "attempt": v.Attempt, "status": v.Status, "verdict": v.Verdict, "confidence": v.Confidence, "attack_probability": v.AttackProbability, "provider": v.Provider, "model": v.Model, "summary": v.Output, "error_code": v.ErrorCode, "error_message": v.ErrorMessage, "queued_at": v.QueuedAt, "started_at": v.StartedAt, "completed_at": v.CompletedAt}}, nil
}
func wrapAction(v *model.AgentGuardAction, err error) (interface{}, error) {
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"scope_type": "action", "scope": summarizeAgentGuardAction(v)}, nil
}
func summarizeAgentGuardAction(v *model.AgentGuardAction) map[string]interface{} {
	if v == nil {
		return nil
	}
	return map[string]interface{}{"action_id": v.ID, "command_id": v.CommandID, "host_id": v.HostID, "instance_id": v.InstanceID, "execution_unit_id": v.ExecutionUnitID, "action": v.Action, "status": v.Status, "source": v.Source, "requested_by": v.RequestedBy, "hold_requested": v.HoldRequested, "error_code": v.ErrorCode, "error_message": v.ErrorMessage, "requested_at": v.RequestedAt, "completed_at": v.CompletedAt}
}
func summarizeAgentGuardFinding(v *model.AgentSecurityFinding) map[string]interface{} {
	if v == nil {
		return nil
	}
	return map[string]interface{}{"finding_id": v.ID, "finding_key": v.FindingKey, "host_id": v.HostID, "instance_id": v.InstanceID, "session_id": v.SessionID, "execution_unit_id": v.ExecutionUnitID, "title": v.Title, "severity": v.Severity, "verdict": v.Verdict, "confidence": v.Confidence, "status": v.Status, "summary": v.Summary, "recommended_action": v.RecommendedAction, "rule_hits": decodeStringArray(v.RuleHits), "evidence_event_count": jsonArrayLen(v.EvidenceEventIDs), "first_observed_at": v.FirstObservedAt, "last_observed_at": v.LastObservedAt}
}
func summarizeAgentConfigScan(v *service.AgentConfigScanResult) map[string]interface{} {
	if v == nil {
		return nil
	}
	agents := make([]map[string]interface{}, 0, len(v.Agents))
	for _, agent := range v.Agents {
		findings := make([]map[string]interface{}, 0)
		for _, finding := range agent.Files {
			for _, item := range finding.Findings {
				findings = append(findings, map[string]interface{}{"rule_id": item.RuleID, "severity": item.Severity, "title": item.Title, "reason": item.Reason, "remediation": item.Remediation})
			}
		}
		for _, hook := range agent.Hooks {
			for _, item := range hook.Findings {
				findings = append(findings, map[string]interface{}{"rule_id": item.RuleID, "severity": item.Severity, "title": item.Title, "reason": item.Reason, "remediation": item.Remediation})
			}
		}
		agents = append(agents, map[string]interface{}{"agent_type": agent.AgentType, "display_name": agent.DisplayName, "file_count": len(agent.Files), "hook_count": len(agent.Hooks), "finding_count": agent.FindingCount, "findings": findings})
	}
	return map[string]interface{}{"host_id": v.HostID, "hostname": v.Hostname, "scanned_at": v.ScannedAt, "finding_count": v.FindingCount, "errors": v.Errors, "agents": agents, "raw_configuration_hidden": true}
}
func decodeStringArray(raw []byte) []string {
	var values []string
	_ = json.Unmarshal(raw, &values)
	return values
}
func jsonArrayLen(raw []byte) int {
	var values []interface{}
	if json.Unmarshal(raw, &values) != nil {
		return 0
	}
	return len(values)
}
