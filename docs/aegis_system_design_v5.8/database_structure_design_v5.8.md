# V5.8 数据库结构设计: 动态 eBPF DetectionPackage

**版本**: 5.8  
**日期**: 2026-05-22  
**状态**: 设计中

---

## 1. 变更概述

V5.8 新增 7 张表：

| 表 | 说明 |
|:---|:---|
| `detection_package_drafts` | 当前检测包草稿，只保留最后一版 |
| `detection_packages` | 已构建/签名/启用的检测包元数据 |
| `detection_package_builds` | 构建任务和构建结果 |
| `detection_package_host_status` | 每台 agent 的包安装运行状态 |
| `detection_package_operations` | 签名、启用、禁用、卸载等操作审计 |
| `ebpf_hook_allowlist_configs` | 全局 hook 白名单配置 |
| `correlation_rules` | package 内 correlation rule 元数据 |

复用现有表：

| 表 | 用途 |
|:---|:---|
| `sigma_rules` | 保存 package 生成的 atomic Sigma rules |
| `runtime_events` | 保存 correlation 命中后的最终告警事件 |
| `alerts` | 保存最终告警 |
| `system_configs` | 可选保存当前 allowlist 快照，不作为主表 |

---

## 2. 状态枚举

### 2.1 Package 状态

```text
draft
build_pending
build_running
build_failed
built
signed
enabled
disabled
uninstalled
```

### 2.2 Host Package 状态

```text
pending
downloading
signature_failed
blocked_by_hook_allowlist
installing
active
degraded
load_failed
disabled_by_policy
disabled_by_rate
rolled_back
uninstalled
```

### 2.3 Operation 类型

```text
create_draft
update_draft
ai_generate
build
sign
enable
disable
uninstall
rollback
allowlist_update
```

---

## 3. detection_package_drafts

当前草稿表。第一版不保存 revision 历史，人工修改直接覆盖。

```sql
CREATE TABLE IF NOT EXISTS detection_package_drafts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id          VARCHAR(160) NOT NULL UNIQUE,
    target_version      VARCHAR(32)  NOT NULL,
    title               VARCHAR(255) NOT NULL,
    description         TEXT,
    cve_ids             JSONB        NOT NULL DEFAULT '[]',
    ai_generated        BOOLEAN      NOT NULL DEFAULT false,
    ai_generation_input JSONB,
    hook_plan_yaml      TEXT,
    ebpf_source         TEXT,
    sigma_rules_yaml    TEXT,
    correlation_yaml    TEXT,
    build_params        JSONB        NOT NULL DEFAULT '{}',
    status              VARCHAR(32)  NOT NULL DEFAULT 'draft',
    last_build_id       UUID,
    created_by          VARCHAR(100),
    updated_by          VARCHAR(100),
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_detection_package_drafts_status
    ON detection_package_drafts(status);

CREATE INDEX IF NOT EXISTS idx_detection_package_drafts_cve_ids
    ON detection_package_drafts USING GIN(cve_ids);
```

字段说明：

| 字段 | 说明 |
|:---|:---|
| `package_id` | 检测能力稳定身份 |
| `target_version` | 计划发布版本，SemVer |
| `hook_plan_yaml` | HookPlan 草稿 |
| `ebpf_source` | eBPF C 源码草稿，只保存在控制面 |
| `sigma_rules_yaml` | atomic Sigma 草稿 |
| `correlation_yaml` | Correlation DetectionSpec 草稿 |
| `build_params` | builder 镜像、编译参数等 |

---

## 4. detection_packages

已构建、签名、发布或启用的 package 元数据。

```sql
CREATE TABLE IF NOT EXISTS detection_packages (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id          VARCHAR(160) NOT NULL,
    version             VARCHAR(32)  NOT NULL,
    title               VARCHAR(255) NOT NULL,
    description         TEXT,
    cve_ids             JSONB        NOT NULL DEFAULT '[]',
    status              VARCHAR(32)  NOT NULL DEFAULT 'built',
    package_object_key  TEXT,
    signature_object_key TEXT,
    package_size        BIGINT       NOT NULL DEFAULT 0,
    package_sha256      VARCHAR(64),
    signed_by           VARCHAR(100),
    signed_at           TIMESTAMP WITH TIME ZONE,
    enabled_at          TIMESTAMP WITH TIME ZONE,
    disabled_at         TIMESTAMP WITH TIME ZONE,
    build_id            UUID,
    builder_image       VARCHAR(255),
    builder_digest      VARCHAR(128),
    manifest_json       JSONB        NOT NULL DEFAULT '{}',
    hook_summary        JSONB        NOT NULL DEFAULT '[]',
    event_schema        JSONB        NOT NULL DEFAULT '{}',
    limits_json         JSONB        NOT NULL DEFAULT '{}',
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(package_id, version)
);

CREATE INDEX IF NOT EXISTS idx_detection_packages_package_id
    ON detection_packages(package_id);

CREATE INDEX IF NOT EXISTS idx_detection_packages_status
    ON detection_packages(status);

CREATE INDEX IF NOT EXISTS idx_detection_packages_cve_ids
    ON detection_packages USING GIN(cve_ids);
```

约束：

- 同 `package_id` 同时只能有一个 `status='enabled'`，业务层保证。
- 默认不允许降级安装；显式 rollback 指令例外。
- `package_object_key` 和 `signature_object_key` 指向 MinIO 对象。

---

## 5. detection_package_builds

构建任务表。

```sql
CREATE TABLE IF NOT EXISTS detection_package_builds (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    draft_id         UUID,
    package_id       VARCHAR(160) NOT NULL,
    version          VARCHAR(32)  NOT NULL,
    status           VARCHAR(32)  NOT NULL DEFAULT 'pending',
    builder_image    VARCHAR(255) NOT NULL,
    builder_digest   VARCHAR(128),
    clang_version    VARCHAR(100),
    started_at       TIMESTAMP WITH TIME ZONE,
    finished_at      TIMESTAMP WITH TIME ZONE,
    duration_ms      BIGINT,
    artifact_summary JSONB       NOT NULL DEFAULT '{}',
    hook_summary     JSONB       NOT NULL DEFAULT '[]',
    event_schema     JSONB       NOT NULL DEFAULT '{}',
    unsigned_package_object_key TEXT,
    unsigned_package_sha256 VARCHAR(64),
    unsigned_package_size BIGINT NOT NULL DEFAULT 0,
    build_log_object_key TEXT,
    build_log        TEXT,
    error_message    TEXT,
    created_by       VARCHAR(100),
    created_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_detection_package_builds_package
    ON detection_package_builds(package_id, version);

CREATE INDEX IF NOT EXISTS idx_detection_package_builds_status
    ON detection_package_builds(status);
```

`status`：

```text
pending
running
success
failed
cancelled
```

说明：

- `unsigned_package_object_key` 只允许用于签名发布流程，不能下发 agent。
- `build_log_object_key` 保存 builder 上传到 MinIO 的完整构建日志。
- `build_log` 可以只保存日志尾部，便于列表和详情快速展示。

---

## 6. detection_package_host_status

主机级安装运行状态。

```sql
CREATE TABLE IF NOT EXISTS detection_package_host_status (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id         VARCHAR(160) NOT NULL,
    version            VARCHAR(32)  NOT NULL,
    host_id            UUID         NOT NULL,
    hostname           VARCHAR(255),
    status             VARCHAR(64)  NOT NULL DEFAULT 'pending',
    plugin_status      VARCHAR(64),
    sigma_status       VARCHAR(64),
    correlation_status VARCHAR(64),
    active_artifact    VARCHAR(16),
    loaded_hooks       JSONB        NOT NULL DEFAULT '[]',
    kernel_release     VARCHAR(128),
    arch               VARCHAR(32),
    error_message      TEXT,
    metrics_json       JSONB        NOT NULL DEFAULT '{}',
    installed_at       TIMESTAMP WITH TIME ZONE,
    updated_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_reported_at   TIMESTAMP WITH TIME ZONE,
    UNIQUE(package_id, version, host_id)
);

CREATE INDEX IF NOT EXISTS idx_detection_package_host_status_package
    ON detection_package_host_status(package_id, version);

CREATE INDEX IF NOT EXISTS idx_detection_package_host_status_host
    ON detection_package_host_status(host_id);

CREATE INDEX IF NOT EXISTS idx_detection_package_host_status_status
    ON detection_package_host_status(status);
```

---

## 7. detection_package_operations

操作审计表。

```sql
CREATE TABLE IF NOT EXISTS detection_package_operations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id    VARCHAR(160),
    version       VARCHAR(32),
    operation     VARCHAR(64)  NOT NULL,
    operator      VARCHAR(100),
    request_json  JSONB        NOT NULL DEFAULT '{}',
    result_json   JSONB        NOT NULL DEFAULT '{}',
    success       BOOLEAN      NOT NULL DEFAULT true,
    error_message TEXT,
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_detection_package_operations_package
    ON detection_package_operations(package_id, version);

CREATE INDEX IF NOT EXISTS idx_detection_package_operations_operation
    ON detection_package_operations(operation);
```

---

## 8. ebpf_hook_allowlist_configs

全局 hook 白名单。agent 不内置白名单，页面默认配置写入此表后下发。

```sql
CREATE TABLE IF NOT EXISTS ebpf_hook_allowlist_configs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version       BIGSERIAL UNIQUE,
    config_json   JSONB NOT NULL,
    description   TEXT,
    updated_by    VARCHAR(100),
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    activated_at  TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_ebpf_hook_allowlist_configs_activated
    ON ebpf_hook_allowlist_configs(activated_at DESC);
```

当前有效配置：

```sql
SELECT * FROM ebpf_hook_allowlist_configs
WHERE activated_at IS NOT NULL
ORDER BY activated_at DESC
LIMIT 1;
```

默认 `config_json`：

```json
{
  "tracepoints": [
    "syscalls/sys_enter_socket",
    "syscalls/sys_enter_bind",
    "syscalls/sys_enter_splice",
    "syscalls/sys_enter_execve",
    "syscalls/sys_exit_execve",
    "syscalls/sys_enter_setuid",
    "syscalls/sys_enter_setgid",
    "syscalls/sys_enter_capset",
    "sched/sched_process_fork",
    "sched/sched_process_exit"
  ],
  "kprobes": [],
  "lsm": [],
  "xdp": [],
  "tc": []
}
```

---

## 9. correlation_rules

保存 package 内 correlation 规则元数据，便于列表和详情查询。YAML 原文仍保存在 package 中，表中保存解析摘要。

```sql
CREATE TABLE IF NOT EXISTS correlation_rules (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id       VARCHAR(160) NOT NULL,
    package_version  VARCHAR(32)  NOT NULL,
    rule_id          VARCHAR(220) NOT NULL,
    title            VARCHAR(255),
    severity         VARCHAR(32),
    by_key           VARCHAR(32),
    window_seconds   INTEGER,
    ordered          BOOLEAN      NOT NULL DEFAULT true,
    sequence_json    JSONB        NOT NULL DEFAULT '[]',
    content          TEXT         NOT NULL,
    created_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(package_id, package_version, rule_id)
);

CREATE INDEX IF NOT EXISTS idx_correlation_rules_package
    ON correlation_rules(package_id, package_version);
```

---

## 10. sigma_rules 扩展建议

复用现有 `sigma_rules` 表，新增 package 关联字段：

```sql
ALTER TABLE sigma_rules
    ADD COLUMN IF NOT EXISTS package_id VARCHAR(160),
    ADD COLUMN IF NOT EXISTS package_version VARCHAR(32),
    ADD COLUMN IF NOT EXISTS package_rule_type VARCHAR(32) DEFAULT 'standalone';

CREATE INDEX IF NOT EXISTS idx_sigma_rules_package
    ON sigma_rules(package_id, package_version);
```

`package_rule_type`：

```text
standalone
atomic
```

---

## 11. runtime_events / alerts 扩展建议

Correlation 命中的事件继续写入现有运行时事件和告警表。建议新增或复用 `event_data_json` 保存 evidence chain。

如需查询优化，可新增字段：

```sql
ALTER TABLE runtime_events
    ADD COLUMN IF NOT EXISTS package_id VARCHAR(160),
    ADD COLUMN IF NOT EXISTS correlation_rule_id VARCHAR(220);

CREATE INDEX IF NOT EXISTS idx_runtime_events_package
    ON runtime_events(package_id);
```

---

## 12. 初始化与迁移顺序

1. 创建新表。
2. 扩展 `sigma_rules`。
3. 扩展 `runtime_events`。
4. 初始化默认 hook allowlist。
5. 初始化 package 状态枚举的应用层校验。

---

## 13. 数据保留策略

| 数据 | 保留策略 |
|:---|:---|
| 草稿 | 只保存最后一版，删除 package 时可保留或清理 |
| 构建日志 | 默认保留 180 天 |
| package 元数据 | 长期保留 |
| host status | 只保留当前状态 |
| operation audit | 长期保留 |
| MinIO package artifact | 已卸载包可按策略清理 |
