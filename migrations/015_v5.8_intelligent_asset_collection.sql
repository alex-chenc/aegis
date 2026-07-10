-- =====================================================
-- V5.8 Intelligent Asset Collection
-- =====================================================

-- 1. Asset Collection Configs
CREATE TABLE IF NOT EXISTS asset_collection_configs (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    enabled            BOOLEAN NOT NULL DEFAULT true,
    interval_hours     INT NOT NULL DEFAULT 12,
    collect_types      JSONB NOT NULL DEFAULT '["process","application_analysis"]',
    scope              VARCHAR(32) NOT NULL DEFAULT 'all_hosts',
    next_run_at        TIMESTAMPTZ,
    last_run_at        TIMESTAMPTZ,
    updated_by         VARCHAR(100),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_asset_collection_config_interval CHECK (interval_hours >= 1 AND interval_hours <= 168),
    CONSTRAINT chk_asset_collection_config_scope CHECK (scope IN ('all_hosts','host_group','hosts'))
);

CREATE INDEX IF NOT EXISTS idx_asset_collection_configs_next_run
    ON asset_collection_configs(enabled, next_run_at);

-- Insert default config if not exists
INSERT INTO asset_collection_configs (enabled, interval_hours, collect_types, scope)
SELECT true, 12, '["process","application_analysis"]'::jsonb, 'all_hosts'
WHERE NOT EXISTS (SELECT 1 FROM asset_collection_configs);

-- 2. Asset Collection Tasks
CREATE TABLE IF NOT EXISTS asset_collection_tasks (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_type          VARCHAR(32) NOT NULL DEFAULT 'full',
    trigger_source     VARCHAR(32) NOT NULL DEFAULT 'manual',
    scope              VARCHAR(32) NOT NULL DEFAULT 'hosts',
    host_filter        JSONB NOT NULL DEFAULT '[]',
    collect_types      JSONB NOT NULL DEFAULT '["process","application_analysis"]',
    status             VARCHAR(32) NOT NULL DEFAULT 'collecting',
    total_hosts        INT NOT NULL DEFAULT 0,
    success_hosts      INT NOT NULL DEFAULT 0,
    failed_hosts       INT NOT NULL DEFAULT 0,
    current_stage      VARCHAR(64),
    error_message      TEXT,
    requested_by       VARCHAR(100),
    started_at         TIMESTAMPTZ,
    finished_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_asset_collection_task_status CHECK (
        status IN ('collecting','analyzing','completed','failed','cancelled')
    )
);

CREATE INDEX IF NOT EXISTS idx_asset_collection_tasks_status
    ON asset_collection_tasks(status);
CREATE INDEX IF NOT EXISTS idx_asset_collection_tasks_created_at
    ON asset_collection_tasks(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_asset_collection_tasks_host_filter
    ON asset_collection_tasks USING GIN(host_filter);

-- 3. Asset Collection Task Hosts
CREATE TABLE IF NOT EXISTS asset_collection_task_hosts (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID NOT NULL REFERENCES asset_collection_tasks(id) ON DELETE CASCADE,
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    hostname           VARCHAR(255),
    ip_address         VARCHAR(45),
    status             VARCHAR(32) NOT NULL DEFAULT 'collecting',
    collect_started_at TIMESTAMPTZ,
    collect_finished_at TIMESTAMPTZ,
    software_count     INT NOT NULL DEFAULT 0,
    process_count      INT NOT NULL DEFAULT 0,
    application_count  INT NOT NULL DEFAULT 0,
    error_message      TEXT,
    raw_snapshot_id    UUID,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(task_id, host_id)
);

CREATE INDEX IF NOT EXISTS idx_asset_collection_task_hosts_task
    ON asset_collection_task_hosts(task_id);
CREATE INDEX IF NOT EXISTS idx_asset_collection_task_hosts_host
    ON asset_collection_task_hosts(host_id);
CREATE INDEX IF NOT EXISTS idx_asset_collection_task_hosts_status
    ON asset_collection_task_hosts(status);

-- 4. Host Software Assets
CREATE TABLE IF NOT EXISTS host_software_assets (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    hostname           VARCHAR(255),
    ip_address         VARCHAR(45),
    group_name         VARCHAR(255) NOT NULL DEFAULT '默认分组',
    os_type            VARCHAR(50) NOT NULL,
    package_manager    VARCHAR(32) NOT NULL,
    name               VARCHAR(255) NOT NULL,
    version            VARCHAR(255),
    release            VARCHAR(255),
    epoch              VARCHAR(64),
    architecture       VARCHAR(64),
    source_name        VARCHAR(255),
    vendor             VARCHAR(255),
    license            VARCHAR(255),
    install_paths      JSONB NOT NULL DEFAULT '[]',
    file_count         INT NOT NULL DEFAULT 0,
    package_metadata   JSONB NOT NULL DEFAULT '{}',
    fingerprint        VARCHAR(128) NOT NULL,
    status             VARCHAR(32) NOT NULL DEFAULT 'active',
    last_modified_at   TIMESTAMPTZ,
    first_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    collected_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_host_software_package_manager CHECK (package_manager IN ('rpm','dpkg','apk','unknown')),
    CONSTRAINT chk_host_software_status CHECK (status IN ('active','inactive','deleted')),
    UNIQUE(host_id, package_manager, fingerprint)
);

CREATE INDEX IF NOT EXISTS idx_host_software_assets_host
    ON host_software_assets(host_id);
CREATE INDEX IF NOT EXISTS idx_host_software_assets_name
    ON host_software_assets(name);
CREATE INDEX IF NOT EXISTS idx_host_software_assets_version
    ON host_software_assets(version);
CREATE INDEX IF NOT EXISTS idx_host_software_assets_manager
    ON host_software_assets(package_manager);
CREATE INDEX IF NOT EXISTS idx_host_software_assets_seen
    ON host_software_assets(last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_host_software_assets_paths
    ON host_software_assets USING GIN(install_paths);

-- 5. Host Process Snapshots
CREATE TABLE IF NOT EXISTS host_process_snapshots (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID REFERENCES asset_collection_tasks(id) ON DELETE SET NULL,
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    hostname           VARCHAR(255),
    ip_address         VARCHAR(45),
    process_count      INT NOT NULL DEFAULT 0,
    listen_port_count  INT NOT NULL DEFAULT 0,
    snapshot_hash      VARCHAR(64) NOT NULL,
    snapshot_json      JSONB NOT NULL DEFAULT '{}',
    redaction_summary  JSONB NOT NULL DEFAULT '{}',
    collected_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_host_process_snapshots_host
    ON host_process_snapshots(host_id, collected_at DESC);
CREATE INDEX IF NOT EXISTS idx_host_process_snapshots_task
    ON host_process_snapshots(task_id);
CREATE INDEX IF NOT EXISTS idx_host_process_snapshots_hash
    ON host_process_snapshots(snapshot_hash);

-- 6. Host Application Assets
CREATE TABLE IF NOT EXISTS host_application_assets (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    hostname           VARCHAR(255),
    ip_address         VARCHAR(45),
    group_name         VARCHAR(255) NOT NULL DEFAULT '默认分组',
    os_type            VARCHAR(50) NOT NULL,
    category           VARCHAR(32) NOT NULL DEFAULT 'unknown',
    name               VARCHAR(255) NOT NULL,
    display_name       VARCHAR(255),
    version            VARCHAR(255),
    version_source     VARCHAR(64),
    install_path       TEXT,
    start_path         TEXT,
    config_paths       JSONB NOT NULL DEFAULT '[]',
    site_paths         JSONB NOT NULL DEFAULT '[]',
    domains            JSONB NOT NULL DEFAULT '[]',
    listen_ports       JSONB NOT NULL DEFAULT '[]',
    run_user           VARCHAR(255),
    runtime_name       VARCHAR(100),
    runtime_version    VARCHAR(100),
    framework_name     VARCHAR(100),
    framework_version  VARCHAR(100),
    related_pids       JSONB NOT NULL DEFAULT '[]',
    is_container       BOOLEAN NOT NULL DEFAULT FALSE,
    container_id       VARCHAR(128),
    container_runtime  VARCHAR(64),
    related_packages   JSONB NOT NULL DEFAULT '[]',
    ai_confidence      NUMERIC(4,3) NOT NULL DEFAULT 0,
    ai_evidence        JSONB NOT NULL DEFAULT '[]',
    ai_raw_output      JSONB NOT NULL DEFAULT '{}',
    manual_overrides   JSONB NOT NULL DEFAULT '{}',
    review_status      VARCHAR(32) NOT NULL DEFAULT 'auto',
    status             VARCHAR(32) NOT NULL DEFAULT 'active',
    fingerprint        VARCHAR(128) NOT NULL,
    first_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    collected_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_host_application_category CHECK (
        category IN ('database','web_service','web_framework','web_site','other','unknown')
    ),
    CONSTRAINT chk_host_application_review CHECK (
        review_status IN ('pending','confirmed','rejected','auto')
    ),
    CONSTRAINT chk_host_application_status CHECK (
        status IN ('active','inactive','deleted','needs_review')
    ),
    UNIQUE(host_id, fingerprint)
);

CREATE INDEX IF NOT EXISTS idx_host_application_assets_host
    ON host_application_assets(host_id);
CREATE INDEX IF NOT EXISTS idx_host_application_assets_category
    ON host_application_assets(category);
CREATE INDEX IF NOT EXISTS idx_host_application_assets_name
    ON host_application_assets(name);
CREATE INDEX IF NOT EXISTS idx_host_application_assets_version
    ON host_application_assets(version);
CREATE INDEX IF NOT EXISTS idx_host_application_assets_ports
    ON host_application_assets USING GIN(listen_ports);
CREATE INDEX IF NOT EXISTS idx_host_application_assets_review
    ON host_application_assets(review_status);
CREATE INDEX IF NOT EXISTS idx_host_application_assets_seen
    ON host_application_assets(last_seen_at DESC);

-- 7. Host Application Tool Calls
CREATE TABLE IF NOT EXISTS host_application_tool_calls (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID REFERENCES asset_collection_tasks(id) ON DELETE SET NULL,
    application_id     UUID REFERENCES host_application_assets(id) ON DELETE SET NULL,
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    call_id            VARCHAR(128) NOT NULL,
    tool_name          VARCHAR(128) NOT NULL,
    arguments_json     JSONB NOT NULL DEFAULT '{}',
    result_json        JSONB NOT NULL DEFAULT '{}',
    success            BOOLEAN NOT NULL DEFAULT false,
    error_message      TEXT,
    execution_time_ms  BIGINT NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(call_id)
);

CREATE INDEX IF NOT EXISTS idx_host_application_tool_calls_app
    ON host_application_tool_calls(application_id);
CREATE INDEX IF NOT EXISTS idx_host_application_tool_calls_host
    ON host_application_tool_calls(host_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_host_application_tool_calls_tool
    ON host_application_tool_calls(tool_name);

-- 8. Extend host_vulnerabilities with asset references
ALTER TABLE host_vulnerabilities
    ADD COLUMN IF NOT EXISTS software_asset_id UUID REFERENCES host_software_assets(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS application_asset_id UUID REFERENCES host_application_assets(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS asset_name VARCHAR(255),
    ADD COLUMN IF NOT EXISTS asset_version VARCHAR(255),
    ADD COLUMN IF NOT EXISTS asset_collected_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS vulnerability_source JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS match_evidence JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS verification_status VARCHAR(32) NOT NULL DEFAULT 'verified';

CREATE INDEX IF NOT EXISTS idx_host_vulnerabilities_software_asset
    ON host_vulnerabilities(software_asset_id);
CREATE INDEX IF NOT EXISTS idx_host_vulnerabilities_application_asset
    ON host_vulnerabilities(application_asset_id);
CREATE INDEX IF NOT EXISTS idx_host_vulnerabilities_verification_status
    ON host_vulnerabilities(verification_status);
