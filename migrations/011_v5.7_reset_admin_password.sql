-- ============================================================
-- Aegis V5.7 - 重置管理员密码 & 添加密码重置密钥
-- ============================================================

-- 重置管理员密码为 Admin@123
UPDATE auth_users
SET password_hash = '$2a$10$Kv4gIwyjYuP7tbqJR1ApceeBKySPq/qWpDglRNv8PdIETwirzkM6u',
    force_password_change = false,
    updated_at = NOW()
WHERE username = 'admin';

-- 添加密码重置密钥配置
INSERT INTO system_configs (config_key, config_value, description, category)
VALUES (
    'password_reset_key',
    '"a2201db8365505113eded4555f0e29acd72c8c23ccfc28cf654674dc18e9ed71"',
    '管理员密码重置密钥',
    'auth'
)
ON CONFLICT (config_key) DO UPDATE
SET config_value = '"a2201db8365505113eded4555f0e29acd72c8c23ccfc28cf654674dc18e9ed71"',
    updated_at = NOW();
