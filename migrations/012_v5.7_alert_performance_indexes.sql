-- Migration: 012_v5.7_alert_performance_indexes.sql
-- Description: 添加alerts表性能优化索引
-- Date: 2026-05-12

-- 时间范围查询索引
CREATE INDEX IF NOT EXISTS idx_alerts_last_seen_at ON alerts(last_seen_at);

-- 排序字段索引
CREATE INDEX IF NOT EXISTS idx_alerts_created_at ON alerts(created_at DESC);

-- 复合索引：时间范围 + 主机筛选（AI分析页面典型查询模式）
CREATE INDEX IF NOT EXISTS idx_alerts_last_seen_at_host_id ON alerts(last_seen_at, host_id);

-- sigma_rules表mitre_id索引优化（支持LOWER函数查询）
CREATE INDEX IF NOT EXISTS idx_sigma_rules_mitre_id_lower ON sigma_rules(LOWER(mitre_id));
