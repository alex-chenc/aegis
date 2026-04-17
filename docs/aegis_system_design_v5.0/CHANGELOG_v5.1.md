# Aegis V5.1 版本变更日志

**版本**: 5.1
**日期**: 2026-03-24
**状态**: 已完成

---

## 1. 版本概述

V5.1版本在V5.0智能异常检测基础上，进行了以下增强：
- 简化Agent事件采集（只保留进程事件）
- 优化告警列表显示（中文状态、规则名称、主机名）
- 增强AI降噪功能（手动触发、每条告警独立分析）
- 完善阻断策略（动作选择、自动处置）
- 改进安全概览（可点击告警、折线图、MITRE关联）

---

## 2. 后端变更

### 2.1 数据库变更

#### alerts表新增字段
```sql
ALTER TABLE alerts ADD COLUMN judgment_source VARCHAR(20) DEFAULT 'system';  -- system/ai
ALTER TABLE alerts ADD COLUMN block_status VARCHAR(20) DEFAULT NULL;          -- pending/blocking/success/failed
ALTER TABLE alerts ADD COLUMN block_message TEXT DEFAULT NULL;
ALTER TABLE alerts ADD COLUMN auto_dispose BOOLEAN DEFAULT FALSE;
ALTER TABLE alerts ADD COLUMN llm_disposal_strategy TEXT DEFAULT NULL;
ALTER TABLE alerts ADD COLUMN rule_id VARCHAR(128) DEFAULT NULL;
ALTER TABLE alerts ADD COLUMN rule_title VARCHAR(255) DEFAULT NULL;
```

#### 新增runtime_events表
```sql
CREATE TABLE runtime_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id VARCHAR(64) UNIQUE NOT NULL,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    event_type VARCHAR(32) NOT NULL,
    event_data JSONB NOT NULL,
    matched_rule_id VARCHAR(128),
    rule_title VARCHAR(255),
    mitre_id VARCHAR(20),
    severity VARCHAR(16),
    pid INTEGER,
    command_line TEXT,
    timestamp BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    aggregated BOOLEAN DEFAULT FALSE
);
```

#### 新增llm_aggregations表
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
```

#### block_policies表新增字段
```sql
ALTER TABLE block_policies ADD COLUMN auto_dispose BOOLEAN DEFAULT FALSE;
```

### 2.2 告警状态变更

**旧状态**: active/resolved
**新状态**: pending/resolved + block_status

状态流转：
```
告警生成 → pending
用户点击处置 → resolved
用户点击阻断 → block_status = blocking
Agent执行阻断 → block_status = success/failed
```

### 2.3 命中次数聚合逻辑变更

**旧逻辑**: host_id:pid:mitre_id
**新逻辑**: host_id:pid:rule_id

同一进程同一规则命中，hit_count+1，不增加告警条数。

### 2.4 LLM聚合功能变更

**旧逻辑**: 自动2分钟聚合
**新逻辑**: 手动按时间筛选（最大24小时）

API变更：
```
POST /api/v1/detection/llm/aggregate
Request: {
    "start_time": "2026-03-24T00:00:00Z",
    "end_time": "2026-03-24T23:59:59Z",
    "host_ids": ["host-1"],
    "auto_dispose": false
}

GET /api/v1/detection/llm/aggregate/:id
Response: {
    "aggregation_id": "AGG-xxx",
    "status": "completed",
    "event_count": 3,
    "alert_count": 3,
    "ai_judged_count": 2,
    "llm_response": "..."
}
```

LLM返回格式（每条告警独立分析）：
```json
{
  "alerts": [
    {
      "alert_id": "ALT-xxx",
      "is_threat": true,
      "llm_summary": "针对这条告警的安全分析摘要",
      "recommendation": "针对这条告警的具体处置建议"
    }
  ]
}
```

### 2.5 新增API

```
GET  /api/v1/detection/attack-matrix       # MITRE矩阵+告警统计
POST /api/v1/detection/llm/aggregate       # 启动AI降噪
GET  /api/v1/detection/llm/aggregate/:id   # 查询降噪状态
GET  /api/v1/agent/uninstall.sh            # Agent卸载脚本
```

---

## 3. 前端变更

### 3.1 安全概览页面 (Overview.vue)

**变更内容**:
- 今日告警数字可点击 → 跳转到告警列表
- 告警趋势改为ECharts折线图（小时维度）
- MITRE矩阵卡片可点击 → 跳转到告警列表并筛选mitre_id

### 3.2 告警列表页面 (Alerts.vue)

**变更内容**:
- 隐藏alert_id列，显示rule_title列
- 严重程度中文显示（严重/高危/中危/低危）
- 状态中文显示（待处置/已处置/阻断中/阻断失败/阻断成功）
- 新增判定来源列（系统判定/AI判定）
- 新增AI降噪按钮和时间选择对话框
- 阻断按钮支持选择动作（终止进程/隔离文件/阻断网络/禁用用户）

### 3.3 阻断策略页面 (Policies.vue)

**变更内容**:
- 阻断动作改为下拉选择
- 新增自动处置开关

### 3.4 规则管理页面 (Rules.vue)

**变更内容**:
- 状态中文显示（待审核/实验性/已激活/已禁用）

### 3.5 类型定义 (types/index.ts)

**新增类型**:
```typescript
export interface AttackMatrix {
  tactics: AttackTactic[]
}

export interface LLMAggregation {
  id: string
  aggregation_id: string
  start_time: string
  end_time: string
  event_count: number
  alert_count: number
  ai_judged_count: number
  status: 'pending' | 'processing' | 'completed' | 'failed'
  llm_response?: string
}

export const SeverityLabels = {
  critical: '严重',
  high: '高危',
  medium: '中危',
  low: '低危'
}

export const AlertStatusLabels = {
  pending: '待处置',
  resolved: '已处置'
}

export const BlockStatusLabels = {
  pending: '待阻断',
  blocking: '阻断中',
  success: '阻断成功',
  failed: '阻断失败'
}

export const JudgmentSourceLabels = {
  system: '系统判定',
  ai: 'AI判定'
}

export const RuleStatusLabels = {
  pending: '待审核',
  experimental: '实验性',
  active: '已激活',
  disabled: '已禁用'
}
```

---

## 4. Agent变更

### 4.1 eBPF事件采集简化

**变更内容**:
- 只保留进程事件：execve、fork、exit
- 移除文件事件：openat
- 移除网络事件：connect
- 移除权限事件：setuid、setgid、capset

**原因**:
1. 现有规则主要基于进程行为判定
2. 减少事件量，降低系统负载
3. 简化实现复杂度

### 4.2 文件内容读取工具

新增 `read_file_content` 工具：
```go
func (t *FileTools) ReadFileContent(filePath string, maxSize int64) (*FileContent, error)
```

功能：
- 读取文件内容（限制大小）
- 禁止敏感目录访问（/etc/shadow、/root/.ssh等）
- 支持文件截断标记

### 4.3 日志自动清理

使用lumberjack库配置日志轮转：
```go
lumberjackLogger := &lumberjack.Logger{
    Filename:   logPath,
    MaxSize:    10,   // 单个日志文件最大10MB
    MaxBackups: 5,    // 保留5个备份
    MaxAge:     7,    // 保留7天
    Compress:   true, // 压缩旧日志
}
```

---

## 5. 部署变更

### 5.1 Agent安装

**安装命令**:
```bash
curl -sSL http://SERVER_IP:8080/api/v1/agent/install.sh | sudo bash
```

**安装目录结构**:
```
/opt/aegis-agent/
├── aegis-agent           # Agent可执行文件
├── uninstall.sh          # 卸载脚本
├── bpf/                  # eBPF文件
│   ├── execve.bpf.o
│   ├── fork.bpf.o
│   └── exit.bpf.o
├── logs/                 # 日志目录
│   └── agent.log
├── quarantine/           # 隔离目录（权限700）
├── rules/                # 规则目录
└── config/               # 配置目录
```

**卸载命令**:
```bash
# 方式1：本地脚本
sudo /opt/aegis-agent/uninstall.sh

# 方式2：页面脚本
curl -sSL http://SERVER_IP:8080/api/v1/agent/uninstall.sh | sudo bash
```

### 5.2 数据库迁移

```bash
docker compose exec -T postgres psql -U aegis_user -d aegis_db -f - < backend/migrations/006_v5.1_enhancements.sql
```

### 5.3 Docker镜像构建

```bash
# Backend
cd backend && make build && docker build -t aegis-system/backend:latest .

# Frontend
cd frontend && npm run build && docker build -t aegis-system/frontend:latest .

# Agent
cd agent && make build && cd dist && tar -czf aegis-agent.tar.gz aegis-agent-linux-amd64 bpf/*.bpf.o
```

---

## 6. 测试验证

### 6.1 API测试

```bash
# 健康检查
curl http://localhost:8080/health

# MITRE矩阵
curl http://localhost:8080/api/v1/detection/attack-matrix

# 告警列表（带新字段）
curl "http://localhost:8080/api/v1/detection/alerts?judgment_source=ai&pageSize=5"

# AI降噪
curl -X POST http://localhost:8080/api/v1/detection/llm/aggregate \
  -H "Content-Type: application/json" \
  -d '{"start_time": "2026-03-24T00:00:00Z", "end_time": "2026-03-24T12:00:00Z"}'

# 阻断策略更新
curl -X PUT http://localhost:8080/api/v1/detection/block-policies/T1059.004 \
  -H "Content-Type: application/json" \
  -d '{"auto_dispose": true, "action": "quarantine_file"}'
```

### 6.2 Agent测试

```bash
# 检查Agent状态
sudo systemctl status aegis-agent

# 查看日志
tail -f /opt/aegis-agent/logs/agent.log

# 验证eBPF事件（只有进程事件）
sudo journalctl -u aegis-agent | grep -E "process_exec|process_fork|process_exit" | tail -10

# 卸载测试
sudo /opt/aegis-agent/uninstall.sh
```

---

## 7. 已知问题

1. **告警趋势折线图**: 如果时间范围内没有告警，折线图可能显示为空。这是正常行为。

2. **AI降噪依赖LLM**: 如果LLM服务不可用，AI降噪会失败但不会影响系统正常运行。

3. **eBPF事件类型**: Agent只采集进程事件，文件和网络事件已移除。如果规则依赖这些事件类型，需要更新规则或恢复事件采集。

---

## 8. 后续计划

1. **V5.2**: 增强LLM分析能力，支持更细粒度的威胁分类
2. **V5.3**: 添加文件事件和网络事件支持（可选启用）
3. **V6.0**: 多主机关联分析，横向移动检测
