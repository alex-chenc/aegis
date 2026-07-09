-- Preserve transport status separately from the business-operation outcome.
-- A successful tool RPC may only enqueue an asynchronous operation; it must not
-- be displayed or audited as terminal business success until status evidence
-- confirms completion.
ALTER TABLE assistant_tool_calls
    ADD COLUMN IF NOT EXISTS operation_status VARCHAR(24),
    ADD COLUMN IF NOT EXISTS operation_terminal BOOLEAN,
    ADD COLUMN IF NOT EXISTS outcome JSONB;

CREATE INDEX IF NOT EXISTS idx_assistant_tool_calls_operation_status
    ON assistant_tool_calls(operation_status);
