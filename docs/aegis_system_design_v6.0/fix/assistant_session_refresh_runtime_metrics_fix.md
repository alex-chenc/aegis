# 智能体会话刷新续跑、消息去重和运行指标展示修复

## Bug 描述与症状

1. 最新智能体会话刷新后，对话中正在生成的内容丢失。
2. 新会话发送 `帮我排查一下192.168.152.159 这个机器上面有哪些安全问题` 时，同一条用户消息会出现两次。
3. 页面刷新或多开同一对话后，运行状态没有稳定恢复；任务不应因前端连接断开而停止，应持续运行直到达到最大轮数、runtime 超时策略或用户手动停止。
4. 智能体对话头部缺少与 AI 分析模块一致的运行指标：最大对话轮数、当前 token 使用量、上下文压缩/预算圆环。

## 复现路径

1. 打开智能体模式，进入新会话状态。
2. 输入 `帮我排查一下192.168.152.159 这个机器上面有哪些安全问题` 并发送。
3. 观察会话历史或刷新页面后重新进入最新会话，可看到同一条用户问题被持久化为两条。
4. 在任务运行中刷新页面，前端只重新拉取数据库消息；已经通过 SSE 推送但尚未最终落库的运行事件和助手内容不会恢复。
5. 查看对话右上角，缺少 AI 分析模块已有的上下文预算/压缩指标。

## 根因分析

消息重复发生在前端 `store.sendMessage` 与后端 `Service.CreateSession/SendMessage` 的组合路径：

```text
AssistantWorkspace.handleSend
  -> store.sendMessage
    -> createSession({ initial_message: content })
      -> Service.CreateSession 创建第一条 user message
    -> apiSendMessage(content)
      -> Service.SendMessage 创建第二条 user message 并启动 run
```

刷新丢内容发生在运行事件链路：

```text
Service.SendMessage
  -> RunManager.Start
  -> Orchestrator.Run
  -> RunManager.Publish
  -> Service.Stream
  -> EventSource
```

`RunManager.Publish` 只推送给当前订阅者，不保留活跃 run 的事件历史。浏览器刷新时旧 EventSource 断开，新页面 `openSession` 只拉取数据库中的消息、工具调用和审批；运行中尚未最终落库的计划、步骤、上下文预算、压缩记录和助手增量内容无法从新连接恢复。

运行不中断问题的另一处根因是 `RunManager.Start` 使用固定 `context.WithTimeout(..., 5*time.Minute)`，覆盖了复杂任务已经对齐 AI 分析的 `TaskTimeout=2h` 和 `MaxTotalTurns=500` 配置。

指标缺失是因为智能体 `AssistantHookSink` 没有像 AI 分析 `SSEHookSink` 那样透传 `HookContextBudgetChecked`、`HookContextCompressed`、`HookContextCompressionFailed`，前端也没有为智能体会话维护这些状态。

## 修复设计

1. 新会话发送去重：前端发送新会话时只创建 session，不再传 `initial_message`；用户消息统一由 `POST /message` 持久化并启动 run。
2. 运行历史重放：`RunManager` 为活跃 run 保留有上限的事件历史；新订阅者连接后先重放历史，再接收后续事件，从而支持刷新和多开对话。
3. 运行生命周期解耦：`RunManager.Start` 使用可取消 context，不再写死 5 分钟超时；最大轮数和任务超时由 agent-runtime 配置控制，用户手动停止走 `CancelRun`。
4. 运行状态恢复：前端 `openSession` 加载到 `running` 会话后自动重新建立 SSE；切到非运行会话时关闭当前 SSE。
5. 指标事件透传：智能体 HookSink 对齐 AI 分析事件协议，发送 `context_budget`、`context_compressed`、`context_compression_failed`。
6. 指标持久化：后端把实际 runtime profile、最大轮数、token 统计、上下文预算、压缩记录写入 `assistant_sessions.metadata`，刷新后可恢复。
7. 头部展示：复用 `ContextBudgetIndicator`，在每个智能体会话右上角展示最大轮数、当前 token 使用量和上下文预算圆环。

## 回归测试用例

1. 新会话发送：断言 `createSession` 请求不包含 `initial_message`，`sendMessage` 只调用一次，避免同一句双写。
2. 运行事件重放：先发布事件再订阅，断言订阅者能收到历史事件；多个订阅者互不影响。
3. 运行上下文：断言新 run context 不带 5 分钟 deadline，由 runtime 配置控制生命周期。
4. SSE 指标事件：构造 context budget/compression hook，断言智能体 HookSink 发布对应事件。
5. 前端恢复：打开 `running` 会话后自动调用 `startStream`，并从 metadata 恢复 token/压缩指标。
6. 构建验证：运行后端聚焦测试、前端智能体组件测试、api-server 构建和 frontend 构建。

## 影响范围

涉及 `api-server/internal/assistant`、`api-server/internal/repository/assistant_session_repo.go`、`frontend/src/store/assistant.ts`、`frontend/src/views/assistant/AssistantWorkspace.vue` 和智能体组件测试。

## 风险与回滚

事件历史只保留活跃 run 内存事件，设置上限避免无限增长；服务重启后仍以数据库持久化消息和 metadata 为准。回滚时恢复 `RunManager` 的无历史订阅逻辑、前端新会话 `initial_message` 行为和智能体头部指标展示即可。

## 实际验证结果

验证时间：2026-06-07。

1. `go test ./internal/assistant ./internal/llm/adapters`：通过。
2. `npm run test -- AssistantWorkspace.test.ts --run`：通过，4 个用例覆盖分页、新会话去重、running 会话重连和头部指标。
3. `cd api-server && make build`：通过。
4. `cd frontend && npm run build`：通过，保留现有 Vite 大 chunk 警告。
5. `docker compose up -d --build api-server frontend`：通过，`aegis-api-server` 和 `aegis-frontend` 均为 healthy。
6. `GET /health`：返回 `{"status":"ok"}`。
7. `HEAD http://localhost:8081/`：返回 `HTTP/1.1 200 OK`。
8. 使用管理员账号创建不带 `initial_message` 的智能体会话后，调用消息列表接口返回 `message_count_after_create=0`，说明第一条用户消息不会在创建会话阶段被提前持久化。
9. 取消态修正后再次执行 `docker compose up -d --build api-server`：通过，`aegis-api-server` 为 healthy；重复执行健康检查和空会话消息数冒烟，结果仍正常。
