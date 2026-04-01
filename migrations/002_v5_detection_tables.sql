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
