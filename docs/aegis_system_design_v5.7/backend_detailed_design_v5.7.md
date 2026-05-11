# V5.7 后端详细设计

**版本**: 5.7
**日期**: 2026-05-07
**状态**: 设计中

---

## 1. 模块变更总览

| 服务 | 变更 | 内容 |
|:---|:---|:---|
| API Server | 新增 | ScriptAuditService, BlacklistChecker, 命令审计API, 审计日志API |
| API Server | 改造 | ScriptGenerationService, VulnerabilityService, SelfHealingService, TaskService |
| Agent | 改造 | eBPF Loader(openat/connect), 内核适配, Pipeline |
| Agent | 新增 | Agent侧BlacklistChecker |
| Server | 无变更 | 规则同步复用UpdateRules RPC |

---

## 2. API Server新增文件

```
api-server/internal/
├── service/script_audit_service.go       # 统一脚本审计
├── checker/blacklist_checker.go          # 黑名单检查器
├── api/handler/
│   ├── command_audit_handler.go          # 命令审计配置API
│   └── audit_log_handler.go             # 审计日志API
├── model/
│   ├── command_audit_rule.go             # 规则模型
│   ├── script_audit_log.go               # 审计日志模型
│   └── system_config.go                  # 系统配置模型
└── repository/
    ├── command_audit_rule_repo.go
    ├── audit_log_repo.go
    └── system_config_repo.go
```

## 3. API Server改造文件

| 文件 | 改造 |
|:---|:---|
| script_generation_service.go | 移除validateScript()，替换为AuditWithRetry() |
| self_healing_service.go | 移除validateScript()，替换为AuditWithRetry() |
| vulnerability_service.go | 增加AuditWithRetry()调用 |
| task_service.go | dispatchToAgent()增加AuditForDispatch() |
| router.go | 增加审计路由 |
| prompts.go | 增加ScriptAuditSystemPrompt |

## 4. API路由

```go
// 命令审计
GET    /api/v1/settings/command-audit/rules
POST   /api/v1/settings/command-audit/rules
PUT    /api/v1/settings/command-audit/rules/:id
DELETE /api/v1/settings/command-audit/rules/:id
PUT    /api/v1/settings/command-audit/rules/:id/toggle
POST   /api/v1/settings/command-audit/rules/batch-toggle
POST   /api/v1/settings/command-audit/rules/test
GET    /api/v1/settings/command-audit/settings
PUT    /api/v1/settings/command-audit/settings

// 审计日志
GET    /api/v1/settings/audit-logs
GET    /api/v1/settings/audit-logs/:id
GET    /api/v1/settings/audit-logs/stats
```

## 5. Agent变更

### 5.1 新增/改造文件

| 文件 | 变更 |
|:---|:---|
| ebpf/loader.go | LoadAll()增加openat/connect, 支持perf buffer |
| ebpf/events.go | 新增FileEvent, ConnEvent |
| ebpf/pipeline.go | buildEventMap()扩展file_access/network_connect |
| ebpf/collector.go | Event结构体扩展 |
| executor/executor.go | 增加BlacklistChecker调用 |
| kernel/detector.go | 新增：内核能力检测 |
| checker/blacklist.go | 新增：Agent侧黑名单 |
| bpf/openat.bpf.c | 增加敏感路径过滤 |
| bpf/connect.bpf.c | 增加IPv6、源地址 |
| Makefile | 增加bpf-noncore目标 |

### 5.2 初始化流程

```go
func main() {
    // 现有流程...
    caps, _ := kernel.Detect()
    logger.Info("内核能力", "version", caps.KernelVersion, "btf", caps.BTFAvailable, "ringbuf", caps.RingbufAvailable)

    blacklistChecker, _ := checker.NewBlacklistChecker("/etc/aegis-agent/audit_rules.json")
    executor := executor.NewExecutor(blacklistChecker)

    collector := ebpf.NewCollector(caps)
    pipeline := ebpf.NewPipeline(collector, sigmaLoader, reporter)
    // ...
}
```

---

## 6. 数据库迁移

```sql
-- V5.7.0

CREATE TABLE command_audit_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(200) NOT NULL,
    description TEXT,
    rule_type VARCHAR(20) NOT NULL DEFAULT 'hard_block',
    match_type VARCHAR(20) NOT NULL DEFAULT 'regex',
    pattern TEXT NOT NULL,
    category VARCHAR(50) NOT NULL DEFAULT 'system',
    severity VARCHAR(20) NOT NULL DEFAULT 'high',
    applies_to JSONB NOT NULL DEFAULT '["all"]',
    is_preset BOOLEAN NOT NULL DEFAULT false,
    is_enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE script_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id VARCHAR(100),
    rule_id VARCHAR(100),
    script_type VARCHAR(50),
    script_content TEXT,
    audit_source VARCHAR(20),
    attempt INT,
    passed BOOLEAN,
    risk_level VARCHAR(20),
    blacklist_hits JSONB,
    ai_analysis JSONB,
    error_msg TEXT,
    duration_ms BIGINT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE system_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    config_key VARCHAR(200) NOT NULL UNIQUE,
    config_value JSONB NOT NULL,
    description TEXT,
    category VARCHAR(50) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_command_audit_rules_category ON command_audit_rules(category);
CREATE INDEX idx_command_audit_rules_enabled ON command_audit_rules(is_enabled);
CREATE INDEX idx_script_audit_log_task_id ON script_audit_log(task_id);
CREATE INDEX idx_script_audit_log_created_at ON script_audit_log(created_at);
CREATE INDEX idx_system_configs_category ON system_configs(category);

-- 初始化配置
INSERT INTO system_configs (config_key, config_value, category)
VALUES ('command_audit.settings', '{"blacklist_enabled":true,"ai_enabled":true,"max_retry":3,"dispatch_check":true,"agent_check":true}', 'command_audit');
```

---

## 7. 任务组状态计算规范

### 7.1 子任务状态（task_logs.status）

| 状态 | 说明 |
|:---|:---|
| `PENDING` | 待执行 |
| `RUNNING` | 执行中 |
| `SUCCESS` | 成功 |
| `FAILED` | 失败 |
| `TIMEOUT` | 超时 |

### 7.2 任务组状态（派生状态）

任务组状态由子任务状态聚合计算，规则如下：

| 任务组状态 | 条件 | 说明 |
|:---|:---|:---|
| `pending` | 所有子任务均为 PENDING | 待执行 |
| `running` | 任意子任务为 RUNNING | 执行中 |
| `success` | 所有子任务均为 SUCCESS | 全部成功 |
| `failed` | 任意子任务为 FAILED 或 TIMEOUT，或非全部成功 | 失败 |

**关键规则**：不存在 `partial` 或 `partial_success` 状态。只要不是全部成功，任务组状态就是 `failed`。

### 7.3 状态计算逻辑（SQL）

```sql
CASE
    WHEN SUM(CASE WHEN status = 'SUCCESS' THEN 1 ELSE 0 END) = COUNT(*) THEN 'success'
    WHEN SUM(CASE WHEN status = 'FAILED' THEN 1 ELSE 0 END) + SUM(CASE WHEN status = 'TIMEOUT' THEN 1 ELSE 0 END) = COUNT(*) THEN 'failed'
    WHEN SUM(CASE WHEN status = 'RUNNING' THEN 1 ELSE 0 END) > 0 THEN 'running'
    WHEN SUM(CASE WHEN status = 'PENDING' THEN 1 ELSE 0 END) = COUNT(*) THEN 'pending'
    ELSE 'failed'
END as status
```

### 7.4 状态计算逻辑（Go 代码）

```go
if status.Running > 0 {
    status.Status = "running"
} else if status.Pending > 0 {
    status.Status = "pending"
} else if status.Failed > 0 || status.Timeout > 0 {
    status.Status = "failed"
} else {
    status.Status = "success"
}
```
