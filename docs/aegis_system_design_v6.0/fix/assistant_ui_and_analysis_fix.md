# Assistant UI 和分析能力修复设计文档

## 问题描述

用户反馈了 4 个问题：
1. **历史会话分页不显示** — 智能体模式下，历史会话列表的分页组件应该在最下面一行显示，但目前页面没有任何显示
2. **上下文对象替换为计划模块** — 右侧"上下文对象"栏没有实际用途，需要删除并替换为"执行计划"模块（参考 AI 分析模块的实现）
3. **工具调用状态不变更** — 工具调用记录显示正常，但执行状态不会从 running 变为 completed/failed
4. **安全事件分析质量差** — 进行安全事件分析时，没有查询安全事件信息，也没有调用 Agent 工具进行深入调查

## 根因分析

### 问题 1：历史会话分页不显示

**根因**：分页组件的 `v-if="total > 0"` 条件依赖 `total` prop，但后端 API 返回格式与前端期望不匹配。

后端返回格式：
```json
{ "code": 0, "data": { "sessions": [...], "total": N } }
```

Axios 拦截器解包 `response.data.data`，返回 `{ sessions: [...], total: N }` 给 store。

Store 代码：
```typescript
const items = result?.sessions || result?.items || []
const total = result?.total || 0
```

**问题**：当 sessions 为空数组时，`sessions.length === 0`，模板进入 `empty-state` 分支，分页组件被跳过。即使 `total > 0`，分页也不会显示。

**修复**：将分页组件移到 `session-items` 外部，确保只要有数据（`total > 0`）就显示分页。

### 问题 2：上下文对象替换为计划模块

**根因**：右侧 `AssistantContextRail.vue` 显示"上下文对象"、"待审批动作"、"工具调用记录"三个区域。其中"上下文对象"区域显示会话关联的上下文引用（主机、告警等），但这些信息对用户价值不大。

**修复方案**：
1. 删除"上下文对象"区域
2. 新增"执行计划"区域，显示当前会话的最新执行计划
3. 计划数据来自 store 中的 `messages`，查找最新一条包含 `plan` 字段的助手消息
4. 复用 `ExecutionPlan.vue` 组件渲染计划

### 问题 3：工具调用状态不变更

**根因**：SSE 事件处理逻辑正确更新了 store 中的 `toolCalls` 数组，但 `AssistantConversation.vue` 渲染工具调用卡片时使用的是 `msg.tool_calls`（消息级别的工具调用），而不是 store 级别的 `toolCalls`。

当 SSE `tool_call` 事件到达时，store 将工具调用添加到 `toolCalls` 数组。当 `tool_result` 事件到达时，store 更新 `toolCalls` 数组中对应条目的状态。但 `AssistantConversation.vue` 的模板：
```vue
<div v-if="msg.tool_calls?.length" class="tool-calls">
  <AssistantToolCallCard v-for="call in msg.tool_calls" ... />
</div>
```

这里渲染的是 `msg.tool_calls`，而不是 store 的 `toolCalls`。消息级别的 `tool_calls` 在消息创建时就固定了，不会随 SSE 事件更新。

**修复**：在 `AssistantConversation.vue` 中，将工具调用卡片的渲染改为使用 store 传入的 `toolCalls` prop，按 `message_id` 过滤当前消息的工具调用。

### 问题 4：安全事件分析质量差

**根因**：`prompt_fragments.go` 中的 `security_analysis` 提示词片段过于笼统，没有明确告诉 LLM 应该使用哪些工具：

```
## 安全分析规范
1. 收集证据：获取进程树、网络连接、文件操作等数据
2. 分析攻击路径：识别攻击入口、横向移动、提权等行为
3. 评估影响：确定受影响范围和严重程度
4. 提出建议：给出修复和防护建议
```

LLM 不知道应该调用 `Detection.Alert.List` 查询告警、调用 `Agent.Process.List` 获取进程信息、调用 `Agent.Network.Connections` 获取网络连接等。

**修复**：增强 `security_analysis` 提示词片段，明确列出安全分析应使用的工具和调用顺序。

## 修复设计

### 修复 1：历史会话分页显示

**修改文件**：`frontend/src/views/assistant/components/AssistantSessionSidebar.vue`

**改动**：
1. 将分页组件从 `session-items` 内部移到外部
2. 分页显示条件改为 `total > pageSize`（超过一页才显示分页）
3. 确保分页始终在会话列表底部显示

```vue
<!-- 会话列表 -->
<div class="session-items">
  <div v-for="session in sessions" ...>
    ...
  </div>
</div>

<!-- 分页（独立于 session-items） -->
<div v-if="total > 10" class="session-pagination">
  <el-pagination ... />
</div>
```

### 修复 2：上下文对象替换为计划模块

**修改文件**：
- `frontend/src/views/assistant/components/AssistantContextRail.vue`
- `frontend/src/views/assistant/AssistantWorkspace.vue`

**改动**：
1. `AssistantContextRail.vue`：
   - 删除"上下文对象"区域
   - 新增"执行计划"区域，使用 `ExecutionPlan.vue` 组件
   - 接收 `plan` prop（从 store 中最新助手消息的 plan 字段获取）
   - 保留"待审批动作"和"工具调用记录"区域

2. `AssistantWorkspace.vue`：
   - 从 store 中提取最新的计划数据
   - 传递给 `AssistantContextRail`

**数据流**：
```
store.messages → 最新助手消息的 plan 字段 → AssistantContextRail.plan → ExecutionPlan
```

### 修复 3：工具调用状态同步

**修改文件**：`frontend/src/views/assistant/components/AssistantConversation.vue`

**改动**：
1. 接收 store 级别的 `toolCalls` prop
2. 在渲染工具调用卡片时，使用 store 的 `toolCalls` 按 `message_id` 过滤
3. 确保 SSE 事件更新 store 的 `toolCalls` 后，对应的卡片状态同步更新

```vue
<!-- 修改前 -->
<div v-if="msg.tool_calls?.length" class="tool-calls">
  <AssistantToolCallCard v-for="call in msg.tool_calls" ... />
</div>

<!-- 修改后 -->
<div v-if="getMessageToolCalls(msg).length" class="tool-calls">
  <AssistantToolCallCard v-for="call in getMessageToolCalls(msg)" ... />
</div>
```

```typescript
function getMessageToolCalls(msg: AssistantMessage): AssistantToolCall[] {
  // 优先使用 store 级别的 toolCalls（SSE 实时更新）
  const storeCalls = props.toolCalls.filter(tc =>
    tc.message_id === msg.message_id || tc.message_id === msg.id
  )
  if (storeCalls.length > 0) return storeCalls
  // 降级使用消息级别的 tool_calls（历史数据）
  return msg.tool_calls || []
}
```

### 修复 4：安全事件分析提示词增强

**修改文件**：`api-server/internal/assistant/prompt_fragments.go`

**改动**：增强 `security_analysis` 提示词片段，明确列出安全分析应使用的工具：

```
## 安全分析规范

### 第一步：收集安全事件
- 使用 Detection.Alert.List 查询最近的告警事件
- 使用 Detection.Alert.Get 获取告警详情
- 使用 Detection.Statistics.Get 获取告警统计信息

### 第二步：收集主机信息
- 使用 Host.List 查询相关主机
- 使用 Host.Get 获取主机详情
- 使用 Host.AgentStatus.Get 检查 Agent 状态

### 第三步：深入调查（需要 Agent 在线）
- 使用 Agent.Process.List 获取进程列表
- 使用 Agent.Network.Connections 获取网络连接
- 使用 Agent.File.Activity 获取文件操作记录
- 使用 Agent.User.Login 获取用户登录记录

### 第四步：分析攻击路径
- 识别攻击入口（初始访问）
- 追踪横向移动行为
- 检测提权行为
- 评估数据泄露风险

### 第五步：评估影响和建议
- 确定受影响范围和严重程度
- 给出修复和防护建议
- 提供检测规则建议

分析时应基于实际数据，不要推测或编造信息。
每一步都应调用相应工具获取数据，不要跳过数据收集步骤。
```

## 影响范围

| 组件 | 文件 | 改动类型 |
|------|------|----------|
| Frontend | `AssistantSessionSidebar.vue` | 分页位置调整 |
| Frontend | `AssistantContextRail.vue` | 替换上下文对象为计划模块 |
| Frontend | `AssistantWorkspace.vue` | 传递计划数据 |
| Frontend | `AssistantConversation.vue` | 工具调用状态同步 |
| Backend | `prompt_fragments.go` | 增强安全分析提示词 |

## 测试用例

### 测试 1：分页显示
1. 创建超过 10 个会话
2. 验证分页组件在会话列表底部显示
3. 点击分页验证翻页功能

### 测试 2：计划模块显示
1. 发送一个复杂任务（如"分析最近的安全事件"）
2. 验证右侧栏显示执行计划
3. 验证计划步骤状态随执行更新

### 测试 3：工具调用状态更新
1. 发送一个需要工具调用的任务
2. 观察工具调用卡片从 running 变为 completed/failed
3. 验证状态变化实时反映在 UI 上

### 测试 4：安全事件分析质量
1. 发送"分析最近的安全事件"
2. 验证 LLM 调用 Detection.Alert.List 查询告警
3. 验证 LLM 调用 Agent 工具进行深入调查
4. 验证分析结果包含具体的安全事件数据

## 回滚计划

如果修复引入新问题：
1. 分页问题：恢复原始分页组件位置
2. 计划模块：恢复上下文对象区域
3. 工具调用状态：恢复使用 msg.tool_calls
4. 提示词：恢复原始 security_analysis 提示词
