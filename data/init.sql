-- 启用 pgcrypto 扩展以生成 UUID
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 资产表
CREATE TABLE hosts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ip_address VARCHAR(45) NOT NULL UNIQUE,
    hostname VARCHAR(255) NOT NULL,
    os_type VARCHAR(50) NOT NULL,
    os_version VARCHAR(100) NOT NULL,
    kernel_version VARCHAR(100),
    agent_version VARCHAR(50) NOT NULL,
    architecture VARCHAR(20) NOT NULL,
    cpu_info JSONB,
    total_memory_mb BIGINT,
    total_disk_gb BIGINT,
    network_interfaces JSONB,
    cpu_load_1min REAL,
    mem_usage_percent REAL,
    last_heartbeat_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 为 hosts 表创建索引
CREATE INDEX idx_hosts_hostname ON hosts(hostname);
CREATE INDEX idx_hosts_last_heartbeat_at ON hosts(last_heartbeat_at);

-- 模板元数据表
CREATE TABLE templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    file_type VARCHAR(20) NOT NULL,
    minio_object_name VARCHAR(255) NOT NULL,
    llm_prompt_template TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 基线规则表
CREATE TABLE baseline_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    check_content TEXT NOT NULL,
    fix_content TEXT NOT NULL,
    generated_check_script TEXT,
    generated_fix_script TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 为 baseline_rules 表创建索引
CREATE INDEX idx_baseline_rules_template_id ON baseline_rules(template_id);

-- 执行日志表
CREATE TABLE task_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID NOT NULL REFERENCES baseline_rules(id) ON DELETE CASCADE,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    task_type VARCHAR(20) NOT NULL, -- CHECK or FIX
    status VARCHAR(20) NOT NULL, -- SUCCESS, FAILED, TIMEOUT
    stdout TEXT,
    stderr TEXT,
    exit_code INT,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 为 task_logs 表创建索引
CREATE INDEX idx_task_logs_rule_id ON task_logs(rule_id);
CREATE INDEX idx_task_logs_host_id ON task_logs(host_id);
CREATE INDEX idx_task_logs_created_at ON task_logs(created_at);

-- 自动更新 updated_at 时间戳的触发器函数
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- 为每个表创建触发器
CREATE TRIGGER update_hosts_updated_at BEFORE UPDATE ON hosts FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_templates_updated_at BEFORE UPDATE ON templates FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_baseline_rules_updated_at BEFORE UPDATE ON baseline_rules FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
