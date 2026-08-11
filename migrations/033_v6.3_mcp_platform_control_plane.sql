-- Aegis V6.3 MCP aggregation control plane.
-- Idempotent by design. Historical MCP records are never dropped by rollback.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS mcp_servers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_key VARCHAR(100) NOT NULL UNIQUE,
    display_name VARCHAR(160) NOT NULL,
    owner_team_id UUID,
    owner_user_id VARCHAR(100) NOT NULL,
    environment VARCHAR(32) NOT NULL,
    transport VARCHAR(32) NOT NULL,
    endpoint_url TEXT NOT NULL,
    endpoint_display VARCHAR(255) NOT NULL,
    credential_ref VARCHAR(255),
    auth_type VARCHAR(32) NOT NULL,
    protocol_version VARCHAR(64),
    risk_tier VARCHAR(8) NOT NULL DEFAULT 'l2',
    lifecycle_status VARCHAR(40) NOT NULL,
    active_revision_id UUID,
    tool_count INTEGER NOT NULL DEFAULT 0,
    published_tool_count INTEGER NOT NULL DEFAULT 0,
    last_health_status VARCHAR(32),
    last_error_code VARCHAR(100),
    last_error_message TEXT,
    last_synced_at TIMESTAMPTZ,
    created_by VARCHAR(100) NOT NULL,
    updated_by VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_mcp_servers_status ON mcp_servers(lifecycle_status);
CREATE INDEX IF NOT EXISTS idx_mcp_servers_environment ON mcp_servers(environment);
CREATE INDEX IF NOT EXISTS idx_mcp_servers_risk ON mcp_servers(risk_tier);

CREATE TABLE IF NOT EXISTS mcp_onboarding_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key VARCHAR(128) NOT NULL UNIQUE,
    server_id UUID REFERENCES mcp_servers(id),
    display_name VARCHAR(160) NOT NULL,
    endpoint_url TEXT NOT NULL,
    endpoint_display VARCHAR(255) NOT NULL,
    credential_ref VARCHAR(255),
    auth_type VARCHAR(32) NOT NULL,
    owner_team_id UUID,
    owner_user_id VARCHAR(100) NOT NULL,
    environment VARCHAR(32) NOT NULL,
    target_catalog_id UUID,
    publish_policy VARCHAR(32) NOT NULL,
    status VARCHAR(40) NOT NULL,
    step VARCHAR(40) NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 0,
    error_code VARCHAR(100),
    error_message TEXT,
    revision_id UUID,
    created_by VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_mcp_onboarding_status ON mcp_onboarding_jobs(status);
CREATE INDEX IF NOT EXISTS idx_mcp_onboarding_owner ON mcp_onboarding_jobs(owner_user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS mcp_server_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id UUID NOT NULL REFERENCES mcp_servers(id),
    revision_no BIGINT NOT NULL,
    protocol_version VARCHAR(64),
    capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
    tools_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb,
    digest VARCHAR(80) NOT NULL,
    status VARCHAR(40) NOT NULL,
    discovery_error TEXT,
    created_by VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(server_id, revision_no),
    UNIQUE(server_id, digest)
);
CREATE INDEX IF NOT EXISTS idx_mcp_server_revisions_status ON mcp_server_revisions(status);

CREATE TABLE IF NOT EXISTS mcp_tool_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_revision_id UUID NOT NULL REFERENCES mcp_server_revisions(id),
    upstream_name VARCHAR(255) NOT NULL,
    alias VARCHAR(255) NOT NULL,
    title VARCHAR(255),
    description TEXT,
    input_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
    output_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
    verified_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    risk_tier VARCHAR(8) NOT NULL DEFAULT 'l2',
    status VARCHAR(32) NOT NULL DEFAULT 'discovered',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(server_revision_id, alias)
);
CREATE INDEX IF NOT EXISTS idx_mcp_tool_revisions_alias ON mcp_tool_revisions(alias);

CREATE TABLE IF NOT EXISTS mcp_admission_reviews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_type VARCHAR(32) NOT NULL,
    subject_id UUID NOT NULL,
    subject_digest VARCHAR(80) NOT NULL,
    required_roles JSONB NOT NULL DEFAULT '[]'::jsonb,
    quorum INTEGER NOT NULL DEFAULT 1,
    decision VARCHAR(32) NOT NULL DEFAULT 'pending',
    reviewer_id VARCHAR(100),
    reason TEXT,
    evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_mcp_admission_reviews_subject ON mcp_admission_reviews(subject_type, subject_id);

CREATE TABLE IF NOT EXISTS mcp_catalogs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    catalog_key VARCHAR(100) NOT NULL UNIQUE,
    display_name VARCHAR(160) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_by VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mcp_catalog_releases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    catalog_id UUID NOT NULL REFERENCES mcp_catalogs(id),
    server_revision_id UUID REFERENCES mcp_server_revisions(id),
    release_no BIGINT NOT NULL,
    manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
    manifest_digest VARCHAR(80) NOT NULL,
    status VARCHAR(32) NOT NULL,
    created_by VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    UNIQUE(catalog_id, release_no),
    UNIQUE(catalog_id, manifest_digest)
);

CREATE TABLE IF NOT EXISTS mcp_catalog_release_tools (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    release_id UUID NOT NULL REFERENCES mcp_catalog_releases(id),
    tool_revision_id UUID NOT NULL REFERENCES mcp_tool_revisions(id),
    exposed_name VARCHAR(255) NOT NULL,
    title VARCHAR(255),
    description TEXT,
    input_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
    output_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
    approval_mode VARCHAR(32) NOT NULL DEFAULT 'none',
    rate_limit JSONB NOT NULL DEFAULT '{}'::jsonb,
    resource JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(32) NOT NULL DEFAULT 'staged',
    display_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(release_id, exposed_name)
);
CREATE INDEX IF NOT EXISTS idx_mcp_release_tools_revision ON mcp_catalog_release_tools(tool_revision_id);

ALTER TABLE mcp_catalog_releases
    ADD COLUMN IF NOT EXISTS server_revision_id UUID REFERENCES mcp_server_revisions(id);
CREATE INDEX IF NOT EXISTS idx_mcp_releases_server_revision ON mcp_catalog_releases(server_revision_id);

CREATE TABLE IF NOT EXISTS mcp_clients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_key VARCHAR(100) NOT NULL UNIQUE,
    display_name VARCHAR(160) NOT NULL,
    client_type VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    created_by VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mcp_client_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id UUID NOT NULL REFERENCES mcp_clients(id),
    catalog_id UUID NOT NULL REFERENCES mcp_catalogs(id),
    tool_allowlist JSONB NOT NULL DEFAULT '[]'::jsonb,
    resource_scope JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(32) NOT NULL,
    expires_at TIMESTAMPTZ,
    created_by VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_mcp_grants_client_status ON mcp_client_grants(client_id, status);

CREATE TABLE IF NOT EXISTS mcp_policy_sets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_key VARCHAR(128) NOT NULL UNIQUE,
    display_name VARCHAR(160) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'draft',
    created_by VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mcp_policy_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_set_id UUID NOT NULL REFERENCES mcp_policy_sets(id),
    version BIGINT NOT NULL,
    language_version VARCHAR(32) NOT NULL,
    source JSONB NOT NULL DEFAULT '{}'::jsonb,
    compiled_bundle_ref VARCHAR(512),
    digest VARCHAR(80) NOT NULL,
    test_report JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(32) NOT NULL DEFAULT 'draft',
    signature TEXT,
    signing_key_id VARCHAR(128),
    created_by VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(policy_set_id, version)
);

CREATE TABLE IF NOT EXISTS mcp_approval_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    approval_type VARCHAR(32) NOT NULL,
    subject_type VARCHAR(32) NOT NULL,
    subject_id UUID NOT NULL,
    requested_by VARCHAR(100) NOT NULL,
    status VARCHAR(32) NOT NULL,
    request_digest VARCHAR(80),
    reason TEXT,
    decision_reason TEXT,
    decided_by VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_mcp_approval_status ON mcp_approval_requests(status, created_at DESC);

CREATE TABLE IF NOT EXISTS mcp_invocations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id UUID REFERENCES mcp_clients(id),
    catalog_release_id UUID REFERENCES mcp_catalog_releases(id),
    tool_revision_id UUID REFERENCES mcp_tool_revisions(id),
    user_id VARCHAR(100),
    tool_alias VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL,
    policy_decision VARCHAR(32),
    rule_status VARCHAR(32),
    ai_status VARCHAR(32),
    request_digest VARCHAR(80),
    result_digest VARCHAR(80),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_mcp_invocations_created ON mcp_invocations(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_mcp_invocations_status ON mcp_invocations(status);

CREATE TABLE IF NOT EXISTS mcp_security_verdicts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invocation_id UUID NOT NULL UNIQUE REFERENCES mcp_invocations(id),
    deterministic_severity VARCHAR(16) NOT NULL,
    ai_verdict VARCHAR(32),
    overall_risk VARCHAR(16) NOT NULL,
    evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
