# Aegis V6.0 API 与数据库设计: Assistant 智能模式

**版本**: 6.0  
**日期**: 2026-05-29  
**状态**: 设计中

---

## 1. HTTP API 总览

新增路由组：

```text
/api/v1/assistant
```

| 方法 | 路径 | 说明 |
|:---|:---|:---|
| GET | `/sessions` | 会话列表 |
| POST | `/sessions` | 创建会话 |
| GET | `/sessions/:session_id` | 会话详情 |
| GET | `/sessions/:session_id/messages` | 消息历史 |
| POST | `/sessions/:session_id/message` | 发送消息并启动 run |
| GET | `/sessions/:session_id/stream` | SSE 事件流 |
| POST | `/sessions/:session_id/cancel` | 取消当前 run |
| GET | `/sessions/:session_id/context-refs` | 上下文对象 |
| GET | `/sessions/:session_id/tool-calls` | 工具调用记录 |
| GET | `/sessions/:session_id/approvals` | 审批列表 |
| GET | `/approvals/:approval_id` | 审批详情 |
| POST | `/approvals/:approval_id/approve` | 批准并执行 |
| POST | `/approvals/:approval_id/reject` | 拒绝 |

---

## 2. API 详情

### 2.1 创建会话

```http
POST /api/v1/assistant/sessions
```

请求：

```json
{
  "title": "分析最近 24 小时高危告警",
  "task_type": "investigation",
  "initial_message": "帮我分析最近 24 小时的高危告警",
  "context_refs": [
    { "object_type": "alert", "object_id": "ALT-xxx" }
  ]
}
```

响应：

```json
{
  "code": 0,
  "data": {
    "session_id": "asst_20260529_xxx",
    "title": "分析最近 24 小时高危告警",
    "task_type": "investigation",
    "status": "active",
    "mode_source": "assistant",
    "created_at": "2026-05-29T08:00:00Z"
  }
}
```

### 2.2 发送消息

```http
POST /api/v1/assistant/sessions/:session_id/message
```

请求：

```json
{
  "content": "找出最可疑的一条并给出证据",
  "context_refs": [
    { "object_type": "host", "object_id": "host-uuid" }
  ]
}
```

响应：

```json
{
  "code": 0,
  "data": {
    "message_id": "msg_xxx",
    "run_id": "run_xxx",
    "status": "running"
  }
}
```

### 2.3 审批批准

```http
POST /api/v1/assistant/approvals/:approval_id/approve
```

请求：

```json
{
  "comment": "确认启用，当前为测试环境"
}
```

响应：

```json
{
  "code": 0,
  "data": {
    "approval_id": "appr_xxx",
    "status": "executed",
    "execution_result": {
      "success": true,
      "summary": "DetectionPackage 已启用并下发到 12 个在线 agent"
    }
  }
}
```

---

## 3. SSE 协议

事件格式：

```json
{
  "type": "tool_result",
  "session_id": "asst_xxx",
  "run_id": "run_xxx",
  "message_id": "msg_xxx",
  "payload": {}
}
```

事件类型：

| type | payload |
|:---|:---|
| `thinking` | `{ "content": "..." }` |
| `message_delta` | `{ "delta": "..." }` |
| `plan` | `AssistantPlan` |
| `step_started` | `AssistantPlanStep` |
| `step_completed` | `AssistantPlanStep` |
| `tool_call` | `AssistantToolCall` |
| `tool_result` | `AssistantToolCall` |
| `tool_error` | `{ "call_id": "...", "error": "..." }` |
| `approval_required` | `AssistantApproval` |
| `approval_updated` | `AssistantApproval` |
| `context_ref_added` | `AssistantContextRef` |
| `result_card` | `AssistantResultCard` |
| `done` | `{ "status": "completed" }` |
| `error` | `{ "message": "..." }` |

---

## 4. 数据库表

### 4.1 assistant_sessions

```sql
CREATE TABLE IF NOT EXISTS assistant_sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      VARCHAR(100) UNIQUE NOT NULL,
    title           VARCHAR(255) NOT NULL,
    task_type       VARCHAR(40)  NOT NULL DEFAULT 'explanation',
    mode_source     VARCHAR(40)  NOT NULL DEFAULT 'assistant',
    status          VARCHAR(32)  NOT NULL DEFAULT 'active',
    created_by      VARCHAR(100),
    message_count   INTEGER      NOT NULL DEFAULT 0,
    tool_call_count INTEGER      NOT NULL DEFAULT 0,
    approval_count  INTEGER      NOT NULL DEFAULT 0,
    last_message_at TIMESTAMPTZ,
    metadata        JSONB        NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_assistant_sessions_status
    ON assistant_sessions(status);
CREATE INDEX IF NOT EXISTS idx_assistant_sessions_created_by
    ON assistant_sessions(created_by);
CREATE INDEX IF NOT EXISTS idx_assistant_sessions_updated_at
    ON assistant_sessions(updated_at DESC);
```

### 4.2 assistant_messages

```sql
CREATE TABLE IF NOT EXISTS assistant_messages (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id   VARCHAR(100) NOT NULL,
    message_id   VARCHAR(100) UNIQUE NOT NULL,
    role         VARCHAR(20)  NOT NULL,
    content      TEXT,
    thinking     TEXT,
    plan         JSONB,
    tool_calls   JSONB,
    approvals    JSONB,
    result_cards JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_assistant_messages_session
    ON assistant_messages(session_id, created_at);
```

### 4.3 assistant_context_refs

```sql
CREATE TABLE IF NOT EXISTS assistant_context_refs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id  VARCHAR(100) NOT NULL,
    object_type VARCHAR(40)  NOT NULL,
    object_id   VARCHAR(160) NOT NULL,
    title       VARCHAR(255),
    summary     TEXT,
    route_path  VARCHAR(255),
    snapshot    JSONB        NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE(session_id, object_type, object_id)
);

CREATE INDEX IF NOT EXISTS idx_assistant_context_refs_session
    ON assistant_context_refs(session_id);
CREATE INDEX IF NOT EXISTS idx_assistant_context_refs_object
    ON assistant_context_refs(object_type, object_id);
```

### 4.4 assistant_tool_calls

```sql
CREATE TABLE IF NOT EXISTS assistant_tool_calls (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id     VARCHAR(100) NOT NULL,
    message_id     VARCHAR(100),
    call_id        VARCHAR(100) UNIQUE NOT NULL,
    tool_name      VARCHAR(120) NOT NULL,
    domain         VARCHAR(40)  NOT NULL,
    risk_level     VARCHAR(20)  NOT NULL,
    status         VARCHAR(32)  NOT NULL,
    args           JSONB        NOT NULL DEFAULT '{}',
    args_summary   TEXT,
    result         JSONB,
    result_summary TEXT,
    error_message  TEXT,
    duration_ms    BIGINT       NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_assistant_tool_calls_session
    ON assistant_tool_calls(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_assistant_tool_calls_status
    ON assistant_tool_calls(status);
CREATE INDEX IF NOT EXISTS idx_assistant_tool_calls_risk
    ON assistant_tool_calls(risk_level);
```

### 4.5 assistant_approvals

```sql
CREATE TABLE IF NOT EXISTS assistant_approvals (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    approval_id    VARCHAR(100) UNIQUE NOT NULL,
    session_id     VARCHAR(100) NOT NULL,
    tool_call_id   VARCHAR(100) NOT NULL,
    tool_name      VARCHAR(120) NOT NULL,
    risk_level     VARCHAR(20)  NOT NULL,
    title          VARCHAR(255) NOT NULL,
    impact_summary TEXT,
    params_preview JSONB        NOT NULL DEFAULT '{}',
    rollback_hint  TEXT,
    status         VARCHAR(32)  NOT NULL DEFAULT 'pending',
    requested_by   VARCHAR(100),
    reviewed_by    VARCHAR(100),
    review_comment TEXT,
    expires_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    reviewed_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_assistant_approvals_session
    ON assistant_approvals(session_id);
CREATE INDEX IF NOT EXISTS idx_assistant_approvals_status
    ON assistant_approvals(status);
CREATE INDEX IF NOT EXISTS idx_assistant_approvals_tool_call
    ON assistant_approvals(tool_call_id);
```

### 4.6 assistant_memory

```sql
CREATE TABLE IF NOT EXISTS assistant_memory (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id  VARCHAR(100) NOT NULL,
    memory_type VARCHAR(40)  NOT NULL,
    content     TEXT         NOT NULL,
    metadata    JSONB        NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_assistant_memory_session
    ON assistant_memory(session_id, memory_type);
```

---

## 5. 枚举

### 5.1 session status

```text
active
running
waiting_approval
completed
cancelled
failed
```

### 5.2 task type

```text
investigation
operations
generation
remediation
configuration
explanation
```

### 5.3 risk level

```text
readonly
low
medium
high
critical
```

### 5.4 approval status

```text
pending
approved
rejected
expired
executed
failed
```

---

## 6. 配置项

建议新增系统配置：

| Key | 默认值 | 说明 |
|:---|:---|:---|
| `assistant.enabled` | `true` | 是否启用智能模式 |
| `assistant.max_iterations` | `500` | 单次 run 最大迭代 |
| `assistant.require_approval_medium` | `true` | medium 工具是否默认审批 |
| `assistant.approval_ttl_minutes` | `30` | 审批过期时间 |
| `assistant.max_context_refs` | `50` | 单会话上下文对象上限 |
| `assistant.max_tool_calls` | `100` | 单次 run 工具调用上限 |

---

## 7. 迁移文件建议

```text
migrations/015_v6.0_assistant_tables.sql
```

该迁移只新增 `assistant_*` 表和索引，不修改 V5.8 业务表。

---

## 8. API 测试用例

```bash
# 创建会话
curl -X POST http://localhost:8082/api/v1/assistant/sessions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"title":"测试智能模式","task_type":"explanation","initial_message":"总结主机态势"}'

# 发送消息
curl -X POST http://localhost:8082/api/v1/assistant/sessions/<session_id>/message \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"content":"列出离线主机"}'

# 查看审批
curl http://localhost:8082/api/v1/assistant/sessions/<session_id>/approvals \
  -H "Authorization: Bearer <token>"
```

