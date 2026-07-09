# 智能助手受约束工具执行与证据一致性修复

## 1. 问题、范围与成功标准

指定 CVE 的 POC、修复和在线主机下发任务已经能够由 LLM 识别为漏洞处置任务，但运行链路仍存在以下契约断点：

1. LLM 输出的范围值没有受枚举约束，`scope.kind=host` 无法触发 `Host.List`。
2. Aegis 先生成 `ToolExecutionPlan`，agent-runtime 随后再次生成执行计划，两个计划可能选择不同工具。
3. 同一个工具需要分别处理 `poc` 和 `fix` 时，计划按工具名去重或参数只命中第一个同名步骤。
4. ReAct 提示词含有未入选工具的硬编码示例，模型会调用本轮不可用或历史旧工具名。
5. agent-runtime 在进入 Aegis ToolGateway 前发生的工具名、参数和策略校验失败没有进入 Aegis 工具调用记录。
6. 最终总结缺少完整工具观察值和事实一致性校验，可能与已经成功返回的数据相矛盾。

本次修改 `agent-runtime` 和 Aegis `api-server`。不修改公网 HTTP API、数据库表、gRPC 协议、Agent 命令协议或现有审批数据。

成功标准：

- 业务语义仍由 LLM 产生，不恢复关键词意图路由或规则工具选择。
- `IntentBreakdown` 的范围、脚本类型、修复轮数和自动验证参数经过结构化校验；不合法输出最多纠正一次。
- Aegis 的 `ToolExecutionPlan` 是 agent-runtime 的唯一初始计划，运行时不再二次规划。
- 固定计划的每一步只能使用该步骤允许的工具，计划参数优先于模型参数。
- `poc` 和 `fix` 生成、状态查询能作为独立同名工具步骤执行，并按 `step_id` 绑定不同参数。
- 工具目录、参数说明和示例只从本轮真实描述符动态生成。
- 工具不存在、参数错误、策略拒绝等网关前失败可在 Hook、TaskResult 和 Aegis 工具记录中追踪。
- 最终回答必须以真实工具观察值为证据；发现矛盾时拒绝原总结并输出证据化回退结论。
- 指定 CVE 已存在时不启动自定义 CVE 查询；只有精确列表为空时才允许回退。

## 2. 目标数据流

```text
User message
  -> LLM IntentRouter
  -> LLM IntentDecomposer
  -> IntentBreakdown contract validation/correction
  -> LLM capability candidates
  -> ToolDecisionEngine
       -> typed ToolExecutionPlan
       -> repeated steps keep independent step_id and args
  -> agent-runtime InitialPlan
       -> skip planner/router plan generation
       -> enforce per-step allowed tools and bound args
       -> emit validation-stage tool records
  -> Aegis ToolGateway/ToolDispatcher
       -> approval and execution
       -> persisted tool evidence
  -> agent-runtime evidence-aware summary
  -> Aegis evidence consistency guard
  -> final answer
```

## 3. 跨仓库契约

### 3.1 agent-runtime

`TaskInput` 新增可选 `InitialPlan`。调用方提供时：

- 深拷贝并规范化计划元数据；
- 跳过 Router、Assess 和 Planner；
- 仍执行计划结构、依赖、工具存在性和禁用工具校验；
- 禁止动态修正新增计划步骤，由调用方计划保持唯一事实来源。

`PlanStep` 新增：

- `allowed_tools`：非空时作为当前步骤严格工具白名单；
- `tool_args`：计划预绑定参数，覆盖模型提供的同名参数。

工具范围或 Schema 校验失败必须生成 `ToolCallRecord` 和 `HookToolCallFinished`，包含稳定的 `validation_stage`。相同工具和参数的相同失败最多允许一次纠正，第二次终止该步骤。

最终总结输入包含每个真实工具观察的 `call_id/tool_name/status/content/error`，而不只包含步骤的 LLM 摘要。

### 3.2 Aegis

`IntentBreakdown` 增加结构化参数：

```json
{
  "scope": {"kind": "online_hosts"},
  "parameters": {
    "script_types": ["poc", "fix"],
    "execute_script_type": "poc",
    "max_rounds": 5,
    "auto_verify": true
  }
}
```

`scope.kind` 仅允许：

- `unspecified`
- `online_hosts`
- `all`
- `affected_hosts`
- `specified_hosts`
- `context_refs`

`script_types` 仅允许 `poc/fix`，`max_rounds` 范围为 1 至 10。在线或全部主机范围必须具有 `list_hosts` 能力。结构错误将反馈给 LLM 纠正一次，不根据用户关键词替代 LLM 语义。

`ToolExecutionPlan` 转换为 agent-runtime `InitialPlan`。每个步骤携带唯一 `step_id`、唯一工具白名单和预绑定参数。工具网关按 `step_id + tool_name` 查找参数，不再按第一个同名工具匹配。

agent-runtime 同时要求计划步骤的 `title` 唯一。Aegis 转换层不得直接把
`tool_name` 作为标题；重复的 Generate/Status 步骤使用
`<tool_name> (<script_type>) [<step_id>]` 作为稳定标题，普通步骤也附加
`step_id`。这保留了 Runtime 校验，且不会因 POC/FIX 使用同一个工具而出现
`duplicate step title`。

指定 CVE 处置计划至少包含：

1. 精确查询漏洞；
2. 仅在漏洞为空时执行自定义查询回退；
3. 解析受影响主机；
4. 在线范围解析；
5. 分别生成并等待 POC、FIX；
6. 使用 POC 下发检测，传递 `max_rounds` 并启用自动验证修复闭环。

工具网关在 Schema 校验之前执行动态参数准备和业务前置条件检查：

- 自定义 CVE 查询启动前必须存在成功的精确 `Vulnerability.List` 调用，且结果明确为 `total=0`；
- 漏洞已存在时，自定义查询启动和状态步骤返回带原因的成功跳过记录，不产生外部查询；
- `vulnerability_id` 只从漏洞列表结果或自定义查询终态的 `result_vulnerability_id` 解析；
- 脚本下发前必须存在相同 `cve_id + script_type` 的 `generated` 状态证据；
- 下发主机只取在线主机与受影响主机的交集；自定义 CVE 尚无主机关联时允许使用用户明确要求的在线主机范围；
- 状态类工具不复用同一消息内的旧成功结果，允许 Runtime 读取真实变化并轮询到终态。

## 4. 证据与最终回答

Aegis 从 `TaskResult.StepExecutions`、`ToolCalls` 和 `Errors` 构建证据账本，保存：

- 实际调用和调用阶段；
- 成功、失败、超时或拒绝；
- 工具原始观察值；
- 主机、漏洞、脚本状态和任务组引用；
- 未执行步骤及真实原因。

确定性事实守卫至少检查：

- 成功结果存在在线主机时，不得声称“没有在线主机”；
- 漏洞列表存在目标 CVE 时，不得声称“漏洞不存在/未收录”；
- 未发生的工具调用不得报告为失败；
- 没有成功 `Vulnerability.Script.Execute` 和 `task_group_id` 时不得声称“已下发”；
- 脚本状态不是 `generated` 时不得声称“脚本已生成”。

发现冲突时记录一次结构化 WARN，不记录原始脚本或大段工具结果；用户收到基于证据账本构造的保守回退结论。

## 5. 日志与失败处理

新增或调整日志边界：

- IntentBreakdown 契约纠正：WARN，字段包含 `action/domains/error_category/attempt`；Orchestrator 失败日志补充 `session_id/run_id`。
- 固定计划启用：INFO，字段包含 `plan_id/step_count/tool_count`。
- 步骤工具范围拒绝：WARN，字段包含 `task_id/step_id/tool_name/validation_stage`。
- 最终证据冲突：WARN，字段包含 `session_id/run_id/conflict_codes`。

不得记录 API Key、Authorization、完整脚本、完整 LLM 原始响应或大段主机数据。

## 6. 测试设计

### agent-runtime

| 用例 | 期望 |
| --- | --- |
| 提供 InitialPlan | Router 和 Planner 不被调用，直接执行固定计划 |
| 固定计划引用未知工具 | 计划校验失败 |
| 步骤调用 allowed_tools 外工具 | 网关不执行，TaskResult 和 Hook 记录 `step_tool_scope` |
| 步骤绑定参数与模型冲突 | 计划参数生效 |
| 网关 Schema 校验失败 | ToolCallRecord 保存 `arguments` 阶段 |
| 重复相同失败 | 第二次终止步骤 |
| 总结输入 | 包含真实 Observation 内容和错误 |

### Aegis

| 用例 | 期望 |
| --- | --- |
| scope.kind=host | 拆解结果被拒绝并触发一次 LLM 纠正 |
| online_hosts 缺少 list_hosts | 契约补齐或纠正后包含能力 |
| POC+FIX+5轮 | 生成两组独立步骤，参数分别为 `poc/fix`，执行步骤为 `poc/max_rounds=5` |
| 同名工具两步骤 | 按 step_id 获得不同 script_type |
| POC/FIX Generate 与 Status 重复工具 | Runtime 计划标题唯一并通过 agent-runtime Validator |
| CVE 已存在 | 自定义查询步骤被跳过且不产生外部调用 |
| 网关前失败 | Aegis 工具记录和运行事件均可见 |
| 在线主机返回 total>0 | 最终回答不能输出“没有在线主机” |
| 未返回 task_group_id | 最终回答不能输出“下发成功” |

## 7. 兼容性、发布与回滚

- agent-runtime 新字段均为可选字段；未提供 `InitialPlan/allowed_tools/tool_args` 的调用方保持原动态规划行为。
- Aegis 更新到 agent-runtime 新提交后启用固定计划，不影响其他仓库旧版本调用方。
- 不需要数据库迁移。
- 先发布 agent-runtime 提交，再更新 Aegis `api-server/go.mod/go.sum`，仅重建和重启 `api-server`。
- 回滚时将 Aegis 依赖恢复到旧伪版本并回滚本修复代码；无业务数据回滚。

## 8. 实现与验证记录

### 8.1 agent-runtime

- 分支：`codex/fixed-tool-plan-evidence`
- 提交：`f48921e326f2b873f7cbc19ffec11bc00fc89a65`
- Aegis 引用版本：`v0.0.0-20260709133937-f48921e326f2`
- 验证：`go test ./...` 通过。

### 8.2 Aegis api-server

已完成：

- LLM-only IntentRouter、IntentDecomposer 和工具选择，删除生产链路的规则语义降级；
- 结构化范围和 CVE 脚本参数契约及一次 LLM 纠正；
- 固定执行计划、重复同名脚本步骤和逐步工具白名单；
- 动态 CVE、漏洞 UUID、在线主机参数准备及业务前置条件；
- 网关前失败的事件、数据库工具调用记录和消息关联；
- 工具原始观察证据账本和最终结论冲突回退；
- Runtime 参数 Schema 数字兼容，避免 JSON 数字被误判为非整数。

验证结果：

| 验证项 | 结果 |
| --- | --- |
| `go test ./internal/assistant ./internal/assistant/tools -count=1` | 通过 |
| `go test ./internal/service -count=1` | 通过 |
| `make build` | 通过 |
| `docker compose up -d --build api-server` | 镜像构建和容器替换成功 |
| `docker compose ps api-server` | `healthy` |
| `GET http://localhost:8082/health` | `{"status":"ok"}` |
| 启动日志检查 | 未发现 panic、fatal、漏洞工具注册失败或 agent-runtime 启动错误 |

启动期间仍会出现已有的阻断策略种子数据 `record not found` 警告，随后种子流程正常完成；该日志与本次智能助手修改无关。
