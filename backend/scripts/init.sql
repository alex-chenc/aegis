-- 自动化基线检查与自愈系统 - 数据库初始化脚本
-- 版本：V2.0

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

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

CREATE INDEX idx_hosts_ip ON hosts(ip_address);
CREATE INDEX idx_hosts_hostname ON hosts(hostname);
CREATE INDEX idx_hosts_last_heartbeat ON hosts(last_heartbeat_at);

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

CREATE TABLE IF NOT EXISTS baseline_rules (
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

CREATE INDEX idx_rules_template ON baseline_rules(template_id);
CREATE INDEX idx_rules_status ON baseline_rules(script_status);

CREATE TABLE IF NOT EXISTS task_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_group_id UUID NOT NULL,
    rule_id UUID NOT NULL REFERENCES baseline_rules(id),
    host_id UUID NOT NULL REFERENCES hosts(id),
    task_type VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL,
    script_content TEXT,
    script_version INT,
    stdout TEXT,
    stderr TEXT,
    exit_code INT,
    healing_id UUID,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_logs_group ON task_logs(task_group_id);
CREATE INDEX idx_task_logs_rule ON task_logs(rule_id);
CREATE INDEX idx_task_logs_host ON task_logs(host_id);
CREATE INDEX idx_task_logs_status ON task_logs(status);
CREATE INDEX idx_task_logs_created ON task_logs(created_at);

CREATE TABLE IF NOT EXISTS llm_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    api_key_encrypted TEXT NOT NULL,
    api_key_masked VARCHAR(50) NOT NULL,
    base_url VARCHAR(500) NOT NULL,
    model_name VARCHAR(100) NOT NULL DEFAULT 'qwen-plus',
    is_active BOOLEAN NOT NULL DEFAULT true,
    last_test_status VARCHAR(20),
    last_test_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_llm_configs_active ON llm_configs(is_active);

CREATE TABLE IF NOT EXISTS script_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID NOT NULL REFERENCES baseline_rules(id),
    script_type VARCHAR(10) NOT NULL,
    version INT NOT NULL,
    script_content TEXT NOT NULL,
    generation_source VARCHAR(20) NOT NULL,
    llm_prompt_used TEXT,
    llm_response_raw TEXT,
    minio_object_name VARCHAR(255),
    is_current BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_script_versions_rule ON script_versions(rule_id);
CREATE INDEX idx_script_versions_type ON script_versions(script_type);
CREATE INDEX idx_script_versions_current ON script_versions(is_current);

CREATE TABLE IF NOT EXISTS self_healing_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    original_task_id UUID NOT NULL REFERENCES task_logs(id),
    rule_id UUID NOT NULL REFERENCES baseline_rules(id),
    host_id UUID NOT NULL REFERENCES hosts(id),
    script_type VARCHAR(10) NOT NULL,
    status VARCHAR(20) NOT NULL,
    total_attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    trigger_error TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE INDEX idx_healing_logs_task ON self_healing_logs(original_task_id);
CREATE INDEX idx_healing_logs_rule ON self_healing_logs(rule_id);
CREATE INDEX idx_healing_logs_status ON self_healing_logs(status);

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_hosts_updated_at
    BEFORE UPDATE ON hosts
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_templates_updated_at
    BEFORE UPDATE ON templates
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_baseline_rules_updated_at
    BEFORE UPDATE ON baseline_rules
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_llm_configs_updated_at
    BEFORE UPDATE ON llm_configs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();