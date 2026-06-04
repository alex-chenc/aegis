# Aegis V6.0 后端开发文档: Assistant 智能体编排层

**版本**: 6.0  
**日期**: 2026-05-29  
**状态**: 设计中

---

## 1. 后端目标

在 `api-server` 内新增 `assistant` 编排层，复用现有 LLM 配置、agent-runtime、业务 service、server gRPC、builder gRPC 和 repository。第一版不新增独立微服务。

V6.0 的智能体运行时必须继续使用 `github.com/alex-chenc/agent-runtime`。实现时参考现有 `api-server/internal/api/handler/ai_analysis_handler.go` 中 `adapters.NewAegisRuntime(...)` 和 `runtime.Run(ctx, agentruntime.TaskInput{...})` 的链路，把它通用化为 Assistant RuntimeFactory。

---

## 2. 目录结构

```text
api-server/internal/assistant/
  service.go
  orchestrator.go
  runtime_factory.go
  runtime_config.go
  runtime_events.go
  context_loader.go
  memory_service.go
  risk_policy.go
  approval_gate.go
  tool_policy_service.go
  tool_registry.go
  tool_catalog.go
  tool_selector.go
  intent_router.go
  tool_expansion.go
  tool_dispatcher.go
  tool_gateway.go
  prompt_provider.go
  result_card_builder.go
  sse_event.go
  tools/
    host_tools.go
    baseline_tools.go
    vulnerability_tools.go
    detection_tools.go
    detection_query_tools.go
    sigma_rule_tools.go
    block_tools.go
    package_tools.go
    config_tools.go
    audit_tools.go
    agent_tool_proxy.go

api-server/internal/api/handler/
  assistant_handler.go
  assistant_approval_handler.go

api-server/internal/model/
  assistant.go

api-server/internal/repository/
  assistant_session_repo.go
  assistant_message_repo.go
  assistant_context_ref_repo.go
  assistant_tool_call_repo.go
  assistant_approval_repo.go
  assistant_tool_policy_repo.go
  assistant_memory_repo.go
```

---

## 3. 核心模型

```go
package model

type AssistantSession struct {
    ID             uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
    SessionID      string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"session_id"`
    Title          string         `gorm:"type:varchar(255);not null" json:"title"`
    TaskType       string         `gorm:"type:varchar(40);not null;default:'explanation'" json:"task_type"`
    ModeSource     string         `gorm:"type:varchar(40);not null;default:'assistant'" json:"mode_source"`
    Status         string         `gorm:"type:varchar(32);not null;default:'active'" json:"status"`
    CreatedBy      string         `gorm:"type:varchar(100)" json:"created_by"`
    MessageCount   int            `gorm:"not null;default:0" json:"message_count"`
    ToolCallCount  int            `gorm:"not null;default:0" json:"tool_call_count"`
    ApprovalCount  int            `gorm:"not null;default:0" json:"approval_count"`
    LastMessageAt  *time.Time     `json:"last_message_at,omitempty"`
    Metadata       datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
    CreatedAt      time.Time      `gorm:"not null;default:now()" json:"created_at"`
    UpdatedAt      time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

type AssistantMessage struct {
    ID          uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
    SessionID   string         `gorm:"type:varchar(100);index;not null" json:"session_id"`
    MessageID   string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"message_id"`
    Role        string         `gorm:"type:varchar(20);not null" json:"role"`
    Content     string         `gorm:"type:text" json:"content"`
    Thinking    string         `gorm:"type:text" json:"thinking,omitempty"`
    Plan        datatypes.JSON `gorm:"type:jsonb" json:"plan,omitempty"`
    ToolCalls   datatypes.JSON `gorm:"type:jsonb" json:"tool_calls,omitempty"`
    Approvals   datatypes.JSON `gorm:"type:jsonb" json:"approvals,omitempty"`
    ResultCards datatypes.JSON `gorm:"type:jsonb" json:"result_cards,omitempty"`
    CreatedAt   time.Time      `gorm:"not null;default:now()" json:"created_at"`
}

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
```

---

## 4. Handler 设计

文件：`api-server/internal/api/handler/assistant_handler.go`

```go
type AssistantHandler struct {
    assistantService *assistant.Service
    approvalGate     *assistant.ApprovalGate
}

func NewAssistantHandler(
    assistantService *assistant.Service,
    approvalGate *assistant.ApprovalGate,
) *AssistantHandler

func (h *AssistantHandler) RegisterRoutes(group *gin.RouterGroup)
func (h *AssistantHandler) ListSessions(c *gin.Context)
func (h *AssistantHandler) CreateSession(c *gin.Context)
func (h *AssistantHandler) GetSession(c *gin.Context)
func (h *AssistantHandler) GetMessages(c *gin.Context)
func (h *AssistantHandler) SendMessage(c *gin.Context)
func (h *AssistantHandler) StreamSession(c *gin.Context)
func (h *AssistantHandler) CancelRun(c *gin.Context)
func (h *AssistantHandler) ListContextRefs(c *gin.Context)
func (h *AssistantHandler) ListToolCalls(c *gin.Context)
func (h *AssistantHandler) ListApprovals(c *gin.Context)
func (h *AssistantHandler) ListTools(c *gin.Context)
func (h *AssistantHandler) GetToolApprovalPolicy(c *gin.Context)
func (h *AssistantHandler) UpdateToolApprovalPolicy(c *gin.Context)
func (h *AssistantHandler) UpdateToolWhitelist(c *gin.Context)
func (h *AssistantHandler) BatchUpdateToolWhitelist(c *gin.Context)
func (h *AssistantHandler) ResetToolWhitelistDefaults(c *gin.Context)
```

文件：`api-server/internal/api/handler/assistant_approval_handler.go`

```go
func (h *AssistantHandler) Approve(c *gin.Context)
func (h *AssistantHandler) Reject(c *gin.Context)
func (h *AssistantHandler) GetApproval(c *gin.Context)
```

路由注册：

```go
assistantGroup := v1.Group("/assistant")
{
    assistantGroup.GET("/sessions", h.ListSessions)
    assistantGroup.POST("/sessions", h.CreateSession)
    assistantGroup.GET("/sessions/:session_id", h.GetSession)
    assistantGroup.GET("/sessions/:session_id/messages", h.GetMessages)
    assistantGroup.POST("/sessions/:session_id/message", h.SendMessage)
    assistantGroup.GET("/sessions/:session_id/stream", h.StreamSession)
    assistantGroup.POST("/sessions/:session_id/cancel", h.CancelRun)
    assistantGroup.GET("/sessions/:session_id/context-refs", h.ListContextRefs)
    assistantGroup.GET("/sessions/:session_id/tool-calls", h.ListToolCalls)
    assistantGroup.GET("/sessions/:session_id/approvals", h.ListApprovals)
    assistantGroup.GET("/tools", h.ListTools)
    assistantGroup.GET("/tool-approval-policy", h.GetToolApprovalPolicy)
    assistantGroup.PUT("/tool-approval-policy", h.UpdateToolApprovalPolicy)
    assistantGroup.PUT("/tools/:tool_name/whitelist", h.UpdateToolWhitelist)
    assistantGroup.POST("/tools/whitelist/batch", h.BatchUpdateToolWhitelist)
    assistantGroup.POST("/tools/whitelist/reset-defaults", h.ResetToolWhitelistDefaults)
    assistantGroup.GET("/approvals/:approval_id", h.GetApproval)
    assistantGroup.POST("/approvals/:approval_id/approve", h.Approve)
    assistantGroup.POST("/approvals/:approval_id/reject", h.Reject)
}
```

---

## 5. Assistant Service

```go
package assistant

type Service struct {
    sessionRepo    SessionRepository
    messageRepo    MessageRepository
    contextRepo    ContextRefRepository
    toolCallRepo   ToolCallRepository
    approvalRepo   ApprovalRepository
    memoryService  *MemoryService
    orchestrator   *Orchestrator
    contextLoader  *ContextLoader
    logger         *zap.Logger
}

func NewService(deps ServiceDeps) *Service

func (s *Service) ListSessions(ctx context.Context, q SessionQuery) ([]model.AssistantSession, int64, error)
func (s *Service) CreateSession(ctx context.Context, req CreateSessionRequest, operator string) (*model.AssistantSession, error)
func (s *Service) GetSession(ctx context.Context, sessionID string) (*model.AssistantSession, error)
func (s *Service) GetMessages(ctx context.Context, sessionID string) ([]model.AssistantMessage, error)
func (s *Service) AttachContextRefs(ctx context.Context, sessionID string, refs []ContextRefInput) ([]model.AssistantContextRef, error)
func (s *Service) SendMessage(ctx context.Context, sessionID string, req SendMessageRequest, operator string) (*RunHandle, error)
func (s *Service) Stream(ctx context.Context, sessionID string, writer EventWriter) error
func (s *Service) CancelRun(ctx context.Context, sessionID string, operator string) error
func (s *Service) CompleteRun(ctx context.Context, sessionID string, result RunResult) error
```

关键行为：

- `CreateSession` 创建会话，可同时绑定上下文对象。
- `SendMessage` 保存用户消息，创建异步 run。
- `Stream` 输出计划、步骤、工具、审批、结果。
- `CancelRun` 取消未完成的运行。

---

## 6. Orchestrator

```go
type Orchestrator struct {
    llmFactory      LLMFactory
    runtimeFactory  RuntimeFactory
    toolCatalog     *ToolCatalog
    toolSelector    *ToolSelector
    intentRouter    *IntentRouter
    riskPolicy      *RiskPolicy
    approvalGate    *ApprovalGate
    contextLoader   *ContextLoader
    memoryService   *MemoryService
    eventSink       EventSink
}

func (o *Orchestrator) Run(ctx context.Context, input RunInput) (*RunResult, error)
func (o *Orchestrator) buildRuntime(ctx context.Context, input RunInput) (*agentruntime.Runtime, error)
func (o *Orchestrator) buildPromptContext(ctx context.Context, input RunInput) (map[string]interface{}, error)
func (o *Orchestrator) handleToolRequest(ctx context.Context, req ToolRequest) (ToolResponse, error)
func (o *Orchestrator) emitPlan(plan AssistantPlan) error
func (o *Orchestrator) emitResultCards(cards []ResultCard) error
```

`Orchestrator` 不直接重写 ReAct 循环，主职责是准备 agent-runtime 所需输入：

1. 调 `ContextLoader` 加载页面对象、会话摘要、业务对象快照。
2. 调 `IntentRouter` 识别领域、动作、对象和风险。
3. 调 `ToolSelector` 从 `ToolCatalog` 检索本轮工具。
4. 调 `RuntimeFactory.Build` 创建 `agentruntime.Runtime`。
5. 调 `runtime.Run(ctx, agentruntime.TaskInput{...})`。
6. 处理 `Tool.Search` 触发的工具扩展，必要时启动第二段 runtime。

详细设计见 `agent_runtime_tool_orchestration_design_v6.0.md`。

`RunInput`：

```go
type RunInput struct {
    RunID       string
    SessionID   string
    MessageID   string
    UserID      string
    UserMessage string
    TaskType    string
    ContextRefs []model.AssistantContextRef
}
```

---

## 7. ToolCatalog、ToolSelector 与 Tool Registry

V6.0 中 `ToolRegistry` 只负责执行期按名称解析工具；全量工具元数据、检索索引和按需注入由 `ToolCatalog` 与 `ToolSelector` 负责。

```go
type ToolCatalog struct {
    byName map[string]ToolSpec
    index  *ToolIndex
}

type ToolSelector struct {
    catalog      *ToolCatalog
    intentRouter *IntentRouter
    roleRepo     RoleRepository
}

func (c *ToolCatalog) Register(spec ToolSpec) error
func (c *ToolCatalog) Search(ctx context.Context, q ToolSearchQuery) ([]ToolMatch, error)
func (c *ToolCatalog) BuildDescriptors(names []string) []agentruntime.ToolDescriptor

func (s *ToolSelector) Select(ctx context.Context, req ToolSelectionRequest) (ToolSelectionResult, error)
func (s *ToolSelector) Expand(ctx context.Context, current ToolSelectionResult, query string, names []string) (ToolSelectionResult, error)
```

规则：

- 所有 DB 查询接口都登记为 `ToolSpec`，但只读工具必须分页和摘要化。
- 每次 `agent-runtime` 只接收 `ToolSelector` 选出的 `[]agentruntime.ToolDescriptor`。
- `Tool.Search` 常驻，用于发现未注入工具；发现后由 Orchestrator 启动第二段 runtime 注入扩展工具。
- high/critical 工具只有明确执行意图时才进入本轮候选，即使进入候选也必须走 `ApprovalGate`。

```go
type ToolRegistry struct {
    tools map[string]ToolDescriptor
    mu    sync.RWMutex
}

type ToolDescriptor struct {
    Name             string
    Domain           string
    Description      string
    InputSchema      map[string]interface{}
    OutputSchema     map[string]interface{}
    RiskLevel        RiskLevel
    AutoCallable     bool
    RequiresApproval bool
    Idempotent       bool
    Timeout          time.Duration
    Handler          ToolHandler
}

type ToolHandler interface {
    Execute(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
    BuildApproval(ctx context.Context, req ToolExecutionRequest) (*ApprovalDraft, error)
}

func NewToolRegistry() *ToolRegistry
func (r *ToolRegistry) Register(tool ToolDescriptor) error
func (r *ToolRegistry) MustRegister(tool ToolDescriptor)
func (r *ToolRegistry) Get(name string) (ToolDescriptor, bool)
func (r *ToolRegistry) List(domain string) []ToolDescriptor
func (r *ToolRegistry) Execute(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
```

---

## 8. RiskPolicy 与 ApprovalGate

### 8.1 工具审批模式与白名单服务

智能体工具执行支持三种审批模式：

| 模式 | 配置值 | 行为 |
|:---|:---|:---|
| 请求批准 | `request_approval` | 所有工具调用都创建审批，审批执行成功后才继续 |
| 白名单 | `whitelist` | 白名单工具自动执行，非白名单工具创建审批 |
| 完全权限 | `full_access` | 所有已注册且被本轮注入的工具直接执行 |

```go
type ToolApprovalMode string

const (
    ApprovalModeRequestApproval ToolApprovalMode = "request_approval"
    ApprovalModeWhitelist       ToolApprovalMode = "whitelist"
    ApprovalModeFullAccess      ToolApprovalMode = "full_access"
)

type ToolPolicyService struct {
    catalog      *ToolCatalog
    policyRepo   ToolPolicyRepository
    systemConfig SystemConfigRepository
}

func NewToolPolicyService(deps ToolPolicyServiceDeps) *ToolPolicyService
func (s *ToolPolicyService) SyncCatalogTools(ctx context.Context) error
func (s *ToolPolicyService) ListTools(ctx context.Context, q ToolPolicyQuery) ([]ToolPolicyView, int64, error)
func (s *ToolPolicyService) GetApprovalMode(ctx context.Context) (ToolApprovalMode, error)
func (s *ToolPolicyService) UpdateApprovalMode(ctx context.Context, mode ToolApprovalMode, operator string) error
func (s *ToolPolicyService) IsWhitelisted(ctx context.Context, toolName string) (bool, error)
func (s *ToolPolicyService) UpdateWhitelist(ctx context.Context, toolName string, whitelisted bool, operator string) error
func (s *ToolPolicyService) BatchUpdateWhitelist(ctx context.Context, items []WhitelistUpdateItem, operator string) error
func (s *ToolPolicyService) ResetDefaultWhitelist(ctx context.Context, operator string) error
```

`SyncCatalogTools` 在服务启动时执行：

1. 遍历 `ToolCatalog` 全量工具。
2. 对 `assistant_tool_policies` 做 upsert。
3. 初始化默认低危白名单。
4. 不覆盖管理员已经手动修改的 `whitelisted`。

默认白名单原则：

- readonly 查询工具默认加入白名单。
- low 且无业务写入副作用的工具可以加入白名单。
- medium/high/critical 默认不加入白名单。
- 管理员可在配置页手动调整。

### 8.2 风险策略

```go
type RiskLevel string

const (
    RiskReadonly RiskLevel = "readonly"
    RiskLow      RiskLevel = "low"
    RiskMedium   RiskLevel = "medium"
    RiskHigh     RiskLevel = "high"
    RiskCritical RiskLevel = "critical"
)

type RiskDecision struct {
    Allow            bool
    RequiresApproval bool
    Mode             ToolApprovalMode
    RiskLevel        RiskLevel
    Reason           string
    ImpactSummary    string
    ApprovalTTL      time.Duration
}

type RiskPolicy struct {
    roleRepo     *repository.RoleRepo
    systemConfig *repository.SystemConfigRepo
}

func (p *RiskPolicy) Evaluate(ctx context.Context, user string, tool ToolDescriptor, args map[string]interface{}) (*RiskDecision, error)
func (p *RiskPolicy) isCriticalTool(toolName string) bool
func (p *RiskPolicy) requiresRole(user string, operation string) error
```

审批：

```go
type ApprovalGate struct {
    approvalRepo      ApprovalRepository
    toolCallRepo      ToolCallRepository
    registry          *ToolRegistry
    riskPolicy        *RiskPolicy
    toolPolicyService *ToolPolicyService
}

func (g *ApprovalGate) Evaluate(ctx context.Context, req ApprovalEvaluateRequest) (*RiskDecision, error)
func (g *ApprovalGate) CreateApproval(ctx context.Context, req CreateApprovalRequest) (*model.AssistantApproval, error)
func (g *ApprovalGate) Approve(ctx context.Context, approvalID string, operator string, comment string) (*ApprovalExecutionResult, error)
func (g *ApprovalGate) Reject(ctx context.Context, approvalID string, operator string, comment string) (*model.AssistantApproval, error)
func (g *ApprovalGate) ExecuteApprovedTool(ctx context.Context, approval *model.AssistantApproval) (*ToolExecutionResult, error)
func (g *ApprovalGate) expirePendingApprovals(ctx context.Context) error
```

`Evaluate` 逻辑：

```go
func (g *ApprovalGate) Evaluate(ctx context.Context, req ApprovalEvaluateRequest) (*RiskDecision, error) {
    mode, err := g.toolPolicyService.GetApprovalMode(ctx)
    if err != nil {
        return &RiskDecision{Allow: false, RequiresApproval: true, Reason: "approval mode unavailable"}, nil
    }

    decision, err := g.riskPolicy.Evaluate(ctx, req.Operator, req.Spec, req.Args)
    if err != nil {
        return nil, err
    }
    decision.Mode = mode

    switch mode {
    case ApprovalModeFullAccess:
        decision.Allow = true
        decision.RequiresApproval = false
        decision.Reason = "full_access mode"
    case ApprovalModeRequestApproval:
        decision.Allow = false
        decision.RequiresApproval = true
        decision.Reason = "request_approval mode"
    case ApprovalModeWhitelist:
        ok, _ := g.toolPolicyService.IsWhitelisted(ctx, req.ToolName)
        if ok {
            decision.Allow = true
            decision.RequiresApproval = false
            decision.Reason = "tool in whitelist"
            return decision, nil
        }
        decision.Allow = false
        decision.RequiresApproval = true
        decision.Reason = "tool not in whitelist"
    default:
        decision.Allow = false
        decision.RequiresApproval = true
        decision.Reason = "unknown approval mode"
    }
    return decision, nil
}
```

这里的 `Allow=false` 表示“不可直接执行”，不是拒绝业务操作；只要 `RequiresApproval=true`，就进入 `ApprovalGate.CreateApproval`，审批通过后再执行原工具。

---

## 9. ContextLoader

```go
type ContextLoader struct {
    hostRepo       *repository.HostRepository
    alertRepo      *repository.AlertRepository
    taskRepo       *repository.TaskLogRepository
    vulnRepo       *repository.VulnerabilityRepository
    packageRepo    *repository.DetectionPackageRepo
    auditLogRepo   *repository.AuditLogRepository
}

func (l *ContextLoader) Resolve(ctx context.Context, input ContextRefInput) (*model.AssistantContextRef, error)
func (l *ContextLoader) LoadSessionContext(ctx context.Context, sessionID string) (*AssistantContext, error)
func (l *ContextLoader) LoadHost(ctx context.Context, id string) (*ContextObject, error)
func (l *ContextLoader) LoadAlert(ctx context.Context, id string) (*ContextObject, error)
func (l *ContextLoader) LoadTask(ctx context.Context, id string) (*ContextObject, error)
func (l *ContextLoader) LoadVulnerability(ctx context.Context, id string) (*ContextObject, error)
func (l *ContextLoader) LoadPackage(ctx context.Context, id string) (*ContextObject, error)
func (l *ContextLoader) BuildRoutePath(objectType, objectID string) string
```

---

## 10. 工具实现清单

### 10.1 Host tools

```go
func RegisterHostTools(registry *ToolRegistry, deps HostToolDeps)

func HostListTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func HostGetDetailTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func HostSummarizePostureTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func HostFindOfflineAgentsTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
```

### 10.2 Vulnerability tools

```go
func RegisterVulnerabilityTools(registry *ToolRegistry, deps VulnerabilityToolDeps)

func VulnerabilityListTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func VulnerabilityStartScanTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func VulnerabilityAffectedHostsTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func VulnerabilityGenerateFixScriptTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func VulnerabilityGeneratePOCTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func VulnerabilityExecuteFixTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
```

风险：

- `List`、`AffectedHosts`: readonly。
- `StartScan`: medium。
- `GenerateFixScript`、`GeneratePOC`: low，但必须经过脚本审计。
- `ExecuteFix`: high。

### 10.3 Detection query / SigmaRule / Block tools

```go
func RegisterDetectionQueryTools(registry *ToolRegistry, deps DetectionQueryToolDeps)
func RegisterSigmaRuleTools(registry *ToolRegistry, deps SigmaRuleToolDeps)
func RegisterBlockTools(registry *ToolRegistry, deps BlockToolDeps)

func DetectionAlertListTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func DetectionAlertGetTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func DetectionStatisticsGetTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func DetectionTrendGetTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func SigmaRuleListTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func SigmaRuleGenerateTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func SigmaRuleStatusUpdateTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func BlockPolicyListTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func BlockPolicyUpdateTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func DetectionAlertBlockTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
```

### 10.4 Package tools

```go
func RegisterPackageTools(registry *ToolRegistry, deps PackageToolDeps)

func PackageListTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func PackageDraftGenerateTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func PackageDraftUpdateTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func PackageBuildStartTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func PackageBuildExplainFailureTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func PackageSignTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func PackageEnableTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func PackageDisableTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
```

风险：

- `GenerateDraft`: low。
- `UpdateDraft`: medium。
- `StartBuild`: medium。
- `Sign`: critical。
- `Enable`: critical。
- `Disable`: high。

### 10.5 Agent host tools

复用现有 `serverClient.ExecuteTool`：

```go
func RegisterAgentToolProxy(registry *ToolRegistry, deps AgentToolDeps)

func AgentGetProcessTreeTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func AgentGetNetworkConnectionsTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func AgentGetOpenFilesTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func AgentGetRunningProcessesTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func AgentGetUserSessionsTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func AgentQueryHistoricalLogsTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
```

---

## 11. ResultCard Builder

```go
type ResultCard struct {
    Type    string                 `json:"type"`
    Title   string                 `json:"title"`
    Payload map[string]interface{} `json:"payload"`
}

type ResultCardBuilder struct{}

func (b *ResultCardBuilder) HostList(hosts []model.Host) ResultCard
func (b *ResultCardBuilder) AlertList(alerts []model.Alert) ResultCard
func (b *ResultCardBuilder) TaskStatus(task *model.TaskLog) ResultCard
func (b *ResultCardBuilder) PackageSummary(pkg *model.DetectionPackage) ResultCard
func (b *ResultCardBuilder) AttackGraph(graph map[string]interface{}) ResultCard
func (b *ResultCardBuilder) Markdown(title, content string) ResultCard
```

---

## 12. 测试设计

| 测试文件 | 内容 |
|:---|:---|
| `assistant/service_test.go` | 创建会话、发送消息、绑定上下文 |
| `assistant/tool_registry_test.go` | 注册、查找、重复注册、执行工具 |
| `assistant/risk_policy_test.go` | 风险等级、默认白名单、审批模式覆盖规则 |
| `assistant/approval_gate_test.go` | 创建审批、批准、拒绝、执行 approved tool |
| `assistant/context_loader_test.go` | host/alert/task/package 上下文加载 |
| `handler/assistant_handler_test.go` | HTTP API 参数和响应 |
| `tools/package_tools_test.go` | Package.Sign/Enable 在需要审批的模式下拆成独立审批，full_access 下仍拆成独立工具调用 |
| `tools/vulnerability_tools_test.go` | ExecuteFix 在 request_approval 或 whitelist 非白名单状态下必须审批 |

---

## 13. 构建验证

实现后使用：

```bash
cd api-server && go test ./...
cd api-server && make build
docker compose up -d --build api-server frontend
curl -s http://localhost:8082/health
```

如涉及前端：

```bash
cd frontend
npm run build
```

