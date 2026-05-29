# Aegis V6.0 实施蓝图: 双模智能体工具链落地

**版本**: 6.0  
**日期**: 2026-05-29  
**状态**: 开发落地设计  
**目标读者**: 后端负责人、前端负责人、全栈开发、测试开发

---

## 1. 本文目标

前几份文档定义“做什么”和“长什么样”。本文定义“开发人员怎么做”。

本文明确：

- 不做 MCP。
- 智能体只调用 Aegis 内部 Tool。
- Tool 是一层新写的适配函数，不重写业务逻辑。
- Tool 调用现有 `service/repository/gRPC`。
- 如果当前能力只在 handler 内，需要先抽 service，再给普通 handler 和 Tool 共用。
- 每条核心链路的函数顺序、文件位置、参数、返回值、风险判断、审批恢复执行方式。

---

## 2. 总体落地原则

### 2.1 禁止 MCP

V6.0 第一版不提供 MCP server，不引入 MCP 协议，不让外部智能体直接调用 Aegis。

统一调用路径：

```text
LLM / agent-runtime
  -> Assistant ToolRegistry
  -> Assistant ToolHandler
  -> Aegis existing service/repository/gRPC
  -> PostgreSQL / Redis / MinIO / server / builder / agent
```

### 2.2 Tool 层定位

Tool 层只做五件事：

1. 参数解析和校验。
2. 风险等级声明。
3. 调用现有业务函数。
4. 格式化智能体返回结果。
5. 为高风险动作生成审批摘要。

Tool 层不做：

- 不直接拼 SQL。
- 不复制业务逻辑。
- 不绕过审计。
- 不绕过现有 service。
- 不调用 HTTP handler。

### 2.3 Handler 逻辑下沉规则

如果某个普通页面功能只存在于 handler 方法中，没有 service 方法：

```text
错误做法:
Assistant Tool -> 调用 Handler 方法

正确做法:
1. 从 Handler 抽出业务逻辑到 Service
2. Handler 调 Service
3. Assistant Tool 也调同一个 Service
```

---

## 3. 开发阶段总览

| 阶段 | 后端目标 | 前端目标 | 可验收结果 |
|:---|:---|:---|:---|
| Phase 0 | 新增表、模型、repository | 无 | `assistant_*` 表可迁移，repo 单测通过 |
| Phase 1 | AssistantService、RunManager、SSE | `/assistant` 空工作台 | 可创建会话、发送消息、看到 SSE done |
| Phase 2 | ToolRegistry、只读工具 | 展示工具调用卡片 | 可查询主机、告警、任务、检测包 |
| Phase 3 | RiskPolicy、ApprovalGate | 审批卡片 | 高风险动作只生成审批，不直接执行 |
| Phase 4 | 写操作工具 | 审批后执行并展示结果 | 扫描、构建、阻断、启用等按策略执行 |
| Phase 5 | 普通页面接入智能体 | `AskAssistantButton` | 从主机/告警/任务/检测包进入智能模式 |

---

## 4. 需要新增和改造的文件

### 4.1 后端新增文件

```text
api-server/internal/model/assistant.go

api-server/internal/repository/assistant_session_repo.go
api-server/internal/repository/assistant_message_repo.go
api-server/internal/repository/assistant_context_ref_repo.go
api-server/internal/repository/assistant_tool_call_repo.go
api-server/internal/repository/assistant_approval_repo.go
api-server/internal/repository/assistant_memory_repo.go

api-server/internal/assistant/service.go
api-server/internal/assistant/run_manager.go
api-server/internal/assistant/orchestrator.go
api-server/internal/assistant/tool_registry.go
api-server/internal/assistant/tool_dispatcher.go
api-server/internal/assistant/risk_policy.go
api-server/internal/assistant/approval_gate.go
api-server/internal/assistant/context_loader.go
api-server/internal/assistant/memory_service.go
api-server/internal/assistant/event.go
api-server/internal/assistant/result_card_builder.go
api-server/internal/assistant/prompt_provider.go

api-server/internal/assistant/tools/host_tools.go
api-server/internal/assistant/tools/task_tools.go
api-server/internal/assistant/tools/vulnerability_tools.go
api-server/internal/assistant/tools/detection_tools.go
api-server/internal/assistant/tools/package_tools.go
api-server/internal/assistant/tools/config_tools.go
api-server/internal/assistant/tools/audit_tools.go
api-server/internal/assistant/tools/agent_tool_proxy.go

api-server/internal/api/handler/assistant_handler.go

migrations/015_v6.0_assistant_tables.sql
```

### 4.2 后端改造文件

```text
api-server/cmd/main.go
api-server/internal/repository/db.go
api-server/internal/api/router.go
```

可能需要抽 service 的文件：

```text
api-server/internal/api/handler/config_handler.go
api-server/internal/api/handler/host_handler.go
api-server/internal/api/handler/detection_handler.go
```

原则：已有 service 就用已有 service；没有 service 且逻辑较重，就补 service。

### 4.3 前端新增文件

```text
frontend/src/api/assistant.ts
frontend/src/store/assistant.ts
frontend/src/views/assistant/AssistantWorkspace.vue
frontend/src/views/assistant/components/AssistantSessionSidebar.vue
frontend/src/views/assistant/components/AssistantConversation.vue
frontend/src/views/assistant/components/AssistantComposer.vue
frontend/src/views/assistant/components/AssistantPlanPanel.vue
frontend/src/views/assistant/components/AssistantToolCallCard.vue
frontend/src/views/assistant/components/AssistantApprovalCard.vue
frontend/src/views/assistant/components/AssistantContextRail.vue
frontend/src/views/assistant/components/AssistantObjectCard.vue
frontend/src/views/assistant/components/AssistantResultRenderer.vue
frontend/src/views/assistant/composables/useAssistantStream.ts
frontend/src/components/assistant/AskAssistantButton.vue
```

### 4.4 前端改造文件

```text
frontend/src/router/index.ts
frontend/src/App.vue
frontend/src/views/Dashboard.vue
frontend/src/views/TaskDetail.vue
frontend/src/views/Vulnerability.vue
frontend/src/views/detection/Alerts.vue
frontend/src/views/detection/DetectionPackages/PackageDetail.vue
```

---

## 5. 后端依赖注入链

当前 `api-server/cmd/main.go` 已经初始化 repo、service、handler、router。V6.0 按这个模式继续扩展。

### 5.1 main.go 新增初始化顺序

放在现有 `auditLogRepo`、`systemConfigRepo` 初始化之后：

```go
assistantSessionRepo := repository.NewAssistantSessionRepo(db)
assistantMessageRepo := repository.NewAssistantMessageRepo(db)
assistantContextRefRepo := repository.NewAssistantContextRefRepo(db)
assistantToolCallRepo := repository.NewAssistantToolCallRepo(db)
assistantApprovalRepo := repository.NewAssistantApprovalRepo(db)
assistantMemoryRepo := repository.NewAssistantMemoryRepo(db)
```

放在现有业务 service 初始化之后：

```go
assistantContextLoader := assistant.NewContextLoader(assistant.ContextLoaderDeps{
    HostRepo: hostRepo,
    AlertRepo: alertRepo,
    TaskLogRepo: taskLogRepo,
    VulnerabilityService: vulnService,
    DetectionPackageService: detectionPkgService,
    AuditLogRepo: auditLogRepo,
})

assistantRiskPolicy := assistant.NewRiskPolicy(assistant.RiskPolicyDeps{
    RoleRepo: roleRepo,
    SystemConfigRepo: systemConfigRepo,
})

assistantToolRegistry := assistant.NewToolRegistry()
tools.RegisterHostTools(assistantToolRegistry, tools.HostToolDeps{
    HostRepo: hostRepo,
    ServerClient: serverClient,
})
tools.RegisterTaskTools(assistantToolRegistry, tools.TaskToolDeps{
    TaskLogRepo: taskLogRepo,
    TaskService: taskService,
})
tools.RegisterVulnerabilityTools(assistantToolRegistry, tools.VulnerabilityToolDeps{
    VulnerabilityService: vulnService,
    HostScriptService: hostVulnerabilityScriptService,
})
tools.RegisterDetectionTools(assistantToolRegistry, tools.DetectionToolDeps{
    AlertRepo: alertRepo,
    AlertService: alertService,
    SigmaRuleService: sigmaRuleService,
})
tools.RegisterPackageTools(assistantToolRegistry, tools.PackageToolDeps{
    DetectionPackageService: detectionPkgService,
})
tools.RegisterAgentToolProxy(assistantToolRegistry, tools.AgentToolDeps{
    ServerClient: serverClient,
})

assistantRunManager := assistant.NewRunManager()

assistantApprovalGate := assistant.NewApprovalGate(assistant.ApprovalGateDeps{
    ApprovalRepo: assistantApprovalRepo,
    ToolCallRepo: assistantToolCallRepo,
    ToolRegistry: assistantToolRegistry,
    RiskPolicy: assistantRiskPolicy,
})

assistantOrchestrator := assistant.NewOrchestrator(assistant.OrchestratorDeps{
    ConfigRepo: configRepo,
    ToolRegistry: assistantToolRegistry,
    RiskPolicy: assistantRiskPolicy,
    ApprovalGate: assistantApprovalGate,
    ContextLoader: assistantContextLoader,
    MessageRepo: assistantMessageRepo,
    ToolCallRepo: assistantToolCallRepo,
    RunManager: assistantRunManager,
    LLMTimeoutSeconds: cfg.LLM.TimeoutSeconds,
    LLMMaxRetries: cfg.LLM.MaxRetries,
})

assistantService := assistant.NewService(assistant.ServiceDeps{
    SessionRepo: assistantSessionRepo,
    MessageRepo: assistantMessageRepo,
    ContextRefRepo: assistantContextRefRepo,
    ToolCallRepo: assistantToolCallRepo,
    ApprovalRepo: assistantApprovalRepo,
    MemoryRepo: assistantMemoryRepo,
    ContextLoader: assistantContextLoader,
    Orchestrator: assistantOrchestrator,
    RunManager: assistantRunManager,
})

assistantHandler := handler.NewAssistantHandler(assistantService, assistantApprovalGate)
```

最后把 `assistantHandler` 加入 `api.NewRouter(...)` 构造函数。

### 5.2 repository/db.go AutoMigrate

在 `repository.NewDB` 的 `AutoMigrate` 加入：

```go
&model.AssistantSession{},
&model.AssistantMessage{},
&model.AssistantContextRef{},
&model.AssistantToolCall{},
&model.AssistantApproval{},
&model.AssistantMemory{},
```

同时保留 SQL migration，AutoMigrate 只是开发期兜底。

### 5.3 router.go 改造

`Router` 增加字段：

```go
assistantHandler *handler.AssistantHandler
```

`NewRouter` 增加参数：

```go
assistantHandler *handler.AssistantHandler
```

`Setup` 中增加：

```go
assistantGroup := v1.Group("/assistant")
{
    r.assistantHandler.RegisterRoutes(assistantGroup)
}
```

---

## 6. 核心链路 1: 创建智能体会话

### 6.1 调用链

```text
frontend AssistantWorkspace.handleNewSession()
  -> assistantApi.createSession()
  -> POST /api/v1/assistant/sessions
  -> AssistantHandler.CreateSession()
  -> AssistantService.CreateSession()
  -> ContextLoader.Resolve() for each context ref
  -> AssistantSessionRepo.Create()
  -> AssistantContextRefRepo.UpsertMany()
  -> AssistantMessageRepo.Create() if initial_message exists
  -> return session
```

### 6.2 Handler 函数

```go
func (h *AssistantHandler) CreateSession(c *gin.Context) {
    var req assistant.CreateSessionRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
        return
    }

    operator := middleware.GetUsername(c)
    session, err := h.assistantService.CreateSession(c.Request.Context(), req, operator)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"code": 0, "data": session})
}
```

### 6.3 Service 函数

```go
func (s *Service) CreateSession(ctx context.Context, req CreateSessionRequest, operator string) (*model.AssistantSession, error) {
    sessionID := newAssistantSessionID()
    title := inferTitle(req.Title, req.InitialMessage, req.ContextRefs)

    session := &model.AssistantSession{
        ID: uuid.New(),
        SessionID: sessionID,
        Title: title,
        TaskType: defaultTaskType(req.TaskType),
        ModeSource: inferModeSource(req.ContextRefs),
        Status: "active",
        CreatedBy: operator,
    }

    if err := s.sessionRepo.Create(ctx, session); err != nil {
        return nil, err
    }

    refs, err := s.AttachContextRefs(ctx, sessionID, req.ContextRefs)
    if err != nil {
        return nil, err
    }

    if strings.TrimSpace(req.InitialMessage) != "" {
        msg := newUserMessage(sessionID, req.InitialMessage)
        if err := s.messageRepo.Create(ctx, msg); err != nil {
            return nil, err
        }
        _ = s.sessionRepo.IncrementMessageCount(ctx, sessionID)
    }

    session.Metadata = buildSessionMetadata(refs)
    return session, s.sessionRepo.Update(ctx, session)
}
```

---

## 7. 核心链路 2: 发送消息并启动 Run

### 7.1 设计决策

`SendMessage` 不阻塞等待模型完成。它只保存用户消息并启动后台 run，然后返回 `run_id`。前端通过 SSE 接收过程。

### 7.2 调用链

```text
AssistantComposer.submit()
  -> assistantStore.sendMessage()
  -> assistantApi.sendMessage()
  -> POST /api/v1/assistant/sessions/:session_id/message
  -> AssistantHandler.SendMessage()
  -> AssistantService.SendMessage()
  -> MessageRepo.Create(user)
  -> RunManager.Start(sessionID, runID)
  -> goroutine Orchestrator.Run()
  -> return {message_id, run_id}
```

### 7.3 Service 函数

```go
func (s *Service) SendMessage(ctx context.Context, sessionID string, req SendMessageRequest, operator string) (*RunHandle, error) {
    session, err := s.sessionRepo.FindBySessionID(ctx, sessionID)
    if err != nil {
        return nil, err
    }
    if session.Status == "running" {
        return nil, ErrSessionAlreadyRunning
    }

    if len(req.ContextRefs) > 0 {
        if _, err := s.AttachContextRefs(ctx, sessionID, req.ContextRefs); err != nil {
            return nil, err
        }
    }

    userMsg := &model.AssistantMessage{
        ID: uuid.New(),
        SessionID: sessionID,
        MessageID: newMessageID(),
        Role: "user",
        Content: req.Content,
    }
    if err := s.messageRepo.Create(ctx, userMsg); err != nil {
        return nil, err
    }

    run := s.runManager.Start(sessionID)
    _ = s.sessionRepo.UpdateStatus(ctx, sessionID, "running")

    go func() {
        runCtx := run.Context()
        result, err := s.orchestrator.Run(runCtx, RunInput{
            RunID: run.RunID,
            SessionID: sessionID,
            MessageID: userMsg.MessageID,
            UserID: operator,
            UserMessage: req.Content,
        })
        s.completeRun(context.Background(), sessionID, run.RunID, result, err)
    }()

    return &RunHandle{RunID: run.RunID, MessageID: userMsg.MessageID}, nil
}
```

---

## 8. 核心链路 3: SSE 事件流

### 8.1 调用链

```text
frontend openAssistantStream(sessionID)
  -> GET /api/v1/assistant/sessions/:session_id/stream
  -> AssistantHandler.StreamSession()
  -> AssistantService.Stream()
  -> RunManager.Subscribe(sessionID)
  -> writer.Write(event)
```

### 8.2 RunManager

```go
type RunManager struct {
    mu   sync.RWMutex
    runs map[string]*ActiveRun // key: sessionID
}

type ActiveRun struct {
    RunID    string
    SessionID string
    ctx      context.Context
    cancel   context.CancelFunc
    events   chan AssistantEvent
    startedAt time.Time
}

func NewRunManager() *RunManager
func (m *RunManager) Start(sessionID string) *ActiveRun
func (m *RunManager) Get(sessionID string) (*ActiveRun, bool)
func (m *RunManager) Publish(sessionID string, event AssistantEvent)
func (m *RunManager) Subscribe(sessionID string) (<-chan AssistantEvent, func(), error)
func (m *RunManager) Cancel(sessionID string) bool
func (m *RunManager) Finish(sessionID string)
```

### 8.3 EventWriter

```go
type EventWriter interface {
    Write(event AssistantEvent) error
}

type AssistantEvent struct {
    Type      string      `json:"type"`
    SessionID string      `json:"session_id"`
    RunID     string      `json:"run_id,omitempty"`
    MessageID string      `json:"message_id,omitempty"`
    Payload   interface{} `json:"payload,omitempty"`
    Error     string      `json:"error,omitempty"`
}
```

### 8.4 Stream 函数

```go
func (s *Service) Stream(ctx context.Context, sessionID string, writer EventWriter) error {
    ch, unsubscribe, err := s.runManager.Subscribe(sessionID)
    if err != nil {
        return err
    }
    defer unsubscribe()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case event, ok := <-ch:
            if !ok {
                return nil
            }
            if err := writer.Write(event); err != nil {
                return err
            }
            if event.Type == "done" || event.Type == "error" {
                return nil
            }
        }
    }
}
```

---

## 9. 核心链路 4: Orchestrator 执行

### 9.1 调用链

```text
Orchestrator.Run()
  -> load LLM config from ConfigRepository.GetActive()
  -> create llm.LLMClient
  -> load messages from AssistantMessageRepo
  -> load context from ContextLoader.LoadSessionContext()
  -> load memory from MemoryService
  -> build prompt
  -> agent-runtime plan/execute
  -> ToolDispatcher.Execute() for each tool call
  -> collect final answer
  -> persist assistant message
  -> emit done
```

### 9.2 Orchestrator 主函数

```go
func (o *Orchestrator) Run(ctx context.Context, input RunInput) (*RunResult, error) {
    o.emit(input.SessionID, EventThinking("正在加载会话上下文"))

    llmClient, err := o.createLLMClient(ctx)
    if err != nil {
        return nil, err
    }

    messages, err := o.messageRepo.ListBySession(ctx, input.SessionID)
    if err != nil {
        return nil, err
    }

    assistantCtx, err := o.contextLoader.LoadSessionContext(ctx, input.SessionID)
    if err != nil {
        return nil, err
    }

    runtime, err := o.buildRuntime(ctx, llmClient, assistantCtx)
    if err != nil {
        return nil, err
    }

    result, err := runtime.Run(ctx, buildRuntimeInput(input, messages, assistantCtx))
    if err != nil {
        o.emit(input.SessionID, EventError(err.Error()))
        return nil, err
    }

    assistantMsg := o.buildAssistantMessage(input.SessionID, result)
    if err := o.messageRepo.Create(ctx, assistantMsg); err != nil {
        return nil, err
    }

    o.emit(input.SessionID, EventDone(input.RunID))
    return &RunResult{MessageID: assistantMsg.MessageID, FinalAnswer: assistantMsg.Content}, nil
}
```

### 9.3 agent-runtime ToolGateway 适配

不要让 agent-runtime 直接知道业务 service。它只知道一个 ToolGateway：

```go
type RuntimeToolGateway struct {
    dispatcher *ToolDispatcher
    sessionID  string
    userID     string
}

func (g *RuntimeToolGateway) Call(ctx context.Context, req agentruntime.ToolRequest) (agentruntime.ToolResponse, error) {
    result, err := g.dispatcher.Execute(ctx, ToolExecutionRequest{
        SessionID: g.sessionID,
        CallID: req.CallID,
        ToolName: req.ToolName,
        Args: req.Args,
        Operator: g.userID,
        Source: "assistant",
    })
    return toRuntimeToolResponse(req, result, err), nil
}
```

---

## 10. 核心链路 5: ToolDispatcher 执行和审批

### 10.1 调用链

```text
RuntimeToolGateway.Call()
  -> ToolDispatcher.Execute()
  -> ToolRegistry.Get(toolName)
  -> RiskPolicy.Evaluate()
  -> ToolCallRepo.Create(status=running or approval_required)
  -> if approval required:
       tool.BuildApproval()
       ApprovalGate.CreateApproval()
       emit approval_required
       return approval_required result
     else:
       tool.Execute()
       ToolCallRepo.MarkSuccess/MarkFailed
       emit tool_result/tool_error
```

### 10.2 ToolExecutionRequest

```go
type ToolExecutionRequest struct {
    SessionID string
    MessageID string
    CallID    string
    ToolName  string
    Args      map[string]interface{}
    Operator  string
    Source    string
    Approved  bool
}

func (r ToolExecutionRequest) StringArg(name string) (string, error)
func (r ToolExecutionRequest) StringSliceArg(name string) ([]string, error)
func (r ToolExecutionRequest) IntArg(name string, def int) int
func (r ToolExecutionRequest) BoolArg(name string, def bool) bool
```

### 10.3 ToolExecutionResult

```go
type ToolExecutionResult struct {
    Success     bool
    Summary     string
    Data        interface{}
    ResultCards []ResultCard
    Error       string
    ApprovalID  string
}
```

### 10.4 Dispatcher 函数

```go
func (d *ToolDispatcher) Execute(ctx context.Context, req ToolExecutionRequest) (*ToolExecutionResult, error) {
    tool, ok := d.registry.Get(req.ToolName)
    if !ok {
        return nil, fmt.Errorf("unknown tool: %s", req.ToolName)
    }

    decision, err := d.riskPolicy.Evaluate(ctx, req.Operator, tool, req.Args)
    if err != nil {
        return nil, err
    }

    call := newToolCallModel(req, tool, decision)
    if err := d.toolCallRepo.Create(ctx, call); err != nil {
        return nil, err
    }

    if decision.RequiresApproval && !req.Approved {
        draft, err := tool.Handler.BuildApproval(ctx, req)
        if err != nil {
            _ = d.toolCallRepo.MarkFailed(ctx, req.CallID, err.Error())
            return nil, err
        }
        approval, err := d.approvalGate.CreateApproval(ctx, CreateApprovalRequest{
            ToolCallID: req.CallID,
            SessionID: req.SessionID,
            ToolName: req.ToolName,
            RiskLevel: tool.RiskLevel,
            Draft: draft,
            RequestedBy: req.Operator,
        })
        if err != nil {
            _ = d.toolCallRepo.MarkFailed(ctx, req.CallID, err.Error())
            return nil, err
        }
        _ = d.toolCallRepo.MarkApprovalRequired(ctx, req.CallID, approval.ApprovalID)
        d.emit(req.SessionID, EventApprovalRequired(approval))
        return &ToolExecutionResult{
            Success: false,
            Summary: "该操作需要人工审批",
            ApprovalID: approval.ApprovalID,
        }, nil
    }

    started := time.Now()
    result, err := tool.Handler.Execute(ctx, req)
    duration := time.Since(started).Milliseconds()
    if err != nil {
        _ = d.toolCallRepo.MarkFailed(ctx, req.CallID, err.Error(), duration)
        d.emit(req.SessionID, EventToolError(req.CallID, err.Error()))
        return nil, err
    }

    _ = d.toolCallRepo.MarkSuccess(ctx, req.CallID, result, duration)
    d.emit(req.SessionID, EventToolResult(req.CallID, result))
    return result, nil
}
```

---

## 11. 核心链路 6: 审批后恢复执行

### 11.1 调用链

```text
AssistantApprovalCard.approve()
  -> POST /api/v1/assistant/approvals/:approval_id/approve
  -> AssistantHandler.Approve()
  -> ApprovalGate.Approve()
  -> ApprovalRepo.FindByApprovalID()
  -> ToolCallRepo.FindByCallID()
  -> ToolRegistry.Get(toolName)
  -> ToolHandler.Execute(req Approved=true)
  -> ApprovalRepo.MarkExecuted()
  -> ToolCallRepo.MarkSuccess()
```

### 11.2 ApprovalGate.Approve

```go
func (g *ApprovalGate) Approve(ctx context.Context, approvalID string, operator string, comment string) (*ApprovalExecutionResult, error) {
    approval, err := g.approvalRepo.FindByApprovalID(ctx, approvalID)
    if err != nil {
        return nil, err
    }
    if approval.Status != "pending" {
        return nil, ErrApprovalNotPending
    }

    call, err := g.toolCallRepo.FindByCallID(ctx, approval.ToolCallID)
    if err != nil {
        return nil, err
    }

    tool, ok := g.registry.Get(call.ToolName)
    if !ok {
        return nil, fmt.Errorf("tool not found: %s", call.ToolName)
    }

    if err := g.riskPolicy.CheckApprovalPermission(ctx, operator, tool); err != nil {
        return nil, err
    }

    _ = g.approvalRepo.MarkApproved(ctx, approvalID, operator, comment)

    req := ToolExecutionRequest{
        SessionID: call.SessionID,
        MessageID: call.MessageID,
        CallID: call.CallID,
        ToolName: call.ToolName,
        Args: jsonToMap(call.Args),
        Operator: operator,
        Source: "approval",
        Approved: true,
    }

    result, err := tool.Handler.Execute(ctx, req)
    if err != nil {
        _ = g.approvalRepo.MarkFailed(ctx, approvalID, err.Error())
        _ = g.toolCallRepo.MarkFailed(ctx, call.CallID, err.Error())
        return nil, err
    }

    _ = g.toolCallRepo.MarkSuccess(ctx, call.CallID, result, 0)
    _ = g.approvalRepo.MarkExecuted(ctx, approvalID)
    return &ApprovalExecutionResult{Approval: approval, ToolResult: result}, nil
}
```

---

## 12. 工具实现映射: 现有函数怎么接

### 12.1 Host 工具

| Tool | 风险 | 调用现有函数 | 缺口 |
|:---|:---|:---|:---|
| `Host.List` | readonly | `hostRepo.FindAll(page,pageSize,query)` + `hostRepo.Count(query)` | 无 |
| `Host.GetDetail` | readonly | `hostRepo.FindByID(uuid)` + `serverClient.GetAgentStatus(ctx, hostID)` | 无 |
| `Host.FindOfflineAgents` | readonly | `hostRepo.FindAll` 后按 heartbeat/status 判断 | 可后续补 repo 查询优化 |
| `Host.SummarizePosture` | readonly | `hostRepo.Count` + `serverClient.ListConnectedAgents` | 无 |

函数骨架：

```go
type HostListTool struct {
    hostRepo *repository.HostRepository
}

func (t *HostListTool) Execute(ctx context.Context, req assistant.ToolExecutionRequest) (*assistant.ToolExecutionResult, error) {
    page := req.IntArg("page", 1)
    pageSize := req.IntArg("page_size", 20)
    query, _ := req.OptionalStringArg("query")

    hosts, err := t.hostRepo.FindAll(page, pageSize, query)
    if err != nil {
        return nil, err
    }
    total, err := t.hostRepo.Count(query)
    if err != nil {
        return nil, err
    }

    return &assistant.ToolExecutionResult{
        Success: true,
        Summary: fmt.Sprintf("查询到 %d 台主机", total),
        Data: map[string]interface{}{"data": hosts, "total": total},
        ResultCards: []assistant.ResultCard{assistant.NewHostListCard(hosts, total)},
    }, nil
}
```

### 12.2 Task / Baseline 工具

| Tool | 风险 | 调用现有函数 | 缺口 |
|:---|:---|:---|:---|
| `Task.List` | readonly | `taskLogRepo.ListTaskGroups(params)` + `CountTaskGroups` | 无 |
| `Task.GetDetail` | readonly | `taskLogRepo.FindByID(id)` 或 `FindByGroupID(groupID)` | 无 |
| `Task.Redispatch` | medium | `taskService.RedispatchTask(ctx, id)` | 需要审批策略 |
| `Baseline.RunCheck` | medium | `taskService.CreateAndDispatchTasks(ctx, ruleIDs, hostIDs, "check")` | 需要 rule 选择工具 |
| `Baseline.RunFix` | high | `taskService.CreateAndDispatchTasks(ctx, ruleIDs, hostIDs, "fix")` | 必须审批 |

### 12.3 Vulnerability 工具

| Tool | 风险 | 调用现有函数 | 说明 |
|:---|:---|:---|:---|
| `Vulnerability.List` | readonly | `vulnService.ListVulnerabilities(params)` | 查询漏洞 |
| `Vulnerability.StartScan` | medium | `vulnService.StartScan(ctx, hostIDs)` | 创建扫描 |
| `Vulnerability.StopScan` | medium | `vulnService.StopScan(ctx)` | 停止扫描 |
| `Vulnerability.GetScanStatus` | readonly | `vulnService.GetScanStatus(scanID)` | 查询扫描 |
| `Vulnerability.AffectedHosts` | readonly | `vulnService.GetVulnerabilityByCveID(cveID)` + `GetAffectedHosts(vuln.ID)` | 两步 |
| `Vulnerability.GenerateFixScript` | low | `hostVulnerabilityScriptService.GenerateScripts(ctx,cveID,hostIDs,"fix")` | 只生成 |
| `Vulnerability.GeneratePOC` | low | `hostVulnerabilityScriptService.GenerateScripts(ctx,cveID,hostIDs,"poc")` | 只生成 |
| `Vulnerability.ExecuteFix` | high | `hostVulnerabilityScriptService.ExecuteScripts(ctx,cveID,"fix",hostIDs)` | 必须审批 |
| `Vulnerability.ExecutePOC` | high | `hostVulnerabilityScriptService.ExecuteScripts(ctx,cveID,"poc",hostIDs)` | 必须审批 |

关键实现：

```go
func (t *VulnerabilityExecuteFixTool) BuildApproval(ctx context.Context, req assistant.ToolExecutionRequest) (*assistant.ApprovalDraft, error) {
    cveID := req.MustStringArg("cve_id")
    hostIDs := req.MustStringSliceArg("host_ids")
    return &assistant.ApprovalDraft{
        Title: "执行漏洞修复脚本",
        ImpactSummary: fmt.Sprintf("将在 %d 台主机上执行 %s 的修复脚本", len(hostIDs), cveID),
        RollbackHint: "修复脚本执行不可自动回滚，请先在测试主机验证",
        ParamsPreview: req.Args,
    }, nil
}
```

### 12.4 Detection 工具

| Tool | 风险 | 调用现有函数 | 说明 |
|:---|:---|:---|:---|
| `Detection.ListAlerts` | readonly | `alertRepo.List(page,pageSize,filters)` | 告警列表 |
| `Detection.GetAlert` | readonly | `alertRepo.FindByID(id)` | 告警详情 |
| `Detection.ResolveAlert` | medium | `alertService.Resolve(alertID)` | 标记解决 |
| `Detection.BlockAlert` | critical | `alertService.ManualBlock(alertID, action)` | 必须审批 |
| `Detection.ListBlockRecords` | readonly | `blockRepo` 对应列表方法，如缺失补 repo | 需要检查现有 repo |
| `Detection.GenerateSigmaRule` | medium | `ruleGenerationService` 或 `sigmaRuleService` | 需抽明确 service 方法 |

`Detection.BlockAlert` 审批必须展示：

- alert_id。
- host_id / hostname。
- action。
- target。
- 是否已有 block record。
- 回滚提示。

### 12.5 AgentTool 工具

复用 [api-server/internal/grpc/client.go](/Users/chenchen/Documents/code/aegis/api-server/internal/grpc/client.go) 中：

```go
serverClient.ExecuteTool(ctx, callID, hostID, tool, arguments, timeoutSeconds)
```

映射：

| Assistant Tool | Agent Tool |
|:---|:---|
| `AgentTool.GetProcessTree` | `GetProcessTree` |
| `AgentTool.GetNetworkConnections` | `GetNetworkConnections` |
| `AgentTool.GetOpenFiles` | `GetOpenFiles` |
| `AgentTool.GetRunningProcesses` | `GetRunningProcesses` |
| `AgentTool.GetUserSessions` | `GetUserSessions` |
| `AgentTool.QueryHistoricalLogs` | `QueryHistoricalLogs` |

函数骨架：

```go
func (t *AgentToolProxy) Execute(ctx context.Context, req assistant.ToolExecutionRequest) (*assistant.ToolExecutionResult, error) {
    hostID := req.MustStringArg("host_id")
    agentToolName := strings.TrimPrefix(req.ToolName, "AgentTool.")
    argsJSON := marshalArgs(req.Args)

    resp, err := t.serverClient.ExecuteTool(ctx, req.CallID, hostID, agentToolName, argsJSON, 60)
    if err != nil {
        return nil, err
    }
    if !resp.Success {
        return nil, fmt.Errorf(resp.Error)
    }

    return &assistant.ToolExecutionResult{
        Success: true,
        Summary: fmt.Sprintf("%s 执行成功", agentToolName),
        Data: jsonRawOrText(resp.Result),
    }, nil
}
```

### 12.6 DetectionPackage 工具

直接复用 [api-server/internal/service/detection_package_service.go](/Users/chenchen/Documents/code/aegis/api-server/internal/service/detection_package_service.go)。

| Tool | 风险 | 调用现有函数 |
|:---|:---|:---|
| `Package.List` | readonly | `detectionPkgService.ListPackages(ctx,page,pageSize,status,search)` |
| `Package.Get` | readonly | `detectionPkgService.GetPackage(ctx,packageID)` |
| `Package.GetDraft` | readonly | `detectionPkgService.GetDraft(ctx,packageID)` |
| `Package.CreateDraft` | low | `detectionPkgService.CreateDraft(ctx,req,operator)` |
| `Package.UpdateDraft` | medium | `detectionPkgService.UpdateDraft(ctx,draftID,req,operator)` |
| `Package.StartBuild` | medium | `detectionPkgService.StartBuild(ctx,packageID,operator)` |
| `Package.GetBuild` | readonly | `detectionPkgService.GetBuild(ctx,buildID)` |
| `Package.GetBuildLog` | readonly | `detectionPkgService.GetBuildLogURL(ctx,buildID)` |
| `Package.ReviewBuild` | high | `detectionPkgService.ReviewBuild(ctx,buildID,approved,comment,operator)` |
| `Package.Sign` | critical | `detectionPkgService.SignPackage(ctx,packageID,operator)` |
| `Package.Enable` | critical | `detectionPkgService.EnablePackage(ctx,packageID,operator)` |
| `Package.Disable` | high | `detectionPkgService.DisablePackage(ctx,packageID,operator)` |
| `Package.Rollback` | critical | `detectionPkgService.RollbackPackage(ctx,packageID,targetVersion,operator)` |
| `Package.UpdateAllowlist` | critical | `detectionPkgService.UpdateAllowlist(ctx,configJSON,description,operator)` |

注意：

- `Package.Sign` 和 `Package.Enable` 必须是两个独立审批。
- 智能体不能把“签名并启用”合成一个直接执行动作。
- `StartBuild` 可按系统配置决定是否需要审批，默认 medium 需要确认。

### 12.7 Config 工具

当前配置测试主要在 `ConfigHandler`。不要从 Tool 调 Handler。需要抽服务：

新增：

```text
api-server/internal/service/config_service.go
```

函数：

```go
type ConfigService struct {
    configRepo *repository.ConfigRepository
}

func (s *ConfigService) GetLLMConfig(ctx context.Context) (*model.LLMConfig, error)
func (s *ConfigService) TestLLMConnection(ctx context.Context, req TestLLMConnectionRequest) error
func (s *ConfigService) GetImageModelConfig(ctx context.Context) (*model.ImageModelConfig, error)
func (s *ConfigService) TestImageModelConnection(ctx context.Context, req TestImageModelConnectionRequest) error
```

普通 `ConfigHandler` 和 `Config.*` Tool 都调用 `ConfigService`。

---

## 13. Prompt 与工具说明

### 13.1 系统 Prompt 目标

智能体必须知道：

- 它是 Aegis 安全运营智能体。
- 它只能调用注册工具。
- 只读工具可以自动用。
- 高风险工具必须请求审批。
- DetectionPackage 签名和启用不能自动执行。
- 不知道对象 ID 时先查询或追问。
- 所有用户可见文本用中文。

### 13.2 PromptProvider 函数

```go
type PromptProvider struct {
    registry *ToolRegistry
}

func (p *PromptProvider) BuildSystemPrompt(ctx AssistantContext) string
func (p *PromptProvider) BuildToolCatalogPrompt() string
func (p *PromptProvider) BuildRiskPolicyPrompt() string
func (p *PromptProvider) BuildContextPrompt(ctx AssistantContext) string
```

---

## 14. 前端开发链路

### 14.1 创建 `/assistant` 工作台

步骤：

1. 新建 `frontend/src/api/assistant.ts`。
2. 新建 `frontend/src/store/assistant.ts`。
3. 新建 `AssistantWorkspace.vue`。
4. 在 `router/index.ts` 注册 `/assistant`。
5. 在 `App.vue` 侧边栏和顶部加入智能模式入口。

### 14.2 发送消息前端链

```text
AssistantComposer.submit()
  -> assistantStore.sendMessage(content)
  -> assistantApi.sendMessage(sessionId, {content})
  -> store.openStream(sessionId)
  -> useAssistantStream.onEvent()
  -> store.applyStreamEvent(event)
  -> Conversation 组件响应式渲染
```

### 14.3 审批前端链

```text
AssistantApprovalCard.approve()
  -> ElMessageBox.confirm()
  -> assistantApi.approve(approvalId)
  -> store.upsertApproval()
  -> store.upsertToolCall()
  -> result card 展示执行结果
```

### 14.4 普通页面入口链

```text
AskAssistantButton.click()
  -> router.push({
       path: '/assistant',
       query: {
         object_type: 'alert',
         object_ids: 'id1,id2',
         prompt: '对这些告警做攻击溯源'
       }
     })
  -> AssistantWorkspace.createSessionFromRouteContext()
  -> assistantApi.createSession({context_refs, initial_message})
```

---

## 15. 端到端样例链路

### 15.1 “分析最近 24 小时高危告警”

```text
User Message
  -> Orchestrator
  -> Tool Detection.ListAlerts
       alertRepo.List(page=1,pageSize=50,filters={severity:["high","critical"],time_range:24h})
  -> Tool AgentTool.GetProcessTree for selected alert PID
       serverClient.ExecuteTool()
  -> Tool AgentTool.QueryHistoricalLogs
       serverClient.ExecuteTool()
  -> ResultCard AttackGraph
  -> AssistantMessageRepo.Create(assistant final answer)
```

### 15.2 “扫描这几台主机并生成修复脚本”

```text
User Message
  -> Host.GetDetail 确认主机存在
  -> Vulnerability.StartScan medium
       RiskPolicy requires approval if assistant.require_approval_medium=true
       ApprovalGate.CreateApproval
  -> user approve
  -> vulnService.StartScan(ctx, hostIDs)
  -> Vulnerability.GetScanStatus
  -> Vulnerability.GenerateFixScript low
       hostVulnerabilityScriptService.GenerateScripts(ctx,cveID,hostIDs,"fix")
  -> return script status card
```

### 15.3 “为 CVE 生成检测包，构建并启用”

```text
User Message
  -> Package.CreateDraft low
       detectionPkgService.CreateDraft()
  -> Package.StartBuild medium approval
       detectionPkgService.StartBuild()
  -> Package.GetBuild readonly polling
       detectionPkgService.GetBuild()
  -> Package.Sign critical approval
       detectionPkgService.SignPackage()
  -> Package.Enable critical approval
       detectionPkgService.EnablePackage()
```

必须拆成至少三个可见节点：

1. 创建草稿。
2. 提交构建。
3. 签名审批。
4. 启用审批。

---

## 16. 后端测试任务拆分

### 16.1 Phase 0 测试

```bash
cd api-server
go test ./internal/repository -run Assistant
```

测试：

- `AssistantSessionRepo.Create/Find/List/UpdateStatus`
- `AssistantMessageRepo.Create/ListBySession`
- `AssistantContextRefRepo.UpsertMany/ListBySession`
- `AssistantToolCallRepo.Create/MarkSuccess/MarkFailed`
- `AssistantApprovalRepo.Create/Find/MarkApproved/MarkRejected`

### 16.2 Phase 1 测试

```bash
go test ./internal/assistant -run 'Service|RunManager'
```

测试：

- 创建会话。
- 附加上下文。
- 发送消息创建 run。
- 取消 run。
- SSE channel 收到 done/error。

### 16.3 Phase 2 测试

```bash
go test ./internal/assistant -run 'ToolRegistry|ToolDispatcher|HostTools|DetectionTools'
```

测试：

- 未注册工具返回错误。
- readonly 工具自动执行。
- 参数缺失返回结构化错误。
- 工具结果写入 tool call repo。

### 16.4 Phase 3 测试

```bash
go test ./internal/assistant -run 'RiskPolicy|ApprovalGate'
```

测试：

- `critical` 工具必定审批。
- 未审批不执行。
- 审批通过后执行一次。
- 已执行审批不能重复执行。
- 拒绝后不执行。

---

## 17. 前端测试任务拆分

```bash
cd frontend
npm run test -- assistant
```

测试：

- `assistantApi` endpoint 正确。
- `assistantStore.applyStreamEvent` 可合并 plan/tool/approval。
- `AssistantApprovalCard` approve/reject 触发正确事件。
- `AskAssistantButton` 生成正确 query。
- `AssistantWorkspace` 可从 route query 创建会话。

---

## 18. 开发任务清单

### Backend Task 1: 表和模型

1. 新增 `migrations/015_v6.0_assistant_tables.sql`。
2. 新增 `model/assistant.go`。
3. `repository/db.go` 加入 AutoMigrate。
4. 新增 6 个 repository。
5. 单测 repository。

### Backend Task 2: 会话与 SSE

1. 新增 `RunManager`。
2. 新增 `AssistantService.CreateSession/SendMessage/Stream/CancelRun`。
3. 新增 `AssistantHandler`。
4. 修改 `router.go`。
5. 修改 `main.go` 注入依赖。
6. curl 验证创建会话和 stream。

### Backend Task 3: Tool 基础设施

1. 新增 `ToolRegistry`。
2. 新增 `ToolDispatcher`。
3. 新增 `RiskPolicy`。
4. 新增 `ToolExecutionRequest/Result`。
5. 接入 RunManager 事件。
6. 写 fake tool 单测。

### Backend Task 4: 只读工具

1. Host tools。
2. Task tools。
3. Detection list/get tools。
4. Package list/get/build log tools。
5. Agent readonly proxy tools。

### Backend Task 5: 审批和写工具

1. ApprovalGate。
2. Vulnerability.StartScan。
3. Task.Redispatch。
4. Detection.BlockAlert。
5. Package.StartBuild/Sign/Enable。
6. 确认 critical 工具无法绕过审批。

### Frontend Task 1: 智能工作台骨架

1. 路由 `/assistant`。
2. 三栏布局。
3. 会话列表。
4. 消息列表。
5. 输入框。

### Frontend Task 2: SSE 和卡片

1. `useAssistantStream`。
2. Plan card。
3. Tool call card。
4. Result card。
5. Error/Done 状态。

### Frontend Task 3: 审批

1. Approval card。
2. Approve/reject API。
3. 风险标签。
4. 执行结果回填。

### Frontend Task 4: 普通页面入口

1. `AskAssistantButton`。
2. 主机、告警、任务、漏洞、检测包页面接入。
3. route query 到 session context。

---

## 19. 第一版最小可交付范围

为了避免 V6.0 一开始过大，建议 MVP 只做：

1. `/assistant` 会话。
2. SSE。
3. Host.List / Host.GetDetail。
4. Detection.ListAlerts / Detection.GetAlert。
5. AgentTool.GetProcessTree / QueryHistoricalLogs。
6. Package.List / Package.GetBuildLog。
7. Detection.BlockAlert 审批但可以暂不执行，先打通审批链。

MVP 完成后，再扩展写工具和全页面入口。

---

## 20. 验收命令

```bash
cd api-server && go test ./internal/assistant ./internal/repository
cd api-server && make build
cd frontend && npm run build
docker compose up -d --build api-server frontend
curl -s http://localhost:8082/health
```

---

## 21. 架构师检查清单

开发评审时逐条检查：

- Tool 是否只调 service/repo/gRPC，不调 handler。
- high/critical 工具是否一定进 ApprovalGate。
- Package.Sign/Package.Enable 是否拆成两个审批。
- Tool call 是否持久化。
- Approval 是否持久化。
- 普通模式是否能看到智能体产生的业务结果。
- 智能模式是否能引用普通模式对象。
- LLM 失败是否不影响普通模式。
- SSE 断开是否不导致 run 崩溃。
- 审批重复点击是否幂等。
- 取消 run 是否停止后续工具调用。

