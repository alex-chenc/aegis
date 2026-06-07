# Bug Fix: 智能助手 SSE 事件处理、会话状态持久化与 agent-runtime 集成

## Bug 描述与症状

### Bug 1: 对话输入消息没有返回结果

**症状**：用户在智能助手页面输入消息后，界面没有任何响应。后端实际已处理请求并返回 LLM 响应，但前端未能正确解析和展示。

**影响范围**：所有助手对话功能完全不可用。

### Bug 2: 页面刷新后数据丢失

**症状**：用户刷新页面后，当前对话会话丢失，回到空状态页面。需要手动从左侧栏重新点击会话才能恢复对话。

**影响范围**：所有助手会话的用户体验。

### Bug 3: 后端未接入 agent-runtime 框架

**症状**：后端 Orchestrator 直接调用 LLM，没有使用 agent-runtime 框架的 Plan → ReAct → Tool Dispatch 循环。工具调用基础设施（ToolDispatcher/ToolGateway）存在但未连接，导致：
- 没有执行计划生成
- 没有工具调用循环
- 没有反思/审计/纠正机制
- LLM 只做简单的单轮问答，无法完成复杂的安全分析任务

**影响范围**：智能助手的核心能力完全缺失。

## 复现步骤

### Bug 1 复现

1. 登录系统，导航至 `/assistant`
2. 创建新会话或选择已有会话
3. 在输入框输入消息并发送
4. **观察**：用户消息显示后，无助手回复出现

### Bug 2 复现

1. 登录系统，导航至 `/assistant`
2. 创建会话并进行对话
3. 刷新页面（F5 或 Cmd+R）
4. **观察**：页面回到空状态，当前会话丢失

## 根因分析

### Bug 1 根因：SSE 事件格式前后端不匹配

#### 后端事件格式（`api-server/internal/assistant/event.go`）

后端 `AssistantEvent` 结构体：
```go
type AssistantEvent struct {
    Type      string      `json:"type"`
    SessionID string      `json:"session_id"`
    RunID     string      `json:"run_id,omitempty"`
    MessageID string      `json:"message_id,omitempty"`
    Payload   interface{} `json:"payload,omitempty"`
    Error     string      `json:"error,omitempty"`
    Timestamp time.Time   `json:"timestamp"`
}
```

`message_delta` 事件的实际 JSON 输出：
```json
{
  "type": "message_delta",
  "session_id": "asst_xxx",
  "run_id": "run_xxx",
  "payload": {
    "message_id": "msg_xxx",
    "delta": "这是AI的回复内容"
  },
  "timestamp": "2026-06-07T..."
}
```

#### 前端事件处理（`frontend/src/store/assistant.ts`）

```typescript
// 前端 onmessage 处理
eventSource.onmessage = (event) => {
  const data = JSON.parse(event.data)  // data = 完整的 AssistantEvent
  applyStreamEvent(data)                // 传入完整对象
}

// applyStreamEvent 中的处理
function applyStreamEvent(event: { type: string; data: any }) {
  switch (event.type) {
    case 'message_delta': {
      const { message_id, content } = event.data  // ❌ event.data 是 payload 对象
      // 实际上 event.data = { message_id: "msg_xxx", delta: "..." }
      // content 为 undefined！
    }
  }
}
```

#### 不匹配点汇总

| 问题 | 后端实际 | 前端期望 | 后果 |
|------|---------|---------|------|
| 消息内容字段名 | `payload.delta` | `event.data.content` | content 为 undefined，创建空消息 |
| 完成事件类型 | 无类型 data 事件 | `addEventListener('done', ...)` | done 监听器永不触发 |
| 工具调用事件名 | `tool_call`, `tool_result` | `tool_call_start`, `tool_call_completed` | 工具调用事件不识别 |
| 事件数据路径 | `payload` 子对象 | `event.data` 顶层 | 所有事件数据访问错误 |

### Bug 2 根因：无 URL 状态持久化

`AssistantWorkspace.vue` 的 `onMounted` 仅调用 `store.fetchSessions()` 加载会话列表，但不恢复当前会话。`currentSession` 仅存在于 Pinia store 的 `ref()` 中，刷新后丢失。

- 会话 ID 不在 URL 参数中（路由固定为 `/assistant`）
- 会话 ID 不在 `localStorage` 中
- 无自动选择最近会话的逻辑

### Bug 3 根因：Orchestrator 未接入 agent-runtime

`Orchestrator.Run()` 直接调用 `llmClient.ChatCompletionWithMessages()`，绕过了 agent-runtime 框架。V6.0 设计要求复用 `github.com/alex-chenc/agent-runtime`，参考 AI 分析页面的 `adapters.NewAegisRuntime()` + `runtime.Run()` 模式。

现有基础设施已存在但未连接：
- `ToolDispatcher` — 工具调度器（含审批网关）
- `ToolGateway` / `ToolGatewayAdapter` — 工具网关适配器
- `ToolRegistry` — 27 个工具注册
- `ToolSelector` — 意图驱动的工具选择
- `RuntimeConfig` — 运行时配置（max 80 turns, 8 plan steps, etc.）

## 修复设计

### Bug 1 修复：统一 SSE 事件格式

**方案**：修改前端 `applyStreamEvent` 方法，使其正确解析后端 `AssistantEvent` 结构。

#### 前端修改

1. **`applyStreamEvent`**：从 `event.data.payload` 中提取数据，使用正确的字段名
2. **`startStream`**：移除无效的 `addEventListener('done', ...)`，在 `onmessage` 中处理 `done` 事件
3. **事件类型映射**：对齐前后端事件类型名称

#### 事件类型映射表

| 后端事件类型 | 前端处理 |
|-------------|---------|
| `thinking` | 显示思考状态 |
| `message_delta` | 增量更新消息内容（从 `payload.delta` 读取） |
| `intent_detected` | 更新意图显示 |
| `tools_selected` | 更新工具列表 |
| `tool_call` | 添加工具调用记录 |
| `tool_result` | 更新工具调用结果 |
| `tool_error` | 更新工具调用错误 |
| `approval_required` | 添加审批请求 |
| `approval_updated` | 更新审批状态 |
| `context_ref_added` | 添加上下文引用 |
| `result_card` | 显示结果卡片 |
| `done` | 结束流式传输 |
| `error` | 显示错误信息 |

### Bug 2 修复：URL 参数持久化 + 自动恢复

**方案**：使用 URL 查询参数 `?session=xxx` 持久化当前会话 ID。

#### 修改点

1. **路由**：支持 `?session=<session_id>` 查询参数
2. **Workspace**：选择会话时同步更新 URL
3. **Workspace.onMounted**：从 URL 参数恢复会话
4. **会话列表**：高亮 URL 中的会话

## 代码变更

### 文件 1: `frontend/src/store/assistant.ts`

**变更**：重写 `applyStreamEvent` 和 `startStream`

```typescript
// 修改 applyStreamEvent，正确解析后端 AssistantEvent 结构
function applyStreamEvent(event: { type: string; payload?: any; error?: string }) {
  const payload = event.payload || {}
  switch (event.type) {
    case 'thinking':
      // 显示思考状态（可选：添加系统消息）
      break

    case 'message_delta': {
      // 从 payload 中读取 message_id 和 delta
      const { message_id, delta } = payload
      if (!delta) break
      const existing = messages.value.find(m => m.id === message_id)
      if (existing) {
        existing.content += delta
      } else {
        messages.value.push({
          id: message_id,
          session_id: currentSession.value?.session_id || '',
          role: 'assistant',
          content: delta,
          created_at: new Date().toISOString(),
        })
      }
      break
    }

    case 'tool_call': {
      const toolCall: AssistantToolCall = {
        id: payload.call_id,
        session_id: currentSession.value?.session_id || '',
        message_id: '',
        tool_name: payload.tool_name,
        tool_input: payload.args || {},
        status: 'running',
        risk_level: 'readonly',
        created_at: new Date().toISOString(),
      }
      toolCalls.value.push(toolCall)
      break
    }

    case 'tool_result': {
      const idx = toolCalls.value.findIndex(tc => tc.id === payload.call_id)
      if (idx > -1) {
        toolCalls.value[idx].status = 'completed'
        toolCalls.value[idx].tool_output = typeof payload.result === 'string'
          ? payload.result
          : JSON.stringify(payload.result)
      }
      break
    }

    case 'tool_error': {
      const errIdx = toolCalls.value.findIndex(tc => tc.id === payload.call_id)
      if (errIdx > -1) {
        toolCalls.value[errIdx].status = 'failed'
        toolCalls.value[errIdx].tool_output = payload.error
      }
      break
    }

    case 'approval_required': {
      const approval = payload as AssistantApproval
      approvals.value.push(approval)
      if (currentSession.value) {
        currentSession.value.status = 'waiting_approval'
      }
      break
    }

    case 'approval_updated': {
      const resolved = payload as AssistantApproval
      const approvalIdx = approvals.value.findIndex(a => a.id === resolved.id)
      if (approvalIdx > -1) {
        approvals.value[approvalIdx] = resolved
      }
      break
    }

    case 'context_ref_added': {
      const ref = payload as AssistantContextRef
      contextRefs.value.push(ref)
      break
    }

    case 'result_card': {
      // 结果卡片处理
      break
    }

    case 'done': {
      streaming.value = false
      eventSource?.close()
      eventSource = null
      // 刷新会话列表以更新状态
      fetchSessions()
      break
    }

    case 'error': {
      error.value = event.error || payload.message || '助手运行出错'
      streaming.value = false
      eventSource?.close()
      eventSource = null
      break
    }

    default:
      console.warn('未知的 SSE 事件类型:', event.type)
  }
}

// 修改 startStream，移除无效的 addEventListener
function startStream(sessionId: string) {
  stopStream()
  streaming.value = true

  eventSource = createAssistantStream(sessionId)

  eventSource.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data)
      applyStreamEvent(data)
    } catch (err) {
      console.error('解析 SSE 事件失败:', err)
    }
  }

  eventSource.onerror = (event) => {
    console.error('SSE 连接错误:', event)
    streaming.value = false
    eventSource?.close()
    eventSource = null
  }

  // 移除无效的 addEventListener('done', ...)
  // done 事件已在 applyStreamEvent 的 'done' case 中处理
}
```

### 文件 2: `frontend/src/views/assistant/AssistantWorkspace.vue`

**变更**：URL 参数持久化 + 自动恢复会话

```typescript
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()

// 选择会话时同步更新 URL
async function handleSessionSelected(sessionId: string) {
  await store.openSession(sessionId)
  // 更新 URL 参数
  router.replace({ query: { ...route.query, session: sessionId } })
}

// 新建会话后更新 URL
async function handleNewSession(taskType?: AssistantTaskType) {
  // ... 创建会话逻辑 ...
  if (session) {
    router.replace({ query: { ...route.query, session: session.session_id } })
  }
}

onMounted(async () => {
  const auth = getStoredAuth()
  if (!auth) {
    ElMessage.warning('请先登录系统')
    router.replace('/login')
    return
  }

  await store.fetchSessions()

  // 从 URL 参数恢复会话
  const sessionId = route.query.session as string
  if (sessionId) {
    await store.openSession(sessionId)
  }
})
```

### 文件 3: `frontend/src/api/assistant.ts`

**变更**：对齐所有 TypeScript 接口与后端模型字段名

```typescript
// AssistantSession: 添加 session_id, mode_source, created_by 等字段
// AssistantMessage: 添加 message_id, thinking, plan, result_cards 等字段
// AssistantContextRef: 使用 object_type, object_id 替代 ref_type, ref_id
// AssistantToolCall: 使用 call_id, args, result 替代 tool_input, tool_output
// AssistantApproval: 添加 approval_id, title, impact_summary 等字段
// AssistantResultCard: 新增结果卡片类型
// SendMessageRequest: context_refs 使用 {object_type, object_id} 格式
// RunHandle: sendMessage 返回类型修正
// SessionsQueryParams: search → keyword（与后端一致）
```

### 文件 4: `api-server/internal/assistant/adapter_tool_gateway.go` (新增)

**变更**：agent-runtime ToolGateway 适配器

桥接 `agentruntime.ToolGateway` 接口到 `ToolDispatcher`，处理工具调用、审批和回调。

### 文件 5: `api-server/internal/assistant/adapter_hook_sink.go` (新增)

**变更**：agent-runtime HookSink 适配器

将 agent-runtime 的 HookEvent 桥接到 `RunManager.Publish` 的 SSE 事件，覆盖任务生命周期、计划、步骤、工具调用、审计、反思、纠正等所有事件类型。

### 文件 6: `api-server/internal/assistant/adapter_prompt_provider.go` (新增)

**变更**：agent-runtime PromptProvider 适配器

为智能助手生成 Plan/React/Summarize 阶段的提示词，包含可用工具列表和上下文信息。

### 文件 7: `api-server/internal/assistant/orchestrator.go`

**变更**：重写 Orchestrator.Run()，接入 agent-runtime 框架

```go
// 创建 agent-runtime 实例
runtime, err := agentruntime.New(
    agentruntime.WithLLMClient(llmAdapter),
    agentruntime.WithToolGateway(toolGateway),
    agentruntime.WithTools(toolDescriptors),
    agentruntime.WithHooks(hookSink),
    agentruntime.WithPromptProvider(promptProvider),
    agentruntime.WithConfig(runtimeConfig),
)

// 运行 agent-runtime
taskResult, err := runtime.Run(ctx, agentruntime.TaskInput{
    UserInput:   input.UserMessage,
    UserContext: userContext,
    Metadata:    map[string]string{"session_id": input.SessionID},
})
```

### 文件 8: `api-server/cmd/main.go`

**变更**：创建 ToolDispatcher 并传入 OrchestratorDeps

## 验证步骤

### Bug 1 验证

1. 启动服务：`docker compose up -d --build`
2. 登录系统，导航至 `/assistant`
3. 创建新会话，输入 "查看所有主机列表"
4. **预期**：
   - 用户消息立即显示
   - 助手回复逐步出现（或一次性出现）
   - 工具调用记录在右侧面板显示
5. 检查浏览器 DevTools Network 标签，确认 SSE 连接正常
6. 检查 Console 无 JavaScript 错误

### Bug 2 验证

1. 创建会话并发送消息
2. 复制当前 URL
3. 刷新页面
4. **预期**：会话自动恢复，消息历史完整
5. 在新标签页粘贴 URL
6. **预期**：直接打开对应会话

## 受影响组件

| 组件 | 变更类型 | 影响 |
|------|---------|------|
| `frontend/src/store/assistant.ts` | 重写 | SSE 事件处理逻辑 |
| `frontend/src/views/assistant/AssistantWorkspace.vue` | 修改 | URL 参数持久化 |
| `frontend/src/api/assistant.ts` | 类型修正 | 所有接口字段名对齐 |
| `api-server/internal/assistant/orchestrator.go` | 重写 | 接入 agent-runtime 框架 |
| `api-server/internal/assistant/adapter_tool_gateway.go` | 新增 | agent-runtime ToolGateway 适配器 |
| `api-server/internal/assistant/adapter_hook_sink.go` | 新增 | agent-runtime HookSink 适配器 |
| `api-server/internal/assistant/adapter_prompt_provider.go` | 新增 | agent-runtime PromptProvider 适配器 |
| `api-server/cmd/main.go` | 修改 | 创建 ToolDispatcher 并传入依赖 |

## 代码审查额外发现与修复

### CR-1: `SessionsQueryParams.search` 应为 `keyword`

**问题**：`SessionsQueryParams` 接口定义了 `search` 字段，但后端使用 `keyword` 查询参数。
**修复**：将接口字段名从 `search` 改为 `keyword`。

### CR-2: 临时用户消息缺少 `message_id` 字段

**问题**：`sendMessage` 创建的临时用户消息只有 `id` 字段，缺少 `message_id`。`AssistantConversation` 组件使用 `msg.message_id` 作为 Vue `:key`，导致 key 为 `undefined`。
**修复**：临时消息同时设置 `id` 和 `message_id`。

### CR-3: `approval_id` vs `id` 查找不一致

**问题**：审批卡片组件使用 `approval_id` 发射事件，但 store 中 `approveApproval`/`rejectApproval` 用 `id` 查找本地列表。
**修复**：查找时兼容两种 ID：`a.approval_id === approvalId || a.id === approvalId`。

### CR-4: `fetchSessions` 在 `done` 事件中失败会清空会话列表

**问题**：`done` 事件触发 `fetchSessions()`，如果网络失败，`fetchSessions` 会将 `sessions` 设为空数组。
**修复**：移除 `done` 事件中的 `fetchSessions()` 调用，改为直接更新 `currentSession.status`。

### CR-5: 恢复会话失败时未清除 URL 参数

**问题**：`openSession` 失败后，`?session=xxx` 参数仍留在 URL 中，导致每次刷新都重复尝试。
**修复**：失败时调用 `router.replace({ query: {} })` 清除参数。

## 风险与回滚计划

### 风险

- **低风险**：修改仅涉及前端代码，不影响后端服务
- **兼容性**：后端 AssistantEvent 格式未变更，前端仅修正解析逻辑

### 回滚计划

1. 如出现问题，`git revert` 回退前端变更
2. 后端无需回滚
3. 数据库无变更，无数据迁移风险

## 测试用例

### TC-1: 消息发送与接收

| 步骤 | 操作 | 预期结果 |
|------|------|---------|
| 1 | 创建新会话 | 会话创建成功，URL 更新为 `?session=xxx` |
| 2 | 输入 "查看主机列表" 并发送 | 用户消息显示，助手回复出现 |
| 3 | 检查右侧面板 | 工具调用记录显示 |

### TC-2: 页面刷新恢复

| 步骤 | 操作 | 预期结果 |
|------|------|---------|
| 1 | 发送消息后刷新页面 | 页面加载后自动恢复当前会话 |
| 2 | 检查消息列表 | 历史消息完整显示 |
| 3 | 检查 URL | 包含 `?session=xxx` 参数 |

### TC-3: SSE 事件完整性

| 步骤 | 操作 | 预期结果 |
|------|------|---------|
| 1 | 打开 DevTools Network | SSE 连接建立 |
| 2 | 发送消息 | 收到 thinking, message_delta, done 事件 |
| 3 | 检查 Console | 无 "未知的 SSE 事件类型" 警告 |
