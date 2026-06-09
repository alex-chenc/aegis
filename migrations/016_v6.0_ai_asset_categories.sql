-- V6.0 AI asset categories for host_application_assets.
-- V5.8 created a category CHECK constraint before AI asset categories existed.

ALTER TABLE host_application_assets
    DROP CONSTRAINT IF EXISTS chk_host_application_category;

ALTER TABLE host_application_assets
    ADD CONSTRAINT chk_host_application_category CHECK (
        category IN (
            'database',
            'web_service',
            'web_framework',
            'web_site',
            'llm_service',
            'ai_agent',
            'mcp_server',
            'other',
            'unknown'
        )
    );
