-- V5.7 Agent Execution Tables
-- 7 tables for agent-runtime execution persistence: reflections, audits, corrections, tool calls, model errors

-- 1. agent_executions — 单次agent-runtime执行记录
CREATE TABLE IF NOT EXISTS agent_executions (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id        VARCHAR(100) NOT NULL,
    task_id           VARCHAR(100) NOT NULL,
    status            VARCHAR(20),
    exit_reason       VARCHAR(50),
    final_answer      TEXT,
    initial_plan      JSONB,
    final_plan        JSONB,
    completion        JSONB,
    metrics           JSONB,
    started_at        TIMESTAMP,
    ended_at          TIMESTAMP,
    total_duration_ms BIGINT,
    created_at        TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_executions_task_id ON agent_executions(task_id);
CREATE INDEX IF NOT EXISTS idx_agent_executions_session_id ON agent_executions(session_id);

-- 2. agent_step_executions — 步骤执行详情
CREATE TABLE IF NOT EXISTS agent_step_executions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id  UUID NOT NULL,
    task_id       VARCHAR(100),
    step_id       VARCHAR(50),
    attempt       INT,
    status        VARCHAR(20),
    result        TEXT,
    evidence      JSONB,
    error         JSONB,
    react_turns   JSONB,
    started_at    TIMESTAMP,
    ended_at      TIMESTAMP,
    duration_ms   BIGINT,
    created_at    TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_step_executions_execution_id ON agent_step_executions(execution_id);
CREATE INDEX IF NOT EXISTS idx_agent_step_executions_task_id ON agent_step_executions(task_id);

-- 3. agent_reflections — 反思记录
CREATE TABLE IF NOT EXISTS agent_reflections (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id    UUID NOT NULL,
    task_id         VARCHAR(100),
    step_id         VARCHAR(50),
    reflection_id   VARCHAR(100),
    trigger         VARCHAR(50),
    root_cause      TEXT,
    impact          TEXT,
    recoverable     BOOLEAN,
    recommendation  VARCHAR(50),
    disable_tools   JSONB,
    correction_hint TEXT,
    reusable_lesson TEXT,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_reflections_execution_id ON agent_reflections(execution_id);
CREATE INDEX IF NOT EXISTS idx_agent_reflections_task_id ON agent_reflections(task_id);

-- 4. agent_audits — 审计记录
CREATE TABLE IF NOT EXISTS agent_audits (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id    UUID NOT NULL,
    task_id         VARCHAR(100),
    audit_id        VARCHAR(100),
    trigger         VARCHAR(50),
    drifted         BOOLEAN,
    risk_level      VARCHAR(20),
    findings        JSONB,
    decision        VARCHAR(50),
    correction_hint TEXT,
    should_exit     BOOLEAN,
    exit_reason     VARCHAR(50),
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_audits_execution_id ON agent_audits(execution_id);
CREATE INDEX IF NOT EXISTS idx_agent_audits_task_id ON agent_audits(task_id);

-- 5. agent_corrections — 计划纠正记录
CREATE TABLE IF NOT EXISTS agent_corrections (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id      UUID NOT NULL,
    task_id           VARCHAR(100),
    correction_id     VARCHAR(100),
    trigger           VARCHAR(50),
    from_plan_version INT,
    to_plan_version   INT,
    reason            TEXT,
    actions           JSONB,
    valid             BOOLEAN,
    validation_errors JSONB,
    created_at        TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_corrections_execution_id ON agent_corrections(execution_id);
CREATE INDEX IF NOT EXISTS idx_agent_corrections_task_id ON agent_corrections(task_id);

-- 6. agent_tool_call_records — 工具调用详情
CREATE TABLE IF NOT EXISTS agent_tool_call_records (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id   UUID NOT NULL,
    task_id        VARCHAR(100),
    step_id        VARCHAR(50),
    call_id        VARCHAR(100),
    tool_name      VARCHAR(100),
    reason         TEXT,
    args_summary   TEXT,
    status         VARCHAR(20),
    result_summary TEXT,
    error_message  TEXT,
    risk_level     VARCHAR(20),
    duration_ms    BIGINT,
    started_at     TIMESTAMP,
    ended_at       TIMESTAMP,
    created_at     TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_tool_call_records_execution_id ON agent_tool_call_records(execution_id);
CREATE INDEX IF NOT EXISTS idx_agent_tool_call_records_task_id ON agent_tool_call_records(task_id);

-- 7. agent_model_errors — 模型调用错误
CREATE TABLE IF NOT EXISTS agent_model_errors (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id  UUID NOT NULL,
    task_id       VARCHAR(100),
    step_id       VARCHAR(50),
    call_id       VARCHAR(100),
    purpose       VARCHAR(20),
    error_kind    VARCHAR(50),
    message       TEXT,
    recoverable   BOOLEAN,
    model         VARCHAR(100),
    tokens_used   INT,
    latency_ms    BIGINT,
    occurred_at   TIMESTAMP,
    created_at    TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_model_errors_execution_id ON agent_model_errors(execution_id);
CREATE INDEX IF NOT EXISTS idx_agent_model_errors_task_id ON agent_model_errors(task_id);
