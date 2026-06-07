# 智能体模式分页、运行时工具注入和页面体验修复

## Bug 描述与症状

1. 历史会话分页不显示：会话总数超过第一页容量时，侧栏分页控件没有稳定出现。
2. 智能体复杂会话报工具不存在：模型尝试调用 `Detection.Alert.List`、`Agent.Process.List` 时，agent-runtime 当前运行的工具注册表中没有这些工具。
3. 模型调用超时和解析异常后，ReAct 循环继续重试并触发上限。
4. 智能体工作台页面缺少进入和状态变化动效，交互反馈偏硬。

## 根因分析

分页问题发生在 `AssistantWorkspace.vue -> AssistantSessionSidebar.vue`。Pinia setup store 暴露到组件实例后会自动解包 ref，模板中使用 `store.sessionTotal.value` 会得到 `undefined`，导致子组件收到的 `total` 为空，`total > 10` 条件不成立。

工具不存在发生在 `Orchestrator.Run -> ToolSelector.Select -> buildAgentToolDescriptors -> RuntimeFactory.Build -> agentruntime.WithTools`。Aegis 的 `ToolRegistry` 已经注册了相关工具，但 agent-runtime 只知道本轮 `WithTools(...)` 注入的描述符。复杂安全任务中，计划或 `Tool.Search` 可能提到未注入的工具，随后模型直接调用这些工具时，agent-runtime 内部注册表会报“工具不存在”。

超时和解析异常来自模型调用链路。智能体模式当前使用 `MaxTotalTurns=80`、`TaskTimeout=30m`，而 AI 分析路径使用 `MaxTotalTurns=500`、`TaskTimeout=2h`，并启用上下文压缩、反思、审计和纠错。复杂任务遇到模型超时或异常字符时，更容易因为工具缺失和轮数偏小叠加失败。

## 修复设计

1. 前端分页：继续使用 `store.sessionTotal`、`store.hasMoreSessions` 这类 Pinia 解包值传参；搜索分页时保留关键词，页码变化时带上当前筛选条件。
2. 前端 GSAP：在 Vue `onMounted/onUnmounted` 中用 `gsap.context` 绑定工作台根节点，添加侧栏、对话区、上下文栏和欢迎区的轻量入场动画，并通过 `matchMedia` 尊重 `prefers-reduced-motion`。
3. 后端运行时配置：新增 AI 分析对齐配置入口，复杂任务使用 500 轮、2 小时任务超时、100 次工具调用、反思/审计/纠错/上下文压缩配置。
4. 后端工具注入：复杂安全任务在初始工具选择基础上追加安全分析常用只读工具，包括 `Detection.Alert.List`、`Detection.Alert.Get`、`Detection.Statistics.Get`、`Agent.Process.List`、`Agent.Process.Tree`、`Agent.Network.List`、`Agent.File.OpenList`、`Agent.Log.Query`，保证模型计划中出现的关键工具已进入 agent-runtime 当前运行注册表。
5. 日志与可观测性：运行时构建阶段记录实际工具数量、配置 profile 和缺失工具名，便于排查后续模型误调用。

## 回归测试用例

1. 前端分页：模拟 `getSessions` 返回 10 条数据、`total=12`，断言侧栏收到 `total=12` 且 `hasMore=true`。
2. 前端分页切换：触发 `page-change=2`，断言请求 `page=2&page_size=10`，且不丢失搜索关键词。
3. 后端复杂任务工具注入：构造 detection/investigation 意图，断言扩展后的描述符包含 `Detection.Alert.List` 和 `Agent.Process.List`。
4. 后端配置对齐：复杂任务构建 runtime 时使用 AI 分析 profile，断言 `MaxTotalTurns=500`、`TaskTimeout=2h`、`EnableContextCompress=true`、`EnableReflection=true`。
5. 构建验证：运行 `go test ./internal/assistant ./internal/llm/adapters` 和前端相关 Vitest，最后执行前端构建。

## 风险与回滚

风险主要是复杂任务初始工具集变大，提示词 token 占用略增；只追加只读或默认白名单查询工具，不扩大写操作权限。回滚时恢复 `RuntimeFactory` 配置选择和 `ToolSelector` 扩展逻辑，并移除 `AssistantWorkspace.vue` GSAP 动画代码。

## 实际验证结果

验证时间：2026-06-07。

1. `go test ./internal/assistant ./internal/llm/adapters`：通过。
2. `npm run test -- AssistantWorkspace.test.ts --run`：通过。
3. `cd api-server && make build`：通过。
4. `cd frontend && npm run build`：通过，保留现有 Vite 大 chunk 警告。
5. `docker compose up -d --build api-server frontend`：通过，两个容器重建并健康启动。
6. `GET /health`：返回 `{"status":"ok"}`。
7. 使用管理员账号登录后调用 `GET /api/v1/assistant/sessions?page=1&page_size=10`：返回 10 条会话，`total=29`。
8. 调用 `GET /api/v1/assistant/sessions?page=2&page_size=10`：返回 10 条会话，`total=29`。
9. 调用 `GET /api/v1/assistant/sessions?page=1&page_size=10&keyword=查看`：返回 3 条会话，`total=3`。
10. 分别调用 `GET /api/v1/assistant/tools?keyword=Detection.Alert.List` 和 `GET /api/v1/assistant/tools?keyword=Agent.Process.List`：确认两个工具的 `tool_name` 均存在。

限制：`npm run type-check` 在当前 Node.js `v22.22.2` 环境下启动失败，错误为 `vue-tsc` 内部 `Search string not found: "/supportedTSExtensions = .*(?=;)/"`，未进入项目代码类型检查阶段。
