-- V6.1 baseline task execution/large-model repair semantics

ALTER TABLE task_logs
  ADD COLUMN IF NOT EXISTS attempt_no INTEGER NOT NULL DEFAULT 1;

ALTER TABLE task_logs
  ADD COLUMN IF NOT EXISTS max_rounds INTEGER NOT NULL DEFAULT 3;

CREATE INDEX IF NOT EXISTS idx_task_logs_attempt_rounds
  ON task_logs(attempt_no, max_rounds);
