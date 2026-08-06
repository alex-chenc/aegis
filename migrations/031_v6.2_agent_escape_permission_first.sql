-- V6.2 permission-first escape detection.
-- This migration is repeatable and intentionally removes findings produced by
-- the retired isolation-drift/proc-cgroup detector. New findings are written
-- only after a restricted-session permission boundary and Hook/PID evidence
-- have both matched.

ALTER TABLE IF EXISTS agent_behavior_sessions
    ADD COLUMN IF NOT EXISTS permission JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Replace the retired profile-level isolation rule lists so a newly generated
-- bundle cannot provision the old cgroup/namespace drift detector. Digests
-- match the V6.2 immutable profile manifests in api-server and Agent.
UPDATE agent_guard_adapter_profiles
SET default_escape_rules = '["access_outside_workspace","network_boundary_violation","access_container_runtime_socket","process_boundary_operation"]'::jsonb,
    digest = CASE profile_key
        WHEN 'codex-linux' THEN 'sha256:5e2058f4656ea4d7540ac1a662b4806edcc46aa9a860ac38ca65c1bb27deb629'
        WHEN 'openclaw-linux' THEN 'sha256:e6f916a2eb9b4fab6efd72f539efa7d7c9d51ab2f2d14255a55689249b9cfb79'
        WHEN 'hermes-linux' THEN 'sha256:eccaf4fdc6287ff8cfb74e03c3c15aa86304d32d2f2401794d7f31b6fbfb9166'
        WHEN 'claude-code-linux' THEN 'sha256:94eb603baadec817c6e03857064fbe809aa5da42d612d9dd7e8b486f66cb63a7'
        WHEN 'opencode-linux' THEN 'sha256:b0fff61d935a97de75d5e90c658248201117608d256cd9be3f9a30d1ee3a34c2'
        WHEN 'gemini-cli-linux' THEN 'sha256:300f72f233925ac36203a8a6d6ad4d8aa3247b93cf03474e8cede761315f5f66'
        WHEN 'zcode-linux' THEN 'sha256:dd1e0a0d89bf1fdb6152ce92c57ef7cf460c9f49f1402b503348bba407ff8c2f'
        ELSE digest
    END
WHERE profile_key IN ('codex-linux','openclaw-linux','hermes-linux','claude-code-linux','opencode-linux','gemini-cli-linux','zcode-linux');

-- The old detector was not session/permission scoped and therefore cannot be
-- safely shown in the new session detail view. Analysis rows cascade from the
-- finding; action references are nulled by the existing FK.
DELETE FROM agent_security_findings
WHERE finding_key LIKE 'escape:v1:%';

CREATE INDEX IF NOT EXISTS idx_agent_behavior_sessions_permission_class
    ON agent_behavior_sessions ((permission ->> 'class'));
