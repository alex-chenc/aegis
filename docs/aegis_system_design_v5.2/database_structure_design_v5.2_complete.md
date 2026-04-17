# 数据库结构设计文档 - V5.2 完整版

**数据库**: PostgreSQL 15+
**版本**: 5.2
**状态**: 定稿
**日期**: 2026-03-26

---

## 1. 数据库概述

Aegis系统使用PostgreSQL作为主数据库，主要存储以下数据：
- 主机信息
- 告警数据
- 阻断策略
- 阻断记录
- Sigma规则
- 运行时事件
- LLM聚合记录
- 规则调整历史

### MITRE ID格式规范

- **统一格式**: 大写T开头，如 `T1059.004`
- **数据一致性**: 规则、阻断策略、告警三表MITRE ID格式一致
- **唯一约束**: sigma_rules表的mitre_id字段具有唯一约束
- **关联关系**: 阻断策略通过mitre_id与规则1对1关联

---

## 2. 表结构定义

### 2.1 hosts（主机表）

```sql
CREATE TABLE hosts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ip_address VARCHAR(45) NOT NULL,
    hostname VARCHAR(128),
    os_type VARCHAR(64),
    agent_version VARCHAR(32),
    last_heartbeat_at TIMESTAMP WITH TIME ZONE,
    online BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_hosts_ip ON hosts(ip_address);
CREATE INDEX idx_hosts_online ON hosts(online);
```

### 2.2 alerts（告警表）

```sql
CREATE TABLE alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id VARCHAR(64) UNIQUE NOT NULL,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    hostname VARCHAR(128),
    pid INTEGER,
    ppid INTEGER DEFAULT 0,
    command_line TEXT,
    process_tree JSONB,
    mitre_id VARCHAR(20),  -- 大写T格式
    mitre_name VARCHAR(100),
    severity VARCHAR(20) DEFAULT 'medium',
    description TEXT,
    llm_summary TEXT,
    llm_disposal_strategy TEXT,
    dedupe_key VARCHAR(256) UNIQUE NOT NULL,
    hit_count INTEGER DEFAULT 1,
    status VARCHAR(20) DEFAULT 'pending',
    judgment_source VARCHAR(20) DEFAULT 'system',
    block_status VARCHAR(20),
    block_message TEXT,
    auto_blocked BOOLEAN DEFAULT FALSE,
    manual_blocked BOOLEAN DEFAULT FALSE,
    auto_dispose BOOLEAN DEFAULT FALSE,
    rule_id VARCHAR(128),
    rule_title VARCHAR(255),
    first_seen_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_seen_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_alerts_host_id ON alerts(host_id);
CREATE INDEX idx_alerts_mitre_id ON alerts(mitre_id);
CREATE INDEX idx_alerts_severity ON alerts(severity);
CREATE INDEX idx_alerts_status ON alerts(status);
CREATE INDEX idx_alerts_created_at ON alerts(created_at);
CREATE INDEX idx_alerts_dedupe_key ON alerts(dedupe_key);
CREATE INDEX idx_alerts_rule_id ON alerts(rule_id);

-- 复合索引
CREATE INDEX idx_alerts_status_created ON alerts(status, created_at DESC);
```

### 2.3 block_policies（阻断策略表）

```sql
CREATE TABLE block_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mitre_id VARCHAR(20) UNIQUE NOT NULL,  -- 大写T格式，唯一约束
    mitre_name VARCHAR(100),  -- 规则标题
    enabled BOOLEAN DEFAULT TRUE,
    auto_block BOOLEAN DEFAULT FALSE,
    auto_dispose BOOLEAN DEFAULT FALSE,
    action VARCHAR(50) DEFAULT 'kill_process',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_block_policies_mitre_id ON block_policies(mitre_id);
CREATE INDEX idx_block_policies_enabled ON block_policies(enabled);
```

**默认策略数据**（33条）：

| mitre_id | mitre_name | enabled | auto_block | auto_dispose | action |
|----------|------------|---------|------------|--------------|--------|
| T1003 | OS Credential Dumping | true | false | true | quarantine_file |
| T1003.001 | LSASS Memory Dump | true | false | false | kill_process |
| T1003.008 | /etc/passwd and /etc/shadow Access | true | false | false | kill_process |
| T1059.004 | Unix Shell | true | false | false | kill_process |
| T1113 | Screen Capture | true | false | false | kill_process |
| ... | ... | ... | ... | ... | ... |

**关联规则**:
- 阻断策略的mitre_id与sigma_rules的mitre_id为1对1关系
- 删除规则时级联删除对应阻断策略

### 2.4 block_records（阻断记录表）

```sql
CREATE TABLE block_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    block_id VARCHAR(32) UNIQUE NOT NULL,
    alert_id UUID REFERENCES alerts(id) ON DELETE SET NULL,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    action VARCHAR(32) NOT NULL,
    target VARCHAR(256),
    status VARCHAR(20) DEFAULT 'pending',
    message TEXT,
    issued_by VARCHAR(32) DEFAULT 'manual',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    executed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_block_records_host_id ON block_records(host_id);
CREATE INDEX idx_block_records_status ON block_records(status);
CREATE INDEX idx_block_records_created_at ON block_records(created_at);
```

### 2.5 sigma_rules（Sigma规则表）

```sql
CREATE TABLE sigma_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id VARCHAR(128) UNIQUE NOT NULL,
    title VARCHAR(255),
    description TEXT,
    content TEXT NOT NULL,
    status VARCHAR(20) DEFAULT 'pending',  -- pending/experimental/active/disabled
    mitre_id VARCHAR(20) UNIQUE,  -- 大写T格式，唯一约束
    severity VARCHAR(20),
    generated_by VARCHAR(20),  -- import/llm
    version VARCHAR(10) DEFAULT '1.0',
    activated_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_sigma_rules_mitre_id ON sigma_rules(mitre_id);
CREATE INDEX idx_sigma_rules_status ON sigma_rules(status);
CREATE INDEX idx_sigma_rules_rule_id ON sigma_rules(rule_id);

-- 唯一约束：MITRE ID不能重复
ALTER TABLE sigma_rules ADD CONSTRAINT sigma_rules_mitre_id_unique UNIQUE (mitre_id);
```

**约束说明**:
- `mitre_id`字段具有唯一约束，不允许重复
- 创建规则时需要检查MITRE ID是否已存在
- 批量导入时跳过重复的MITRE ID

### 2.6 runtime_events（运行时事件表）

```sql
CREATE TABLE runtime_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id VARCHAR(64) UNIQUE NOT NULL,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    event_type VARCHAR(32) NOT NULL,
    event_data JSONB NOT NULL,
    matched_rule_id VARCHAR(128),
    rule_title VARCHAR(255),
    mitre_id VARCHAR(20),  -- 大写T格式
    severity VARCHAR(16),
    pid INTEGER,
    command_line TEXT,
    timestamp BIGINT NOT NULL,
    aggregated BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_runtime_events_host_id ON runtime_events(host_id);
CREATE INDEX idx_runtime_events_timestamp ON runtime_events(timestamp);
CREATE INDEX idx_runtime_events_aggregated ON runtime_events(aggregated);
CREATE INDEX idx_runtime_events_mitre_id ON runtime_events(mitre_id);

-- 复合索引：聚合查询
CREATE INDEX idx_runtime_events_agg_time ON runtime_events(aggregated, timestamp);
```

### 2.7 llm_aggregations（LLM聚合表）

```sql
CREATE TABLE llm_aggregations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregation_id VARCHAR(64) UNIQUE NOT NULL,
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE NOT NULL,
    host_ids TEXT[],
    event_count INTEGER DEFAULT 0,
    alert_count INTEGER DEFAULT 0,
    ai_judged_count INTEGER DEFAULT 0,
    auto_dispose_count INTEGER DEFAULT 0,
    llm_response TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    error TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_llm_aggregations_status ON llm_aggregations(status);
CREATE INDEX idx_llm_aggregations_created_at ON llm_aggregations(created_at);
```

### 2.8 tool_calls（工具调用表）

```sql
CREATE TABLE tool_calls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id VARCHAR(64) UNIQUE NOT NULL,
    alert_id UUID REFERENCES alerts(id) ON DELETE CASCADE,
    tool_name VARCHAR(64) NOT NULL,
    arguments JSONB,
    result JSONB,
    status VARCHAR(20) DEFAULT 'pending',
    error TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_tool_calls_alert_id ON tool_calls(alert_id);
CREATE INDEX idx_tool_calls_status ON tool_calls(status);
```

### 2.9 rule_adjustment_histories（规则调整历史表）

```sql
CREATE TABLE rule_adjustment_histories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id VARCHAR(128) NOT NULL,
    trigger_count INTEGER NOT NULL,
    time_window VARCHAR(10),  -- 10m/30m/60m
    is_false_positive BOOLEAN NOT NULL,
    llm_reason TEXT,
    old_content TEXT,
    new_content TEXT,
    applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_rule_adjustment_rule_id ON rule_adjustment_histories(rule_id);
CREATE INDEX idx_rule_adjustment_applied_at ON rule_adjustment_histories(applied_at);
```

**用途**:
- 记录智能误报检测服务的规则调整历史
- 记录每次触发的规则数量、时间窗口
- 记录LLM的判断原因和规则修改内容

---

## 3. 索引策略

### 3.1 主要查询路径

| 查询场景 | 涉及表 | 使用索引 |
|----------|--------|----------|
| 告警列表分页 | alerts | idx_alerts_created_at |
| 按MITRE筛选告警 | alerts | idx_alerts_mitre_id |
| 按状态筛选告警 | alerts | idx_alerts_status |
| 告警去重检查 | alerts | idx_alerts_dedupe_key |
| 按规则ID查询告警 | alerts | idx_alerts_rule_id |
| AI降噪事件查询 | runtime_events | idx_runtime_events_agg_time |
| 阻断策略查询 | block_policies | idx_block_policies_mitre_id |
| 规则状态筛选 | sigma_rules | idx_sigma_rules_status |
| 规则MITRE唯一检查 | sigma_rules | sigma_rules_mitre_id_unique |

### 3.2 复合索引

```sql
-- 告警状态+时间复合索引
CREATE INDEX idx_alerts_status_created ON alerts(status, created_at DESC);

-- 运行时事件聚合查询复合索引
CREATE INDEX idx_runtime_events_agg_time ON runtime_events(aggregated, timestamp);
```

---

## 4. 数据关系图

```
┌─────────────┐     ┌─────────────┐     ┌─────────────────┐
│   hosts     │────<│   alerts    │────<│  block_records  │
└─────────────┘     └─────────────┘     └─────────────────┘
       │                   │
       │                   │
       ▼                   ▼
┌─────────────────┐ ┌─────────────┐
│ runtime_events  │ │ tool_calls  │
└─────────────────┘ └─────────────┘
       │
       ▼
┌──────────────────┐
│ llm_aggregations │
└──────────────────┘

┌─────────────────────┐
│    block_policies   │←── mitre_id ──→ sigma_rules.mitre_id (1对1)
└─────────────────────┘

┌─────────────┐
│ sigma_rules │─── mitre_id (UNIQUE)
└─────────────┘
       ↑
       │ mitre_id
       │
┌─────────────────────────────┐
│ rule_adjustment_histories   │
└─────────────────────────────┘
```

### 关联关系说明

| 关系 | 说明 |
|------|------|
| hosts ← alerts | 一对多，通过host_id关联 |
| alerts ← block_records | 一对多，通过alert_id关联 |
| sigma_rules.mitre_id ↔ block_policies.mitre_id | 一对一，通过mitre_id关联 |
| sigma_rules.mitre_id ↔ alerts.mitre_id | 一对多，同一规则可能产生多条告警 |
| sigma_rules.rule_id → rule_adjustment_histories.rule_id | 一对多，记录规则调整历史 |

---

## 5. 数据清理策略

### 5.1 自动清理

```sql
-- 清理已聚合的运行时事件（保留7天）
DELETE FROM runtime_events 
WHERE aggregated = true 
AND created_at < NOW() - INTERVAL '7 days';

-- 清理已完成的LLM聚合记录（保留30天）
DELETE FROM llm_aggregations 
WHERE status = 'completed' 
AND created_at < NOW() - INTERVAL '30 days';

-- 清理规则调整历史（保留90天）
DELETE FROM rule_adjustment_histories 
WHERE applied_at < NOW() - INTERVAL '90 days';
```

### 5.2 告警保留策略

| 状态 | 保留时间 |
|------|----------|
| pending | 永久保留 |
| resolved | 保留90天 |
| auto_dispose=true | 保留30天 |

---

## 6. 迁移脚本

### 6.1 V5.2迁移

```sql
-- 添加MITRE中文映射字段（如果不存在）
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'alerts' AND column_name = 'mitre_name') THEN
        ALTER TABLE alerts ADD COLUMN mitre_name VARCHAR(100);
    END IF;
END $$;

-- 添加description字段（如果不存在）
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'alerts' AND column_name = 'description') THEN
        ALTER TABLE alerts ADD COLUMN description TEXT;
    END IF;
END $$;

-- 添加auto_dispose字段（如果不存在）
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'alerts' AND column_name = 'auto_dispose') THEN
        ALTER TABLE alerts ADD COLUMN auto_dispose BOOLEAN DEFAULT FALSE;
    END IF;
END $$;

-- 添加auto_dispose字段到block_policies（如果不存在）
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'block_policies' AND column_name = 'auto_dispose') THEN
        ALTER TABLE block_policies ADD COLUMN auto_dispose BOOLEAN DEFAULT FALSE;
    END IF;
END $$;

-- 添加rule_id字段到alerts（如果不存在）
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'alerts' AND column_name = 'rule_id') THEN
        ALTER TABLE alerts ADD COLUMN rule_id VARCHAR(128);
    END IF;
END $$;

-- 添加generated_by字段到sigma_rules（如果不存在）
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sigma_rules' AND column_name = 'generated_by') THEN
        ALTER TABLE sigma_rules ADD COLUMN generated_by VARCHAR(20);
    END IF;
END $$;

-- 添加activated_at字段到sigma_rules（如果不存在）
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sigma_rules' AND column_name = 'activated_at') THEN
        ALTER TABLE sigma_rules ADD COLUMN activated_at TIMESTAMP WITH TIME ZONE;
    END IF;
END $$;

-- 添加MITRE ID唯一约束到sigma_rules（如果不存在）
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'sigma_rules_mitre_id_unique') THEN
        -- 先删除重复的MITRE ID
        DELETE FROM sigma_rules WHERE id NOT IN (
            SELECT MIN(id) FROM sigma_rules WHERE mitre_id IS NOT NULL GROUP BY mitre_id
        );
        -- 添加唯一约束
        ALTER TABLE sigma_rules ADD CONSTRAINT sigma_rules_mitre_id_unique UNIQUE (mitre_id);
    END IF;
END $$;

-- 创建规则调整历史表（如果不存在）
CREATE TABLE IF NOT EXISTS rule_adjustment_histories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id VARCHAR(128) NOT NULL,
    trigger_count INTEGER NOT NULL,
    time_window VARCHAR(10),
    is_false_positive BOOLEAN NOT NULL,
    llm_reason TEXT,
    old_content TEXT,
    new_content TEXT,
    applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rule_adjustment_rule_id ON rule_adjustment_histories(rule_id);
CREATE INDEX IF NOT EXISTS idx_rule_adjustment_applied_at ON rule_adjustment_histories(applied_at);

-- 统一MITRE ID为大写T格式
UPDATE sigma_rules SET mitre_id = UPPER(mitre_id) WHERE mitre_id ~ '^t[0-9]';
UPDATE sigma_rules SET mitre_id = 'T' || mitre_id WHERE mitre_id ~ '^[0-9]';
UPDATE block_policies SET mitre_id = UPPER(mitre_id) WHERE mitre_id ~ '^t[0-9]';
UPDATE block_policies SET mitre_id = 'T' || mitre_id WHERE mitre_id ~ '^[0-9]';
UPDATE alerts SET mitre_id = UPPER(mitre_id) WHERE mitre_id ~ '^t[0-9]';
UPDATE alerts SET mitre_id = 'T' || mitre_id WHERE mitre_id ~ '^[0-9]';

-- 删除小写t开头的重复阻断策略
DELETE FROM block_policies WHERE mitre_id ~ '^t[0-9]';

-- 同步阻断策略名称与规则标题一致
UPDATE block_policies bp
SET mitre_name = sr.title
FROM sigma_rules sr
WHERE bp.mitre_id = sr.mitre_id
AND bp.mitre_name != sr.title;

-- 添加索引（如果不存在）
CREATE INDEX IF NOT EXISTS idx_alerts_rule_id ON alerts(rule_id);
CREATE INDEX IF NOT EXISTS idx_alerts_status_created ON alerts(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_runtime_events_agg_time ON runtime_events(aggregated, timestamp);
```

---

## 7. 查询示例

### 7.1 告警列表查询

```sql
SELECT 
    a.id, a.alert_id, a.hostname, a.mitre_id, a.mitre_name,
    a.severity, a.status, a.hit_count, a.auto_dispose,
    a.rule_id, a.rule_title,
    a.first_seen_at, a.last_seen_at
FROM alerts a
WHERE a.status = 'pending'
ORDER BY a.created_at DESC
LIMIT 10 OFFSET 0;
```

### 7.2 按时间范围查询告警（AI降噪）

```sql
SELECT 
    id, alert_id, mitre_id, status, hit_count
FROM alerts
WHERE status = 'pending'
AND created_at >= $1
AND created_at <= $2
AND (array_length($3, 1) IS NULL OR host_id = ANY($3))
ORDER BY created_at DESC;
```

### 7.3 阻断策略分页查询（带规则标题）

```sql
SELECT 
    bp.id, bp.mitre_id, bp.mitre_name, bp.enabled,
    bp.auto_block, bp.auto_dispose, bp.action,
    (SELECT title FROM sigma_rules WHERE mitre_id = bp.mitre_id LIMIT 1) as rule_title
FROM block_policies bp
WHERE bp.mitre_id ILIKE '%T105%' OR bp.mitre_name ILIKE '%Shell%'
ORDER BY bp.mitre_id
LIMIT 10 OFFSET 0;
```

### 7.4 规则触发统计（智能误报检测）

```sql
SELECT 
    rule_id, mitre_id, COUNT(*) as alert_count
FROM alerts
WHERE created_at >= $1
AND created_at <= $2
AND rule_id IS NOT NULL AND rule_id != ''
GROUP BY rule_id, mitre_id
HAVING COUNT(*) >= $3
ORDER BY alert_count DESC;
```

### 7.5 检查MITRE ID是否存在

```sql
SELECT COUNT(*) > 0 as exists
FROM sigma_rules
WHERE mitre_id = $1;
```

### 7.6 MITRE告警统计

```sql
SELECT 
    mitre_id, 
    COUNT(*) as alert_count
FROM alerts
WHERE created_at >= NOW() - INTERVAL '24 hours'
GROUP BY mitre_id
ORDER BY alert_count DESC;
```

### 7.7 规则与阻断策略对应检查

```sql
-- 查找规则有但阻断策略没有的MITRE ID
SELECT DISTINCT sr.mitre_id 
FROM sigma_rules sr 
WHERE sr.mitre_id IS NOT NULL AND sr.mitre_id != '' 
AND sr.mitre_id NOT IN (SELECT mitre_id FROM block_policies);

-- 查找阻断策略有但规则没有的MITRE ID
SELECT DISTINCT bp.mitre_id 
FROM block_policies bp 
WHERE bp.mitre_id NOT IN (
    SELECT mitre_id FROM sigma_rules 
    WHERE mitre_id IS NOT NULL AND mitre_id != ''
);
```

---

**文档结束**