-- V6.1 follow-up: one active recovery decision per run/step/tool/error.
-- Preserve the earliest request as the user-facing decision and expire later
-- retry artifacts without deleting their audit records.
WITH ranked_active_recoveries AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY
                run_id,
                COALESCE(NULLIF(step_id, ''), tool_name),
                tool_name,
                code
            ORDER BY
                CASE status
                    WHEN 'executing' THEN 0
                    WHEN 'paused' THEN 1
                    ELSE 2
                END,
                created_at ASC,
                id ASC
        ) AS duplicate_rank
    FROM assistant_recovery_requests
    WHERE status IN ('pending', 'executing', 'paused')
)
UPDATE assistant_recovery_requests AS recovery
SET
    status = 'expired',
    updated_at = NOW()
FROM ranked_active_recoveries AS ranked
WHERE recovery.id = ranked.id
  AND ranked.duplicate_rank > 1;

CREATE UNIQUE INDEX IF NOT EXISTS uq_assistant_recovery_active_run_step_tool_code
    ON assistant_recovery_requests(
        run_id,
        COALESCE(NULLIF(step_id, ''), tool_name),
        tool_name,
        code
    )
    WHERE status IN ('pending', 'executing', 'paused');
