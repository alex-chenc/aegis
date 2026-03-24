-- V5.1 Enhancement Migration

-- Alert status change: 'active' -> 'pending', keep 'resolved'
-- New fields for AI judgment, block status, auto disposal

ALTER TABLE alerts ADD COLUMN IF NOT EXISTS judgment_source VARCHAR(20) DEFAULT 'system';
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS block_status VARCHAR(20) DEFAULT NULL;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS block_message TEXT DEFAULT NULL;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS auto_dispose BOOLEAN DEFAULT FALSE;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS llm_disposal_strategy TEXT DEFAULT NULL;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS rule_id VARCHAR(128) DEFAULT NULL;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS rule_title VARCHAR(255) DEFAULT NULL;

-- Migrate existing alerts: 'active' -> 'pending', and set block_status for blocked ones
UPDATE alerts SET status = 'pending' WHERE status = 'active';
UPDATE alerts SET block_status = 'success' WHERE auto_blocked = TRUE OR manual_blocked = TRUE;
UPDATE alerts SET status = 'resolved' WHERE block_status = 'success';

-- Add indexes for new fields
CREATE INDEX IF NOT EXISTS idx_alerts_judgment_source ON alerts(judgment_source);
CREATE INDEX IF NOT EXISTS idx_alerts_block_status ON alerts(block_status);
CREATE INDEX IF NOT EXISTS idx_alerts_rule_id ON alerts(rule_id);

-- Runtime events table for manual LLM aggregation
CREATE TABLE IF NOT EXISTS runtime_events (
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

CREATE INDEX IF NOT EXISTS idx_runtime_events_host_time ON runtime_events(host_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_runtime_events_aggregated ON runtime_events(aggregated);
CREATE INDEX IF NOT EXISTS idx_runtime_events_type ON runtime_events(event_type);

-- LLM aggregation tracking table
CREATE TABLE IF NOT EXISTS llm_aggregations (
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

CREATE INDEX IF NOT EXISTS idx_llm_aggregations_status ON llm_aggregations(status);
CREATE INDEX IF NOT EXISTS idx_llm_aggregations_time ON llm_aggregations(start_time, end_time);

-- Update block_policies: add auto_dispose field
ALTER TABLE block_policies ADD COLUMN IF NOT EXISTS auto_dispose BOOLEAN DEFAULT FALSE;

-- Update sigma_rules: add rule_id as foreign key reference
ALTER TABLE sigma_rules ADD CONSTRAINT IF NOT EXISTS fk_sigma_rules_rule_id UNIQUE (rule_id);

-- Comments for documentation
COMMENT ON COLUMN alerts.judgment_source IS 'system or ai - indicates who made the judgment';
COMMENT ON COLUMN alerts.block_status IS 'pending, blocking, success, failed - block execution status';
COMMENT ON COLUMN alerts.block_message IS 'Error message when block_status is failed';
COMMENT ON COLUMN alerts.auto_dispose IS 'Whether auto disposal was applied';
COMMENT ON COLUMN alerts.llm_disposal_strategy IS 'LLM recommended disposal strategy';
COMMENT ON COLUMN alerts.rule_id IS 'Reference to sigma_rules.rule_id';
COMMENT ON COLUMN alerts.rule_title IS 'Rule title for display (denormalized)';
COMMENT ON COLUMN runtime_events.event_data IS 'Full event JSON from agent';
COMMENT ON COLUMN llm_aggregations.status IS 'pending, processing, completed, failed';