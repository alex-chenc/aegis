# Aegis V6.0 API 与数据库设计: Assistant 智能模式

**版本**: 6.0
**日期**: 2026-05-29
**状态**: 已实现 (2026-06-05 更新)

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
| POST | `/investigations/host-attack` | 创建主机攻击研判 |
| GET | `/investigations/:investigation_id` | 查询主机攻击研判报告 |
| GET | `/investigations/:investigation_id/evidence` | 查询研判证据明细 |
| POST | `/investigations/:investigation_id/rebuild-report` | 基于已收集证据重生成研判报告 |
| GET | `/mcp-sources` | 外接 MCP 数据源列表 |
| POST | `/mcp-sources` | 新增外接 MCP 数据源 |
| GET | `/mcp-sources/:source_id` | 外接 MCP 数据源详情 |
| PUT | `/mcp-sources/:source_id` | 更新外接 MCP 数据源 |
| DELETE | `/mcp-sources/:source_id` | 删除外接 MCP 数据源 |
| POST | `/mcp-sources/:source_id/test` | 测试 MCP 数据源连接 |
| POST | `/mcp-sources/:source_id/sync-schema` | 同步 MCP schema/tool 摘要 |
| GET | `/mcp-sources/:source_id/query-logs` | 查询 MCP 数据源调用日志 |
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

### 2.7 主机攻击研判

#### 创建研判

```http
POST /api/v1/assistant/investigations/host-attack
```

请求：

```json
{
  "session_id": "asst_xxx",
  "host_id": "host-curl-test-001",
  "alert_ids": ["ALT-CURL-TEST-001"],
  "time_range": {
    "from": "2026-06-04T00:00:00+08:00",
    "to": "2026-06-05T00:00:00+08:00"
  },
  "include_agent_live": true,
  "include_external_mcp": false,
  "mcp_source_ids": [],
  "max_evidence_items": 200
}
```

响应：

```json
{
  "code": 0,
  "data": {
    "investigation_id": "inv_20260605_xxx",
    "status": "completed",
    "compromise_assessment": {
      "verdict": "suspicious",
      "score": 72,
      "confidence": 0.76,
      "summary": "发现攻击迹象但入口仍需补充证据"
    },
    "entry_point_candidates": [],
    "attack_timeline": { "events": [] },
    "attack_path": { "nodes": [], "edges": [] },
    "evidence_matrix": { "items": [] },
    "missing_evidence": []
  }
}
```

字段约束：

- `verdict` 只能是 `confirmed_compromised`、`suspicious`、`likely_benign`、`insufficient_evidence`。
- `score` 必须在 0-100。
- `confidence` 必须在 0-1。
- 如果 `evidence_matrix.items` 为空，`verdict` 必须为 `insufficient_evidence`。
- `include_external_mcp=true` 时必须走 `Investigation.HostAttack.AnalyzeWithExternal` 和外部 MCP 审批/审计策略。

#### 查询报告

```http
GET /api/v1/assistant/investigations/:investigation_id
```

响应必须返回和创建接口一致的完整研判结构。

#### 查询证据

```http
GET /api/v1/assistant/investigations/:investigation_id/evidence?page=1&page_size=50&source_type=aegis_alert
```

响应：

```json
{
  "code": 0,
  "data": {
    "items": [
      {
        "evidence_id": "ev_xxx",
        "source_type": "aegis_alert",
        "source_name": "Aegis",
        "host_id": "host-curl-test-001",
        "title": "反弹 shell 告警",
        "summary": "bash 连接外部 IP",
        "supports": ["compromise", "execution"],
        "confidence": 0.9
      }
    ],
    "total": 1
  }
}
```

#### 重生成报告

```http
POST /api/v1/assistant/investigations/:investigation_id/rebuild-report
```

该接口只基于已落库证据重建大模型报告，不重新查询 Agent 或外部 MCP。用于 Prompt 调整后的报告重放测试。

### 2.8 外接 MCP 数据源

#### 2.8.1 创建数据源

```http
POST /api/v1/assistant/mcp-sources
```

请求：

```json
{
  "name": "prod-siem",
  "source_type": "siem",
  "transport": "streamable_http",
  "endpoint_url": "https://siem.example.com/mcp",
  "auth_type": "bearer",
  "credential": {
    "token": "只在请求中提交，后端加密保存"
  },
  "description": "生产 SIEM 查询源",
  "query_limits": {
    "max_rows": 100,
    "timeout_seconds": 20,
    "max_context_chars": 12000
  }
}
```

响应必须脱敏：

```json
{
  "code": 0,
  "data": {
    "source_id": "mcp_prod_siem",
    "name": "prod-siem",
    "source_type": "siem",
    "transport": "streamable_http",
    "endpoint_url_masked": "https://siem.example.com/mcp",
    "auth_type": "bearer",
    "credential_configured": true,
    "enabled": true,
    "last_test_status": null
  }
}
```

#### 2.8.2 测试连接

```http
POST /api/v1/assistant/mcp-sources/:source_id/test
```

响应：

```json
{
  "code": 0,
  "data": {
    "source_id": "mcp_prod_siem",
    "success": true,
    "latency_ms": 85,
    "tool_count": 8,
    "message": "connection ok"
  }
}
```

#### 2.8.3 同步 schema

```http
POST /api/v1/assistant/mcp-sources/:source_id/sync-schema
```

响应：

```json
{
  "code": 0,
  "data": {
    "source_id": "mcp_prod_siem",
    "schema_version": "2026-06-05T10:00:00+08:00",
    "tool_count": 8,
    "fields": ["timestamp", "host", "username", "src_ip", "event_type"]
  }
}
```

#### 2.8.4 查询日志

```http
GET /api/v1/assistant/mcp-sources/:source_id/query-logs?page=1&page_size=20
```

响应：

```json
{
  "code": 0,
  "data": {
    "items": [
      {
        "query_id": "mcpq_xxx",
        "session_id": "asst_xxx",
        "source_id": "mcp_prod_siem",
        "query_goal": "查询 host-001 最近 24 小时登录失败事件",
        "result_count": 12,
        "status": "success",
        "duration_ms": 120
      }
    ],
    "total": 1
  }
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

### 4.9 assistant_investigation_reports

```sql
CREATE TABLE IF NOT EXISTS assistant_investigation_reports (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    investigation_id  VARCHAR(100) UNIQUE NOT NULL,
    session_id        VARCHAR(100) NOT NULL,
    run_id            VARCHAR(100),
    host_id           VARCHAR(160) NOT NULL,
    task_type         VARCHAR(60) NOT NULL DEFAULT 'host_attack_investigation',
    verdict           VARCHAR(40) NOT NULL,
    score             INTEGER NOT NULL DEFAULT 0,
    confidence        NUMERIC(5,4) NOT NULL DEFAULT 0,
    time_range        JSONB NOT NULL DEFAULT '{}',
    source_coverage   JSONB NOT NULL DEFAULT '{}',
    entry_candidates  JSONB NOT NULL DEFAULT '[]',
    attack_timeline   JSONB NOT NULL DEFAULT '{}',
    attack_path       JSONB NOT NULL DEFAULT '{}',
    impact_scope      JSONB NOT NULL DEFAULT '{}',
    missing_evidence  JSONB NOT NULL DEFAULT '[]',
    report_markdown   TEXT NOT NULL DEFAULT '',
    created_by        VARCHAR(100),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_assistant_investigation_reports_session
    ON assistant_investigation_reports(session_id);

CREATE INDEX IF NOT EXISTS idx_assistant_investigation_reports_host
    ON assistant_investigation_reports(host_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_assistant_investigation_reports_verdict
    ON assistant_investigation_reports(verdict);
```

### 4.10 assistant_investigation_evidence

```sql
CREATE TABLE IF NOT EXISTS assistant_investigation_evidence (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    investigation_id  VARCHAR(100) NOT NULL,
    evidence_id       VARCHAR(100) NOT NULL,
    source_type       VARCHAR(60) NOT NULL,
    source_name       VARCHAR(120) NOT NULL,
    object_type       VARCHAR(60) NOT NULL,
    object_id         VARCHAR(160),
    host_id           VARCHAR(160),
    event_time        TIMESTAMPTZ,
    severity          VARCHAR(40),
    mitre_id          VARCHAR(40),
    title             VARCHAR(255) NOT NULL,
    summary           TEXT NOT NULL DEFAULT '',
    normalized        JSONB NOT NULL DEFAULT '{}',
    supports          JSONB NOT NULL DEFAULT '[]',
    confidence        NUMERIC(5,4) NOT NULL DEFAULT 0,
    is_external       BOOLEAN NOT NULL DEFAULT FALSE,
    is_truncated      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(investigation_id, evidence_id)
);

CREATE INDEX IF NOT EXISTS idx_assistant_investigation_evidence_inv
    ON assistant_investigation_evidence(investigation_id);

CREATE INDEX IF NOT EXISTS idx_assistant_investigation_evidence_host_time
    ON assistant_investigation_evidence(host_id, event_time DESC);

CREATE INDEX IF NOT EXISTS idx_assistant_investigation_evidence_source
    ON assistant_investigation_evidence(source_type);
```

### 4.11 external_mcp_sources

```sql
CREATE TABLE IF NOT EXISTS external_mcp_sources (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id             VARCHAR(100) UNIQUE NOT NULL,
    name                  VARCHAR(120) NOT NULL,
    source_type           VARCHAR(40) NOT NULL,
    transport             VARCHAR(40) NOT NULL DEFAULT 'streamable_http',
    endpoint_url          TEXT NOT NULL,
    auth_type             VARCHAR(40) NOT NULL DEFAULT 'none',
    credential_ref        VARCHAR(255),
    enabled               BOOLEAN NOT NULL DEFAULT TRUE,
    description           TEXT,
    allowed_tool_names    JSONB NOT NULL DEFAULT '[]',
    schema_cache          JSONB NOT NULL DEFAULT '{}',
    query_limits          JSONB NOT NULL DEFAULT '{}',
    data_classification   VARCHAR(40) NOT NULL DEFAULT 'internal',
    last_test_status      VARCHAR(40),
    last_test_error       TEXT,
    last_test_at          TIMESTAMPTZ,
    created_by            VARCHAR(100),
    updated_by            VARCHAR(100),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_external_mcp_sources_type
    ON external_mcp_sources(source_type);

CREATE INDEX IF NOT EXISTS idx_external_mcp_sources_enabled
    ON external_mcp_sources(enabled);
```

### 4.12 external_mcp_query_logs

```sql
CREATE TABLE IF NOT EXISTS external_mcp_query_logs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    query_id          VARCHAR(100) UNIQUE NOT NULL,
    session_id        VARCHAR(100),
    run_id            VARCHAR(100),
    tool_call_id      VARCHAR(100),
    source_id         VARCHAR(100) NOT NULL,
    source_name       VARCHAR(120),
    query_goal        TEXT NOT NULL,
    request_summary   TEXT,
    redacted_request  JSONB NOT NULL DEFAULT '{}',
    result_count      INTEGER NOT NULL DEFAULT 0,
    result_digest     TEXT,
    status            VARCHAR(40) NOT NULL,
    error_message     TEXT,
    duration_ms       INTEGER NOT NULL DEFAULT 0,
    created_by        VARCHAR(100),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_external_mcp_query_logs_session
    ON external_mcp_query_logs(session_id);

CREATE INDEX IF NOT EXISTS idx_external_mcp_query_logs_source
    ON external_mcp_query_logs(source_id);

CREATE INDEX IF NOT EXISTS idx_external_mcp_query_logs_created
    ON external_mcp_query_logs(created_at DESC);
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
host_attack_investigation
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

系统配置（当前实现状态）：

| Key | 默认值 | 实现状态 | 说明 |
|:---|:---|:---|:---|
| `assistant.enabled` | `true` | 始终启用 | 是否启用智能模式 |
| `assistant.max_iterations` | `80` | 硬编码 | 单次 run 最大迭代 |
| `assistant.max_selected_tools` | `24` | 硬编码 | 单次注入 agent-runtime 的最大工具数 |
| `assistant.max_write_tools` | `6` | 硬编码 | 单次注入的写操作工具上限 |
| `assistant.tool_approval_mode` | `whitelist` | 硬编码 | 工具审批模式: request_approval/whitelist/full_access |
| `assistant.require_approval_medium` | `true` | 未实现 | medium 工具是否默认审批 |
| `assistant.approval_ttl_minutes` | `30` | 硬编码 | 审批过期时间 |
| `assistant.max_context_refs` | `50` | 未实现 | 单会话上下文对象上限 |
| `assistant.max_tool_calls` | `60` | 硬编码 | 单次 run 工具调用上限 |
| `assistant.investigation.enabled` | `true` | 始终启用 | 是否启用主机攻击研判 Profile |
| `assistant.investigation.default_time_range_hours` | `24` | 未实现 | 只给主机时默认研判时间范围 |
| `assistant.investigation.alert_context_before_hours` | `2` | 未实现 | 告警上下文向前扩展时间 |
| `assistant.investigation.alert_context_after_hours` | `6` | 未实现 | 告警上下文向后扩展时间 |
| `assistant.investigation.max_evidence_items` | `200` | 来自 API 请求 | 单次研判最大证据数 |
| `assistant.investigation.agent_live_probe_enabled` | `true` | 来自 API 请求 | 是否允许研判读取 Agent 实时只读证据 |
| `assistant.investigation.external_mcp_default` | `false` | 来自 API 请求 | 是否默认启用外部 MCP 证据；建议默认 false |
| `assistant.investigation.report_prompt_max_chars` | `32000` | 未实现 | 研判报告 Prompt 最大上下文字符数 |
| `assistant.external_mcp.enabled` | `true` | 始终启用 | 是否启用外接 MCP 数据源 |
| `assistant.external_mcp.max_sources_per_run` | `3` | 未实现 | 单轮最多查询的数据源数 |
| `assistant.external_mcp.max_query_per_run` | `6` | 未实现 | 单轮最多外部查询次数 |
| `assistant.external_mcp.default_max_rows` | `50` | 来自数据源配置 | 外部查询默认最大行数 |
| `assistant.external_mcp.max_context_chars` | `24000` | 未实现 | 外部结果注入大模型前的最大字符数 |
| `assistant.external_mcp.allowed_transports` | `streamable_http,sse` | 未实现 | 允许的 MCP transport |

---

## 7. 迁移文件建议

```text
migrations/015_v6.0_assistant_tables.sql
```

该迁移只新增 `assistant_*`、`assistant_investigation_*`、`external_mcp_*` 表和索引，不修改 V5.8 业务表。

---

## 8. API 测试用例

完整接口测试用例见 `assistant_api_curl_test_cases_v6.0.md`。该文档覆盖：

- `curl + jq` 通用断言函数。
- Assistant 会话、消息、SSE、审批、工具配置、白名单全接口测试。
- 主机攻击研判接口、证据明细、报告重建、返回结构和证据链断言。
- `request_approval` / `whitelist` / `full_access` 三种审批模式行为验证。
- 审批批准/拒绝后的工具调用状态、run 恢复和数据一致性校验。
- 未授权、非法模式、不存在对象、重复审批等异常用例。

---

## 9. 实现状态 (2026-06-05)

### 9.1 数据库

| 组件 | 状态 | 备注 |
|:---|:---|:---|
| 表定义 (12/12) | ✅ 完成 | `migrations/015_v6.0_assistant_tables.sql` |
| GORM 模型 (12/12) | ✅ 完成 | `model/assistant.go`, `model/assistant_investigation.go`, `model/external_mcp.go` |
| AutoMigrate (12/12) | ✅ 完成 | `repository/db.go` |
| Repository (11/12) | ⚠️ 缺失 | `assistant_tool_selections` 无独立 Repository |

### 9.2 API 端点

| 类别 | 设计 | 已实现 | 缺失 |
|:---|:---|:---|:---|
| 会话管理 | 10 | 10 | - |
| 工具策略 | 6 | 6 | - |
| 审批 | 3 | 3 | - |
| 研判 | 4 | 3 | `POST /investigations/:investigation_id/rebuild-report` |
| MCP 数据源 | 7 | 6 | `GET /mcp-sources/:source_id/query-logs` |
| **总计** | **30** | **28** | **2** |

### 9.3 配置项

| 类别 | 设计 | 已实现 | 备注 |
|:---|:---|:---|:---|
| 顶层配置 | 9 | 0 | 全部硬编码 |
| 研判配置 | 8 | 0 | 部分来自 API 请求 |
| 外部 MCP 配置 | 6 | 0 | 部分来自数据源配置 |
| **总计** | **23** | **0** | **全部未实现为可配置项** |

### 9.4 SSE 事件类型

| 状态 | 数量 | 说明 |
|:---|:---|:---|
| 已实现并使用 | 16 | 正常工作 |
| 已定义未使用 | 2 | `approval_updated`, `context_ref_added` |
| 额外新增 | 3 | `run_started`, `run_waiting_approval`, `business_object` |

### 9.5 硬编码参数

| 参数 | 当前值 | 位置 |
|:---|:---|:---|
| `assistant.max_iterations` | 80 | `orchestrator.go:369`, `runtime_factory.go:183` |
| `assistant.max_tool_calls` | 60 | `runtime_factory.go:188` |
| `assistant.max_selected_tools` | 24 | `orchestrator.go:132`, `tool_selector.go:65` |
| `assistant.max_write_tools` | 6 | `tool_selector.go:69` |
| `assistant.approval_ttl_minutes` | 30 | `approval_gate.go:129` |

### 9.6 待实现功能

| 优先级 | 功能 | 说明 |
|:---|:---|:---|
| 高 | `POST /investigations/:investigation_id/rebuild-report` | 基于已收集证据重生成研判报告 |
| 高 | `GET /mcp-sources/:source_id/query-logs` | 查询 MCP 数据源调用日志 |
| 高 | `assistant_tool_selection_repo.go` | 工具选择记录 Repository |
| 中 | 配置项读取 | 使用 `SystemConfigRepo` 读取 `assistant.*` 配置 |
| 中 | `approval_updated` 事件 | 补齐事件触发逻辑 |
| 中 | `context_ref_added` 事件 | 补齐事件触发逻辑 |
| 低 | 配置管理 API | 提供配置项 CRUD 接口 |

> 详细差异分析见 `api_database_design_gap_analysis_v6.0.md`
