# Code Review: AI分析页面性能优化

**Reviewed**: 2026-05-12
**Branch**: develop
**Decision**: APPROVE

## Summary

本次修改旨在优化AI分析页面的告警规则加载性能，通过消除数据库子查询、添加索引和前端并行加载来提升用户体验。

## Findings

### CRITICAL
None

### HIGH
None

### MEDIUM
None

### LOW
None

## Validation Results

| Check | Result |
|---|---|
| Go vet | Pass |
| Docker build (api-server) | Pass |
| Docker build (frontend) | Pass |
| API test | Pass |

## Files Reviewed

| File | Change Type | Description |
|---|---|---|
| api-server/internal/repository/alert_repo.go | Modified | 消除子查询，改用LEFT JOIN |
| frontend/src/views/detection/AIAnalysis.vue | Modified | 实现loadHosts和loadAlerts并行加载 |
| migrations/012_v5.7_alert_performance_indexes.sql | Added | 添加性能优化索引 |
| docs/aegis_system_design_v5.7/ai_analysis_performance_optimization.md | Added | 性能优化设计文档 |

## Change Details

### 1. alert_repo.go - 消除子查询

**Before:**
```go
Select(`alerts.*,
    hosts.hostname,
    COALESCE(
        NULLIF(alerts.rule_title, ''),
        (SELECT title FROM sigma_rules WHERE LOWER(mitre_id) = LOWER(alerts.mitre_id) LIMIT 1),
        alerts.mitre_name
    ) as rule_title`).
Joins("LEFT JOIN hosts ON alerts.host_id = hosts.id")
```

**After:**
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

**Impact**: 将N+1子查询改为单次LEFT JOIN，显著减少数据库查询次数。

### 2. AIAnalysis.vue - 并行加载

**Before:**
```typescript
loadHosts()
loadAlerts(Boolean(alertIdsParam)).then(() => {
```

**After:**
```typescript
Promise.all([
  loadHosts(),
  loadAlerts(Boolean(alertIdsParam))
]).then(() => {
```

**Impact**: 主机列表和告警列表并行加载，减少总等待时间。

### 3. 数据库索引

新增索引：
- `idx_alerts_last_seen_at` - 时间范围查询
- `idx_alerts_created_at` - 排序优化
- `idx_alerts_last_seen_at_host_id` - 复合查询
- `idx_sigma_rules_mitre_id_lower` - LOWER函数查询优化

**Impact**: 避免全表扫描，提升查询性能。

## Performance Improvement

预期性能提升：
- 告警查询响应时间减少50%以上
- 页面加载总时间减少30%以上
