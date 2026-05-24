-- V5.8 动态 eBPF DetectionPackage 数据库迁移
-- 日期: 2026-05-23

-- 1. detection_package_drafts
CREATE TABLE IF NOT EXISTS detection_package_drafts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id          VARCHAR(160) NOT NULL UNIQUE,
    target_version      VARCHAR(32)  NOT NULL,
    title               VARCHAR(255) NOT NULL,
    description         TEXT,
    cve_ids             JSONB        NOT NULL DEFAULT '[]',
    ai_generated        BOOLEAN      NOT NULL DEFAULT false,
    ai_generation_input JSONB,
    hook_plan_yaml      TEXT,
    ebpf_source         TEXT,
    sigma_rules_yaml    TEXT,
    correlation_yaml    TEXT,
    build_params        JSONB        NOT NULL DEFAULT '{}',
    status              VARCHAR(32)  NOT NULL DEFAULT 'draft',
    last_build_id       UUID,
    created_by          VARCHAR(100),
    updated_by          VARCHAR(100),
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_detection_package_drafts_status ON detection_package_drafts(status);
CREATE INDEX IF NOT EXISTS idx_detection_package_drafts_cve_ids ON detection_package_drafts USING GIN(cve_ids);

-- 2. detection_packages
CREATE TABLE IF NOT EXISTS detection_packages (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id          VARCHAR(160) NOT NULL,
    version             VARCHAR(32)  NOT NULL,
    title               VARCHAR(255) NOT NULL,
    description         TEXT,
    cve_ids             JSONB        NOT NULL DEFAULT '[]',
    status              VARCHAR(32)  NOT NULL DEFAULT 'built',
    package_object_key  TEXT,
    signature_object_key TEXT,
    package_size        BIGINT       NOT NULL DEFAULT 0,
    package_sha256      VARCHAR(64),
    signed_by           VARCHAR(100),
    signed_at           TIMESTAMP WITH TIME ZONE,
    enabled_at          TIMESTAMP WITH TIME ZONE,
    disabled_at         TIMESTAMP WITH TIME ZONE,
    build_id            UUID,
    builder_image       VARCHAR(255),
    builder_digest      VARCHAR(128),
    manifest_json       JSONB        NOT NULL DEFAULT '{}',
    hook_summary        JSONB        NOT NULL DEFAULT '[]',
    event_schema        JSONB        NOT NULL DEFAULT '{}',
    limits_json         JSONB        NOT NULL DEFAULT '{}',
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(package_id, version)
);

CREATE INDEX IF NOT EXISTS idx_detection_packages_package_id ON detection_packages(package_id);
CREATE INDEX IF NOT EXISTS idx_detection_packages_status ON detection_packages(status);
CREATE INDEX IF NOT EXISTS idx_detection_packages_cve_ids ON detection_packages USING GIN(cve_ids);

-- 3. detection_package_builds
CREATE TABLE IF NOT EXISTS detection_package_builds (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    draft_id         UUID,
    package_id       VARCHAR(160) NOT NULL,
    version          VARCHAR(32)  NOT NULL,
    status           VARCHAR(32)  NOT NULL DEFAULT 'pending',
    builder_image    VARCHAR(255) NOT NULL,
    builder_digest   VARCHAR(128),
    clang_version    VARCHAR(100),
    started_at       TIMESTAMP WITH TIME ZONE,
    finished_at      TIMESTAMP WITH TIME ZONE,
    duration_ms      BIGINT,
    artifact_summary JSONB       NOT NULL DEFAULT '{}',
    hook_summary     JSONB       NOT NULL DEFAULT '[]',
    event_schema     JSONB       NOT NULL DEFAULT '{}',
    unsigned_package_object_key TEXT,
    unsigned_package_sha256 VARCHAR(64),
    unsigned_package_size BIGINT NOT NULL DEFAULT 0,
    build_log_object_key TEXT,
    build_log        TEXT,
    error_message    TEXT,
    created_by       VARCHAR(100),
    created_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_detection_package_builds_package ON detection_package_builds(package_id, version);
CREATE INDEX IF NOT EXISTS idx_detection_package_builds_status ON detection_package_builds(status);

-- 4. detection_package_host_status
CREATE TABLE IF NOT EXISTS detection_package_host_status (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id         VARCHAR(160) NOT NULL,
    version            VARCHAR(32)  NOT NULL,
    host_id            UUID         NOT NULL,
    hostname           VARCHAR(255),
    status             VARCHAR(64)  NOT NULL DEFAULT 'pending',
    plugin_status      VARCHAR(64),
    sigma_status       VARCHAR(64),
    correlation_status VARCHAR(64),
    active_artifact    VARCHAR(16),
    loaded_hooks       JSONB        NOT NULL DEFAULT '[]',
    kernel_release     VARCHAR(128),
    arch               VARCHAR(32),
    error_message      TEXT,
    metrics_json       JSONB        NOT NULL DEFAULT '{}',
    installed_at       TIMESTAMP WITH TIME ZONE,
    updated_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_reported_at   TIMESTAMP WITH TIME ZONE,
    UNIQUE(package_id, version, host_id)
);

CREATE INDEX IF NOT EXISTS idx_detection_package_host_status_package ON detection_package_host_status(package_id, version);
CREATE INDEX IF NOT EXISTS idx_detection_package_host_status_host ON detection_package_host_status(host_id);
CREATE INDEX IF NOT EXISTS idx_detection_package_host_status_status ON detection_package_host_status(status);

-- 5. detection_package_operations
CREATE TABLE IF NOT EXISTS detection_package_operations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id    VARCHAR(160),
    version       VARCHAR(32),
    operation     VARCHAR(64)  NOT NULL,
    operator      VARCHAR(100),
    request_json  JSONB        NOT NULL DEFAULT '{}',
    result_json   JSONB        NOT NULL DEFAULT '{}',
    success       BOOLEAN      NOT NULL DEFAULT true,
    error_message TEXT,
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_detection_package_operations_package ON detection_package_operations(package_id, version);
CREATE INDEX IF NOT EXISTS idx_detection_package_operations_operation ON detection_package_operations(operation);

-- 6. ebpf_hook_allowlist_configs
CREATE TABLE IF NOT EXISTS ebpf_hook_allowlist_configs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version       BIGSERIAL UNIQUE,
    config_json   JSONB NOT NULL,
    description   TEXT,
    updated_by    VARCHAR(100),
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    activated_at  TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_ebpf_hook_allowlist_configs_activated ON ebpf_hook_allowlist_configs(activated_at DESC);

-- 7. correlation_rules
CREATE TABLE IF NOT EXISTS correlation_rules (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id       VARCHAR(160) NOT NULL,
    package_version  VARCHAR(32)  NOT NULL,
    rule_id          VARCHAR(220) NOT NULL,
    title            VARCHAR(255),
    severity         VARCHAR(32),
    by_key           VARCHAR(32),
    window_seconds   INTEGER,
    ordered          BOOLEAN      NOT NULL DEFAULT true,
    sequence_json    JSONB        NOT NULL DEFAULT '[]',
    content          TEXT         NOT NULL,
    created_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(package_id, package_version, rule_id)
);

CREATE INDEX IF NOT EXISTS idx_correlation_rules_package ON correlation_rules(package_id, package_version);

-- 8. 扩展 sigma_rules
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS package_id VARCHAR(160);
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS package_version VARCHAR(32);
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS package_rule_type VARCHAR(32) DEFAULT 'standalone';
CREATE INDEX IF NOT EXISTS idx_sigma_rules_package ON sigma_rules(package_id, package_version);

-- 9. 扩展 runtime_events
ALTER TABLE runtime_events ADD COLUMN IF NOT EXISTS package_id VARCHAR(160);
ALTER TABLE runtime_events ADD COLUMN IF NOT EXISTS correlation_rule_id VARCHAR(220);
CREATE INDEX IF NOT EXISTS idx_runtime_events_package ON runtime_events(package_id);

-- 10. 初始化默认 allowlist
INSERT INTO ebpf_hook_allowlist_configs (config_json, description, activated_at)
VALUES ('{"tracepoints":["syscalls/sys_enter_socket","syscalls/sys_enter_bind","syscalls/sys_enter_splice","syscalls/sys_enter_execve","syscalls/sys_exit_execve","syscalls/sys_enter_setuid","syscalls/sys_enter_setgid","syscalls/sys_enter_capset","sched/sched_process_fork","sched/sched_process_exit"],"kprobes":[],"lsm":[],"xdp":[],"tc":[]}', 'Default allowlist', NOW())
ON CONFLICT DO NOTHING;
