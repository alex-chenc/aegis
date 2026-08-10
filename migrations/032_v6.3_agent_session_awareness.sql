-- V6.3 Agent Session Awareness
-- Static snapshots from ~/.claude/projects and ~/.codex/sessions. Raw paths and
-- unredacted payloads are intentionally not persisted.

CREATE TABLE IF NOT EXISTS agent_conversation_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    agent_type VARCHAR(32) NOT NULL CHECK (agent_type IN ('claude-code', 'codex')),
    source_mode VARCHAR(24) NOT NULL DEFAULT 'static_scan',
    source_subject_uid BIGINT NOT NULL,
    external_session_id VARCHAR(255) NOT NULL,
    project_digest VARCHAR(80),
    title TEXT,
    model VARCHAR(128),
    state VARCHAR(32) NOT NULL DEFAULT 'unknown',
    first_seen_at TIMESTAMPTZ,
    last_seen_at TIMESTAMPTZ,
    last_item_at TIMESTAMPTZ,
    item_count BIGINT NOT NULL DEFAULT 0,
    prompt_count BIGINT NOT NULL DEFAULT 0,
    assistant_count BIGINT NOT NULL DEFAULT 0,
    tool_call_count BIGINT NOT NULL DEFAULT 0,
    estimated_input_tokens BIGINT NOT NULL DEFAULT 0,
    estimated_output_tokens BIGINT NOT NULL DEFAULT 0,
    estimated_total_tokens BIGINT NOT NULL DEFAULT 0,
    token_estimate_method VARCHAR(32) NOT NULL DEFAULT 'chars_div_4',
    risk_level VARCHAR(16) NOT NULL DEFAULT 'unknown',
    rule_hit_count BIGINT NOT NULL DEFAULT 0,
    ai_risk_score NUMERIC(5,2),
    last_sequence BIGINT NOT NULL DEFAULT -1,
    last_collected_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (host_id, agent_type, source_subject_uid, external_session_id)
);
CREATE INDEX IF NOT EXISTS idx_agent_conv_sessions_last_seen
    ON agent_conversation_sessions (last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_conv_sessions_risk
    ON agent_conversation_sessions (risk_level, updated_at DESC);

CREATE TABLE IF NOT EXISTS agent_conversation_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES agent_conversation_sessions(id) ON DELETE CASCADE,
    item_id VARCHAR(128) NOT NULL,
    sequence BIGINT NOT NULL,
    item_type VARCHAR(32) NOT NULL,
    role VARCHAR(32),
    occurred_at TIMESTAMPTZ,
    content_digest VARCHAR(80),
    content_redacted TEXT,
    normalized_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    visibility VARCHAR(16) NOT NULL DEFAULT 'normal',
    redaction_applied BOOLEAN NOT NULL DEFAULT false,
    input_tokens BIGINT,
    output_tokens BIGINT,
    total_tokens BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (session_id, item_id),
    UNIQUE (session_id, sequence)
);
CREATE INDEX IF NOT EXISTS idx_agent_conv_items_session_seq
    ON agent_conversation_items (session_id, sequence);

CREATE TABLE IF NOT EXISTS agent_conversation_collection_cursors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    source VARCHAR(32) NOT NULL,
    source_subject_uid BIGINT NOT NULL,
    file_identity VARCHAR(255) NOT NULL,
    device BIGINT,
    inode BIGINT,
    byte_offset BIGINT NOT NULL DEFAULT 0,
    last_mtime TIMESTAMPTZ,
    last_scan_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    parser_version VARCHAR(64) NOT NULL,
    UNIQUE (host_id, source, source_subject_uid, file_identity)
);

CREATE TABLE IF NOT EXISTS agent_session_rule_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_key VARCHAR(128) NOT NULL,
    version BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    category VARCHAR(64) NOT NULL,
    severity VARCHAR(16) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    pattern JSONB NOT NULL DEFAULT '{}'::jsonb,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (rule_key, version)
);

CREATE TABLE IF NOT EXISTS agent_session_rule_hits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES agent_conversation_sessions(id) ON DELETE CASCADE,
    item_id UUID REFERENCES agent_conversation_items(id) ON DELETE SET NULL,
    rule_id UUID REFERENCES agent_session_rule_definitions(id) ON DELETE SET NULL,
    rule_key VARCHAR(128) NOT NULL,
    severity VARCHAR(16) NOT NULL,
    category VARCHAR(64) NOT NULL,
    evidence_digest VARCHAR(80),
    evidence_excerpt TEXT,
    status VARCHAR(24) NOT NULL DEFAULT 'open',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (session_id, item_id, rule_key, evidence_digest)
);
CREATE INDEX IF NOT EXISTS idx_agent_session_rule_hits_session
    ON agent_session_rule_hits (session_id, created_at DESC);

CREATE TABLE IF NOT EXISTS agent_session_ai_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES agent_conversation_sessions(id) ON DELETE CASCADE,
    provider VARCHAR(64) NOT NULL,
    model VARCHAR(128) NOT NULL,
    prompt_version VARCHAR(64) NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'queued',
    chunk_count INT NOT NULL DEFAULT 0,
    risk_score NUMERIC(5,2),
    summary TEXT,
    error_code VARCHAR(64),
    error_message TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_agent_session_ai_runs_session
    ON agent_session_ai_runs (session_id, created_at DESC);

CREATE TABLE IF NOT EXISTS agent_session_ai_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES agent_session_ai_runs(id) ON DELETE CASCADE,
    chunk_index INT NOT NULL,
    item_start_sequence BIGINT NOT NULL,
    item_end_sequence BIGINT NOT NULL,
    input_token_estimate BIGINT NOT NULL DEFAULT 0,
    output_json JSONB,
    status VARCHAR(24) NOT NULL DEFAULT 'queued',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (run_id, chunk_index)
);

CREATE TABLE IF NOT EXISTS agent_session_risk_markings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES agent_conversation_sessions(id) ON DELETE CASCADE,
    source VARCHAR(16) NOT NULL CHECK (source IN ('rule', 'ai', 'manual')),
    level VARCHAR(16) NOT NULL,
    score NUMERIC(5,2),
    reason TEXT,
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_agent_session_risk_markings_session
    ON agent_session_risk_markings (session_id, created_at DESC);
