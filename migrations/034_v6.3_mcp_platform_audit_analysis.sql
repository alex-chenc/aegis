-- V6.3 MCP invocation evidence and analysis projections.
CREATE TABLE IF NOT EXISTS mcp_invocation_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invocation_id UUID NOT NULL REFERENCES mcp_invocations(id),
    event_seq BIGINT NOT NULL,
    event_type VARCHAR(48) NOT NULL,
    status VARCHAR(32) NOT NULL,
    trace_id VARCHAR(128),
    digest VARCHAR(80),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(invocation_id, event_seq)
);

CREATE TABLE IF NOT EXISTS mcp_invocation_payload_refs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invocation_id UUID NOT NULL REFERENCES mcp_invocations(id),
    stage VARCHAR(48) NOT NULL,
    object_ref VARCHAR(512) NOT NULL,
    digest VARCHAR(80) NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    classification VARCHAR(32),
    status VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(invocation_id, stage)
);

CREATE TABLE IF NOT EXISTS mcp_audit_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL UNIQUE,
    topic VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    attempt INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_mcp_audit_outbox_status ON mcp_audit_outbox(status, next_attempt_at);

CREATE TABLE IF NOT EXISTS mcp_rule_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_key VARCHAR(128) NOT NULL,
    version BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    phase VARCHAR(16) NOT NULL,
    severity VARCHAR(16) NOT NULL,
    definition JSONB NOT NULL DEFAULT '{}'::jsonb,
    digest VARCHAR(80) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(rule_key, version)
);

CREATE TABLE IF NOT EXISTS mcp_rule_hits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invocation_id UUID NOT NULL REFERENCES mcp_invocations(id),
    rule_definition_id UUID NOT NULL REFERENCES mcp_rule_definitions(id),
    severity VARCHAR(16) NOT NULL,
    phase VARCHAR(16) NOT NULL,
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_mcp_rule_hits_invocation ON mcp_rule_hits(invocation_id, created_at DESC);

CREATE TABLE IF NOT EXISTS mcp_ai_analysis_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invocation_id UUID NOT NULL REFERENCES mcp_invocations(id),
    activity_id UUID,
    status VARCHAR(32) NOT NULL,
    model VARCHAR(160),
    verdict VARCHAR(32),
    error_code VARCHAR(100),
    attempts INTEGER NOT NULL DEFAULT 0,
    lease_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_mcp_ai_runs_status ON mcp_ai_analysis_runs(status, lease_until);

CREATE TABLE IF NOT EXISTS mcp_ai_analysis_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES mcp_ai_analysis_runs(id),
    chunk_no INTEGER NOT NULL,
    input_digest VARCHAR(80) NOT NULL,
    status VARCHAR(32) NOT NULL,
    output JSONB NOT NULL DEFAULT '{}'::jsonb,
    input_tokens INTEGER,
    output_tokens INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    UNIQUE(run_id, chunk_no)
);
