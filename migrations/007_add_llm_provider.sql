ALTER TABLE llm_configs ADD COLUMN IF NOT EXISTS provider VARCHAR(50) NOT NULL DEFAULT 'custom';

UPDATE llm_configs
SET provider = CASE
    WHEN lower(base_url) LIKE '%deepseek%' THEN 'deepseek'
    WHEN lower(base_url) LIKE '%dashscope%' OR lower(base_url) LIKE '%aliyuncs%' THEN 'dashscope'
    WHEN lower(base_url) LIKE '%minimaxi%' OR lower(base_url) LIKE '%minimax%' THEN 'minimax'
    WHEN lower(base_url) LIKE '%openai%' THEN 'openai'
    ELSE 'custom'
END
WHERE provider = 'custom' OR provider = '';
