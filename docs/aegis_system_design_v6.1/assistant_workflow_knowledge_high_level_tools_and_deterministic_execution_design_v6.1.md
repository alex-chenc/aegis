# Aegis V6.1 智能助手流程知识、高层业务工具与确定性执行设计

**版本**：6.1  
**状态**：待实现  
**日期**：2026-07-21  
**影响范围**：`api-server/`、`frontend/`、数据库迁移、`agent-runtime` 工具描述符契约、离线发布包  

## 1. 文档定位与现行设计关系

本文针对智能助手在复杂业务目标下出现的工具误选、参数错误、底层 ID 传递失败、
异步操作未监控到终态以及“调用成功但业务未完成”等问题，定义 V6.1 的增量设计：

1. 将各业务模块的流程指导知识结构化为可检索的 `WorkflowSpec`；
2. 将需要多个底层工具协作的业务闭环封装为高层业务工具；
3. 增加工具暴露等级，只向模型提供当前目标需要的少量工具；
4. 由后端完成实体解析、前置条件、权限审批、任务下发、状态轮询、覆盖率和结果验真；
5. `agent-runtime` 只执行后端 Mapping 生成的单工具绑定步骤，不再拥有工具选举权。

本文与以下现行文档共同生效：

- `fix/assistant_generic_agent_flow_only.md`
- `fix/assistant_mapping_authorization_and_truthful_completion_fix.md`
- `fix/assistant_runtime_evidence_and_async_execution_fix.md`

`assistant_tool_mapping_and_intent_decomposition_design_v6.1.md` 已标记为历史废弃设计，
不得作为本文实现依据。

如果本文与现行修复文档存在表面冲突，按以下边界解释：

- 通用意图、授权和 ToolGateway 层不得按关键词暗中补跑业务步骤；
- 所有工具选举只能来自 capability exact mapping；Runtime 不得通过自由
  `tool_name` 再次选举、增加或替换工具；
- `WorkflowSpec` 只用于能力检索、模型指导和确定性校验，不直接授权写操作；
- 高层业务工具是模型显式选择的单一业务能力，其领域编排器可以执行该工具公开契约
  内部的确定性状态机；
- 高层工具内部调用底层写能力时，仍必须携带绑定用户意图、范围和审批的执行授权；
- 未经高层工具契约声明和授权的底层工具不得被隐式调用。

## 2. 问题背景与根因

### 2.1 最新会话暴露的问题

最新基线会话要求对用户描述的“159IP”执行 CIS Ubuntu 24.04 全模板检查、自动修复
五轮并监控结果。实际运行出现了以下偏差：

- 14 次工具调用中有 9 次失败，其中多次为参数或步骤范围校验失败；
- 模型将自然语言主机选择器当成 `host_id`，而不是先解析为真实主机 UUID；
- 模板包含 524 条规则，但大结果压缩后模型只继续携带了少量规则 ID；
- 检测和修复脚本大部分尚未准备完成，任务链路仍可能继续下发；
- 任务服务可以在零任务创建时返回任务组引用，传输层将其包装为成功；
- 下发结果使用任务组 ID 和任务 ID，后续查询工具却缺少统一任务组状态入口；
- 工具已经支持 `auto_verify` 和 `max_rounds`，但规划阶段缺少参数语义，实际调用未正确
  传递用户要求的五轮策略；
- 最终结果与用户要求的“下发并监控修复闭环”存在明显差距。

### 2.2 当前架构的首要问题

当前智能助手注册了 69 个业务和系统工具。执行模型最终只看到授权后的工具子集，
但意图拆解模型会收到几乎全部 `Enabled=true` 的非系统 capability 目录，由模型一次性
选择 `candidate_capabilities`。

现有字段的职责如下：

| 字段 | 当前职责 | 不能解决的问题 |
| --- | --- | --- |
| `Enabled` | 工具是否注册启用 | 不能区分是否对模型可见 |
| `DefaultWhitelisted` | 默认审批/白名单策略 | 不能作为目录暴露策略 |
| `AutoCallable` | 运行时是否可自动调用 | 不能限制意图模型看到工具 |
| `Risk` | 风险等级 | 不能表达工具是高层、配套还是内部工具 |

因此，不在白名单或不能自动调用的工具仍可能出现在意图模型候选目录中。当前
`Tool.Search` 也会搜索所有启用工具，而不是本轮用户真正可见的工具集合。

### 2.3 流程知识和参数语义不完整

当前流程知识分散在：

- 工具中文 `Description`；
- 少量 `ModelDescription`；
- `ExecutionContract` 和 `ResultContract`；
- Handler 中的默认参数和校验；
- 服务代码中的业务状态机；
- 个别工具返回的 `next_action`；
- 历史设计文档和模块文档。

模型无法在一次规划中稳定获得完整的前置条件、参数来源、状态分支、终态定义和失败
恢复方式。执行阶段还会移除参数 Schema 中的 `description/title/examples`，模型通常只
能看到参数名、类型、required 和部分枚举，无法理解：

- 一个字符串究竟是自然语言选择器、UUID、CVE ID、MITRE ID 还是任务组 ID；
- `template_id` 和 `rule_ids` 是替代关系还是同时必需；
- `auto_verify=true` 时 `max_rounds` 才生效；
- 触发工具返回的是终态结果还是异步受理；
- 后续查询需要使用哪个结果字段。

### 2.4 根因结论

问题不能归因于单一模型解析能力，也不能只通过增加几个调用示例解决。根因是系统将
过多确定性责任交给模型，同时没有提供足够集中的业务契约：

1. 模型选择空间过大；
2. 底层工具抽象层级过低；
3. 自然语言对象与内部 ID 没有统一解析层；
4. 大批量 ID 需要经过模型上下文传递；
5. 异步触发、任务组、单任务和终态证据缺少统一抽象；
6. 工具返回成功与业务成功仍存在契约断点；
7. 各模块流程知识完整度不一致。

## 3. 目标、非目标与成功标准

### 3.1 目标

1. 普通助手请求不再让意图模型从全部 69 个工具中直接筛选。
2. 模型主要选择用户可理解的高层业务能力，不复制大规模内部 ID。
3. 自然语言对象在进入写操作前全部解析为存在、唯一、有权限的真实实体。
4. 高层工具统一返回可监控的 `operation_id` 和标准业务状态。
5. 写操作只有在真实创建 side effect 且覆盖率满足要求时才能成功。
6. `accepted/running`、部分完成、失败、待输入和完整成功在后端、事件和 UI 中保持一致。
7. 所有最终成功结论可追溯到终态工具证据、任务记录和覆盖率。
8. 新增模块能够通过声明 `WorkflowSpec`、暴露策略和结果契约接入，不新增关键词路由。

### 3.2 非目标

- 不允许模型或 agent-runtime 绕过 Mapping 自由选择具体工具。
- 不让 ToolGateway 根据工具名称暗中补跑其他工具。
- 不允许流程知识绕过用户写意图、审批、RBAC、租户和目标范围校验。
- 不要求一次性删除全部现有底层工具或修改现有领域 API。
- 不允许仅凭提示词或示例决定业务成功。
- 不把领域流程硬编码到 `agent-runtime`。

### 3.3 用户可见成功标准

- 用户可以使用主机名、IP、IP 片段、页面当前对象等选择器提出目标，不需要知道 UUID。
- 助手能明确表达“待确认”“待审批”“已受理”“执行中”“部分完成”“失败”和“成功”。
- 用户要求“全部”时，助手必须报告期望数、覆盖数、失败数和未覆盖对象。
- 用户要求自动验证或多轮修复时，真实业务请求必须携带相同参数。
- 助手不会在零任务、无 side effect、脚本未完成或状态仍运行中时报告已完成。

## 4. 设计原则与责任边界

### 4.1 模型负责

- 理解用户目标和约束；
- 从当前暴露的工作流或 capability 中选择最合适能力；
- 对真正会改变目标或安全边界的歧义发起澄清；
- 在后端绑定的当前步骤内，根据结构化观察决定完成、等待、请求审批或结束；
- 基于证据生成用户可读总结。

### 4.2 Aegis 后端负责

- 工作流检索和工具暴露范围；
- capability 到工具的精确映射；
- Mapping 结果的执行顺序、单步骤工具绑定和不可变运行快照；
- 实体解析、参数补全来源和类型校验；
- RBAC、租户、风险、审批和范围约束；
- 高层业务工具内部的领域状态机；
- 幂等、任务创建、状态轮询和超时；
- 结果契约、覆盖率、side effect 和终态验真；
- 操作账本、审计和最终 claim 校验。

### 4.3 agent-runtime 负责

- 执行后端传入的 Mapping-bound initial plan；
- 在每个步骤中复制唯一绑定的工具名并产生合法参数，不能选举工具；
- 展示真实 observation、call ID 和错误；
- 对通用异步状态工具执行有界轮询；
- 只有终态证据满足步骤条件时完成步骤。

## 5. 全模块流程指导知识

下表定义 V6.1 应提供给工作流检索器和高层业务工具的模块级流程知识。模块领域服务
仍是业务事实来源，`WorkflowSpec` 不复制脚本内容或敏感数据。

| 模块 | 标准流程 | 必需完成证据 |
| --- | --- | --- |
| 主机管理 | 解析用户选择器 → 查询候选 → 唯一绑定主机 UUID → 校验权限 → 查询详情和 Agent 状态 | 唯一真实主机、匹配依据、权限通过；需要 Agent 操作时主机在线 |
| Agent 实时取证 | 解析主机 → 校验在线 → 按需获取进程、进程树、网络、打开文件和日志 → 记录采集时间 | 真实 Agent 返回、采集时间和来源；离线、超时、跳过不能算成功 |
| 资产管理 | 解析范围 → 判断资产新鲜度 → 触发采集 → 监控任务 → 查询软件、应用和摘要 | 每台主机和资产类型有终态，记录成功、失败、跳过和数据时间 |
| 漏洞扫描 | 解析主机 → 校验资产清单 → 启动扫描 → 监控 → 查询漏洞和影响主机 → 汇总风险 | 扫描终态、有效 scan ID、请求主机与实际覆盖一致 |
| CVE 查询 | 精确查询目录 → 非空直接使用 → 仅空结果时启动自定义补录 → 监控补录 | 不重复补录；成功时存在 `result_vulnerability_id` |
| 漏洞验证与修复 | 解析 CVE/主机 → 生成 POC/FIX → 等待脚本可用 → 执行 → 监控任务 → 修复复测 | 脚本终态、非空任务引用、逐主机结果和复测终态 |
| 基线检查与修复 | 解析主机/模板 → 等待模板解析 → 后端枚举全部规则 → 确保 CHECK/FIX 脚本 → 检查 → 监控 → 审批修复 → 多轮复查 | `规则×主机` 覆盖完整、脚本就绪、任务真实落库、修复复查终态 |
| 任务中心 | 区分操作、任务组和单任务 → 查询组进度 → 展开失败任务 → 重试、取消或结束 | 组内所有任务可解释；零任务不能成功；组 ID 不当作单任务 ID |
| 异常检测 | 统计/趋势 → 告警筛选 → 详情 → 研判 → 解决、阻断或观察 → 回读状态 | 告警存在且状态适用；阻断产生真实记录；解决后状态已持久化 |
| 主机攻击研判 | 解析主机/告警 → 收集告警、漏洞、基线、阻断和现场证据 → 形成时间线与攻击路径 | 报告列出证据来源、时间和缺口，缺失证据不能推断为安全 |
| 弱密码 | 确认应用资产 → 分析候选应用 → 选择/生成字典 → 审批扫描 → 监控 → 查询命中 → 整改复测 | 候选应用真实、任务终态、密码脱敏、整改后复测 |
| Sigma 规则 | 选择 MITRE 和样本 → 生成 → 语法校验 → 样本回放/误报评估 → 进入检测包 | 生成、语法和回放结果齐全，生成成功不等于可启用 |
| 动态检测包 | 草稿 → 构建 → 等待构建 → 校验产物 → 审批签名 → 审批启用 → Agent 分发确认 | 构建产物、签名、版本、分发和 Agent 确认 |
| 阻断策略 | 查询并唯一定位 MITRE 策略 → 变更预览 → 审批 → 更新 → 回读 → 审计 | 前后差异、操作者、审批、持久化结果和审计记录 |
| 外部 MCP | 查询数据源 → 权限/Schema → 必要时测连接 → 单源/多源查询 → 归一化脱敏 → 融合分析 | query ID、来源、时间范围、脱敏状态和失败数据源 |
| 配置、审计、通知 | 根据配置键、来源、时间和状态查询 → 分页覆盖 → 关联操作或任务 | 查询范围、分页总量、数据时间和关联对象 |
| 助手系统 | 获取上下文 → 检索工作流/能力 → 压缩会话 → 保留对象和操作状态 | 不丢失对象类型/ID、未完成操作、审批、证据和缺口 |

## 6. WorkflowSpec 设计

### 6.1 数据结构

V6.1 新增版本化、只读的工作流知识注册表。初期以代码仓库中的 Go 结构或 JSON/YAML
资源维护，随 api-server 发布，不允许运行时由普通管理员任意修改。

```go
type WorkflowSpec struct {
    ID                  string
    Version             string
    Domain              string
    Goal                string
    TriggerIntents      []string
    NegativeIntents     []string
    ObjectTypes         []string
    RequiredEntities    []WorkflowEntitySpec
    Parameters          map[string]WorkflowParameterSpec
    Preconditions       []WorkflowCondition
    Stages              []WorkflowStage
    CompletionPolicy    WorkflowCompletionPolicy
    RecoveryPolicy      WorkflowRecoveryPolicy
    Risk                ToolRisk
    ApprovalStages      []string
    ExposedCapabilities []string
    Examples            []WorkflowExample
}
```

每个工作流至少声明：

- 适用意图和明确反例；
- 用户可提供的自然语言选择器；
- 参数含义、默认值、格式、范围和条件依赖；
- 前置条件、阶段、条件分支和安全停止条件；
- 哪些阶段需要审批；
- 异步状态工具和有界轮询策略；
- 完成条件、覆盖率算法和所需证据；
- 可恢复失败、不可恢复失败和用户输入条件；
- 正确示例、歧义示例和禁止示例。

### 6.2 流程知识注入位置

不得把全部模块流程和全部工具 Schema 一次性写入系统提示词。按阶段渐进披露：

| 阶段 | 模型看到的内容 | 建议规模 |
| --- | --- | --- |
| 意图阶段 | 工作流卡片：ID、目标、领域、对象、风险 | 5～15 张 |
| 计划阶段 | 命中的完整 WorkflowSpec、高层工具摘要、完成条件 | 1～3 个工作流、5～12 个工具 |
| ReAct 阶段 | 当前步骤允许的工具、完整参数语义、结果引用和示例 | 1 个主工具、2～4 个配套工具 |
| 总结阶段 | 终态证据、覆盖率、失败和证据缺口 | 不再注入无关工具 |

参数 Schema 对选中工具保留精简英文 `description`、`format`、`enum`、
`minimum/maximum`、条件依赖和示例。禁止再次统一删除全部参数语义。中文 UI 描述继续
与英文模型契约分离。

### 6.3 工作流检索

工作流检索输入包括：

- IntentRouter 的领域、动作和对象；
- 页面和会话上下文对象；
- 用户明确写操作意图和约束；
- 当前助手模式、租户功能和用户权限。

检索只产生候选工作流，不产生执行顺序和写授权。候选工作流 ID 必须来自实时注册表，
模型输出后由后端做 exact membership 校验。没有匹配工作流时，助手仍可使用安全的
上下文只读工具回答或明确报告能力缺口。

## 7. 工具暴露与自动隐藏设计

### 7.1 暴露等级

`ToolSpec` 新增与审批策略独立的暴露策略：

```go
type ToolExposure string

const (
    ToolExposurePrimary    ToolExposure = "primary"
    ToolExposureContextual ToolExposure = "contextual"
    ToolExposureCompanion  ToolExposure = "companion"
    ToolExposureInternal   ToolExposure = "internal"
)

type ToolExposurePolicy struct {
    Exposure       ToolExposure
    WorkflowIDs    []string
    AssistantModes []string
    Discoverable   bool
    DirectCallable bool
    CatalogPriority int
}
```

| 等级 | 意图模型可见 | 执行模型可见 | 调用方式 |
| --- | --- | --- | --- |
| `primary` | 是 | 选择后可见 | 用户级高层业务工具 |
| `contextual` | 仅领域/页面/工作流匹配时 | 选择后可见 | 查询、详情和高级只读工具 |
| `companion` | 否 | 由契约按需追加 | 实体解析、状态和完成查询工具 |
| `internal` | 否 | 否 | 仅高层领域编排器调用的原子工具 |

`companion` 自动追加仅允许只读或低风险幂等工具。取消、修复、阻断、签名、启用等
改变状态的能力即使是高层操作的配套动作，也必须作为 `primary/contextual` 能力由用户
明确请求并经过授权，不能通过 companion 自动授权。

### 7.2 有效暴露范围

本轮有效工具集合为以下条件的交集：

```text
Enabled
∩ code exposure ceiling
∩ tenant feature
∩ user RBAC
∩ assistant mode
∩ page/context scope
∩ selected WorkflowSpec
∩ current approval/risk boundary
```

代码声明是暴露上限。数据库管理员策略默认只能收紧，不能把 `internal` 工具提升为
普通助手的 `primary`；需要提升时必须通过代码评审和新版本发布。

### 7.3 选择流水线

```text
User request
  -> domain/object intent
  -> WorkflowCatalog filtering
  -> LLM selects exact workflow/capability
  -> ToolExposureResolver
       enabled + RBAC + tenant + mode + workflow
  -> exact capability mapping
  -> ToolAuthorization hard gates
  -> append declared readonly companions
  -> freeze ExposureSnapshot
  -> compile immutable Mapping-bound execution plan
  -> agent-runtime receives the plan and selected descriptors
  -> every step allows exactly one mapped tool
```

每轮运行持久化 `ExposureSnapshot`，至少包含：

- workflow IDs 和版本；
- 候选和最终暴露工具；
- 被过滤工具及原因分类；
- 工具目录 hash；
- 用户、租户、模式和审批范围引用。

ToolGateway 必须校验调用来源：

- `source=model` 只能调用当前 Mapping-bound step 唯一绑定且
  `DirectCallable=true` 的工具；
- `source=workflow_engine` 只能调用当前高层操作授权令牌中声明的内部 capability；
- 模型即使猜出隐藏工具名，也返回 `tool_not_exposed_for_current_run`；
- 暴露校验失败必须发生在 Handler 之前，且不得产生业务副作用。

### 7.4 工具选举不可变规则

所有 Assistant 工具调用必须满足：

```text
tool_name ∈ exact_mapping(intent.capability)
tool_name == current_mapping_bound_step.tool_name
```

禁止以下路径：

- Intent 或 Runtime 直接把任意 `tool_name` 加入执行集合；
- Runtime 在 Mapping 后重新生成自由工具计划；
- 通过 Prompt、Tool.Search、历史经验或模型纠错增加未映射工具；
- Mapping 关闭、缺失或失败时退回动态工具选择。

Mapping 不可用时必须 fail closed。没有映射工具的请求可以自然语言回复，但 Runtime
收到任何工具描述符时必须同时收到不可变的 Mapping execution plan。该规则是代码级
安全不变量，不得仅依赖提示词。

### 7.5 Tool.Search 调整

普通助手的 `Tool.Search` 改为搜索“当前可见业务能力”，而不是所有已启用工具：

- 只返回 `primary` 和当前上下文允许的 `contextual`；
- 不返回 `internal`；
- `companion` 只在解释流程时作为关联能力展示，不允许主动授权写操作；
- 搜索结果不能直接改变本轮执行集合，必须重新经过暴露和授权校验；
- 高级模式可以增加只读原子工具，但仍不得暴露内部写工具。

### 7.6 兼容迁移默认值

暴露策略不能直接用空值代表 `primary`，否则新工具会意外进入全量目录。迁移过程为：

1. 新增字段但保持功能开关关闭；
2. 为全部内置工具显式写入初始暴露等级；
3. 在影子模式记录新旧目录差异；
4. 灰度启用新目录；
5. 稳定后将缺少 ExposurePolicy 的生产工具视为 `internal`，并在启动时记录错误。

## 8. 高层业务工具设计

### 8.1 工具清单

| 高层工具 | 主要输入 | 后端确定性职责 |
| --- | --- | --- |
| `Host.Resolve` | `host_selectors`、是否要求在线 | 模糊匹配、唯一性、UUID、权限、在线校验 |
| `Asset.Inventory.Refresh` | 主机选择、资产类型、是否强制 | 采集触发、监控、新鲜度和覆盖率 |
| `Host.Forensics.Collect` | 主机、采集项、时间范围 | 在线判断、受控采集、超时和结果归一化 |
| `Vulnerability.Assessment.Run` | 主机选择、扫描策略 | 资产前置、扫描、监控和漏洞汇总 |
| `Vulnerability.Remediation.Run` | CVE、主机、POC/FIX、复测轮次 | CVE 解析、脚本、执行、任务监控和复测 |
| `Baseline.Compliance.Run` | 主机、模板、规则范围、修复策略 | 全规则枚举、脚本准备、检查、修复和多轮复查 |
| `Operation.Get` | `operation_id` | 聚合所有异步领域状态和覆盖率 |
| `Operation.Cancel` | `operation_id`、原因 | 可取消性、领域取消和审计 |
| `Detection.Alert.Investigate` | 告警/主机、研判深度 | 多源证据收集、时间线和证据缺口 |
| `Detection.Alert.Respond` | 告警、动作、目标 | 处置预览、审批、适配校验、执行和回读 |
| `Credential.WeakPassword.Assessment.Run` | 主机/应用范围、字典策略 | 资产分析、候选应用、扫描、监控和命中汇总 |
| `DetectionRule.GenerateAndValidate` | MITRE、样本、保守度 | Sigma 生成、语法校验、样本回放和评分 |
| `DetectionPackage.Publish` | 包需求、目标版本、发布策略 | 草稿、构建、产物校验、签名、启用和分发确认 |
| `ExternalEvidence.SearchAndAnalyze` | 目标、时间、数据源范围 | Schema、连接、多源查询、脱敏和融合分析 |
| `Block.Policy.Change` | MITRE、期望配置 | 唯一定位、差异预览、审批、更新、回读和审计 |

配置、审计、通知、统计、列表等简单只读场景不强制包装为高层异步工具，保留为
`contextual` 工具即可。

### 8.2 高层工具的输入原则

高层工具优先接受用户语义选择器，不要求模型提供内部批量 ID：

```json
{
  "host_selectors": ["159IP"],
  "template_selector": "CIS Ubuntu 24.04",
  "scope": "all_rules",
  "remediation": {
    "enabled": true,
    "max_rounds": 5
  }
}
```

参数约束：

- `*_selector` 是自然语言选择条件，不能直接下发给领域服务；
- `*_id` 必须通过格式、存在性、租户和权限校验；
- `scope=all_*` 由后端分页或数据库查询完整展开，禁止经模型复制全部 ID；
- 条件参数使用 JSON Schema `if/then/else` 或后端等价校验；
- 高风险参数在解析范围后生成审批预览，范围变化则原审批失效。

### 8.3 Baseline.Compliance.Run

输入：

```json
{
  "host_selectors": ["string"],
  "template_selector": "string",
  "scope": "all_rules|selected_rules",
  "rule_selectors": [],
  "remediation": {
    "enabled": false,
    "max_rounds": 3
  },
  "idempotency_key": "string"
}
```

领域状态机：

```text
resolve_hosts
  -> resolve_template
  -> wait_template_parsed
  -> enumerate_rules_server_side
  -> ensure_check_scripts_ready
  -> ensure_fix_scripts_ready when remediation enabled
  -> dispatch_check
  -> monitor_check
  -> compute_noncompliant_scope
  -> approval_required when remediation enabled and targets exist
  -> dispatch_fix
  -> monitor_fix
  -> recheck until compliant or max_rounds reached
  -> verify_coverage
  -> terminal outcome
```

确定性条件：

- `expected_count = resolved_host_count × resolved_rule_count`；
- 脚本未准备完成时不得使用占位脚本下发；
- 无效主机或规则不能静默 `continue` 后返回成功；
- `created_count=0` 必须失败；
- 允许部分执行时，必须返回逐项 `invalid/skipped/failed`；
- 任务组必须能关联到真实任务记录；
- `max_rounds` 必须进入实际下发选项和操作账本；
- 达到最大轮次但仍不合规时结果为 `partially_succeeded` 或 `failed`，不能是成功。

### 8.4 Host.Resolve

`Host.Resolve` 是只读公共解析能力，输入选择器，输出：

```json
{
  "requested": ["159IP"],
  "resolved": [
    {
      "selector": "159IP",
      "host_id": "uuid",
      "hostname": "...",
      "ip": "192.168.152.159",
      "agent_status": "online",
      "matched_by": "ip_token_unique"
    }
  ],
  "ambiguous": [],
  "unresolved": [],
  "coverage": {
    "requested": 1,
    "resolved": 1
  }
}
```

任何非 UUID 字符串不得直接进入 UUID Repository 查询。多候选时返回 `needs_input`，
不得根据排序自动选择写操作目标。

### 8.5 Operation.Get

`Operation.Get` 是所有高层异步工具的只读 companion。模型和 UI 不再分别猜测 scan ID、
task group ID、task ID、build ID 和 query ID 的查询工具。领域引用保留在
`references` 中，但统一通过 `operation_id` 监控。

## 9. 统一操作契约与状态机

### 9.1 OperationHandle

所有异步或多阶段高层工具统一返回：

```json
{
  "operation_id": "op_xxx",
  "workflow_id": "baseline_compliance",
  "workflow_version": "6.1.1",
  "status": "running",
  "terminal": false,
  "current_stage": "dispatch_check",
  "scope": {
    "requested_hosts": 1,
    "resolved_hosts": 1,
    "expected_rules": 524
  },
  "counts": {
    "expected": 524,
    "created": 524,
    "running": 524,
    "succeeded": 0,
    "failed": 0,
    "skipped": 0
  },
  "references": {
    "task_group_ids": ["uuid"]
  },
  "violations": [],
  "next_poll_after_seconds": 5
}
```

### 9.2 状态定义

```text
resolving
  -> needs_input
  -> approval_required
  -> accepted
  -> running
      -> succeeded
      -> partially_succeeded
      -> failed
      -> cancelled
      -> timed_out
```

终态为：`succeeded/partially_succeeded/failed/cancelled/timed_out`。

- `accepted/running` 不能满足步骤和目标完成；
- `approval_required/needs_input` 必须明确阻塞原因；
- `partially_succeeded` 不能映射为 UI 的完整“已完成”；
- `skipped` 是单项结果，不应作为整个用户目标默认成功；
- 操作终态与 Runtime 生命周期状态分别保存。

### 9.3 覆盖率不变量

所有集合型操作满足：

```text
expected = succeeded + failed + skipped + cancelled + unresolved
created > 0 for any claimed dispatch
requested_all = true  => unresolved = 0 for succeeded outcome
```

如果业务过程还包含多轮，每轮都应保存独立计数，并以最后验证轮次和累计失败共同判断
最终结果。

## 10. 后端确定性校验

### 10.1 参数结构校验

- UUID、RFC3339、CVE、MITRE、版本使用明确 `format/pattern`；
- 枚举、数值范围、数组最小长度、唯一项和闭合对象必须执行；
- 使用 `oneOf/anyOf/if/then/else` 表达替代参数和条件参数；
- Schema 校验失败发生在领域 Handler 之前；
- 未声明字段不得被 Handler 静默忽略或触发默认行为。

### 10.2 实体解析校验

新增统一 `EntityResolverRegistry`，至少支持：

- host selector → host UUID；
- template selector → baseline template UUID；
- CVE string → vulnerability UUID；
- alert selector → alert ID；
- MITRE selector → policy/rule ID；
- package/source/task/operation 引用解析。

解析结果包含来源、匹配方式、候选数、权限状态和可信状态。写操作只接受唯一解析结果。

### 10.3 权限与审批校验

- 原始用户消息必须存在相应写意图；
- RBAC 和租户权限对解析后的真实对象重新校验；
- 审批记录绑定 `workflow_id + action + scope_hash + parameters_hash`；
- 目标范围、动作或高风险参数改变后必须重新审批；
- 高层工具内部调用不能继承一个无范围的全局写授权。

### 10.4 业务前置条件校验

按 WorkflowSpec 和领域服务检查：

- Agent 在线和能力版本；
- 资产数据新鲜度；
- 模板解析状态；
- 规则与模板所属关系；
- 脚本生成终态和脚本类型；
- 告警当前状态与阻断动作适配性；
- 包构建、签名和启用顺序；
- MCP 来源权限、Schema 和连接状态。

### 10.5 下发和事务校验

- 先校验全部目标，再创建任务；
- 默认采用全有或明确的部分成功策略，禁止静默跳过；
- 创建结果必须返回真实 side effect 引用；
- 任务组创建和任务行创建应在同一事务或具备补偿语义；
- 下发失败必须保留已创建任务的真实状态，不能伪装为未发生；
- 幂等键绑定用户、工作流、解析范围和参数，避免模型重试重复执行。

### 10.6 结果和后置条件校验

`ToolResultVerifier` 改为消费声明式 ResultContract、OperationHandle 和领域查询证据，
不再只检查 `task_id_created` 等固定字段。

必须校验：

- operation、artifact、side effect 引用存在且可回读；
- 任务组下存在预期数量的任务；
- 状态字段属于已声明终态；
- 每个目标对象都有结果；
- 修复后的复测状态符合用户目标；
- “全部”范围达到完整覆盖；
- 失败、跳过、离线和超时被纳入最终 outcome。

### 10.7 最终陈述校验

模型生成的成功 claim 必须引用：

- 真实 call ID；
- `terminal=true` 的 ToolOutcome；
- 匹配的 operation/side effect/artifact；
- 满足要求的覆盖率。

没有任务引用不得声称“已下发”；没有复测证据不得声称“已修复”；状态仍运行中只能
声称“已受理”或“处理中”。ClaimValidator 失败时使用操作账本生成保守回退答案。

## 11. 内部编排授权

高层工具内部调用 `internal` 工具时使用短期 `WorkflowExecutionGrant`：

```json
{
  "operation_id": "op_xxx",
  "workflow_id": "baseline_compliance",
  "workflow_version": "6.1.1",
  "allowed_capabilities": [
    "generate_baseline_scripts",
    "run_baseline_check",
    "run_baseline_fix"
  ],
  "scope_hash": "sha256:...",
  "parameters_hash": "sha256:...",
  "approval_id": "optional",
  "expires_at": "RFC3339"
}
```

约束：

- Grant 由后端在实体解析和授权通过后签发；
- 只能用于当前 operation；
- 写 capability 必须来自高层工具契约和用户明确意图；
- 审批前不得签发包含高风险写 capability 的可执行 Grant；
- ToolGateway 校验 Grant、调用来源、范围 hash 和有效期；
- Grant 不能转换为普通模型可见工具。

## 12. 数据与接口设计

### 12.1 ToolSpec 和工具策略

`ToolSpec` 增加 `ExposurePolicy`。`assistant_tool_policies` 建议新增：

- `default_exposure`、`exposure`；
- `discoverable`；
- `direct_callable`；
- `assistant_modes JSONB`；
- `workflow_ids JSONB`。

现有 `enabled/whitelisted/risk_level` 继续保留，不能与 exposure 合并。

### 12.2 工具选择记录

`assistant_tool_selections` 建议增加：

- `selected_workflows JSONB`；
- `workflow_versions JSONB`；
- `exposure_snapshot JSONB`；
- `catalog_hash VARCHAR`；
- `catalog_count INTEGER`。

### 12.3 统一操作账本

新增 `assistant_operations`：

| 字段 | 说明 |
| --- | --- |
| `operation_id` | 对外统一操作引用 |
| `session_id/run_id` | 会话和运行关联 |
| `workflow_id/version` | 使用的流程契约 |
| `request` | 规范化请求，不保存敏感正文 |
| `resolved_scope` | 解析后的对象及覆盖摘要 |
| `status/current_stage/terminal` | 操作状态 |
| `counts` | 期望、创建、成功、失败、跳过等计数 |
| `references` | 任务组、扫描、构建等领域引用 |
| `violations` | 校验失败和覆盖缺口 |
| `idempotency_key` | 幂等键 |
| `approval_id` | 审批引用 |
| `created_by/created_at/updated_at` | 审计字段 |

可选新增 `assistant_operation_steps` 保存领域阶段状态；如果阶段详情已存在于可靠领域表，
则只保存引用和摘要，避免复制领域事实。

### 12.4 内部接口

建议新增以下内部组件：

- `WorkflowRegistry`
- `WorkflowRetriever`
- `ToolExposureResolver`
- `EntityResolverRegistry`
- `HighLevelOperationService`
- `OperationRepository`
- `WorkflowExecutionGrantIssuer`
- `CoverageValidator`
- `OperationResultNormalizer`

高层工具仍通过现有 ToolRegistry 注册；领域编排服务优先直接调用领域 Service，而不是
模拟模型逐个调用公开 Tool Handler。共享校验和结果归一化应下沉为可复用服务，避免
领域 API 与助手工具出现两套业务语义。

### 12.5 HTTP 与前端

建议增加：

```text
GET  /api/v1/assistant/operations/:operation_id
POST /api/v1/assistant/operations/:operation_id/cancel
```

前端展示：

- 工作流名称和当前阶段；
- 请求范围、已解析范围和覆盖率；
- 已受理、运行中、待审批、待输入、部分成功和终态失败；
- 逐对象失败摘要和安全的错误码；
- 任务组、扫描或报告的跳转引用；
- 不能把 `partially_succeeded` 渲染为完整“已完成”。

## 13. 现有工具初始暴露分级

以下是 69 个现有工具的初始建议。高层工具实现前，相关底层写工具可保持现状并通过
功能开关逐步迁移；高层工具上线后按表切换。

| 模块 | `primary/contextual` | `companion` | `internal` |
| --- | --- | --- | --- |
| 系统 | `Tool.Search`（contextual） | `Context.Get`、`Session.Summarize` | — |
| 主机 | `Host.List`、`Host.Get`、`Host.AgentStatus.Get`（contextual） | — | — |
| Agent 取证 | — | — | `Agent.Process.List`、`Agent.Process.Tree`、`Agent.Network.List`、`Agent.File.OpenList`、`Agent.Log.Query` |
| 资产 | `Asset.Summary.Get`、`Asset.Software.List`、`Asset.Application.List`、`Asset.Collection.List`（contextual） | `Asset.Collection.Get` | `Asset.Collection.Trigger` |
| 漏洞 | `Vulnerability.List`、`Vulnerability.AffectedHosts`、`Software.Installed.Search`（contextual） | `Vulnerability.CustomQuery.Status`、`Vulnerability.Scan.Status`、`Vulnerability.Script.Status` | `Vulnerability.CustomQuery.Start`、`Vulnerability.Scan.Start`、`Vulnerability.Scan.Stop`、`Vulnerability.Script.Generate`、`Vulnerability.Script.Execute` |
| 基线 | `Baseline.Template.List`、`Baseline.Template.Status.Get`（contextual） | — | `Baseline.Template.Rules.List`、`Baseline.Script.Generate`、`Task.RunCheck`、`Task.RunFix` |
| 任务 | `Task.List`（contextual） | `Task.GetDetail` | — |
| 检测查询 | `Detection.Alert.List`、`Detection.Alert.Get`、`Detection.Statistics.Get`、`Detection.Trend.Get`（contextual） | — | — |
| 检测处置 | — | — | `Detection.Alert.Resolve`、`Detection.Alert.Block` |
| 攻击研判 | `Investigation.HostAttack.Analyze`（过渡期 contextual） | — | `Investigation.HostAttack.Plan` |
| 弱密码 | `Credential.WeakPassword.GenerateDictionary`、`Credential.WeakPassword.QueryFindings`、`Credential.WeakPassword.Explain`（contextual） | `Credential.WeakPassword.QueryProgress` | `Credential.WeakPassword.AnalyzeApplications`、`Credential.WeakPassword.Scan` |
| Sigma | `SigmaRule.List`（contextual） | — | `SigmaRule.Generate` |
| 检测包 | `Package.List`、`Package.Get`（contextual） | — | `Package.Draft.Generate`、`Package.Build.Start`、`Package.Sign`、`Package.Enable` |
| 阻断策略 | `Block.Policy.List`（contextual） | — | `Block.Policy.Update` |
| 外部 MCP | `ExternalMCP.Source.List`（contextual） | `ExternalMCP.Source.GetSchema` | `ExternalMCP.Source.TestConnection`、`ExternalMCP.Query`、`ExternalMCP.MultiQuery`、`ExternalMCP.Analyze` |
| 配置 | `Config.Get`（contextual） | — | — |
| 审计 | `Audit.Log.List`（contextual） | — | — |
| 通知 | `Notification.List`（contextual） | — | — |

新高层工具默认标记为 `primary`；`Host.Resolve` 可以同时作为公共 contextual 能力，
`Operation.Get` 作为 companion，`Operation.Cancel` 作为需要明确取消意图的
contextual 能力。所有内部写工具继续保留 ToolSpec、风险和审批契约，但不进入普通
模型目录。

## 14. 安全、日志与可运维性

### 14.1 安全

- 暴露隐藏不是授权替代；隐藏工具仍必须经过 ToolGateway 校验；
- 工作流检索不得扩大 RBAC 或租户范围；
- 高层工具不得把用户自然语言直接拼成 Shell 或脚本；
- 脚本正文、密码、Token、Authorization 和外部 MCP 敏感字段不得进入操作账本；
- 选择器、解析结果和审批范围必须可审计；
- 外部 MCP 结果继续归一化和脱敏后才能进入模型上下文。

### 14.2 结构化日志

至少记录：

- `workflow_id/workflow_version`；
- `exposure_snapshot_id/catalog_hash/catalog_count`；
- `candidate_tools/exposed_tools/hidden_reason`；
- `operation_id/current_stage/status/terminal`；
- `selector_count/resolved_count/ambiguous_count`；
- `expected/created/succeeded/failed/skipped`；
- `approval_id/scope_hash`；
- `result_validation/coverage_validation`；
- `goal_outcome`。

不得记录完整用户敏感查询、脚本正文、密码、密钥或大段工具结果。

### 14.3 指标

建议增加：

- 每轮意图目录工具数、执行目录工具数；
- 工具选择纠正率、参数校验失败率；
- 隐藏工具猜测调用次数；
- 实体解析未命中/多候选率；
- 高层操作零任务拒绝次数；
- operation 各状态耗时和超时率；
- 完整成功、部分成功、失败和待输入比例；
- 最终 ClaimValidator 拒绝率。

## 15. 测试设计

### 15.1 单元测试

| 用例 | 期望 |
| --- | --- |
| intent 目录构建 | 不包含 `internal/companion`，只含本轮相关能力 |
| contextual 工具领域不匹配 | 不进入目录 |
| RBAC/租户无权限 | 工具和工作流均不可见 |
| 模型猜测 internal 工具 | Gateway 返回 `tool_not_exposed_for_current_run` |
| Tool.Search | 只返回当前可见能力 |
| 参数 Schema | 保留英文语义、format、枚举和条件关系 |
| 非 UUID 选择器 | 不进入 UUID Repository，先走 Resolver |
| 多主机候选写操作 | 返回 `needs_input` |
| scope=all_rules | 后端完整枚举，不依赖模型传 ID |
| created_count=0 | 操作失败，不产生成功 ToolOutcome |
| accepted/running | 不满足完成条件 |
| 部分覆盖 | outcome 为 `partially_succeeded` |
| 幂等重试 | 不重复创建任务或阻断动作 |

### 15.2 契约测试

每个高层工具必须具备：

- 输入 Schema 正常、边界和非法值测试；
- 实体解析 0、1、多候选测试；
- 前置条件失败测试；
- 审批前后和范围变化测试；
- 异步状态迁移测试；
- 逐对象部分失败和覆盖率测试；
- OperationHandle 和 ToolOutcome 映射测试；
- 最终 claim 允许和拒绝测试。

### 15.3 集成测试

至少覆盖：

1. 主机选择器 → 唯一解析 → 资产采集 → 终态查询；
2. 漏洞扫描 → 状态监控 → 结果汇总；
3. CVE 脚本生成 → 执行 → 修复复测；
4. 告警研判 → 审批阻断 → 状态回读；
5. 弱密码候选应用 → 扫描 → 进度 → 命中；
6. Sigma 生成 → 校验 → 检测包构建签名分发；
7. 外部 MCP 多源查询中一个来源失败，结果为部分成功；
8. Agent 离线、超时、取消、重复请求和服务重启恢复。

### 15.4 核心真实回归

使用以下请求作为 V6.1 首要验收：

> 对“159IP”执行 CIS Ubuntu 24.04 全模板检查，不合规自动修复，最多 5 轮并持续监控。

必须满足：

1. “159IP”不会被当成 UUID；
2. 唯一解析到目标 IP 对应主机，多候选时先询问；
3. 模板全部规则由后端枚举，不经过模型传递 524 个规则 ID；
4. CHECK/FIX 脚本未准备好时不使用占位脚本下发；
5. 五轮和自动验证参数进入真实业务请求；
6. 创建任务数为零时操作失败；
7. 操作能通过统一 operation ID 监控；
8. 数据库存在任务组和预期任务记录；
9. 只有终态且覆盖完整时才能报告成功；
10. 离线、失败、跳过或超时必须报告为部分完成或失败并列出缺口。

## 16. 分阶段实施

### P0：正确性止血

1. 增加工具 ExposurePolicy 和目录过滤；
2. 增加 `Host.Resolve`；
3. 增加统一 `Operation.Get` 和任务组状态适配；
4. 修复零任务、无 side effect 返回成功；
5. 为全部写工具补齐 ResultContract；
6. 保留选中工具的参数语义和条件约束；
7. UI 区分部分成功和完整成功。

### P1：高频业务闭环

1. `Baseline.Compliance.Run`；
2. `Vulnerability.Assessment.Run`；
3. `Vulnerability.Remediation.Run`；
4. `Asset.Inventory.Refresh`；
5. 建立 WorkflowRegistry、OperationRepository 和覆盖率校验。

### P2：安全运营闭环

1. 告警研判与处置；
2. 弱密码评估；
3. Sigma 生成验证；
4. 检测包发布；
5. 阻断策略变更；
6. 外部 MCP 证据分析。

### P3：收敛和评测

1. 普通模式隐藏底层原子工具；
2. 高级模式仅开放必要只读原子工具；
3. 使用历史失败会话建立回放数据集；
4. 对比工具目录规模、失败率、完成率和 token 消耗；
5. 清理不再使用的 ToolUseContract 硬编码和历史提示词。

## 17. 兼容性、发布与回滚

### 17.1 兼容性

- 现有 69 个工具暂不删除，避免 API 和历史记录断裂；
- ToolSpec 新字段使用显式迁移，灰度阶段允许旧行为；
- 高层工具复用现有领域 Service，不改变 Agent gRPC 命令协议；
- OperationHandle 是助手统一视图，不替代领域表中的事实状态；
- 已运行会话继续使用创建时保存的 ExposureSnapshot 和工作流版本；
- 新旧高层工具并存期间，同一请求只能进入一种执行路径，禁止双重下发。

### 17.2 发布

建议新增 V6.1 数据库迁移，迁移号应接续当前最新迁移；发布顺序：

1. 数据库增加 exposure、selection snapshot 和 operation 表/字段；
2. 发布向后兼容的 api-server，默认关闭新高层写工具；
3. 开启只读目录过滤和影子 WorkflowRetriever；
4. 灰度 `Host.Resolve`、`Operation.Get` 和资产/漏洞只读闭环；
5. 经审批启用基线、漏洞修复等写闭环；
6. 更新 frontend 状态展示；
7. 更新离线包中的迁移、环境模板、api-server/frontend 镜像和启动检查。

发布后必须记录当前 exposure 模式、WorkflowSpec 版本和是否启用高层工具，禁止静默
降级到全量工具目录。

### 17.3 回滚

- 通过显式开关恢复旧 capability 目录和底层工具暴露；
- 停止创建新的高层 operation，但继续允许查询已有 operation；
- 不删除已创建的任务、脚本、阻断记录和操作账本；
- 对回滚时处于 accepted/running 的 operation 按领域引用继续核查，不能重新下发；
- 恢复上一版 api-server/frontend 镜像时保留新增数据库字段；
- 离线包回滚必须使用对应版本镜像和迁移兼容说明。

## 18. 风险与缓解

| 风险 | 缓解 |
| --- | --- |
| 隐藏工具导致模型无法完成长尾任务 | 保留 contextual 搜索、高级只读模式和明确能力缺口 |
| 高层工具成为新的大而全服务 | 每个工具绑定一个清晰用户目标和单一 operation，不合并无关领域 |
| 工作流知识与代码漂移 | WorkflowSpec 版本化，契约测试绑定领域状态和结果字段 |
| 内部编排绕过审批 | 使用绑定 scope/parameter hash 的短期 Grant，Gateway 二次校验 |
| operation 与领域状态不一致 | 领域表为事实来源，operation 保存引用并在查询时对账 |
| 旧会话与新目录不一致 | 每轮冻结 ExposureSnapshot 和 WorkflowSpec 版本 |
| 模型仍输出错误参数 | 保留严格 Schema、后端实体/前置校验和一次纠正上限 |
| 批量范围过大 | 后端分页、并发上限、超时、取消和逐对象计数，不经模型传递 ID |

## 19. 验收标准与完成定义

### 19.1 验收标准

1. 普通请求的意图目录不再包含全部 69 个工具能力。
2. `internal` 和 `companion` 工具不会出现在意图模型可选目录。
3. 每个执行步骤只暴露当前允许工具，Gateway 能拒绝模型猜测的隐藏工具。
4. Tool.Search 只返回当前用户、租户、模式和工作流允许的能力。
5. 高层工具不要求模型传递大批量 UUID。
6. 所有写操作拥有真实 side effect、任务记录和覆盖率后才能成功。
7. 所有异步高层工具可用统一 operation ID 查询到终态。
8. 部分成功不会在会话和 UI 中显示为完整成功。
9. 最终成功结论可追溯到 call ID、operation、任务和终态证据。
10. 核心“159IP 全量基线五轮修复”真实回归满足第 15.4 节全部要求。
11. Assistant 定向测试、api-server 构建、数据库迁移、前端构建和受控 API 冒烟通过。
12. 离线发布包包含对应迁移、配置、镜像和回滚说明，并完成全新环境安装验证。

### 19.2 完成定义

只有以下条件同时满足，本设计才视为完成：

- WorkflowSpec、ExposurePolicy、高层工具和后端校验已实现；
- 所有现有工具完成显式暴露分级；
- P0/P1 契约和集成测试通过；
- 真实受控基线与漏洞会话达到终态且证据一致；
- UI、日志、数据库状态和最终回答使用同一完成语义；
- 发布和回滚路径完成验证；
- 文档、代码和实际部署版本一致。

## 20. 首批实现状态（2026-07-21）

本设计首批开发已经落入 api-server，范围聚焦最新会话暴露的工具选择、参数错误、基线
下发和异步完成误判问题：

- 新增 `WorkflowRegistry` 和 17 张 V6.1 模块级工作流卡片；意图拆解只接收命中的
  1～15 张卡片及过滤后的能力目录，并对模型返回的工作流 ID、能力 ID 做 exact
  membership 校验；
- `ToolSpec` 新增 `primary/contextual/companion/internal` 暴露策略；意图目录、
  `Tool.Search` 和模型直接调用分别执行过滤，`internal` 工具不能由模型直接调用；
- 所有生产工具的 `Description`、`ModelDescription`、参数 Schema `description` 和
  `ServiceBinding.Notes` 已统一为英文；中文别名仅作为多语言检索键，不进入执行契约；
- 模型参数 Schema 重新保留英文 `description`、`format`、`examples`、范围和枚举，
  api-server 在 Handler 前再次执行 required/type/format/enum/range/pattern/
  additionalProperties 确定性校验；
- 新增 `Host.Resolve`，支持 UUID、IP、主机名和 `159IP` 一类短标签，返回唯一匹配依据、
  歧义、未解析、离线和覆盖计数；写操作遇到歧义、离线或缺失目标时停止；
- 新增 `Baseline.Compliance.Run` 和 `Operation.Get`：后端解析主机和模板、枚举全量规则、
  等待 CHECK/FIX 脚本、持久化操作账本、原子创建 `规则×主机` 任务、下发检查并聚合
  自动修复/复查终态；
- 新增 `assistant_operations` 迁移，保存会话、运行、工作流版本、规范化请求、解析范围、
  阶段、计数、领域引用、违规、幂等键和终态；
- `TaskService.CreateAndDispatchTasks` 不再对无效 UUID、缺失实体、未就绪脚本或创建错误
  静默 `continue`，不再下发占位脚本；任务批次在事务中完整落库后才开始 Agent 下发，
  并强制校验 `created_count == expected_count > 0`；
- 已增加英文契约、Schema 保真、暴露过滤、工作流检索、主机短标签解析、基线全规则下发、
  操作终态和非法任务范围的回归测试。

后续高层工具仍按第 15、16 节分阶段实现。本次首批不宣称已经完成
`Asset.Inventory.Refresh`、`Vulnerability.Assessment.Run`、检测响应、弱密码、检测包发布
等全部高层包装；这些模块继续使用已收敛的 contextual 工具，直至对应高层编排服务上线。

## 21. 线上失败会话修正：全部在线主机基线闭环（2026-07-21）

### 21.1 触发条件与根因

会话“给存活的机器下发 CIS Ubuntu 基线、开启五轮自动修复”已正确解析出
`scope=all_alive_hosts`、模板和修复轮数，但通用 agent runtime 丢弃授权计划，重新让模型
把自然语言“存活的机器”填入 `Host.Resolve.host_selectors`。该字段只接受 UUID、IP、主机名
或短 IP 标签；因此先出现缺失参数、字符串代替数组，最终得到 `resolved=0`。

这不是在线主机或模板缺失：失败时目标主机有有效心跳，模板已完成解析且包含规则。首个
偏差是“语义范围”未转为后端可执行范围；后续失败则来自零匹配仍被按 transport success
处理，以及动态计划没有阻断依赖步骤。

### 21.2 修正契约

1. `Host.Resolve` 与 `Baseline.Compliance.Run` 增加互斥的 `target_scope`：
   `all_online_hosts`。服务端枚举当前连接或有效心跳主机，模型不得用自然语言分组填充
   `host_selectors`。
2. `Host.Resolve` 返回 `operation_status`。零解析、歧义、未解析或离线目标均为业务失败，
   不再作为满足前置条件的终态成功。
3. 当意图为 V6.1 基线合规工作流且 scope、模板已被结构化解析时，授权层生成固定顺序：
   `Host.Resolve -> Baseline.Compliance.Run -> Operation.Get`。固定计划向 Gateway 提供并覆盖
   `target_scope`、`template_selector`、`scope=all_rules` 与 `remediation`。所有其他包含工具的
   助手请求同样必须执行 capability Mapping 产生的单工具绑定计划；Runtime 不再拥有自由
   `tool_name` 选举权。
4. 运行循环正常结束但 `goal_outcome=failed` 时，最终回答必须输出“任务未完成”，不能显示
   `Task status: completed`；没有任务组证据时明确说明未下发任务。

### 21.3 验收与回归

- “所有存活/在线主机”请求不再调用 `Host.Resolve(["存活的机器"])`；
- 解析到的在线范围仅含在线主机，零在线主机时基线工具不会创建 operation 或下发任务；
- 固定计划的模型冲突参数不能覆盖后端绑定的 scope、模板、全规则和修复轮数；
- 前置主机解析失败后，高风险基线步骤及状态查询被依赖关系阻断；
- 失败会话的正文、SSE done 事件和持久化会话状态均表达失败，而非完成。
