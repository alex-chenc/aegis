-- Persist complete AI analysis ReAct traces for history playback.

CREATE TABLE IF NOT EXISTS ai_analysis_session (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id VARCHAR(100) UNIQUE NOT NULL,
    alert_ids JSONB DEFAULT '[]',
    host_ids JSONB DEFAULT '[]',
    host_filter JSONB DEFAULT '[]',
    time_range JSONB,
    status VARCHAR(20) DEFAULT 'active',
    max_iterations INTEGER DEFAULT 15,
    message_count INTEGER DEFAULT 0,
    tool_call_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    concluded_at TIMESTAMP WITH TIME ZONE,
    conclusion JSONB
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_analysis_session_session_id
    ON ai_analysis_session(session_id);

CREATE TABLE IF NOT EXISTS ai_analysis_message (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id VARCHAR(100) NOT NULL,
    message_id VARCHAR(100) UNIQUE NOT NULL DEFAULT gen_random_uuid()::text,
    role VARCHAR(20) NOT NULL,
    content TEXT,
    thinking TEXT,
    tool_calls JSONB,
    tool_results JSONB,
    steps JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE ai_analysis_message
    ADD COLUMN IF NOT EXISTS message_id VARCHAR(100),
    ADD COLUMN IF NOT EXISTS thinking TEXT,
    ADD COLUMN IF NOT EXISTS tool_calls JSONB,
    ADD COLUMN IF NOT EXISTS tool_results JSONB;

UPDATE ai_analysis_message
SET message_id = gen_random_uuid()::text
WHERE message_id IS NULL OR message_id = '';

ALTER TABLE ai_analysis_message
    ALTER COLUMN message_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_analysis_message_message_id
    ON ai_analysis_message(message_id);
CREATE INDEX IF NOT EXISTS idx_analysis_message_session
    ON ai_analysis_message(session_id);
CREATE INDEX IF NOT EXISTS idx_analysis_message_created
    ON ai_analysis_message(created_at);
