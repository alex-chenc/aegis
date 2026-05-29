# Aegis V6.0 后端开发文档: Assistant 智能体编排层

**版本**: 6.0  
**日期**: 2026-05-29  
**状态**: 设计中

---

## 1. 后端目标

在 `api-server` 内新增 `assistant` 编排层，复用现有 LLM 配置、agent-runtime、业务 service、server gRPC、builder gRPC 和 repository。第一版不新增独立微服务。

---

## 2. 目录结构

```text
api-server/internal/assistant/
  service.go
  orchestrator.go
  context_loader.go
  memory_service.go
  risk_policy.go
  approval_gate.go
  tool_registry.go
  tool_dispatcher.go
  prompt_provider.go
  result_card_builder.go
  sse_event.go
  tools/
    host_tools.go
    baseline_tools.go
    vulnerability_tools.go
    detection_tools.go
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
    toolRegistry    *ToolRegistry
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

## 7. Tool Registry

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
    Reason           string
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
    approvalRepo ApprovalRepository
    toolCallRepo ToolCallRepository
    registry     *ToolRegistry
    riskPolicy   *RiskPolicy
}

func (g *ApprovalGate) CreateApproval(ctx context.Context, req CreateApprovalRequest) (*model.AssistantApproval, error)
func (g *ApprovalGate) Approve(ctx context.Context, approvalID string, operator string, comment string) (*ApprovalExecutionResult, error)
func (g *ApprovalGate) Reject(ctx context.Context, approvalID string, operator string, comment string) (*model.AssistantApproval, error)
func (g *ApprovalGate) ExecuteApprovedTool(ctx context.Context, approval *model.AssistantApproval) (*ToolExecutionResult, error)
func (g *ApprovalGate) expirePendingApprovals(ctx context.Context) error
```

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

### 10.3 Detection tools

```go
func RegisterDetectionTools(registry *ToolRegistry, deps DetectionToolDeps)

func DetectionListAlertsTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func DetectionAnalyzeAlertsTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func DetectionGetAlertDetailTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func DetectionResolveAlertTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func DetectionBlockAlertTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func DetectionGenerateSigmaRuleTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
```

### 10.4 Package tools

```go
func RegisterPackageTools(registry *ToolRegistry, deps PackageToolDeps)

func PackageListTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func PackageGenerateDraftTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func PackageUpdateDraftTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func PackageStartBuildTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
func PackageExplainBuildFailureTool(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error)
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
| `assistant/risk_policy_test.go` | 风险等级、审批判断、critical 工具强制审批 |
| `assistant/approval_gate_test.go` | 创建审批、批准、拒绝、执行 approved tool |
| `assistant/context_loader_test.go` | host/alert/task/package 上下文加载 |
| `handler/assistant_handler_test.go` | HTTP API 参数和响应 |
| `tools/package_tools_test.go` | Package.Sign/Enable 必须审批 |
| `tools/vulnerability_tools_test.go` | ExecuteFix 必须审批 |

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

