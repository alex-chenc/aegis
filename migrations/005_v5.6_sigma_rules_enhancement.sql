-- V5.6 Sigma Rules Table Enhancement
-- 新增字段用于支持上传Sigma规则文件解析

-- 源类型枚举: 'manual'(手动上传), 'upload'(文件上传), 'ai_generated'(AI生成), 'converted'(格式转换)
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS source VARCHAR(50) DEFAULT 'upload';

-- 文件信息
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS file_name VARCHAR(255);
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS file_hash VARCHAR(64);  -- SHA256哈希，用于去重
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS file_size INTEGER;
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS parsed_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS parse_error TEXT;

-- AI生成相关
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS ai_generated BOOLEAN DEFAULT FALSE;
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS parent_rule_id VARCHAR(100);
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS generation_prompt TEXT;
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS generation_context TEXT;

-- 审批和下发相关
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS approved_by VARCHAR(100);
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS approved_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS dispatch_hosts TEXT DEFAULT '[]';  -- 已下发的主机ID列表
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS dispatch_status VARCHAR(20) DEFAULT 'pending';  -- pending, dispatched, partial_failed

-- 添加注释
COMMENT ON COLUMN sigma_rules.source IS '规则来源: manual(手动), upload(文件上传), ai_generated(AI生成), converted(格式转换)';
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
