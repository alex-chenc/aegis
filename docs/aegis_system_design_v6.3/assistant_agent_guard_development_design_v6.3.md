# Aegis V6.3 智能体模式集成 Agent Guard 开发文档

**版本**：V6.3 方案版  
**日期**：2026-08-11  
**状态**：第一阶段代码已完成；共享应用服务、细粒度授权和前端上下文仍待开发/验收  
**上位设计**：
[assistant_agent_guard_integration_design_v6.3.md](assistant_agent_guard_integration_design_v6.3.md)

## 1. 开发目标

在不修改 V6.2 Agent Guard 数据面、规则归属和动作状态机的前提下，为 `/assistant`
注册完整的 `agent_guard` 工具域，并完成以下工程闭环：

1. HTTP 页面与 Assistant 工具共用一套应用服务、scope、脱敏和错误语义。
2. Assistant capability 目录支持 V6.2 P0～P4 全部当前产品能力。
3. V6.2 P5 通过 V6.3 `AgentSessionService` 接入会话感知工具。
4. RBAC、审批、硬确认、状态轮询和工具审计满足 Agent Guard 现有安全边界。
5. Agent Guard 普通页面可以将受信对象上下文交给智能体模式。

本开发不修改以下事实源：

- `migrations/029_v6.2_agent_guard.sql`、030、031；
- `proto/agent_comm.proto` 和 `proto/agent_session_comm.proto`；
- Kafka topic 和 DC 规则归属；
- Agent BPF LSM、Native Hook、ConfigSync 和 BlockCommand 实现；
- V6.2 历史 policy API 的兼容状态。

## 2. 实施前代码基线

开发开始前必须重新读取：

```text
docs/aegis_system_design_v6.2/current_implementation_baseline_2026-08-06.md
docs/aegis_system_design_v6.2/implementation_status_v6.2.md
docs/aegis_system_design_v6.1/fix/assistant_generic_agent_flow_only.md
api-server/internal/api/handler/agent_guard_handler.go
api-server/internal/repository/agent_guard_*_repo.go
api-server/internal/service/agent_guard_*_service.go
api-server/internal/assistant/tool_registry.go
api-server/internal/assistant/tool_exposure.go
api-server/internal/assistant/tool_dispatcher.go
api-server/internal/assistant/tool_invocation_filter.go
api-server/internal/assistant/tools/*.go
api-server/cmd/main.go
frontend/src/views/detection/AgentGuard/**
```

以代码为准复核接口名、错误类型、状态枚举和依赖初始化顺序。开发时必须同时参考现有
高层实现：`api-server/internal/assistant/tools/baseline_compliance_tools.go` 的
`Baseline.Compliance.Run + Operation.Get`，以及
`api-server/internal/assistant/workflow_plan_compiler.go` 的
`VulnerabilityRemediationCompiler`。前者作为高层 operation 样板，后者作为
completion/prerequisite/previous-step 绑定样板。不得把设计文档中的旧
policy UI、P5 原文 reveal/export 或尚未完成的专用宿主机门禁当成可用工具。

## 3. 代码改动总览

### 3.1 新增文件

```text
api-server/internal/assistant/tools/agent_guard_tools.go
api-server/internal/assistant/tools/agent_guard_tools_test.go
api-server/internal/assistant/tools/english_contract_test.go (Agent Guard registration coverage)

第一阶段已实际创建/修改的文件为：

```text
api-server/internal/assistant/tools/agent_guard_tools.go
api-server/internal/assistant/tools/agent_guard_tools_test.go
api-server/internal/assistant/tool_registry.go
api-server/internal/assistant/tools/english_contract_test.go
api-server/cmd/main.go
```

下一阶段仍计划新增共享应用服务、权限过滤、审计迁移和页面上下文文件；它们不是本次第一阶段实现的一部分。
```

名称可按现有包风格微调，但职责不能合并进 Gin Handler 或 frontend store。

### 3.2 修改文件

```text
api-server/internal/assistant/tool_registry.go
api-server/internal/assistant/tool_exposure.go
api-server/internal/assistant/tool_invocation_context.go
api-server/internal/assistant/tool_invocation_filter.go
api-server/internal/assistant/tool_dispatcher.go
api-server/internal/assistant/context_loader.go
api-server/internal/api/handler/assistant_handler.go
api-server/internal/api/handler/agent_guard_handler.go
api-server/internal/model/assistant.go
api-server/internal/repository/db.go
api-server/cmd/main.go
frontend/src/views/assistant/components/AskAssistantButton.vue
frontend/src/views/detection/AgentGuard/AgentGuardLayout.vue
frontend/src/views/detection/AgentGuard/AgentConfigurationDetection.vue
frontend/src/views/detection/AgentSessionAwareness.vue
frontend/src/i18n/locales/zh-CN/agentGuard.ts
frontend/src/i18n/locales/en-US/agentGuard.ts
docker-compose.yml
.env.example
```

若 `repository/db.go` 只做模型注册且正式迁移由 SQL 管理，仍需同步模型字段，但不得
用 AutoMigrate 替代 033 migration。

## 4. P0：共享应用服务与权限基础

### 4.1 提取 `AgentGuardApplicationService`

现有 `AgentGuardHandler` 同时承担 HTTP 和部分应用层逻辑。先提取可复用服务，再注册
工具。推荐接口按业务视图组织，不暴露 Gin 或 HTTP 类型：

```go
type AgentGuardApplicationService interface {
    Overview(context.Context, AgentGuardOverviewRequest) (AgentGuardOverviewView, error)
    ListAgents(context.Context, AgentGuardAgentRequest) (AgentGuardPage[AgentGuardAgentView], error)
    InspectAgent(context.Context, AgentGuardInspectRequest) (AgentGuardInspectView, error)
    ListCatalog(context.Context, AgentGuardCatalogRequest) (AgentGuardPage[AgentGuardCatalogItem], error)
    QueryBehaviors(context.Context, AgentGuardBehaviorRequest) (AgentGuardPage[AgentGuardBehaviorView], error)
    GetEvidence(context.Context, AgentGuardEvidenceRequest) (AgentGuardEvidenceView, error)
    QueryFindings(context.Context, AgentGuardFindingRequest) (AgentGuardFindingResult, error)
    GetAnalysis(context.Context, AgentGuardAnalysisRequest) (AgentGuardAnalysisResult, error)
    QueryActions(context.Context, AgentGuardActionQueryRequest) (AgentGuardActionResult, error)
    GetRuntimeSettings(context.Context, uuid.UUID) (*model.AgentGuardRuntimeSettings, error)
    ScanConfiguration(context.Context, AgentGuardConfigScanRequest) (AgentGuardConfigScanView, error)
}
```

写操作继续调用现有领域 service：

```text
Finding.Analyze           -> AgentGuardAnalysisService.Request
RuntimeSettings.Update    -> AgentGuardRuntimeSettingsService.Update
Session.Delete            -> AgentGuardQueryRepository.DeleteSessions
ExecutionUnit.*           -> AgentGuardActionService.RequestExecutionUnit
Instance.Kill             -> AgentGuardActionService.RequestInstanceKill
Conversation.*            -> AgentSessionService
```

应用服务必须复用或迁移 Handler 当前已有的：

- `applyFindingScope`、scope signer 验证；
- `redactAgentGuard*`；
- Finding matched rule/process tree/escape chain 组装；
- optional UUID、分页、time range 和 finding domain 校验；
- raw behavior 和 evidence 的输出限制。

完成后 Handler 使用应用服务返回 DTO，不保留一份不同的业务判断。为控制改动风险，
可先为工具增加 facade 并编写 HTTP/tool parity test，再逐步让 Handler 调用同一 facade；
在 parity 完成前不得上线写工具。

### 4.2 新增通用工具授权元数据

`ToolSpec` 增加：

```go
type ToolAuthorizationPolicy struct {
    RequiredPermissions []string
    Confirmation        string // none | user_challenge
}

type ToolSpec struct {
    // existing fields...
    Authorization ToolAuthorizationPolicy
}
```

`ToolInvocationFilterRequest` 增加可信字段：

```go
Operator string
Role     string
```

`ToolDispatcher` 注入只读 authorizer：

```go
type ToolAuthorizer interface {
    ResolveRole(context.Context, string) (string, error)
    HasPermission(string, string) bool
}
```

新增优先级位于 schema validation 之前的 `toolAuthorizationFilter`：

1. operator 为空直接拒绝；
2. 读取当前角色；
3. 检查工具所有 `RequiredPermissions`；
4. 在 approval resume 再次执行；
5. 只记录 permission key、role 和结果，不记录 token 或证据正文。

候选目录通过 `ToolExposureContext` 接收已解析 permission set，隐藏未授权工具；但隐藏
不是授权依据，dispatcher filter 必须保留。

### 4.3 修正 Assistant 身份来源

当前全局鉴权写入 `auth_username`。Assistant Handler 中所有创建会话、发送消息、审批、
执行审批和恢复操作统一通过一个 helper 获取：

```go
func authenticatedOperator(c *gin.Context) (string, bool)
```

不得继续依赖未由 `AuthRequired` 写入的 `username` key。operator 进入
`DispatchRequest.Operator`，模型 schema 中不出现 `operator/requested_by/reviewed_by`。

### 4.4 审计字段迁移

新增 `033_v6.3_assistant_agent_guard_integration.sql`，只扩展 Assistant 审计，不修改
Agent Guard 业务表：

```sql
ALTER TABLE assistant_tool_calls
    ADD COLUMN IF NOT EXISTS requested_by VARCHAR(100),
    ADD COLUMN IF NOT EXISTS authorization_snapshot JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS approval_id VARCHAR(100);

CREATE INDEX IF NOT EXISTS idx_assistant_tool_calls_requested_by
    ON assistant_tool_calls(requested_by, created_at DESC);
```

`authorization_snapshot` 只保存 role、permission keys、decision 和 checked_at。禁止保存
JWT、密码、命令行、配置正文或会话内容。审批明细继续复用 `assistant_approvals` 的
`requested_by/reviewed_by/impact_summary/rollback_hint`。

### 4.5 P0 测试先行

- analyst 无法看见/调用 action 和 evidence 工具；
- developer 可以 evidence/analyze，不能 settings/action/delete；
- admin 可以进入写工具审批；
- 目录隐藏后手工构造 tool request 仍被 dispatcher 拒绝；
- 等待审批期间角色降级，resume 返回 authorization revoked；
- Assistant operator 来自 `auth_username`，模型参数无法覆盖；
- HTTP 和 facade 的 Finding/detail/redaction fixture 完全一致。

## 5. P1：注册领域 capability，并向上封装模型工具

### 5.1 依赖结构

```go
type AgentGuardToolDeps struct {
    Application      AgentGuardApplicationService
    Analysis         AgentGuardAnalysisRequester
    Actions          AgentGuardActionRequester
    RuntimeSettings  AgentGuardRuntimeSettingsService
    SessionDeleter   AgentGuardSessionDeleter
    Conversations    *service.AgentSessionService
    Flags            AgentGuardAssistantFlags
    Logger           *zap.Logger
}
```

接口应按最小方法集合定义，单元测试使用 fake；不要让工具依赖具体 repository 类型。

这里的 `agent_guard_tools.go` 是领域 capability 适配层，不代表所有 capability 都进入
模型目录。模型可见工具由 `agent_guard_workflow_tools.go` 的高层工具注册器生成。

### 5.2 两级注册顺序

在 `cmd/main.go` 完成 Agent Guard repository/service 初始化之后、
`ValidateModelFacingEnglish` 和 `ValidateCapabilityUniqueness` 之前调用：

```go
assistantTools.RegisterAgentGuardTools(toolRegistry, deps)
```

新增 `DomainAgentGuard ToolDomain = "agent_guard"`。所有 capability 采用设计文档中的
小写英文标识，保证一对一唯一。`RegisterAgentGuardTools` 任一工具注册失败应使服务启动
失败或至少阻止整组 Agent Guard 工具暴露，不能留下半套目录。

随后注册高层模型能力：

```go
assistantTools.RegisterAgentGuardWorkflowTools(toolRegistry, workflowDeps)
```

高层工具必须声明 `primary` exposure、对应 WorkflowSpec、允许的内部 capability、
完成条件和所需 `WorkflowExecutionGrant`。模型目录只由高层工具和命中的 contextual
工具组成；原子 capability 不能因为注册成功、审批白名单或 `full_access` 自动进入目录。

### 5.3 高层工具内部授权

高层工具内部调用遵循 V6.1 的 grant 约束：

1. 先解析用户语义范围为唯一 Agent/instance/session/unit/Finding/conversation ID；
2. 校验用户权限、tenant/scope、Agent 状态和目标前置条件；
3. 由后端签发短期、绑定 `workflow_id + scope_hash + parameters_hash` 的 grant；
4. grant 只列出本高层契约声明的 internal capability，且按步骤和调用次数限制；
5. grant 不会转换为模型可见工具，也不能授权 policy publish、任意 action 或跨 scope 读取。

策略/规则装载、raw evidence、dispatch 和状态聚合放在 internal capability。模型只能
看到语义化结果；需要解释规则时，通过权限控制的 `Catalog.List` contextual 工具返回
稳定 key/version/severity/说明，不返回策略实现和修改入口。

### 5.4 工具参数要点

#### `AgentGuard.Overview.Get`

```json
{
  "host_ids": ["uuid"],
  "agent_types": ["codex"]
}
```

返回 overview、coverage buckets、更新时间和 gap。未知 coverage 枚举原样保留并附
`coverage_interpretation=unknown`，不能默认成 monitor-only。

#### `AgentGuard.Agent.List`

支持 host、agent type、runtime status、coverage、keyword、page。返回外层窄 DTO，
不得增加 controller cmdline、path、address 或 evidence。

#### `AgentGuard.Agent.Inspect`

```json
{
  "scope_kind": "agent|instance|behavior_session|execution_unit",
  "scope_id": "stable id",
  "view": "summary|instances|sessions|execution_units",
  "page": 1,
  "page_size": 20
}
```

`scope_kind=agent` 时 `scope_id` 为签名的 `agent_scope_key`；服务端重新验证签名。
summary 只包含 host status、coverage、计数和稳定引用。

#### `AgentGuard.Catalog.List`

`catalog_type` 只允许 `profile/behavior_rule/escape_rule/configuration_rule`。规则返回稳定
key、version、digest、severity/action、required evidence 和 allow conditions；不返回
历史 policy draft/publish 数据。

#### `AgentGuard.Behavior.Query`

`view=events|panorama`。events 使用现有 filters；panorama 必须绑定单一 instance 和
真实 behavior session。默认不返回 raw event JSON。

#### `AgentGuard.Evidence.Get`

`evidence_type` 为 `execution_unit/behavior/raw_behavior/panorama_children`。每次只读取一个
对象或一个子节点页，要求 evidence 权限。输出先通过应用服务二次脱敏，再执行字节限制。

#### `AgentGuard.Finding.Query`

列表支持 domain、host、instance、behavior session、severity、verdict、status、rule ID
和 time range。`finding_domain=tool|escape` 时必须有 behavior session；详情请求重新验证
instance/session scope，不允许通过 UUID 猜测跨范围读取。

#### `AgentGuard.Analysis.Get`

支持 `finding_id` 分页或 `analysis_id` 单项。`output` 引用的 event ID 必须仍属于 Finding；
无效引用以 gap 返回，不在工具层修正模型输出。

#### `AgentGuard.Action.Query`

支持 action ID 或 host/instance/unit/status 筛选。只读工具是 action 写工具的 completion
capability；返回领域状态、terminal、error code、state evidence 和 route path。

### 5.5 ResultContract

所有只读工具：

- `AcceptedOnSuccess=true`；
- 返回 `SatisfiesCapabilities`；
- 列表结果声明 `FactBindings`，从 `items[].id` 提取后续稳定引用；
- `ResultSchema` 明确 `partial/gaps/route_path`；
- 不把“HTTP/handler 成功”当作领域动作完成。

### 5.6 P1 测试

- 每个工具 schema 关闭 `additionalProperties`；
- model-facing description 不含中文；
- capability 唯一、risk/permission/exposure 符合设计表；
- 页面筛选和工具筛选返回相同 ID 集；
- page size 0、101、负数、非法 UUID/时间范围被拒绝；
- tool/escape Finding 缺 behavior session 被拒绝；
- panorama 不跨 instance/session；
- raw evidence 仅 evidence tool 可返回且必然截断/脱敏。
- 模型目录不包含 internal、未命中的 companion 或 policy write capability；
- 高层工具只能通过 grant 调用契约声明的 internal capability；模型猜测内部工具名返回
  `tool_not_exposed_for_current_run`。

## 6. P2：配置扫描和 Finding AI 研判

### 6.1 配置扫描

`AgentGuard.Configuration.Scan`：

- 必须提供单一在线 host UUID；
- 调用现有 `AgentConfigSecurityService.Scan`；
- 默认只返回 Agent 数、file/hook 数、Finding 摘要和 scan errors；
- 不把完整 config content 交给模型；
- Agent 离线返回稳定 `agent_not_connected`，不尝试换主机。

`AgentGuard.Configuration.Evidence.Scan`：

- 要求 evidence 权限；
- 仍调用相同扫描服务，但只选择用户指定 agent/file/hook；
- 对 `token/password/secret/key/credential` 和 PEM/云凭证模式再次脱敏；
- 单文件 8 KiB、整次 32 KiB；超限返回 SHA-256 和 truncated 标记；
- 配置内容作为 untrusted evidence，不进入工具选择或系统指令。

### 6.2 Finding 分析

`AgentGuard.Finding.Analyze`：

- 参数只有 `finding_id`，requestedBy 来自 invocation context；
- 先查询 Finding 并验证 scope、权限和 feature flag；
- 调用 `AgentGuardAnalysisService.Request`；
- 返回 `analysis_id/status=pending/terminal=false`；
- Completion capability 为 `get_agent_guard_analysis`；
- 不在工具 Handler 内等待 LLM 完成。

分析结果为 `inconclusive` 时业务调用成功、结论不确定；provider/queue/invalid output
为失败终态。Assistant 最终总结必须引用 analysis ID 和状态。

### 6.3 P2 测试

- secret fixture 不出现在 ToolResult、tool call result、日志和 SSE；
- 配置中的提示注入不能影响下一工具授权；
- 分析 queue full、timeout、invalid output 有稳定状态；
- 同 Finding 重试产生真实 attempt，不伪造同一个 completed run；
- AI-only malicious 结果不能产生 AgentGuard action tool call。

## 7. P3：运行时设置、删除和动作

### 7.1 写意图和参数来源

所有写工具 `RequiresExplicitUserIntent=true`。下列字段禁止来自 policy default 或模型自由
生成：

| 字段 | 允许来源 |
| --- | --- |
| target UUID | user message、page context、previous tool result |
| reason | user message；审批 UI 可补充 |
| confirmation | user message 或 approval UI，禁止 LLM source |
| requested_by | authenticated invocation context |
| expected_version | previous `RuntimeSettings.Get` result |

`ArgSource` 不满足时，ToolDecisionEngine 追问或等待审批，不能由 gateway 猜值。

### 7.2 运行时设置

参数必须提交完整目标状态和 `expected_version`。工具流程：

1. 读取当前 settings；
2. 校验 expected version；
3. 生成字段级 diff 和影响摘要；
4. 创建审批；
5. 审批恢复后再次读取并比较版本；
6. 调用 `AgentGuardRuntimeSettingsService.Update`；
7. 返回新 version、dispatch status 和 Agent applied gap。

关闭 behavior/escape policy、Hook 或 adapter 的影响摘要必须明确“将减少遥测或防护”。

### 7.3 session 删除

- 一次 1～100 个精确 UUID，去重但不扩大范围；
- 审批前读取每个 session 的 host/agent/时间和级联计数；
- 挑战字符串为 `DELETE <count> AGENT SESSIONS`；
- 审批恢复时重新解析 ID，已不存在项单独报告；
- 调用现有 `DeleteSessions`；
- 成功后返回 deleted count 和不可恢复标记。

### 7.4 execution unit/instance 动作

每个动作使用独立 ToolSpec 和独立 permission：

```text
Freeze -> agent_guard:action:freeze
Resume -> agent_guard:action:resume
Kill   -> agent_guard:action:kill
```

执行前从 repository/service 重新解析：

- unit/instance/host ownership；
- controller PID + start_ticks；
- coverage 和 capability delivery；
- 当前 running/frozen/stopped 状态；
- Agent 在线状态；
- protected target 和 remote_unobservable；
- active freeze 幂等记录。

工具只调用 `AgentGuardActionService`。action 返回 pending/dispatching 后由 Runtime 使用
`AgentGuard.Action.Query` 轮询。达到 Runtime 时间预算仍非终态时返回 operation reference，
不能把 dispatch success 当成 state change。

kill unit/instance 使用后端挑战：

```text
KILL UNIT <last8>
KILL AGENT <last8>
```

挑战生成和校验位于可信后端；它不作为模型可填写的普通 schema 字段。

### 7.5 P3 测试

- full_access 不绕过 kill/delete 硬确认和 Agent Guard RBAC；
- settings version 冲突不覆盖新状态；
- 审批后 unit 从 running 变 stopped，动作拒绝；
- remote_unobservable、monitor_only、unsupported profile 不展示/执行不支持动作；
- freeze 幂等返回 existing action；
- resume 仅针对有效 active freeze；
- kill 不接受 host、wildcard、PID、path 或空 target；
- gRPC dispatch success 但 Agent terminal failed 时工具最终为 failed；
- 原始 Agent error code 保留，敏感 error detail 截断。

## 8. P4：V6.3 会话感知和页面上下文

### 8.1 会话感知工具

复用 `AgentSessionService`，不读取 `agent_conversation_*` 表的内部实现细节：

- `Conversation.Query`：List/Detail/RuleHits/GetAIAnalysis；
- `Conversation.Content.Get`：分页 items，evidence 权限；
- `Conversation.Collect`：`RequestCollection`，单 host + `claude-code|codex`；
- `Conversation.Analyze`：`Analyze`，复用现有 Token chunk 和 AI safety。

工具 DTO 对 conversation/behavior session 使用不同字段名。关联行为仅返回已由 V6.3
服务或 repository 确认的 relation；没有关联时返回 `unlinked`，不按时间邻近伪造。

### 8.2 ContextLoader 扩展

为以下类型增加服务端 resolver：

```text
agent_guard_agent
agent_guard_instance
agent_guard_behavior_session
agent_guard_execution_unit
agent_guard_finding
agent_guard_action
agent_conversation_session
```

resolver 只返回窄快照：稳定 ID、标题、host/agent、severity/status、coverage、route。
不把 command line、raw evidence、config content 或 conversation content写入
`assistant_context_refs.snapshot`。

### 8.3 前端入口

扩展 `AskAssistantButton` 支持：

```ts
interface AssistantContextInput {
  objectType: string
  objectId: string
  prompt?: string
}
```

在 Agent Guard 列表/详情、Finding、execution unit action、配置检测结果和会话感知
详情中放置按钮。URL 只传 `context_type/context_id` 和短 prompt；进入 Assistant 后立即
由 API 创建 context ref，随后清理 URL 中的对象参数。

结果卡至少支持：

- Agent/coverage 摘要；
- Finding/analysis 摘要；
- pending action/analysis 的轮询状态；
- 跳回 events/escape/configurations/session-awareness 页面。

### 8.4 P4 测试

- 客户端伪造 title/severity/host 被服务端忽略；
- 无权限用户通过 context ID 不能读取 evidence；
- agent_scope_key 签名错误或过期被拒绝；
- page context 的 stable ID 可直接绑定工具参数；
- conversation 和 behavior session 不串 ID；
- 页面切换 Assistant 后上下文、语言和预填 prompt 正确；
- URL、localStorage 和 console 不出现 evidence/content。

## 9. 日志和指标

### 9.1 日志事件

新增稳定事件名：

```text
assistant_agent_guard_tool_authorized
assistant_agent_guard_tool_denied
assistant_agent_guard_query_completed
assistant_agent_guard_analysis_requested
assistant_agent_guard_settings_requested
assistant_agent_guard_action_requested
assistant_agent_guard_action_terminal
assistant_agent_guard_confirmation_rejected
assistant_agent_guard_context_resolved
```

允许字段：

```text
session_id, run_id, call_id, tool_name, capability,
operator, role, permission, host_id, instance_id, behavior_session_id,
execution_unit_id, finding_id, analysis_id, action_id,
status, error_code, duration_ms, item_count, truncated
```

禁止字段：

```text
JWT/API key/password/token/secret、完整命令行、tool input/output、
配置正文、会话正文、raw event、approval params 原文
```

reason 只记录是否存在和长度，不记录全文。ID 使用现有稳定 ID，不记录 scope signing key
或 correlation token。

### 9.2 指标

```text
assistant_agent_guard_tool_calls_total{tool,status}
assistant_agent_guard_tool_duration_seconds{tool}
assistant_agent_guard_authorization_denied_total{permission}
assistant_agent_guard_approval_total{tool,status}
assistant_agent_guard_pending_operations{kind,status}
assistant_agent_guard_result_truncated_total{tool}
assistant_agent_guard_context_resolution_total{object_type,status}
```

指标标签禁止 host ID、username、finding ID 等高基数字段。

## 10. Feature Flag 和配置

api-server 配置增加：

```yaml
agent_guard:
  assistant_enabled: false
  assistant_write_enabled: false
  assistant_action_enabled: false
  assistant_session_awareness_enabled: false
```

环境变量与设计文档一致。启用条件：

```text
readonly tools  = AgentGuard.Enabled && AssistantEnabled
analysis tool   = readonly && AgentGuard.AnalysisEnabled
settings tool   = readonly && AssistantWriteEnabled
action tools    = readonly && AssistantActionEnabled && AgentGuard.ActionEnabled
conversation    = readonly && AssistantSessionAwarenessEnabled && AgentSession.Enabled
```

配置缺失默认 false。工具禁用时不进入 model-facing catalog；直接调用仍返回 disabled，
不能静默回退到 HTTP 或通用 Agent tools。

## 11. 测试与验证矩阵

| 层 | 必测内容 |
| --- | --- |
| Tool Registry | 23 个领域 capability 与高层模型 capability 均唯一、英文契约、风险、权限、exposure、feature flag |
| Intent/Mapping | 中英文自然语言映射 exact capability；不生成固定 Agent Guard 计划 |
| Authorization | 三角色矩阵、目录裁剪、dispatcher 硬门、审批恢复重授权 |
| Application service | HTTP/tool parity、scope、分页、错误码、redaction |
| Query tools | 空数据、边界页、partial/degraded/remote unobservable、跨 scope 拒绝 |
| Config scan | 在线/离线、secret、超限、提示注入、evidence 权限 |
| Analysis | pending/succeeded/inconclusive/failed/invalid output、真实 ID 轮询 |
| Settings | expected version、dispatch failed、pending reconnect、关闭防护影响摘要 |
| Action | target ownership、coverage、状态冲突、审批、确认、幂等、真实终态 |
| Session delete | 精确 ID、去重、部分不存在、级联、不可恢复确认 |
| Session awareness | collect/list/content/rules/AI、Token/正文边界、ID 类型不串联 |
| Frontend | AskAssistant context、结果卡、审批卡、pending 状态、i18n、无敏感 URL |
| 回归 | V6.2 Agent Guard、V6.3 AgentSession、Assistant 既有工具、组件构建 |

## 12. 验证命令起点

实现时使用 `aegis-build-test` 选择最窄验证，至少覆盖：

```bash
cd api-server
go test ./internal/assistant/... ./internal/service/... ./internal/api/handler/... -run 'AgentGuard|AgentSession|Assistant' -count=1
go test -race ./internal/assistant/... ./internal/service/... -run 'AgentGuard|AgentSession' -count=1
go build ./...

cd frontend
npm run test -- src/views/detection/AgentGuard src/views/detection/AgentSessionAwareness.vue
npm run build

cd server
go test ./internal/grpc_server ./internal/queue -run 'AgentGuard|AgentSession' -count=1
go build ./...

cd dc
go test ./internal/sessionaudit ./internal/pipeline ./internal/repository -run 'AgentGuard|AgentSession' -count=1
go build ./...

cd agent
go test ./internal/agentguard ./internal/agentsession ./internal/client -count=1
go build ./...

cd /code/aegis
scripts/tests/agent_guard_cross_contract_test.sh
docker compose config --quiet
git diff --check
```

若本次实现不修改 Agent/eBPF，无需重新生成 BPF；但必须运行 Agent Guard 定向回归，
证明控制面工具没有改变数据面协议。真实 freeze/kill 只在专用测试主机执行，禁止在共享
开发 Compose 主机上做破坏性验证。

## 13. 灰度和回滚

### 13.1 灰度顺序

1. 仅注册但全局禁用，完成 registry/schema 测试。
2. 管理员测试 readonly overview/agent/catalog。
3. 开放 analyst readonly；验证 scope 和脱敏。
4. 开放 config summary 和 Finding analysis。
5. 开放 developer evidence；检查输出大小和 secret leak。
6. 单管理员开放 settings。
7. 专用测试主机开放 freeze/resume，再开放 kill/delete 硬确认。
8. 最后开放 V6.3 conversation content/analysis。

### 13.2 停止扩量条件

- 任一跨 host/instance/session 的数据泄漏；
- 未授权工具可执行；
- full_access 绕过 kill/delete 硬确认；
- pending 被回答为 completed；
- secret/正文进入日志、URL 或普通 tool summary；
- action target 与审批卡目标不一致；
- 现有 Agent Guard 页面与工具返回的 scope/状态不同。

### 13.3 回滚

- 先关闭 `assistant_action_enabled` 和 `assistant_write_enabled`；
- 保留 Action 查询，使已发起操作仍可查看终态；
- 再关闭 session awareness 和 readonly master flag；
- 不删除 033 新列和已有审计数据；
- 不回滚 V6.2/V6.3 migration、Kafka 或 Agent 配置；
- 已执行 kill/delete 无法回滚；runtime settings 使用一次新的受控更新恢复；freeze 使用
  timeout 或现有 resume 路径恢复。

## 14. 完成定义

只有以下条件全部满足，才能报告“V6.3 智能体模式已集成 V6.2 全能力”：

1. 设计文档功能矩阵中所有“必须接入”项都有启用的工具和测试。
2. 历史 policy 写 API、策略解析和 raw evidence internal capability 未进入 Assistant catalog。
3. Tool Registry、capability uniqueness 和英文契约验证通过。
4. 普通助手优先看到高层业务 capability；23 个领域 capability 按 contextual/companion/internal 正确分层。
5. RBAC、审批恢复重授权、硬确认和参数来源测试通过。
6. HTTP/tool parity、redaction、scope 和异步终态测试通过。
7. api-server 定向测试、race test 和 build 通过。
8. frontend 定向测试和生产 build 通过。
9. Agent、Server、DC 的 Agent Guard/AgentSession 回归通过。
10. 至少完成一个无破坏性的 overview → finding → analysis E2E。
11. freeze/resume/kill 只在专用主机完成资格验证，并保留 action ID 和状态证据。
12. 关闭 feature flag 后普通模式完全不回归。
13. 未执行的专用宿主机、离线发布或真实产品验证被明确报告，不能用 mock/build 代替。
