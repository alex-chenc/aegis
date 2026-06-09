# Assistant Approval, Upload Context, and Baseline Flow Fix

## Bug Description

The assistant request-approval mode can create approval records without rendering an actionable approval card in the conversation. The agent runtime treats approval-required tool calls as normal tool failures, so the model may continue producing responses while tools are still waiting for user approval.

Uploaded files are also shown primarily in the right context rail, while the composer does not show an upload chip/progress state. For uploaded baseline templates, the runtime context only receives the context title and summary; structured snapshot fields such as `template_id`, `filename`, and parse status are not provided to the model, which can lead to responses claiming that no file is visible.

## Reproduction Steps

1. Open the assistant page on the normal business frontend port `8081`.
2. Set tool approval mode to `request_approval`.
3. Ask the assistant to call a tool such as asset collection or baseline task dispatch.
4. Observe that the conversation can continue even though the tool call is waiting for approval, and the approval button may not be visible in the conversation stream.
5. Upload a baseline PDF and ask the assistant to parse/generate scripts. The model may not identify the uploaded file because the structured context is missing from the runtime prompt.

## Root Cause

- `AssistantToolGatewayAdapter` returns `ToolCallFailed` with `approval_required:<id>` as a regular observation, so `agent-runtime` can continue ReAct turns.
- The frontend stores approvals in a global list but does not consistently attach `approval_required` events to the active assistant message.
- The upload path persists rich file data in `AssistantContextRef.Snapshot`, but `ContextLoader.ResolveSession` and runtime prompt construction drop that snapshot data.
- The composer exposes only a loading button; it has no per-file upload chip or failure state.

## Fix Design

- Add a first-class waiting-approval result to the assistant run.
- When a tool requires approval, publish the full approval payload, mark the run/session as `waiting_approval`, persist an assistant message with the approval object, and stop final-answer generation for that run.
- On approval, execute the original tool, append a compact assistant message with the execution result, publish `tool_result` and `done`, and complete the run. On rejection, mark the session failed and publish a rejected message.
- Attach approval events to the current conversation message in the frontend and render approval buttons inline.
- Replace right-rail-first upload feedback with composer-level upload chips containing file icon, filename, progress spinner, success, and failure state.
- Include context ref snapshot data in runtime user context and prompt text so the model can see `template_id`, parse status, rule count, and file preview.

## Regression Test Cases

- `request_approval`: sending a write tool request creates a visible inline approval card, keeps the session in `waiting_approval`, and does not produce a final answer before approval. Approving executes the tool and resolves the stream; rejecting stops the session.
- `whitelist`: readonly tools run without approval; non-whitelisted write tools show the inline approval card.
- `full_access`: tools run without approval cards.
- File upload: selecting `/tmp/test1-2.pdf` or the local fallback `/tmp/test-1-2.pdf` shows a composer chip while uploading, success state after upload, and failure state for invalid upload attempts.
- Baseline assistant flow: after upload, the assistant can reference the baseline template context, parse completion can be observed, detection/fix scripts can be generated, tasks can be dispatched, task status can be read, and failures can be surfaced for script repair.

## Verification Steps

- Run Go unit tests for assistant packages.
- Run frontend build and targeted component/e2e tests.
- Rebuild and restart `api-server` and `frontend` containers.
- Execute Playwright tests only against `http://localhost:8081` and `/api/v1` on normal business ports with the provided admin credentials.

## Affected Components

- `api-server/internal/assistant`
- `api-server/internal/api/handler/assistant_handler.go`
- `frontend/src/views/assistant`
- `frontend/src/store/assistant.ts`
- `frontend/e2e`

## Risk and Rollback

Risk is limited to assistant execution and upload UI. Rollback can restore the previous behavior by reverting this fix and restarting `api-server` and `frontend`.
