# V5.7 智能体优化设计 — Agent-Runtime 集成方案

**版本**: 5.7
**日期**: 2026-05-12
**状态**: 设计中

---

## 1. 现状分析

### 1.1 当前ReAct智能体架构

核心文件：
- `api-server/internal/llm/react_agent.go` — 手写ReAct循环引擎（~750行）
- `api-server/internal/api/handler/ai_analysis_handler.go` — Session管理与工具执行
- `api-server/internal/llm/prompts.go` — 系统提示词和工具描述
- `api-server/internal/llm/sse_writer.go` — SSE事件输出
- `frontend/src/api/aiAnalysis.ts` — 前端SSE事件消费
- `frontend/src/views/detection/AIAnalysis.vue` — AI分析页面

### 1.2 已知问题

| # | 问题 | 位置 | 影响 |
|:--|:---|:---|:---|
| 1 | `forceFinalAnswerAfterIterations = 50` | react_agent.go:53 | 浪费token，简单分析循环过多 |
| 2 | `maxNoActionIterations = 2` | react_agent.go:55 | LLM仅2次无动作就被强制结束 |
| 3 | `maxObservationChars = 12000` 硬截断 | react_agent.go:52 | 丢失结构化输出的尾部数据 |
| 4 | `normalizeToolName()` 模糊匹配 | react_agent.go:393-446 | 前缀/子串匹配可能误判 |
| 5 | `inferToolFromInput()` 关键词推断 | react_agent.go:579-603 | 简单匹配可能误判工具 |
| 6 | JSON流式提取不可靠 | react_agent.go:449-516 | 流式场景下可能漏解析 |
| 7 | Session仅内存存储 | ai_analysis_handler.go | API Server重启丢失活跃会话 |
| 8 | 无可观测性指标 | 全局 | 无法量化分析效率和失败率 |

---

## 2. 优化方案：集成 agent-runtime SDK

### 2.1 方案概述

放弃对 `react_agent.go` 的增量改造，直接集成 `agent-runtime` SDK（`github.com/chenchen511/agent-runtime v0.1.0`）作为新的执行引擎。

**agent-runtime 提供的完整生命周期**：

```
Plan → Execute Steps (ReAct per step) → Reflect → Audit → Correct → Summarize
```

**对比当前手写循环**：

| 能力 | 当前 react_agent.go | agent-runtime |
|:---|:---|:---|
| 规划 | 无 | LLM生成结构化Plan，含Step依赖 |
| 执行 | 单循环ReAct | 每Step独立ReAct循环，互不干扰 |
| 反思 | 无 | Step失败时自动Reflect，分析根因 |
| 审计 | 无 | 每N步自动Audit，检测目标偏移 |
| 纠正 | 无 | Audit/Reflect触发后自动修正Plan |
| 总结 | 手写parseFinalAnswer | LLM生成结构化FinalAnswer |
| 可观测 | 无 | 18种HookEvent |
| 工具策略 | 无 | ToolPolicy接口，支持拒绝/替换/审批 |
| 经验复用 | 无 | ExperienceProvider接口 |

### 2.2 问题映射

| 原问题 | agent-runtime 解决方案 |
|:---|:---|
| ① forceFinalAnswer=50 | `RuntimeConfig.MaxTotalTurns=20`，全局总轮次上限 |
| ② maxNoAction=2 | `RuntimeConfig.MaxNoProgressTurns=3`，无进展容忍度 |
| ③ maxObservationChars硬截断 | `textutil.Truncate()` 智能截断，JSON数组头尾保留 |
| ④ normalizeToolName模糊匹配 | 消除 — JSON Action格式使用精确工具名 |
| ⑤ inferToolFromInput关键词推断 | 消除 — ToolDescriptor定义精确ArgsSchema |
| ⑥ JSON流式提取不可靠 | 消除 — agent-runtime使用非流式Complete() |
| ⑦ Session仅内存存储 | 不变 — 独立于agent-runtime，后续单独优化 |
| ⑧ 无可观测性 | HookSink接口提供18种事件类型 |

---

## 3. 架构设计

### 3.1 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                    AIAnalysisHandler                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │ Session Mgmt │  │ SSE Writer   │  │ ToolExecutor (gRPC)  │  │
│  └──────┬───────┘  └──────┬───────┘  └──────────┬───────────┘  │
│         │                 │                      │              │
│  ┌──────▼─────────────────▼──────────────────────▼───────────┐  │
│  │                    Adapters Layer                         │  │
│  │  ┌────────────┐ ┌────────────┐ ┌────────────┐            │  │
│  │  │LLMClient   │ │ToolGateway │ │HookSink    │            │  │
│  │  │Adapter     │ │Adapter     │ │SSE         │            │  │
│  │  └────────────┘ └────────────┘ └────────────┘            │  │
│  │  ┌────────────┐ ┌────────────┐ ┌────────────┐            │  │
│  │  │Prompt      │ │Tool        │ │Runtime     │            │  │
│  │  │Provider    │ │Descriptors │ │Factory     │            │  │
│  │  └────────────┘ └────────────┘ └────────────┘            │  │
│  └────────────────────────┬──────────────────────────────────┘  │
│                           │                                     │
│  ┌────────────────────────▼──────────────────────────────────┐  │
│  │                 agent-runtime SDK                         │  │
│  │  Runtime.Run(ctx, TaskInput) → TaskResult                │  │
│  │  Plan → Execute → Reflect → Audit → Correct → Summarize  │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### 3.2 适配器层设计

Aegis通过6个适配器文件桥接现有基础设施到agent-runtime接口：

#### 3.2.1 LLMClientAdapter

**文件**: `api-server/internal/llm/adapters/llm_client_adapter.go`

**职责**: 包装 `llm.LLMClient`，实现 `core.LLMClient` 接口。

```go
type LLMClientAdapter struct {
    client   *llm.LLMClient
    alertCtx map[string]interface{}
}
```

**核心逻辑**:
- `Complete(ctx, req LLMRequest) (LLMResponse, error)`
  - 将 `core.LLMMessage` 转换为 `llm.Message`
  - 根据 `req.Purpose` 设置temperature：plan=0.4, react=0.7, audit=0.3, reflect=0.3, correct=0.4, summarize=0.3
  - 对 plan/react/summarize purpose，注入alert context到system prompt
  - 调用 `client.ChatCompletionWithMessages()` 返回结果

**Temperature映射表**:

| Purpose | Temperature | 原因 |
|:---|:---|:---|
| plan | 0.4 | 规划需要一定创造性但不能太随机 |
| react | 0.7 | 当前react_agent使用的温度 |
| audit | 0.3 | 审计需要精确判断 |
| reflect | 0.3 | 反思需要精确分析 |
| correct | 0.4 | 纠正需要一定灵活性 |
| summarize | 0.3 | 总结需要准确 |

#### 3.2.2 ToolGatewayAdapter

**文件**: `api-server/internal/llm/adapters/tool_gateway_adapter.go`

**职责**: 包装现有 `ToolExecutor`（gRPC调度），实现 `core.ToolGateway` 接口。

```go
type ToolGatewayAdapter struct {
    serverClient   *grpc.ServerClient
    defaultHostIDs []string
}
```

**核心逻辑**:
- `Call(ctx, req ToolRequest) (ToolResponse, error)`
  - 参数归一化：camelCase → snake_case（从 ai_analysis_handler.go 迁移 `normalizeArgs` 逻辑）
  - 默认host_id填充（从 ai_analysis_handler.go 迁移）
  - QueryHistoricalLogs时间参数处理（从 ai_analysis_handler.go 迁移）
  - 调用 `serverClient.ExecuteTool()` 执行gRPC调用
  - 返回 `ToolResponse`（含状态、内容、摘要、耗时）
- `Cancel(ctx, taskID, callID) error` → no-op（gRPC同步调用，60s超时自动终止）

**需要迁移的函数**:
- `normalizeArgs()` — ai_analysis_handler.go 中的 camelCase→snake_case 转换
- `isPlaceholderToolValue()` — 占位符值检测
- host_id 默认值逻辑
- QueryHistoricalLogs 时间参数注入

#### 3.2.3 SSEHookSink

**文件**: `api-server/internal/llm/adapters/hook_sink_sse.go`

**职责**: 实现 `core.HookSink` 接口，将 Runtime 事件桥接到 SSE。

```go
type SSEHookSink struct {
    writer    *llm.SSEWriter
    collector *handler.SSEResponseCollector
}
```

**事件映射表（18种HookEvent → SSE）**:

| HookEventType | SSE Event Type | Payload提取 | 说明 |
|:---|:---|:---|:---|
| `task_started` | (无) | 内部跟踪 | 任务开始 |
| `experience_loaded` | `thinking` | "Loading experience..." | 经验加载 |
| `plan_created` | `thinking` + `plan` | `Snapshot.CurrentPlan` → 步骤列表 | **新增SSE类型** |
| `step_started` | `thinking` | "Step N: {title}" | 步骤开始 |
| `model_call_started` | (无) | 内部跟踪 | 模型调用开始 |
| `model_call_finished` | `thinking` | `ModelCallRecord.OutputSummary` | 模型调用完成 |
| `tool_call_started` | `tool_call` | `ToolCallRecord.ToolName`, `.CallID`, `.ArgsSummary` | 工具调用开始 |
| `tool_call_finished` | `tool_result`/`tool_error` | `ToolCallRecord.Status`, `.ResultSummary`, `.ErrorMessage` | 工具调用完成 |
| `step_completed` | `thinking` | "Step completed: {result}" | 步骤完成 |
| `step_failed` | `thinking` | "Step failed: {error}" | 步骤失败 |
| `audit_started` | `thinking` | "Auditing progress..." | 审计开始 |
| `audit_finished` | `thinking` + `audit` | `AuditResult.Findings`, `.Decision` | **新增SSE类型** |
| `reflection_started` | `thinking` | "Reflecting on failure..." | 反思开始 |
| `reflection_finished` | `thinking` + `reflection` | `ReflectionResult.RootCause`, `.Recommendation` | **新增SSE类型** |
| `correction_applied` | `thinking` + `correction` | `CorrectionResult.Reason`, `.Actions` | **新增SSE类型** |
| `config_changed` | (无) | 内部跟踪 | 配置变更 |
| `task_interrupted` | `error` | "Analysis interrupted" | 任务中断 |
| `task_finished` | `content` + `done` | `TaskResult.FinalAnswer` | 任务完成 |

**新增SSE事件类型**:
- `plan` — 执行计划创建，包含步骤列表
- `audit` — 审计结果
- `reflection` — 反思结果
- `correction` — 计划纠正

#### 3.2.4 PromptProvider

**文件**: `api-server/internal/llm/adapters/prompt_provider.go`

**职责**: 实现 `core.PromptProvider` 接口，提供Aegis专用提示词。

**Purpose对应提示词**:

| Purpose | 提示词内容 |
|:---|:---|
| `PurposePlan` | 安全分析规划提示词，包含工具列表和JSON schema输出格式 |
| `PurposeReact` | 适配的ReAct提示词，JSON Action格式：`{"action":"tool_call","tool_call":{"tool_name":"...","args":{...}}}` |
| `PurposeSummarize` | Attack Graph JSON格式提示词，保留 `attack_graph` + `conclusions` 结构 |
| `PurposeAudit` | 使用agent-runtime内置默认 |
| `PurposeReflect` | 使用agent-runtime内置默认 |
| `PurposeCorrect` | 使用agent-runtime内置默认 |

**关键设计决策**:
- React阶段使用JSON Action格式替代文本格式（Thought/Action/ActionInput），消除normalizeToolName和inferToolFromInput
- Summarize阶段显式要求输出attack_graph JSON格式，保证前端兼容性
- Plan阶段要求输出结构化步骤列表

#### 3.2.5 ToolDescriptors

**文件**: `api-server/internal/llm/adapters/tool_descriptors.go`

**职责**: 定义6个工具的描述符（含JSON Schema）。

```go
var AegisTools = []agentruntime.ToolDescriptor{
    {
        Name:        "GetProcessTree",
        Description: "获取指定主机上指定进程的完整进程树",
        ArgsSchema:  map[string]any{
            "host_id": map[string]any{"type": "string", "description": "主机ID", "required": true},
            "pid":     map[string]any{"type": "number", "description": "进程PID", "required": true},
        },
        RiskLevel: core.RiskReadOnly,
    },
    // ... 其他5个工具
}
```

**工具清单**:

| 工具名 | 必需参数 | 可选参数 | RiskLevel |
|:---|:---|:---|:---|
| GetProcessTree | host_id (string), pid (number) | — | read_only |
| GetNetworkConnections | host_id (string) | pid (number) | read_only |
| GetOpenFiles | host_id (string), pid (number) | — | read_only |
| GetRunningProcesses | host_id (string) | filter (string) | read_only |
| GetUserSessions | host_id (string) | — | read_only |
| QueryHistoricalLogs | host_id (string), start_time (string), end_time (string) | filter (string) | read_only |

#### 3.2.6 RuntimeFactory

**文件**: `api-server/internal/llm/adapters/runtime_factory.go`

**职责**: 组装所有适配器，创建 `agentruntime.Runtime` 实例。

```go
func NewAegisRuntime(
    llmClient *llm.LLMClient,
    serverClient *grpc.ServerClient,
    sseWriter *llm.SSEWriter,
    collector *handler.SSEResponseCollector,
    defaultHostIDs []string,
    alertCtx map[string]interface{},
    maxIterations int,
) (*agentruntime.Runtime, error)
```

**RuntimeConfig映射**:

| RuntimeConfig字段 | Aegis值 | 说明 |
|:---|:---|:---|
| MaxTotalTurns | maxIterations (默认15) | 全局总轮次 |
| MaxPlanSteps | 8 | 最大计划步骤数 |
| MaxStepReactTurns | 6 | 每步最大ReAct轮次 |
| MaxToolCalls | 100 | 单会话工具调用上限 |
| MaxToolCallsPerStep | 10 | 每步工具调用上限 |
| MaxToolFailures | 10 | 工具失败容忍 |
| MaxModelFailures | 5 | 模型失败容忍 |
| MaxParseFailures | 3 | 解析失败容忍 |
| MaxNoProgressTurns | 3 | 无进展容忍（原maxNoActionIterations=2） |
| TaskTimeout | 10min | 任务超时 |
| ModelTimeout | 60s | 模型调用超时 |
| ToolTimeout | 60s | 工具调用超时 |
| HookTimeout | 10s | Hook超时 |
| EnableReflection | true | 启用反思 |
| EnableAudit | true | 启用审计 |
| EnableCorrection | true | 启用纠正 |
| EnableExperience | true | 启用经验（通过ExperienceProvider集成VectorService） |
| AuditEveryNSteps | 3 | 每3步审计一次 |
| MaxAudits | 2 | 最大审计次数 |
| MaxReflections | 3 | 最大反思次数 |
| MaxCorrections | 2 | 最大纠正次数 |
| AllowDynamicNewSteps | true | 允许动态添加步骤 |
| AllowSkipFailedStep | true | 允许跳过失败步骤 |
| AllowBestEffortAnswer | true | 允许尽力回答 |
| AllowHighRiskTools | false | 禁止高风险工具 |
| AllowDangerousTools | false | 禁止危险工具 |

---

## 4. 集成改造

### 4.1 后端改造

#### 4.1.1 go.mod 依赖

```go
require github.com/chenchen511/agent-runtime v0.1.0

// 开发阶段使用本地路径
replace github.com/chenchen511/agent-runtime => /code/agent-runtime
```

#### 4.1.2 ai_analysis_handler.go 改造

**Session结构变更**:
```go
// 改造前
type AISSESion struct {
    // ...
    ReActAgent *llm.ReActAgent
}

// 改造后
type AISSESion struct {
    // ...
    Runtime *agentruntime.Runtime  // 替换 ReActAgent
}
```

**StreamMessage改造**:
```go
// 改造前
session.ReActAgent.Stream(ctx, message, history, sseWriter, alertCtx)

// 改造后
runtime, _ := adapters.NewAegisRuntime(
    session.LLMClient, h.serverClient, sseWriter, collector,
    session.HostIDs, alertCtx, session.MaxIterations,
)
result, _ := runtime.Run(ctx, agentruntime.TaskInput{
    UserInput:    message,
    UserContext:  alertCtx,
    Metadata:     map[string]string{"session_id": sessionID},
})
// HookSink在Run过程中自动通过SSE推送所有中间事件
// Run完成后，task_finished hook自动写入content + done
```

**SSE响应收集器扩展**:
```go
type SSEResponseCollector struct {
    // 现有字段
    Content   string
    Thinking  []string
    ToolCalls []ToolCallRecord
    ToolResults []ToolResultRecord
    Steps     []AgentStep

    // 新增字段
    Plan         *agentruntime.Plan
    Reflections  []agentruntime.ReflectionResult
    Audits       []agentruntime.AuditResult
    Corrections  []agentruntime.CorrectionResult
    Metrics      *agentruntime.RuntimeMetrics
}
```

**持久化增强**:
- `persistAnalysisOutcome()` 从 `TaskResult` 提取更丰富的数据
- 保存 Plan、StepExecutions、Reflections、Audits、Corrections、Metrics

#### 4.1.3 prompts.go 改造

新增3个提示词模板：

- `PlanPromptTemplate` — 安全分析规划提示词
- `ReActJSONPromptTemplate` — JSON Action格式的ReAct提示词
- `SummarizePromptTemplate` — Attack Graph输出格式提示词

保留现有 `ReActPromptTemplate` 用于向后兼容（feature flag路径）。

#### 4.1.4 react_agent.go

保留不删除，用于向后兼容。当feature flag `use_agent_runtime` 关闭时，回退到旧路径。

#### 4.1.5 TaskResult 持久化

agent-runtime 的 `TaskResult` 包含丰富的分析过程数据，需要全部持久化到数据库。

**现有持久化**（仅保存结论）:
- `ai_analysis_session.conclusion` JSONB — 仅保存 attack_graph + conclusions
- `ai_analysis_message` — 保存 Content/Thinking/ToolCalls/ToolResults/Steps

**新增持久化**（保存完整分析过程）:

##### 4.1.5.1 新增数据库模型

**文件**: `api-server/internal/model/agent_execution.go`

```go
// AgentExecution — 单次agent-runtime执行记录
type AgentExecution struct {
    ID              uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    SessionID       string     `gorm:"type:varchar(100);index;not null"`
    TaskID          string     `gorm:"type:varchar(100);uniqueIndex;not null"`
    Status          string     `gorm:"type:varchar(20)"`  // completed/failed/interrupted/limited
    ExitReason      string     `gorm:"type:varchar(50)"`
    FinalAnswer     string     `gorm:"type:text"`
    InitialPlan     JSONB      `gorm:"type:jsonb"`        // agentruntime.Plan
    FinalPlan       JSONB      `gorm:"type:jsonb"`        // agentruntime.Plan (after corrections)
    Completion      JSONB      `gorm:"type:jsonb"`        // agentruntime.CompletionSummary
    Metrics         JSONB      `gorm:"type:jsonb"`        // agentruntime.RuntimeMetrics
    StartedAt       time.Time
    EndedAt         time.Time
    TotalDurationMs int64
    CreatedAt       time.Time
}

// AgentStepExecution — 步骤执行详情
type AgentStepExecution struct {
    ID            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    ExecutionID   uuid.UUID `gorm:"type:uuid;index;not null"`  // FK → AgentExecution
    TaskID        string    `gorm:"type:varchar(100);index"`
    StepID        string    `gorm:"type:varchar(50)"`
    Attempt       int
    Status        string    `gorm:"type:varchar(20)"`  // completed/failed/skipped
    Result        string    `gorm:"type:text"`
    Evidence      JSONB     `gorm:"type:jsonb"`         // []string
    Error         JSONB     `gorm:"type:jsonb"`         // *RuntimeError
    ReactTurns    JSONB     `gorm:"type:jsonb"`         // []ReactTurn
    StartedAt     time.Time
    EndedAt       time.Time
    DurationMs    int64
    CreatedAt     time.Time
}

// AgentReflection — 反思记录
type AgentReflection struct {
    ID              uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    ExecutionID     uuid.UUID `gorm:"type:uuid;index;not null"`
    TaskID          string    `gorm:"type:varchar(100);index"`
    StepID          string    `gorm:"type:varchar(50)"`
    ReflectionID    string    `gorm:"type:varchar(100)"`
    Trigger         string    `gorm:"type:varchar(50)"`  // step_failed
    RootCause       string    `gorm:"type:text"`
    Impact          string    `gorm:"type:text"`
    Recoverable     bool
    Recommendation  string    `gorm:"type:varchar(50)"`  // retry_step/skip_step/correct_plan/summarize_now/fail
    DisableTools    JSONB     `gorm:"type:jsonb"`         // []string
    CorrectionHint  string    `gorm:"type:text"`
    ReusableLesson  string    `gorm:"type:text"`
    CreatedAt       time.Time
}

// AgentAudit — 审计记录
type AgentAudit struct {
    ID              uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    ExecutionID     uuid.UUID `gorm:"type:uuid;index;not null"`
    TaskID          string    `gorm:"type:varchar(100);index"`
    AuditID         string    `gorm:"type:varchar(100)"`
    Trigger         string    `gorm:"type:varchar(50)"`  // step_completed
    Drifted         bool
    RiskLevel       string    `gorm:"type:varchar(20)"`
    Findings        JSONB     `gorm:"type:jsonb"`         // []string
    Decision        string    `gorm:"type:varchar(50)"`  // continue/correct_plan/summarize_now/fail
    CorrectionHint  string    `gorm:"type:text"`
    ShouldExit      bool
    ExitReason      string    `gorm:"type:varchar(50)"`
    CreatedAt       time.Time
}

// AgentCorrection — 计划纠正记录
type AgentCorrection struct {
    ID               uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    ExecutionID      uuid.UUID `gorm:"type:uuid;index;not null"`
    TaskID           string    `gorm:"type:varchar(100);index"`
    CorrectionID     string    `gorm:"type:varchar(100)"`
    Trigger          string    `gorm:"type:varchar(50)"`  // audit/reflection
    FromPlanVersion  int
    ToPlanVersion    int
    Reason           string    `gorm:"type:text"`
    Actions          JSONB     `gorm:"type:jsonb"`         // []CorrectionAction
    Valid            bool
    ValidationErrors JSONB     `gorm:"type:jsonb"`         // []string
    CreatedAt        time.Time
}

// AgentToolCall — 工具调用详情（agent-runtime维度）
type AgentToolCallRecord struct {
    ID            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    ExecutionID   uuid.UUID `gorm:"type:uuid;index;not null"`
    TaskID        string    `gorm:"type:varchar(100);index"`
    StepID        string    `gorm:"type:varchar(50)"`
    CallID        string    `gorm:"type:varchar(100)"`
    ToolName      string    `gorm:"type:varchar(100)"`
    Reason        string    `gorm:"type:text"`
    ArgsSummary   string    `gorm:"type:text"`
    Status        string    `gorm:"type:varchar(20)"`  // success/failed/timeout/cancelled
    ResultSummary string    `gorm:"type:text"`
    ErrorMessage  string    `gorm:"type:text"`
    RiskLevel     string    `gorm:"type:varchar(20)"`
    DurationMs    int64
    StartedAt     time.Time
    EndedAt       time.Time
    CreatedAt     time.Time
}

// AgentModelError — 模型调用错误记录
type AgentModelError struct {
    ID            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    ExecutionID   uuid.UUID `gorm:"type:uuid;index;not null"`
    TaskID        string    `gorm:"type:varchar(100);index"`
    StepID        string    `gorm:"type:varchar(50)"`
    CallID        string    `gorm:"type:varchar(100)"`
    Purpose       string    `gorm:"type:varchar(20)"`  // plan/react/audit/reflect/correct/summarize
    ErrorKind     string    `gorm:"type:varchar(50)"`
    Message       string    `gorm:"type:text"`
    Recoverable   bool
    Model         string    `gorm:"type:varchar(100)"`
    TokensUsed    int
    LatencyMs     int64
    OccurredAt    time.Time
    CreatedAt     time.Time
}
```

##### 4.1.5.2 Repository层

**文件**: `api-server/internal/repository/agent_execution_repository.go`

```go
type AgentExecutionRepository struct {
    db *gorm.DB
}

func (r *AgentExecutionRepository) CreateExecution(exec *model.AgentExecution) error
func (r *AgentExecutionRepository) CreateStepExecution(step *model.AgentStepExecution) error
func (r *AgentExecutionRepository) CreateReflection(refl *model.AgentReflection) error
func (r *AgentExecutionRepository) CreateAudit(audit *model.AgentAudit) error
func (r *AgentExecutionRepository) CreateCorrection(corr *model.AgentCorrection) error
func (r *AgentExecutionRepository) CreateToolCall(tc *model.AgentToolCallRecord) error
func (r *AgentExecutionRepository) CreateModelError(err *model.AgentModelError) error

func (r *AgentExecutionRepository) FindByTaskID(taskID string) (*model.AgentExecution, error)
func (r *AgentExecutionRepository) FindStepsByExecutionID(execID uuid.UUID) ([]*model.AgentStepExecution, error)
func (r *AgentExecutionRepository) FindReflectionsByExecutionID(execID uuid.UUID) ([]*model.AgentReflection, error)
func (r *AgentExecutionRepository) FindAuditsByExecutionID(execID uuid.UUID) ([]*model.AgentAudit, error)
func (r *AgentExecutionRepository) FindCorrectionsByExecutionID(execID uuid.UUID) ([]*model.AgentCorrection, error)
func (r *AgentExecutionRepository) FindToolCallsByExecutionID(execID uuid.UUID) ([]*model.AgentToolCallRecord, error)
func (r *AgentExecutionRepository) FindModelErrorsByExecutionID(execID uuid.UUID) ([]*model.AgentModelError, error)

// RAG查询：获取失败的反思记录用于经验学习
func (r *AgentExecutionRepository) FindFailedReflections(ctx context.Context, limit int) ([]*model.AgentReflection, error)
// RAG查询：获取成功的分析摘要用于相似案例
func (r *AgentExecutionRepository) FindSuccessfulSummaries(ctx context.Context, limit int) ([]*model.AgentExecution, error)
```

##### 4.1.5.3 持久化流程

在 `StreamMessage` 中，`runtime.Run()` 返回 `TaskResult` 后执行：

```go
func (h *AIAnalysisHandler) persistAgentResult(sessionID string, result *agentruntime.TaskResult) {
    // 1. 保存执行记录
    exec := &model.AgentExecution{
        SessionID:       sessionID,
        TaskID:          result.TaskID,
        Status:          string(result.Status),
        ExitReason:      string(result.ExitReason),
        FinalAnswer:     result.FinalAnswer,
        InitialPlan:     marshalJSON(result.InitialPlan),
        FinalPlan:       marshalJSON(result.FinalPlan),
        Completion:      marshalJSON(result.Completion),
        Metrics:         marshalJSON(result.Metrics),
        StartedAt:       result.StartedAt,
        EndedAt:         result.EndedAt,
        TotalDurationMs: result.Metrics.TotalDuration.Milliseconds(),
    }
    h.agentExecRepo.CreateExecution(exec)

    // 2. 保存步骤执行详情
    for _, step := range result.StepExecutions {
        h.agentExecRepo.CreateStepExecution(&model.AgentStepExecution{
            ExecutionID: exec.ID,
            TaskID:      result.TaskID,
            StepID:      step.StepID,
            Attempt:     step.Attempt,
            Status:      string(step.Status),
            Result:      step.Result,
            Evidence:    marshalJSON(step.Evidence),
            Error:       marshalJSON(step.Error),
            ReactTurns:  marshalJSON(step.ReactTurns),
            StartedAt:   step.StartedAt,
            EndedAt:     step.EndedAt,
            DurationMs:  step.EndedAt.Sub(step.StartedAt).Milliseconds(),
        })
    }

    // 3. 保存反思记录
    for _, refl := range result.Reflections {
        h.agentExecRepo.CreateReflection(&model.AgentReflection{
            ExecutionID:    exec.ID,
            TaskID:         result.TaskID,
            StepID:         refl.StepID,
            ReflectionID:   refl.ReflectionID,
            Trigger:        refl.Trigger,
            RootCause:      refl.RootCause,
            Impact:         refl.Impact,
            Recoverable:    refl.Recoverable,
            Recommendation: string(refl.Recommendation),
            DisableTools:   marshalJSON(refl.DisableTools),
            CorrectionHint: refl.CorrectionHint,
            ReusableLesson: refl.ReusableLesson,
            CreatedAt:      refl.CreatedAt,
        })
    }

    // 4. 保存审计记录
    for _, aud := range result.Audits {
        h.agentExecRepo.CreateAudit(&model.AgentAudit{
            ExecutionID:    exec.ID,
            TaskID:         result.TaskID,
            AuditID:        aud.AuditID,
            Trigger:        aud.Trigger,
            Drifted:        aud.Drifted,
            RiskLevel:      string(aud.RiskLevel),
            Findings:       marshalJSON(aud.Findings),
            Decision:       string(aud.Decision),
            CorrectionHint: aud.CorrectionHint,
            ShouldExit:     aud.ShouldExit,
            ExitReason:     string(aud.ExitReason),
            CreatedAt:      aud.CreatedAt,
        })
    }

    // 5. 保存纠正记录
    for _, corr := range result.Corrections {
        h.agentExecRepo.CreateCorrection(&model.AgentCorrection{
            ExecutionID:     exec.ID,
            TaskID:          result.TaskID,
            CorrectionID:    corr.CorrectionID,
            Trigger:         corr.Trigger,
            FromPlanVersion: corr.FromPlanVersion,
            ToPlanVersion:   corr.ToPlanVersion,
            Reason:          corr.Reason,
            Actions:         marshalJSON(corr.Actions),
            Valid:           corr.Valid,
            ValidationErrors: marshalJSON(corr.ValidationErrors),
            CreatedAt:       corr.CreatedAt,
        })
    }

    // 6. 保存工具调用记录
    for _, tc := range result.ToolCalls {
        h.agentExecRepo.CreateToolCall(&model.AgentToolCallRecord{
            ExecutionID:   exec.ID,
            TaskID:        result.TaskID,
            StepID:        tc.StepID,
            CallID:        tc.CallID,
            ToolName:      tc.ToolName,
            Reason:        tc.Reason,
            ArgsSummary:   tc.ArgsSummary,
            Status:        string(tc.Status),
            ResultSummary: tc.ResultSummary,
            ErrorMessage:  tc.ErrorMessage,
            RiskLevel:     string(tc.RiskLevel),
            DurationMs:    tc.EndedAt.Sub(tc.StartedAt).Milliseconds(),
            StartedAt:     tc.StartedAt,
            EndedAt:       tc.EndedAt,
        })
    }

    // 7. 保存模型调用错误
    for _, mc := range result.ModelCalls {
        if mc.Error != "" {
            h.agentExecRepo.CreateModelError(&model.AgentModelError{
                ExecutionID: exec.ID,
                TaskID:      result.TaskID,
                StepID:      mc.StepID,
                CallID:      mc.CallID,
                Purpose:     string(mc.Purpose),
                Message:     mc.Error,
                Model:       mc.Model,
                TokensUsed:  mc.TokensUsed,
                LatencyMs:   mc.Latency.Milliseconds(),
                OccurredAt:  mc.OccurredAt,
            })
        }
    }

    // 8. 更新ai_analysis_session的结论（兼容现有逻辑）
    h.persistAnalysisOutcome(session, result.FinalAnswer)

    // 9. 异步生成embedding并保存到ai_analysis_record（用于RAG）
    go h.saveAnalysisRecordForRAG(session, result)
}
```

#### 4.1.6 RAG 集成与 ExperienceProvider

##### 4.1.6.1 现状分析

现有RAG基础设施：
- `VectorService` — 向量搜索服务（`service/vector_service.go`）
- `EmbeddingService` — OpenAI embedding调用（`service/vector_service.go`）
- `ai_analysis_record` 表 — 存储分析摘要和向量
- 问题：`main.go` 中 `EmbeddingService` 传入 `nil`，RAG功能未实际启用

##### 4.1.6.2 ExperienceProvider 实现

**文件**: `api-server/internal/llm/adapters/experience_provider.go`

实现 `core.ExperienceProvider` 接口，桥接 `VectorService`：

```go
type ExperienceProviderAdapter struct {
    vectorSvc    *service.VectorService
    agentExecRepo *repository.AgentExecutionRepository
    maxItems     int  // 默认5
}

func (p *ExperienceProviderAdapter) Fetch(ctx context.Context, req core.ExperienceRequest) (core.ExperienceResponse, error) {
    // 1. 从VectorService获取相似历史分析
    similar, err := p.vectorSvc.FindSimilarAnalysis(ctx, req.Query, "", 0.7, req.MaxItems)
    if err != nil {
        // RAG不可用时降级：从DB获取最近的成功分析
        return p.fallbackFromDB(ctx, req)
    }

    items := make([]core.ExperienceItem, 0, len(similar))
    for _, s := range similar {
        items = append(items, core.ExperienceItem{
            ID:      s.SessionID,
            Summary: s.Summary,
            Content: formatSimilarCase(s),
            Tags:    []string{"similar_case", "rag"},
            Metadata: map[string]any{
                "similarity": s.Similarity,
                "alert_ids":  s.AlertIDs,
            },
        })
    }

    // 2. 补充失败反思经验
    reflections, _ := p.agentExecRepo.FindFailedReflections(ctx, 3)
    for _, r := range reflections {
        items = append(items, core.ExperienceItem{
            ID:      r.ReflectionID,
            Summary: fmt.Sprintf("历史失败教训: %s", r.RootCause),
            Content: fmt.Sprintf("根因: %s\n影响: %s\n教训: %s", r.RootCause, r.Impact, r.ReusableLesson),
            Tags:    []string{"reflection", "lesson"},
        })
    }

    return core.ExperienceResponse{Items: items}, nil
}

// 降级：从DB获取最近成功分析
func (p *ExperienceProviderAdapter) fallbackFromDB(ctx context.Context, req core.ExperienceRequest) (core.ExperienceResponse, error) {
    execs, _ := p.agentExecRepo.FindSuccessfulSummaries(ctx, req.MaxItems)
    items := make([]core.ExperienceItem, 0, len(execs))
    for _, e := range execs {
        items = append(items, core.ExperienceItem{
            ID:      e.TaskID,
            Summary: truncate(e.FinalAnswer, 200),
            Content: e.FinalAnswer,
            Tags:    []string{"recent_success"},
        })
    }
    return core.ExperienceResponse{Items: items}, nil
}
```

##### 4.1.6.3 分析完成后自动保存RAG记录

在 `persistAgentResult` 末尾异步执行：

```go
func (h *AIAnalysisHandler) saveAnalysisRecordForRAG(session *AISSESion, result *agentruntime.TaskResult) {
    if h.vectorSvc == nil {
        return
    }

    // 构建摘要
    summary := buildAnalysisSummary(result)

    record := &service.AIAnalysisRecord{
        ID:              uuid.New().String(),
        SessionID:       session.SessionID,
        AlertIDs:        marshalJSON(session.AlertIDs),
        HostFilter:      marshalJSON(session.HostFilter),
        InitialQuery:    session.InitialQuery,
        FinalConclusion: result.FinalAnswer,
        Summary:         summary,
    }

    // 生成embedding并保存
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    h.vectorSvc.GenerateAndSaveEmbedding(ctx, record)
}

func buildAnalysisSummary(result *agentruntime.TaskResult) string {
    // 从TaskResult提取关键信息构建摘要：
    // - 计划目标
    // - 完成步骤数/总步骤数
    // - 关键发现（从step results提取）
    // - 威胁等级
    // - 结论摘要
    var b strings.Builder
    if result.InitialPlan != nil {
        b.WriteString("目标: ")
        b.WriteString(result.InitialPlan.Goal)
        b.WriteString("\n")
    }
    b.WriteString(fmt.Sprintf("完成: %d/%d 步骤\n", result.Completion.CompletedSteps, len(result.StepExecutions)))
    b.WriteString(fmt.Sprintf("工具调用: %d次, 模型调用: %d次\n", result.Completion.ToolCalls, result.Completion.ModelCalls))
    if len(result.Reflections) > 0 {
        b.WriteString(fmt.Sprintf("反思: %d次\n", len(result.Reflections)))
    }
    if len(result.Audits) > 0 {
        b.WriteString(fmt.Sprintf("审计: %d次\n", len(result.Audits)))
    }
    return b.String()
}
```

##### 4.1.6.4 VectorService 初始化修复

**文件**: `api-server/cmd/main.go`

```go
// 改造前
vectorService := service.NewVectorService(db, nil)

// 改造后 — 正确初始化EmbeddingService
embeddingSvc := service.NewEmbeddingService(cfg.LLM.APIKey, cfg.LLM.BaseURL)
vectorService := service.NewVectorService(db, embeddingSvc)
```

##### 4.1.6.5 RAG上下文注入到PromptProvider

在 `PromptProvider.Build()` 中，对 `PurposePlan` 和 `PurposeReact` 注入RAG上下文：

```go
func (p *AegisPromptProvider) Build(ctx context.Context, req core.PromptRequest) (core.PromptBundle, error) {
    switch req.Purpose {
    case core.PurposePlan:
        systemPrompt := PlanPromptTemplate
        // 注入RAG经验上下文
        if p.experienceProvider != nil {
            expResp, err := p.experienceProvider.Fetch(ctx, core.ExperienceRequest{
                TaskID:   req.TaskID,
                Query:    p.alertCtx["query"].(string),
                MaxItems: 3,
            })
            if err == nil && len(expResp.Items) > 0 {
                systemPrompt += "\n\n## 历史经验参考\n" + formatExperienceForPrompt(expResp.Items)
            }
        }
        return core.PromptBundle{SystemPrompt: systemPrompt}, nil

    case core.PurposeReact:
        return core.PromptBundle{SystemPrompt: ReActJSONPromptTemplate}, nil

    case core.PurposeSummarize:
        return core.PromptBundle{SystemPrompt: SummarizePromptTemplate}, nil

    default:
        return core.PromptBundle{}, nil  // 使用agent-runtime内置
    }
}
```

#### 4.1.7 错误与反思数据的闭环利用

##### 4.1.7.1 数据流向

```
分析执行 → TaskResult → 持久化到DB
                          ↓
              ┌───────────┼───────────┐
              ↓           ↓           ↓
        AgentReflection  AgentAudit  AgentModelError
              ↓           ↓           ↓
              └───────────┼───────────┘
                          ↓
                 ExperienceProvider.Fetch()
                          ↓
                 注入下次分析的Plan/React Prompt
```

##### 4.1.7.2 经验查询策略

| 查询来源 | 条件 | 用途 |
|:---|:---|:---|
| `ai_analysis_record` | 向量相似度 ≥ 0.7 | 相似案例参考 |
| `agent_reflection` | `recoverable=true`, 最近7天 | 避免重复失败 |
| `agent_audit` | `decision='correct_plan'`, 最近7天 | 学习审计纠正模式 |
| `agent_model_error` | `recoverable=true`, 最近3天 | 模型调用失败降级策略 |

##### 4.1.7.3 反思记录的ReusableLesson提取

在 `reflection.Reflect()` 调用后，从LLM返回的反思结果中提取 `ReusableLesson` 字段，该字段是一段可复用的经验教训文本，保存到 `agent_reflection.reusable_lesson`。后续 `ExperienceProvider.Fetch()` 会查询这些教训并注入到新的分析会话中。

### 4.2 前端改造

#### 4.2.1 SSE事件类型扩展

**文件**: `frontend/src/api/aiAnalysis.ts`

```typescript
// 改造前
export type SSEEventType = 'thinking' | 'tool_call' | 'tool_result' | 'tool_error'
  | 'content' | 'flowchart_image' | 'done' | 'error'

// 改造后
export type SSEEventType = 'thinking' | 'tool_call' | 'tool_result' | 'tool_error'
  | 'content' | 'flowchart_image' | 'done' | 'error'
  | 'plan' | 'step_started' | 'step_completed' | 'audit' | 'reflection' | 'correction'
```

**新增接口**:

```typescript
export interface PlanStep {
  step_id: string
  title: string
  objective: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped'
  suggested_tools?: string[]
}

export interface PlanEvent {
  plan_id: string
  goal: string
  steps: PlanStep[]
}

export interface AuditEvent {
  audit_id: string
  findings: string[]
  decision: string
  risk_level: string
}

export interface ReflectionEvent {
  reflection_id: string
  root_cause: string
  recommendation: string
  impact: string
}

export interface CorrectionEvent {
  correction_id: string
  reason: string
  actions: Array<{
    type: string
    step_id: string
    reason: string
  }>
}
```

#### 4.2.2 新增 ExecutionPlan 组件

**文件**: `frontend/src/components/ExecutionPlan.vue`

可折叠面板，展示执行计划和步骤状态：
- 步骤列表，带状态徽章（pending/running/completed/failed/skipped）
- 实时更新步骤状态
- 显示建议工具列表
- 审计/反思/纠正事件的时间线展示

#### 4.2.3 AIAnalysis.vue 改造

- 新增 `executionPlan` reactive ref 存储计划数据
- 新增 `auditResults`、`reflectionResults`、`correctionResults` 存储分析过程数据
- `createSSEHandler()` 增加对 `plan`、`step_started`、`step_completed`、`audit`、`reflection`、`correction` 事件的处理
- `Message` 接口扩展：`planStepId`、`auditResult`、`reflectionResult`、`correctionResult`
- 左侧面板增加"执行计划"折叠区域
- 右侧消息流增加审计/反思/纠正消息气泡

---

## 5. SSE协议对比

### 5.1 当前协议

```
thinking → tool_call → tool_result → thinking → ... → content → done
```

### 5.2 新协议

```
plan → step_started → thinking → tool_call → tool_result → thinking → step_completed
→ step_started → thinking → tool_call → tool_result → step_completed
→ audit → thinking → step_started → ... → step_completed
→ reflection → correction → ... → content → done
```

### 5.3 向后兼容

- 旧事件类型（thinking, tool_call, tool_result, tool_error, content, done, error）保持不变
- 新增事件类型（plan, step_started, step_completed, audit, reflection, correction）为可选
- 前端对未知事件类型做忽略处理，不影响现有功能

---

## 6. 实施顺序

| 步骤 | 内容 | 文件 |
|:---|:---|:---|
| 1 | 添加agent-runtime依赖 | `api-server/go.mod` |
| 2 | 创建工具描述符 | `api-server/internal/llm/adapters/tool_descriptors.go` |
| 3 | 创建LLM适配器 | `api-server/internal/llm/adapters/llm_client_adapter.go` |
| 4 | 创建工具网关适配器 | `api-server/internal/llm/adapters/tool_gateway_adapter.go` |
| 5 | 创建提示词提供器 | `api-server/internal/llm/adapters/prompt_provider.go` |
| 6 | 创建SSE Hook Sink | `api-server/internal/llm/adapters/hook_sink_sse.go` |
| 7 | 创建ExperienceProvider适配器 | `api-server/internal/llm/adapters/experience_provider.go` |
| 8 | 创建Runtime工厂 | `api-server/internal/llm/adapters/runtime_factory.go` |
| 9 | 新增AgentExecution数据库模型 | `api-server/internal/model/agent_execution.go` |
| 10 | 新增AgentExecution Repository | `api-server/internal/repository/agent_execution_repository.go` |
| 11 | 更新DB Schema迁移 | `api-server/internal/repository/db.go` |
| 12 | 修复VectorService初始化 | `api-server/cmd/main.go` |
| 13 | 更新prompts.go | `api-server/internal/llm/prompts.go` |
| 14 | 改造ai_analysis_handler.go | `api-server/internal/api/handler/ai_analysis_handler.go` |
| 15 | 更新前端API类型 | `frontend/src/api/aiAnalysis.ts` |
| 16 | 创建ExecutionPlan组件 | `frontend/src/components/ExecutionPlan.vue` |
| 17 | 改造AIAnalysis页面 | `frontend/src/views/detection/AIAnalysis.vue` |
| 18 | 更新设计文档 | 本文档 + prompt文件 |
| 19 | 编译测试 | 使用 aegis-build-test skill |

---

## 7. 验证方案

1. **单元测试**: Mock LLMClient和ToolGateway，验证Runtime产出有效TaskResult
2. **适配器测试**: 验证LLMClientAdapter按Purpose正确路由，ToolGatewayAdapter正确归一化参数
3. **SSE桥接测试**: 验证18种HookEvent产生正确的SSE事件
4. **持久化测试**: 验证TaskResult完整数据（Reflections/Audits/Corrections/Errors）正确入库
5. **RAG集成测试**: 验证ExperienceProvider正确查询相似案例和失败教训
6. **集成测试**: 使用 aegis-build-test skill 运行完整流程，验证attack_graph输出格式
7. **前端测试**: 验证新事件类型正确渲染，现有事件不受影响
8. **回滚测试**: 关闭feature flag，验证旧ReActAgent路径仍可用

---

## 8. 风险与缓解

| 风险 | 缓解措施 |
|:---|:---|
| LLM输出格式变更（JSON vs 文本） | PromptProvider适配提示词；多模型测试 |
| attack_graph格式兼容性 | SummarizePrompt显式要求JSON格式；Hook桥接层验证 |
| 性能回归（额外LLM调用） | 保守配置：AuditEveryNSteps=3, MaxAudits=2 |
| 流式延迟（无token级流式） | Hook事件在关键节点（plan_created, tool_call_started）即时推送 |
| 依赖风险（本地replace） | 开发完成后发布agent-runtime到GitHub，使用正式版本 |
| RAG Embedding服务不可用 | ExperienceProvider降级到DB查询最近成功案例 |
| 持久化数据量增长 | 定期清理30天前的AgentExecution记录；对大字段(JSONB)做归档 |
