# Aegis智能主机安全系统 V5.5 数据库结构设计文档

**版本**: 5.5
**日期**: 2026-03-30
**状态**: 定稿

---

## 1. 数据库概述

Aegis系统使用PostgreSQL作为主数据库，V5.5版本在V5.2基础上新增运行时事件存储、智能特征数据等模块，以支持微服务架构和Agent本地智能。

### V5.5 新增特性

| 模块 | 说明 |
|------|------|
| runtime_events | Agent上报的运行时事件存储 |
| event_features | 特征数据存储（压缩后） |
| block_policy_v2 | 增强版阻断策略（支持本地阻断） |
| agent_intelligence_config | Agent智能配置 |

### MITRE ID格式规范

- **统一格式**: 大写T开头，如 `T1059.004`
- **数据一致性**: 规则、阻断策略、告警三表MITRE ID格式一致
- **唯一约束**: sigma_rules表的mitre_id字段具有唯一约束

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

### 2.2 runtime_events（运行时事件表）V5.5新增

```sql
CREATE TABLE runtime_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id VARCHAR(64) UNIQUE NOT NULL,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    event_type VARCHAR(32) NOT NULL,        -- execve, connect, open, fork
    pid INTEGER,
    ppid INTEGER DEFAULT 0,
    uid INTEGER,
    process_name VARCHAR(256),
    command_line TEXT,
    parent_name VARCHAR(256),
    working_dir VARCHAR(512),
    mitre_id VARCHAR(20),
    severity VARCHAR(20) DEFAULT 'medium',
    
    -- 本地智能处理结果
    priority SMALLINT,              -- 0=CRITICAL, 1=HIGH, 2=MEDIUM, 3=LOW
    decision VARCHAR(20),           -- BLOCK, REPORT_HIGH, REPORT_FEATURE, SKIP
    is_blocked BOOLEAN DEFAULT FALSE,
    blocked_by VARCHAR(20),         -- 'local' or 'backend'
    
    -- 特征数据 (可选，用于特征上报场景)
    feature_data JSONB,
    
    -- 时间戳
    event_time TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 索引
CREATE INDEX idx_runtime_events_host_id ON runtime_events(host_id);
CREATE INDEX idx_runtime_events_event_type ON runtime_events(event_type);
CREATE INDEX idx_runtime_events_mitre_id ON runtime_events(mitre_id);
CREATE INDEX idx_runtime_events_priority ON runtime_events(priority);
CREATE INDEX idx_runtime_events_event_time ON runtime_events(event_time DESC);
CREATE INDEX idx_runtime_events_dedupe ON runtime_events(host_id, event_type, pid, mitre_id);
```

### 2.3 event_features（特征数据表）V5.5新增

```sql
-- 用于存储Agent压缩上报的特征数据
CREATE TABLE event_features (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    
    -- 基础特征
    event_type VARCHAR(32),
    process_hash VARCHAR(32),         -- 哈希后的进程名
    uid INTEGER,
    
    -- 统计特征
    frequency_score FLOAT,
    process_tree_score FLOAT,
    
    -- 上下文特征
    time_of_day SMALLINT,             -- 0-23
    is_system_process BOOLEAN,
    has_network_access BOOLEAN,
    
    -- 原始数据指纹
    command_hash VARCHAR(32),
    
    -- 批次信息
    batch_id VARCHAR(64),
    batch_time TIMESTAMP WITH TIME ZONE,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_event_features_host_id ON event_features(host_id);
CREATE INDEX idx_event_features_batch_id ON event_features(batch_id);
CREATE INDEX idx_event_features_batch_time ON event_features(batch_time DESC);
```

### 2.4 alerts（告警表）

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
CREATE INDEX idx_alerts_status_created ON alerts(status, created_at DESC);
```

### 2.5 block_policies（阻断策略表）V5.5增强

```sql
CREATE TABLE block_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mitre_id VARCHAR(20) UNIQUE NOT NULL,
    mitre_name VARCHAR(100),
    rule_title VARCHAR(255),
    severity VARCHAR(20) DEFAULT 'medium',
    auto_block BOOLEAN DEFAULT FALSE,
    enabled BOOLEAN DEFAULT TRUE,
    
    -- V5.5新增: 本地阻断配置
    local_block BOOLEAN DEFAULT FALSE,       -- 支持本地阻断
    block_method VARCHAR(20) DEFAULT 'kill_process',  -- kill_process/kill_parent/quarantine
    
    -- 统计信息
    block_count INTEGER DEFAULT 0,
    last_blocked_at TIMESTAMP WITH TIME ZONE,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_by VARCHAR(64),
    updated_by VARCHAR(64)
);

CREATE INDEX idx_block_policies_mitre_id ON block_policies(mitre_id);
CREATE INDEX idx_block_policies_enabled ON block_policies(enabled);
CREATE INDEX idx_block_policies_auto_block ON block_policies(auto_block);
```

### 2.6 block_records（阻断记录表）

```sql
CREATE TABLE block_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id UUID REFERENCES alerts(id) ON DELETE SET NULL,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    mitre_id VARCHAR(20),
    mitre_name VARCHAR(100),
    process_name VARCHAR(256),
    pid INTEGER,
    block_method VARCHAR(20),
    block_source VARCHAR(20),         -- 'manual', 'auto', 'local'
    success BOOLEAN DEFAULT TRUE,
    error_message TEXT,
    blocked_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_block_records_host_id ON block_records(host_id);
CREATE INDEX idx_block_records_alert_id ON block_records(alert_id);
CREATE INDEX idx_block_records_blocked_at ON block_records(blocked_at DESC);
```

### 2.7 sigma_rules（Sigma规则表）

```sql
CREATE TABLE sigma_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id VARCHAR(128) UNIQUE NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    mitre_id VARCHAR(20) UNIQUE NOT NULL,
    mitre_name VARCHAR(100),
    severity VARCHAR(20) DEFAULT 'medium',
    category VARCHAR(64),
    product VARCHAR(64),
    service VARCHAR(64),
    
    -- 规则内容 (YAML格式)
    rule_content TEXT,
    
    -- 状态
    enabled BOOLEAN DEFAULT TRUE,
    auto_generated BOOLEAN DEFAULT FALSE,
    
    -- 统计
    hit_count INTEGER DEFAULT 0,
    false_positive_count INTEGER DEFAULT 0,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_sigma_rules_mitre_id ON sigma_rules(mitre_id);
CREATE INDEX idx_sigma_rules_enabled ON sigma_rules(enabled);
CREATE INDEX idx_sigma_rules_severity ON sigma_rules(severity);
CREATE INDEX idx_sigma_rules_category ON sigma_rules(category);
```

### 2.8 agent_intelligence_config（Agent智能配置表）V5.5新增

```sql
CREATE TABLE agent_intelligence_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id UUID REFERENCES hosts(id) ON DELETE CASCADE,
    
    -- 滑动窗口阈值
    fork_threshold INTEGER DEFAULT 10,
    exec_threshold INTEGER DEFAULT 50,
    network_threshold INTEGER DEFAULT 20,
    file_threshold INTEGER DEFAULT 30,
    window_size INTEGER DEFAULT 5,  -- 秒
    
    -- 通信配置
    batch_size INTEGER DEFAULT 100,
    batch_timeout INTEGER DEFAULT 5,  -- 秒
    compression_enabled BOOLEAN DEFAULT TRUE,
    
    -- 阻断配置
    auto_block_enabled BOOLEAN DEFAULT TRUE,
    local_block_enabled BOOLEAN DEFAULT TRUE,
    
    -- 白名单
    whitelist JSONB DEFAULT '[]',
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_agent_intelligence_config_host_id ON agent_intelligence_config(host_id);
```

---

## 3. 微服务架构下的表访问策略

### 3.1 服务与表对应关系

| 服务 | 访问的表 | 权限 |
|------|----------|------|
| API Service | hosts, alerts, block_policies, sigma_rules, agent_intelligence_config | 读写 |
| Agent Hub | runtime_events, event_features | 读写 |
| Pipeline Service | runtime_events, alerts, block_policies, block_records | 读写 |

### 3.2 共享资源

```sql
-- 所有服务共享的表:
-- - hosts (主表)
-- - alerts
-- - block_policies
-- - sigma_rules
-- - runtime_events
-- - event_features
```

---

## 4. 分区策略

### 4.1 大表分区

对于高写入量的表，建议使用表分区:

```sql
-- runtime_events 按月分区
CREATE TABLE runtime_events_y2026m01 PARTITION OF runtime_events
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

CREATE TABLE runtime_events_y2026m02 PARTITION OF runtime_events
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');

-- event_features 按月分区
CREATE TABLE event_features_y2026m01 PARTITION OF event_features
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
```

### 4.2 数据保留策略

```sql
-- 事件数据保留30天
DELETE FROM runtime_events 
WHERE created_at < NOW() - INTERVAL '30 days';

-- 特征数据保留7天
DELETE FROM event_features 
WHERE created_at < NOW() - INTERVAL '7 days';

-- 告警数据保留90天
DELETE FROM alerts 
WHERE created_at < NOW() - INTERVAL '90 days';
```

---

## 5. 索引优化

### 5.1 常用查询索引

```sql
-- 告警查询 (按主机和时间)
CREATE INDEX idx_alerts_host_time ON alerts(host_id, created_at DESC);

-- 事件查询 (按主机、类型、时间)
CREATE INDEX idx_events_type_time ON runtime_events(event_type, event_time DESC);

-- 阻断记录查询 (按时间和类型)
CREATE INDEX idx_block_records_time ON block_records(blocked_at DESC, block_source);

-- LLM分析查询 (按状态和时间)
CREATE INDEX idx_alerts_llm_pending ON alerts(status, created_at DESC) 
WHERE status = 'pending' AND llm_summary IS NULL;
```

### 5.2 部分索引

```sql
-- 待处理的告警
CREATE INDEX idx_alerts_pending ON alerts(created_at DESC) 
WHERE status = 'pending';

-- 启用的规则
CREATE INDEX idx_rules_enabled ON sigma_rules(enabled) 
WHERE enabled = TRUE;

-- 需要LLM分析的告警
CREATE INDEX idx_alerts_need_llm ON alerts(created_at DESC) 
WHERE llm_summary IS NULL;
```

---

## 6. 性能优化配置

### 6.1 PostgreSQL配置建议

```sql
-- 内存配置
SET shared_buffers = '256MB';
SET effective_cache_size = '1GB';
SET work_mem = '64MB';

-- 并行查询
SET max_parallel_workers_per_gather = 4;

-- 写入优化
SET synchronous_commit = 'off';
SET wal_level = 'minimal';
```

---

**文档结束**