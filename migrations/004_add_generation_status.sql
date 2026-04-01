-- ============================================================
-- 迁移: 为脚本表添加生成状态字段
-- 版本: V3.1
-- 说明: 为 poc_scripts 和 vulnerability_fix_scripts 表添加
--       generation_status 等字段，支持脚本生成过程持久化与刷新恢复
-- 可重复执行: 是（使用 IF NOT EXISTS / DO $$ 块保护）
-- ============================================================

-- ============================================================
-- 1. poc_scripts 表新增字段
-- ============================================================

ALTER TABLE poc_scripts
    ADD COLUMN IF NOT EXISTS generation_status VARCHAR(20) NOT NULL DEFAULT 'generated',
    ADD COLUMN IF NOT EXISTS generation_started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS generation_error TEXT,
    ADD COLUMN IF NOT EXISTS generation_error_detail TEXT,
    ADD COLUMN IF NOT EXISTS requested_host_id UUID;

-- 添加 generation_status 约束（如不存在则添加）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_poc_generation_status'
          AND conrelid = 'poc_scripts'::regclass
    ) THEN
        ALTER TABLE poc_scripts
            ADD CONSTRAINT chk_poc_generation_status
            CHECK (generation_status IN ('generating', 'generated', 'failed'));
    END IF;
END $$;

-- 索引
CREATE INDEX IF NOT EXISTS idx_poc_scripts_generation_status
    ON poc_scripts(generation_status);

CREATE INDEX IF NOT EXISTS idx_poc_scripts_requested_host_id
    ON poc_scripts(requested_host_id);

-- ============================================================
-- 2. vulnerability_fix_scripts 表新增字段
-- ============================================================

ALTER TABLE vulnerability_fix_scripts
    ADD COLUMN IF NOT EXISTS generation_status VARCHAR(20) NOT NULL DEFAULT 'generated',
    ADD COLUMN IF NOT EXISTS generation_started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS generation_error TEXT,
    ADD COLUMN IF NOT EXISTS generation_error_detail TEXT,
    ADD COLUMN IF NOT EXISTS requested_host_ids JSONB;

-- 添加 generation_status 约束（如不存在则添加）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_fix_script_generation_status'
          AND conrelid = 'vulnerability_fix_scripts'::regclass
    ) THEN
        ALTER TABLE vulnerability_fix_scripts
            ADD CONSTRAINT chk_fix_script_generation_status
            CHECK (generation_status IN ('generating', 'generated', 'failed'));
    END IF;
END $$;

-- 索引
CREATE INDEX IF NOT EXISTS idx_vulnerability_fix_scripts_generation_status
    ON vulnerability_fix_scripts(generation_status);
