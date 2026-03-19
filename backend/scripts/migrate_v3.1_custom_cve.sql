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
