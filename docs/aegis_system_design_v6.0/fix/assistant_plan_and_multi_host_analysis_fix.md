# Bug Fix: Assistant 计划栏恢复与多主机分析覆盖

## Bug 描述与症状

1. 会话运行中，部分思考框、工具结果框、回答框之间视觉距离过大，长 JSON 工具结果会把对话流拉得很散。
2. 会话结束后刷新页面，右侧计划栏从完整计划退化为单个步骤，例如只显示 `1/1 Agent 实时取证`。
3. 用户要求分析全部主机或在线主机时，运行过程可能只对部分主机做 Agent 取证，最终回答也不是“每台主机分析 + 整体汇总”的格式。

## 复现步骤

1. 使用管理员账号登录系统，进入 `/assistant`。
2. 打开最新完成会话 `asst_56a64252`。
3. 观察右侧计划栏刷新后只剩一个计划步骤。
4. 查看该会话工具调用记录：`Host.List` 返回 2 台主机，`Host.Get` 覆盖 2 台，但 `Agent.Process.List` / `Agent.Network.List` 只对其中 1 台主机执行。

## 根因分析

- 前端历史重建会把 `步骤完成: X` 拆成单独展示片段，并为该片段创建单步 `plan` 以展示步骤结果卡。`AssistantWorkspace` 倒序选择最新带 `plan` 的消息，结果拿到了单步结果计划，而不是原始完整计划。
- 后端压缩到 session metadata 的 `assistant_runtime_events` 只保留 `thinking` 和 `tool_call`，缺少 `plan` / `step_started` / `step_completed`，新会话缺少更稳定的计划回放来源。
- Prompt 只描述“某台主机”的最低证据要求，没有明确“全部主机/所有主机/整体平台/在线主机”必须逐台覆盖，并且最终回答必须逐台输出。
- `Host.List` 的 schema 和 handler 不兼容模型实际传入的 `limit/status/agent_status/filters` 参数，在线筛选可能被静默忽略。
- 工具结果默认预览长度偏大，格式化 JSON 结果会形成很高的卡片，造成视觉上的大断层。

## 修复设计

- 前端历史重建时，最终回答片段保留完整 `plan`；单步结果片段仍保留单步 `plan` 用于步骤结果卡。
- 右侧计划栏改为从候选消息中选择步骤数最多的 plan，避免单步结果 plan 覆盖完整计划。
- 对话渲染层识别历史最终回答片段：该片段上的完整 plan 只用于计划栏，不重复渲染成步骤结果卡。
- 后端 runtime display events 增加 `plan` / `step_started` / `step_completed` 压缩保存，前端可从 metadata 回放计划状态。
- PromptProvider 增加批量主机硬约束：目标主机数大于 1 时必须逐台覆盖、逐台输出，并给整体汇总。
- `Host.List` 增加 `limit/status/agent_status/filters` 兼容，按最近心跳计算在线/离线；`Host.AgentStatus.Get` 返回在线/离线主机与统计。
- 工具结果预览长度从 900 降到 560，并给结果区域设置最大高度滚动。

## 验证步骤

- `cd frontend && npm run test -- src/store/assistant.test.ts src/views/assistant/components/AssistantConversation.test.ts --run`
- `cd api-server && go test ./internal/assistant/...`
- Playwright 登录 `/assistant?session=asst_56a64252`，确认右侧计划栏恢复为完整多步骤计划，工具结果卡不再无限撑高。

## 风险与回滚

- `Host.List` 在线状态基于 90 秒心跳窗口，和原 `Host.AgentStatus.Get` 逻辑保持一致；如果实际在线判定策略调整，应同步修改这两个工具。
- 若新 prompt 导致模型执行时间变长，可在批量主机场景保留逐台覆盖要求，但限制每台主机的 Agent 取证工具集合。
- 回滚时恢复本次前端 store/workspace/conversation 改动、后端 runtime display events 和 host tool 参数兼容即可。
