# Aegis智能主机安全系统 V5.6 数据库结构设计文档

**版本**: 5.6
**日期**: 2026-04-14
**状态**: 设计中

---

## 1. 概述

### 1.1 设计目标

V5.6版本数据库主要新增以下表：

| 表名 | 说明 |
|------|------|
| `ai_analysis_session` | AI分析会话表 |
| `ai_analysis_message` | AI分析消息表 |
| `tool_execution_log` | 工具执行日志表 |
| `sigma_rules` | Sigma规则表（增强） |

---

## 2. 新增表设计

### 2.1 ai_analysis_session（AI分析会话表）

```sql
-- AI分析会话表
CREATE TABLE IF NOT EXISTS ai_analysis_session (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id VARCHAR(100) UNIQUE NOT NULL,
    user_id VARCHAR(100),
    user_name VARCHAR(200),

    -- 分析范围
    alert_ids JSONB DEFAULT '[]',           -- 分析的告警ID列表
    host_filter JSONB DEFAULT '[]',         -- 主机过滤列表
    time_range JSONB,                       -- 时间范围 {"start": "2026-04-14T10:00:00Z", "end": "2026-04-14T11:00:00Z"}

    -- 会话状态
    status VARCHAR(20) DEFAULT 'active',    -- active, completed, cancelled

    -- 统计信息
    message_count INTEGER DEFAULT 0,
    tool_call_count INTEGER DEFAULT 0,

    -- 时间戳
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    ended_at TIMESTAMP WITH TIME ZONE,

    -- 备注
    conclusion JSONB,                       -- 分析结论 {"actions": [...]}

    CONSTRAINT chk_status CHECK (status IN ('active', 'completed', 'cancelled'))
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_ai_session_session_id ON ai_analysis_session(session_id);
CREATE INDEX IF NOT EXISTS idx_ai_session_user_id ON ai_analysis_session(user_id);
CREATE INDEX IF NOT EXISTS idx_ai_session_status ON ai_analysis_session(status);
CREATE INDEX IF NOT EXISTS idx_ai_session_created_at ON ai_analysis_session(created_at);
```

### 2.2 ai_analysis_message（AI分析消息表）

```sql
-- AI分析消息表
CREATE TABLE IF NOT EXISTS ai_analysis_message (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id VARCHAR(100) NOT NULL,
    message_id VARCHAR(100) UNIQUE NOT NULL,

    -- 消息角色和内容
    role VARCHAR(20) NOT NULL,             -- user, assistant, system, tool
    content TEXT,

    -- 工具调用信息
    tool_calls JSONB,                       -- [{"call_id": "xxx", "tool": "GetProcessTree", "arguments": {...}}]
    tool_results JSONB,                     -- [{"call_id": "xxx", "result": {...}, "executed_at": "..."}]

    -- 消息关联
    parent_message_id VARCHAR(100),        -- 父消息ID（用于对话树）
    root_message_id VARCHAR(100),          -- 根消息ID

    -- 时间戳
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- 约束
    CONSTRAINT chk_role CHECK (role IN ('user', 'assistant', 'system', 'tool'))
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_ai_msg_session_id ON ai_analysis_message(session_id);
CREATE INDEX IF NOT EXISTS idx_ai_msg_message_id ON ai_analysis_message(message_id);
CREATE INDEX IF NOT EXISTS idx_ai_msg_parent ON ai_analysis_message(parent_message_id);
CREATE INDEX IF NOT EXISTS idx_ai_msg_created_at ON ai_analysis_message(created_at);

-- 外键
ALTER TABLE ai_analysis_message
    ADD CONSTRAINT fk_ai_msg_session
    FOREIGN KEY (session_id) REFERENCES ai_analysis_session(session_id)
    ON DELETE CASCADE;
```

### 2.3 tool_execution_log（工具执行日志表）

```sql
-- 工具执行日志表
CREATE TABLE IF NOT EXISTS tool_execution_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id VARCHAR(100),               -- 可为空（直接调用工具时）
    call_id VARCHAR(100) UNIQUE NOT NULL,

    -- 工具调用信息
    tool_name VARCHAR(50) NOT NULL,
    host_id UUID,
    host_ip VARCHAR(45),
    host_name VARCHAR(200),

    -- 调用参数和结果
    parameters JSONB,                      -- {"pid": 12345, ...}
    result JSONB,                           -- {"process": {...}, "children": [...]}
    error TEXT,

    -- 执行统计
    execution_time_ms INTEGER,
    queue_time_ms INTEGER,                 -- 排队等待时间
    total_time_ms INTEGER,                 -- 总耗时

    -- 调用来源
    source VARCHAR(20) DEFAULT 'ai_analysis',  -- ai_analysis, manual, system
    triggered_by VARCHAR(100),             -- 触发者（用户ID或系统）

    -- 时间戳
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- 状态
    status VARCHAR(20) DEFAULT 'pending',   -- pending, running, completed, failed, timeout

    CONSTRAINT chk_tool_status CHECK (status IN ('pending', 'running', 'completed', 'failed', 'timeout'))
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_tool_log_session_id ON tool_execution_log(session_id);
CREATE INDEX IF NOT EXISTS idx_tool_log_call_id ON tool_execution_log(call_id);
CREATE INDEX IF NOT EXISTS idx_tool_log_tool_name ON tool_execution_log(tool_name);
CREATE INDEX IF NOT EXISTS idx_tool_log_host_id ON tool_execution_log(host_id);
CREATE INDEX IF NOT EXISTS idx_tool_log_status ON tool_execution_log(status);
CREATE INDEX IF NOT EXISTS idx_tool_log_created_at ON tool_execution_log(created_at);

-- 外键（可选）
ALTER TABLE tool_execution_log
    ADD CONSTRAINT fk_tool_log_session
    FOREIGN KEY (session_id) REFERENCES ai_analysis_session(session_id)
    ON DELETE SET NULL;
```

### 2.4 sigma_rules表（增强）

```sql
-- Sigma规则表（V5.6增强）
-- 假设原有表结构如下，添加新增字段：

ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS source VARCHAR(50) DEFAULT 'manual';
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS file_name VARCHAR(255);
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS file_hash VARCHAR(64);
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS file_size INTEGER;
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS parsed_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS parse_error TEXT;
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS ai_generated BOOLEAN DEFAULT FALSE;
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS parent_rule_id VARCHAR(100);
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS generation_prompt TEXT;
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS generation_context JSONB;
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS approved_by VARCHAR(100);
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS approved_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS dispatch_hosts JSONB DEFAULT '[]';  -- 已下发的主机列表
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS dispatch_status VARCHAR(20) DEFAULT 'pending';  -- pending, dispatched, partial_failed

-- 源类型枚举
-- 'manual': 手动上传
-- 'ai_generated': AI生成
-- 'converted': 从其他格式转换

COMMENT ON COLUMN sigma_rules.source IS '规则来源: manual(手动上传), ai_generated(AI生成), converted(格式转换)';
COMMENT ON COLUMN sigma_rules.file_hash IS '文件SHA256哈希，用于去重';
COMMENT ON COLUMN sigma_rules.ai_generated IS '是否为AI生成的规则';
COMMENT ON COLUMN sigma_rules.parent_rule_id IS '父规则ID（AI基于某规则改进时）';
COMMENT ON COLUMN sigma_rules.dispatch_hosts IS '已下发的主机ID列表';
COMMENT ON COLUMN sigma_rules.dispatch_status IS '下发状态: pending(待下发), dispatched(已下发), partial_failed(部分失败)';

-- 新增索引
CREATE INDEX IF NOT EXISTS idx_sigma_rules_source ON sigma_rules(source);
CREATE INDEX IF NOT EXISTS idx_sigma_rules_file_hash ON sigma_rules(file_hash);
CREATE INDEX IF NOT EXISTS idx_sigma_rules_ai_generated ON sigma_rules(ai_generated);
CREATE INDEX IF NOT EXISTS idx_sigma_rules_parent_rule_id ON sigma_rules(parent_rule_id);
CREATE INDEX IF NOT EXISTS idx_sigma_rules_dispatch_status ON sigma_rules(dispatch_status);
```

---

## 3. 现有表修改

### 3.1 alerts表（可能需要添加字段）

```sql
-- 如果需要，添加以下字段
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS analysis_session_id VARCHAR(100);
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS ai_judgment TEXT;                  -- AI分析判断
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS ai_confidence DECIMAL(5,2);       -- AI置信度
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS false_positive_reason TEXT;       -- 误报原因
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS confirmed_threat BOOLEAN DEFAULT FALSE;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS confirmed_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS confirmed_by VARCHAR(100);

-- 索引
CREATE INDEX IF NOT EXISTS idx_alerts_analysis_session ON alerts(analysis_session_id);
CREATE INDEX IF NOT EXISTS idx_alerts_confirmed_threat ON alerts(confirmed_threat) WHERE confirmed_threat = TRUE;
```

### 3.2 block_records表（可能需要添加字段）

```sql
-- 添加工具调用相关信息
ALTER TABLE block_records ADD COLUMN IF NOT EXISTS tool_call_id VARCHAR(100);
ALTER TABLE block_records ADD COLUMN IF NOT EXISTS tool_name VARCHAR(50);
```

---

## 4. AI规则配置表

### 4.1 ai_rule_config（AI规则配置表）

```sql
-- AI规则生成配置表
CREATE TABLE IF NOT EXISTS ai_rule_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- 配置名称
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,

    -- 功能开关
    enabled BOOLEAN DEFAULT FALSE,

    -- 模式: suggest(仅建议), auto(自动)
    mode VARCHAR(20) DEFAULT 'suggest',
    CONSTRAINT chk_mode CHECK (mode IN ('suggest', 'auto')),

    -- 触发条件
    triggers JSONB DEFAULT '[]',           -- ["high_frequency", "new_mitre", "critical", "manual"]

    -- 触发阈值
    thresholds JSONB DEFAULT '{"high_frequency_count": 5, "high_frequency_hours": 24}',

    -- 生成策略 (0.0-1.0, 越低越保守)
    conservatism DECIMAL(3,2) DEFAULT 0.5,

    -- 审核配置
    require_approval BOOLEAN DEFAULT TRUE,
    auto_activate_after_approval BOOLEAN DEFAULT FALSE,
    activation_delay_hours INTEGER DEFAULT 24,  -- 审核通过后延迟激活时间

    -- 通知配置
    notify_on_generation BOOLEAN DEFAULT TRUE,
    notify_on_approval BOOLEAN DEFAULT TRUE,
    notification_targets JSONB DEFAULT '[]',  -- ["email:xxx", "webhook:xxx"]

    -- 统计
    rules_generated_count INTEGER DEFAULT 0,
    rules_approved_count INTEGER DEFAULT 0,

    -- 时间戳
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(100),
    updated_by VARCHAR(100)
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_ai_config_enabled ON ai_rule_config(enabled);
CREATE INDEX IF NOT EXISTS idx_ai_config_mode ON ai_rule_config(mode);
```

---

## 5. ER图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              ER关系图                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────┐         ┌─────────────────────┐                    │
│  │  ai_analysis_       │         │  sigma_rules        │                    │
│  │  session            │         │                     │                    │
│  ├─────────────────────┤         ├─────────────────────┤                    │
│  │ id (PK)             │         │ id (PK)             │                    │
│  │ session_id (UK)     │         │ rule_id (UK)        │                    │
│  │ user_id             │         │ title               │                    │
│  │ alert_ids (JSONB)   │◄───────►│ mitre_id            │                    │
│  │ host_filter (JSONB)│  多对多  │ status              │                    │
│  │ time_range (JSONB)  │         │ content             │                    │
│  │ status              │         │ source              │                    │
│  │ created_at          │         │ ai_generated        │                    │
│  └─────────┬───────────┘         │ parent_rule_id (FK) │                    │
│            │                     └─────────────────────┘                    │
│            │ 1:N                                                ▲
│            │                     ┌─────────────────────┐         │
│            ▼                     │  ai_analysis_       │         │
│  ┌─────────────────────┐         │  message            │         │
│  │  tool_execution_log  │         ├─────────────────────┤         │
│  ├─────────────────────┤         │ id (PK)             │         │
│  │ id (PK)             │         │ session_id (FK)     │─────────┘
│  │ session_id (FK)     │─────────│ message_id (UK)     │
│  │ call_id (UK)        │         │ role                │
│  │ tool_name           │         │ content             │
│  │ host_id             │         │ tool_calls (JSONB)  │
│  │ parameters (JSONB)  │         │ tool_results (JSONB)│
│  │ result (JSONB)      │         │ created_at          │
│  │ status              │         └─────────────────────┘
│  │ execution_time_ms   │
│  │ created_at          │
│  └─────────────────────┘
│                                                                              │
│  ┌─────────────────────┐
│  │  ai_rule_config     │
│  ├─────────────────────┤
│  │ id (PK)             │
│  │ name (UK)           │
│  │ enabled             │
│  │ mode                │
│  │ triggers (JSONB)    │
│  │ thresholds (JSONB)  │
│  │ conservatism        │
│  │ require_approval    │
│  └─────────────────────┘
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 6. 查询示例

### 6.1 获取会话的所有消息（带工具调用结果）

```sql
SELECT
    m.*,
    COALESCE(
        (SELECT json_agg(tr)
         FROM tool_execution_log tr
         WHERE tr.call_id = ANY(SELECT (json_array_elements(m.tool_calls)->>'call_id')::text)
        ), '[]'
    ) as tool_execution_results
FROM ai_analysis_message m
WHERE m.session_id = 'sess_xxx'
ORDER BY m.created_at;
```

### 6.2 获取规则的下发状态详情

```sql
SELECT
    r.rule_id,
    r.title,
    r.status,
    r.dispatch_status,
    h.hostname,
    h.ip_address,
    CASE
        WHEN dispatch.detail IS NULL THEN 'pending'
        ELSE dispatch.detail
    END as host_dispatch_status
FROM sigma_rules r
CROSS JOIN LATERAL json_array_elements(r.dispatch_hosts::json) AS host_elem
LEFT JOIN hosts h ON h.id = host_elem::uuid
LEFT JOIN LATERAL (
    SELECT json_object_agg(host_id, status) as detail
    FROM agent_rule_dispatch_status
    WHERE rule_id = r.rule_id
) dispatch ON true
WHERE r.rule_id = 'xxx';
```

### 6.3 AI规则生成统计

```sql
SELECT
    DATE_TRUNC('day', created_at) as day,
    COUNT(*) as rules_generated,
    COUNT(*) FILTER (WHERE status = 'active') as rules_activated,
    COUNT(*) FILTER (WHERE ai_generated = TRUE) as ai_generated
FROM sigma_rules
WHERE created_at > NOW() - INTERVAL '30 days'
GROUP BY day
ORDER BY day;
```

---

**文档结束**
