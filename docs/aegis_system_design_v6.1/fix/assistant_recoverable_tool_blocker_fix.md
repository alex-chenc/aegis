# 修复：智能体工具可恢复阻塞与用户决策续接

## 1. 问题与根因

动态检测包生成会在当前 Hook 白名单无法覆盖漏洞利用链时失败关闭。这一安全边界
本身正确，但运行时把服务层的结构化业务错误压成了字符串：

```text
DetectionPackageGenerationService
  -> unsupported coverage typed error
  -> ToolRegistry converts error to ToolExecutionResult.Error string
  -> ToolDispatcher marks failed
  -> agent-runtime returns failed goal
  -> generic failed answer
```

因此系统丢失了错误类别、可选动作、风险等级和恢复上下文，无法向用户提供
“扩展白名单并继续 / 仅查看变更建议 / 暂停 / 取消 / 其他说明”等决策。刷新页面后
也没有可恢复状态。

该问题不是 Hook 白名单专属问题。缺少权限、策略冲突、资源锁、可安全重试的外部
依赖失败和需要补充业务参数，都可能是可恢复阻塞；但编译错误、程序缺陷、未知错误
不应伪装成可恢复选项。

## 2. 目标

1. 在服务、工具注册、调度、编排、HTTP、SSE 和前端之间保留结构化恢复契约。
2. 仅对实现 `DescribedError` 契约的错误创建恢复请求，未知错误继续失败关闭。
3. 恢复请求持久化，页面刷新、重新登录和 API 服务重启后仍可查看和决策。
4. 恢复动作由后端声明和执行；大模型不能发明动作 ID、权限或执行参数。
5. 高风险动作展示精确影响并要求用户再次确认。
6. 用户选择会改变安全配置的动作后，重新校验当前状态，再创建关联的新运行；
   不唤醒已经结束的内存运行。
7. 已完成的外部副作用不得因恢复而盲目重复。只有错误契约显式声明
   `resumes_run=true`，且对应工作流满足幂等/前置阶段未产生副作用时才自动续接。
8. `full_access` 不自动扩大 Hook、网络、主机或权限边界；扩大边界必须由用户明确
   选择恢复动作。

## 3. 错误分类

| 分类 | 默认处理 | 是否生成决策卡 |
| --- | --- | --- |
| `automatic_correction` | 在限定次数内自动纠正 | 否 |
| `needs_input` | 请求补充业务输入 | 是 |
| `recoverable_business_blocker` | 提供安全恢复动作 | 是 |
| `authorization_required` | 进入审批/授权流程 | 是 |
| `transient_dependency` | 提供重试/暂停 | 是 |
| `terminal_failure` | 失败关闭并记录错误 | 否 |
| 未分类异常 | 失败关闭 | 否 |

本次首先落地 `recoverable_business_blocker`。后续模块只需返回同一恢复描述契约，
无需在编排器中增加工具名称分支。

## 4. 持久化模型

新增 `assistant_recovery_requests`：

- 关联：`session_id`、`run_id`、`message_id`、`step_id`、`tool_call_id`、
  `tool_name`。
- 分类：`code`、`category`、`risk_level`、`summary`、`detail`。
- 快照：原始用户目标、工具参数、恢复上下文、后端声明的动作列表。
- 决策：`status`、`selected_action_id`、`decision_input`、`resolution_result`、
  `decided_by`、时间戳。
- 续接：`resume_run_id`，用于把新运行关联回产生阻塞的运行。

状态机：

```text
pending
  -> executing -> resolved
  -> paused
  -> cancelled
  -> expired
  -> failed
```

决策接口使用数据库条件更新保证只有 `pending` 请求可以被第一次选择；重复提交
返回当前状态，不重复执行副作用。

## 5. 通用恢复协议

服务层错误提供：

- 稳定错误码和分类；
- 面向用户的安全摘要与说明；
- 不含凭证的上下文；
- 后端动作列表；
- 每个动作的风险、是否需要确认、是否自动续接和执行器标识。

工具注册层在返回普通显示字符串的同时保留内部 `Cause`，该字段不序列化。调度器
识别恢复契约后：

1. 创建持久化恢复请求；
2. 将工具调用标记为 `blocked`，而不是普通 `failed`；
3. 推送 `recovery_required` SSE；
4. 让本轮运行在证据收敛阶段转为 `needs_input`。

编排器按 `run_id` 查询待处理恢复请求，优先生成恢复说明，不再用通用失败文案覆盖。

## 6. Hook 白名单场景

### 6.1 覆盖性结果

检测包生成器的 `Coverage Decision` 新增 `required_hooks`：

```yaml
status: unsupported
reason: active allowlist cannot observe AF_ALG and splice
covered_behaviors: []
uncovered_core_behaviors: [AF_ALG socket setup, splice into page cache]
required_hooks:
  - attach_type: tracepoint
    attach: syscalls/sys_enter_socket
  - attach_type: tracepoint
    attach: syscalls/sys_enter_bind
  - attach_type: tracepoint
    attach: syscalls/sys_enter_splice
```

后端仅接受受支持的 attach 类型和合法名称。缺少或非法的建议不会出现“自动加入”
动作，只允许查看建议、暂停、取消或补充说明。

### 6.2 用户动作

1. `extend_hook_allowlist`：展示精确新增项，高风险确认后读取最新白名单、合并去重、
   创建新版本并同步 Agent，成功后创建关联的新运行。
2. `prepare_hook_allowlist_change`：只返回当前配置与建议差异，不改变配置。
3. `pause`：保留恢复请求与原始目标，稍后仍可选择其他已声明动作。
4. `cancel`：取消当前目标，不执行配置变更。
5. `provide_other`：记录用户说明，不执行模型生成的任意动作。

执行器在真正修改前再次读取当前生效版本并重新计算差异；如果建议已存在则按幂等
处理。动作只有显式声明 `retry_safe=true` 时，执行失败才回到 `pending` 并允许用户
重试；其他动作标记失败，防止不确定副作用被重复执行。如果同步 Agent 失败，保留
新配置版本和错误证据，不自动声称恢复成功。

## 7. API 与事件

HTTP：

- `GET /assistant/sessions/:session_id/recoveries`
- `GET /assistant/recoveries/:recovery_id`
- `POST /assistant/recoveries/:recovery_id/decision`

决策请求：

```json
{
  "action_id": "extend_hook_allowlist",
  "input": {
    "comment": "同意将卡片中列出的 3 个 tracepoint 加入白名单"
  }
}
```

SSE：

- `recovery_required`：创建并等待用户决策；
- `recovery_updated`：状态或执行结果变化；
- 自动续接成功时返回新的 `run_handle`，前端切换到新运行的流。

## 8. 前端行为

- 对话区与输入框之间显示持久化恢复决策卡。
- 卡片展示原因、影响、精确配置差异、风险和动作。
- `confirmation_required=true` 的动作使用确认框。
- `provide_other` 要求输入说明。
- 决策进行中禁用重复点击。
- 返回 `run_handle` 时自动连接新运行 SSE；不创建伪造的用户消息。
- 刷新会话时并行加载恢复请求，已暂停/已解决记录保留审计状态，默认突出
  `pending` 请求。

## 9. 日志与安全

必须记录：

- 恢复请求创建：session/run/tool/error code/action IDs；
- 用户决策：recovery/action/operator，不记录自由文本全文；
- 执行开始、完成、失败和续接 run ID；
- Hook 变更只记录新增 Hook 和版本，不记录凭证或模型原始提示。

严禁：

- 根据错误字符串模糊匹配后直接修改配置；
- 允许客户端提交未在持久化动作列表中的 action ID；
- 允许客户端覆盖后端保存的 Hook 列表；
- 用 `full_access` 绕过恢复动作确认；
- 在未知错误上展示“重试即可成功”的误导选项。

## 10. 验收与回归测试

1. 结构化恢复错误穿过 ToolRegistry 后仍可被 ToolDispatcher 识别。
2. 普通错误不创建恢复请求。
3. 恢复请求字段、动作和原始目标持久化正确。
4. 同一 tool call/error code 不重复创建多个 pending 请求。
5. 编排器检测到本轮 pending recovery 后返回 `needs_input`，不返回失败结论。
6. 非法 action ID、重复决策、缺少“其他”说明均被拒绝。
7. Hook 建议合法时显示扩展动作；建议缺失或非法时不显示扩展动作。
8. Hook 合并保持原配置、去重并创建新版本。
9. 仅查看建议、暂停、取消均不修改白名单。
10. 高风险动作成功后返回关联新运行，前端连接 SSE 且不重复显示用户消息。
11. 刷新会话后恢复卡仍存在。
12. API Go 定向测试、前端组件测试、类型检查和构建通过。
13. 新迁移被 `scripts/build_release_package.sh` 的词法聚合逻辑收入离线
    `backend/scripts/init.sql`；本任务不生成或覆盖发布 zip。

## 11. 回滚

- 代码回滚后保留 `assistant_recovery_requests` 表不会影响旧版本读取。
- 前端不认识恢复事件时仍会收到本轮 `needs_input` 文案。
- 若 Hook 扩展已经创建新白名单版本，回滚应用代码不会自动回滚安全配置；管理员
  应通过现有白名单管理接口重新提交上一版本配置，并确认 Agent 同步结果。

## 12. 2026-07-25 重复恢复卡与不可交互修复

### 12.1 新发现的契约缺口

最初实现将可恢复阻断持久化并推送给前端，但 ToolGateway 仍把该调用返回为普通
`ToolCallFailed`。agent-runtime 的 ReAct 循环无法区分“可修正失败”和“必须等待
用户决策”，因此会使用新的 `tool_call_id` 重复调用同一工具。原唯一键只覆盖
`tool_call_id + code`，每次重试都会创建新的恢复请求。与此同时，前端在 SSE 仍
处于 streaming 状态时渲染恢复卡，并以全局 streaming 状态禁用卡片动作，导致
用户看得到决策卡却不能作出决策。

### 12.2 目标状态

```text
typed recoverable blocker
  -> persist or reuse one active recovery by run/step/tool/code
  -> mark tool call blocked
  -> mark ActiveRun paused-for-recovery and cancel only this run context
  -> agent-runtime unwinds without another ReAct tool call
  -> orchestrator resolves the persisted recovery as needs_input
  -> publish one recovery_required event after runtime has stopped
  -> frontend renders one enabled decision card
```

普通失败、超时和可由模型修正的参数错误继续使用原有重试机制；只有已经通过
`DescribedError` 契约并成功持久化的恢复请求可以暂停运行。

### 12.3 幂等与迁移

活跃恢复请求的业务唯一键调整为：

```text
run_id + normalized_step_id + tool_name + code
```

`normalized_step_id` 在 `step_id` 为空时回退到 `tool_name`。升级迁移优先保留
`executing`，其次保留 `paused`，最后保留 `pending`；同状态下保留最早创建的
请求。其余历史重复请求标记为 `expired`，不删除审计记录，随后建立部分唯一索引。
仓储创建还必须处理“查询后并发插入”的唯一键竞争，并返回已经存在的请求。

### 12.4 前端约束

- 活跃卡片按同一业务键和相同状态优先级防御性去重，后端历史脏数据不能再次形成
  多张相同卡片；`executing` 卡保持禁用以防重复提交。
- `recovery_required` 只在运行时已停止后推送；卡片动作不能再由全局 streaming
  状态禁用，只能由该恢复请求自身的提交状态禁用。
- 恢复事件到达后将 UI 运行状态切换为 `needs_input`；SSE 仍由随后到达的 `done`
  事件正常关闭，避免丢失最终会话快照。

### 12.5 新增回归验收

1. 同一运行步骤使用不同 `tool_call_id` 连续报告相同阻断时只返回一个恢复请求。
2. 首次恢复请求创建后当前运行上下文立即取消，ReAct 不得发起第二次工具调用。
3. 同一业务键的并发创建由数据库唯一键收敛为一条活跃记录。
4. 会话存在历史重复 active 请求时，迁移只保留最早一条 active，其余标记
   `expired`。
5. 前端收到两条相同业务键恢复数据时只渲染一张卡。
6. 即使 SSE 尚在收尾，恢复卡动作仍可点击并提交；重复提交由恢复请求自身的 busy
   状态阻止。
