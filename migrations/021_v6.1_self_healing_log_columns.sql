-- V6.1 self-healing log compatibility columns

ALTER TABLE self_healing_logs
  ADD COLUMN IF NOT EXISTS final_script_version_id UUID;

ALTER TABLE self_healing_logs
  ADD COLUMN IF NOT EXISTS user_suggestion TEXT;

ALTER TABLE self_healing_logs
  ADD COLUMN IF NOT EXISTS last_error TEXT;

CREATE INDEX IF NOT EXISTS idx_healing_logs_final_script_version
  ON self_healing_logs(final_script_version_id);
