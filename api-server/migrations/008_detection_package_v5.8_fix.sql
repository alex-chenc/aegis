-- 008_detection_package_v5.8_fix.sql
-- V5.8 设计偏差修复：审核字段、角色权限表、变更原因

ALTER TABLE detection_packages ADD COLUMN IF NOT EXISTS reviewed_by VARCHAR(64);
ALTER TABLE detection_packages ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMP;

ALTER TABLE detection_package_builds ADD COLUMN IF NOT EXISTS reviewed_by VARCHAR(64);
ALTER TABLE detection_package_builds ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMP;
ALTER TABLE detection_package_builds ADD COLUMN IF NOT EXISTS review_comment TEXT;

ALTER TABLE ebpf_hook_allowlist_configs ADD COLUMN IF NOT EXISTS change_reason TEXT;

CREATE TABLE IF NOT EXISTS role_permissions (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    role VARCHAR(32) NOT NULL CHECK (role IN ('security_analyst', 'security_developer', 'admin')),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id)
);

CREATE TABLE IF NOT EXISTS allowlist_change_history (
    id SERIAL PRIMARY KEY,
    version BIGINT NOT NULL REFERENCES ebpf_hook_allowlist_configs(version),
    operator VARCHAR(64) NOT NULL,
    change_reason TEXT,
    diff_json TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO role_permissions (user_id, role)
SELECT DISTINCT operator, 'admin' FROM detection_package_operations
WHERE operator IS NOT NULL AND operator != ''
ON CONFLICT (user_id) DO NOTHING;
