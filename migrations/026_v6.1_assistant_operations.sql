-- V6.1 durable operation ledger for asynchronous high-level assistant tools.

ALTER TABLE assistant_tool_calls
    ADD COLUMN IF NOT EXISTS run_id VARCHAR(100);

CREATE INDEX IF NOT EXISTS idx_assistant_tool_calls_run
    ON assistant_tool_calls(run_id);

CREATE TABLE IF NOT EXISTS assistant_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(80) NOT NULL,
    session_id VARCHAR(100),
    run_id VARCHAR(100),
    workflow_id VARCHAR(80) NOT NULL,
    workflow_version VARCHAR(20) NOT NULL,
    status VARCHAR(32) NOT NULL,
    current_stage VARCHAR(80),
    terminal BOOLEAN NOT NULL DEFAULT FALSE,
    request JSONB NOT NULL DEFAULT '{}'::jsonb,
    resolved_scope JSONB NOT NULL DEFAULT '{}'::jsonb,
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    counts JSONB NOT NULL DEFAULT '{}'::jsonb,
    domain_references JSONB NOT NULL DEFAULT '{}'::jsonb,
    violations JSONB NOT NULL DEFAULT '[]'::jsonb,
    task_group_id UUID,
    idempotency_key VARCHAR(160),
    created_by VARCHAR(100),
    error_code VARCHAR(80),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_assistant_operations_type
    ON assistant_operations(type);
CREATE INDEX IF NOT EXISTS idx_assistant_operations_session
    ON assistant_operations(session_id);
CREATE INDEX IF NOT EXISTS idx_assistant_operations_run
    ON assistant_operations(run_id);
CREATE INDEX IF NOT EXISTS idx_assistant_operations_workflow
    ON assistant_operations(workflow_id);
CREATE INDEX IF NOT EXISTS idx_assistant_operations_status
    ON assistant_operations(status);
CREATE INDEX IF NOT EXISTS idx_assistant_operations_task_group
    ON assistant_operations(task_group_id);
CREATE INDEX IF NOT EXISTS idx_assistant_operations_idempotency
    ON assistant_operations(idempotency_key);
CREATE UNIQUE INDEX IF NOT EXISTS uq_assistant_operations_idempotency_scope
    ON assistant_operations(session_id, workflow_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';
