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
| GET | `/tools` | 获取全部智能体工具配置列表 |
| GET | `/tool-approval-policy` | 获取工具审批模式和白名单 |
| PUT | `/tool-approval-policy` | 更新工具审批模式 |
| PUT | `/tools/:tool_name/whitelist` | 更新单个工具白名单状态 |
| POST | `/tools/whitelist/batch` | 批量更新工具白名单 |
| POST | `/tools/whitelist/reset-defaults` | 恢复默认低危工具白名单 |
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
| `intent_detected` | `AssistantIntentResult` |
| `tools_selected` | `AssistantToolSelection` |
| `tool_search` | `ToolSearchResult` |
| `tool_expansion` | `AssistantToolSelection` |
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

`tools_selected` 示例：

```json
{
  "type": "tools_selected",
  "session_id": "asst_xxx",
  "run_id": "run_xxx",
  "message_id": "msg_xxx",
  "payload": {
    "domains": ["package"],
    "operations": ["get", "approve"],
    "tool_count": 4,
    "tools": [
      { "name": "Package.Get", "risk": "readonly", "reason": "当前页面为检测包详情" },
      { "name": "Package.Build.GetLatest", "risk": "readonly", "reason": "签名前需要确认最新构建状态" },
      { "name": "Package.Sign", "risk": "critical", "reason": "用户明确要求签名" },
      { "name": "Package.Enable", "risk": "critical", "reason": "用户明确要求启用" }
    ]
  }
}
```

### 2.4 获取智能体工具列表

```http
GET /api/v1/assistant/tools?domain=package&risk=critical&keyword=sign&whitelisted=false
```

响应：

```json
{
  "code": 0,
  "data": {
    "mode": "whitelist",
    "tools": [
      {
        "name": "Package.Sign",
        "domain": "package",
        "operation": "approve",
        "risk_level": "critical",
        "description": "对构建成功的动态 eBPF DetectionPackage 进行签名",
        "args_summary": "package_id:string",
        "default_whitelisted": false,
        "whitelisted": false,
        "enabled": true,
        "updated_at": "2026-06-04T10:00:00Z"
      }
    ],
    "total": 1
  }
}
```

### 2.5 更新审批模式

```http
PUT /api/v1/assistant/tool-approval-policy
```

请求：

```json
{
  "mode": "whitelist"
}
```

`mode` 可选值：

```text
request_approval
whitelist
full_access
```

### 2.6 更新白名单

```http
PUT /api/v1/assistant/tools/Package.Sign/whitelist
```

请求：

```json
{
  "whitelisted": false
}
```

批量请求：

```http
POST /api/v1/assistant/tools/whitelist/batch
```

```json
{
  "items": [
    { "tool_name": "Host.List", "whitelisted": true },
    { "tool_name": "Package.Sign", "whitelisted": false }
  ]
}
```

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

### 4.6 assistant_tool_selections

用于记录每次 run 传给 agent-runtime 的工具集合，便于调试“为什么模型能/不能调用某个工具”。

```sql
CREATE TABLE IF NOT EXISTS assistant_tool_selections (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      VARCHAR(100) NOT NULL,
    run_id          VARCHAR(100) NOT NULL,
    message_id      VARCHAR(100),
    stage           VARCHAR(32)  NOT NULL DEFAULT 'initial',
    query           TEXT         NOT NULL,
    intent          JSONB        NOT NULL DEFAULT '{}',
    selected_tools  JSONB        NOT NULL DEFAULT '[]',
    candidate_tools JSONB        NOT NULL DEFAULT '[]',
    max_tools       INTEGER      NOT NULL DEFAULT 24,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_assistant_tool_selections_session
    ON assistant_tool_selections(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_assistant_tool_selections_run
    ON assistant_tool_selections(run_id);
```

`stage` 枚举：

```text
initial
expanded
approval_resume
retry
```

### 4.7 assistant_tool_policies

用于保存工具白名单和配置页展示需要的覆盖项。完整工具元数据来自后端 `ToolCatalog`，该表只保存运行时可配置项和初始化快照。

```sql
CREATE TABLE IF NOT EXISTS assistant_tool_policies (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tool_name           VARCHAR(160) UNIQUE NOT NULL,
    domain              VARCHAR(40)  NOT NULL,
    operation           VARCHAR(40)  NOT NULL,
    risk_level          VARCHAR(20)  NOT NULL,
    description         TEXT,
    args_summary        TEXT,
    default_whitelisted BOOLEAN      NOT NULL DEFAULT FALSE,
    whitelisted         BOOLEAN      NOT NULL DEFAULT FALSE,
    enabled             BOOLEAN      NOT NULL DEFAULT TRUE,
    source              VARCHAR(32)  NOT NULL DEFAULT 'builtin',
    updated_by          VARCHAR(100),
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_assistant_tool_policies_domain
    ON assistant_tool_policies(domain);
CREATE INDEX IF NOT EXISTS idx_assistant_tool_policies_risk
    ON assistant_tool_policies(risk_level);
CREATE INDEX IF NOT EXISTS idx_assistant_tool_policies_whitelisted
    ON assistant_tool_policies(whitelisted);
```

初始化规则：

- 服务启动时读取 `ToolCatalog`，对缺失工具执行 upsert。
- `readonly` 工具默认可加入白名单。
- `low` 工具可按工具清单显式加入默认白名单。
- `medium/high/critical` 默认不加入白名单。
- 管理员可在配置页修改 `whitelisted`。

### 4.8 assistant_memory

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

### 5.5 tool approval mode

```text
request_approval
whitelist
full_access
```

---

## 6. 配置项

建议新增系统配置：

| Key | 默认值 | 说明 |
|:---|:---|:---|
| `assistant.enabled` | `true` | 是否启用智能模式 |
| `assistant.max_iterations` | `500` | 单次 run 最大迭代 |
| `assistant.max_selected_tools` | `24` | 单次注入 agent-runtime 的最大工具数 |
| `assistant.max_write_tools` | `6` | 单次注入的写操作工具上限 |
| `assistant.tool_approval_mode` | `whitelist` | 工具审批模式: request_approval/whitelist/full_access |
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

完整接口测试用例见 `assistant_api_curl_test_cases_v6.0.md`。该文档覆盖：

- `curl + jq` 通用断言函数。
- Assistant 会话、消息、SSE、审批、工具配置、白名单全接口测试。
- `request_approval` / `whitelist` / `full_access` 三种审批模式行为验证。
- 审批批准/拒绝后的工具调用状态、run 恢复和数据一致性校验。
- 未授权、非法模式、不存在对象、重复审批等异常用例。

