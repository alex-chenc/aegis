-- V5.7 命令审计与脚本审计日志
-- Migration: 010_v5.7_command_audit

BEGIN;

-- 1. 命令审计规则表
CREATE TABLE IF NOT EXISTS command_audit_rules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(200) NOT NULL,
    description TEXT,
    rule_type   VARCHAR(20)  NOT NULL DEFAULT 'hard_block',
    match_type  VARCHAR(20)  NOT NULL DEFAULT 'regex',
    pattern     TEXT         NOT NULL,
    category    VARCHAR(50)  NOT NULL DEFAULT 'system',
    severity    VARCHAR(20)  NOT NULL DEFAULT 'high',
    applies_to  JSONB        NOT NULL DEFAULT '["all"]',
    is_preset   BOOLEAN      NOT NULL DEFAULT false,
    is_enabled  BOOLEAN      NOT NULL DEFAULT true,
    created_at  TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_command_audit_rules_category ON command_audit_rules(category);
CREATE INDEX IF NOT EXISTS idx_command_audit_rules_enabled ON command_audit_rules(is_enabled);

-- 2. 脚本审计日志表
CREATE TABLE IF NOT EXISTS script_audit_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id         VARCHAR(100),
    rule_id         VARCHAR(100),
    script_type     VARCHAR(50),
    script_content  TEXT,
    audit_source    VARCHAR(20),
    attempt         INT,
    passed          BOOLEAN,
    risk_level      VARCHAR(20),
    blacklist_hits  JSONB,
    ai_analysis     JSONB,
    error_msg       TEXT,
    duration_ms     BIGINT,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_script_audit_log_task_id ON script_audit_log(task_id);
CREATE INDEX IF NOT EXISTS idx_script_audit_log_created_at ON script_audit_log(created_at);

-- 3. 系统配置表
CREATE TABLE IF NOT EXISTS system_configs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    config_key   VARCHAR(200) NOT NULL UNIQUE,
    config_value JSONB        NOT NULL,
    description  TEXT,
    category     VARCHAR(50)  NOT NULL,
    created_at   TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_system_configs_category ON system_configs(category);

-- 4. 初始化命令审计配置
INSERT INTO system_configs (config_key, config_value, description, category)
VALUES (
    'command_audit.settings',
    '{"blacklist_enabled":true,"ai_enabled":true,"max_retry":3,"dispatch_check":true,"agent_check":true}',
    '命令审计全局配置',
    'command_audit'
)
ON CONFLICT (config_key) DO NOTHING;

-- 5. 初始化预置规则（15条）
INSERT INTO command_audit_rules (name, description, rule_type, match_type, pattern, category, severity, applies_to, is_preset, is_enabled, created_at, updated_at) VALUES
-- 文件系统
('rm -rf /', '禁止删除根目录', 'hard_block', 'regex', 'rm\s+(-[a-zA-Z]*[rRfF]+\s+)*(/|/\*)', 'filesystem', 'critical', '["all"]', true, true, NOW(), NOW()),
('chmod 777', '禁止全局可写权限', 'hard_block', 'regex', 'chmod\s+777\s+', 'filesystem', 'high', '["all"]', true, true, NOW(), NOW()),
('敏感文件写入', '禁止直接写入/etc关键文件', 'hard_block', 'regex', '(echo|cat|tee|printf).*>\s*/etc/(passwd|shadow|sudoers|crontab)', 'filesystem', 'critical', '["all"]', true, true, NOW(), NOW()),
-- 权限
('危险chown', '禁止修改文件所有者为root', 'hard_block', 'regex', 'chown\s+(root|0)\s+', 'permission', 'high', '["all"]', true, true, NOW(), NOW()),
('SUID设置', '禁止设置SUID位', 'hard_block', 'regex', 'chmod\s+[0-7]*[4-7][0-7]*\s+', 'permission', 'high', '["all"]', true, true, NOW(), NOW()),
('sudo提权', '限制sudo使用', 'soft_warn', 'regex', 'sudo\s+(su|bash|sh|dash)', 'permission', 'medium', '["all"]', true, true, NOW(), NOW()),
-- 网络
('curl管道执行', '禁止curl下载并直接执行', 'hard_block', 'regex', 'curl.*\|.*(bash|sh|python|perl)', 'network', 'critical', '["all"]', true, true, NOW(), NOW()),
('wget管道执行', '禁止wget下载并直接执行', 'hard_block', 'regex', 'wget.*-O\s*-\s*\|.*(bash|sh|python)', 'network', 'critical', '["all"]', true, true, NOW(), NOW()),
('nc反弹shell', '禁止netcat反弹shell', 'hard_block', 'regex', 'nc\s+.*-e\s+(/bin/)?(bash|sh)', 'network', 'critical', '["all"]', true, true, NOW(), NOW()),
('iptables清空', '禁止清空防火墙规则', 'hard_block', 'regex', 'iptables\s+-F', 'network', 'high', '["all"]', true, true, NOW(), NOW()),
-- 系统
('禁用SELinux', '禁止关闭SELinux', 'hard_block', 'regex', 'setenforce\s+0', 'system', 'high', '["all"]', true, true, NOW(), NOW()),
('清空日志', '禁止清空系统日志', 'hard_block', 'regex', '(echo\s*>|truncate\s+-s\s+0)\s*/var/log/', 'system', 'high', '["all"]', true, true, NOW(), NOW()),
('kill系统进程', '禁止kill init/systemd进程', 'hard_block', 'regex', 'kill\s+-9\s+1\s', 'system', 'critical', '["all"]', true, true, NOW(), NOW()),
-- 特权
('useradd特权', '禁止添加特权用户', 'soft_warn', 'regex', 'useradd\s+.*-o\s+-u\s+0', 'privilege', 'high', '["all"]', true, true, NOW(), NOW()),
('密码修改', '限制密码修改操作', 'soft_warn', 'regex', '(passwd|chpasswd)\s+root', 'privilege', 'medium', '["all"]', true, true, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- 6. 更新 task_logs 状态约束，添加 AUDIT_BLOCKED
ALTER TABLE task_logs DROP CONSTRAINT IF EXISTS chk_task_status;
ALTER TABLE task_logs ADD CONSTRAINT chk_task_status CHECK (status IN ('PENDING', 'RUNNING', 'SUCCESS', 'FAILED', 'TIMEOUT', 'HEALING', 'AUDIT_BLOCKED'));

COMMIT;
