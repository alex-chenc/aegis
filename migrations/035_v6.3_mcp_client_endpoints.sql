-- V6.3 MCP client-specific endpoints and runtime grants.
CREATE TABLE IF NOT EXISTS mcp_client_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id UUID NOT NULL REFERENCES mcp_clients(id),
    token_prefix VARCHAR(32) NOT NULL UNIQUE,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_by VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_mcp_client_credentials_client_status
    ON mcp_client_credentials(client_id, status);

-- A client endpoint is intentionally single-service: at most one active
-- grant can be used by a client at any time.
CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_one_active_grant_per_client
    ON mcp_client_grants(client_id)
    WHERE status = 'active';
