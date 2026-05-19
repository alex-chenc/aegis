# Code Review: Context Budget and Progressive Compression

**Reviewed**: 2026-05-18
**Branch**: develop (uncommitted changes)
**Decision**: APPROVE

## Summary

Implements three-tier progressive context compression (70%/80%/95%) for agent-runtime integration. The changes span backend LLM client usage parsing, SSE event streaming, handler persistence, and a new frontend context budget indicator. All changes are well-scoped and follow existing patterns.

## Findings

### CRITICAL
None

### HIGH
None

### MEDIUM

1. **`ai_analysis_handler.go:1820-1845` — JSONB type assertion fragility**
   The `total_prompt_tokens` and `total_completion_tokens` are extracted via `v.(float64)` from the Metrics JSONB. If the database driver returns these as `int64` or `json.Number` instead of `float64`, the values silently default to 0. Consider adding `case int64` and `case json.Number` fallbacks.
   **Severity**: MEDIUM — silent data loss, not a crash.

### LOW

1. **`hook_sink_sse.go:278-286` — JSON round-trip for token counts**
   `before_tokens` and `after_tokens` are extracted as `float64` from JSON-marshaled payload. Same JSON number precision concern as above. Not a bug with standard `encoding/json`, but worth noting.
   **Severity**: LOW — standard json.Unmarshal always produces float64 for numbers.

2. **`AIAnalysis.vue:1523-1553` — silent catch blocks**
   The `context_budget` and `context_compressed` SSE handlers have empty `catch {}` blocks. While SSE parse errors are non-critical, logging to console would aid debugging.
   **Severity**: LOW — user-facing impact is nil.

3. **`ContextBudgetIndicator.vue:91-93` — `context_ratio` clamping**
   `ratio` is clamped to `min(context_ratio, 1.0)` which is correct. However, if the backend ever sends a negative ratio, it would display as 0%. Consider `Math.max(0, ...)`.
   **Severity**: LOW — defensive only.

4. **`client_test.go` — no integration-level test for `sendRequestResult`**
   The unit tests cover struct deserialization well, but the `sendRequestResult` / `sendAnthropicRequestResult` methods lack tests with a mock HTTP server. These are tested indirectly via adapter integration.
   **Severity**: LOW — covered by integration path.

## Validation Results

| Check | Result |
|-------|--------|
| Go build (changed packages) | Pass |
| Go tests (adapters) | Pass |
| Go tests (llm/client) | Pass (new tests) |
| Frontend type-check | Pass (pre-existing errors in unrelated files) |
| Frontend build | Pass |
| Docker containers | Healthy |

## Files Reviewed

| File | Change Type | Lines Changed |
|------|------------|---------------|
| `api-server/go.mod` | Modified | +1/-1 |
| `api-server/go.sum` | Modified | +2 |
| `api-server/internal/llm/client.go` | Modified | +210 |
| `api-server/internal/llm/client_test.go` | Modified | +96 |
| `api-server/internal/llm/adapters/llm_client_adapter.go` | Modified | +14/-3 |
| `api-server/internal/llm/adapters/llm_client_adapter_test.go` | Modified | +17 |
| `api-server/internal/llm/adapters/runtime_factory.go` | Modified | +9 |
| `api-server/internal/llm/adapters/hook_sink_sse.go` | Modified | +38 |
| `api-server/internal/api/handler/ai_analysis_handler.go` | Modified | +87/-12 |
| `frontend/src/api/aiAnalysis.ts` | Modified | +22 |
| `frontend/src/views/detection/AIAnalysis.vue` | Modified | +66/-1 |
| `frontend/src/components/ContextBudgetIndicator.vue` | Added | +219 |
| `docs/aegis_system_design_v5.7/agent_context_compression_design.md` | Added | (design doc) |
