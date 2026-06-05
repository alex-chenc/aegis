# Aegis V6.0 总体架构设计: 双模智能安全指挥台

**版本**: 6.0  
**日期**: 2026-05-29  
**状态**: 设计中

---

## 1. 架构目标

V6.0 在现有 V5.8 架构上新增“Assistant 智能体编排层”。该层位于 `api-server` 内部，负责会话、上下文、工具目录、意图路由、工具检索、计划执行、审批和审计。它继续使用现有 `github.com/alex-chenc/agent-runtime` 执行 Plan/ReAct/Audit/Reflect/Correct，不重新实现智能体运行时。

核心原则是“全量注册，按需注入”：所有业务查询、动态检测包、规则管理、阻断管理、漏洞治理、基线任务、配置审计、主机攻击研判、外接 MCP 数据源查询能力都注册为 Aegis 内部工具，但每次对话只根据用户意图和页面上下文挑选少量工具传给 agent-runtime。

主机攻击研判采用 Profile 工具模式：命中 `host_attack_investigation` 意图时，模型优先调用 `Investigation.HostAttack.*` 高层工具；后端内部再按固定链路收集资产、漏洞、基线、告警、Agent 实时证据和外部 MCP 证据，避免把底层几十个查询函数一次性暴露给模型。

---

## 2. 总体组件图

```mermaid
flowchart TB
  User["用户"]
  Frontend["frontend Vue 3<br/>普通模式 + 智能模式"]
  AssistantUI["/assistant 智能工作台"]
  NormalUI["现有业务页面"]

  APIServer["api-server"]
  Assistant["assistant orchestrator<br/>会话/计划/工具/审批"]
  ToolRegistry["tool registry<br/>工具注册中心"]
  ToolSelector["tool selector<br/>意图路由/工具检索/按需注入"]
  ApprovalGate["approval gate<br/>风险与审批"]
  BusinessServices["existing services<br/>host/task/vulnerability/detection/package/config"]
  InvestigationEngine["investigation engine<br/>证据收集/入口推断/攻击路径"]
  ExternalMCP["external MCP datasource client<br/>外部数据源受控查询"]
  ExternalMCPServer["external MCP servers<br/>SIEM/CMDB/EDR/工单/情报"]
  LLM["LLM client<br/>OpenAI-compatible/Anthropic-compatible"]
  Runtime["agent-runtime<br/>Plan/ReAct/Audit/Reflect/Correct"]

  Server["server<br/>agent hub"]
  Agent["agent<br/>eBPF/Sigma/命令执行/工具执行"]
  DC["dc<br/>Kafka consumer/alert pipeline"]
  Builder["builder<br/>DetectionPackage build/sign"]

  PG["PostgreSQL"]
  Redis["Redis"]
  Kafka["Kafka"]
  MinIO["MinIO"]

  User --> Frontend
  Frontend --> AssistantUI
  Frontend --> NormalUI
  AssistantUI --> APIServer
  NormalUI --> APIServer

  APIServer --> Assistant
  Assistant --> Runtime
  Assistant --> LLM
  Assistant --> ToolRegistry
  Assistant --> ToolSelector
  Assistant --> ApprovalGate
  ToolSelector --> ToolRegistry
  ToolRegistry --> BusinessServices
  ToolRegistry --> InvestigationEngine
  ToolRegistry --> ExternalMCP
  ApprovalGate --> BusinessServices
  InvestigationEngine --> BusinessServices
  InvestigationEngine --> Server
  InvestigationEngine --> ExternalMCP
  ExternalMCP --> ExternalMCPServer
  ExternalMCP --> PG
  BusinessServices --> PG
  BusinessServices --> Redis
  BusinessServices --> MinIO
  BusinessServices --> Server
  BusinessServices --> Builder

  Server <--> Agent
  Agent --> Server
  Server --> Kafka
  Kafka --> DC
  DC --> PG
  Builder --> MinIO
```

---

## 3. 双模式数据互通图

```mermaid
flowchart LR
  Normal["普通模式页面"]
  AssistantMode["智能模式对话"]
  AssistantTables["assistant_*<br/>会话/消息/工具/审批"]
  BusinessTables["业务表<br/>hosts/tasks/alerts/packages/rules"]
  Services["api-server services"]

  Normal --> Services
  AssistantMode --> Services
  AssistantMode --> AssistantTables
  Services --> BusinessTables
  BusinessTables --> Normal
  BusinessTables --> AssistantMode
```

核心原则：

- 普通模式和智能模式都通过 `api-server` 访问业务服务。
- 智能模式新增的 `assistant_*` 表只存智能体过程，不复制业务事实。
- 智能模式产生的业务变更必须在普通模式立即可见。
- 普通模式中选中的对象可以作为 `assistant_context_refs` 注入智能模式。

---

## 4. 智能体执行链路

```mermaid
sequenceDiagram
  participant U as User
  participant FE as Frontend Assistant UI
  participant API as api-server AssistantHandler
  participant AS as AssistantService
  participant OR as Orchestrator
  participant AG as ApprovalGate
  participant TPS as ToolPolicyService
  participant TR as ToolRegistry
  participant BS as BusinessService
  participant DB as PostgreSQL

  U->>FE: 输入自然语言任务
  FE->>API: POST /api/v1/assistant/sessions/:id/message
  API->>AS: SendMessage(ctx, sessionID, req)
  AS->>DB: 保存 user message
  AS->>OR: Run(ctx, session, message)
  OR->>FE: SSE plan
  OR->>TR: ResolveTool(name)
  TR->>AG: Evaluate(tool, args, user)
  AG->>TPS: GetApprovalMode + IsWhitelisted
  alt 当前审批模式允许直接执行
    AG-->>TR: allow
    TR->>BS: 调用现有 service 方法
    BS->>DB: 读写业务表
    TR-->>OR: tool result
    OR->>FE: SSE tool_result
  else 当前审批模式要求人工审批
    AG-->>TR: require_approval
    TR->>DB: 创建 assistant_approvals
    TR-->>OR: approval_required
    OR->>FE: SSE approval_required
  end
  OR->>DB: 保存 assistant message/tool calls
  OR->>FE: SSE done
```

---

## 5. 审批执行链路

```mermaid
sequenceDiagram
  participant U as User
  participant FE as ApprovalCard
  participant API as AssistantApprovalHandler
  participant AG as ApprovalGate
  participant TR as ToolRegistry
  participant BS as BusinessService
  participant DB as PostgreSQL

  U->>FE: 点击批准
  FE->>API: POST /api/v1/assistant/approvals/:id/approve
  API->>AG: Approve(ctx, approvalID, user)
  AG->>DB: 校验审批状态和权限
  AG->>TR: ExecuteApprovedTool(ctx, approval.ToolCallID)
  TR->>BS: 执行原待审批工具
  BS->>DB: 写业务表
  TR-->>AG: result
  AG->>DB: 更新审批和工具调用结果
  AG-->>FE: 执行结果
```

---

## 6. 动态检测包智能链路

```mermaid
flowchart TD
  A["用户: 为 CVE 生成检测包"] --> B["Assistant: Package.Draft.Generate"]
  B --> C["detection_package_drafts"]
  C --> D["用户继续对话修改草稿"]
  D --> E["Package.Draft.Update"]
  E --> F["Package.Build.Start"]
  F --> G["builder gRPC 编译"]
  G --> H["MinIO unsigned artifact"]
  H --> I["Package.Build.ExplainFailure"]
  I --> J{"用户要求签名启用"}
  J --> K["默认模式下创建 SignPackage critical 审批"]
  K --> L["管理员批准"]
  L --> M["builder SignPackage"]
  M --> N["默认模式下创建 EnablePackage critical 审批"]
  N --> O["管理员批准"]
  O --> P["server 下发到 agent"]
```

约束：

- `Package.Draft.Generate` 可自动执行或按 medium 确认保存草稿。
- `Package.Build.Start` 默认 medium，在 `whitelist` 默认策略下需要二次确认。
- `Package.Sign` 和 `Package.Enable` 必须拆成两个独立工具调用；在 `request_approval` 或 `whitelist` 非白名单状态下分别创建 critical 审批。
- `Package.Allowlist.Update` 默认不进白名单；在 `request_approval` 或 `whitelist` 非白名单状态下创建 critical 审批。
- `full_access` 只跳过工具审批，不允许把签名、启用、allowlist 修改合并成一个隐藏动作。

---

## 7. 逻辑分层

```text
frontend
  assistant workspace
  existing business pages
  shared cards and object links

api-server
  handler layer
    AssistantHandler
    AssistantApprovalHandler
  assistant layer
    AssistantService
    Orchestrator
    RuntimeFactory
    IntentRouter
    ToolRegistry
    ToolCatalog
    ToolSelector
    ApprovalGate
    ContextLoader
    MemoryService
  existing service layer
    HostService
    TaskService
    VulnerabilityService
    DetectionPackageService
    AlertService
    ConfigService
  repository layer
    assistant repositories
    existing repositories

server
  existing agent hub
  existing command forwarding

agent
  existing tool execution
  existing detection package manager
```

---

## 8. 新增 api-server 模块

| 模块 | 路径 | 职责 |
|:---|:---|:---|
| handler | `api-server/internal/api/handler/assistant_handler.go` | HTTP/SSE 入口 |
| service | `api-server/internal/assistant/service.go` | 会话、消息、运行控制 |
| orchestrator | `api-server/internal/assistant/orchestrator.go` | agent-runtime 编排 |
| runtime factory | `api-server/internal/assistant/runtime_factory.go` | 复用 agent-runtime 创建每轮运行时 |
| intent router | `api-server/internal/assistant/intent_router.go` | 从用户输入识别业务域、动作、对象和风险 |
| tool catalog | `api-server/internal/assistant/tool_catalog.go` | 全量工具目录和 ToolSpec 元数据 |
| tool selector | `api-server/internal/assistant/tool_selector.go` | 从全量工具中检索本轮注入工具 |
| tools | `api-server/internal/assistant/tools/` | 业务工具实现 |
| investigation engine | `api-server/internal/assistant/host_attack_investigation_*.go` | 主机攻击研判、证据矩阵、入口推断、攻击路径和报告 |
| external MCP | `api-server/internal/assistant/external_mcp_*.go` | 外接 MCP 数据源配置、受控查询、脱敏和证据归一化 |
| policy | `api-server/internal/assistant/risk_policy.go` | 风险等级和审批策略 |
| approval | `api-server/internal/assistant/approval_gate.go` | 审批创建、批准、拒绝、执行 |
| context | `api-server/internal/assistant/context_loader.go` | 加载上下文对象 |
| memory | `api-server/internal/assistant/memory_service.go` | 会话摘要和长期记忆 |
| repository | `api-server/internal/repository/assistant_*.go` | 新表读写 |

---

## 9. 工具域设计

| 域 | 工具前缀 | 对应现有能力 |
|:---|:---|:---|
| 主机 | `Host.*` | host repo, agent online status |
| 基线 | `Baseline.*` | task/template/rule service |
| 漏洞 | `Vulnerability.*` | vulnerability service, host script service |
| 检测查询 | `Detection.*` | alert repo, runtime event repo, llm aggregation service |
| Sigma 规则 | `SigmaRule.*` | sigma rule repo/service, rule generation service |
| 阻断管理 | `Block.*` | block policy repo, block repo, alert service |
| 动态检测包 | `Package.*` | detection package service, package generation service, builder client |
| 配置 | `Config.*` | config service, system config |
| 主机攻击研判 | `Investigation.*` | host attack investigation service, evidence collector, entry inferer |
| 外接 MCP 数据源 | `ExternalMCP.*` | external MCP source service, controlled MCP client |
| 审计 | `Audit.*` | audit log repo, command audit repo |
| agent 主机工具 | `AgentTool.*` | server gRPC ExecuteTool |

完整工具目录、工具检索算法和 V5.8 动态检测包/规则/阻断函数映射见 `agent_runtime_tool_orchestration_design_v6.0.md`。

---

## 9.1 主机攻击研判链路

```mermaid
flowchart TD
  A["用户: 这台主机是不是被攻击了"]
  B["IntentRouter<br/>host_attack_investigation"]
  C["ToolSelector<br/>注入 Investigation.HostAttack.Analyze"]
  D["agent-runtime<br/>调用高层研判工具"]
  E["EvidenceCollector<br/>资产/漏洞/基线/告警/Agent"]
  F["ExternalMCP.*<br/>SIEM/CMDB/EDR 可选证据"]
  G["EvidenceCorrelator<br/>去重/归一/关联"]
  H["EntryPointInferer<br/>入口候选"]
  I["AttackTimelineBuilder<br/>攻击时间线"]
  J["AttackPathBuilder<br/>攻击路径图"]
  K["CompromiseScorer<br/>被攻击判断"]
  L["InvestigationReportBuilder<br/>Prompt + 报告"]
  M["Result cards<br/>证据矩阵/入口/路径/建议"]

  A --> B --> C --> D --> E
  E --> F
  E --> G
  F --> G
  G --> H
  G --> I
  G --> J
  H --> K
  I --> K
  J --> K
  K --> L --> M
```

该链路详细结构体、函数、Prompt、数据库和测试设计见 `host_attack_investigation_agent_design_v6.0.md`。

---

## 10. 安全边界

### 10.1 不允许的调用

- 智能体直接拼 SQL 写业务表。
- 智能体直接调用 `builder.SignPackage` 绕过审批。
- 智能体直接调用 `server` 下发阻断命令绕过 BlockService。
- 智能体修改 hook allowlist 不经审批。
- 智能体使用未注册工具名。

### 10.2 必须审计的行为

- 每次模型调用。
- 每次工具调用。
- 每次审批创建。
- 每次审批批准或拒绝。
- 每次高风险业务写操作。
- 每次执行失败和回滚建议。

---

## 11. 兼容性

| 项 | 策略 |
|:---|:---|
| V5.8 API | 不删除、不改语义 |
| 前端现有路由 | 保留 |
| AI 分析页 | 保留，可逐步迁移到全局 assistant 能力 |
| 数据库 | 只新增表和索引，不破坏现有表 |
| agent | 第一版不要求 agent 升级，复用现有工具执行 |
| builder | 不改签名边界 |

---

## 12. 回滚方案

1. 前端隐藏 `/assistant` 路由和模式切换入口。
2. 后端关闭 `ASSISTANT_ENABLED=false`。
3. 保留 `assistant_*` 表不删除，不影响业务表。
4. 现有普通模式页面继续工作。
5. 已由智能体创建的业务任务继续按原业务流程完成。

