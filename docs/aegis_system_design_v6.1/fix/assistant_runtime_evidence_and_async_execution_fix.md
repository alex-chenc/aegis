# 纯智能体证据可见性与异步执行可靠性修复

## 1. 问题、范围和成功标准

最新受控会话 `asst_ebe8fda3` / `run_b6451bbe` 已证明 capability exact mapping、
工具授权和领域工具本身均可工作，但纯智能体执行仍在通用运行时层发生偏差：

1. ReAct 要求 `step_result.evidence` 引用终态工具调用的精确 `call_id`，当前步骤的
   observation 却没有向模型展示 `call_id`，工具失败时也没有展示 `error`。
2. 异步 `accepted/running` 每轮都消耗 `MaxStepReactTurns`，脚本实际完成前步骤先因
   推理轮数耗尽失败。
3. ReAct 只依靠提示词要求模型使用精确工具和参数；运行时没有把实时工具目录转换为
   结构化输出 Schema，模型会发明工具名或参数别名。
4. Aegis 的 LLM adapter 忽略 agent-runtime 提供的 `ResponseFormat`，即使运行时生成
   JSON Schema 也无法传递给支持结构化输出的模型。
5. 最终总结会把模型发明的未注册工具解释成“平台缺少能力”，而不是执行阶段违反
   当前目录契约。
6. 真实回归 `asst_e21d35f3` / `run_05ab4be8` 暴露了后续偏差：每次
   `Vulnerability.Script.Status=running` 都重新调用模型、创建新的 call ID 和工具卡；
   脚本终态后模型又在主机查询步骤发明 `Task.Create/Host.Task.Create`，并且
   agent-runtime 的默认高风险策略在 Aegis mapping 已授权、平台为 `full_access` 后
   仍二次拒绝 `Vulnerability.Script.Execute`。

本次修改 `/code/agent-runtime` 和 Aegis `api-server`，更新 agent-runtime 依赖引用。
不增加 CVE、主机、基线或其他领域专用流程，不恢复 caller-supplied 固定计划，不修改
公网 API、数据库和 gRPC/Agent 协议。

成功标准：

- `agent-runtime` 仍是唯一业务规划器，Aegis `InitialPlan=nil`。
- 当前和历史 observation 都向模型提供真实
  `call_id/tool_name/call_status/error/outcome/content`。
- 终态成功后模型能引用真实 call ID 完成步骤；失败或非终态不能伪装成完成。
- `accepted/running` 使用独立、有界、带退避的异步等待额度，不消耗步骤推理纠错额度。
- 模型选择只读、幂等状态工具后，重复轮询由 Runtime 接管：不重复调用模型，不创建
  重复逻辑工具记录，不向前端推送重复思考和工具卡。
- ReAct 请求携带由本轮实时工具描述符生成的 JSON Schema；工具名只能来自实时目录，
  工具参数使用对应 `ArgsSchema`。
- 动态 Planner 的每步 `suggested_tools` 会过滤为已注册工具并固化为该步骤
  `AllowedTools`，ReAct 不得跨步骤调用或发明工具。
- Runtime 只接收已经通过 Aegis capability mapping/hard gates 的工具子集，不再对
  高风险工具做冲突的二次 deny；最终审批仍由 Aegis
  `request_approval/whitelist/full_access` 策略负责。
- 不支持 JSON Schema 的模型提供方可回退到 JSON object/文本模式，但仍能看到完整
  目录、参数 Schema 和校验错误并纠正。
- 异步操作始终受 `TaskTimeout`、`MaxToolCalls` 和 `MaxToolCallsPerStep` 约束。
- 最终总结把 descriptor/arguments validation failure 表达为“本轮模型调用违反工具
  契约”，不能推断平台缺少已经由授权目录提供的能力。
- 真实漏洞会话最终产生脚本终态证据、在线主机证据以及非空下发任务记录。

## 2. 首个偏差和目标数据流

当前首个偏差位于 `agent-runtime/executor/react.go`：

```text
ToolResponse(call_id, status, error, outcome)
  -> Observation（字段完整）
  -> buildReactMessages（仅输出 content，丢失 call_id/error）
  -> LLM 无法构造合法 evidence，也无法根据校验错误纠参
```

目标数据流：

```text
Runtime dynamic tool registry
  -> ReAct JSON Schema (exact tool enum + per-tool args schema)
  -> LLM action
  -> descriptor / args / policy validation
  -> Observation envelope
       call_id + tool_name + call_status + error + outcome + content
  -> accepted/running
       model selects the exact read-only idempotent status tool once
       Runtime polls the same logical call with bounded exponential backoff
       duplicate model calls and duplicate visible events are suppressed
       does not consume reasoning-recovery budget
  -> terminal succeeded/skipped
       LLM cites call_id in step_result
  -> deterministic StepCompletionValidator
  -> verified step completion
```

## 3. agent-runtime 设计

### 3.1 统一 observation envelope

当前步骤的每条 observation 使用稳定英文格式发送给模型：

```text
Tool observation:
- tool_name: Host.List
- call_id: ...
- call_status: success
- operation_status: succeeded
- terminal: true
- error: ...
- outcome: {...}
- content: {...}
```

空字段不输出。错误优先使用 `Observation.Error`，其次使用 `Summary`。不得把大段内容
重复输出；继续沿用现有截断和上下文压缩。

### 3.2 异步等待预算

`RuntimeConfig` 增加：

- `AsyncPollInitialBackoff`
- `AsyncPollMaxBackoff`

当成功工具调用返回 `terminal=false` 且 operation status 为 `accepted/running`：

1. 首次异步写工具返回 accepted 后，由模型从实时目录中选择正确的完成状态工具；
2. 当状态工具为 `read_only + idempotent` 且返回 running，Runtime 固定其精确工具名
   和参数，后续轮询不再调用模型；
3. 内部轮询复用同一个逻辑 `call_id`，Aegis 更新原记录，不新增工具调用记录；
4. 非终态内部轮询不推送重复 thinking/tool_call/tool_result；终态结果更新原工具卡；
5. 实际状态查询仍计入 Runtime 工具调用硬上限，但不重复增加 Aegis 的逻辑工具调用数；
6. 该轮不消耗 `MaxStepReactTurns` 所代表的解析、纠错和完成推理预算；
7. 下一次状态查询按连续非终态次数指数退避；
8. 退避不超过 `AsyncPollMaxBackoff`；
9. 工具终态、失败、解析失败或其他动作会重置连续非终态计数；
10. `MaxToolCallsPerStep`、全局 `MaxToolCalls` 和 `TaskTimeout` 仍是硬上限。

Runtime 不根据 CVE、主机或任务名称写业务规则。是否可自动轮询只依赖工具 descriptor
中的通用属性 `RiskReadOnly + Idempotent` 以及标准 `ToolOutcome.Terminal`。

### 3.3 动态 ReAct JSON Schema

Runtime 从当前 registry 构建 `response_format=json_schema`：

- `action` 只允许 `tool_call/step_result/request_experience/fail_step`；
- `tool_call.tool_name` 为当前允许工具的精确枚举；
- 每个工具分支的 `args` 直接复用对应 `ToolDescriptor.ArgsSchema`；
- `step_result.evidence` 为字符串数组；
- 如果步骤有 `AllowedTools`，Schema 只包含该子集。

Schema 是提供方能力增强，不替代 Runtime 的 descriptor、arguments、policy 和 step scope
校验。模型提供方拒绝结构化格式时，调用方可以回退，但不能跳过执行边界校验。

### 3.4 风险元数据

动态 Planner 的 `risk_level` 不作为真实工具风险来源。执行时始终以 registry descriptor
为准。Planner 生成计划后可按 `suggested_tools` 的 descriptor 规范化步骤风险，避免
界面和审计显示高风险工具为只读。

### 3.5 动态步骤工具边界

Planner 仍由模型根据任意用户目标生成步骤。生成后 Runtime 对每步
`suggested_tools` 做一次通用注册表交集和去重，并写入 `AllowedTools`：

- ReAct JSON Schema 只暴露该步骤工具；
- 即使提供方不执行 JSON Schema，Runtime step-scope validator 仍会拒绝目录外名称；
- 需要不同工具时应通过计划纠正替换步骤，不允许在当前步骤盲猜工具名；
- 无计划的简单任务保持动态目录能力，不引入领域固定流程。

## 4. Aegis 设计

### 4.1 LLM adapter

`LLMClientAdapter` 完整转发 agent-runtime `ResponseFormat`：

- 支持 `json_schema` 时原样转换 Schema；
- 仅有 `ResponseSchema` 时继续使用兼容 JSON object 模式；
- 提供方拒绝 response format 时仅重试一次无格式请求；
- 日志只记录 purpose、format type 和错误类别，不记录用户消息或 Schema 全文。

### 4.2 模型侧参数契约

ReAct 提示词的工具目录输出无本地化描述的精简 JSON Schema，包括：

- 参数名；
- `type/items/enum/minimum/maximum`；
- required 列表。

工具 UI 中文描述不进入模型静态指令。用户原文和真实业务数据保持原语言。

### 4.3 总结归因

总结提示词明确区分：

- descriptor validation failure：模型提出了本轮目录外名称；
- arguments validation failure：模型参数未通过已注册工具 Schema；
- capability unavailable：实时授权目录中确实没有可满足能力。

如果目录中存在合法工具，前两类错误不得总结为“平台缺少或未部署能力”。

### 4.4 授权单一权威

Assistant 在创建 Runtime 前已经完成 capability mapping、工具启用状态、用户写意图、
必要实体和审批要求硬闸门，传入 Runtime 的 descriptor 是已授权子集。因此 Runtime
允许该子集进入 Aegis ToolDispatcher；ToolDispatcher 再按当前平台模式执行审批或
直接放行。该设计消除 Runtime `AllowHighRiskTools=false` 与 Aegis `full_access` 的
双重策略冲突，同时保留 Aegis 的最终安全控制。

## 5. 测试设计

### 5.1 agent-runtime

| 用例 | 期望 |
| --- | --- |
| 终态 observation | 下一轮 prompt 包含精确 call ID、状态和 outcome |
| 失败 observation | 下一轮 prompt 包含 validation error |
| step_result 引用 observation call ID | 完成校验通过 |
| 多次 running 后 succeeded | 非终态轮不耗尽 ReAct 推理预算，最终完成 |
| 只读幂等状态工具持续 running | 仅前后各一次模型决策，内部自动轮询并复用逻辑 call ID |
| 异步持续 running | 达到工具调用或任务超时上限后失败，不无限轮询 |
| ReAct ResponseFormat | tool name 为当前 registry 枚举，args 使用对应 Schema |
| 步骤 AllowedTools | ResponseFormat 不暴露步骤外工具 |
| Planner 风险元数据 | 高风险 suggested tool 不再显示 read_only |
| Planner 步骤工具边界 | 只保留注册表中的 suggested tools，ReAct 无法调用步骤外名称 |

### 5.2 Aegis

| 用例 | 期望 |
| --- | --- |
| adapter 收到 json_schema | 完整转发到 OpenAI-compatible 请求 |
| 提供方拒绝 json_schema | 回退一次且仍返回模型结果 |
| 工具目录参数 | 输出精确名称、类型、required、enum，不输出中文描述 |
| descriptor failure 总结 | 表达模型目录违约，不声称平台缺失已有能力 |
| accepted/running SSE | 显示已受理/执行中，不显示业务成功 |
| Runtime 内部状态轮询 | 数据库只有一条逻辑工具记录，前端不产生重复状态卡 |
| full_access 高风险工具 | 已 mapping 授权的工具可到达 Aegis ToolDispatcher，不被 Runtime 二次 deny |

### 5.3 真实受控回归

使用任意存在漏洞、在线主机和写意图的请求，Runtime 可自由规划。完成必须具备：

- 请求中要求的脚本均有终态 artifact 证据；
- 在线目标主机来自真实工具结果；
- 下发参数使用工具 Schema 中的精确字段；
- 下发返回非空 task group/side effect；
- 数据库存在本轮创建的主机任务记录；
- 最终回答引用上述真实结果，未执行部分不得声称成功。

再运行一个非漏洞异步任务或模拟测试，证明实现不依赖 CVE 工具名。

## 6. 日志、安全、兼容性与回滚

- Aegis 继续使用现有 tool outcome 和 validation-stage 结构化日志。
- response format 回退记录 WARN，字段包含 `purpose/format/error`，不记录 prompt、
  tool args、脚本内容或主机详情。
- observation 只进入当前模型上下文和现有受控证据存储；继续执行截断和压缩。
- 新增 RuntimeConfig 字段向后兼容；零值使用无额外等待的兼容行为，Aegis 显式设置
  生产退避值。
- 不支持 JSON Schema 的 provider 保持可运行，Runtime 服务端校验始终生效。
- 发布顺序：agent-runtime 提交并推送，Aegis 更新伪版本，再重建 api-server；前端
  无新增契约时无需重建。
- 回滚：恢复 Aegis agent-runtime 伪版本和本次 adapter/prompt 修改；不删除已生成
  脚本或任务数据。
