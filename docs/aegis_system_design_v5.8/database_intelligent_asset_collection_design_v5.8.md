# V5.8 数据库设计: 智能资产采集

**版本**: 5.8  
**日期**: 2026-06-04  
**状态**: 设计中  

---

## 1. 变更概述

新增 7 张表：

| 表 | 说明 |
|:---|:---|
| `asset_collection_configs` | 资产周期采集配置 |
| `asset_collection_tasks` | 采集任务主表 |
| `asset_collection_task_hosts` | 任务主机执行明细 |
| `host_software_assets` | 主机软件包资产 |
| `host_process_snapshots` | 主机进程原始快照摘要 |
| `host_application_assets` | AI 识别后的应用资产 |
| `host_application_tool_calls` | AI 版本识别工具调用证据 |

复用现有表：

| 表 | 用途 |
|:---|:---|
| `hosts` | 主机基础信息 |
| `llm_configs` | LLM 模型配置 |
| `audit_logs` | 手动采集、人工复核审计 |

---

## 2. 枚举

### 2.1 采集任务状态

```text
pending
running
agent_offline
collect_failed
ai_analyzing
ai_failed
completed
cancelled
```

### 2.2 采集类型

```text
software
process
application_analysis
full
```

### 2.3 软件状态

```text
active
inactive
deleted
```

### 2.4 应用分类

```text
database
web_service
web_framework
web_site
other
unknown
```

### 2.5 应用复核状态

```text
pending
confirmed
rejected
auto
```

---

## 3. asset_collection_configs

资产周期采集配置。第一版只保留一条全局配置。

```sql
CREATE TABLE IF NOT EXISTS asset_collection_configs (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    enabled            BOOLEAN NOT NULL DEFAULT true,
    interval_hours     INT NOT NULL DEFAULT 12,
    collect_types      JSONB NOT NULL DEFAULT '["software","process","application_analysis"]',
    scope              VARCHAR(32) NOT NULL DEFAULT 'all_hosts',
    next_run_at        TIMESTAMPTZ,
    last_run_at        TIMESTAMPTZ,
    updated_by         VARCHAR(100),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_asset_collection_config_interval CHECK (interval_hours >= 1 AND interval_hours <= 168),
    CONSTRAINT chk_asset_collection_config_scope CHECK (scope IN ('all_hosts','host_group','hosts'))
);

CREATE INDEX IF NOT EXISTS idx_asset_collection_configs_next_run
    ON asset_collection_configs(enabled, next_run_at);

INSERT INTO asset_collection_configs (enabled, interval_hours, collect_types, scope)
SELECT true, 12, '["software","process","application_analysis"]'::jsonb, 'all_hosts'
WHERE NOT EXISTS (SELECT 1 FROM asset_collection_configs);
```

字段说明：

| 字段 | 说明 |
|:---|:---|
| `enabled` | 是否启用周期采集 |
| `interval_hours` | 采集周期，默认 12 小时，范围 1 到 168 |
| `collect_types` | 周期采集内容 |
| `scope` | 第一版默认 all_hosts |
| `next_run_at` | 下一次计划执行时间 |
| `last_run_at` | 最近一次周期执行时间 |

---

## 4. asset_collection_tasks

```sql
CREATE TABLE IF NOT EXISTS asset_collection_tasks (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_type          VARCHAR(32) NOT NULL DEFAULT 'full',
    trigger_source     VARCHAR(32) NOT NULL DEFAULT 'manual',
    scope              VARCHAR(32) NOT NULL DEFAULT 'hosts',
    host_filter        JSONB NOT NULL DEFAULT '[]',
    collect_types      JSONB NOT NULL DEFAULT '["software","process","application_analysis"]',
    status             VARCHAR(32) NOT NULL DEFAULT 'pending',
    total_hosts        INT NOT NULL DEFAULT 0,
    success_hosts      INT NOT NULL DEFAULT 0,
    failed_hosts       INT NOT NULL DEFAULT 0,
    current_stage      VARCHAR(64),
    error_message      TEXT,
    requested_by       VARCHAR(100),
    started_at         TIMESTAMPTZ,
    finished_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_asset_collection_task_status CHECK (
        status IN ('pending','running','agent_offline','collect_failed','ai_analyzing','ai_failed','completed','cancelled')
    )
);

CREATE INDEX IF NOT EXISTS idx_asset_collection_tasks_status
    ON asset_collection_tasks(status);
CREATE INDEX IF NOT EXISTS idx_asset_collection_tasks_created_at
    ON asset_collection_tasks(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_asset_collection_tasks_host_filter
    ON asset_collection_tasks USING GIN(host_filter);
```

---

## 5. asset_collection_task_hosts

```sql
CREATE TABLE IF NOT EXISTS asset_collection_task_hosts (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID NOT NULL REFERENCES asset_collection_tasks(id) ON DELETE CASCADE,
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    hostname           VARCHAR(255),
    ip_address         VARCHAR(45),
    status             VARCHAR(32) NOT NULL DEFAULT 'pending',
    collect_started_at TIMESTAMPTZ,
    collect_finished_at TIMESTAMPTZ,
    software_count     INT NOT NULL DEFAULT 0,
    process_count      INT NOT NULL DEFAULT 0,
    application_count  INT NOT NULL DEFAULT 0,
    error_message      TEXT,
    raw_snapshot_id    UUID,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(task_id, host_id)
);

CREATE INDEX IF NOT EXISTS idx_asset_collection_task_hosts_task
    ON asset_collection_task_hosts(task_id);
CREATE INDEX IF NOT EXISTS idx_asset_collection_task_hosts_host
    ON asset_collection_task_hosts(host_id);
CREATE INDEX IF NOT EXISTS idx_asset_collection_task_hosts_status
    ON asset_collection_task_hosts(status);
```

---

## 6. host_software_assets

主机软件包资产表。保留当前 active 版本，重复采集时 upsert。

```sql
CREATE TABLE IF NOT EXISTS host_software_assets (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    hostname           VARCHAR(255),
    ip_address         VARCHAR(45),
    group_name         VARCHAR(255) NOT NULL DEFAULT '默认分组',
    os_type            VARCHAR(50) NOT NULL,
    package_manager    VARCHAR(32) NOT NULL,
    name               VARCHAR(255) NOT NULL,
    version            VARCHAR(255),
    release            VARCHAR(255),
    epoch              VARCHAR(64),
    architecture       VARCHAR(64),
    source_name        VARCHAR(255),
    vendor             VARCHAR(255),
    license            VARCHAR(255),
    install_paths      JSONB NOT NULL DEFAULT '[]',
    file_count         INT NOT NULL DEFAULT 0,
    package_metadata   JSONB NOT NULL DEFAULT '{}',
    fingerprint        VARCHAR(128) NOT NULL,
    status             VARCHAR(32) NOT NULL DEFAULT 'active',
    last_modified_at   TIMESTAMPTZ,
    first_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    collected_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_host_software_package_manager CHECK (package_manager IN ('rpm','dpkg','apk','unknown')),
    CONSTRAINT chk_host_software_status CHECK (status IN ('active','inactive','deleted')),
    UNIQUE(host_id, package_manager, fingerprint)
);

CREATE INDEX IF NOT EXISTS idx_host_software_assets_host
    ON host_software_assets(host_id);
CREATE INDEX IF NOT EXISTS idx_host_software_assets_name
    ON host_software_assets(name);
CREATE INDEX IF NOT EXISTS idx_host_software_assets_version
    ON host_software_assets(version);
CREATE INDEX IF NOT EXISTS idx_host_software_assets_manager
    ON host_software_assets(package_manager);
CREATE INDEX IF NOT EXISTS idx_host_software_assets_seen
    ON host_software_assets(last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_host_software_assets_paths
    ON host_software_assets USING GIN(install_paths);
```

`fingerprint` 规则：

```text
sha256(host_id + package_manager + name + version + release + architecture)
```

---

## 7. host_process_snapshots

保存每次采集的进程快照摘要。原始大 JSON 可压缩存储在 `snapshot_json`，保留时间短于资产表。

```sql
CREATE TABLE IF NOT EXISTS host_process_snapshots (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID REFERENCES asset_collection_tasks(id) ON DELETE SET NULL,
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    hostname           VARCHAR(255),
    ip_address         VARCHAR(45),
    process_count      INT NOT NULL DEFAULT 0,
    listen_port_count  INT NOT NULL DEFAULT 0,
    snapshot_hash      VARCHAR(64) NOT NULL,
    snapshot_json      JSONB NOT NULL DEFAULT '{}',
    redaction_summary  JSONB NOT NULL DEFAULT '{}',
    collected_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_host_process_snapshots_host
    ON host_process_snapshots(host_id, collected_at DESC);
CREATE INDEX IF NOT EXISTS idx_host_process_snapshots_task
    ON host_process_snapshots(task_id);
CREATE INDEX IF NOT EXISTS idx_host_process_snapshots_hash
    ON host_process_snapshots(snapshot_hash);
```

---

## 8. host_application_assets

AI 识别后的应用资产表。

```sql
CREATE TABLE IF NOT EXISTS host_application_assets (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    hostname           VARCHAR(255),
    ip_address         VARCHAR(45),
    group_name         VARCHAR(255) NOT NULL DEFAULT '默认分组',
    os_type            VARCHAR(50) NOT NULL,
    category           VARCHAR(32) NOT NULL DEFAULT 'unknown',
    name               VARCHAR(255) NOT NULL,
    display_name       VARCHAR(255),
    version            VARCHAR(255),
    version_source     VARCHAR(64),
    install_path       TEXT,
    start_path         TEXT,
    config_paths       JSONB NOT NULL DEFAULT '[]',
    site_paths         JSONB NOT NULL DEFAULT '[]',
    domains            JSONB NOT NULL DEFAULT '[]',
    listen_ports       JSONB NOT NULL DEFAULT '[]',
    run_user           VARCHAR(255),
    runtime_name       VARCHAR(100),
    runtime_version    VARCHAR(100),
    framework_name     VARCHAR(100),
    framework_version  VARCHAR(100),
    related_pids       JSONB NOT NULL DEFAULT '[]',
    related_packages   JSONB NOT NULL DEFAULT '[]',
    ai_confidence      NUMERIC(4,3) NOT NULL DEFAULT 0,
    ai_evidence        JSONB NOT NULL DEFAULT '[]',
    ai_raw_output      JSONB NOT NULL DEFAULT '{}',
    manual_overrides   JSONB NOT NULL DEFAULT '{}',
    review_status      VARCHAR(32) NOT NULL DEFAULT 'auto',
    status             VARCHAR(32) NOT NULL DEFAULT 'active',
    fingerprint        VARCHAR(128) NOT NULL,
    first_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    collected_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_host_application_category CHECK (
        category IN ('database','web_service','web_framework','web_site','other','unknown')
    ),
    CONSTRAINT chk_host_application_review CHECK (
        review_status IN ('pending','confirmed','rejected','auto')
    ),
    CONSTRAINT chk_host_application_status CHECK (
        status IN ('active','inactive','deleted','needs_review')
    ),
    UNIQUE(host_id, fingerprint)
);

CREATE INDEX IF NOT EXISTS idx_host_application_assets_host
    ON host_application_assets(host_id);
CREATE INDEX IF NOT EXISTS idx_host_application_assets_category
    ON host_application_assets(category);
CREATE INDEX IF NOT EXISTS idx_host_application_assets_name
    ON host_application_assets(name);
CREATE INDEX IF NOT EXISTS idx_host_application_assets_version
    ON host_application_assets(version);
CREATE INDEX IF NOT EXISTS idx_host_application_assets_ports
    ON host_application_assets USING GIN(listen_ports);
CREATE INDEX IF NOT EXISTS idx_host_application_assets_review
    ON host_application_assets(review_status);
CREATE INDEX IF NOT EXISTS idx_host_application_assets_seen
    ON host_application_assets(last_seen_at DESC);
```

`fingerprint` 规则：

```text
sha256(host_id + category + normalized_name + install_path + start_path + sorted(listen_ports))
```

---

## 9. host_application_tool_calls

记录 AI 为版本识别和配置识别调用 Agent 工具的证据。

```sql
CREATE TABLE IF NOT EXISTS host_application_tool_calls (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID REFERENCES asset_collection_tasks(id) ON DELETE SET NULL,
    application_id     UUID REFERENCES host_application_assets(id) ON DELETE SET NULL,
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    call_id            VARCHAR(128) NOT NULL,
    tool_name          VARCHAR(128) NOT NULL,
    arguments_json     JSONB NOT NULL DEFAULT '{}',
    result_json        JSONB NOT NULL DEFAULT '{}',
    success            BOOLEAN NOT NULL DEFAULT false,
    error_message      TEXT,
    execution_time_ms  BIGINT NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(call_id)
);

CREATE INDEX IF NOT EXISTS idx_host_application_tool_calls_app
    ON host_application_tool_calls(application_id);
CREATE INDEX IF NOT EXISTS idx_host_application_tool_calls_host
    ON host_application_tool_calls(host_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_host_application_tool_calls_tool
    ON host_application_tool_calls(tool_name);
```

---

## 10. 漏洞扫描资产引用扩展

V5.8 漏洞扫描改为读取资产库，不再扫描时调用 Agent 采集。建议扩展现有主机漏洞关联表，或新增扫描结果表字段，保存资产引用。

如果沿用 `host_vulnerabilities`，建议新增字段：

```sql
ALTER TABLE host_vulnerabilities
    ADD COLUMN IF NOT EXISTS software_asset_id UUID REFERENCES host_software_assets(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS application_asset_id UUID REFERENCES host_application_assets(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS asset_name VARCHAR(255),
    ADD COLUMN IF NOT EXISTS asset_version VARCHAR(255),
    ADD COLUMN IF NOT EXISTS asset_collected_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS vulnerability_source JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS match_evidence JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS verification_status VARCHAR(32) NOT NULL DEFAULT 'verified';

CREATE INDEX IF NOT EXISTS idx_host_vulnerabilities_software_asset
    ON host_vulnerabilities(software_asset_id);
CREATE INDEX IF NOT EXISTS idx_host_vulnerabilities_application_asset
    ON host_vulnerabilities(application_asset_id);
CREATE INDEX IF NOT EXISTS idx_host_vulnerabilities_verification_status
    ON host_vulnerabilities(verification_status);
```

`verification_status` 枚举：

```text
verified
needs_review
rejected
```

约束规则：

- 正式漏洞必须 `verification_status='verified'`。
- `vulnerability_source` 必须包含真实来源类型和 URL 或人工来源说明。
- `software_asset_id` 与 `application_asset_id` 至少一个不为空。
- LLM 无来源候选不得写入正式漏洞，可写入任务详情 JSON 或候选表。

---

## 11. 视图建议

为前端分类查询创建视图或 repository 查询方法：

```sql
CREATE OR REPLACE VIEW host_database_assets AS
SELECT * FROM host_application_assets
WHERE category = 'database' AND status <> 'deleted';

CREATE OR REPLACE VIEW host_web_service_assets AS
SELECT * FROM host_application_assets
WHERE category = 'web_service' AND status <> 'deleted';

CREATE OR REPLACE VIEW host_web_framework_assets AS
SELECT * FROM host_application_assets
WHERE category = 'web_framework' AND status <> 'deleted';

CREATE OR REPLACE VIEW host_web_site_assets AS
SELECT * FROM host_application_assets
WHERE category = 'web_site' AND status <> 'deleted';
```

如果迁移策略不希望创建 view，可在 repository 层用固定 category 查询替代。

---

## 12. 数据保留策略

| 数据 | 保留策略 |
|:---|:---|
| 软件资产 | 长期保留，消失后标记 inactive |
| 应用资产 | 长期保留，消失后标记 inactive |
| 进程快照 | 默认保留 30 天 |
| 工具调用记录 | 默认保留 90 天 |
| 采集任务 | 默认保留 180 天 |
| 周期采集配置 | 长期保留 |

---

## 13. 迁移文件

建议新增：

```text
migrations/015_v5.8_intelligent_asset_collection.sql
```

迁移顺序位于 V5.7 `014_v5.7_backfill_session_conclusions.sql` 之后。

---

## 14. 回滚

回滚时：

- 前端隐藏菜单。
- 周期任务关闭。
- 新表可保留，不影响现有表。
- 如必须清理，按外键顺序删除：
  1. `host_application_tool_calls`
  2. `host_application_assets`
  3. `host_process_snapshots`
  4. `host_software_assets`
  5. `asset_collection_task_hosts`
  6. `asset_collection_tasks`
  7. `asset_collection_configs`
