-- Enable pgvector extension for vector similarity search
CREATE EXTENSION IF NOT EXISTS vector;

-- AI Analysis Session table (in-memory only, not stored in DB for now)
-- AI Analysis Record table for storing analysis history with vector embeddings
CREATE TABLE IF NOT EXISTS ai_analysis_record (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id VARCHAR(100) UNIQUE NOT NULL,
    user_id VARCHAR(100),
    alert_ids JSONB NOT NULL DEFAULT '[]',
    host_filter JSONB DEFAULT '[]',
    time_range JSONB,
    initial_query TEXT,
    final_conclusion JSONB,
    summary TEXT,
    summary_vector VECTOR(1536),
    created_by VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for ai_analysis_record
CREATE INDEX IF NOT EXISTS idx_analysis_record_alerts ON ai_analysis_record USING gin(alert_ids);
CREATE INDEX IF NOT EXISTS idx_analysis_record_hosts ON ai_analysis_record USING gin(host_filter);
CREATE INDEX IF NOT EXISTS idx_analysis_record_session ON ai_analysis_record(session_id);
CREATE INDEX IF NOT EXISTS idx_analysis_record_vector ON ai_analysis_record USING ivfflat(summary_vector vector_cosine_ops);

-- AI Analysis Message table for storing message history
CREATE TABLE IF NOT EXISTS ai_analysis_message (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id VARCHAR(100) NOT NULL,
    role VARCHAR(20) NOT NULL, -- 'user' or 'assistant'
    content TEXT,
    steps JSONB,
    call_id VARCHAR(100),
    tool_name VARCHAR(50),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_analysis_message_session ON ai_analysis_message(session_id);
CREATE INDEX IF NOT EXISTS idx_analysis_message_created ON ai_analysis_message(created_at);
