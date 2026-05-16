# AI 分析历史会话 UI 修复设计文档

## 概述

修复 AI 分析功能中 5 个 UI 问题，涉及历史会话加载、结论显示、执行计划状态、分析事件展示和思考内容格式化。

## 问题分析

### 问题 1：历史会话结论 UI 缺失

**根因：** `loadExecutionResultForSession()` 调用 `GET /:session_id/execution-result` 接口，如果该接口返回 404（agent-runtime 执行记录不存在），catch 块静默返回 null。此时 `executionResult.value` 为 null，`TaskExecutionResult` 组件不会渲染。

历史会话的 `GET /:session_id/history` 接口已经返回了 `conclusion` 字段，但前端从未使用它来构建 `executionResult`。

**修复方案：** 在 `loadSession` 中，当 `loadExecutionResultForSession` 返回 null 时，使用 history API 返回的 `conclusion` 字段构建 fallback `ExecutionResult`，确保 `TaskExecutionResult` 组件能正确渲染。

### 问题 2：已完成会话状态显示"未完成"

**根因：** `getDisplayStatus()` 检查 `session.conclusion && Object.keys(session.conclusion).length > 0`。如果后端 `persistAnalysisOutcome()` 未能解析最终答案（`extractFinalAnswerResult` 返回 error），conclusion 字段为 null，状态显示为"未完成"。

同时，session list 接口返回的 `conclusion` 可能是 JSONB 字符串而非已解析的对象，导致前端 `Object.keys()` 检查失败。

**修复方案：**
- 后端：在 `GetSessionList` 中确保 conclusion 正确序列化
- 前端：`getDisplayStatus` 增加对字符串类型 conclusion 的处理

### 问题 3：已完成会话执行计划步骤状态

**根因：** 加载历史会话时，执行计划从 `payload.execution_plan` 直接加载，步骤状态保持为流式传输时的最终状态。对于已完成会话，部分步骤可能仍显示为 "pending" 或 "running"（如果流式传输中断但会话最终完成）。

**修复方案：** 对于已完成会话，加载执行计划后将所有步骤状态强制设为 "completed"（除非已经是 "failed"）。

### 问题 4：分析事件显示 "undefined"

**根因：** `timelineEvents` 计算属性中，审计事件使用 `a.decision` 和 `a.findings`，但 API 返回的数据结构中这些字段可能不存在或命名不同。当字段为 undefined 时，模板渲染为 "undefined" 字符串。

**修复方案：** 为所有 timeline 事件的字段访问添加 fallback 值。

### 问题 5：思考内容中的 JSON 直白显示

**根因：** `msg.thought` 使用 `{{ msg.thought }}` 直接渲染为纯文本。当 thinking 内容包含 JSON 时，显示为未格式化的原始文本。

**修复方案：** 检测 thought 内容中的 JSON，使用语法高亮和格式化显示。

## 修改文件清单

| 文件 | 修改内容 |
|------|----------|
| `frontend/src/views/detection/AIAnalysis.vue` | 问题 1, 3, 5 的修复 |
| `frontend/src/components/ExecutionPlan.vue` | 问题 3, 4 的修复 |
| `frontend/src/utils/sessionStatus.ts` | 问题 2 的修复 |
| `frontend/src/components/ThoughtDisplay.vue` | 问题 5 新建组件 |

## 测试计划

1. 使用 curl 测试 session list API 验证 conclusion 字段正确返回
2. 使用 curl 测试 history API 验证 conclusion 字段格式
3. 前端单元测试验证 `getDisplayStatus` 对各种 conclusion 格式的处理
4. 前端单元测试验证 timeline 事件 fallback
5. 编译验证
