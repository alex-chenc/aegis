# AI 分析历史会话 UI 修复设计文档

## 概述

修复 AI 分析功能中 6 个 UI 问题，涉及历史会话加载、结论显示、执行计划状态、分析事件展示、思考内容格式化和结论 UI 重复渲染。

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

### 问题 6：历史会话加载后结论 UI 重复渲染

**现象：** 加载历史 AI 分析会话后，出现 2 套结论 UI，一套显示"可疑"（suspicious），一套显示"恶意"（malicious）。

**根因：** `loadSession` 流程中，执行结果附加顺序导致重复渲染：

1. `applyParsedExecutionResultFromContent()` 解析内容文本，前端 `normalizeVerdict` 匹配到步骤结果中的"可疑"子串（如"可疑文件系统路径"），创建 verdict="可疑" 的 `ExecutionResult`，附加到消息 N。此时消息 N 是最后一条消息。
2. `appendHistoryRuntimeEventMessages()` 在消息 N 之后追加审计和纠正消息（带 `type` 字段）。
3. `loadExecutionResultForSession()` 从 API 获取 verdict="恶意" 的结果，调用 `attachExecutionResultToLatestMessage`。
4. **Bug:** `findLatestFinalAssistantMessageIndex()` 只检查 `messages.value[messages.value.length - 1]`（最后一条消息）。最后一条是纠正消息（`type: 'correction'`），不满足 `isFinalAssistantMessage` 条件（因为 `!msg.type` 为 false），返回 -1。
5. `attachExecutionResultToLatestMessage` 走 else 分支，PUSH 一条新消息，只含 `executionResult`。

**结果：** 两条消息都有 `executionResult`，都渲染 `TaskExecutionResult` 组件。

**次要问题：** 前端 `normalizeVerdict` 先检查"可疑"再检查"恶意"，与后端的严重度优先策略不一致。当文本同时包含两个关键词时，前端返回"可疑"，后端返回"恶意"。

**修复方案：**
- **Fix 6a:** `findLatestFinalAssistantMessageIndex` 改为从后向前遍历，找到正确的 final assistant message
- **Fix 6b:** `normalizeVerdict` 调整检查顺序，"恶意"优先于"可疑"，与后端一致
- **Fix 6c:** 从 `loadSession` 流程中移除不必要的 `applyParsedExecutionResultFromContent` 调用（API 调用会提供权威结果）

### 问题 7：历史会话加载后左侧告警事件未显示

**现象：** 加载历史 AI 分析会话后，左侧面板不显示该会话分析的告警事件。

**根因：** `GetSessionHistory` API 不返回告警数据，前端 `loadSession()` 中 `analysisAlertSnapshot` 被清空后从未重新填充。`isAnalysisSnapshotActive` 为 false，左侧面板显示空的实时搜索结果。

**修复方案：**
- **Fix 7a:** 后端 `GetSessionHistory` 增加 `alerts` 字段，DB 路径通过 `alertRepo.FindByIDs` + `buildAlertSnapshots` 加载，内存路径直接使用 `session.AlertSnapshots`
- **Fix 7b:** 前端 `getSessionHistory` 返回类型增加 `alerts` 字段
- **Fix 7c:** 前端 `loadSession()` 从 `payload.alerts` 填充 `analysisAlertSnapshot`

**附加修复：**
- **Fix 7d:** `normalizeVerdict` 移除死代码 `text.includes('potentially malicious')`（已被 `text.includes('malicious')` 覆盖）

## 修改文件清单

| 文件 | 修改内容 |
|------|----------|
| `frontend/src/views/detection/AIAnalysis.vue` | 问题 1, 3, 5, 6, 7 的修复 |
| `frontend/src/components/ExecutionPlan.vue` | 问题 3, 4 的修复 |
| `frontend/src/utils/sessionStatus.ts` | 问题 2 的修复 |
| `frontend/src/components/ThoughtDisplay.vue` | 问题 5 新建组件 |
| `frontend/src/utils/taskExecutionResult.ts` | 问题 6, 7 的修复（normalizeVerdict 严重度排序 + 移除死代码） |
| `frontend/src/utils/taskExecutionResult.test.ts` | 问题 6 的单元测试 |
| `api-server/internal/api/handler/ai_analysis_handler.go` | 问题 7 的修复（GetSessionHistory 增加 alerts 字段） |
| `api-server/internal/api/handler/ai_analysis_handler_test.go` | 问题 7 的单元测试 |
| `frontend/src/api/aiAnalysis.ts` | 问题 7 的修复（getSessionHistory 返回类型增加 alerts） |
| `frontend/src/api/aiAnalysis.test.ts` | 问题 7 的单元测试 |

## 测试计划

1. 使用 curl 测试 session list API 验证 conclusion 字段正确返回
2. 使用 curl 测试 history API 验证 conclusion 字段格式
3. 前端单元测试验证 `getDisplayStatus` 对各种 conclusion 格式的处理
4. 前端单元测试验证 timeline 事件 fallback
5. 编译验证
6. `normalizeVerdict` 单元测试：文本同时包含"可疑"和"恶意"时返回"恶意"（严重度优先）
7. `normalizeExecutionResult` 单元测试：步骤结果含"可疑"子串但 API 结论为"恶意"时，最终 verdict 为"恶意"
8. `findLatestFinalAssistantMessageIndex` 验证：在 runtime event 消息之后仍能找到 final assistant message
9. 加载会话 `06b89c4b-4c1e-4a74-bf57-b6fe3e18e57e` 验证只显示一个结论 UI（"恶意"）
10. 后端单元测试：`TestGetSessionHistoryReturnsAlerts` 验证内存路径返回 alerts 字段
11. 后端单元测试：`TestGetSessionHistoryReturnsEmptyAlertsWhenDeleted` 验证告警删除后返回空数组
12. 前端单元测试：`getSessionHistory API` 验证 alerts 字段类型和结构
13. curl 测试：加载会话 `06b89c4b-4c1e-4a74-bf57-b6fe3e18e57e` 验证 alerts 字段正确返回
14. `normalizeVerdict` 单元测试：移除 `potentially malicious` 死代码后测试仍通过
