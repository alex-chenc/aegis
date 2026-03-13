# 数据库设计文档 - V2.2 完整版

**版本**: 2.2
**状态**: 定稿
**作者**: Manus AI, Sisyphus

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 2.2 | 2026-03-12 | Sisyphus | **修复任务状态与自愈清理**。更新：task_logs表status字段说明，修复任务exit_code!=0时status为failed；新增：重新下发时清除self_healing_logs关联记录的说明。 |
| 2.1 | 2026-03-12 | Sisyphus | **任务重新下发支持**。task_logs表新增started_at字段用途说明（重新下发时记录下发时间）；新增Repository层UpdateForRedispatch方法说明。 |
| 2.0 | 2026-03-05 | Manus AI | **全面更新**。在 V1.6 基础上新增 `llm_configs`（LLM 配置表）、`script_versions`（脚本版本表）、`self_healing_logs`（自愈日志表）三张表，更新 `task_logs` 表增加自愈关联字段，补充完整的 Redis 缓存数据结构设计，提供更新后的完整 `init.sql` 脚本。 |
| 1.6 | 2026-03-05 | Manus AI | 完整重写，包含 4 张核心表的详细字段定义。 |

## 2. 概述

本文档为自动化基线检查与自愈系统提供 PostgreSQL 数据库和 Redis 缓存的完整数据模型设计。V2.2 版本在 V1.6 的 4 张核心表基础上，新增了 3 张表以支持 LLM 配置管理、脚本版本追踪和自愈流程记录等后端核心功能。同时，本文档首次纳入了 Redis 缓存的数据结构设计，使数据层设计更加完整。

## 3. 数据库表结构总览

V2.2 版本的数据库包含 7 张表，按业务领域可分为三组。

| 业务领域 | 表名 | 描述 | 版本 |
|:---|:---|:---|:---|
| 资产管理 | `hosts` | 存储 Agent 上报的主机核心身份信息 | V1.6 保留 |
| 模板与规则 | `templates` | 存储用户上传的基线模板文件元数据 | V1.6 保留 |
| 模板与规则 | `aegis_rules` | 存储 LLM 解析出的基线规则 | V1.6 保留 |
| 任务执行 | `task_logs` | 记录检查/修复任务的执行日志 | V1.6 更新 |
| 系统配置 | `llm_configs` | 存储 LLM 服务的配置信息 | **V2.2 新增** |
| 脚本管理 | `script_versions` | 记录 LLM 生成/修复脚本的版本历史 | **V2.2 新增** |
| 自愈管理 | `self_healing_logs` | 记录自愈修复流程的详细日志 | **V2.2 新增** |

## 4. 表结构详述

### 4.1 `hosts` (资产表) — V1.6 保留

本表的设计与 V1.6 完全一致，不做任何修改。

| 字段名 | 数据类型 | 约束 | 描述 |
|:---|:---|:---|:---|
| `id` | `UUID` | `PRIMARY KEY`, `DEFAULT gen_random_uuid()` | 记录的唯一标识符，由数据库自动生成。 |
| `ip_address` | `VARCHAR(45)` | `NOT NULL`, `UNIQUE` | Agent 上报的主机 IP 地址，作为资产的唯一标识之一。 |
| `hostname` | `VARCHAR(255)` | `NOT NULL` | Agent 上报的主机名。 |
| `os_type` | `VARCHAR(50)` | `NOT NULL` | Agent 上报的操作系统类型，例如 "linux"。 |
| `agent_version` | `VARCHAR(50)` | `NOT NULL` | Agent 的版本号，例如 "v2.2.0"。 |
| `last_heartbeat_at` | `TIMESTAMPTZ` | `NOT NULL` | 最后一次收到该 Agent 心跳的时间戳。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的创建时间戳。 |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的最后更新时间戳，由触发器自动维护。 |

### 4.2 `templates` (模板元数据表) — V1.6 保留

本表的设计与 V1.6 完全一致，不做任何修改。

| 字段名 | 数据类型 | 约束 | 描述 |
|:---|:---|:---|:---|
| `id` | `UUID` | `PRIMARY KEY`, `DEFAULT gen_random_uuid()` | 记录的唯一标识符。 |
| `name` | `VARCHAR(255)` | `NOT NULL` | 上传文件的原始名称。 |
| `file_type` | `VARCHAR(20)` | `NOT NULL` | 文件类型，例如 "PDF", "YAML"。 |
| `minio_object_name` | `VARCHAR(255)` | `NOT NULL` | 文件在 MinIO 对象存储中的唯一对象名称。 |
| `llm_prompt_template` | `TEXT` | `NULL` | 用于指导 LLM 解析此模板的特定提示，由系统自动生成。 |
| `status` | `VARCHAR(20)` | `NOT NULL`, `DEFAULT 'parsing'` | 模板状态：'pending'、'parsing'、'completed'、'failed'。 |
| `error_message` | `TEXT` | `NULL` | 解析失败时的错误信息。 |
| `rule_count` | `INT` | `DEFAULT 0` | 从该模板中解析出的规则数量。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的创建时间戳。 |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的最后更新时间戳。 |

### 4.3 `aegis_rules` (基线规则表) — V1.6 保留

本表的设计与 V1.6 完全一致，不做任何修改。

| 字段名 | 数据类型 | 约束 | 描述 |
|:---|:---|:---|:---|
| `id` | `UUID` | `PRIMARY KEY`, `DEFAULT gen_random_uuid()` | 记录的唯一标识符。 |
| `template_id` | `UUID` | `NOT NULL`, `FOREIGN KEY (templates.id)` | 关联的模板 ID，表示此规则的来源。 |
| `title` | `VARCHAR(255)` | `NOT NULL` | 基线规则的标题，由 LLM 解析得出。 |
| `check_content` | `TEXT` | `NOT NULL` | 基线规则的检查内容描述，由 LLM 解析得出。 |
| `fix_content` | `TEXT` | `NOT NULL` | 基线规则的修复方法描述，由 LLM 解析得出。 |
| `generated_check_script` | `TEXT` | `NULL` | 由 LLM 根据检查内容生成的最新版 Shell 检查脚本。 |
| `generated_fix_script` | `TEXT` | `NULL` | 由 LLM 根据修复内容生成的最新版 Shell 修复脚本。 |
| `check_script_version` | `INT` | `DEFAULT 0` | 检查脚本的当前版本号。 |
| `fix_script_version` | `INT` | `DEFAULT 0` | 修复脚本的当前版本号。 |
| `script_status` | `VARCHAR(20)` | `DEFAULT 'pending'` | 脚本生成状态：'pending'、'generating'、'ready'、'failed'。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的创建时间戳。 |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的最后更新时间戳。 |

### 4.4 `task_logs` (执行日志表) — V1.6 更新

在 V1.6 基础上新增了 `task_group_id` 和 `healing_id` 字段，分别用于关联批量任务和自愈流程。

| 字段名 | 数据类型 | 约束 | 描述 |
|:---|:---|:---|:---|
| `id` | `UUID` | `PRIMARY KEY`, `DEFAULT gen_random_uuid()` | 记录的唯一标识符。**重要**：重新下发时保持此ID不变，实现原地更新。 |
| `task_group_id` | `UUID` | `NOT NULL` | 批量任务组 ID，同一次下发的所有子任务共享此 ID。 |
| `rule_id` | `UUID` | `NOT NULL`, `FOREIGN KEY (aegis_rules.id)` | 关联的基线规则 ID。 |
| `host_id` | `UUID` | `NOT NULL`, `FOREIGN KEY (hosts.id)` | 任务执行的目标主机 ID。 |
| `task_type` | `VARCHAR(20)` | `NOT NULL` | 任务类型：'CHECK' 或 'FIX'。 |
| `status` | `VARCHAR(20)` | `NOT NULL` | 任务状态：'PENDING'、'RUNNING'、'SUCCESS'、'FAILED'、'TIMEOUT'、'HEALING'。**V2.2更新**：SUCCESS仅表示执行过程正常完成，不代表检查通过。**修复任务特殊处理**：exit_code!=0时status为FAILED以触发自愈。 |
| `script_content` | `TEXT` | `NULL` | 本次执行使用的脚本内容（快照，用于审计和自愈对比）。重新下发时更新为最新脚本。 |
| `script_version` | `INT` | `NULL` | 本次执行使用的脚本版本号。重新下发时更新为当前版本。 |
| `stdout` | `TEXT` | `NULL` | 任务执行的标准输出日志。重新下发时清空。 |
| `stderr` | `TEXT` | `NULL` | 任务执行的标准错误日志。重新下发时清空。 |
| `exit_code` | `INT` | `NULL` | 脚本执行的退出码。**V2.1说明**：0=通过，1=未通过，2=执行出错。重新下发时清空。 |
| `healing_id` | `UUID` | `NULL`, `FOREIGN KEY (self_healing_logs.id)` | 关联的自愈日志 ID，如果此任务是自愈重试产生的。 |
| `started_at` | `TIMESTAMPTZ` | `NULL` | 任务开始执行的时间戳。**V2.1说明**：首次下发时设置为创建时间；重新下发时更新为重新下发时间。 |
| `finished_at` | `TIMESTAMPTZ` | `NULL` | 任务执行完成的时间戳。重新下发时清空。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的创建时间戳。 |

**重新下发原地更新策略 — V2.2 更新**

当任务需要重新下发时（如脚本修复后重试），系统采用**原地更新策略**：

1. **保持 `id` 不变**：便于前端与日志链路追踪同一任务
2. **更新脚本**：`script_content` 和 `script_version` 更新为最新版本
3. **重置状态**：`status` 重置为 `pending`
4. **清空结果**：`stdout`、`stderr`、`exit_code`、`finished_at` 设为 `NULL`
5. **记录时间**：`started_at` 更新为重新下发时间
6. **V2.2 新增：清除自愈状态**：删除 `self_healing_logs` 表中关联的自愈记录，删除 Redis 中的 `self_healing:{task_id}` 缓存

**清除自愈状态的原因**：
- 避免前端显示过时的"脚本修复成功"等状态
- 确保根据新的执行结果判断任务状态
- 清除历史的自愈尝试记录，使重新下发后的自愈流程从零开始

**Repository 层方法**：

```go
// UpdateForRedispatch 重新下发任务时更新原有任务记录（原地更新，保留原始ID）
// 清空上次执行的输出，重置状态为pending，更新脚本内容和版本号
func (r *TaskLogRepository) UpdateForRedispatch(id uuid.UUID, scriptContent string, scriptVersion int) error {
    now := time.Now()
    result := r.db.Model(&model.TaskLog{}).
        Where("id = ?", id).
        Updates(map[string]interface{}{
            "script_content": scriptContent,
            "script_version": scriptVersion,
            "status":         "pending",
            "stdout":         nil,
            "stderr":         nil,
            "exit_code":      nil,
            "started_at":     now,
            "finished_at":    nil,
        })
    // ...
}
```

### 4.5 `llm_configs` (LLM 配置表) — V2.2 新增

存储用户配置的大语言模型服务连接信息。系统同一时间只有一套生效的 LLM 配置，但保留历史记录用于审计。

| 字段名 | 数据类型 | 约束 | 描述 |
|:---|:---|:---|:---|
| `id` | `UUID` | `PRIMARY KEY`, `DEFAULT gen_random_uuid()` | 记录的唯一标识符。 |
| `api_key_encrypted` | `TEXT` | `NOT NULL` | AES-256 加密后的 API Key 密文。 |
| `api_key_masked` | `VARCHAR(50)` | `NOT NULL` | 脱敏后的 API Key（如 `sk-xxxx...1234`），用于前端展示。 |
| `base_url` | `VARCHAR(500)` | `NOT NULL` | LLM 服务的 API 端点 URL。 |
| `model_name` | `VARCHAR(100)` | `NOT NULL`, `DEFAULT 'qwen-plus'` | 使用的模型名称。 |
| `is_active` | `BOOLEAN` | `NOT NULL`, `DEFAULT true` | 是否为当前生效的配置。同一时间只有一条记录为 true。 |
| `last_test_status` | `VARCHAR(20)` | `NULL` | 最后一次连通性测试的结果：'success' 或 'failed'。 |
| `last_test_at` | `TIMESTAMPTZ` | `NULL` | 最后一次连通性测试的时间。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的创建时间戳。 |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的最后更新时间戳。 |

### 4.6 `script_versions` (脚本版本表) — V2.2 新增

记录 LLM 生成的每个版本的脚本内容，支持版本追溯和回滚。每次 LLM 生成新脚本或自愈修复脚本时，都会在此表中创建一条新记录。

| 字段名 | 数据类型 | 约束 | 描述 |
|:---|:---|:---|:---|
| `id` | `UUID` | `PRIMARY KEY`, `DEFAULT gen_random_uuid()` | 记录的唯一标识符。 |
| `rule_id` | `UUID` | `NOT NULL`, `FOREIGN KEY (aegis_rules.id)` | 关联的基线规则 ID。 |
| `script_type` | `VARCHAR(10)` | `NOT NULL` | 脚本类型：'CHECK' 或 'FIX'。 |
| `version` | `INT` | `NOT NULL` | 版本号，从 1 开始递增。 |
| `script_content` | `TEXT` | `NOT NULL` | 完整的脚本内容。 |
| `generation_source` | `VARCHAR(20)` | `NOT NULL` | 生成来源：'initial'（首次生成）或 'self_healing'（自愈修复）。 |
| `llm_prompt_used` | `TEXT` | `NULL` | 生成此版本脚本时使用的完整 LLM Prompt（用于调试和审计）。 |
| `llm_response_raw` | `TEXT` | `NULL` | LLM 的原始返回内容（用于调试）。 |
| `minio_object_name` | `VARCHAR(255)` | `NULL` | 脚本文件在 MinIO 中的对象名称。 |
| `is_current` | `BOOLEAN` | `NOT NULL`, `DEFAULT true` | 是否为当前使用的版本。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的创建时间戳。 |

### 4.7 `self_healing_logs` (自愈日志表) — V2.2 新增

记录每一次自愈修复流程的完整生命周期，包括触发原因、每次重试的详细信息和最终结果。

| 字段名 | 数据类型 | 约束 | 描述 |
|:---|:---|:---|:---|
| `id` | `UUID` | `PRIMARY KEY`, `DEFAULT gen_random_uuid()` | 记录的唯一标识符。 |
| `original_task_id` | `UUID` | `NOT NULL`, `FOREIGN KEY (task_logs.id)` | 触发自愈的原始任务 ID。 |
| `rule_id` | `UUID` | `NOT NULL`, `FOREIGN KEY (aegis_rules.id)` | 关联的基线规则 ID。 |
| `host_id` | `UUID` | `NOT NULL`, `FOREIGN KEY (hosts.id)` | 目标主机 ID。 |
| `script_type` | `VARCHAR(10)` | `NOT NULL` | 脚本类型：'CHECK' 或 'FIX'。 |
| `trigger_error` | `TEXT` | `NOT NULL` | 触发自愈的原始错误信息（stderr 内容）。 |
| `trigger_exit_code` | `INT` | `NOT NULL` | 触发自愈的原始退出码。 |
| `total_attempts` | `INT` | `NOT NULL`, `DEFAULT 0` | 总尝试次数。 |
| `max_attempts` | `INT` | `NOT NULL`, `DEFAULT 3` | 最大允许尝试次数。 |
| `status` | `VARCHAR(20)` | `NOT NULL`, `DEFAULT 'healing'` | 自愈状态：'healing'、'healed'、'failed'。 |
| `final_script_version_id` | `UUID` | `NULL`, `FOREIGN KEY (script_versions.id)` | 最终成功的脚本版本 ID（如果自愈成功）。 |
| `attempts_detail` | `JSONB` | `NULL` | 每次重试的详细信息，JSON 数组格式。 |
| `started_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 自愈流程开始时间。 |
| `finished_at` | `TIMESTAMPTZ` | `NULL` | 自愈流程结束时间。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录的创建时间戳。 |

`attempts_detail` 字段的 JSON 结构示例：

```json
[
  {
    "attempt": 1,
    "script_version_id": "uuid-sv-1",
    "error_input": "bash: line 5: sshd_config: command not found",
    "llm_fix_summary": "将 sshd_config 替换为 /etc/ssh/sshd_config 的完整路径",
    "result_exit_code": 1,
    "result_stderr": "Permission denied",
    "timestamp": "2026-03-05T14:30:00Z"
  },
  {
    "attempt": 2,
    "script_version_id": "uuid-sv-2",
    "error_input": "Permission denied",
    "llm_fix_summary": "在关键命令前添加 sudo 提权",
    "result_exit_code": 0,
    "result_stderr": "",
    "timestamp": "2026-03-05T14:30:30Z"
  }
]
```

## 5. 索引策略

在 V1.6 的索引基础上，为新增的表和字段补充索引。

| 表名 | 字段名 | 索引类型 | 理由 |
|:---|:---|:---|:---|
| `hosts` | `ip_address` | `BTREE` (UNIQUE) | 主键级唯一标识，高频查询。 |
| `hosts` | `hostname` | `BTREE` | 支持按主机名进行搜索。 |
| `hosts` | `last_heartbeat_at` | `BTREE` | 用于后台任务快速筛选离线主机。 |
| `aegis_rules` | `template_id` | `BTREE` | 高频用于查询某个模板下的所有规则。 |
| `aegis_rules` | `script_status` | `BTREE` | 用于脚本生成 Worker 筛选待生成的规则。 |
| `task_logs` | `task_group_id` | `BTREE` | 高频用于查询某次批量任务的所有子任务。 |
| `task_logs` | `rule_id`, `host_id` | `BTREE` | 高频用于查询某个主机或某条规则的执行历史。 |
| `task_logs` | `created_at` | `BTREE` | 支持按时间范围查询日志。 |
| `llm_configs` | `is_active` | `BTREE` | 快速查找当前生效的配置。 |
| `script_versions` | `rule_id`, `script_type` | `BTREE` | 查询某条规则的脚本版本历史。 |
| `script_versions` | `is_current` | `BTREE` | 快速查找当前使用的脚本版本。 |
| `self_healing_logs` | `original_task_id` | `BTREE` | 关联查询原始任务的自愈记录。 |
| `self_healing_logs` | `status` | `BTREE` | 筛选进行中的自愈流程。 |

## 6. ER 关系图（文字描述）

各表之间的关系如下。

`templates` 1:N `aegis_rules`：一个模板可以解析出多条基线规则。

`aegis_rules` 1:N `task_logs`：一条规则可以被多次执行。

`aegis_rules` 1:N `script_versions`：一条规则可以有多个版本的脚本。

`hosts` 1:N `task_logs`：一台主机可以执行多个任务。

`task_logs` N:1 `self_healing_logs`：一个自愈流程可以产生多条任务日志（重试）。

`self_healing_logs` N:1 `script_versions`：自愈成功时关联最终使用的脚本版本。

## 7. 完整 SQL 初始化脚本 (`init.sql`)

此脚本可直接用于初始化一个全新的数据库，包含建表、建索引和创建触发器的所有操作。

```sql
-- ============================================================
-- 自动化基线检查与自愈系统 - 数据库初始化脚本
-- 版本: V2.2
-- ============================================================

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

-- ============================================================
-- 1. 资产表 (hosts)
-- ============================================================
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
CREATE TRIGGER update_hosts_updated_at BEFORE UPDATE ON hosts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================
-- 2. LLM 配置表 (llm_configs) [V2.2 新增]
-- ============================================================
CREATE TABLE llm_configs (
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
CREATE INDEX idx_llm_configs_is_active ON llm_configs(is_active);
CREATE TRIGGER update_llm_configs_updated_at BEFORE UPDATE ON llm_configs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- 插入默认配置（API Key 为空，需用户配置）
INSERT INTO llm_configs (api_key_encrypted, api_key_masked, base_url, model_name, is_active)
VALUES ('', '未配置', 'https://dashscope.aliyuncs.com/compatible-mode/v1', 'qwen-plus', true);

-- ============================================================
-- 3. 模板元数据表 (templates)
-- ============================================================
CREATE TABLE templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    file_type VARCHAR(20) NOT NULL,
    minio_object_name VARCHAR(255) NOT NULL,
    llm_prompt_template TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'parsing',
    error_message TEXT,
    rule_count INT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TRIGGER update_templates_updated_at BEFORE UPDATE ON templates
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================
-- 4. 基线规则表 (aegis_rules)
-- ============================================================
CREATE TABLE aegis_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    check_content TEXT NOT NULL,
    fix_content TEXT NOT NULL,
    generated_check_script TEXT,
    generated_fix_script TEXT,
    check_script_version INT DEFAULT 0,
    fix_script_version INT DEFAULT 0,
    script_status VARCHAR(20) DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_aegis_rules_template_id ON aegis_rules(template_id);
CREATE INDEX idx_aegis_rules_script_status ON aegis_rules(script_status);
CREATE TRIGGER update_aegis_rules_updated_at BEFORE UPDATE ON aegis_rules
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================
-- 5. 脚本版本表 (script_versions) [V2.2 新增]
-- ============================================================
CREATE TABLE script_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID NOT NULL REFERENCES aegis_rules(id) ON DELETE CASCADE,
    script_type VARCHAR(10) NOT NULL,
    version INT NOT NULL,
    script_content TEXT NOT NULL,
    generation_source VARCHAR(20) NOT NULL DEFAULT 'initial',
    llm_prompt_used TEXT,
    llm_response_raw TEXT,
    minio_object_name VARCHAR(255),
    is_current BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_script_type CHECK (script_type IN ('CHECK', 'FIX')),
    CONSTRAINT chk_generation_source CHECK (generation_source IN ('initial', 'self_healing'))
);
CREATE INDEX idx_script_versions_rule_id_type ON script_versions(rule_id, script_type);
CREATE INDEX idx_script_versions_is_current ON script_versions(is_current);

-- ============================================================
-- 6. 自愈日志表 (self_healing_logs) [V2.2 新增]
-- 注意：此表需在 task_logs 之前创建，因为 task_logs 引用此表
-- ============================================================
CREATE TABLE self_healing_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID NOT NULL REFERENCES aegis_rules(id) ON DELETE CASCADE,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    script_type VARCHAR(10) NOT NULL,
    trigger_error TEXT NOT NULL,
    trigger_exit_code INT NOT NULL,
    total_attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    status VARCHAR(20) NOT NULL DEFAULT 'healing',
    final_script_version_id UUID REFERENCES script_versions(id),
    attempts_detail JSONB,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_healing_script_type CHECK (script_type IN ('CHECK', 'FIX')),
    CONSTRAINT chk_healing_status CHECK (status IN ('healing', 'healed', 'failed'))
);
CREATE INDEX idx_self_healing_logs_status ON self_healing_logs(status);

-- ============================================================
-- 7. 执行日志表 (task_logs)
-- ============================================================
CREATE TABLE task_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_group_id UUID NOT NULL,
    rule_id UUID NOT NULL REFERENCES aegis_rules(id) ON DELETE CASCADE,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
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
    CONSTRAINT chk_task_type CHECK (task_type IN ('CHECK', 'FIX')),
    CONSTRAINT chk_task_status CHECK (status IN ('PENDING', 'RUNNING', 'SUCCESS', 'FAILED', 'TIMEOUT', 'HEALING'))
);
CREATE INDEX idx_task_logs_task_group_id ON task_logs(task_group_id);
CREATE INDEX idx_task_logs_rule_id_host_id ON task_logs(rule_id, host_id);
CREATE INDEX idx_task_logs_created_at ON task_logs(created_at);
CREATE INDEX idx_task_logs_healing_id ON task_logs(healing_id);

-- 为 self_healing_logs 添加对 task_logs 的外键引用
-- （由于循环引用，使用 ALTER TABLE 延迟添加）
ALTER TABLE self_healing_logs
    ADD COLUMN original_task_id UUID NOT NULL REFERENCES task_logs(id) DEFAULT gen_random_uuid();
CREATE INDEX idx_self_healing_logs_original_task_id ON self_healing_logs(original_task_id);
```

## 8. 数据生命周期管理

| 表名 | 保留策略 | 清理方式 |
|:---|:---|:---|
| `hosts` | 永久保留，除非主机被明确下线 | 手动删除 |
| `templates` | 永久保留，除非用户手动删除 | 级联删除关联的 `aegis_rules` |
| `aegis_rules` | 跟随模板生命周期 | 级联删除关联的 `script_versions` 和 `task_logs` |
| `task_logs` | 保留最近 90 天 | 定时任务清理：`DELETE FROM task_logs WHERE created_at < NOW() - INTERVAL '90 days'` |
| `llm_configs` | 永久保留（含历史记录） | 无需清理 |
| `script_versions` | 保留最近 180 天的非当前版本 | 定时任务清理：`DELETE FROM script_versions WHERE is_current = false AND created_at < NOW() - INTERVAL '180 days'` |
| `self_healing_logs` | 保留最近 90 天 | 定时任务清理：`DELETE FROM self_healing_logs WHERE created_at < NOW() - INTERVAL '90 days'` |

## 9. Redis 缓存数据结构设计

Redis 缓存的完整 Key 设计已在后端详细设计文档中定义。此处提供一份汇总表，便于数据库设计文档的完整性。

| Key 模式 | 数据类型 | TTL | 对应的数据库表 |
|:---|:---|:---|:---|
| `agent:heartbeat:{host_id}` | STRING | 90s | `hosts.last_heartbeat_at` |
| `agent:session:{host_id}` | STRING | 无 | 无（内存状态） |
| `template:parse:status:{template_id}` | HASH | 1h | `templates.status` |
| `task:status:{task_id}` | HASH | 2h | `task_logs.status` |
| `task:logs:{task_id}` | LIST | 2h | `task_logs.stdout/stderr` |
| `config:llm` | HASH | 无 | `llm_configs` |
| `self_healing:{task_id}` | HASH | 1h | `self_healing_logs` |

Redis 中的数据与数据库中的数据存在对应关系，但 Redis 侧重于存储实时、高频访问的临时状态，而数据库存储持久化的最终结果。两者之间的数据一致性通过"写入时双写、读取时优先 Redis"的策略保证。
