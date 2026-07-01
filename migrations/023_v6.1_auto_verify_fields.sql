-- V6.1: Add auto-verify fields to task_logs
-- Supports automatic detection-repair-verification loop

ALTER TABLE task_logs ADD COLUMN IF NOT EXISTS auto_verify BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE task_logs ADD COLUMN IF NOT EXISTS verify_round INTEGER NOT NULL DEFAULT 0;

-- Index for finding auto-verify tasks that need follow-up
CREATE INDEX IF NOT EXISTS idx_task_logs_auto_verify ON task_logs(auto_verify) WHERE auto_verify = true;
