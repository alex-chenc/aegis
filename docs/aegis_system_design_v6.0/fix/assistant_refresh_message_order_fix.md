# Bug Fix: 智能体模式刷新页面后消息顺序混乱

## Bug 描述与症状

### 症状
在智能体（Assistant）模式下，当一个任务结束后点击刷新页面，页面的思考（thinking）、工具执行（tool call）等框顺序会乱。思考框都集中在最上面和最下面，中间的工具执行框消失了。

### 影响范围
- 影响组件：
  - 后端 `api-server/internal/assistant/adapter_hook_sink.go`
  - 后端 `api-server/internal/assistant/orchestrator.go`
  - 后端 `api-server/internal/assistant/runtime_factory.go`
  - 前端 `frontend/src/store/assistant.ts`
  - 前端 `frontend/src/views/assistant/components/AssistantConversation.vue`
- 影响功能：智能体模式页面刷新后的历史消息展示
- 严重程度：高（影响用户体验，核心功能不可用）

## 复现步骤

1. 进入智能体模式，发送一个需要多步工具调用的任务
2. 等待任务执行完成（看到多个 thinking 块和 tool call 块交替显示）
3. 点击浏览器刷新按钮
4. 观察页面消息顺序：thinking 块集中在顶部，中间的 tool call 块消失

## 根因分析

### 核心问题：运行时事件没有持久化顺序 + 中间模型输出重复展示

运行中页面按 SSE 事件到达顺序展示；任务完成后刷新只能从 `assistant_messages.thinking` 和 `assistant_tool_calls` 重新猜测顺序。重复工具名、分页、异步加载都会导致刷新后顺序和运行中不一致。

#### 问题 1: `fetchToolCalls` 内部提前重建，且重建结果不可逆

`openSession` 函数通过 `Promise.all` 并行执行四个数据加载：

```typescript
async function openSession(sessionId: string) {
  await Promise.all([
    fetchMessages(sessionId),      // 设置 messages.value
    fetchContextRefs(sessionId),
    fetchToolCalls(sessionId),     // 设置 toolCalls.value + 调用 rebuild！
    fetchApprovals(sessionId),
  ])
  rebuildAssistantHistoryCycles()  // 再次调用
}
```

旧实现中 `fetchToolCalls` 在内部调用 `rebuildAssistantHistoryCycles()`：

```typescript
async function fetchToolCalls(sessionId, params) {
  const result = await apiGetToolCalls(sessionId, params)
  toolCalls.value = result.items
  rebuildAssistantHistoryCycles()  // ← 问题所在！
}
```

`rebuildAssistantHistoryCycles` 会把一条持久化 assistant 消息拆成多条 `_history_` 展示片段。拆分后再次调用重建时会跳过这些 `_history_` 片段：

- 如果首次重建时工具调用列表还没加载完成，或接口只返回了部分工具调用，工具标记 `正在调用工具: X` 会无法正确匹配。
- 原始消息被替换为 `_history_` 克隆后，后续即使拿到完整工具调用，也不会再基于原始消息重新拆分。
- 展示组件对单条 assistant 消息的渲染顺序是 `thinking → content → tool`，因此未正确拆分时会看到 thinking 集中在上方/下方，工具执行框不在原始位置。

#### 问题 2: 工具调用 API 分页限制和排序方向

后端 `ListToolCalls` 默认 `page_size=20`（[assistant_handler.go:240](api-server/internal/api/handler/assistant_handler.go#L240)），并按 `created_at DESC` 返回。前端 `fetchToolCalls` 不传参数时只获取最新 20 条；如果会话超过 20 个工具调用，较早工具调用缺失，thinking 步骤中的工具调用标记无法匹配。

旧的匹配逻辑在“命名工具标记找不到同名工具调用”时会退化匹配任意未使用工具调用，进一步导致工具卡片错配。

#### 问题 3: 消息保存时未嵌入工具调用

后端 `completeRun`（[service.go:340](api-server/internal/assistant/service.go#L340)）和 `runAgentRuntime`（[orchestrator.go:514](api-server/internal/assistant/orchestrator.go#L514)）保存 `AssistantMessage` 时**不设置 `ToolCalls` 字段**。工具调用单独存储在 `assistant_tool_calls` 表。前端重建时依赖 `getRelatedToolCalls` 从全局 `toolCalls` store 获取，增加了竞态风险。

#### 问题 4: 运行过程事件混入 thinking 展示

后端 `adapter_hook_sink.go` 会把步骤开始/完成、审计、反思、工具调用提示等运行过程事件也发布为 `thinking`。历史消息持久化时 `extractThinkingFromHistory` 会把所有 `EventThinking` 原样存入 `assistant_messages.thinking`。刷新后前端逐条拆分这些 thinking，导致 `开始执行步骤`、`步骤完成`、`审计完成: null`、`正在调用工具: Host.List` 等过程文案集中显示成多个“思考”卡片。

#### 问题 5: 中间模型输出和最终回答都走 `message_delta`

`HookModelCallFinished` 会把 ReAct 中间模型输出解析后发布为 `message_delta`。复杂分析中多个步骤的 `step_result` 可能都像“最终分析报告”，而任务结束时 `orchestrator` 又会发布真正最终回答，导致前端看到多份分析报告。

#### 问题 6: 反思结果作为可见 thinking 推送

工具或步骤失败后，runtime 会触发反思。旧逻辑把反思开始/结果缓冲到任务结束后再 flush 成 `thinking`，刷新后也会进入 `assistant_messages.thinking`，但反思属于内部恢复策略，不应该在前端页面展示。

#### 问题 7: 没有可回放的运行时事件流

`assistant_tool_calls` 可以保存工具调用结果，但没有保存“这个工具调用在第几个 thinking 后出现”的事件序。只靠工具名匹配无法处理重复的 `Host.List` / `Vulnerability.List`。

#### 问题 8: 运行中组件渲染仍按类型集中分组

即使后端和 store 已经把工具调用与 `call_id` 关联，运行中页面仍可能把多个事件落在同一条 assistant 消息里。旧组件 `getAssistantSegments` 固定按 `thinking → content → tool_calls` 输出展示段，导致实时页面也会出现多条“正在调用工具”集中在前面、工具结果卡集中在后面的现象。

#### 问题 9: 中间步骤失败被当成最终状态展示

agent-runtime 在单步执行过程中可能先触发 `HookStepFailed`，随后通过反思、重试或再次调用工具恢复成功。旧逻辑把 `步骤失败: X` 直接推到前端 thinking，后续成功不会撤销该文案，导致 UI 出现“工具成功但步骤失败”的矛盾。

#### 问题 10: 步骤完成缺少紧邻的步骤结果

`HookStepCompleted` 只推送 `步骤完成: X` thinking 和 plan step 状态，未携带 `result_summary`；持久化 plan 也只保存初始 plan 事件，没有回放后续 `step_completed`。因此刷新和运行中都会出现“步骤完成”后面没有配套结果卡的问题。

## 修复设计

### 修复方案

### UI 参考

参考 `assistant-ui` 的 thread/message primitives 思路：前端应按运行时事件 part 的顺序渲染，不应先按类型集中分组再展示。Aegis 当前仍使用本地 Vue 组件，不引入 React 依赖，只吸收“顺序化消息 part / tool result / step result”的展示模型。

补充参考：
- `langchain-ai/agent-chat-ui`：以 `messages` 数组作为唯一顺序来源，AI message 的 `tool_calls` 和后续 tool message 通过 `tool_call_id` 绑定；缺失工具响应时补不可渲染 tool response，避免状态机断裂。
- `CopilotKit/CopilotKit`：同样围绕 tool call id 维护工具调用与结果的对应关系，展示层不按类型重新集中分组。

#### 修改 1: 移除 `fetchToolCalls` 内部的提前重建

**文件**: `frontend/src/store/assistant.ts`
**修改**: 删除 `fetchToolCalls` 中的 `rebuildAssistantHistoryCycles()` 调用（第 675 行）

`openSession` 在 `Promise.all` 之后已有 `rebuildAssistantHistoryCycles()` 调用（第 1400 行），确保所有数据加载完成后再重建。

#### 修改 2: 增加工具调用 API 的 page_size 并自动补齐分页

**文件**: `frontend/src/store/assistant.ts`
**修改**: `fetchToolCalls` 默认请求 `page_size=100`；如果 `total` 超过当前页数量且调用方没有指定固定页码，则继续拉取后续分页，确保用于历史重建的工具调用集合完整。

#### 修改 3: 保证重建时数据完整性

**文件**: `frontend/src/store/assistant.ts`
**修改**: `rebuildAssistantHistoryCycles` 增加防护，如果 `toolCalls.value` 为空且消息中有工具调用标记，跳过重建（等待后续调用）。

#### 修改 4: 避免工具卡片错配

**文件**: `frontend/src/store/assistant.ts`
**修改**: 当 thinking 标记包含明确工具名时，仅匹配同名工具调用；找不到同名工具时不再退化匹配任意工具。

#### 修改 5: 按运行时事件顺序原样重放

**文件**: `frontend/src/store/assistant.ts`、`frontend/src/views/assistant/components/AssistantConversation.vue`
**修改**: 历史重建时逐条保留 thinking 原文和顺序；遇到 `正在调用工具: X` 时，在该 thinking 后面插入对应工具卡。组件渲染层也保持逐条 thinking 展示，保证刷新后与运行中的展示形态一致。

#### 修改 6: 最终报告只保留一个可见出口

**文件**: `api-server/internal/assistant/adapter_hook_sink.go`
**修改**: `HookModelCallFinished` 不再发布可见 `message_delta`。模型中间输出只用于 runtime 内部决策；前端最终回答由 `orchestrator.runAgentRuntime` 在任务结束后统一发布并保存，避免出现 2-3 份“最终分析报告”。

#### 修改 7: 反思结果持久化，不展示到前端

**文件**:
- `api-server/internal/assistant/adapter_hook_sink.go`
- `api-server/internal/assistant/reflection_experience_provider.go`
- `api-server/internal/assistant/runtime_factory.go`
- `api-server/internal/assistant/adapter_prompt_provider.go`

**修改**:
- `HookReflectionStarted` 不发布前端事件。
- `HookReflectionFinished` 将 root cause / recommendation 等写入 `assistant_memory`，类型为 `reflection`。
- RuntimeFactory 启用 memory-backed `ExperienceProvider`，后续分析可先读取反思经验。
- PromptProvider 注入最近反思，并要求工具错误后参考反思重试一次，再失败则跳过并记录证据缺口。
- `MaxStepRetries` 从 2 收紧为 1，避免同一错误无限重试。

#### 修改 8: 保存可回放的 runtime display events

**文件**:
- `api-server/internal/assistant/runtime_display_events.go`
- `api-server/internal/assistant/orchestrator.go`
- `frontend/src/store/assistant.ts`

**修改**:
- 任务完成保存 assistant message 前，从 `RunManager.History()` 压缩出 `thinking` / `tool_call` 事件流。
- 事件流按 `message_id` 写入 `assistant_sessions.metadata.assistant_runtime_events`。
- 前端刷新时优先使用 metadata 事件流，按 `call_id` 精确插入工具卡；没有 metadata 的旧会话再走 thinking + tool_calls 降级匹配。

#### 修改 9: 工具调用必须成对展示

**文件**:
- `api-server/internal/assistant/adapter_hook_sink.go`
- `api-server/internal/assistant/runtime_display_events.go`
- `frontend/src/store/assistant.ts`
- `frontend/src/views/assistant/components/AssistantConversation.vue`

**修改**:
- 后端 `HookToolCallStarted` 发布的 thinking payload 带上 `call_id` / `tool_name`。
- 压缩 runtime display events 时保留工具 thinking 的 `call_id` / `tool_name`。
- 前端重建时，`正在调用工具: X` 必须匹配到工具记录才显示；匹配后立即插入工具卡。
- 没有对应工具记录的孤儿工具 thinking 不展示。
- 有工具记录但没有 thinking 标记时，前端补一个 `正在调用工具: X` 思考，再紧跟工具卡。
- 运行中组件渲染同一条 assistant 消息时，也按工具名把 `正在调用工具: X` 与对应工具卡立即配对输出，避免同一消息内部再次按类型集中分组。
- 如果匹配到的工具卡仍是 `pending/running`，组件暂停渲染后续 thinking、工具卡、最终内容和结果卡，等该工具变为 `completed/success/failed/cancelled/approval_required/rejected` 后再继续展示后续内容。

#### 修改 10: 步骤失败改为内部恢复事件，步骤完成必须带结果

**文件**:
- `api-server/internal/assistant/adapter_hook_sink.go`
- `api-server/internal/assistant/plan_history.go`
- `api-server/internal/assistant/orchestrator.go`
- `api-server/internal/assistant/service.go`
- `frontend/src/store/assistant.ts`
- `frontend/src/views/assistant/components/AssistantConversation.vue`

**修改**:
- `HookStepFailed` / `HookStepRetrying` 不再发布可见 thinking，避免展示 transient failure。
- `HookModelCallFinished` 不发布可见消息，但解析 ReAct `step_result` 并按 `step_id` 缓存。
- `HookStepCompleted` 发布结构化 `step_completed` 时带上 `result_summary`；没有模型摘要时兜底为“已完成步骤：X”。
- 后端保存历史 plan 时回放 `step_started` / `step_completed`，把 step 状态和 `result_summary` 写入消息 plan。
- 前端历史重建时，`步骤完成: X` 克隆消息自带单步 plan 结果。
- 组件渲染时，`步骤完成: X` 后立即插入步骤结果卡；旧历史里的 `步骤失败` / `正在重试步骤` 被过滤。
- UI 间距按 timeline 形态收紧，降低工具结果与后续步骤之间的视觉断层。

#### 修改 11: 审计事件不进入前端 timeline

**文件**:
- `api-server/internal/assistant/adapter_hook_sink.go`
- `frontend/src/store/assistant.ts`
- `frontend/src/views/assistant/components/AssistantConversation.vue`

**修改**:
- `HookAuditStarted` / `HookAuditFinished` 不再发布可见 `thinking`。
- 旧历史中已经保存的 `正在审计执行进度...` / `审计完成: null` 在 store 和组件层过滤。
- 根因是 `HookAuditFinished` payload 为空时 `json.Marshal(nil)` 会得到 `null`，旧逻辑拼成了 `审计完成: null`。

#### 修改 12: 运行中刷新时恢复只有 tool_calls 的 assistant timeline

**文件**:
- `frontend/src/store/assistant.ts`
- `frontend/src/views/assistant/components/AssistantConversation.vue`

**修改**:
- 刷新运行中会话时，后端可能尚未保存最终 `assistant_messages`，但 `assistant_tool_calls` 已经落库。
- `openSession` 在加载消息和工具调用后，读取 session metadata 中的 `current_message_id` / `current_run_id`。
- 如果没有对应 assistant message，则根据同 `message_id` 的工具调用合成一条运行中 assistant 消息，并补 `正在调用工具: X` thinking marker。
- 启动 SSE 后设置当前运行 message id，后续工具结果继续更新同一张工具卡。
- 组件层用全局 `toolCalls` 覆盖消息内旧工具状态，避免刷新后工具卡状态 stale。

### 修改文件清单

| 文件 | 修改内容 |
|------|---------|
| `api-server/internal/assistant/adapter_hook_sink.go` | 不再发布中间模型输出；反思写 memory，不进入前端 thinking |
| `api-server/internal/assistant/reflection_experience_provider.go` | 将 `assistant_memory.reflection` 暴露给 agent-runtime ExperienceProvider |
| `api-server/internal/assistant/plan_history.go` | 从运行事件回放 plan 状态和 step result_summary |
| `api-server/internal/assistant/runtime_factory.go` | 注入 memory repo、启用 experience、收紧步骤重试次数 |
| `api-server/internal/assistant/adapter_prompt_provider.go` | 注入历史反思和工具失败重试/跳过规则 |
| `api-server/internal/assistant/runtime_display_events.go` | 压缩可回放的 thinking/tool_call 事件流 |
| `api-server/internal/assistant/orchestrator.go` | 完成时把事件流写入 session metadata |
| `frontend/src/store/assistant.ts` | 事件流优先重建；降级分页补齐；过滤内部反思；工具 thinking 与工具卡成对展示 |
| `frontend/src/views/assistant/components/AssistantConversation.vue` | 渲染层逐条展示 thinking，并过滤内部反思 |
| `frontend/src/store/assistant.test.ts` | 覆盖 metadata 顺序、重复工具、分页、反思过滤 |
| `frontend/src/views/assistant/components/AssistantConversation.test.ts` | 覆盖组件层反思过滤 |

### 回归测试用例

#### TC1: 刷新后消息顺序正确
- 前提：会话有 thinking + tool_call + thinking + tool_call + content 的消息
- 操作：调用 openSession
- 验证：rebuild 后消息顺序为 thinking → tool → thinking → tool → content

#### TC2: fetchToolCalls 完成顺序不影响结果
- 前提：模拟 fetchToolCalls 先于 fetchMessages 完成
- 操作：调用 openSession
- 验证：rebuild 在所有数据加载后执行，消息顺序正确

#### TC3: 工具调用超过 20 个时仍能正确重建
- 前提：会话有 30 个工具调用
- 操作：调用 openSession（page_size=100）
- 验证：所有工具调用都被正确关联到 thinking 步骤

#### TC4: 过早重建等待工具调用数据
- 前提：消息包含 `正在调用工具: X` 标记，但工具调用列表尚未加载
- 操作：先调用 `rebuildAssistantHistoryCycles`，再加载工具调用并再次重建
- 验证：首次重建保留原始消息；工具调用加载后按 thinking → tool → thinking → content 正确拆分

#### TC5: 工具调用超过 100 个时补齐后续分页
- 前提：会话有 130 个工具调用，API 第一页返回 100 个
- 操作：调用 `fetchToolCalls`
- 验证：继续请求第 2 页并合并全部 130 个工具调用

#### TC6: thinking 和工具卡按运行顺序原样重放
- 前提：历史 thinking 包含 `开始执行步骤`、`步骤完成`、`审计完成`、`正在调用工具` 等过程文案
- 操作：加载并重建历史消息，或直接渲染未重建消息
- 验证：每条 thinking 原样保留；匹配到的工具卡插入在对应 `正在调用工具` thinking 后面；最终结果仍在最后

#### TC7: 重复工具名按 runtime metadata 的 call_id 顺序重建
- 前提：同一个 `Host.List` 出现两次，数据库 `created_at` 顺序和真实运行事件顺序不同
- 操作：session metadata 提供 `assistant_runtime_events`
- 验证：前端按 metadata 中的 `call_id` 顺序插入工具卡，不按工具名猜测

#### TC8: 中间模型输出不产生可见报告
- 前提：`HookModelCallFinished` 收到 ReAct `step_result`
- 操作：调用 hook sink
- 验证：不发布 `message_delta`

#### TC9: 反思结果只写 memory，不显示到前端
- 前提：`HookReflectionFinished` 收到 root cause / recommendation
- 操作：调用 hook sink
- 验证：创建一条 `assistant_memory.reflection`，不发布 thinking；前端旧历史中的“正在反思/反思结果”也会被过滤

#### TC10: 孤儿工具 thinking 不显示，未标记工具自动补 thinking
- 前提：历史 thinking 中有两个 `正在调用工具: Host.List`，但只有一个 Host.List 工具记录；另有一个工具记录缺少 thinking 标记
- 操作：刷新并重建历史消息
- 验证：只显示一个 Host.List thinking 并紧跟 Host.List 工具卡；缺少标记的工具前自动补 `正在调用工具: X`

#### TC11: 运行中组件不集中展示工具调用提示
- 前提：同一条 assistant 消息内包含两个 `正在调用工具` thinking 和两个工具卡
- 操作：直接渲染该消息
- 验证：DOM 顺序为 `正在调用工具: A → A 工具卡 → 正在调用工具: B → B 工具卡 → 后续普通思考 → 最终内容`

#### TC12: 当前工具未完成前不显示下一次工具调用
- 前提：同一条 assistant 消息内包含两个工具提示，但第一个工具卡状态仍为 `running`
- 操作：直接渲染该消息
- 验证：DOM 只显示 `正在调用工具: A → A running 工具卡`，不提前显示 B 工具提示、B 工具卡或后续思考

#### TC13: transient step failure 不显示
- 前提：同一步先触发 `HookStepFailed` / `HookStepRetrying`，后续工具调用成功并触发 `HookStepCompleted`
- 操作：调用 hook sink 并渲染历史 thinking
- 验证：前端不显示 `步骤失败` / `正在重试步骤`，只显示最终 `步骤完成`

#### TC14: 步骤完成后紧跟步骤结果卡
- 前提：`HookModelCallFinished` 产生 ReAct `step_result`，随后 `HookStepCompleted`
- 操作：渲染该 assistant 消息
- 验证：DOM 顺序为 `步骤完成: X → 步骤结果卡 → 后续 thinking/tool/content`

#### TC15: 审计完成不显示
- 前提：后端触发 `HookAuditStarted` / `HookAuditFinished`，或旧历史 thinking 包含 `审计完成: null`
- 操作：调用 hook sink、刷新并渲染历史消息
- 验证：不会发布可见 thinking；旧历史中也不会显示 `审计完成`

#### TC16: 运行中刷新不丢工具结果
- 前提：会话 status 为 `running`，metadata 有 `current_message_id`，消息表只有 user message，工具调用表已有同 message id 的工具结果
- 操作：调用 `openSession`
- 验证：前端合成 assistant timeline，顺序为 `正在调用工具: A → A 工具卡 → 正在调用工具: B → B 工具卡`，刷新后工具结果仍可见

## 验证步骤

1. `cd api-server && go test ./internal/assistant`
2. `cd api-server && go test ./cmd`
3. `cd frontend && npm run test -- src/store/assistant.test.ts src/views/assistant/components/AssistantConversation.test.ts --run`
4. `cd frontend && npm run build`
5. 手动验证：发送多步工具调用任务，刷新页面，检查消息顺序

## 风险与回滚计划

- **风险**: 中。修改涉及 assistant runtime 可见事件出口、memory experience 和前端历史重建。
- **兼容**: 新会话优先使用 metadata 事件流；旧会话仍使用 thinking/tool_calls 降级重建。
- **回滚**: 恢复 `HookModelCallFinished` 可见输出、恢复反思 thinking 发布、移除 `assistant_runtime_events` 优先重建。
