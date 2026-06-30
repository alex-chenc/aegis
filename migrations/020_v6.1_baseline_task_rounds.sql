-- V6.1 baseline task multi-round dispatch metadata

ALTER TABLE task_logs
  ADD COLUMN IF NOT EXISTS attempt_no INT NOT NULL DEFAULT 1;

ALTER TABLE task_logs
  ADD COLUMN IF NOT EXISTS max_rounds INT NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_task_logs_rounds
  ON task_logs(task_group_id, host_id, rule_id, task_type, attempt_no);
