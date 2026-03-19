# 数据库设计文档 - V3.1 自定义CVE功能

**版本**: 3.1
**状态**: 定稿
**作者**: 安全产品团队
**日期**: 2026-03-19

---

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 3.1 | 2026-03-19 | 安全产品团队 | **新增自定义CVE功能**。在V3.0基础上新增2张表：`custom_cve_queries`（自定义CVE查询状态表）、`host_vulnerability_scripts`（主机漏洞脚本状态表），支持自定义CVE查询入库和多主机脚本状态追踪。 |
| 3.0 | 2026-03-13 | 安全产品团队 | 新增漏洞管理模块，新增5张表。 |
| 2.2 | 2026-03-12 | Sisyphus | 任务管理与超时机制增强。 |

---

## 2. 概述

本文档为Aegis智能主机安全系统V3.1版本提供新增数据库表的设计。V3.1版本在V3.0的12张表基础上，新增2张表以支持**自定义CVE查询与管理**功能。

### 2.1 新增表概览

| 表名 | 描述 | 核心功能 |
|:---|:---|:---|
| `custom_cve_queries` | 自定义CVE查询状态表 | 追踪用户发起的CVE查询任务，实现查询互斥 |
| `host_vulnerability_scripts` | 主机漏洞脚本状态表 | 追踪每个CVE针对每个主机的脚本生成和执行状态 |

---

## 3. 新增表结构详述

### 3.1 `custom_cve_queries` (自定义CVE查询状态表)

存储用户发起的自定义CVE查询任务，实现查询状态追踪和互斥控制。

#### 3.1.1 表结构定义

| 字段名 | 数据类型 | 约束 | 描述 |
|:---|:---|:---|:---|
| `id` | `UUID` | `PRIMARY KEY`, `DEFAULT gen_random_uuid()` | 查询任务的唯一标识符。 |
| `cve_id` | `VARCHAR(20)` | `NOT NULL` | 用户输入的CVE编号，如 'CVE-2021-44228'。 |
| `status` | `VARCHAR(20)` | `NOT NULL`, `DEFAULT 'querying'` | 查询状态：'querying'（查询中）、'success'（成功）、'failed'（失败）。 |
| `result_vulnerability_id` | `UUID` | `NULL`, `REFERENCES vulnerabilities(id)` | 查询成功后关联的漏洞记录ID。 |
| `error_message` | `TEXT` | `NULL` | 查询失败时的错误消息。 |
| `error_detail` | `TEXT` | `NULL` | 查询失败时的详细错误信息。 |
| `started_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 查询开始时间。 |
| `completed_at` | `TIMESTAMPTZ` | `NULL` | 查询完成时间（成功或失败）。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录创建时间。 |

#### 3.1.2 索引设计

```sql
-- 按状态查询的索引
CREATE INDEX idx_custom_cve_queries_status ON custom_cve_queries(status);

-- 按CVE编号查询的索引
CREATE INDEX idx_custom_cve_queries_cve_id ON custom_cve_queries(cve_id);

-- 按开始时间查询的索引（用于清理过期记录）
CREATE INDEX idx_custom_cve_queries_started_at ON custom_cve_queries(started_at);
```

#### 3.1.3 查询互斥约束

**核心约束**：同一时间只允许一条`status='querying'`的记录。

```sql
-- 使用部分唯一索引实现互斥
CREATE UNIQUE INDEX idx_custom_cve_queries_single_querying 
    ON custom_cve_queries ((1)) WHERE status = 'querying';
```

**原理说明**：
- 该索引在`status='querying'`时生效
- 索引表达式为常量`1`，因此表中只能存在一条满足条件的记录
- 当尝试插入第二条`status='querying'`记录时，会违反唯一约束

#### 3.1.4 数据示例

**查询中状态**：
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "cve_id": "CVE-2021-44228",
  "status": "querying",
  "result_vulnerability_id": null,
  "error_message": null,
  "error_detail": null,
  "started_at": "2026-03-19T10:30:00Z",
  "completed_at": null,
  "created_at": "2026-03-19T10:30:00Z"
}
```

**查询成功**：
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "cve_id": "CVE-2021-44228",
  "status": "success",
  "result_vulnerability_id": "660e8400-e29b-41d4-a716-446655440001",
  "error_message": null,
  "error_detail": null,
  "started_at": "2026-03-19T10:30:00Z",
  "completed_at": "2026-03-19T10:30:25Z",
  "created_at": "2026-03-19T10:30:00Z"
}
```

**查询失败**：
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440002",
  "cve_id": "CVE-2099-99999",
  "status": "failed",
  "result_vulnerability_id": null,
  "error_message": "未查询到该CVE信息",
  "error_detail": "LLM返回: 该CVE编号不存在于公开数据库中",
  "started_at": "2026-03-19T10:35:00Z",
  "completed_at": "2026-03-19T10:35:15Z",
  "created_at": "2026-03-19T10:35:00Z"
}
```

---

### 3.2 `host_vulnerability_scripts` (主机漏洞脚本状态表)

存储每个CVE针对每个主机的脚本生成和执行状态，支持多主机独立状态追踪。

#### 3.2.1 表结构定义

| 字段名 | 数据类型 | 约束 | 描述 |
|:---|:---|:---|:---|
| `id` | `UUID` | `PRIMARY KEY`, `DEFAULT gen_random_uuid()` | 脚本记录的唯一标识符。 |
| `cve_id` | `VARCHAR(20)` | `NOT NULL` | CVE编号。 |
| `host_id` | `UUID` | `NOT NULL`, `REFERENCES hosts(id)` | 主机ID。 |
| `script_type` | `VARCHAR(20)` | `NOT NULL` | 脚本类型：'poc'（POC验证脚本）、'fix'（修复脚本）。 |
| `os_type` | `VARCHAR(50)` | `NOT NULL` | 目标主机的操作系统类型，用于脚本适配。 |
| `script_content` | `TEXT` | `NULL` | 生成的脚本内容。 |
| `script_version` | `INT` | `NOT NULL`, `DEFAULT 1` | 脚本版本号，支持版本管理。 |
| `generation_status` | `VARCHAR(20)` | `NOT NULL`, `DEFAULT 'pending'` | 生成状态：'pending'（待生成）、'generating'（生成中）、'generated'（已生成）、'failed'（失败）。 |
| `generation_started_at` | `TIMESTAMPTZ` | `NULL` | 脚本生成开始时间。 |
| `generation_completed_at` | `TIMESTAMPTZ` | `NULL` | 脚本生成完成时间。 |
| `generation_error` | `TEXT` | `NULL` | 生成失败的错误消息。 |
| `generation_error_detail` | `TEXT` | `NULL` | 生成失败的详细错误信息。 |
| `execution_status` | `VARCHAR(20)` | `NULL` | 执行状态：'pending'（待执行）、'running'（执行中）、'success'（成功）、'failed'（失败）、'timeout'（超时）。 |
| `execution_task_id` | `UUID` | `NULL`, `REFERENCES task_logs(id)` | 关联的任务记录ID。 |
| `execution_started_at` | `TIMESTAMPTZ` | `NULL` | 脚本执行开始时间。 |
| `execution_completed_at` | `TIMESTAMPTZ` | `NULL` | 脚本执行完成时间。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录创建时间。 |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL`, `DEFAULT NOW()` | 记录最后更新时间。 |

#### 3.2.2 索引设计

```sql
-- 唯一约束：同一CVE+主机+脚本类型只能有一条记录
CREATE UNIQUE INDEX idx_host_vuln_scripts_unique 
    ON host_vulnerability_scripts (cve_id, host_id, script_type);

-- 按CVE查询的索引
CREATE INDEX idx_host_vuln_scripts_cve ON host_vulnerability_scripts(cve_id);

-- 按主机查询的索引
CREATE INDEX idx_host_vuln_scripts_host ON host_vulnerability_scripts(host_id);

-- 按生成状态查询的索引
CREATE INDEX idx_host_vuln_scripts_gen_status ON host_vulnerability_scripts(generation_status);

-- 按执行状态查询的索引
CREATE INDEX idx_host_vuln_scripts_exec_status ON host_vulnerability_scripts(execution_status);
```

#### 3.2.3 数据示例

**待生成状态**：
```json
{
  "id": "770e8400-e29b-41d4-a716-446655440003",
  "cve_id": "CVE-2021-44228",
  "host_id": "880e8400-e29b-41d4-a716-446655440004",
  "script_type": "poc",
  "os_type": "CentOS 7.9",
  "script_content": null,
  "script_version": 1,
  "generation_status": "pending",
  "generation_started_at": null,
  "generation_completed_at": null,
  "generation_error": null,
  "generation_error_detail": null,
  "execution_status": null,
  "execution_task_id": null,
  "execution_started_at": null,
  "execution_completed_at": null,
  "created_at": "2026-03-19T10:40:00Z",
  "updated_at": "2026-03-19T10:40:00Z"
}
```

**生成中状态**：
```json
{
  "id": "770e8400-e29b-41d4-a716-446655440003",
  "cve_id": "CVE-2021-44228",
  "host_id": "880e8400-e29b-41d4-a716-446655440004",
  "script_type": "poc",
  "os_type": "CentOS 7.9",
  "script_content": null,
  "script_version": 1,
  "generation_status": "generating",
  "generation_started_at": "2026-03-19T10:40:05Z",
  "generation_completed_at": null,
  "generation_error": null,
  "generation_error_detail": null,
  "execution_status": null,
  "execution_task_id": null,
  "execution_started_at": null,
  "execution_completed_at": null,
  "created_at": "2026-03-19T10:40:00Z",
  "updated_at": "2026-03-19T10:40:05Z"
}
```

**已生成状态**：
```json
{
  "id": "770e8400-e29b-41d4-a716-446655440003",
  "cve_id": "CVE-2021-44228",
  "host_id": "880e8400-e29b-41d4-a716-446655440004",
  "script_type": "poc",
  "os_type": "CentOS 7.9",
  "script_content": "#!/bin/bash\n# CVE-2021-44228 POC验证脚本\necho '检查Log4j漏洞...'\n...",
  "script_version": 1,
  "generation_status": "generated",
  "generation_started_at": "2026-03-19T10:40:05Z",
  "generation_completed_at": "2026-03-19T10:40:20Z",
  "generation_error": null,
  "generation_error_detail": null,
  "execution_status": null,
  "execution_task_id": null,
  "execution_started_at": null,
  "execution_completed_at": null,
  "created_at": "2026-03-19T10:40:00Z",
  "updated_at": "2026-03-19T10:40:20Z"
}
```

**生成失败状态**：
```json
{
  "id": "770e8400-e29b-41d4-a716-446655440005",
  "cve_id": "CVE-2021-44228",
  "host_id": "880e8400-e29b-41d4-a716-446655440006",
  "script_type": "poc",
  "os_type": "Ubuntu 20.04",
  "script_content": null,
  "script_version": 1,
  "generation_status": "failed",
  "generation_started_at": "2026-03-19T10:41:00Z",
  "generation_completed_at": "2026-03-19T10:41:30Z",
  "generation_error": "脚本生成超时",
  "generation_error_detail": "LLM响应时间超过30秒限制",
  "execution_status": null,
  "execution_task_id": null,
  "execution_started_at": null,
  "execution_completed_at": null,
  "created_at": "2026-03-19T10:41:00Z",
  "updated_at": "2026-03-19T10:41:30Z"
}
```

---

## 4. 现有表扩展

### 4.1 `vulnerabilities` 表扩展

在现有`vulnerabilities`表中，`source`字段新增枚举值：

| source值 | 描述 |
|:---|:---|
| `llm_analysis` | 扫描时LLM分析得出（原有） |
| `nvd_import` | NVD数据库导入（预留） |
| **`custom_query`** | **用户自定义查询（新增）** |

**区分方式**：
- `source='custom_query'` 的记录为用户自定义添加的CVE
- 这些记录的`affected_hosts_count`初始为0
- 用户可对这些CVE进行POC验证或修复

---

## 5. SQL迁移脚本

### 5.1 新建表迁移

```sql
-- ============================================
-- V3.1 自定义CVE功能数据库迁移脚本
-- 版本: 3.1.0
-- 日期: 2026-03-19
-- ============================================

-- 1. 创建 custom_cve_queries 表
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

-- 索引
CREATE INDEX idx_custom_cve_queries_status ON custom_cve_queries(status);
CREATE INDEX idx_custom_cve_queries_cve_id ON custom_cve_queries(cve_id);
CREATE INDEX idx_custom_cve_queries_started_at ON custom_cve_queries(started_at);

-- 查询互斥约束
CREATE UNIQUE INDEX idx_custom_cve_queries_single_querying 
    ON custom_cve_queries ((1)) WHERE status = 'querying';

COMMENT ON TABLE custom_cve_queries IS '自定义CVE查询状态表';
COMMENT ON COLUMN custom_cve_queries.status IS '查询状态: querying/success/failed';

-- 2. 创建 host_vulnerability_scripts 表
CREATE TABLE IF NOT EXISTS host_vulnerability_scripts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cve_id VARCHAR(20) NOT NULL,
    host_id UUID NOT NULL REFERENCES hosts(id),
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
    execution_task_id UUID REFERENCES task_logs(id),
    execution_started_at TIMESTAMPTZ,
    execution_completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 索引
CREATE UNIQUE INDEX idx_host_vuln_scripts_unique 
    ON host_vulnerability_scripts (cve_id, host_id, script_type);
CREATE INDEX idx_host_vuln_scripts_cve ON host_vulnerability_scripts(cve_id);
CREATE INDEX idx_host_vuln_scripts_host ON host_vulnerability_scripts(host_id);
CREATE INDEX idx_host_vuln_scripts_gen_status ON host_vulnerability_scripts(generation_status);
CREATE INDEX idx_host_vuln_scripts_exec_status ON host_vulnerability_scripts(execution_status);

COMMENT ON TABLE host_vulnerability_scripts IS '主机漏洞脚本状态表';
COMMENT ON COLUMN host_vulnerability_scripts.script_type IS '脚本类型: poc/fix';
COMMENT ON COLUMN host_vulnerability_scripts.generation_status IS '生成状态: pending/generating/generated/failed';
COMMENT ON COLUMN host_vulnerability_scripts.execution_status IS '执行状态: pending/running/success/failed/timeout';

-- 3. 更新 vulnerabilities 表的 source 字段注释
COMMENT ON COLUMN vulnerabilities.source IS '数据来源: llm_analysis(扫描分析)/nvd_import(NVD导入)/custom_query(自定义查询)';
```

### 5.2 回滚脚本

```sql
-- ============================================
-- V3.1 回滚脚本
-- ============================================

DROP TABLE IF EXISTS host_vulnerability_scripts;
DROP TABLE IF EXISTS custom_cve_queries;
```

---

## 6. ER关系图

```
┌─────────────────┐     ┌──────────────────────┐     ┌─────────────────┐
│   hosts         │     │ host_vulnerability   │     │ vulnerabilities │
│                 │     │ _scripts             │     │                 │
│ id (PK)         │◄────┤ host_id (FK)         │     │ id (PK)         │
│ ip_address      │     │ cve_id               │────►│ cve_id (UK)     │
│ hostname        │     │ script_type          │     │ severity        │
│ os_type         │     │ script_content       │     │ cvss_score      │
│ ...             │     │ generation_status    │     │ description     │
└─────────────────┘     │ execution_status     │     │ source          │
                        │ ...                  │     │ ...             │
                        └──────────────────────┘     └────────┬────────┘
                                                              │
                                                              │ FK
                                                              ▼
┌─────────────────┐     ┌──────────────────────┐     ┌─────────────────┐
│ task_logs       │     │ custom_cve_queries   │     │ llm_configs     │
│                 │     │                      │     │ (现有)          │
│ id (PK)         │◄────┤ result_vulnerability │     │                 │
│ task_type       │     │ _id (FK)             │     └─────────────────┘
│ status          │     │ cve_id               │
│ ...             │     │ status               │
└─────────────────┘     │ started_at           │
                        │ ...                  │
                        └──────────────────────┘
```

---

## 7. 数据库表总览（V3.1完整版）

| 业务领域 | 表名 | 描述 | 版本 |
|:---|:---|:---|:---|
| 资产管理 | `hosts` | Agent纳管主机信息 | V1.6保留 |
| 模板与规则 | `templates` | 基线模板元数据 | V1.6保留 |
| 模板与规则 | `aegis_rules` | 基线检查规则 | V1.6保留 |
| 任务执行 | `task_logs` | 检查/修复任务日志 | V2.2更新 |
| 系统配置 | `llm_configs` | LLM服务配置 | V2.2新增 |
| 脚本管理 | `script_versions` | 脚本版本历史 | V2.2新增 |
| 自愈管理 | `self_healing_logs` | 自愈修复日志 | V2.2新增 |
| 漏洞管理 | `vulnerabilities` | CVE漏洞库 | V3.0新增 |
| 漏洞管理 | `host_vulnerabilities` | 主机漏洞关联 | V3.0新增 |
| 漏洞管理 | `installed_software` | 主机软件清单 | V3.0新增 |
| 漏洞管理 | `vulnerability_fix_scripts` | 漏洞修复脚本 | V3.0新增 |
| 漏洞管理 | `poc_scripts` | POC验证脚本 | V3.0新增 |
| **自定义CVE** | **`custom_cve_queries`** | **自定义CVE查询状态** | **V3.1新增** |
| **自定义CVE** | **`host_vulnerability_scripts`** | **主机漏洞脚本状态** | **V3.1新增** |

**总计：14张表**

---

**文档结束**