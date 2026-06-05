-- Aegis V6.0 Assistant Tables Migration
-- Version: 6.0
-- Date: 2026-06-05
-- Description: Creates tables for Assistant intelligent mode

-- 1. assistant_sessions - 智能体会话
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

-- 2. assistant_messages - 智能体消息
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

-- 3. assistant_context_refs - 上下文引用
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

-- 4. assistant_tool_calls - 工具调用记录
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

-- 5. assistant_approvals - 审批记录
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

-- 6. assistant_tool_selections - 工具选择记录
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

-- 7. assistant_tool_policies - 工具策略
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

-- 8. assistant_memory - 智能体记忆
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

-- 9. assistant_investigation_reports - 攻击研判报告
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

-- 10. assistant_investigation_evidence - 攻击研判证据
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

-- 11. external_mcp_sources - 外接 MCP 数据源
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

-- 12. external_mcp_query_logs - 外接 MCP 查询日志
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
