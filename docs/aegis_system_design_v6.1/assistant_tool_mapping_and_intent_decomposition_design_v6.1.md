# Aegis V6.1 智能助手工具映射与意图拆解设计

**版本**: 6.1  
**日期**: 2026-07-02  
**状态**: 已废弃的历史设计（不得用于当前实现）
**关联文档**:

- `docs/aegis_system_design_v6.0/agent_runtime_tool_orchestration_design_v6.0.md`
- `docs/aegis_system_design_v6.0/fix/agent_runtime_tool_orchestration_gaps_analysis.md`
- `docs/aegis_system_design_v5.8/fix/assistant_tool_call_routing_fix.md`

---

> **2026-07-10 替代说明**
>
> 本文保留用于解释历史演进，其中的独立 ToolSelector、软评分、分数阈值、领域召回、
> 预选绕过和固定执行计划均已废弃。当前唯一有效设计为：
>
> - `fix/assistant_generic_agent_flow_only.md`
> - `fix/assistant_mapping_authorization_and_truthful_completion_fix.md`
>
> 当前生产语义是：大模型从实时英文 capability 目录输出 exact capability，后端只做
> exact mapping 与安全硬门，agent-runtime 是唯一规划器，工具完成以终态 ToolOutcome
> 和真实 call ID 证据为准。

## 1. 问题背景与需求

V6.0 已经建立了智能助手的工具注册、按需注入、审批和 agent-runtime 执行框架，但工具选择仍存在两个核心风险：

1. 仅依赖工具 `name`、`description`、`tags` 做匹配时，模型容易把业务目标和工具名做表层关键词匹配，导致错选、漏选或过早选择写操作。
2. 用户问题通常是业务语言，不是工具语言。如果收到用户问题后直接找 tool，系统缺少稳定的目标、对象、动作、约束、缺失信息拆解过程，复杂任务容易只执行第一个命中的工具就收工。

本设计在 V6.1 中补充一层“工具能力映射 + LLM 意图拆解 + 后端裁决”的机制。核心原则：

> LLM 负责理解用户问题，后端 mapping 负责裁决工具和约束。

## 2. 当前行为

当前智能助手链路大致为：

```text
用户消息
  -> ContextLoader 加载上下文
  -> IntentRouter 规则分类
  -> LLM 或规则选择工具
  -> ToolSelector / ToolCatalog 返回 selected_tools
  -> RuntimeFactory 注入工具
  -> agent-runtime 执行
```

已有能力：

- `ToolSpec` 已包含 `Domain`、`Operation`、`Capability`、`Description`、`Aliases`、`Tags`、`ObjectTypes`、`PageRoutes`、`Risk`、`ArgsSchema` 等字段。
- `LLMToolSelector` 已有两阶段工具选择：短目录初选和详情目录终选。
- `ToolDispatcher` 已有审批、白名单、执行记录和超时。
- `AssistantPromptProvider` 已有计划、执行、总结阶段的提示词约束。

主要缺口：

- 缺少独立的后台工具能力 mapping 层，工具选择仍容易受自然语言描述影响。
- LLM 工具选择直接输出 `selected_tools`，缺少稳定的中间意图模型。
- `need_clarification` 已在 LLM 输出结构中出现，但没有成为后端阻断/追问决策。
- 业务闭环仍有不少特殊流程写在 gateway 中，说明工具选择和工作流组合没有完全数据化。

## 3. 目标行为

V6.1 目标链路调整为：

```text
用户消息
  -> 上下文提取
  -> LLM 输出结构化问题拆解 IntentBreakdown
  -> 后端 ToolCapabilityMapping 做能力匹配
  -> ToolDecisionEngine 做参数、风险、前置条件和审批裁决
  -> 生成 ToolExecutionPlan
  -> RuntimeFactory 注入裁决后的工具或组合工作流
  -> agent-runtime 执行工具
  -> 基于工具结果总结
```

设计目标：

1. 用户问题先拆解为业务目标、动作、对象、范围、约束、缺失信息，而不是直接命中工具名。
2. 后台为每个工具维护结构化 mapping，不只依赖 `name` 和 `description`。
3. LLM 只给候选能力和推理依据，最终工具选择由后端裁决。
4. 写操作、高风险操作、目标不明确操作必须经过后端确认或审批。
5. 支持组合工具链路，例如“采集资产并分析漏洞”应自动形成多步计划，而不是只调用资产采集。

### 3.1 责任边界

本方案不是让 LLM 直接决定工具调用，而是把工具选择拆成两个阶段：

| 阶段 | 负责方 | 输出 | 权限边界 |
|:---|:---|:---|:---|
| 问题理解 | LLM | `IntentBreakdown`、候选能力、缺失信息、推理依据 | 只能建议能力，不得直接授权工具执行 |
| 工具裁决 | 后端 | `ToolExecutionPlan`、参数绑定、审批状态、拒绝原因 | 只允许执行通过 mapping、参数、权限、风险和状态机校验的工具 |
| 结果校验 | 后端 + LLM | 工具证据、缺口、总结 | LLM 只能基于工具结果解释，不得虚构已执行结果 |

因此 V6.1 的工具链路应被定义为“LLM 辅助意图理解，后端授权执行计划”。后台不能保证 LLM 永远理解正确，但可以保证工具调用必须落在可验证契约内：工具存在、能力匹配、参数来源明确、风险策略满足、状态顺序合法、执行结果可审计。

## 4. 组件设计

### 4.1 新增组件

| 组件 | 推荐位置 | 职责 |
|:---|:---|:---|
| `IntentDecomposer` | `api-server/internal/assistant/intent_decomposer.go` | 调用 LLM 将用户问题拆为结构化意图 |
| `ToolCapabilityMapping` | `api-server/internal/assistant/tool_capability_mapping.go` | 维护工具能力、动作、对象、前置条件、负例、后续工具 |
| `ToolDecisionEngine` | `api-server/internal/assistant/tool_decision_engine.go` | 根据意图、mapping、上下文和策略裁决工具 |
| `ToolArgumentBinder` | `api-server/internal/assistant/tool_argument_binder.go` | 将用户输入、上下文、默认策略绑定为工具参数，并记录参数来源 |
| `ToolContractValidator` | `api-server/internal/assistant/tool_contract_validator.go` | 校验工具契约、硬性闸门、状态机和审批要求 |
| `ToolExecutionPlanner` | `api-server/internal/assistant/tool_execution_planner.go` | 生成可执行工具计划和组合工作流 |
| `ClarificationGate` | `api-server/internal/assistant/clarification_gate.go` | 在信息不足时中断执行并追问用户 |
| `ToolDecisionRecorder` | `api-server/internal/assistant/tool_decision_recorder.go` | 记录工具命中、拒绝、评分、参数来源和审计证据 |
| `ToolResultVerifier` | `api-server/internal/assistant/tool_result_verifier.go` | 校验工具执行结果是否满足后置条件 |

### 4.2 工具使用契约

后台裁决的核心不是再写一份自然语言描述，而是为每个工具建立可校验的使用契约。契约用于回答三个问题：

1. 这个工具能解决哪类业务意图。
2. 什么情况下绝对不能调用。
3. 调用前后必须满足哪些参数、权限、状态和结果条件。

推荐模型：

```go
type ToolUseContract struct {
    ToolName                   string                `json:"tool_name"`
    Capability                 string                `json:"capability"`
    Domain                     string                `json:"domain"`
    AllowedIntents             []string              `json:"allowed_intents"`
    DeniedIntents              []string              `json:"denied_intents"`
    Actions                    []string              `json:"actions"`
    ObjectTypes                []string              `json:"object_types"`
    RequiredEntities           []string              `json:"required_entities"`
    OptionalEntities           []string              `json:"optional_entities"`
    Preconditions              []string              `json:"preconditions"`
    ArgBindings                []ArgBindingRule      `json:"arg_bindings"`
    StateTransitions           []ToolStateTransition `json:"state_transitions"`
    Postconditions             []string              `json:"postconditions"`
    ResultValidators           []string              `json:"result_validators"`
    NextCapabilities           []string              `json:"next_capabilities"`
    WorkflowHints              []string              `json:"workflow_hints"`
    Risk                       string                `json:"risk"`
    RequiresExplicitUserIntent bool                  `json:"requires_explicit_user_intent"`
    RequiresApproval           bool                  `json:"requires_approval"`
    DecisionExamples           []ToolDecisionExample `json:"decision_examples,omitempty"`
}

type ArgBindingRule struct {
    ArgName       string   `json:"arg_name"`
    Entity        string   `json:"entity"`
    SourceOrder   []string `json:"source_order"` // user_message, page_context, session_context, policy_default, previous_step
    Required      bool     `json:"required"`
    DefaultPolicy string   `json:"default_policy,omitempty"`
}

type ToolStateTransition struct {
    From      string `json:"from"`
    To        string `json:"to"`
    Condition string `json:"condition"`
}
```

契约示例：

```json
{
  "tool_name": "Asset.Collection.Trigger",
  "capability": "trigger_asset_collection",
  "domain": "asset",
  "allowed_intents": ["execute_asset_collection", "refresh_asset_inventory"],
  "denied_intents": ["explain_asset_collection", "query_asset_collection_history"],
  "actions": ["collect", "refresh", "scan"],
  "object_types": ["host", "asset"],
  "required_entities": ["scope|host_ids"],
  "preconditions": ["user_explicit_execute_intent", "scope_resolved"],
  "arg_bindings": [
    {
      "arg_name": "host_ids",
      "entity": "host",
      "source_order": ["user_message", "page_context", "previous_step"],
      "required": true
    }
  ],
  "state_transitions": [
    {"from": "planned", "to": "waiting_approval", "condition": "risk >= medium"},
    {"from": "approved", "to": "running", "condition": "approval_granted"}
  ],
  "postconditions": ["task_id_created"],
  "result_validators": ["asset_collection_task_id_present"],
  "next_capabilities": ["get_asset_collection_task", "list_application_assets", "get_asset_summary"],
  "workflow_hints": ["asset_collection_then_analysis"],
  "risk": "medium",
  "requires_explicit_user_intent": true,
  "requires_approval": true
}
```

### 4.3 IntentBreakdown

LLM 第一轮不直接返回工具名，而是返回业务意图：

```go
type IntentBreakdown struct {
    Goal                  string                 `json:"goal"`
    Domains               []string               `json:"domains"`
    Actions               []string               `json:"actions"`
    Objects               []IntentObject         `json:"objects"`
    Scope                 IntentScope            `json:"scope"`
    Constraints           []string               `json:"constraints"`
    MissingInfo           []MissingInfo          `json:"missing_info"`
    RequiresWrite         bool                   `json:"requires_write"`
    RiskHint              string                 `json:"risk_hint"`
    CandidateCapabilities []string               `json:"candidate_capabilities"`
    NeedClarification     bool                   `json:"need_clarification"`
    ClarifyingQuestion    string                 `json:"clarifying_question"`
    Reason                string                 `json:"reason"`
    Confidence            float64                `json:"confidence"`
    Raw                   map[string]interface{} `json:"raw,omitempty"`
}
```

示例：

```json
{
  "goal": "采集全部在线主机资产并分析 AI 资产情况",
  "domains": ["host", "asset"],
  "actions": ["collect", "analyze"],
  "objects": [
    {"type": "host", "selector": "online"},
    {"type": "asset", "category": "ai_asset"}
  ],
  "scope": {"kind": "online_hosts"},
  "constraints": [],
  "missing_info": [],
  "requires_write": true,
  "risk_hint": "medium",
  "candidate_capabilities": [
    "trigger_asset_collection",
    "get_asset_collection_task",
    "list_application_assets",
    "get_asset_summary"
  ],
  "need_clarification": false,
  "clarifying_question": "",
  "reason": "用户要求执行采集并分析 AI 资产，需要先触发资产采集，再查询采集结果和资产概览。",
  "confidence": 0.86
}
```

IntentDecomposer 的输出约束：

- 不允许输出最终 `tool_name` 作为执行授权，只能输出业务能力 `candidate_capabilities`。
- `requires_write=true` 时必须说明触发写操作的用户原文依据。
- 对象、范围、动作任一缺失时，必须设置 `need_clarification=true`。
- `confidence < 0.65` 时，即使存在候选能力，也进入后端追问或只读兜底。
- 对页面上下文引用，例如“这个告警”“当前主机”，必须保留引用来源，交给后端解析为真实 ID。

### 4.4 ToolCapabilityMapping

每个工具增加后台 mapping，不把工具描述当作唯一依据。

```go
type ToolCapabilityMapping struct {
    ToolName              string            `json:"tool_name"`
    Capability            string            `json:"capability"`
    Domain                string            `json:"domain"`
    AllowedIntents        []string          `json:"allowed_intents"`
    DeniedIntents         []string          `json:"denied_intents"`
    Actions               []string          `json:"actions"`
    ObjectTypes           []string          `json:"object_types"`
    RequiredEntities      []string          `json:"required_entities"`
    OptionalEntities      []string          `json:"optional_entities"`
    Preconditions         []string          `json:"preconditions"`
    ArgBindings           []ArgBindingRule  `json:"arg_bindings"`
    NegativeCases         []string          `json:"negative_cases"`
    Postconditions        []string          `json:"postconditions"`
    ResultValidators      []string          `json:"result_validators"`
    NextCapabilities      []string          `json:"next_capabilities"`
    WorkflowHints         []string          `json:"workflow_hints"`
    Risk                  string            `json:"risk"`
    RequiresExplicitUserIntent bool         `json:"requires_explicit_user_intent"`
    RequiresApproval      bool              `json:"requires_approval"`
    ScoreWeights          map[string]float64 `json:"score_weights,omitempty"`
}
```

示例：

```json
{
  "tool_name": "Asset.Collection.Trigger",
  "capability": "trigger_asset_collection",
  "domain": "asset",
  "allowed_intents": ["execute_asset_collection", "refresh_asset_inventory"],
  "denied_intents": ["explain_asset_collection", "query_asset_collection_history"],
  "actions": ["collect", "refresh", "scan"],
  "object_types": ["host", "asset"],
  "required_entities": ["scope|host_ids"],
  "optional_entities": ["types", "force"],
  "preconditions": ["user_explicit_execute_intent"],
  "arg_bindings": [
    {"arg_name": "host_ids", "entity": "host", "source_order": ["user_message", "page_context", "previous_step"], "required": true}
  ],
  "negative_cases": ["用户只询问资产采集概念时不要调用", "用户只查看采集历史时不要调用"],
  "postconditions": ["task_id_created"],
  "result_validators": ["asset_collection_task_id_present"],
  "next_capabilities": [
    "get_asset_collection_task",
    "list_application_assets",
    "get_asset_summary"
  ],
  "workflow_hints": ["asset_collection_then_analysis"],
  "risk": "medium",
  "requires_explicit_user_intent": true,
  "requires_approval": true
}
```

### 4.5 ToolDecisionEngine

后端裁决不以单次相似度排序为准，而是采用“硬性闸门 + 软评分 + 审计记录”的方式。

硬性闸门任一失败时，不允许进入执行计划：

1. 工具未注册、未启用、未在 mapping 中声明。
2. LLM 候选能力与 mapping `allowed_intents`、`actions`、`object_types` 不匹配。
3. 命中 `denied_intents` 或 `negative_cases`。
4. 必填实体无法从用户消息、页面上下文、会话上下文、策略默认值或前置步骤中解析。
5. 写操作缺少用户显式执行意图。
6. 中高风险工具未满足审批策略。
7. 状态机不允许当前步骤执行，例如任务尚未创建却查询任务结果。
8. 工具 schema 校验失败或参数来源不可追踪。

软评分用于在多个合规候选工具中排序：

```text
score =
  domain_match * 0.20 +
  action_match * 0.20 +
  object_match * 0.20 +
  scope_match * 0.15 +
  context_match * 0.10 +
  workflow_fit * 0.10 +
  risk_fit * 0.05
```

默认策略：

- `score < 0.60`：拒绝执行，返回追问或说明无法定位工具。
- `0.60 <= score < 0.75`：只允许只读工具；写工具必须追问。
- `score >= 0.75`：可以进入执行计划，但仍需经过风险、审批、状态机和 schema 校验。

裁决流水线：

1. 读取 `IntentBreakdown` 和上下文。
2. 用 `candidate_capabilities`、domain、action、object 做候选召回。
3. 执行负例过滤和硬性闸门。
4. 通过 `ToolArgumentBinder` 绑定参数，并记录每个参数来源。
5. 检查前置条件、权限、风险和审批策略。
6. 按状态机生成步骤顺序，必要时补齐后续只读工具。
7. 输出 `ToolExecutionPlan` 和 `ToolDecisionRecord`。
8. 工具执行后由 `ToolResultVerifier` 校验后置条件。

决策记录结构：

```go
type ToolDecisionRecord struct {
    TraceID         string                 `json:"trace_id"`
    ToolName        string                 `json:"tool_name"`
    Capability      string                 `json:"capability"`
    Decision        string                 `json:"decision"` // accepted, rejected, clarification_required
    Score           float64                `json:"score"`
    HardGateResults []HardGateResult       `json:"hard_gate_results"`
    ArgSources      map[string]ArgSource   `json:"arg_sources"`
    ApprovalState   string                 `json:"approval_state,omitempty"`
    Reason          string                 `json:"reason"`
    Evidence        map[string]interface{} `json:"evidence,omitempty"`
}

type HardGateResult struct {
    Name   string `json:"name"`
    Passed bool   `json:"passed"`
    Reason string `json:"reason,omitempty"`
}

type ArgSource struct {
    SourceType string `json:"source_type"` // user_message, page_context, session_context, policy_default, previous_step
    SourceRef  string `json:"source_ref"`
    Confidence float64 `json:"confidence"`
}
```

输出结构：

```go
type ToolExecutionPlan struct {
    Goal              string          `json:"goal"`
    NeedClarification bool            `json:"need_clarification"`
    ClarifyingQuestion string         `json:"clarifying_question,omitempty"`
    Steps             []ToolPlanStep  `json:"steps"`
    EvidencePolicy    EvidencePolicy  `json:"evidence_policy"`
    DecisionTraceID    string          `json:"decision_trace_id"`
}

type ToolPlanStep struct {
    StepID        string                 `json:"step_id"`
    ToolName      string                 `json:"tool_name"`
    Capability    string                 `json:"capability"`
    Args          map[string]interface{} `json:"args"`
    Risk          string                 `json:"risk"`
    RequiresApproval bool                `json:"requires_approval"`
    Reason        string                 `json:"reason"`
    ArgSources    map[string]ArgSource   `json:"arg_sources"`
    Preconditions []string               `json:"preconditions,omitempty"`
    Postconditions []string              `json:"postconditions,omitempty"`
    OnSuccess     []string               `json:"on_success,omitempty"`
}
```

### 4.6 工具注入与执行状态机

agent-runtime 不应拿到完整工具列表，只能拿到 `ToolExecutionPlan.steps` 中已被裁决允许的工具。这样可以避免 LLM 在执行阶段绕过后台 mapping，自行调用未授权工具。

执行状态机：

```text
intent_decomposed
  -> planning
  -> clarification_required
  -> approval_required
  -> ready
  -> running
  -> result_verified
  -> completed
```

异常状态：

```text
planning_failed
approval_rejected
tool_failed
postcondition_failed
cancelled
```

调度规则：

1. `clarification_required` 状态不注入任何写工具。
2. `approval_required` 状态只允许展示计划和审批信息，不执行工具。
3. `ready` 状态只注入计划内工具，不注入全量工具目录。
4. `running` 状态如果 LLM 发现还需要新工具，只能输出 `additional_capability_request`，重新进入 `ToolDecisionEngine` 裁决。
5. `postcondition_failed` 状态不继续执行后续步骤，除非后续步骤被标记为失败诊断只读工具。
6. `completed` 状态的最终回答必须引用已验证工具结果。

示例：用户说“采集资产并分析 MySQL 漏洞”，第一轮计划只注入 `Host.List`、`Asset.Collection.Trigger`、`Asset.Collection.Get`、`Asset.Application.List`、`Vulnerability.List`。如果执行中模型想调用“阻断漏洞主机”，必须重新发起能力请求，后台因原用户目标没有阻断意图而拒绝或要求用户确认。

## 5. 数据流

### 5.1 普通只读查询

```text
用户: "有哪些在线主机"
  -> IntentBreakdown: domain=host, action=query, scope=online
  -> mapping 命中 list_hosts / get_agent_status
  -> 后端裁决 Host.List(status=online/page_size=100)
  -> Runtime 注入 Host.List
  -> 工具执行
  -> 总结在线主机列表
```

### 5.2 需要执行的任务

```text
用户: "对全部在线主机做资产采集并分析 AI Agent"
  -> IntentBreakdown: collect + analyze, requires_write=true
  -> mapping 命中 Asset.Collection.Trigger 及后续查询能力
  -> 后端确认目标为 online_hosts，可安全默认为 all online hosts
  -> 生成计划:
       1. Host.List(status=online)
       2. Asset.Collection.Trigger(scope=hosts, host_ids=...)
       3. Asset.Collection.Get(task_id=...)
       4. Asset.Application.List(category=ai_agent)
       5. Asset.Summary.Get
  -> 中风险工具按审批策略处理
  -> 总结覆盖范围、工具证据和缺口
```

### 5.3 信息不足

```text
用户: "帮我修复一下"
  -> IntentBreakdown: action=repair, object missing
  -> ClarificationGate: need_clarification=true
  -> 返回追问: "请确认要修复的对象，是基线规则、漏洞 CVE、弱密码任务还是检测包？"
  -> 不注入写操作工具
```

### 5.4 后台如何判断工具用得对不对

后台判断的标准不是“模型觉得像不像”，而是“是否满足工具契约”。典型场景如下：

| 用户问题 | LLM 可能输出 | 后台裁决 | 原因 |
|:---|:---|:---|:---|
| `资产采集是什么` | `candidate_capabilities=["trigger_asset_collection"]` | 拒绝触发采集，只做概念解释 | 命中 `denied_intents=explain_asset_collection` |
| `对在线主机做资产采集` | `trigger_asset_collection` | 允许生成计划 | 动作、对象、范围明确，且有显式执行意图 |
| `帮我修复一下` | `repair` | 追问 | 缺少对象、范围和修复类型 |
| `阻断这个告警` | `block_alert` | 解析当前页面告警 ID，缺 ID 则追问，有 ID 则进入审批 | 高风险操作必须有对象 ID 和审批 |
| `采集资产并分析 MySQL 漏洞` | `trigger_asset_collection,list_vulnerabilities` | 生成多步计划 | workflow hint 命中采集后分析链路 |

后台能保证的部分：

- 不执行不存在、未启用、未被 mapping 声明的工具。
- 不执行参数缺失、参数来源不明、schema 校验失败的工具。
- 不执行命中负例、缺显式写意图、缺审批的工具。
- 不跳过状态机，例如不能在没有 `task_id` 时查询采集任务结果。
- 每次调用都留下 `ToolDecisionRecord`，能解释为什么调用、为什么拒绝、参数从哪里来。

后台不能保证的部分：

- 不能保证 LLM 对用户真实意图百分百理解正确。
- 不能保证用户输入本身没有歧义。
- 不能保证工具执行的外部系统一定成功。

因此系统策略是：歧义时追问，高风险时审批，执行后校验结果，最终回答必须标明证据和缺口。

## 6. 接口变化

### 6.1 内部接口

新增内部接口，不直接暴露公网 API：

```go
type IntentDecomposer interface {
    Decompose(ctx context.Context, input IntentDecomposeInput) (*IntentBreakdown, error)
}

type ToolDecisionEngine interface {
    Decide(ctx context.Context, input ToolDecisionInput) (*ToolExecutionPlan, error)
}

type ToolArgumentBinder interface {
    Bind(ctx context.Context, intent *IntentBreakdown, contract ToolUseContract, context AssistantContext) (*BoundToolArgs, error)
}

type ToolContractValidator interface {
    Validate(ctx context.Context, input ToolContractValidateInput) (*ToolContractValidateResult, error)
}

type ToolResultVerifier interface {
    Verify(ctx context.Context, step ToolPlanStep, result ToolExecutionResult) (*ToolResultVerifyResult, error)
}
```

`Orchestrator.Run` 中原工具选择阶段调整为：

```text
IntentRouter 快速规则分类
  -> IntentDecomposer 结构化拆解
  -> ToolDecisionEngine 裁决工具计划
       -> ToolArgumentBinder 绑定参数
       -> ToolContractValidator 校验硬性闸门
       -> ToolDecisionRecorder 写入审计
  -> RuntimeFactory 注入 plan 中允许执行的工具
  -> ToolResultVerifier 校验工具结果
```

### 6.2 HTTP API

本设计不要求新增用户可见 API。可以在调试模式下增加只读诊断接口：

```text
POST /api/v1/assistant/debug/intent-decompose
POST /api/v1/assistant/debug/tool-decision
```

默认关闭，仅开发和测试环境启用。

## 7. 数据库设计

第一阶段建议 mapping 以代码或内置 JSON 配置维护，避免引入迁移风险。

第二阶段如需运营可配置，可新增表：

```sql
CREATE TABLE assistant_tool_capability_mappings (
    id UUID PRIMARY KEY,
    tool_name VARCHAR(128) NOT NULL UNIQUE,
    capability VARCHAR(128) NOT NULL,
    domain VARCHAR(64) NOT NULL,
    allowed_intents JSONB NOT NULL DEFAULT '[]',
    denied_intents JSONB NOT NULL DEFAULT '[]',
    actions JSONB NOT NULL DEFAULT '[]',
    object_types JSONB NOT NULL DEFAULT '[]',
    required_entities JSONB NOT NULL DEFAULT '[]',
    optional_entities JSONB NOT NULL DEFAULT '[]',
    preconditions JSONB NOT NULL DEFAULT '[]',
    arg_bindings JSONB NOT NULL DEFAULT '[]',
    negative_cases JSONB NOT NULL DEFAULT '[]',
    postconditions JSONB NOT NULL DEFAULT '[]',
    result_validators JSONB NOT NULL DEFAULT '[]',
    state_transitions JSONB NOT NULL DEFAULT '[]',
    next_capabilities JSONB NOT NULL DEFAULT '[]',
    workflow_hints JSONB NOT NULL DEFAULT '[]',
    risk VARCHAR(32) NOT NULL DEFAULT 'readonly',
    requires_explicit_user_intent BOOLEAN NOT NULL DEFAULT false,
    requires_approval BOOLEAN NOT NULL DEFAULT false,
    score_weights JSONB NOT NULL DEFAULT '{}',
    decision_examples JSONB NOT NULL DEFAULT '[]',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

如需完整审计，可新增裁决记录表：

```sql
CREATE TABLE assistant_tool_decision_records (
    id UUID PRIMARY KEY,
    trace_id VARCHAR(64) NOT NULL,
    session_id UUID NULL,
    message_id UUID NULL,
    tool_name VARCHAR(128) NOT NULL,
    capability VARCHAR(128) NOT NULL,
    decision VARCHAR(32) NOT NULL,
    score NUMERIC(5, 4) NOT NULL DEFAULT 0,
    hard_gate_results JSONB NOT NULL DEFAULT '[]',
    arg_sources JSONB NOT NULL DEFAULT '{}',
    approval_state VARCHAR(32) NULL,
    reason TEXT NOT NULL,
    evidence JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_assistant_tool_decision_records_trace_id
    ON assistant_tool_decision_records(trace_id);
```

审计增强：

- `assistant_tool_calls` 保留原始工具调用记录。
- `assistant_messages.metadata` 增加 `intent_breakdown`、`tool_execution_plan`、`decision_trace_id`、`decision_reason`，用于问题复盘。
- `assistant_tool_calls.metadata` 增加 `arg_sources`、`postcondition_result`、`contract_version`，用于定位误调用。

## 8. 配置变化

新增可选配置：

| 配置 | 默认值 | 说明 |
|:---|:---|:---|
| `ASSISTANT_INTENT_DECOMPOSE_ENABLED` | `true` | 是否启用 LLM 问题拆解 |
| `ASSISTANT_TOOL_DECISION_ENGINE_ENABLED` | `true` | 是否启用后端工具裁决引擎 |
| `ASSISTANT_TOOL_MAPPING_MODE` | `builtin` | `builtin` 或 `database` |
| `ASSISTANT_TOOL_DECISION_TRACE` | `false` | 是否在 metadata 中记录详细评分 |
| `ASSISTANT_CLARIFICATION_REQUIRED_FOR_WRITE` | `true` | 写操作目标缺失时是否强制追问 |
| `ASSISTANT_TOOL_DECISION_MIN_SCORE` | `0.75` | 工具进入执行计划的默认最低分 |
| `ASSISTANT_TOOL_READONLY_MIN_SCORE` | `0.60` | 只读工具可执行的最低分 |
| `ASSISTANT_TOOL_POSTCONDITION_CHECK_ENABLED` | `true` | 是否启用工具后置条件校验 |
| `ASSISTANT_TOOL_DRY_RUN_FOR_WRITE` | `false` | 写操作是否仅生成计划不实际执行 |

LLM 不可用时降级：

1. 简单只读查询走规则选择器。
2. 写操作和高风险操作不自动执行，返回追问或提示 LLM 服务不可用。

## 9. 安全影响

正向影响：

- 写操作不会仅因模型选择工具而执行，必须经过后端 mapping、参数和审批裁决。
- 工具负例可以阻止“解释概念却触发操作”的误调用。
- `need_clarification` 成为硬约束，目标不明确时停止执行。
- 高风险工具不因描述相似进入执行计划。
- 参数来源、审批状态和后置条件被纳入审计，便于现场复盘。

安全约束：

- LLM 输出的 `candidate_capabilities` 不可信，只作为候选输入。
- 后端不得执行 mapping 中不存在、未启用或参数不满足的工具。
- 任何写操作都必须记录 `decision_reason` 和审批状态。
- 组合工作流中自动补出的后续工具只能是只读工具或已审批链路内的工具。
- 自动补出的后续工具不得扩大用户授权范围，例如从“当前主机”扩大到“全部主机”。
- 工具结果校验失败时，不允许在最终回答中声明任务已经完成，只能说明执行失败或证据不足。

非目标：

- 本方案不尝试让后端完全理解自然语言含义。
- 本方案不把所有业务流程都硬编码到单个状态机中，复杂链路通过 `workflow_hints` 和工具契约逐步数据化。
- 本方案不取消 LLM 的规划能力，而是限制 LLM 只能在工具契约内规划。

## 10. 兼容性影响

- 对现有 `ToolSpec` 向后兼容，mapping 层可以由 `ToolSpec` 自动派生基础字段。
- 现有 `ToolSelector` 可保留为 fallback，但推荐只负责召回候选，不再直接作为最终裁决。
- 现有 gateway 中的硬编码闭环可逐步迁移为 mapping 的 `next_capabilities` 和 `workflow_hints`。
- 前端无需立即改动；如展示意图拆解和工具计划，可复用已有计划/工具调用侧栏。

## 11. 测试用例设计

### 11.1 单元测试

| 用例 | 输入 | 期望 |
|:---|:---|:---|
| 只读主机查询 | `有哪些在线主机` | 命中 `Host.List`，无审批 |
| 概念解释 | `资产采集是什么` | 不命中 `Asset.Collection.Trigger` |
| 明确采集 | `对在线主机做资产采集` | 生成 Host.List + Asset.Collection.Trigger + Asset.Collection.Get |
| 缺少目标修复 | `帮我修复一下` | `need_clarification=true`，不注入写工具 |
| 漏洞脚本闭环 | `为 CVE-xxx 生成并执行 POC` | 生成脚本、状态、执行、任务查询计划 |
| 高风险阻断 | `阻断这个告警` | 需要明确 alert_id，且创建审批 |
| LLM 返回不存在能力 | `candidate_capabilities=["delete_everything"]` | 后端拒绝，记录决策原因 |
| 参数来源缺失 | `阻断这个` 且无页面上下文 | 追问具体告警，不生成阻断计划 |
| 状态机越级 | 未创建任务直接查询采集任务 | 拒绝，原因是缺少 `task_id` |
| 后置条件失败 | 采集触发工具未返回 `task_id` | 标记工具失败，不进入结果查询步骤 |
| 评分不足 | 候选工具相似但对象不匹配 | 拒绝或追问，不执行写工具 |

### 11.2 集成测试

1. 新建助手会话，输入“查询当前主机情况”，确认执行只读工具并返回实际数据。
2. 输入“进行资产采集任务，并分析哪个主机有 MySQL，分析 MySQL 是否有漏洞”，确认计划覆盖资产采集、软件搜索、漏洞列表/影响范围。
3. 输入“帮我修复”，确认前端收到追问，而不是创建任务。
4. 输入高风险阻断请求，确认进入审批态，审批通过后继续后续总结。
5. 输入“资产采集是什么”，确认不产生 `assistant_tool_calls` 写工具记录。
6. 输入“采集当前页面主机资产”，确认 `host_ids` 来自页面上下文，且 `arg_sources` 可审计。
7. 模拟 LLM 返回错误候选能力，确认 `ToolDecisionRecord.decision=rejected`。

### 11.3 回归测试

- `go test ./internal/assistant`
- `go test ./internal/api/handler -run Assistant`
- 前端如展示新 metadata，再执行 `npm run test -- AssistantWorkspace.test.ts --run`

### 11.4 可观测性检查

- 每个工具调用都能通过 `decision_trace_id` 反查裁决记录。
- 被拒绝的候选工具记录拒绝原因，至少包含一个失败硬性闸门。
- 写操作记录用户显式意图原文、审批状态和参数来源。
- 工具执行成功但后置条件失败时，最终回答必须包含证据缺口。

## 12. 实施计划

1. 定义 `IntentBreakdown`、`ToolCapabilityMapping`、`ToolExecutionPlan` 结构体。
2. 定义 `ToolUseContract`、`ArgBindingRule`、`ToolDecisionRecord` 和结果校验结构。
3. 为现有 60+ 工具补齐内置 mapping，先覆盖主机、资产、漏洞、基线、检测、弱密码。
4. 实现 `IntentDecomposer`，要求 LLM 只输出 JSON，不输出工具名决策。
5. 实现 `ToolArgumentBinder`，支持用户消息、页面上下文、会话上下文、策略默认值和前置步骤参数来源。
6. 实现 `ToolContractValidator`，完成硬性闸门、负例过滤、风险、审批、状态机和 schema 校验。
7. 实现 `ToolDecisionEngine`，完成候选召回、软评分、计划生成和追问。
8. 将 `Orchestrator` 工具选择阶段接入新链路，保留旧 selector 作为 fallback。
9. 将 gateway 中已知业务闭环迁移为 `next_capabilities` 和 `workflow_hints`。
10. 增加 `ToolDecisionRecorder` 和调试接口，便于客户现场解释“为什么调用这个工具”。
11. 增加 `ToolResultVerifier`，对 `task_id`、结果数量、状态字段等关键后置条件做校验。
12. 补齐单元测试、集成测试和回归测试。

## 13. 回滚方案

如新链路出现误判，可通过配置回滚：

```text
ASSISTANT_INTENT_DECOMPOSE_ENABLED=false
ASSISTANT_TOOL_DECISION_ENGINE_ENABLED=false
ASSISTANT_TOOL_MAPPING_MODE=builtin
```

回滚后：

- 保留原有 `ToolSelector` 和 LLM 两阶段工具选择。
- 已写入的 `intent_breakdown`、`tool_execution_plan` metadata 不影响运行。
- 数据库 mapping 表如已创建，可保留但不读取。

## 14. 验收标准

1. 复杂业务问题不会只执行第一个命中的工具。
2. 概念解释类问题不会触发写操作。
3. 写操作缺少目标时必须追问。
4. 高风险工具必须进入审批或被拒绝。
5. 工具选择结果可解释：能看到意图拆解、mapping 命中原因、参数来源和风险裁决。
6. LLM 输出错误工具名或不存在能力时，后端不会执行。
7. 每一次工具调用都有 `decision_trace_id`，可追溯候选、拒绝原因、评分和参数来源。
8. 工具执行结果必须通过后置条件校验；校验失败时最终回答不能声明任务完成。
9. 自动补出的后续工具不会扩大用户授权范围。
