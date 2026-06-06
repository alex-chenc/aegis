# Aegis V6.0 agent-runtime 工具编排设计缺漏分析

**版本**: 6.0  
**日期**: 2026-06-05  
**状态**: 设计缺漏分析与补充  
**基准文档**: `agent_runtime_tool_orchestration_design_v6.0.md`  
**对比实现**: `api-server/internal/assistant/` 目录下的现有代码  

---

## 1. 分析范围

本文对照设计文档（第 1–22 节）和当前实现，逐项排查以下维度的缺漏：

1. ToolSpec 结构体字段
2. ToolCatalog 与工具注册
3. 工具命名规范
4. IntentRouter 设计
5. ToolSelector 设计
6. Tool.Search 元工具
7. RuntimeFactory 设计
8. Agent 工具（Agent.*）
9. Investigation 工具（Investigation.*）
10. ExternalMCP 工具
11. Notification / RuntimeEvent / ToolCall 工具
12. 审批暂停与恢复（PauseForApproval / ResumeAfterApproval）
13. SSE 事件覆盖
14. agent-runtime ToolPolicy 接口对接
15. Orchestrator 主流程对齐
16. 双网关冗余问题

---

## 2. 逐项缺漏分析

### 2.1 ToolSpec 结构体字段缺漏

**设计文档要求**（第 5.1 节）:

```go
type ToolSpec struct {
    Name              string
    Domain            ToolDomain
    Operation         ToolOperation
    Capability        string
    Description       string
    Aliases           []string
    Tags              []string
    ObjectTypes       []string
    PageRoutes        []string
    Risk              ToolRisk
    AutoCallable      bool
    RequiresApproval  bool
    Idempotent        bool
    DefaultTimeout    time.Duration
    ArgsSchema        map[string]any
    ResultSchema      map[string]any
    Handler           ToolHandler
    ServiceBinding    ServiceBinding
}
```

**当前实现**（`tool_registry.go`）:

```go
type ToolSpec struct {
    Name               string
    Domain             string
    Operation          string
    Description        string
    RiskLevel          string
    RequiredPermission string
    DefaultWhitelisted bool
    Enabled            bool
    ArgsSchema         map[string]interface{}
    OutputSchema       map[string]interface{}
    Tags               []string
    Handler            ToolHandler
}
```

**缺失字段清单**:

| 缺失字段 | 影响 |
|:---|:---|
| `Capability` | 无法按能力语义做高层工具聚合 |
| `Aliases` | 中文别名匹配缺失，用户说"签名检测包"无法映射到 `Package.Sign` |
| `ObjectTypes` | 页面上下文对象匹配（如 `detection_package`）无法工作 |
| `PageRoutes` | `ToolSelector` 的 `page_route_match` 评分因子无法生效 |
| `AutoCallable` | agent-runtime 无法知道哪些工具可自动调用（不需要模型显式决策） |
| `RequiresApproval` | 与 `DefaultWhitelisted` 语义不完全对称；设计文档要求审批模式独立控制 |
| `Idempotent` | agent-runtime 重试策略无法区分幂等/非幂等工具 |
| `DefaultTimeout` | 所有工具硬编码 30s，无法按工具差异化 |
| `ServiceBinding` | 开发者无法追溯工具对应的 service/repository 函数 |
| `Risk` (枚举类型) | 当前用 `string`，设计文档要求 `ToolRisk` 枚举：readonly/low/medium/high/critical |

**补充建议**:  
将 `ToolSpec` 扩展为设计文档定义的完整结构。`Domain` 和 `Operation` 应使用强类型枚举 `ToolDomain` / `ToolOperation`，而非裸 `string`。

---

### 2.2 工具命名不一致

**设计文档规范**: `Domain.Operation`，如 `Host.Get`、`Package.Sign`、`Detection.Alert.List`

**当前实现**:

| 当前名称 | 设计文档名称 | 问题 |
|:---|:---|:---|
| `Host.GetDetail` | `Host.Get` | 命名不一致，`GetDetail` 不是标准 Operation |
| `Host.FindOffline` | 设计文档无此工具 | 应为 `Host.AgentStatus.Get` 或作为查询过滤参数 |
| `Detection.Alert.List` | 一致 | ✓ |
| `Task.GetDetail` | `Task.Get` | 命名不一致 |
| `Vulnerability.List` | 一致 | ✓ |

**补充建议**:  
统一所有工具命名为 `Domain.Operation[.SubOperation]` 格式。现有不一致的工具名必须修正。

---

### 2.3 IntentRouter 缺少 LLM 降级

**设计文档要求**（第 6.1 节）:

> 规则置信度低时，调用一次轻量 LLM 分类，只返回 JSON，不带工具。

以及 IntentResult 字段：

```go
type IntentResult struct {
    Domains          []ToolDomain
    Operations       []ToolOperation
    ObjectTypes      []string
    ObjectIDs        []string
    Keywords         []string
    ExplicitToolName string
    RiskHint         ToolRisk
    NeedWrite        bool
    NeedApproval     bool
    Confidence       float64
    Reason           string
}
```

**当前实现**（`intent_router.go`）:

```go
type IntentResult struct {
    Domains    []string
    Action     string
    Object     string
    Confidence float64
}
```

**缺失**:

| 缺失项 | 影响 |
|:---|:---|
| LLM 分类降级 | 规则置信度低时无法提升分类准确性 |
| `Operations` | 只有单一 `Action`，无法表达复合操作 |
| `ObjectTypes` / `ObjectIDs` | 无法从用户消息中提取目标对象 |
| `Keywords` | 无法传递给 ToolSelector 做关键词匹配 |
| `ExplicitToolName` | 用户明确说"调用 Package.Sign"时无法直接命中 |
| `RiskHint` | 无法根据用户意图推断风险级别 |
| `NeedWrite` / `NeedApproval` | ToolSelector 需要这两个字段来过滤写操作和高风险工具 |
| `Reason` | 无法向用户解释意图分类的依据 |
| `llmClientFn` | IntentRouter 不持有 LLM 客户端工厂 |

**补充建议**:  
IntentRouter 应增加 `llmClientFn` 字段。当规则置信度 < 0.5 时，调用 LLM 做一次轻量分类（只返回 JSON，不注入工具）。IntentResult 结构体需要对齐设计文档。

---

### 2.4 ToolSelector 评分因子不完整

**设计文档要求**（第 6.3 节）:

```text
score =
  0.35 * domain_match
+ 0.20 * operation_match
+ 0.15 * keyword_match
+ 0.10 * page_route_match
+ 0.10 * context_object_match
+ 0.05 * recent_usage_match
+ 0.05 * risk_fit
```

**当前实现**（`tool_selector.go` `scoreTool`）:

| 因子 | 设计权重 | 当前实现 | 状态 |
|:---|:---|:---|:---|
| domain_match | 0.35 | 有 | ✓ |
| operation_match | 0.20 | 有 | ✓ |
| keyword_match | 0.15 | 有（但只匹配 name/description） | 部分（缺少 Aliases/Tags 匹配） |
| page_route_match | 0.10 | 有（但 PageRoute 从未传入） | 无效（ToolSpec 无 PageRoutes） |
| context_object_match | 0.10 | 有 | ✓ |
| recent_usage_match | 0.05 | **无** | ✗ |
| risk_fit | 0.05 | 有（只加分 readonly） | 部分 |

**补充建议**:  
1. 增加 `recent_usage_match` 因子，需要 `ToolCallRepository` 提供最近调用统计。
2. `keyword_match` 应覆盖 `Aliases` 和 `Tags`。
3. `page_route_match` 需要 ToolSpec 有 `PageRoutes` 字段。

---

### 2.5 Tool.Search 元工具未实现

**设计文档要求**（第 6.4 节）:

> Tool.Search 是常驻工具，返回工具说明列表。采用"两段式扩展"：
> 1. runtime 中模型调用 Tool.Search
> 2. Orchestrator 读取 expansion_requested
> 3. 调 ToolSelector.Expand()
> 4. 构造带前一段摘要的新 runtime
> 5. 第二段 runtime 注入 expanded tools 继续完成任务

**当前实现**（`tool_gateway.go` `handleToolSearch`）:

```go
func (g *ToolGateway) handleToolSearch(ctx context.Context, req ToolRequest) (ToolResponse, error) {
    query, _ := req.Args["query"].(string)
    g.logger.Info("tool search requested", zap.String("query", query))
    return ToolResponse{
        Status: "success",
        Data: map[string]interface{}{
            "expansion_requested": true,
            "query":               query,
        },
        CallID: req.CallID,
    }, nil
}
```

**缺失**:

1. `Tool.Search` 没有实际调用 `ToolCatalog.Search()` 返回工具列表
2. Orchestrator 没有处理 `expansion_requested` 的逻辑
3. 没有第二段 runtime 的构造和恢复流程
4. `ToolSearchArgs` / `ToolSearchResult` / `ToolSearchItem` 结构体未定义
5. `Tool.Search` 没有注册到 ToolRegistry

**补充建议**:  
需要完整实现两段式扩展。第一段 runtime 中 Tool.Search 调用 `ToolCatalog.Search()`，返回匹配工具列表。Orchestrator 检测到 expansion_requested 后，调用 `ToolSelector.Expand()` 扩展工具集，构造新的 runtime 继续执行。

---

### 2.6 RuntimeFactory 功能不完整

**设计文档要求**（第 4.2 节）:

```go
type RuntimeFactory struct {
    configRepo       ConfigRepository
    vectorService    ExperienceVectorService
    reflectionRepo   ReflectionRepository
    catalog          *ToolCatalog
    selector         *ToolSelector
    gatewayFactory   *ToolGatewayFactory
    promptFactory    *PromptProviderFactory
    hookSinkFactory  *HookSinkFactory
}

func (f *RuntimeFactory) Build(ctx context.Context, req RuntimeBuildRequest) (*RuntimeBuildResult, error)
```

Build 函数应完成：
1. 构建 LLM 客户端
2. 加载用户上下文
3. 调用 ToolSelector 选择工具
4. 创建 ToolGateway
5. 创建 HookSink
6. 创建 PromptProvider
7. 创建 agentruntime.Runtime 并返回

**当前实现**（`runtime_factory.go`）:

```go
type RuntimeFactory struct {
    configRepo *repository.ConfigRepository
    logger     *zap.Logger
}

func (f *RuntimeFactory) BuildLLMClient(ctx context.Context) (*llm.LLMClient, error)
```

只构建 LLM 客户端，**不创建 agentruntime.Runtime**。Runtime 的创建散落在 `orchestrator.go` 的 `runAgentRuntime()` 方法中。

**缺失**:

| 缺失项 | 影响 |
|:---|:---|
| `catalog` / `selector` | RuntimeFactory 不感知工具选择 |
| `gatewayFactory` | 每次手动构造 ToolGateway |
| `promptFactory` | 每次手动构造 PromptProvider |
| `hookSinkFactory` | 每次手动构造 HookSink |
| `vectorService` / `reflectionRepo` | 经验和反思数据无法注入 |
| `Build()` 方法 | Orchestrator 承担了 RuntimeFactory 的职责 |
| `RuntimeBuildRequest` / `RuntimeBuildResult` | 缺少标准化的构建请求和结果 |

**补充建议**:  
将 `orchestrator.go` 中 `runAgentRuntime()` 的 runtime 构建逻辑抽取到 `RuntimeFactory.Build()`。Orchestrator 只负责：
1. 调用 IntentRouter
2. 调用 ToolSelector
3. 调用 RuntimeFactory.Build()
4. 调用 runtime.Run()
5. 处理结果和事件

---

### 2.7 Agent 现场查询工具缺失

**设计文档要求**（第 14.1 节）:

| 工具名 | 绑定函数 |
|:---|:---|
| `Agent.Process.List` | server gRPC `ExecuteTool(GetRunningProcesses)` |
| `Agent.Process.Tree` | server gRPC `ExecuteTool(GetProcessTree)` |
| `Agent.Network.List` | server gRPC `ExecuteTool(GetNetworkConnections)` |
| `Agent.File.OpenList` | server gRPC `ExecuteTool(GetOpenFiles)` |
| `Agent.Log.Query` | server gRPC `ExecuteTool(QueryHistoricalLogs)` |

**当前实现**: 未在 `api-server/internal/assistant/tools/` 中注册任何 Agent 工具。

> 注：`api-server/internal/llm/adapters/tool_descriptors.go` 中有 `AegisTools` 定义了这 6 个工具，但仅供 AI 分析页使用，未注册到 Assistant ToolRegistry。

**补充建议**:  
新增 `api-server/internal/assistant/tools/agent_tools.go`，注册 Agent.* 工具。实现需要通过 server gRPC `ExecuteTool` 转发。可复用 `llm/adapters/tool_gateway_adapter.go` 中的 gRPC 调用模式。

---

### 2.8 Investigation 工具缺失

**设计文档要求**（第 14.6 节）:

| 工具名 | 绑定函数 |
|:---|:---|
| `Investigation.HostAttack.Plan` | `HostAttackInvestigationService.BuildPlan` |
| `Investigation.HostAttack.Analyze` | `HostAttackInvestigationService.AnalyzeHostAttack` |
| `Investigation.HostAttack.AnalyzeWithExternal` | `HostAttackInvestigationService.AnalyzeHostAttackWithExternal` |
| `Investigation.Evidence.CollectAegis` | `EvidenceCollector.CollectAegisEvidence` |
| `Investigation.Evidence.CollectAgent` | `EvidenceCollector.CollectAgentEvidence` |
| `Investigation.Timeline.Build` | `AttackTimelineBuilder.Build` |
| `Investigation.EntryPoint.Infer` | `EntryPointInferer.Infer` |
| `Investigation.AttackPath.Build` | `AttackPathBuilder.Build` |
| `Investigation.CompromiseScore.Calculate` | `CompromiseScorer.Calculate` |
| `Investigation.Report.Generate` | `InvestigationReportBuilder.Generate` |

**当前实现**: 
- `investigation_service.go` 有 `HostAttackInvestigationService`，但只实现了 `CreateInvestigation()`（一次性全部执行）
- 没有拆分为独立的子工具（Plan / Analyze / Evidence / Timeline 等）
- 没有注册到 ToolRegistry

**补充建议**:  
1. 将 `HostAttackInvestigationService` 的子步骤拆分为独立工具
2. 新增 `api-server/internal/assistant/tools/investigation_tools.go`
3. 注册 Investigation.* 工具到 ToolCatalog

---

### 2.9 ExternalMCP 工具缺失

**设计文档要求**（第 14.5 节）:

| 工具名 | 绑定函数 |
|:---|:---|
| `ExternalMCP.Source.List` | `ExternalMCPSourceService.ListSources` |
| `ExternalMCP.Source.GetSchema` | `ExternalMCPSourceService.GetSchema` |
| `ExternalMCP.Source.TestConnection` | `ExternalMCPSourceService.TestConnection` |
| `ExternalMCP.Query` | `ExternalMCPSourceService.Query` |
| `ExternalMCP.MultiQuery` | `ExternalMCPSourceService.MultiQuery` |
| `ExternalMCP.Analyze` | `ExternalMCPContextBuilder.BuildAnalysisContext` |

**当前实现**: 
- `external_mcp_service.go` 有 `ExternalMCPSourceService`，但只有 CRUD 和 placeholder 的 `TestConnection` / `SyncSchema`
- 没有 `Query` / `MultiQuery` / `Analyze` 能力
- 没有注册到 ToolRegistry

**补充建议**:  
新增 `api-server/internal/assistant/tools/external_mcp_tools.go`，实现查询类工具并注册。

---

### 2.10 其他缺失工具

| 工具类别 | 设计文档要求 | 当前状态 |
|:---|:---|:---|
| `Notification.List` | `NotificationService.List(filter)` | 未实现 |
| `Detection.RuntimeEvent.List` | `RuntimeEventQueryService.List(ctx,q)` | 未实现 |
| `Detection.ToolCall.List` | `ToolCallRepository.List(page,pageSize,filters)` | 未实现 |
| `Config.LLM.Test` | `ConfigService.TestLLMConnection` | 未实现 |
| `Audit.Log.GetStats` | `AuditLogRepo.GetStats()` | 未实现 |
| `Package.Draft.Explain` | 解析草稿并返回解释 | 未实现 |
| `Package.Build.ExplainFailure` | 读取 build error 并给出修复建议 | 未实现 |
| `Package.Allowlist.CheckPackage` | `DetectionPackagePolicyService.CheckAllowlistAgainstDraft` | 未实现 |

---

### 2.11 审批暂停与恢复缺失

**设计文档要求**（第 18.4 节）:

```go
func (o *Orchestrator) PauseForApproval(ctx context.Context, runID string, approval *model.AssistantApproval) error
func (o *Orchestrator) ResumeAfterApproval(ctx context.Context, req ResumeAfterApprovalRequest) (*RunResult, error)
func (g *ApprovalGate) ExecuteApprovedTool(ctx context.Context, approval *model.AssistantApproval) (*ToolExecutionResult, error)
```

审批后继续运行链路：
```text
ToolGateway.Call → ApprovalGate.Evaluate = require approval
  → 创建 assistant_approvals
  → 当前 run 标记 waiting_approval
  → SSE approval_required
  → Orchestrator 暂停本轮 runtime

用户批准
  → ApprovalGate.Approve
  → ExecuteApprovedTool 执行原工具
  → 保存 tool result
  → Orchestrator.ResumeAfterApproval
  → 构造带"刚才工具结果"的新 TaskInput
  → agent-runtime 继续下一步
```

**当前实现**: 
- `approval_gate.go` 有 `Evaluate` / `CreateApproval` / `Approve` / `Reject` / `MarkExecuted`
- **但没有** `PauseForApproval` / `ResumeAfterApproval`
- 当前审批后 agent-runtime 无法继续，工具调用会卡在 `approval_required` 状态
- 没有 `ExecuteApprovedTool` 函数

**补充建议**:  
1. `ToolGateway.Call` 遇到审批时，需要将 run 状态标记为 `waiting_approval` 并暂停 runtime
2. 用户批准后，`ExecuteApprovedTool` 执行原工具，然后 `ResumeAfterApproval` 以新的 TaskInput 恢复 runtime
3. 需要设计 runtime 暂停/恢复机制（可能需要保存中间状态或使用 context 取消后重建）

---

### 2.12 SSE 事件不完整

**设计文档要求**（第 19 节）:

| SSE 事件 | 当前状态 |
|:---|:---|
| `intent_detected` | ✓ 已实现 |
| `tools_selected` | ✓ 已实现（但格式不完全匹配设计文档） |
| `tool_search` | ✗ 未实现 |
| `tool_expansion` | ✗ 未实现 |
| `tool_call_started` | ✓ 已实现 |
| `approval_required` | ✓ 已实现 |
| `business_object` | ✗ 未实现 |
| `run_waiting_approval` | ✗ 未实现 |

**补充建议**:  
增加 `EventToolSearch` / `EventToolExpansion` / `EventBusinessObject` / `EventRunWaitingApproval` 事件类型及其 payload。

---

### 2.13 agent-runtime ToolPolicy 接口未对接

**agent-runtime 定义**（`core/types.go`）:

```go
type ToolPolicy interface {
    Evaluate(ctx context.Context, req ToolPolicyRequest) (ToolPolicyDecision, error)
}

type ToolPolicyRequest struct {
    TaskID    string
    StepID    string
    ToolName  string
    Args      map[string]any
    RiskLevel RiskLevel
}
```

agent-runtime 支持通过 `WithToolPolicy(policy)` 注入工具策略评估器。

**当前实现**: 
- `risk_policy.go` 有 `RiskPolicy`，但不是 agent-runtime 的 `ToolPolicy` 接口
- `approval_gate.go` 的 `Evaluate` 直接在 `ToolDispatcher.Dispatch` 中调用，不经过 agent-runtime
- agent-runtime 无法在执行前拦截高风险工具调用

**补充建议**:  
实现 `agentruntime.ToolPolicy` 接口，将 `RiskPolicy` 和 `ApprovalGate` 的评估逻辑封装其中，通过 `WithToolPolicy()` 注入 agent-runtime。这样 agent-runtime 在每次工具调用前会先调用 `ToolPolicy.Evaluate`，返回 `PolicyAllow` / `PolicyDeny` / `PolicyRequireApproval`。

---

### 2.14 Orchestrator 主流程未对齐设计文档

**设计文档流程**（第 2.1 节）:

```text
用户消息
  → ContextLoader 加载页面对象和会话上下文
  → IntentRouter 判断领域、动作、对象、风险
  → ToolSelector 从 ToolCatalog 检索候选工具
  → RuntimeFactory 用 selected_tools 创建 agent-runtime
  → agent-runtime.Run()
  → AssistantToolGateway 执行工具或创建审批
  → 写 assistant_* 审计表和业务表
```

**当前实现**（`orchestrator.go` `Run()`）:

```text
用户消息
  → ContextLoader.ResolveSession() ← 部分对齐
  → IntentRouter.Classify() ← 部分对齐（无 LLM 降级）
  → ToolSelector.Select() ← 部分对齐（评分因子不完整）
  → isComplexTask() 判断 ← 设计文档无此分支
    → runDirectLLM() ← 设计文档无此路径
    → runAgentRuntime() ← 部分对齐
      → 手动构造 LLMAdapter / ToolGateway / HookSink / PromptProvider
      → agentruntime.New(...)
      → runtime.Run()
```

**差异**:

1. `isComplexTask()` / `runDirectLLM()` 在设计文档中不存在。设计文档所有对话都走 agent-runtime，简单任务由 agent-runtime 内部处理。
2. Runtime 构建逻辑散落在 Orchestrator 中，应集中在 RuntimeFactory。
3. 工具网关存在两个实现（`ToolGateway` 和 `AssistantToolGatewayAdapter`），职责重叠。

**补充建议**:  
两种方案：
- **方案 A**: 对齐设计文档，所有任务统一走 `RuntimeFactory.Build()` + `agent-runtime.Run()`，由 agent-runtime 内部处理简单/复杂任务。
- **方案 B**: 保留 `runDirectLLM` 路径作为简单任务优化（降低 token 消耗），但需要在设计文档中明确。

推荐方案 B 并更新设计文档，因为简单问候走 agent-runtime 会浪费 token。

---

### 2.15 双网关冗余

**当前实现有两个 ToolGateway**:

1. `tool_gateway.go`: `ToolGateway` + `ToolGatewayAdapter`（实现 `CallAgentTool` 接口）
2. `adapter_tool_gateway.go`: `AssistantToolGatewayAdapter`（实现 `agentruntime.ToolGateway` 接口）

两者都桥接 `ToolDispatcher`，但接口不同：
- `ToolGateway.Call` 接受自定义 `ToolRequest`
- `AssistantToolGatewayAdapter.Call` 接受 `agentruntime.ToolRequest`

Orchestrator 使用的是 `AssistantToolGatewayAdapter`。

**补充建议**:  
删除 `ToolGateway` 和 `ToolGatewayAdapter`（`tool_gateway.go`），只保留 `AssistantToolGatewayAdapter`。或者将 `ToolGateway` 改为直接实现 `agentruntime.ToolGateway` 接口。

---

### 2.16 缺少的服务下沉

**设计文档要求新增的服务**:

| 服务 | 文件路径 | 状态 |
|:---|:---|:---|
| `DetectionQueryService` | `api-server/internal/service/detection_query_service.go` | 未创建 |
| `DetectionPolicyService` | `api-server/internal/service/detection_policy_service.go` | 未创建 |
| `SigmaRuleManagementService` | `api-server/internal/service/sigma_rule_management_service.go` | 未创建 |
| `DetectionPackageGenerationService` | `api-server/internal/service/detection_package_generation_service.go` | 未创建 |
| `ConfigService` (LLM Test/Update) | `api-server/internal/service/config_service.go` | 未创建 |

这些服务需要从 handler 下沉到 service 层，供工具 handler 和页面 handler 共同调用。

---

## 3. 优先级排序

### P0 - 必须修复（阻塞核心流程）

1. **ToolSpec 字段补齐**（2.1）— 所有工具注册依赖完整 ToolSpec
2. **RuntimeFactory 完整实现**（2.6）— agent-runtime 创建逻辑必须集中
3. **Tool.Search 完整实现**（2.5）— 两段式扩展是工具检索的核心机制
4. **审批暂停与恢复**（2.11）— 高风险工具无法审批后继续

### P1 - 强烈建议（影响工具覆盖率）

5. **Agent 现场查询工具**（2.7）— 主机取证必备
6. **Investigation 工具**（2.8）— 攻击研判核心能力
7. **IntentRouter LLM 降级**（2.3）— 复杂意图识别准确性
8. **ToolPolicy 接口对接**（2.13）— agent-runtime 原生风险拦截

### P2 - 建议补充（完善工具生态）

9. **ExternalMCP 工具**（2.9）— 外部数据源查询
10. **服务下沉**（2.16）— 工具实现依赖 service 层
11. **工具命名统一**（2.2）— 开发一致性
12. **SSE 事件补全**（2.12）— 前端展示
13. **其他缺失工具**（2.10）— 按需补充

### P3 - 架构优化

14. **Orchestrator 流程对齐**（2.14）— 设计文档一致性
15. **双网关冗余**（2.15）— 代码简洁性
16. **ToolSelector 评分完善**（2.4）— 工具选择准确性

---

## 4. agent-runtime 侧需要确认的接口

以下能力需要确认 `agent-runtime` (`/code/agent-runtime/`) 是否已支持或需要扩展：

| 需求 | agent-runtime 现状 | 需要的动作 |
|:---|:---|:---|
| `ToolPolicy` 接口 | 已定义（`core/types.go`），支持 `WithToolPolicy()` | 需要 Aegis 侧实现 |
| 运行中动态追加工具 | 不支持（设计文档说"两段式扩展"规避此需求） | 无需修改 |
| `ToolDescriptor.AutoCallable` | 已有字段 | 需要 Aegis 侧正确填充 |
| `ToolDescriptor.RequiresApproval` | 已有字段 | 需要 Aegis 侧正确填充 |
| `ToolDescriptor.RiskLevel` | 已有枚举（ReadOnly/Low/High/Dangerous） | 需要映射 Aegis 的 5 级风险 |
| 暂停/恢复 runtime | 不支持 | 需要评估：context 取消后重建，或增加 Pause/Resume |
| `ToolDescriptor.Idempotent` | 已有字段 | 需要 Aegis 侧填充 |
| `ToolDescriptor.DefaultTimeout` | 已有字段 | 需要 Aegis 侧填充 |

### 风险等级映射

设计文档定义了 5 级风险，agent-runtime 只有 4 级：

| 设计文档 | agent-runtime | 映射方案 |
|:---|:---|:---|
| `readonly` | `RiskReadOnly` | 直接映射 |
| `low` | `RiskLow` | 直接映射 |
| `medium` | `RiskLow` | 合并到 Low，或请求 agent-runtime 增加 Medium |
| `high` | `RiskHigh` | 直接映射 |
| `critical` | `RiskDangerous` | 直接映射 |

**建议**: 向 agent-runtime 提 PR 增加 `RiskMedium`，或在 Aegis 侧用 `RiskHigh` 表示 `medium` + `high`，通过 `ToolPolicy` 区分。

---

## 5. 总结

当前实现已完成 V6.0 智能助手的基础框架（会话管理、SSE 流式、基本意图识别、工具注册、agent-runtime 集成），但对照设计文档存在 **16 项缺漏**，其中 **4 项 P0 级** 阻塞核心流程。

最关键的三个问题是：
1. **RuntimeFactory 未完整实现** — runtime 构建逻辑散落在 Orchestrator 中
2. **Tool.Search 两段式扩展未实现** — 工具动态发现机制缺失
3. **审批暂停/恢复未实现** — 高风险工具审批后无法继续执行

建议按 P0 → P1 → P2 → P3 的优先级逐步补齐，每个优先级完成后进行一次 build 验证。
