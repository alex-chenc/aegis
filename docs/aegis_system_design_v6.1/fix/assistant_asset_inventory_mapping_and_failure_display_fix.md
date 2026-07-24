# Assistant 资产重采集 Mapping 编译与失败展示修复设计

## 1. 背景与问题证据

2026-07-24 最新失败会话：

```text
session_id: asst_670eb1ab
run_id:     run_90a75b6f
goal:       重新采集所有主机上的资产
mode:       mapping_bound_execution
approval:   full_access
outcome:    failed
durable tool calls: 0
```

意图拆解给出的 capability 顺序是：

```text
resolve_hosts -> list_hosts -> trigger_asset_collection
```

后端 Mapping 生成的实际计划却是：

```text
Asset.Collection.Trigger(host_ids="all")
  -> Host.List
  -> Host.Resolve
```

首步骤在执行前参数校验阶段失败。`Asset.Collection.Trigger.host_ids` 的 Schema
要求数组，Mapping 却把业务范围 `scope.kind=all` 直接绑定成字符串 `"all"`。
由于 Mapping 参数会覆盖模型参数，Runtime 的第二次尝试仍得到完全相同的无效调用，
最终触发防死循环错误：

```text
repeated identical failed tool call; stopping step
```

数据库中没有 `assistant_tool_calls` 记录，证明 Handler、审批和业务任务均未执行。
截图中的绿色卡片是另一个独立问题：前端把失败步骤的 `result_summary` 当成成功步骤
结果渲染，丢失了 `failed` 状态并固定使用成功图标和绿色样式。

## 2. 根因

### 2.1 Mapping 只形成能力集合，没有形成业务执行图

`ToolNamesForCapabilities` 将 capability 放入集合后遍历工具注册表，丢失意图中的顺序。
`orderDeterministicWorkflowTools` 目前只处理基线工作流，资产工作流没有后端确定性顺序。

依赖模型输出顺序同样不可靠。工具执行顺序必须由版本化业务工作流定义，而不是由模型、
工具名称排序或注册顺序决定。

### 2.2 通用参数绑定器把业务范围误当成实体 ID

`host_ids` 的绑定实体为 `host`。当意图中没有 UUID、只有 `scope.kind=all` 时，
通用绑定器直接返回字符串 `"all"`，没有区分：

- 业务范围：`all`、`all_online_hosts`；
- 精确实体：一个 UUID；
- 实体集合：UUID 数组。

Mapping 生成计划后也没有立即使用目标工具 Schema 校验编译结果，因此错误一直延迟到
Runtime 执行阶段。

### 2.3 资产采集异步契约不完整

`Asset.Collection.Trigger` 创建后台采集任务并返回 `task_id`，但目前被建模为同步终态
工具，没有声明 `Asset.Collection.Get` 为 completion capability。即使修复当前参数，
Runtime 也可能把“任务创建成功”错误当成“资产采集完成”。

### 2.4 UI 将失败摘要统一渲染成成功卡

`AssistantConversation.vue` 的 `StepResult` 不包含状态，`getStepResults` 只检查
`result_summary` 是否非空，模板固定使用 `CircleCheck` 和绿色样式。最终回答又先于
剩余步骤结果渲染，形成“先说失败、后显示绿色成功卡”的矛盾界面。

## 3. 修复目标

1. “重新采集所有主机资产”必须编译为合法的全主机业务范围，不能生成
   `host_ids="all"`。
2. 工具顺序由后端版本化工作流编译器决定，不依赖模型顺序、工具名称或注册顺序。
3. 所有具体工具仍必须先经过 capability Mapping 和授权 hard gates；工作流编译器
   不得引入未映射工具。
4. Mapping 计划在交给 Runtime 前必须通过完整参数 Schema 和业务前置条件校验。
5. 资产采集任务创建后必须使用真实 `task_id` 监控到终态，不能把 accepted/collecting
   当成 completed。
6. 同一个 Runtime 业务步骤只暴露 Mapping 已授权的 primary tool 和 registered
   completion tool。
7. 失败步骤必须使用失败图标、失败颜色和本地化错误，且展示顺序不得晚于最终总结。
8. `full_access` 继续免交互审批，但不绕过 Mapping、Schema、范围和业务校验。

## 4. 不在本次范围

- 不修改 Agent、Server、gRPC 或 Kafka 协议。
- 不改变资产采集领域服务的实际采集内容。
- 不允许 Runtime 恢复自由 `tool_name` 选举。
- 不通过增加无条件重试掩盖确定性计划错误。
- 不为历史失败会话补造工具调用或任务记录。

## 5. 总体设计

目标链路：

```text
User request
  -> LLM intent decomposition (workflow_id + capabilities + business scope)
  -> exact capability Mapping
  -> authorization hard gates
  -> immutable accepted Mapping set
  -> WorkflowPlanCompilerRegistry
       select compiler by workflow_id
       derive deterministic DAG
       bind typed arguments
       prune unnecessary mapped discovery steps
       attach only registered mapped completion tools
  -> CompiledPlanValidator
       tool still mapped
       args satisfy schema
       dependencies resolvable
       async completion contract closed
  -> agent-runtime InitialPlan
  -> ToolGateway prepares and revalidates
  -> primary operation
  -> mapped completion polling
  -> terminal evidence
  -> truthful summary and status-aware UI
```

必须保持以下代码级不变量：

```text
compiled_tool ∈ accepted_mapping_tools
runtime_tool ∈ current_compiled_step.allowed_tools
current_compiled_step.allowed_tools
  == {mapped_primary_tool}
     ∪ {registered_completion_tool if independently mapped}
compiled_args satisfies registered_tool.args_schema
non_terminal_primary requires mapped completion contract
```

## 6. 后端修复设计

### 6.1 引入工作流计划编译器注册表

新增通用接口，替代继续扩展 `orderDeterministicWorkflowTools` 和
`applyBuiltinDeterministicPlanArgs` 中的工具名特判：

```go
type WorkflowPlanCompiler interface {
    WorkflowID() string
    Compile(input WorkflowCompileInput) (*ToolExecutionPlan, error)
}

type WorkflowCompileInput struct {
    Breakdown    *IntentBreakdown
    AcceptedPlan *ToolExecutionPlan
    Registry     *ToolRegistry
}
```

`WorkflowPlanCompilerRegistry` 根据 `IntentBreakdown.WorkflowIDs` 选择唯一编译器。
V6.1 首先注册：

- `asset_inventory`
- 已有 `baseline_compliance` 逻辑后续迁入同一接口

编译器只能对已经通过 Mapping 的工具执行：

- 排序；
- 按条件删除不必要步骤；
- 绑定后端确定参数；
- 声明依赖；
- 将已映射 completion tool 合并到 primary runtime step。

编译器不得按工具名猜测新能力，也不得将注册表中未被本轮 Mapping 接受的工具加入计划。

### 6.2 资产重采集编译规则

#### 场景 A：所有主机

输入：

```json
{
  "workflow_ids": ["asset_inventory"],
  "scope": {"kind": "all"},
  "actions": ["execute"]
}
```

编译结果：

```json
{
  "tool_name": "Asset.Collection.Trigger",
  "capability": "trigger_asset_collection",
  "args": {
    "scope": "all_hosts",
    "force": true
  }
}
```

此场景不需要先执行 `Host.List` 或 `Host.Resolve`。资产领域服务已经拥有
`scope=all_hosts` 的后端语义，重复列举和重新拼装 UUID 只会增加分页、时序和覆盖风险。

#### 场景 B：用户提供精确主机 UUID

编译结果：

```json
{
  "scope": "hosts",
  "host_ids": ["uuid-1", "uuid-2"],
  "force": true
}
```

`host_ids` 必须始终是去重后的 UUID 数组。单个 UUID 也必须包装为数组。

#### 场景 C：用户提供主机名、IP 或动态选择器

编译为两个业务步骤：

```text
Resolve target hosts
  -> Refresh and verify asset inventory
```

第一步仅允许 `Host.Resolve`。第二步的 `host_ids` 从第一步真实 outcome facts 中聚合，
必须覆盖任何模型值：

```text
host_resolved facts[].id -> []UUID -> Asset.Collection.Trigger.host_ids
```

没有解析结果时第二步跳过并报告覆盖缺口，不能回退到 `scope=all_hosts`。

#### 场景 D：范围含糊

写操作没有明确 `all`、UUID 或可解析选择器时，编译器返回 clarification，不生成 Runtime
工具描述符。不得用空数组、默认全量或模型猜测扩大作用范围。

### 6.3 类型化参数绑定

`ArgBindingRule` 增加或派生以下约束：

```go
type ArgValueKind string

const (
    ArgValueBusinessScope ArgValueKind = "business_scope"
    ArgValueEntityID      ArgValueKind = "entity_id"
    ArgValueEntityIDs     ArgValueKind = "entity_ids"
    ArgValuePreviousFacts ArgValueKind = "previous_facts"
)
```

绑定规则：

- `scope.kind=all` 只能绑定到业务范围参数，不能绑定到 `*_id` 或 `*_ids`。
- `*_id` 只接受一个满足格式约束的 ID。
- `*_ids` 只接受数组；单个真实 ID可安全包装为数组，业务范围字符串不可包装。
- `previous_facts` 必须从真实工具 outcome 聚合，不接受模型输入。
- 类型不兼容时返回编译错误，不做字符串化或静默丢弃。

### 6.4 Runtime 前计划校验

新增 `CompiledPlanValidator`，在 `RuntimeFactory.Build` 之前执行：

1. 每个步骤的 tool/capability 存在于同一 `DecisionTraceID` 的 accepted Mapping 记录。
2. `step_id` 唯一，依赖存在且无环。
3. 编译参数经过 `normalizeRuntimeArgsSchema` 后调用 `ValidateToolArgs`。
4. 必填 `previous_step` 参数有可达生产者。
5. 异步 primary 声明的 completion capability 已被 Mapping 接受且 descriptor 可用。
6. write step 的范围、显式写意图和 approval snapshot 仍满足 hard gates。

失败行为：

```json
{
  "code": "compiled_plan_invalid",
  "stage": "mapping_compile",
  "workflow_id": "asset_inventory",
  "step_id": "authorized_01",
  "tool_name": "Asset.Collection.Trigger",
  "field": "host_ids",
  "reason": "business scope cannot bind to entity ID array"
}
```

计划编译失败不得进入 ReAct，因此不会再产生重复相同工具调用。用户看到的是一次明确的
范围或平台契约错误，而不是 Runtime 防死循环内部错误。

### 6.5 补全资产采集异步契约

`Asset.Collection.Trigger` 调整为：

```go
ExecutionContract: ToolExecutionContract{
    Mode:                 ToolExecutionAsynchronous,
    CompletionCapability: "get_asset_collection_task",
}
ResultContract: ToolResultContract{
    AcceptedOnSuccess:  true,
    OperationRefFields: []string{"task_id"},
}
```

`Asset.Collection.Get` 返回稳定的顶层字段：

```json
{
  "task_id": "uuid",
  "status": "collecting|analyzing|completed|failed|cancelled",
  "progress": {
    "total_hosts": 10,
    "success_hosts": 8,
    "failed_hosts": 2,
    "current_stage": "application_analysis"
  },
  "hosts": []
}
```

并声明：

```go
ResultContract: ToolResultContract{
    OperationStatusField:  "status",
    PendingValues:         []string{"pending", "collecting", "analyzing"},
    SuccessValues:         []string{"completed"},
    FailureValues:         []string{"failed", "cancelled"},
    OperationRefFields:    []string{"task_id"},
    SatisfiesCapabilities: []string{"trigger_asset_collection"},
}
```

Runtime 将 `Asset.Collection.Trigger + Asset.Collection.Get` 合并为同一个业务步骤的固定
允许集合。Trigger 返回的真实 `task_id` 由 ToolGateway 注入 Get，模型不得生成或修改。
Get 是只读、幂等工具，第一次选择后由 Runtime 使用既有退避策略自动轮询到终态。

固定计划的工具调用预算必须与异步轮询预算保持以下不变量：

```text
MaxToolCallsPerStep
  >= primary call + initial completion lookup + MaxAsyncPollAttempts
```

固定计划使用 24 次有界自动轮询；按 2 秒起始、30 秒封顶的指数退避，可提供约 10 分钟
的监控窗口。总工具调用预算必须覆盖所有固定步骤的单步预算并保留收尾余量。自动轮询达到
上限或任务总超时后仍然失败退出，不能无限等待；但通用工具调用上限不得在异步上限之前
抢先终止监控。

### 6.6 终态与覆盖判定

只有以下条件同时满足才能报告资产重采集完成：

- `Asset.Collection.Get.status == completed`；
- 存在真实 `task_id`；
- `total_hosts` 与本次解析范围一致，或明确说明领域服务排除的离线/无权限主机；
- 每台目标主机具有 success/failed/skipped 中的一种终态；
- 没有未解释的 pending/collecting/analyzing 主机。

部分失败必须报告：

```text
目标总数 / 成功数 / 失败数 / 跳过数 / 未覆盖对象及原因
```

任务创建、accepted、collecting 和 analyzing 均不能满足步骤完成条件。

### 6.7 重试与错误传播

- `mapping_compile`、`arguments`、`step_tool_scope` 属于确定性错误，不自动重复相同调用。
- 网络超时、临时依赖失败可按工具幂等性和现有预算重试。
- Runtime 的 `repeated identical failed tool call` 只作为内部保护错误；对外错误必须保留
  首次失败的 `validation_stage`、错误码和安全摘要。
- pre-gateway 候选失败继续不创建 durable tool call，但必须写结构化诊断日志。

## 7. 前端修复设计

### 7.1 保留步骤状态

`StepResult` 调整为：

```ts
type StepResult = {
  key: string
  title: string
  summary: string
  status: 'completed' | 'failed' | 'skipped' | 'retrying'
}
```

`getStepResults` 必须从 `plan.steps[].status` 保留状态，不能只复制标题和摘要。

### 7.2 状态化图标与样式

| 状态 | 图标 | 颜色 | 文案 |
| --- | --- | --- | --- |
| completed | CircleCheck | green | 完成 |
| failed | CircleClose | red | 失败 |
| skipped | RemoveFilled | gray | 已跳过 |
| retrying | Refresh | amber | 正在重试 |

禁止失败步骤使用 `.step-result-card` 的固定绿色样式。

### 7.3 展示顺序

最终消息中的顺序应为：

```text
thinking/tool observations
  -> step completed/failed/skipped cards
  -> final answer
  -> optional domain result cards
```

失败步骤卡不得出现在“任务未完成”的最终回答之后。历史会话重建也使用相同顺序。

### 7.4 错误本地化

面向用户显示：

```text
资产采集未启动：后端编译的主机范围参数不符合工具契约。
```

开发诊断可保留：

```text
code=compiled_plan_invalid
stage=mapping_compile
tool=Asset.Collection.Trigger
field=host_ids
```

不直接把 `repeated identical failed tool call; stopping step` 作为绿色业务结果展示。

## 8. 影响文件

### api-server

- `internal/assistant/workflow_registry.go`
  - 注册 `WorkflowPlanCompiler` 和资产工作流编译器。
- `internal/assistant/tool_decision_engine.go`
  - accepted Mapping 后调用工作流编译器；移除资产流程对注册顺序的依赖。
- `internal/assistant/tool_capability_mapping.go`
  - Mapping 只负责 capability 授权，不再隐含业务顺序。
- `internal/assistant/compiled_plan_validator.go`（新增）
  - Runtime 前计划、参数、依赖和 completion 闭环校验。
- `internal/assistant/adapter_tool_gateway.go`
  - 支持从 `host_resolved` facts 聚合 `host_ids`；保持后端值覆盖模型值。
- `internal/assistant/tools/asset_tools.go`
  - 补全 Trigger/Get 异步结果契约和稳定顶层状态字段。
- `internal/assistant/runtime_evidence.go`
  - 记录资产任务引用、覆盖数和终态。

### frontend

- `src/views/assistant/components/AssistantConversation.vue`
  - 步骤结果保留状态、状态化样式并调整展示顺序。
- `src/store/assistant.ts`
  - 历史与实时 `step_failed/skipped/completed` 使用同一状态归并逻辑。
- 相关 i18n 资源
  - 增加编译失败、跳过和部分完成的用户文案。

### 文档

- 更新 V6.1 工作流知识文档中的资产采集流程：

```text
范围编译 -> 触发采集 -> 监控真实 task_id -> 覆盖率验真
```

## 9. 测试设计

### 9.1 后端单元测试

| 用例 | 预期 |
| --- | --- |
| `scope.kind=all` | 编译为 `scope=all_hosts, force=true`，不存在 `host_ids="all"` |
| 单个 UUID | 编译为 `scope=hosts, host_ids=[uuid]` |
| 多个 UUID | 去重并保持 UUID 数组 |
| 主机名/IP 选择器 | `Host.Resolve` 在 Trigger 前，Trigger 从 facts 获得 UUID 数组 |
| 选择器无匹配 | Trigger skipped，不扩大到所有主机 |
| `host_ids` 为字符串 | `CompiledPlanValidator` 在 Runtime 前拒绝 |
| Mapping 未授权 completion | 编译失败，不能偷偷加入 `Asset.Collection.Get` |
| Mapping 已授权 completion | Trigger/Get 合并到一个 runtime step |
| Trigger 返回 task_id | Get 使用后端真实 task_id，覆盖模型值 |
| Get 返回 collecting | 步骤保持非终态 |
| Get 返回 completed | 步骤完成并记录覆盖证据 |
| Get 返回 failed/cancelled | 步骤失败，不报告完成 |
| 固定计划异步轮询 | 单步与总调用预算能覆盖 primary、首次 Get 和全部自动轮询 |
| 已创建资产任务但监控超时 | 摘要展示 task_id 和“后台仍在运行”，不显示“未下发任务” |
| full_access | 不创建 approval，仍执行全部计划校验 |

必须加入生产会话形状回归：

```json
{
  "goal": "重新采集所有主机上的资产",
  "scope": {"kind": "all"},
  "candidate_capabilities": [
    "resolve_hosts",
    "list_hosts",
    "trigger_asset_collection"
  ],
  "workflow_ids": ["asset_inventory"]
}
```

回归断言：

- 不按工具注册顺序生成计划；
- 不产生 `host_ids="all"`；
- 不调用不必要的 Host.List/Host.Resolve；
- 不出现 repeated identical failed tool call；
- 产生真实 task_id 并轮询到终态。

### 9.2 前端测试

| 用例 | 预期 |
| --- | --- |
| failed step + result_summary | 红色失败卡和失败图标 |
| completed step | 绿色成功卡 |
| skipped step | 灰色跳过卡 |
| failed step + final answer | 失败卡先于最终回答 |
| 历史会话恢复 | 状态、顺序与实时流一致 |
| 内部英文错误 | 显示本地化用户文案，诊断字段仍可检查 |

### 9.3 集成验证

1. 重建并启动 `api-server` 与 `frontend`。
2. 在 `full_access` 会话发送“重新采集所有主机上的资产”。
3. 验证无 approval 记录。
4. 验证首个 durable tool call 参数为：

   ```json
   {"scope":"all_hosts","force":true}
   ```

5. 验证创建非空资产采集 `task_id`。
6. 验证 `Asset.Collection.Get` 使用同一 task_id 并轮询到真实终态。
7. 验证计划栏为业务标题，不暴露内部工具名。
8. 验证失败场景使用红色卡片，最终回答不声称成功。

## 10. 日志与可观测性

新增或调整结构化日志：

### INFO

- `assistant workflow plan compiled`
  - `session_id`
  - `run_id`
  - `workflow_id`
  - `decision_trace_id`
  - `mapped_tool_count`
  - `compiled_step_count`
- `assistant asset collection reached terminal state`
  - `task_id`
  - `status`
  - `total_hosts`
  - `success_hosts`
  - `failed_hosts`
- `assistant runtime config selected`
  - `session_id`
  - `run_id`
  - `max_tool_calls`
  - `max_tool_calls_per_step`
  - `max_async_poll_attempts`

### WARN

- `assistant compiled plan rejected`
  - `workflow_id`
  - `step_id`
  - `tool_name`
  - `validation_stage`
  - `field`
  - `error_code`

不记录完整模型 Prompt、用户敏感输入、凭证或未脱敏工具参数。UUID 可按现有审计策略记录，
但不在普通 INFO 中输出完整主机列表。

## 11. API、数据库与协议影响

- 不修改公网 REST API。
- 不修改 gRPC/proto。
- 不需要数据库迁移。
- `assistant_tool_calls` 继续只记录真正进入 durable dispatch 的调用。
- 现有会话 metadata JSON 字段保持兼容，可选增加 `workflow_compile_error`。
- 历史失败会话不回写，只由新版前端按已有 `step.status` 正确显示。

## 12. 安全影响

- 不扩大模型工具权限。
- 工作流编译器只能使用同一 Mapping accepted set，保持 fail closed。
- “所有主机”属于显式用户范围，只有明确写意图时才可编译为 `all_hosts`。
- 动态选择器解析失败不能退化为全量范围。
- `full_access` 仅跳过交互审批，不跳过 RBAC、范围、Mapping、Schema、在线状态和业务前置条件。

## 13. 兼容性、灰度与回滚

### 兼容性

- 非 `asset_inventory` 工作流保持现有路径。
- 资产查询类请求继续使用只读工具，不触发资产重采集编译器。
- 精确 UUID 调用和页面上下文调用保持支持。

### 灰度

可增加短期配置：

```text
ASSISTANT_ASSET_WORKFLOW_COMPILER_ENABLED=true
```

灰度期间记录旧计划与新计划的差异，但只有新编译计划进入 Runtime。完成回归验证后，
配置默认开启并删除旧的资产注册顺序路径，避免长期双轨。

### 回滚

- 回滚 api-server/frontend 镜像即可恢复旧行为。
- 无数据库和协议回滚。
- 已创建的资产采集任务继续由领域服务运行，不因助手版本回滚而删除。

## 14. 实施顺序

1. 先增加生产会话形状的失败回归测试。
2. 实现 `WorkflowPlanCompilerRegistry` 和 `asset_inventory` 编译器。
3. 实现类型化参数绑定与 `CompiledPlanValidator`。
4. 补全 Asset Trigger/Get 异步契约和真实 task_id 绑定。
5. 补全资产终态证据和覆盖率判定。
6. 修复前端步骤卡状态和展示顺序。
7. 增加结构化日志与错误本地化。
8. 运行 Assistant、资产服务、前端定向测试及 api-server/frontend 构建。
9. 重建最小服务集合，执行一次非破坏性测试范围和一次受控真实采集验证。

## 15. 验收标准

- 最新失败会话的输入形状不再生成 `host_ids="all"`。
- 所有主机重采集计划不再包含无意义的后置 Host.List/Host.Resolve。
- 具体工具全部来自 capability Mapping，Runtime 没有自由工具选举路径。
- 编译错误在 Runtime 前一次性失败，不出现重复相同失败调用。
- 资产采集使用真实 task_id 监控到 completed/failed/cancelled。
- full_access 下无交互审批，其他安全校验全部保留。
- 失败步骤显示红色失败卡，成功步骤才显示绿色成功卡。
- 最终回答位于步骤结果之后，并与真实终态、覆盖率和任务证据一致。
- 后端和前端定向测试、构建以及健康检查全部通过。
