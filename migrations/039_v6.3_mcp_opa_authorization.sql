-- V6.3 OPA call-before authorization. Additive and safe to apply after the
-- existing MCP control-plane/audit migrations.

-- Existing volumes can have auth_users.admin but an empty role_permissions
-- table because the legacy seed only ran during initial database creation.
-- Restore the documented bootstrap administrator role for that one account;
-- do not grant MCP policy permissions to other users implicitly.
CREATE TABLE IF NOT EXISTS role_permissions (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    role VARCHAR(32) NOT NULL CHECK (role IN ('security_analyst', 'security_developer', 'admin')),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id)
);
INSERT INTO role_permissions (user_id, role)
SELECT username, 'admin' FROM auth_users
WHERE username = 'admin'
ON CONFLICT (user_id) DO NOTHING;

ALTER TABLE mcp_policy_versions
    ADD COLUMN IF NOT EXISTS contract_version VARCHAR(64) NOT NULL DEFAULT 'aegis.mcp.authz.input.v1',
    ADD COLUMN IF NOT EXISTS template_version VARCHAR(32) NOT NULL DEFAULT 'v1',
    ADD COLUMN IF NOT EXISTS capabilities_digest VARCHAR(80),
    ADD COLUMN IF NOT EXISTS opa_version VARCHAR(32),
    ADD COLUMN IF NOT EXISTS valid_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS validated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS tested_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS mcp_policy_deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    catalog_id UUID NOT NULL REFERENCES mcp_catalogs(id),
    client_id UUID REFERENCES mcp_clients(id),
    active_version_id UUID NOT NULL REFERENCES mcp_policy_versions(id),
    config_digest VARCHAR(80) NOT NULL,
    generation BIGINT NOT NULL DEFAULT 1,
    mode VARCHAR(16) NOT NULL DEFAULT 'enforce',
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    activated_by VARCHAR(100) NOT NULL,
    activated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(catalog_id, client_id)
);
CREATE INDEX IF NOT EXISTS idx_mcp_policy_deployments_generation
    ON mcp_policy_deployments(generation, status);

CREATE TABLE IF NOT EXISTS mcp_policy_instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id UUID NOT NULL REFERENCES mcp_policy_deployments(id),
    instance_id VARCHAR(128) NOT NULL,
    prepared_digest VARCHAR(80),
    active_digest VARCHAR(80),
    prepared_generation BIGINT,
    active_generation BIGINT,
    status VARCHAR(32) NOT NULL DEFAULT 'unknown',
    last_error_code VARCHAR(64),
    heartbeat_at TIMESTAMPTZ,
    UNIQUE(deployment_id, instance_id)
);

CREATE TABLE IF NOT EXISTS mcp_authorization_decisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id UUID NOT NULL,
    decision_id UUID NOT NULL UNIQUE,
    invocation_id UUID REFERENCES mcp_invocations(id),
    client_id UUID REFERENCES mcp_clients(id),
    credential_id UUID REFERENCES mcp_client_credentials(id),
    grant_id UUID REFERENCES mcp_client_grants(id),
    catalog_release_id UUID REFERENCES mcp_catalog_releases(id),
    release_tool_id UUID REFERENCES mcp_catalog_release_tools(id),
    tool_revision_id UUID REFERENCES mcp_tool_revisions(id),
    user_id VARCHAR(100),
    user_verified BOOLEAN NOT NULL DEFAULT false,
    policy_revision VARCHAR(128),
    policy_digest VARCHAR(80),
    deployment_generation BIGINT,
    outcome VARCHAR(32) NOT NULL,
    reason_code VARCHAR(64) NOT NULL,
    deny_rule_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    audit_rule_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    request_digest VARCHAR(80),
    evaluator_elapsed_ms BIGINT,
    upstream_started BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_mcp_authz_decisions_client_time
    ON mcp_authorization_decisions(client_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_mcp_authz_decisions_outcome_time
    ON mcp_authorization_decisions(outcome, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_mcp_authz_decisions_policy
    ON mcp_authorization_decisions(policy_revision, created_at DESC);

ALTER TABLE mcp_invocations
    ADD COLUMN IF NOT EXISTS authorization_decision_id UUID REFERENCES mcp_authorization_decisions(decision_id),
    ADD COLUMN IF NOT EXISTS upstream_started BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS completion_unknown BOOLEAN NOT NULL DEFAULT false;
CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_invocations_authz_decision
    ON mcp_invocations(authorization_decision_id)
    WHERE authorization_decision_id IS NOT NULL;

ALTER TABLE mcp_client_grants
    ADD COLUMN IF NOT EXISTS authorization_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE mcp_client_credentials
    ADD COLUMN IF NOT EXISTS authorization_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE mcp_servers
    ADD COLUMN IF NOT EXISTS authorization_version BIGINT NOT NULL DEFAULT 1;
