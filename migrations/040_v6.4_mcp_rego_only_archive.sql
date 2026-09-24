-- MCP Rego-only runtime cutover. Keep 034/036 tables and their foreign keys
-- intact for historical audit reads; the runtime no longer reads or writes
-- these legacy matcher tables.
ALTER TABLE mcp_rule_definitions
    ADD COLUMN IF NOT EXISTS legacy_frozen BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE mcp_rule_hits
    ADD COLUMN IF NOT EXISTS legacy_frozen BOOLEAN NOT NULL DEFAULT TRUE;

CREATE TABLE IF NOT EXISTS mcp_legacy_rule_definitions (LIKE mcp_rule_definitions INCLUDING ALL);
CREATE TABLE IF NOT EXISTS mcp_legacy_rule_hits (LIKE mcp_rule_hits INCLUDING ALL);

INSERT INTO mcp_legacy_rule_definitions
SELECT * FROM mcp_rule_definitions
ON CONFLICT (id) DO NOTHING;
INSERT INTO mcp_legacy_rule_hits
SELECT * FROM mcp_rule_hits
ON CONFLICT (id) DO NOTHING;

ALTER TABLE mcp_security_verdicts
    ADD COLUMN IF NOT EXISTS engine VARCHAR(32) NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS source VARCHAR(32) NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS phase VARCHAR(16) NOT NULL DEFAULT 'post',
    ADD COLUMN IF NOT EXISTS action VARCHAR(16) NOT NULL DEFAULT 'allow',
    ADD COLUMN IF NOT EXISTS policy_revision VARCHAR(128),
    ADD COLUMN IF NOT EXISTS rule_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS audit_rule_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Historical rows remain explicitly identifiable and are never re-evaluated.
UPDATE mcp_security_verdicts SET engine = 'legacy', source = 'legacy'
WHERE engine IS NULL OR engine = '';
