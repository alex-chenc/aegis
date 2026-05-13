# AI 分析 SSE 流式输出最终实现契约

**版本**: 5.7
**状态**: 最终实现契约
**适用范围**: `agent-runtime` 驱动的 AI 分析 SSE、暂停/取消、历史会话恢复与前端展示。

## 1. 契约目标

本文档替代早期“修复设计”中的临时事件与展示约定，作为后端、前端、持久化和测试的最终对齐依据。

最终实现必须满足：

1. 思考链展示的是可展示推理摘要与执行过程，不承诺、也不请求或持久化模型隐藏 CoT。
2. 正式 SSE 事件类型固定为 `plan`、`step_started`、`step_completed`、`step_failed`、`tool_call`、`tool_result`、`tool_error`、`audit`、`reflection`、`correction`、`content`、`done`、`error`。
3. `plan` 事件的步骤字段以 `agent-runtime` 的 `step_id`、`title`、`objective`、`suggested_tools` 为唯一可信来源，前端负责规范化为 UI 模型。
4. `content` 代表最终回答，只能在运行结束后处理；UI 完成态以 `done` 和最终计划状态为准。
5. 暂停/取消必须调用显式 `pause`/`cancel` API；只关闭 `EventSource` 只表示断开订阅，不能视为暂停。
6. 历史会话 API 必须返回 `messages`、`execution_plan`、`audits`、`reflections`、`corrections`，用于重建思考、工具调用、观察和计划状态。
7. 实现前先补测试，合入前必须完成自动化验证和 `curl -N` SSE 验证。

## 2. 思考链定义

“思考链”是产品层可展示的执行轨迹，内容包括：

- 计划摘要：目标、步骤标题、步骤目标、建议工具。
- 执行过程：步骤开始/完成/失败、工具调用摘要、工具观察摘要。
- 质量控制过程：审计、反思、纠正的可展示结论和原因。
- 最终回答：运行结束后的结论内容。

思考链不得定义为模型隐藏 Chain-of-Thought。后端不得通过提示词要求模型输出隐藏 CoT，持久化层不得保存隐藏 CoT，前端文案不得承诺“展示完整思维链”。如果模型返回了自然语言推理说明，后端只应将其作为可展示摘要或执行说明处理，并按 UI/history 展示长度策略做必要裁剪。

## 3. SSE 协议

### 3.1 事件外壳

每条 SSE 使用统一 JSON 外壳：

```json
{
  "type": "plan",
  "session_id": "session-uuid",
  "timestamp": "2026-05-13T00:00:00Z",
  "content": {},
  "metadata": {}
}
```

字段约定：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `type` | 是 | 正式事件类型，必须来自 3.2 列表 |
| `session_id` | 是 | AI 分析会话 ID |
| `timestamp` | 是 | 服务端发送时间，ISO-8601/RFC3339 |
| `content` | 是 | 事件负载；错误事件也必须提供结构化内容 |
| `metadata` | 否 | 调试、耗时、版本等非关键字段，前端不能依赖其完成核心状态 |

### 3.2 正式事件类型

| 事件类型 | 触发时机 | UI/状态语义 |
| --- | --- | --- |
| `plan` | `agent-runtime` 创建或纠正计划后 | 初始化或替换执行计划，所有步骤默认为 `pending`，再按已知事件回放状态 |
| `step_started` | 步骤开始执行 | 将对应步骤置为 `running`，记录开始时间 |
| `step_completed` | 步骤成功完成 | 将对应步骤置为 `completed`，记录结果摘要/证据 |
| `step_failed` | 步骤失败且未被当前事件修复 | 将对应步骤置为 `failed`，记录错误摘要 |
| `tool_call` | 工具调用开始 | 追加工具调用节点，状态 `running` |
| `tool_result` | 工具调用成功 | 更新工具调用为 `success`，展示观察摘要 |
| `tool_error` | 工具调用失败 | 更新工具调用为 `failed`，展示错误摘要 |
| `audit` | 审计完成 | 追加审计节点，并可影响计划状态 |
| `reflection` | 反思完成 | 追加反思节点，展示可恢复性和建议 |
| `correction` | 计划纠正完成 | 追加纠正节点；如携带新计划，则按 `plan` 规范化并保留历史状态映射 |
| `content` | 运行已进入结束阶段并产出最终回答 | 缓存最终回答，不单独结束 loading |
| `done` | 会话运行终止并已完成最终状态落盘 | 结束 loading，以最终计划状态和最终回答刷新 UI |
| `error` | 协议错误、运行错误或取消/暂停失败 | 展示错误并进入对应终态或可恢复态 |

`thinking` 和 `flowchart_image` 不属于 V5.7 最终 SSE 契约。兼容旧后端时前端可以容忍并忽略这些类型，或将旧 `thinking` 降级为普通过程摘要，但新实现不得依赖它们完成状态流转。

### 3.3 关键负载

#### plan

`plan.content.steps[]` 的权威字段来自 `agent-runtime`：

```json
{
  "plan_id": "plan-uuid",
  "goal": "分析告警攻击链路",
  "version": 1,
  "steps": [
    {
      "step_id": "step-1",
      "title": "收集进程树",
      "objective": "确认可疑进程的父子关系",
      "suggested_tools": ["GetProcessTree"]
    }
  ]
}
```

前端规范化规则：

- `step_id` 规范化为 UI 内部步骤主键；为空时生成稳定 fallback key，但必须在日志中标记协议异常。
- `title` 为空时使用 `objective` 的短摘要；两者都为空时使用 `步骤 {index + 1}`。
- `objective` 为空时使用空字符串，不用标题反填并伪装为真实目标。
- `suggested_tools` 规范化为字符串数组，空值为 `[]`。
- `status` 不从 `plan` 原始字段盲信；新计划初始为 `pending`，再用已收到或历史回放的 step 事件计算最终状态。
- 未识别字段可以保留到 `raw`，但不能参与核心状态判断。

#### step_started / step_completed / step_failed

步骤事件必须携带 `step_id`。推荐携带 `title`、`objective`、`result`、`evidence`、`error`、`duration_ms`，但 UI 更新状态只能以 `step_id` 匹配计划步骤。

#### tool_call / tool_result / tool_error

工具事件必须携带 `call_id`，推荐携带 `step_id`、`tool_name`、`args_summary`、`result_summary`、`error_message`、`duration_ms`。前端以 `call_id` 关联开始和结束事件，以 `step_id` 归属到步骤。

#### audit / reflection / correction

这些事件必须可独立展示，并可在历史会话中完整恢复。`correction` 如导致计划变化，后端应发送新的 `plan` 或在 `correction.content` 中携带可规范化的新计划；前端最终展示以最后一个有效计划版本为准。

#### content / done / error

`content` 只承载最终回答，不用于 token 级流式展示。前端收到 `content` 后只缓存 `finalContent`。

`done` 是 UI 结束 loading 和启用最终操作的唯一成功信号，必须包含或可关联：

```json
{
  "status": "completed",
  "final_plan_status": "completed",
  "execution_id": "agent-execution-id",
  "final_answer_persisted": true
}
```

`error` 必须包含 `code`、`message`、`recoverable`，可选 `status`。取消或暂停导致的终止也必须有明确状态，不能伪装成网络错误。

## 4. 前端状态机

前端处理顺序：

1. `plan`：规范化计划，建立步骤索引。
2. `step_started`：步骤置为 `running`。
3. `tool_call` / `tool_result` / `tool_error`：维护工具调用链和观察结果。
4. `step_completed` / `step_failed`：收敛步骤状态。
5. `audit` / `reflection` / `correction`：追加过程节点，必要时触发计划版本更新。
6. `content`：缓存最终回答，不关闭 loading，不提前展示为完成态。
7. `done`：以最终计划状态和 `finalContent` 渲染完成结果，关闭 loading。
8. `error`：按 `recoverable` 和 `status` 进入失败、暂停、取消或可重试状态。

如果网络重连导致事件缺失，前端必须通过历史会话 API 重建状态；不能用当前内存状态猜测完成态。

## 5. 暂停与取消

关闭浏览器页面、关闭 `EventSource`、网络断开，只表示客户端取消订阅 SSE。服务端运行不得因此自动暂停，也不得依赖 `c.Request.Context()` 作为唯一运行上下文。

最终 API 契约：

| 方法 | 路径 | 语义 |
| --- | --- | --- |
| `POST` | `/api/v1/detection/alerts/ai-analysis/{session_id}/pause` | 请求协作式暂停当前运行，保留可恢复状态 |
| `POST` | `/api/v1/detection/alerts/ai-analysis/{session_id}/cancel` | 请求取消当前运行，进入终态 `cancelled` |

响应示例：

```json
{
  "code": 0,
  "data": {
    "session_id": "session-uuid",
    "status": "pausing",
    "execution_id": "agent-execution-id"
  }
}
```

前端暂停按钮必须先调用 `pause` API，收到成功响应后再关闭本地 `EventSource` 或切换 UI。取消按钮必须调用 `cancel` API。只关闭 `EventSource` 的行为只能标记为“已断开实时更新”，不能显示为“已暂停”。

## 6. 历史会话 API

`GET /api/v1/detection/alerts/ai-analysis/{session_id}/history` 必须返回足够信息以重建完整 UI：

```json
{
  "code": 0,
  "data": {
    "session_id": "session-uuid",
    "messages": [],
    "execution_plan": {
      "plan_id": "plan-uuid",
      "goal": "分析告警攻击链路",
      "version": 2,
      "steps": []
    },
    "audits": [],
    "reflections": [],
    "corrections": []
  }
}
```

字段要求：

- `messages`：包含用户/助手消息，以及可展示思考摘要、工具调用、工具观察、错误摘要和最终回答。
- `execution_plan`：优先返回最终计划；没有最终计划时返回初始计划；步骤状态必须由 step execution、SSE 持久化事件或最终计划状态计算后返回。
- `audits`：按发生时间升序返回审计记录。
- `reflections`：按发生时间升序返回反思记录。
- `corrections`：按发生时间升序返回纠正记录；如包含计划变更，必须能定位 `from_plan_version` 和 `to_plan_version`。

前端加载历史时必须：

1. 先规范化 `execution_plan`。
2. 回放 `messages` 中的思考摘要、工具调用和观察。
3. 追加 `audits`、`reflections`、`corrections` 时间线。
4. 以步骤执行结果重建计划状态，不因缺少实时 SSE 而全部显示 `pending`。
5. 若历史数据缺字段，展示“历史记录不完整”的可恢复提示，并避免伪造完成状态。

## 7. 测试先行要求

实现顺序必须先补测试，再改实现：

1. 后端 SSE writer / hook sink 测试：覆盖 13 个正式事件类型，特别是 `step_failed` 不得映射为 `step_completed`。
2. 后端历史会话 handler 测试：断言返回 `messages`、`execution_plan`、`audits`、`reflections`、`corrections`。
3. 后端 pause/cancel handler 测试：断言关闭 SSE 不会触发暂停，显式 API 会更新运行状态。
4. 前端 SSE handler 测试：断言 `plan` 规范化、步骤状态流转、`content` 不结束 loading、`done` 才结束 loading。
5. 前端历史恢复测试：断言可从 history payload 重建思考摘要、工具调用、观察、审计/反思/纠正和最终计划状态。

## 8. curl 验证

启动本地服务后，至少执行以下手工验证。

获取 token：

```bash
TOKEN=$(curl -s -X POST http://localhost:8082/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"Cc&324511"}' | jq -r '.token')
```

创建会话：

```bash
SESSION_ID=$(curl -s -X POST http://localhost:8082/api/v1/detection/alerts/ai-analysis/session \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"alert_ids":[],"host_filter":[]}' | jq -r '.data.session_id')
```

发送消息：

```bash
curl -s -X POST "http://localhost:8082/api/v1/detection/alerts/ai-analysis/${SESSION_ID}/message" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message":"请分析当前告警并给出攻击链路"}'
```

验证 SSE 事件：

```bash
curl -N "http://localhost:8082/api/v1/detection/alerts/ai-analysis/${SESSION_ID}/stream?auth_token=${TOKEN}"
```

观察要求：

- 能看到 `plan`、步骤事件、工具事件、过程事件、`content`、`done` 或明确 `error`。
- `step_failed` 必须独立出现，不能伪装成 `step_completed`。
- `content` 后 UI/日志仍等待 `done` 作为完成信号。

验证暂停 API：

```bash
curl -s -X POST "http://localhost:8082/api/v1/detection/alerts/ai-analysis/${SESSION_ID}/pause" \
  -H "Authorization: Bearer $TOKEN" | jq '.data.status'
```

验证取消 API：

```bash
curl -s -X POST "http://localhost:8082/api/v1/detection/alerts/ai-analysis/${SESSION_ID}/cancel" \
  -H "Authorization: Bearer $TOKEN" | jq '.data.status'
```

验证历史恢复字段：

```bash
curl -s "http://localhost:8082/api/v1/detection/alerts/ai-analysis/${SESSION_ID}/history" \
  -H "Authorization: Bearer $TOKEN" \
  | jq '.data | {messages, execution_plan, audits, reflections, corrections}'
```

## 9. Max Iterations 配置修复

### 9.1 问题描述

AI 分析频繁报错"未完成全部执行计划，已停止输出结论"，原因是默认最大迭代轮数过低（50轮），且后端限制（100轮）过早截断分析。

### 9.2 根因分析

| 配置项 | 修改前 | 修改后 | 说明 |
|--------|--------|--------|------|
| `defaultAnalysisMaxIterations` | 50 | 500 | handler 默认值 |
| `analysisMaxIterationsLimit` | 100 | 500 | handler 上限值 |
| `runtime_factory.go` 默认值 | 50 | 500 | agent-runtime 默认值 |
| 前端 `maxIterations` 默认值 | 15 | 500 | Vue 组件默认值 |
| 前端 `el-input-number :max` | 100 | 1000 | 前端最大可配置值 |

### 9.3 配置流向

```
前端 maxIterations (默认500, 最大1000)
  → POST /session { max_iterations: 500 }
    → normalizeAnalysisMaxIterations(req.MaxIterations)
      → session.MaxIterations = 500
        → NewAegisRuntime(..., maxIterations=500, ...)
          → RuntimeConfig.MaxTotalTurns = 500
```

### 9.4 修改文件

- `api-server/internal/api/handler/ai_analysis_handler.go`: 默认值和上限改为 500
- `api-server/internal/llm/adapters/runtime_factory.go`: 默认值改为 500
- `frontend/src/views/detection/AIAnalysis.vue`: 默认值 500, 最大可配 1000

历史响应必须能重建思考摘要、工具调用、观察、审计、反思、纠正和最终计划状态。
