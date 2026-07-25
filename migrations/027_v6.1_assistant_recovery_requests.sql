-- V6.1 durable recovery decisions for backend-declared recoverable tool blockers.
CREATE TABLE IF NOT EXISTS assistant_recovery_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recovery_id VARCHAR(100) NOT NULL UNIQUE,
    session_id VARCHAR(100) NOT NULL,
    run_id VARCHAR(100) NOT NULL,
    message_id VARCHAR(100),
    step_id VARCHAR(100),
    tool_call_id VARCHAR(100) NOT NULL,
    tool_name VARCHAR(160) NOT NULL,
    code VARCHAR(100) NOT NULL,
    category VARCHAR(64) NOT NULL,
    risk_level VARCHAR(20) NOT NULL,
    summary TEXT NOT NULL,
    detail TEXT,
    original_query TEXT,
    original_args JSONB NOT NULL DEFAULT '{}'::jsonb,
    context JSONB NOT NULL DEFAULT '{}'::jsonb,
    actions JSONB NOT NULL DEFAULT '[]'::jsonb,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    selected_action_id VARCHAR(100),
    decision_input JSONB NOT NULL DEFAULT '{}'::jsonb,
    resolution_result JSONB NOT NULL DEFAULT '{}'::jsonb,
    requested_by VARCHAR(100),
    decided_by VARCHAR(100),
    resume_run_id VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decided_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_assistant_recovery_session
    ON assistant_recovery_requests(session_id);
CREATE INDEX IF NOT EXISTS idx_assistant_recovery_run
    ON assistant_recovery_requests(run_id);
CREATE INDEX IF NOT EXISTS idx_assistant_recovery_tool_call
    ON assistant_recovery_requests(tool_call_id);
CREATE INDEX IF NOT EXISTS idx_assistant_recovery_code
    ON assistant_recovery_requests(code);
CREATE INDEX IF NOT EXISTS idx_assistant_recovery_category
    ON assistant_recovery_requests(category);
CREATE INDEX IF NOT EXISTS idx_assistant_recovery_status
    ON assistant_recovery_requests(status);
CREATE INDEX IF NOT EXISTS idx_assistant_recovery_resume_run
    ON assistant_recovery_requests(resume_run_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_assistant_recovery_active_tool_code
    ON assistant_recovery_requests(tool_call_id, code)
    WHERE status IN ('pending', 'executing', 'paused');
