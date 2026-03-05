# 数据库设计文档 - V1.6 完整版

**版本**: 1.6
**状态**: 定稿
**作者**: Manus AI

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 1.6 | 2026-03-05 | Manus AI | **完整重写**。确保文档独立、完整，包含所有表的详细字段定义和完整的 `init.sql` 脚本，移除所有外部引用。 |
| 1.5 | 2026-03-05 | Manus AI | 精简 `hosts` 表的字段。 |

## 2. 概述

本文档为自动化基线检查与自愈系统提供 PostgreSQL 数据库的完整物理模型设计。该设计旨在支持系统的所有业务功能，包括资产管理、模板管理、规则存储和任务日志记录。

## 3. 表结构详述

数据库包含四张核心表：`hosts`, `templates`, `baseline_rules`, 和 `task_logs`。

### 3.1 `hosts` (资产表)

**描述**: 存储由 Agent 上报的主机核心身份信息。此表是所有资产的权威来源。

| 字段名 | 数据类型 | 约束 | 描述 |
|:---|:---|:---|:---|
| `id` | `UUID` | `PRIMARY KEY`, `DEFAULT gen_random_uuid()` | 记录的唯一标识符，由数据库自动生成。 |
| `ip_address` | `VARCHAR(45)` | `NOT NULL`, `UNIQUE` | Agent 上报的主机 IP 地址，作为资产的唯一标识之一。 |
| `hostname` | `VARCHAR(255)` | `NOT NULL` | Agent 上报的主机名。 |
| `os_type` | `VARCHAR(50)` | `NOT NULL` | Agent 上报的操作系统类型，例如 "linux"。 |
| `agent_version` | `VARCHAR(50)` | `NOT NULL` | Agent 的版本号，例如 "v1.6.0"。 |
| `last_heartbeat_at` | `TIMESTAMPTZ` | `NOT NULL` | 最后一次收到该 Agent 心跳的时间戳。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的创建时间戳。 |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的最后更新时间戳，由触发器自动维护。 |

### 3.2 `templates` (模板元数据表)

**描述**: 存储用户上传的基线模板文件的元数据。

| 字段名 | 数据类型 | 约束 | 描述 |
|:---|:---|:---|:---|
| `id` | `UUID` | `PRIMARY KEY`, `DEFAULT gen_random_uuid()` | 记录的唯一标识符。 |
| `name` | `VARCHAR(255)` | `NOT NULL` | 上传文件的原始名称。 |
| `file_type` | `VARCHAR(20)` | `NOT NULL` | 文件类型，例如 "PDF", "YAML"。 |
| `minio_object_name` | `VARCHAR(255)` | `NOT NULL` | 文件在 MinIO 对象存储中的唯一对象名称。 |
| `llm_prompt_template` | `TEXT` | `NULL` | (预留字段) 用于指导 LLM 解析此模板的特定提示。 |
| `status` | `VARCHAR(20)` | `NOT NULL`, `DEFAULT 'parsing'` | 模板状态，如 'parsing', 'completed', 'failed'。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的创建时间戳。 |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的最后更新时间戳。 |

### 3.3 `baseline_rules` (基线规则表)

**描述**: 存储从模板文件中由 LLM 解析出的具体基线规则。

| 字段名 | 数据类型 | 约束 | 描述 |
|:---|:---|:---|:---|
| `id` | `UUID` | `PRIMARY KEY`, `DEFAULT gen_random_uuid()` | 记录的唯一标识符。 |
| `template_id` | `UUID` | `NOT NULL`, `FOREIGN KEY (templates.id)` | 关联的模板 ID，表示此规则的来源。 |
| `title` | `VARCHAR(255)` | `NOT NULL` | 基线规则的标题，由 LLM 解析得出。 |
| `check_content` | `TEXT` | `NOT NULL` | 基线规则的检查内容描述，由 LLM 解析得出。 |
| `fix_content` | `TEXT` | `NOT NULL` | 基线规则的修复方法描述，由 LLM 解析得出。 |
| `generated_check_script` | `TEXT` | `NULL` | 由 LLM 根据检查内容生成的 Shell 检查脚本。 |
| `generated_fix_script` | `TEXT` | `NULL` | 由 LLM 根据修复内容生成的 Shell 修复脚本。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的创建时间戳。 |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的最后更新时间戳。 |

### 3.4 `task_logs` (执行日志表)

**描述**: 记录每一次检查或修复任务的详细执行日志。

| 字段名 | 数据类型 | 约束 | 描述 |
|:---|:---|:---|:---|
| `id` | `UUID` | `PRIMARY KEY`, `DEFAULT gen_random_uuid()` | 记录的唯一标识符。 |
| `rule_id` | `UUID` | `NOT NULL`, `FOREIGN KEY (baseline_rules.id)` | 关联的基线规则 ID。 |
| `host_id` | `UUID` | `NOT NULL`, `FOREIGN KEY (hosts.id)` | 任务执行的目标主机 ID。 |
| `task_type` | `VARCHAR(20)` | `NOT NULL` | 任务类型，'CHECK' 或 'FIX'。 |
| `status` | `VARCHAR(20)` | `NOT NULL` | 任务最终状态，'SUCCESS', 'FAILED', 'TIMEOUT'。 |
| `stdout` | `TEXT` | `NULL` | 任务执行的标准输出日志。 |
| `stderr` | `TEXT` | `NULL` | 任务执行的标准错误日志。 |
| `exit_code` | `INT` | `NULL` | 脚本执行的退出码。 |
| `started_at` | `TIMESTAMPTZ` | `NOT NULL` | 任务开始执行的时间戳。 |
| `finished_at` | `TIMESTAMPTZ` | `NOT NULL` | 任务执行完成的时间戳。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的创建时间戳。 |

## 4. 索引策略

为提高查询性能，对高频查询字段建立索引。

| 表名 | 字段名 | 索引类型 | 理由 |
|:---|:---|:---|:---|
| `hosts` | `ip_address` | `BTREE` (UNIQUE) | 主键级唯一标识，高频查询。 |
| `hosts` | `hostname` | `BTREE` | 支持按主机名进行搜索。 |
| `hosts` | `last_heartbeat_at` | `BTREE` | 用于后台任务快速筛选离线主机。 |
| `baseline_rules` | `template_id` | `BTREE` | `FOREIGN KEY`，高频用于查询某个模板下的所有规则。 |
| `task_logs` | `rule_id`, `host_id` | `BTREE` | `FOREIGN KEY`，高频用于查询某个主机或某条规则的执行历史。 |
| `task_logs` | `created_at` | `BTREE` | 支持按时间范围查询日志，便于日志清理和审计。 |

## 5. 完整 SQL 初始化脚本 (`init.sql`)

此脚本可直接用于初始化一个全新的数据库，包含建表、建索引和创建触发器的所有操作。

```sql
-- 启用 pgcrypto 扩展以生成 UUID
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 自动更新 updated_at 时间戳的触发器函数
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- 1. 资产表 (hosts)
CREATE TABLE hosts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ip_address VARCHAR(45) NOT NULL UNIQUE,
    hostname VARCHAR(255) NOT NULL,
    os_type VARCHAR(50) NOT NULL,
    agent_version VARCHAR(50) NOT NULL,
    last_heartbeat_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_hosts_hostname ON hosts(hostname);
CREATE INDEX idx_hosts_last_heartbeat_at ON hosts(last_heartbeat_at);
CREATE TRIGGER update_hosts_updated_at BEFORE UPDATE ON hosts FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- 2. 模板元数据表 (templates)
CREATE TABLE templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    file_type VARCHAR(20) NOT NULL,
    minio_object_name VARCHAR(255) NOT NULL,
    llm_prompt_template TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'parsing',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TRIGGER update_templates_updated_at BEFORE UPDATE ON templates FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- 3. 基线规则表 (baseline_rules)
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
CREATE INDEX idx_baseline_rules_template_id ON baseline_rules(template_id);
CREATE TRIGGER update_baseline_rules_updated_at BEFORE UPDATE ON baseline_rules FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- 4. 执行日志表 (task_logs)
CREATE TABLE task_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID NOT NULL REFERENCES baseline_rules(id) ON DELETE CASCADE,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    task_type VARCHAR(20) NOT NULL, -- 'CHECK' or 'FIX'
    status VARCHAR(20) NOT NULL, -- 'SUCCESS', 'FAILED', 'TIMEOUT'
    stdout TEXT,
    stderr TEXT,
    exit_code INT,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_task_logs_rule_id_host_id ON task_logs(rule_id, host_id);
CREATE INDEX idx_task_logs_created_at ON task_logs(created_at);

```

## 6. 数据生命周期管理

*   **`hosts` 表**: 数据应永久保留，除非主机被明确下线并从系统中移除。
*   **`templates` 和 `baseline_rules` 表**: 数据应永久保留，除非用户手动删除模板。
*   **`task_logs` 表**: 此表数据量会随时间线性增长。建议设置一个定期的清理策略，例如通过一个 `cronjob` 每天执行 `DELETE FROM task_logs WHERE created_at < NOW() - INTERVAL '90 days';`，仅保留最近 90 天的日志。
