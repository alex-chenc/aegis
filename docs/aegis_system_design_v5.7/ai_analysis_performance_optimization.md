# AI分析页面性能优化设计文档

## 1. 问题描述

AI分析页面中，时间范围和主机筛选下面的告警规则加载很慢，影响用户体验。

**用户指令**: "AI分析处：时间范围，和主机筛选，下面的告警规则都加载都很慢，排查这个慢的问题，并修改"

## 2. 问题分析

### 2.1 数据流分析

```
页面加载 -> loadHosts() + loadAlerts()
         -> GET /api/v1/hosts
         -> GET /api/v1/detection/alerts
```

用户设置筛选条件后：
```
watch([hostFilter, timeRange]) -> loadAlerts()
  -> GET /api/v1/detection/alerts?hostnames=...&start_time=...&end_time=...
```

### 2.2 性能瓶颈识别

#### 瓶颈1（高风险）：告警查询中的关联子查询

**文件**: `api-server/internal/repository/alert_repo.go:92-100`

```go
Select(`alerts.*,
    hosts.hostname,
    COALESCE(
        NULLIF(alerts.rule_title, ''),
        (SELECT title FROM sigma_rules WHERE LOWER(mitre_id) = LOWER(alerts.mitre_id) LIMIT 1),
        alerts.mitre_name
    ) as rule_title`).
```

**问题**：
- 每条告警都需要执行一次sigma_rules子查询
- 返回200条告警时，需要执行200次子查询
- `LOWER()`函数阻止索引使用

#### 瓶颈2（高风险）：缺少时间字段索引

**文件**: `migrations/002_v5_detection_tables.sql:25-28`

当前索引：
```sql
CREATE INDEX IF NOT EXISTS idx_alerts_host_id ON alerts(host_id);
CREATE INDEX IF NOT EXISTS idx_alerts_mitre_id ON alerts(mitre_id);
CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status);
CREATE INDEX IF NOT EXISTS idx_alerts_dedupe_key ON alerts(dedupe_key);
```

**缺失索引**：
- `alerts.last_seen_at` - 时间范围筛选字段
- `alerts.created_at` - 排序字段
- 复合索引 `(last_seen_at, host_id)` - 典型查询模式

#### 瓶颈3（中风险）：前端串行加载

**文件**: `frontend/src/views/detection/AIAnalysis.vue:1295-1297`

```typescript
loadHosts()
loadAlerts(Boolean(alertIdsParam))
```

`loadHosts()`和`loadAlerts()`未并行执行。

## 3. 优化方案

### 3.1 数据库索引优化

创建新的迁移文件添加索引：

```sql
-- 时间范围查询索引
CREATE INDEX IF NOT EXISTS idx_alerts_last_seen_at ON alerts(last_seen_at);

-- 排序字段索引
CREATE INDEX IF NOT EXISTS idx_alerts_created_at ON alerts(created_at DESC);

-- 复合索引：时间范围 + 主机筛选
CREATE INDEX IF NOT EXISTS idx_alerts_last_seen_at_host_id ON alerts(last_seen_at, host_id);
```

### 3.2 消除关联子查询

修改`alert_repo.go`中的`List`方法，使用LEFT JOIN替代子查询：

**方案A：使用LEFT JOIN**
```go
Select(`alerts.*,
    hosts.hostname,
    COALESCE(
        NULLIF(alerts.rule_title, ''),
        sr.title,
        alerts.mitre_name
    ) as rule_title`).
Joins("LEFT JOIN hosts ON alerts.host_id = hosts.id").
Joins("LEFT JOIN sigma_rules sr ON LOWER(sr.mitre_id) = LOWER(alerts.mitre_id)")
```

**方案B：冗余存储rule_title（推荐）**

Alert模型已有`RuleTitle`字段，在告警创建时就填充该字段，查询时直接使用，无需JOIN。

### 3.3 前端并行化

修改`AIAnalysis.vue`的`onMounted`：

```typescript
onMounted(() => {
  // ... query params处理 ...

  // 并行加载主机和告警
  Promise.all([
    loadHosts(),
    loadAlerts(Boolean(alertIdsParam))
  ]).then(() => {
    // ... 后续处理
  })
})
```

## 4. 测试用例

### 4.1 数据库索引测试

```sql
-- 验证索引创建
SELECT indexname, indexdef
FROM pg_indexes
WHERE tablename = 'alerts'
AND indexname IN ('idx_alerts_last_seen_at', 'idx_alerts_created_at', 'idx_alerts_last_seen_at_host_id');

-- 验证查询计划
EXPLAIN ANALYZE
SELECT * FROM alerts
WHERE last_seen_at >= '2026-01-01' AND last_seen_at <= '2026-12-31'
ORDER BY created_at DESC
LIMIT 200;
```

### 4.2 API性能测试

```bash
# 测试告警列表API响应时间
time curl -X GET "http://localhost:8082/api/v1/detection/alerts?page=1&pageSize=200" \
  -H "Authorization: Bearer <token>"
```

### 4.3 前端加载测试

验证`loadHosts()`和`loadAlerts()`是否并行执行。

## 5. 实现步骤

1. 创建数据库迁移文件添加索引
2. 修改`alert_repo.go`消除子查询
3. 修改`AIAnalysis.vue`实现并行加载
4. 运行测试验证
5. 更新相关文档

## 6. 风险评估

- **低风险**：添加索引、前端并行化
- **中风险**：修改查询逻辑（需充分测试）

## 7. 预期效果

- 告警查询响应时间减少50%以上
- 页面加载总时间减少30%以上
- 用户体验显著提升
