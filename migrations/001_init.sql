-- ============================================================
-- Aegis智能主机安全系统 - 数据库初始化脚本
-- 版本: V3.0
-- 说明: 包含完整的漏洞管理模块表结构
-- ============================================================

-- 启用扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 自动更新 updated_at 时间戳的触发器函数
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ============================================================
-- 1. 主机资产表 (hosts)
-- ============================================================
CREATE TABLE IF NOT EXISTS hosts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ip_address VARCHAR(45) NOT NULL UNIQUE,
    hostname VARCHAR(255) NOT NULL,
    os_type VARCHAR(50) NOT NULL,
    agent_version VARCHAR(50) NOT NULL,
    last_heartbeat_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_hosts_ip ON hosts(ip_address);
CREATE INDEX IF NOT EXISTS idx_hosts_hostname ON hosts(hostname);
CREATE INDEX IF NOT EXISTS idx_hosts_last_heartbeat ON hosts(last_heartbeat_at);

CREATE TRIGGER update_hosts_updated_at
    BEFORE UPDATE ON hosts
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================
-- 2. LLM配置表 (llm_configs)
-- ============================================================
CREATE TABLE IF NOT EXISTS llm_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    api_key_encrypted TEXT NOT NULL,
    api_key_masked VARCHAR(50) NOT NULL,
    provider VARCHAR(50) NOT NULL DEFAULT 'dashscope',
    base_url VARCHAR(500) NOT NULL,
    model_name VARCHAR(100) NOT NULL DEFAULT 'qwen-plus',
    is_active BOOLEAN NOT NULL DEFAULT true,
    last_test_status VARCHAR(20),
    last_test_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_llm_configs_active ON llm_configs(is_active);

CREATE TRIGGER update_llm_configs_updated_at
    BEFORE UPDATE ON llm_configs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- 插入默认配置
INSERT INTO llm_configs (api_key_encrypted, api_key_masked, provider, base_url, model_name, is_active)
VALUES ('', '未配置', 'dashscope', 'https://dashscope.aliyuncs.com/compatible-mode/v1', 'qwen-plus', true)
ON CONFLICT DO NOTHING;

-- ============================================================
-- 3. 模板元数据表 (templates)
-- ============================================================
CREATE TABLE IF NOT EXISTS templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    file_type VARCHAR(20) NOT NULL,
    file_md5 VARCHAR(32),
    minio_object_name VARCHAR(255) NOT NULL,
    llm_prompt_template TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'parsing',
    error_message TEXT,
    rule_count INT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_templates_status ON templates(status);
CREATE INDEX IF NOT EXISTS idx_templates_created ON templates(created_at);
CREATE INDEX IF NOT EXISTS idx_templates_md5 ON templates(file_md5);

CREATE TRIGGER update_templates_updated_at
    BEFORE UPDATE ON templates
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================
-- 4. 基线规则表 (aegis_rules)
-- ============================================================
CREATE TABLE IF NOT EXISTS aegis_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    check_content TEXT NOT NULL,
    fix_content TEXT NOT NULL,
    generated_check_script TEXT,
    generated_fix_script TEXT,
    check_script_version INT DEFAULT 0,
    fix_script_version INT DEFAULT 0,
    check_script_status VARCHAR(20) DEFAULT 'pending',
    fix_script_status VARCHAR(20) DEFAULT 'pending',
    check_script_error TEXT,
    fix_script_error TEXT,
    script_status VARCHAR(20) DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rules_template ON aegis_rules(template_id);
CREATE INDEX IF NOT EXISTS idx_rules_status ON aegis_rules(script_status);

CREATE TRIGGER update_aegis_rules_updated_at
    BEFORE UPDATE ON aegis_rules
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================
-- 5. 脚本版本表 (script_versions) - V3.0更新
-- ============================================================
CREATE TABLE IF NOT EXISTS script_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID REFERENCES aegis_rules(id) ON DELETE CASCADE,
    vulnerability_id UUID,
    script_type VARCHAR(20) NOT NULL,
    version INT NOT NULL,
    script_content TEXT NOT NULL,
    generation_source VARCHAR(20) NOT NULL DEFAULT 'initial',
    llm_prompt_used TEXT,
    llm_response_raw TEXT,
    minio_object_name VARCHAR(255),
    os_type VARCHAR(50),
    is_current BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_script_type CHECK (script_type IN ('CHECK', 'FIX', 'VULNERABILITY_FIX', 'POC')),
    CONSTRAINT chk_generation_source CHECK (generation_source IN ('initial', 'self_healing', 'llm_generated', 'manual'))
);

CREATE INDEX IF NOT EXISTS idx_script_versions_rule ON script_versions(rule_id);
CREATE INDEX IF NOT EXISTS idx_script_versions_type ON script_versions(script_type);
CREATE INDEX IF NOT EXISTS idx_script_versions_current ON script_versions(is_current);
CREATE INDEX IF NOT EXISTS idx_script_versions_vulnerability ON script_versions(vulnerability_id);
CREATE INDEX IF NOT EXISTS idx_script_versions_os_type ON script_versions(os_type);

-- ============================================================
-- 6. 自愈日志表 (self_healing_logs) - V3.0更新
-- ============================================================
CREATE TABLE IF NOT EXISTS self_healing_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    original_task_id UUID,
    rule_id UUID REFERENCES aegis_rules(id) ON DELETE CASCADE,
    host_id UUID REFERENCES hosts(id) ON DELETE CASCADE,
    vulnerability_id UUID,
    script_type VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'healing',
    total_attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    trigger_error TEXT NOT NULL,
    trigger_exit_code INT NOT NULL DEFAULT 1,
    attempts_detail JSONB,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_healing_script_type CHECK (script_type IN ('CHECK', 'FIX', 'VULNERABILITY_FIX', 'POC')),
    CONSTRAINT chk_healing_status CHECK (status IN ('healing', 'healed', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_healing_logs_task ON self_healing_logs(original_task_id);
CREATE INDEX IF NOT EXISTS idx_healing_logs_rule ON self_healing_logs(rule_id);
CREATE INDEX IF NOT EXISTS idx_healing_logs_status ON self_healing_logs(status);
CREATE INDEX IF NOT EXISTS idx_healing_logs_vulnerability ON self_healing_logs(vulnerability_id);

-- ============================================================
-- 7. 任务日志表 (task_logs) - V3.0更新
-- ============================================================
CREATE TABLE IF NOT EXISTS task_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_group_id UUID NOT NULL,
    rule_id UUID REFERENCES aegis_rules(id) ON DELETE CASCADE,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    vulnerability_id UUID,
    task_type VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    script_content TEXT,
    script_version INT,
    stdout TEXT,
    stderr TEXT,
    exit_code INT,
    healing_id UUID REFERENCES self_healing_logs(id),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_task_type CHECK (task_type IN ('CHECK', 'FIX', 'VULNERABILITY_FIX', 'POC_VERIFY')),
    CONSTRAINT chk_task_status CHECK (status IN ('PENDING', 'RUNNING', 'SUCCESS', 'FAILED', 'TIMEOUT', 'HEALING'))
);

CREATE INDEX IF NOT EXISTS idx_task_logs_group ON task_logs(task_group_id);
CREATE INDEX IF NOT EXISTS idx_task_logs_rule ON task_logs(rule_id);
CREATE INDEX IF NOT EXISTS idx_task_logs_host ON task_logs(host_id);
CREATE INDEX IF NOT EXISTS idx_task_logs_status ON task_logs(status);
CREATE INDEX IF NOT EXISTS idx_task_logs_created ON task_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_task_logs_vulnerability ON task_logs(vulnerability_id);
CREATE INDEX IF NOT EXISTS idx_task_logs_healing ON task_logs(healing_id);

-- 添加 self_healing_logs 对 task_logs 的外键引用（延迟添加避免循环引用）
ALTER TABLE self_healing_logs
    ADD CONSTRAINT fk_healing_original_task
    FOREIGN KEY (original_task_id) REFERENCES task_logs(id) ON DELETE CASCADE;

-- ============================================================
-- 8. 漏洞主表 (vulnerabilities) - V3.0新增
-- ============================================================
CREATE TABLE IF NOT EXISTS vulnerabilities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cve_id VARCHAR(20) NOT NULL UNIQUE,
    severity VARCHAR(20) NOT NULL,
    cvss_score DECIMAL(3,1),
    description TEXT NOT NULL,
    affected_products JSONB,
    solution TEXT,
    ref_links JSONB,
    cwe_id VARCHAR(50),
    published_at TIMESTAMPTZ,
    last_modified_at TIMESTAMPTZ,
    source VARCHAR(50) NOT NULL DEFAULT 'llm_analysis',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_severity CHECK (severity IN ('Critical', 'High', 'Medium', 'Low')),
    CONSTRAINT chk_cvss_score CHECK (cvss_score >= 0.0 AND cvss_score <= 10.0),
    CONSTRAINT chk_vuln_source CHECK (source IN ('llm_analysis', 'nvd_import', 'manual'))
);

CREATE INDEX IF NOT EXISTS idx_vulnerabilities_cve_id ON vulnerabilities(cve_id);
CREATE INDEX IF NOT EXISTS idx_vulnerabilities_severity ON vulnerabilities(severity);
CREATE INDEX IF NOT EXISTS idx_vulnerabilities_cvss_score ON vulnerabilities(cvss_score);

CREATE TRIGGER update_vulnerabilities_updated_at
    BEFORE UPDATE ON vulnerabilities
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================
-- 9. 主机漏洞关联表 (host_vulnerabilities) - V3.0新增
-- ============================================================
CREATE TABLE IF NOT EXISTS host_vulnerabilities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    vulnerability_id UUID NOT NULL REFERENCES vulnerabilities(id) ON DELETE CASCADE,
    affected_package VARCHAR(255) NOT NULL,
    affected_version VARCHAR(100) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'detected',
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    verified_at TIMESTAMPTZ,
    fixed_at TIMESTAMPTZ,
    poc_result VARCHAR(20),
    fix_task_id UUID REFERENCES task_logs(id) ON DELETE SET NULL,
    poc_task_id UUID REFERENCES task_logs(id) ON DELETE SET NULL,
    scan_session_id UUID NOT NULL,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_hv_status CHECK (status IN ('detected', 'poc_verified', 'fixing', 'fixed', 'ignored', 'false_positive')),
    CONSTRAINT chk_poc_result CHECK (poc_result IS NULL OR poc_result IN ('vulnerable', 'not_vulnerable', 'error')),
    CONSTRAINT uq_host_vuln_package UNIQUE (host_id, vulnerability_id, affected_package, affected_version)
);

CREATE INDEX IF NOT EXISTS idx_host_vulnerabilities_host_id ON host_vulnerabilities(host_id);
CREATE INDEX IF NOT EXISTS idx_host_vulnerabilities_vulnerability_id ON host_vulnerabilities(vulnerability_id);
CREATE INDEX IF NOT EXISTS idx_host_vulnerabilities_status ON host_vulnerabilities(status);
CREATE INDEX IF NOT EXISTS idx_host_vulnerabilities_scan_session_id ON host_vulnerabilities(scan_session_id);

CREATE TRIGGER update_host_vulns_updated_at
    BEFORE UPDATE ON host_vulnerabilities
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================
-- 10. 主机软件清单缓存表 (installed_software) - V3.0新增
-- ============================================================
CREATE TABLE IF NOT EXISTS installed_software (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    package_name VARCHAR(255) NOT NULL,
    package_version VARCHAR(100) NOT NULL,
    package_manager VARCHAR(20) NOT NULL,
    architecture VARCHAR(20),
    scan_session_id UUID NOT NULL,
    collected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_package_manager CHECK (package_manager IN ('rpm', 'dpkg')),
    CONSTRAINT uq_installed_software UNIQUE (host_id, package_name, package_version, scan_session_id)
);

CREATE INDEX IF NOT EXISTS idx_installed_software_host_id ON installed_software(host_id);
CREATE INDEX IF NOT EXISTS idx_installed_software_package_name ON installed_software(package_name);
CREATE INDEX IF NOT EXISTS idx_installed_software_scan_session_id ON installed_software(scan_session_id);

-- ============================================================
-- 11. 漏洞修复脚本表 (vulnerability_fix_scripts) - V3.0新增
-- ============================================================
CREATE TABLE IF NOT EXISTS vulnerability_fix_scripts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vulnerability_id UUID NOT NULL REFERENCES vulnerabilities(id) ON DELETE CASCADE,
    os_type VARCHAR(50) NOT NULL,
    script_content TEXT NOT NULL,
    script_version INT NOT NULL DEFAULT 1,
    generation_source VARCHAR(20) NOT NULL DEFAULT 'llm_generated',
    llm_prompt_used TEXT,
    success_rate DECIMAL(5,2),
    execution_count INT NOT NULL DEFAULT 0,
    success_count INT NOT NULL DEFAULT 0,
    is_recommended BOOLEAN NOT NULL DEFAULT true,
    is_current BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_fix_generation_source CHECK (generation_source IN ('llm_generated', 'manual', 'self_healing')),
    CONSTRAINT uq_vuln_fix_script UNIQUE (vulnerability_id, os_type, script_version)
);

CREATE INDEX IF NOT EXISTS idx_vulnerability_fix_scripts_vuln_os ON vulnerability_fix_scripts(vulnerability_id, os_type);
CREATE INDEX IF NOT EXISTS idx_vulnerability_fix_scripts_is_current ON vulnerability_fix_scripts(is_current);

CREATE TRIGGER update_vulnerability_fix_scripts_updated_at
    BEFORE UPDATE ON vulnerability_fix_scripts
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================
-- 12. POC验证脚本表 (poc_scripts) - V3.0新增
-- ============================================================
CREATE TABLE IF NOT EXISTS poc_scripts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vulnerability_id UUID NOT NULL REFERENCES vulnerabilities(id) ON DELETE CASCADE,
    os_type VARCHAR(50) NOT NULL,
    script_content TEXT NOT NULL,
    script_version INT NOT NULL DEFAULT 1,
    generation_source VARCHAR(20) NOT NULL DEFAULT 'llm_generated',
    llm_prompt_used TEXT,
    safety_verified BOOLEAN NOT NULL DEFAULT false,
    safety_notes TEXT,
    is_current BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_poc_generation_source CHECK (generation_source IN ('llm_generated', 'manual')),
    CONSTRAINT uq_poc_script UNIQUE (vulnerability_id, os_type, script_version)
);

CREATE INDEX IF NOT EXISTS idx_poc_scripts_vuln_os ON poc_scripts(vulnerability_id, os_type);
CREATE INDEX IF NOT EXISTS idx_poc_scripts_is_current ON poc_scripts(is_current);

CREATE TRIGGER update_poc_scripts_updated_at
    BEFORE UPDATE ON poc_scripts
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================
-- 13. 迁移: 为脚本表添加生成状态字段 (V3.1)
-- ============================================================

ALTER TABLE poc_scripts
    ADD COLUMN IF NOT EXISTS generation_status VARCHAR(20) NOT NULL DEFAULT 'generated',
    ADD COLUMN IF NOT EXISTS generation_started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS generation_error TEXT,
    ADD COLUMN IF NOT EXISTS generation_error_detail TEXT,
    ADD COLUMN IF NOT EXISTS requested_host_id UUID;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_poc_generation_status'
          AND conrelid = 'poc_scripts'::regclass
    ) THEN
        ALTER TABLE poc_scripts
            ADD CONSTRAINT chk_poc_generation_status
            CHECK (generation_status IN ('generating', 'generated', 'failed'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_poc_scripts_generation_status
    ON poc_scripts(generation_status);

CREATE INDEX IF NOT EXISTS idx_poc_scripts_requested_host_id
    ON poc_scripts(requested_host_id);

ALTER TABLE vulnerability_fix_scripts
    ADD COLUMN IF NOT EXISTS generation_status VARCHAR(20) NOT NULL DEFAULT 'generated',
    ADD COLUMN IF NOT EXISTS generation_started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS generation_error TEXT,
    ADD COLUMN IF NOT EXISTS generation_error_detail TEXT,
    ADD COLUMN IF NOT EXISTS requested_host_ids JSONB;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_fix_script_generation_status'
          AND conrelid = 'vulnerability_fix_scripts'::regclass
    ) THEN
        ALTER TABLE vulnerability_fix_scripts
            ADD CONSTRAINT chk_fix_script_generation_status
            CHECK (generation_status IN ('generating', 'generated', 'failed'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_vulnerability_fix_scripts_generation_status
    ON vulnerability_fix_scripts(generation_status);

-- V3.1 自定义CVE功能数据库迁移脚本

-- 创建 custom_cve_queries 表
CREATE TABLE IF NOT EXISTS custom_cve_queries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cve_id VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'querying',
    result_vulnerability_id UUID REFERENCES vulnerabilities(id),
    error_message TEXT,
    error_detail TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_custom_cve_queries_status ON custom_cve_queries(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_cve_queries_single_querying ON custom_cve_queries ((1)) WHERE status = 'querying';
COMMENT ON TABLE custom_cve_queries IS '自定义CVE查询状态表';

-- 创建 host_vulnerability_scripts 表
CREATE TABLE IF NOT EXISTS host_vulnerability_scripts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cve_id VARCHAR(20) NOT NULL,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    script_type VARCHAR(20) NOT NULL,
    os_type VARCHAR(50) NOT NULL,
    script_content TEXT,
    script_version INT NOT NULL DEFAULT 1,
    generation_status VARCHAR(20) NOT NULL DEFAULT 'pending',
    generation_started_at TIMESTAMPTZ,
    generation_completed_at TIMESTAMPTZ,
    generation_error TEXT,
    generation_error_detail TEXT,
    execution_status VARCHAR(20),
    execution_task_id UUID REFERENCES task_logs(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_host_vuln_scripts_unique ON host_vulnerability_scripts (cve_id, host_id, script_type);
CREATE INDEX IF NOT EXISTS idx_host_vuln_scripts_cve_id ON host_vulnerability_scripts(cve_id);
CREATE INDEX IF NOT EXISTS idx_host_vuln_scripts_gen_status ON host_vulnerability_scripts(generation_status);
COMMENT ON TABLE host_vulnerability_scripts IS '主机漏洞脚本状态表';

-- 更新 vulnerabilities 表的 source 字段注释
COMMENT ON COLUMN vulnerabilities.source IS '数据来源: llm_analysis(扫描分析)/nvd_import(NVD导入)/custom_query(自定义查询)';
-- V5.0 智能异常检测表

-- 告警表
CREATE TABLE IF NOT EXISTS alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id VARCHAR(64) UNIQUE NOT NULL,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    pid INTEGER NOT NULL,
    mitre_id VARCHAR(20) NOT NULL,
    mitre_name VARCHAR(100),
    severity VARCHAR(20) NOT NULL DEFAULT 'medium',
    description TEXT,
    llm_summary TEXT,
    dedupe_key VARCHAR(256) NOT NULL,
    hit_count INTEGER NOT NULL DEFAULT 1,
    auto_blocked BOOLEAN NOT NULL DEFAULT FALSE,
    manual_blocked BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    first_seen_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_seen_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alerts_host_id ON alerts(host_id);
CREATE INDEX IF NOT EXISTS idx_alerts_mitre_id ON alerts(mitre_id);
CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status);
CREATE INDEX IF NOT EXISTS idx_alerts_dedupe_key ON alerts(dedupe_key);

-- 阻断记录表
CREATE TABLE IF NOT EXISTS block_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    block_id VARCHAR(64) UNIQUE NOT NULL,
    alert_id UUID REFERENCES alerts(id) ON DELETE SET NULL,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    action VARCHAR(50) NOT NULL DEFAULT 'kill_process',
    target VARCHAR(255),
    success BOOLEAN NOT NULL DEFAULT FALSE,
    message TEXT,
    issued_by VARCHAR(20) NOT NULL DEFAULT 'llm',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_block_records_host_id ON block_records(host_id);
CREATE INDEX IF NOT EXISTS idx_block_records_alert_id ON block_records(alert_id);

-- 阻断策略表
CREATE TABLE IF NOT EXISTS block_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mitre_id VARCHAR(20) UNIQUE NOT NULL,
    mitre_name VARCHAR(100),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    auto_block BOOLEAN NOT NULL DEFAULT FALSE,
    action VARCHAR(50) NOT NULL DEFAULT 'kill_process',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Sigma规则表
CREATE TABLE IF NOT EXISTS sigma_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id VARCHAR(128) UNIQUE NOT NULL,
    title VARCHAR(256),
    description TEXT,
    content TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    mitre_id VARCHAR(20),
    severity VARCHAR(20),
    generated_by VARCHAR(20) NOT NULL DEFAULT 'llm',
    version VARCHAR(20) NOT NULL DEFAULT '1.0',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    activated_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sigma_rules_status ON sigma_rules(status);
CREATE INDEX IF NOT EXISTS idx_sigma_rules_mitre_id ON sigma_rules(mitre_id);

-- Sigma规则版本历史表
CREATE TABLE IF NOT EXISTS sigma_rule_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id VARCHAR(128) NOT NULL REFERENCES sigma_rules(rule_id) ON DELETE CASCADE,
    version VARCHAR(20) NOT NULL,
    content TEXT NOT NULL,
    change_reason TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 工具调用记录表
CREATE TABLE IF NOT EXISTS tool_calls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id VARCHAR(64) UNIQUE NOT NULL,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    tool VARCHAR(50) NOT NULL,
    params_json TEXT,
    result_json TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    error TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tool_calls_host_id ON tool_calls(host_id);

-- 插入默认阻断策略（覆盖常见ATT&CK技术）
INSERT INTO block_policies (mitre_id, mitre_name, enabled, auto_block) VALUES
    ('T1059.004', 'Unix Shell', TRUE, FALSE),
    ('T1068', 'Exploitation for Privilege Escalation', TRUE, FALSE),
    ('T1222', 'File and Directory Permissions Modification', TRUE, FALSE),
    ('T1190', 'Exploit Public-Facing Application', TRUE, FALSE),
    ('T1003', 'OS Credential Dumping', TRUE, FALSE),
    ('T1070', 'Indicator Removal', TRUE, FALSE),
    ('T1053', 'Scheduled Task/Job', TRUE, FALSE),
    ('T1021', 'Remote Services', TRUE, FALSE),
    ('T1573', 'Encrypted Channel', TRUE, FALSE),
    ('T1486', 'Data Encrypted for Impact', TRUE, FALSE)
ON CONFLICT (mitre_id) DO NOTHING;
