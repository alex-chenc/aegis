# 纯智能体工具映射、真实完成状态与证据闭环修复方案

> **工具选举章节已被 2026-07-24 方案替代**：本文关于
> `InitialPlan=nil`、agent-runtime 动态选举工具和“不生成固定执行计划”的内容不再
> 适用。所有 Assistant 工具必须由 capability Mapping 生成不可变执行计划，Runtime
> 不得增加、替换、重排或自由选择 `tool_name`。本文关于异步状态、步骤完成和证据
> 真实性的设计仍然有效。

## 1. 文档状态与适用范围

- 状态：已实现，待真实受控会话验收
- 日期：2026-07-10
- 影响仓库：`/code/agent-runtime`、Aegis `api-server/`、`frontend/` 和数据库迁移
- 不涉及：公网业务 API、gRPC/Agent 协议、漏洞和主机领域数据模型

本文修正
`assistant_generic_agent_flow_only.md` 中工具授权和完成状态尚未落地完整的部分。
如果两份文档存在冲突，以本文关于 capability mapping、工具授权、异步状态、
步骤完成和证据判定的定义为准。

修复必须保持通用智能体模式。CVE 会话仅作为回归样例，禁止为 CVE、基线、资产、
告警或其他单一场景增加固定计划、关键词路由、工具名分支或业务流程硬编码。

## 2. 已确认的问题

最新会话 `asst_e4f87ad5`、运行 `run_f1802fa6` 暴露了以下相互关联的缺陷：

1. IntentDecomposer 只校验 capability 是英文标识符，没有校验它是否来自实时目录，
   因而接受了 `generate_poc`、`deploy_check` 等不存在的自由标识。
2. ToolDecisionEngine 同时使用精确 mapping、领域召回、评分阈值和 LLM 预选绕过。
   正确的状态及下发工具被低分拒绝，错误的 `Task.RunCheck` 因被预选而绕过阈值。
3. Orchestrator 只把最终授权工具注入 agent-runtime。模型随后调用未注入的
   `Vulnerability.Script.Status` 时只能得到 `tool not found`。
4. `Vulnerability.Script.Generate` 的成功只表示后台生成已排队，但工具调用状态、
   步骤状态和最终回答把它表达成“脚本已生成”。
5. agent-runtime 在同一步发生工具错误后，仍会无条件接受模型的 `step_result`
   并发送 `step_completed`。
6. Aegis 编排器虽然保存了目标结果，外层 Service 仍无条件把会话状态和第二个 done
   事件写成 `completed`，覆盖了失败、部分成功和待补充信息的真实语义。
7. 最终证据账本按工具名和特定字段硬编码识别事实。`Host.List(status=all)` 中单个
   主机的在线状态、Generate 的异步状态等没有被正确归一化，事实守卫没有生效。
8. `ToolResultVerifier` 只存在于测试和独立实现中，没有接入生产执行和步骤完成链路。
9. agent-runtime 把重复的展示标题当作结构错误；同一工具以不同参数出现多次时会因
   `duplicate step title` 直接进入 `plan_failed`，尽管 step ID 本身唯一。

该会话的真实结果是：POC 在最终回答前生成完成，修复脚本在最终回答后才生成完成；
在线主机存在；下发工具没有执行；数据库中没有创建任何任务记录。

## 3. 修复目标与非目标

### 3.1 修复目标

1. 用户请求的业务理解、任务拆解、执行顺序、条件分支、轮询、重试和替代方案均由
   大模型与 agent-runtime 动态决定。
2. 大模型只能输出实时目录中存在的英文 capability，不得发明 capability 或工具名。
3. capability 到工具使用确定性 mapping；移除工具相关度评分、阈值和 LLM 预选绕过。
4. 工具授权只保留注册、启用、RBAC/租户、显式写意图、审批、目标范围和风险等硬门。
5. Runtime 获得完成目标所需的映射工具及只读配套工具，但 Aegis 不生成固定业务计划。
6. 工具调用成功、业务受理、业务运行中、业务终态成功和业务终态失败具有不同状态。
7. 模型的 `step_result` 只是完成提议；只有确定性完成校验通过后才发送
   `step_completed`。
8. 整轮运行同时保存生命周期状态和目标结果状态。UI 不再把“Runtime 已结束”显示为
   “业务目标已成功”。
9. 最终回答中的成功结论必须引用真实、终态、可验证的工具证据。
10. 所有发送给模型的静态提示词、模型侧工具描述、capability、参数说明和纠错信息均
    使用英文；最终用户回答继续跟随用户语言。

### 3.2 非目标

- 不建立 CVE 固定执行步骤。
- 不让 ToolGateway 自动补跑状态、下发或修复工具。
- 不通过吞掉工具错误或修改文案掩盖失败。
- 不要求所有工具一次性重写业务返回结构。
- 不改变现有写操作审批、权限和租户隔离边界。

## 4. 目标架构

```text
User Request
  -> LLM IntentRouter
  -> LLM IntentDecomposer
       input: exact English capability catalog
       output: intent + required/supporting capability IDs
  -> Capability Contract Validator
       syntax + exact catalog membership + one correction
  -> Capability Mapper
       exact capability -> enabled tool descriptors
       + declared read-only completion/discovery companions
  -> ToolAuthorizationEngine
       hard gates only; no semantic score and no preliminary bypass
  -> agent-runtime dynamic Planner/ReAct
       InitialPlan = nil
  -> ToolGateway
       hard authorization + schema + execution
       raw result -> normalized ToolOutcome
  -> StepCompletionValidator
       deterministic terminal evidence check
  -> GoalOutcomeEvaluator
       succeeded / partially_succeeded / failed / needs_input
  -> Evidence-grounded final answer
```

Aegis 决定“哪些工具允许进入本轮能力边界”，agent-runtime 决定“何时、以什么参数、
按什么顺序调用”。Aegis 不根据工具名称生成执行步骤。

## 5. 详细设计

### 5.1 单一 capability 选择与精确 mapping

IntentDecomposer 接收精简的实时 capability 目录，每项至少包含：

```json
{
  "capability": "execute_vulnerability_host_scripts",
  "domain": "vulnerability",
  "operation": "execute",
  "object_types": ["vulnerability", "host"],
  "risk": "high",
  "execution_mode": "synchronous"
}
```

IntentBreakdown 增加：

```json
{
  "required_capabilities": [],
  "supporting_capabilities": [],
  "candidate_capabilities": []
}
```

- `required_capabilities`：用户目标必须成功满足的能力。
- `supporting_capabilities`：查询、状态、验证等可能需要但不等同于最终目标的能力。
- `candidate_capabilities`：前两者的兼容并集，过渡期保留。

服务端必须执行以下确定性校验：

1. capability 满足小写英文标识格式。
2. capability 必须精确存在于本轮实时目录。
3. required/supporting 不得包含被禁用或当前用户不可见的能力。
4. 首次非法输出携带“允许值列表”要求模型纠正一次。
5. 纠正后仍非法则终止拆解，返回 capability contract error；不得进入模糊搜索、
   领域召回或评分降级。

生产链路删除独立 `LLM ToolSelector`。IntentDecomposer 直接从实时目录选择 exact
capability，最终工具集只能由 Capability Mapper 产生，不能直接接受 LLM 输出的
工具名。

所有内置可执行 ToolSpec 已显式声明唯一英文 capability。`syntheticToolCapability`
只保留给旧测试夹具和兼容性调用，不进入内置生产工具目录。重复或非法 capability
注册失败，并输出不含敏感数据的结构化错误。

### 5.2 配套能力与动态规划

纯动态规划不能依赖模型在首次拆解时准确猜出全部异步状态工具。ToolSpec 增加通用
执行契约：

```go
type ToolExecutionContract struct {
    Mode                  string   // synchronous | asynchronous
    CompletionCapability  string   // optional read-only terminal-status capability
    DiscoveryCapabilities []string // optional read-only argument discovery capabilities
}
```

映射器可以把这些“只读配套能力”加入 Runtime 可用工具集合，但不得创建步骤、绑定
业务参数或决定调用顺序。约束如下：

- 只允许自动暴露 readonly/low 风险配套工具。
- 写能力必须出现在用户明确要求的 `required_capabilities` 中，不能通过
  `CompletionCapability`、`NextCapabilities` 或 `WorkflowHints` 自动授权。
- 配套 capability 也必须经过精确 mapping、注册、启用、RBAC 和租户硬门。
- Runtime 根据实际返回值决定是否调用、轮询、重试或跳过配套工具。
- 配套能力按声明式契约做有环保护的传递展开。例如脚本生成可暴露漏洞目录查询；
  目录为空时 Runtime 可选择低风险自定义 CVE 查询，后者再暴露只读状态工具。这里只
  扩展可用工具边界，不固定调用顺序，也不会在目录非空时自动触发补录。
- 工具目录升级时，未被管理员修改（`updated_by` 为空）的内置策略会同步新的
  `default_whitelisted` 到实际 `whitelisted`；已有管理员覆盖保持不变，避免旧数据库
  继续沿用已废弃的默认值。

现有语义不清晰的 `NextCapabilities` 和 `WorkflowHints` 不再参与授权。可迁移为上面的
明确字段；无法表达为“完成查询”或“参数发现”的业务流程提示应删除。

### 5.3 ToolAuthorizationEngine 只执行安全硬门

ToolDecisionEngine 重命名或收敛为 ToolAuthorizationEngine。授权候选只来自：

1. exact capability mapping；
2. 合法的只读配套 capability；
3. 用户明确指定且能反向解析到唯一 capability 的工具。

必须删除：

- `MinScore`、`ReadonlyMinScore` 及对应环境变量；
- `scoreToolDecision`；
- domain/object/context 相关度召回作为授权来源；
- `preliminarySelected` 绕过；
- 旧 ToolSelector 的评分结果进入生产授权链路。

保留并增强以下硬门：

- capability 已映射；
- 工具已注册且启用；
- 用户和租户有权看到、调用目标工具；
- 写操作有原始用户消息中的明确写意图；
- 高风险操作获得审批；
- 页面/会话对象未越出租户和授权范围；
- 工具参数通过 schema；
- 幂等键和重复执行策略满足工具契约。

授权记录不再包含 Score；运行日志使用
`authorization_mode=mapping_hard_gates` 标识唯一授权模式。

### 5.4 模型侧英文工具契约

ToolSpec 增加 `ModelDescription`，与面向 UI 的本地化 `Description` 分离。

- `ModelDescription`、模型侧参数说明、结果状态、枚举和失败提示必须为英文。
- Orchestrator 构建 ToolDescriptor 时使用 `ModelDescription` 和英文契约，不再直接
  使用可能为中文的 `Description`。
- 工具 schema 中面向模型的 description 应使用英文；中文说明只在 UI/文档层使用。
- api-server 和 agent-runtime 的所有静态 prompt 继续使用英文。
- 用户消息、上下文和真实工具结果保持原始语言；最终回答跟随用户语言。

测试应扫描所有实际发送给模型的静态 prompt 和 descriptor，不能只测试少量 prompt
常量。

### 5.5 统一工具结果语义

传输调用成功不等于业务成功。所有工具进入 Runtime 前统一转换为：

```json
{
  "call_status": "success",
  "operation_status": "accepted",
  "terminal": false,
  "message": "Generation request accepted",
  "operation_ref": {
    "type": "vulnerability_script_generation",
    "id": "..."
  },
  "facts": [],
  "artifacts": [],
  "side_effects": []
}
```

字段语义：

| 字段 | 说明 |
| --- | --- |
| `call_status` | 网关/协议调用是否成功，仅为 `success/failed` |
| `operation_status` | `accepted/running/succeeded/failed/skipped` |
| `terminal` | 当前结果能否作为业务终态 |
| `operation_ref` | 关联异步触发、状态查询和最终结果 |
| `facts` | 查询得到的结构化事实 |
| `artifacts` | 生成的脚本、报告等引用，不保存敏感正文 |
| `side_effects` | 创建的任务组、阻断动作、配置变更等引用 |

ToolSpec 增加声明式 ResultContract，描述状态路径、终态条件和证据字段。通用
ResultNormalizer 根据契约转换旧工具返回，避免在 RuntimeEvidence 中继续按工具名
编写 switch。迁移期间：

- 同步只读工具调用成功可默认是 `terminal=true, operation_status=succeeded`。
- 异步触发工具必须显式返回 `accepted/running, terminal=false`。
- 创建/下发类工具只有取得契约要求的 side-effect ID 才能是终态成功。
- 状态查询只有命中 ResultContract 的成功终态值才可满足原异步操作。
- 业务失败即使 HTTP/handler 正常返回，也必须是
  `call_status=success, operation_status=failed, terminal=true`，不能当作成功。

现有 ToolResultVerifier 改为消费 ResultContract 和 ToolOutcome，删除
`task_id_created`、`script_generated` 等硬编码 switch，并接入生产链路。

### 5.6 agent-runtime 严格步骤完成

`ActionStepResult` 由“直接完成”改为“请求完成”。新增通用
StepCompletionValidator，至少检查：

1. 本步骤所有工具调用记录已经落入 TaskResult。
2. 当前步骤是否存在尚未恢复的工具调用失败。
3. 模型提供的 evidence 是否引用真实 call ID。
4. 所引用调用是否属于当前步骤。
5. 必需的 ToolOutcome 是否为 `terminal=true` 且
   `operation_status=succeeded/skipped`。
6. 异步 `accepted/running` 不能满足完成条件。
7. ToolSpec/计划声明的 postconditions 是否全部通过。

校验失败时：

- 不发送 `HookStepCompleted`；
- 向 ReAct 返回结构化 `completion_validation` observation；
- 在剩余额度内允许模型改参、查询状态、调用替代工具或明确 `fail_step`；
- 重试耗尽后标记 `StepFailed`，错误原因保留未满足的 postcondition 和 call ID；
- 不允许随后一个无证据 `step_result` 覆盖失败状态。

纯推理、解释和总结步骤没有工具后置条件时仍可正常完成。某个工具失败后使用合法
替代工具取得终态证据时，可以完成，但完成证据必须引用替代调用。

### 5.7 运行生命周期与目标结果分离

保留 agent-runtime `TaskStatus` 表示生命周期，同时新增：

```text
GoalOutcome:
  succeeded
  partially_succeeded
  failed
  needs_input
```

判定规则：

- 所有 required capability 均有终态成功证据：`succeeded`。
- 至少一个 required capability 成功、至少一个失败或未满足：
  `partially_succeeded`。
- required capability 全部失败、没有执行，或关键写目标没有 side-effect 证据：
  `failed`。
- 因缺少会改变目标或写操作边界的用户信息停止：`needs_input`。

`TaskStatus=completed` 只允许解释为 Runtime 生命周期结束。Aegis 持久化时分别保存：

```json
{
  "runtime_status": "completed",
  "goal_outcome": "failed",
  "current_run_status": "completed_with_failures"
}
```

事件和 UI 使用 `goal_outcome` 展示“成功、部分成功、失败、待补充信息”。只有
StepCompletionValidator 通过的步骤才显示“已完成”；工具调用失败的步骤必须显示失败
或重试中。

done 事件由 Service 在 Orchestrator 返回后统一发布一次。Service 根据 GoalOutcome
更新会话状态，不再发布无条件 `completed` 事件。

### 5.8 通用证据账本与最终回答

RuntimeEvidence 从 ToolOutcome 构建，不再依赖
`Vulnerability.Script.Status`、`Host.List` 等具体工具名。账本至少保存：

- capability、tool_name、call_id、step_id；
- call_status、operation_status、terminal；
- operation_ref；
- facts、artifact refs、side-effect refs；
- validation stage 和错误；
- 满足或未满足的 required capability/postcondition。

工具结果中的数组、分页和单项状态由各自 ResultContract 声明字段路径。例如主机工具
可以声明从每个 item 的在线字段产生 `host_online` fact，避免仅查看顶层过滤参数。
结果归一化先经过 JSON 形态转换，确保 handler 返回的强类型 slice 与数据库恢复的
`[]interface{}` 使用同一套字段绑定逻辑。

最终总结分两层：

1. 模型根据完整证据生成用户语言的自然回答和结构化 claims；
2. 确定性 ClaimValidator 检查每条成功 claim 引用的 call ID、终态和证据类型。

没有终态生成证据不得声称“已生成”；没有 side-effect/task-group 引用不得声称
“已创建、已下发、已修复”；只有 accepted/running 时必须明确表达“已受理/处理中”。
发现不一致时拒绝原总结，使用证据账本生成保守回退结论。

关键词冲突检测可以保留为补充保护，但不能继续作为主要事实判断机制。

### 5.9 持久化、日志和敏感信息

生产链路必须持久化 agent-runtime 的执行、步骤和工具调用记录，确保以下 ID 可以
串联：

```text
session_id -> run_id -> runtime_task_id -> step_id -> call_id -> operation_ref
```

结构化日志至少包含：

- `authorization_mode=mapping_hard_gates`
- `selected_capabilities`
- `mapped_tools`
- `companion_capabilities`
- `rejected_hard_gates`
- `call_status/operation_status/terminal`
- `completion_validation`
- `runtime_status/goal_outcome`

不得记录 API Key、Authorization、完整脚本内容、完整模型原始输出或不必要的主机
敏感字段。脚本和任务只记录 ID、类型、状态和必要关联键。

## 6. 代码影响范围

### 6.1 agent-runtime

重点文件：

- `core/types.go`：ToolOutcome、GoalOutcome、完成校验错误及兼容字段。
- `executor/react.go`：`step_result` 进入确定性完成校验。
- `runtime.go`：只对验证通过的步骤发送 completed hook，计算 GoalOutcome。
- `executor/`、`hook/`、`task/`：保存 outcome、错误和快照。
- prompt provider：告诉模型 accepted/running 不是终态，evidence 必须使用 call ID。

agent-runtime 保持领域无关，不包含 Aegis 工具名、CVE、基线、主机或告警规则。
计划校验只要求执行身份 `step_id` 唯一；展示标题允许重复，依赖关系只引用
`step_id`。

### 6.2 Aegis api-server

重点文件：

- `intent_decomposer.go`：注入实时 capability 目录，校验 exact membership。
- `llm_json.go`：为 IntentRouter 和 IntentDecomposer 提供统一英文 JSON 纠错。
- `tool_registry.go`：ModelDescription、ExecutionContract、ResultContract 注册校验。
- `tool_capability_mapping.go`：精确映射和只读配套能力解析。
- `tool_decision_engine.go`：移除评分、领域召回和预选绕过，只保留硬门。
- `orchestrator.go`：只从 mapping 构建 descriptor，持久化双状态。
- `adapter_tool_gateway.go`、`tool_outcome.go`：按声明式 ResultContract 统一 ToolOutcome、
  postcondition 和 operation_ref。
- `runtime_evidence.go`：基于 ToolOutcome 构建通用账本和 claims。
- `adapter_hook_sink.go`：只有验证成功才发布步骤完成事件。
- `tools/*`：补齐英文模型描述和声明式结果契约。

### 6.3 frontend

如果当前 UI 将 `runtime_status=completed` 直接显示为任务成功，应调整为：

- 步骤使用经过验证的 step status；
- 整轮使用 `goal_outcome`；
- 工具的 `accepted/running` 显示为“已受理/处理中”；
- 失败详情显示工具、阶段和简短原因，不展示敏感参数或脚本正文。

## 7. 实施顺序

### 阶段一：先建立失败测试

1. 在 agent-runtime 增加“工具失败后 step_result 不得完成”和“异步 accepted 不得完成”
   测试。
2. 在 Aegis 增加“未知 capability 被拒绝”“评分不影响 mapping”“错误预选不能绕过”
   测试。
3. 以最新 CVE 会话构造集成回归测试，但断言使用通用 capability、outcome 和 evidence，
   不断言固定步骤标题或固定调用次数。

### 阶段二：修改 agent-runtime

1. 增加向后兼容的 ToolOutcome 和 GoalOutcome 字段。
2. 实现 StepCompletionValidator。
3. 修正 hook、TaskResult 和最终总结输入。
4. 运行 `go test ./...`，提交并推送 agent-runtime。

### 阶段三：修改 Aegis

1. 改为 capability 目录输入和精确 membership 校验。
2. 删除生产授权链的评分、领域召回和预选绕过。
3. 增加 ToolSpec 执行/结果契约与英文模型描述。
4. 在 ToolGateway 归一化结果并接入 ToolResultVerifier。
5. 重写通用证据账本、目标结果和最终 claim 校验。
6. 更新 `api-server/go.mod/go.sum` 引用新的 agent-runtime 提交。

### 阶段四：UI、文档和真实回归

1. 核对前端步骤和整轮状态文案。
2. 更新纯智能体架构文档，删除评分和旧双选择描述。
3. 使用真实在线主机执行受控冒烟，验证实际任务组和数据库任务记录。

当前阶段一至阶段三已经完成；阶段四的真实写操作冒烟在服务重建、审批和受控主机
确认后执行。

## 8. 测试设计

### 8.1 capability 与授权

| 用例 | 期望 |
| --- | --- |
| 中文请求选择工具 | 输出实时目录中的英文 capability |
| 英文格式但目录不存在的 capability | 纠正一次；仍非法则契约失败 |
| capability 映射到正确工具 | 通过硬门后进入 Runtime |
| LLM 预选错误工具 | 不得进入授权集合 |
| 正确工具历史评分低 | 不影响授权，生产链路不存在评分 |
| 写 capability 无明确写意图 | 被硬门拒绝 |
| 只读 completion capability | 可作为配套工具暴露 |
| 写 next capability 未被用户要求 | 不得自动授权 |

### 8.2 工具结果与步骤状态

| 用例 | 期望 |
| --- | --- |
| handler 返回错误 | call failed，step 不得 completed |
| schema/descriptor/policy 前置失败 | 持久化真实 validation stage |
| 异步 Generate 返回 accepted | call success，但 operation 非终态 |
| Status 返回 running | step 继续或明确失败，不得成功总结 |
| Status 返回 succeeded | 关联 operation_ref 后满足终态 |
| 下发未返回 task/side-effect ID | postcondition 失败 |
| 失败后替代工具终态成功 | 引用替代 call ID 后允许完成 |
| 无工具的解释步骤 | 可正常完成 |
| 不同 step_id 使用相同展示标题 | 计划有效，不得 plan_failed |

### 8.3 目标结果与最终回答

| 用例 | 期望 |
| --- | --- |
| 所有 required capability 成功 | goal_outcome=succeeded |
| 生成成功但下发失败 | goal_outcome=partially_succeeded |
| 没有创建任何任务 | 不得声称“已下发” |
| accepted 但未终态 | 只能声称“已受理/处理中” |
| Host.List 顶层 all、单项 online | 证据账本正确记录在线主机 |
| 模型成功 claim 引用不存在 call ID | ClaimValidator 拒绝并回退 |
| 非漏洞领域的异步工具 | 使用同一套契约和完成规则通过 |

### 8.4 回归场景

针对 `CVE-2021-45340` 的请求，允许 Runtime 自由规划，但完成时必须具备：

- POC 和修复脚本各自的终态 generated 证据；
- 在线目标主机的真实 ID；
- 下发调用的终态成功结果；
- 非空任务组或等价 side-effect ID；
- `max_rounds=5` 和自动验证参数进入真实下发调用。

缺少其中任一用户要求的终态证据时，必须返回部分成功或失败，不得显示任务成功。

同时选取至少两个非漏洞任务进行回归，证明实现没有依赖 CVE 工具名或固定流程。

## 9. 验收标准

1. 生产日志显示 `authorization_mode=mapping_hard_gates`，不存在 score threshold 和
   preliminary bypass。
2. 发送给模型的 capability 均来自实时英文目录；未知 capability 无法进入 Runtime。
3. Runtime descriptor 包含完成目标所需的映射工具和合法只读配套工具。
4. 任意工具调用失败后，UI 和事件流不会出现该步骤“已完成”。
5. 异步触发只显示 accepted/running，查询到终态前不显示成功。
6. `current_run_status`、`runtime_status`、`goal_outcome` 语义明确且一致。
7. 最终回答的每个成功事实都能追溯到 call ID 和终态 ToolOutcome。
8. 没有 task/side-effect ID 时，系统无法生成“已下发成功”的最终结论。
9. agent-runtime 全量测试、Aegis Assistant 定向测试、api-server 构建和健康检查通过。
10. 真实回归中脚本生成并实际创建下发任务，数据库存在对应任务组和主机任务记录。

## 10. 发布与回滚

发布顺序：

1. 发布向后兼容的 agent-runtime 提交。
2. Aegis 更新依赖并完成定向测试和构建。
3. 重建、重启 api-server；如修改状态展示，再发布 frontend。
4. 先执行只读和低风险会话，再执行经过审批的真实下发冒烟。

过渡期可以使用显式开关进行灰度，但启动日志必须显示当前模式，且不能静默降级到
评分授权。完成灰度后 `mapping_hard_gates` 和 strict completion 应成为默认且唯一
生产模式。

本方案新增 `025_v6.1_assistant_tool_outcomes.sql`，将传输状态与业务状态分开持久化：

- `operation_status`
- `operation_terminal`
- `outcome`

若发布异常：

- 回滚 Aegis 提交和 agent-runtime 依赖版本；
- 恢复上一版 api-server/frontend 镜像；
- 不删除已创建的脚本和任务记录；
- 对灰度期间已经 accepted 但未完成的异步操作按 operation_ref 核查，不重复触发写操作。

## 11. 完成定义

只有代码、测试、运行日志、数据库任务证据和本文档一致时，修复才算完成。单元测试
通过但真实会话仍出现“失败工具对应步骤已完成”、异步受理被描述为成功或没有任务组
却声称已下发，均视为未完成。
