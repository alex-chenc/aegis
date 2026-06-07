# Assistant 对话刷新与 UI 精细化修复设计文档

## Bug 描述和症状

用户在智能模式对话中反馈以下问题：

1. 工具调用标题和工具执行结果应放在同一个工具框内，目前视觉上容易被拆散或与其它内容混在一起。
2. 每个对话框都应该有头像，当前一条 assistant 消息内的多个思考、工具、结果块共用一个头像。
3. 点击页面刷新后，思考框顺序丢失，多个思考步骤挤到一个框里。
4. 整个任务完成后刷新页面，右侧执行计划状态回退为“待执行”。
5. 新建会话输入框直角感明显，需要改成圆角并优化细节。
6. 工具执行结果中的 JSON 需要格式化显示，便于排查和阅读。

## 复现步骤

1. 使用 `admin / Admin@123` 登录系统。
2. 进入 `/assistant` 智能模式。
3. 发送一个会调用多个工具的安全分析任务。
4. 等待任务执行完成。
5. 刷新浏览器页面。
6. 观察对话区和右侧执行计划：
   - thinking 历史恢复为一整块。
   - 工具调用与思考/结果之间的层次不清晰。
   - 执行计划步骤状态可能显示为 pending/待执行。
   - JSON 结果不易阅读。

## 根因分析

### 1. V6.0 持久化模型与前端展示模型不一致

V6.0 设计中，`assistant_messages` 保存智能体过程消息，`assistant_tool_calls` 保存工具调用。历史接口返回时，一次运行的 assistant 输出可能仍是一条持久化消息：

```text
assistant_messages.thinking = [思考1, 正在调用工具: Host.List, 思考2, ...]
assistant_messages.content  = 最终报告
assistant_tool_calls        = Host.List / Detection.Alert.List / ...
```

实时 SSE 阶段前端可以按事件流拆分显示，但刷新后只有持久化快照，必须重新从 `thinking` 标记和工具调用记录中还原展示顺序。

### 2. 当前历史重建仍会合并连续 thinking

现有 `rebuildAssistantHistoryCycles()` 会把连续的非工具 thinking 聚合到同一个展示消息中。刷新后如果多个思考步骤在同一工具调用之前或之后，会重新挤进一个“思考”框，破坏用户期望的“一个思考框跟一个结果/执行框”的节奏。

### 3. 消息内部块共用头像

`AssistantConversation.vue` 当前以 `AssistantMessage` 为头像粒度，一条消息内的 thinking、content、tool_calls、result_cards 都渲染在同一个 `.message-body` 下。因此即使视觉上是多个框，也只有一个头像。

### 4. 计划状态只在实时态更新，刷新缺少兜底

实时 `done` 事件会在前端将当前计划标记为 completed；但后端历史消息里的 plan 可能仍保留初始 pending 状态。刷新后 `currentPlan` 从历史消息重新提取 plan，导致右侧计划显示“待执行”。

### 5. 工具结果按普通文本显示

工具结果可能是对象、数组或 JSON 字符串。当前组件主要按文本展示，虽然对象会 `JSON.stringify`，但缺少 JSON 视觉样式和 JSON 字符串解析。

## 修复设计

### 设计原则

依据 V6.0 前端设计文档，智能模式工作台重点展示“任务计划、工具调用、审批、上下文对象、业务结果”的安全运营工作流。本次修复不修改 API 和数据库结构，只在前端恢复历史展示状态并优化 UI。

### 1. 历史消息重建

修改 `frontend/src/store/assistant.ts`：

1. 继续从 `assistant_messages` 和 `assistant_tool_calls` 合并展示数据。
2. 对历史 assistant 消息按原始 thinking 顺序逐条生成展示片段。
3. 遇到 `正在调用工具: <tool_name>` 标记时，按工具名称匹配对应 `AssistantToolCall`，生成单独工具展示消息。
4. 每个非工具 thinking 步骤生成独立 thinking 展示消息，避免刷新后挤到一块。
5. 未匹配到 thinking 标记的工具调用按 `created_at` 顺序补充到消息末尾，避免丢失工具记录。
6. 最终 `content / plan / approvals / result_cards / context_refs` 生成结果展示消息。

### 2. 计划状态刷新兜底

在历史消息归一化阶段，根据当前会话状态修正 plan：

- `completed` 会话：将 plan.status 修正为 `completed`，并将仍处于 `pending/running` 的步骤修正为 `completed`。
- `failed` 会话：将 plan.status 修正为 `failed`，并将 `pending/running` 步骤修正为 `failed`。
- `cancelled` 会话：将 plan.status 修正为 `cancelled`，并将 `pending/running` 步骤修正为 `cancelled`。

该兜底只影响前端历史展示，不回写数据库。

### 3. 对话框分段渲染与头像

修改 `frontend/src/views/assistant/components/AssistantConversation.vue`：

1. 将 assistant 消息转换为展示段：
   - thinking 段
   - model content 段
   - tool call 段
   - approval 段
   - step result 段
   - result card 段
2. 每个展示段都渲染为独立 `.message.assistant` 行，拥有自己的头像。
3. 工具段内同时展示工具名称、状态、执行结果和折叠按钮，确保工具和工具结果在同一个框内。

### 4. JSON 工具结果格式化

工具结果显示规则：

1. `error_message` 优先显示为普通文本。
2. `result` 为对象或数组时，使用 `JSON.stringify(result, null, 2)`。
3. `result` 或 `result_summary` 为 JSON 字符串时，先解析再格式化。
4. JSON 结果使用等宽字体、浅色背景和横向滚动，长内容继续保留折叠/展开能力。

### 5. 输入框圆角和 UI 精细化

修改 `AssistantComposer.vue` 和 `AssistantWorkspace.vue`：

1. Composer 外层改为圆角容器。
2. Textarea 使用圆角、浅色背景和聚焦态边框。
3. 底部输入框与面板边缘留出间距。
4. 新建会话居中输入框保持圆角卡片形态。

## 影响范围

| 组件 | 文件 | 改动类型 |
| --- | --- | --- |
| Frontend Store | `frontend/src/store/assistant.ts` | 历史消息重建、计划状态兜底 |
| Frontend UI | `frontend/src/views/assistant/components/AssistantConversation.vue` | 分段头像、工具结果同框、JSON 格式化 |
| Frontend UI | `frontend/src/views/assistant/components/AssistantComposer.vue` | 输入框圆角与视觉优化 |
| Frontend UI | `frontend/src/views/assistant/AssistantWorkspace.vue` | Composer 布局间距 |
| Tests | `frontend/src/store/assistant.test.ts` | 历史顺序和计划状态回归测试 |
| Tests | `frontend/src/views/assistant/components/AssistantConversation.test.ts` | 分段头像、工具同框、JSON 格式化测试 |

## 回归测试用例

### 测试 1：刷新历史后保持思考/工具顺序

输入一条历史 assistant 消息：

```text
思考 A
正在调用工具: Host.List
思考 B
正在调用工具: Detection.Alert.List
思考 C
最终报告
```

预期展示顺序：

```text
思考 A -> Host.List 工具框 -> 思考 B -> Detection.Alert.List 工具框 -> 思考 C -> 最终报告
```

### 测试 2：每个 assistant 展示框都有头像

构造一条包含两个 thinking、一个 content、一个 tool_call 的 assistant 消息。

预期：

- 页面存在 4 个 assistant 展示行。
- 每个展示行都包含 `.message-avatar`。

### 测试 3：工具和工具结果在同一框

构造一个 `Host.List` 工具调用，结果为 `{ "total": 2 }`。

预期：

- 工具名称和 `"total": 2` 位于同一个 `.tool-call-card` 中。
- 不出现单独的工具结果消息框。

### 测试 4：完成会话刷新后计划状态不回退

构造 `currentSession.status = completed`，历史消息 plan 中步骤为 pending。

预期：

- 刷新归一化后 plan.status 为 completed。
- pending/running 步骤显示为 completed。

### 测试 5：JSON 工具结果格式化

构造工具结果为 JSON 字符串或对象。

预期：

- JSON 使用缩进格式显示。
- JSON 结果区域带 `is-json` 样式。
- 超长 JSON 默认折叠，可点击展开。

### 测试 6：输入框圆角

进入无会话欢迎页和已有会话底部输入区。

预期：

- 输入框外层和 textarea 均为圆角。
- 底部输入框与边缘有稳定间距。

## 验证步骤

1. 执行前端单测：
   ```bash
   cd frontend
   npx vitest run src/store/assistant.test.ts src/views/assistant/components/AssistantConversation.test.ts
   ```
2. 执行前端生产构建：
   ```bash
   cd frontend
   npm run build
   ```
3. 重建前端容器：
   ```bash
   docker compose up -d --build frontend
   ```
4. 使用 `admin / Admin@123` 登录，进入 `/assistant`。
5. 创建或打开一个多工具调用会话，刷新页面后检查：
   - 思考、工具、结果顺序正确。
   - 每个框有头像。
   - 工具和结果同框。
   - 执行计划状态不回退。
   - JSON 格式化显示。

## 风险和回滚计划

### 风险

- 历史展示消息 ID 是前端派生 ID，不应回传给后端写入。
- 对非常长的 JSON 结果，格式化后文本可能更长，但折叠逻辑会限制默认渲染长度。

### 回滚

如出现 UI 回归：

1. 回滚 `AssistantConversation.vue` 的分段渲染。
2. 回滚 `assistant.ts` 的历史重建策略。
3. 保留后端和数据库不变，不需要迁移回滚。
