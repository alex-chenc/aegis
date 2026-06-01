# API Error Handling Fixes v5.8

## Overview

This document describes 17 API error handling bugs found during comprehensive API testing of the Aegis system (api-server), and the corresponding fixes applied.

**Date**: 2026-05-29
**Scope**: `api-server/internal/` (handlers and services only)
**Build Status**: ✅ Compiled successfully

---

## Bug Fixes Summary

### P0: HTTP 500 should be 404/400

#### Bug 1: POST /api/v1/detection/packages/drafts — Returns 500 when creating
- **Symptom**: Returns 500 "get draft: record not found" when creating a new draft
- **Root Cause**: Handler's `GetDraft` call with `package_id` was silently ignoring errors, but the service layer was not handling the "not found" case gracefully
- **Fix**: Added proper error logging for `GetDraft` call; draft creation now falls through cleanly when no existing draft is found
- **Files**: `internal/api/handler/detection_package_handler.go`

#### Bug 2-6: POST build/sign/enable/disable/uninstall — Returns 500 when resource not found
- **Symptom**: Returns 500 when draft/build/package not found
- **Root Cause**: Service methods properly wrap `gorm.ErrRecordNotFound` with `ErrNotFound` sentinel, and handlers use `classifyServiceError()` — these were already correctly implemented
- **Status**: Already fixed in current codebase (handlers use `classifyServiceError`)

#### Bug 7: POST /hosts/report — Returns 500 for invalid host_id
- **Symptom**: Returns 500 "invalid host_id: invalid UUID length" instead of 400
- **Root Cause**: Service wraps with `ErrInvalidState`, handler's `classifyServiceError` maps to 400 — already correctly implemented
- **Status**: Already fixed in current codebase

#### Bug 8: GET /builds/:build_id/log — Returns 500 when build not found
- **Symptom**: Returns 500 when build not found
- **Root Cause**: Service properly wraps with `ErrNotFound`, handler uses `classifyServiceError` — already correctly implemented
- **Status**: Already fixed in current codebase

#### Bug 9: POST /builds/:build_id/review — Returns 500 when build not found
- **Symptom**: Returns 500 when build not found
- **Root Cause**: Service properly wraps with `ErrNotFound`, handler uses `classifyServiceError` — already correctly implemented
- **Status**: Already fixed in current codebase

#### Bug 10: GET /vulnerability/:cve_id/task-status — Returns 500 "vulnerability not found"
- **Symptom**: Returns 500 with raw error message instead of 404
- **Root Cause**: `VulnerabilityService.GetCveTaskStatus` did not check for `gorm.ErrRecordNotFound` — just wrapped raw error
- **Fix**: 
  1. Service: Added `errors.Is(err, gorm.ErrRecordNotFound)` check to wrap with `ErrNotFound` sentinel
  2. Handler: Added `errors.Is(err, service.ErrNotFound)` check to return 404
- **Files**: `internal/service/vulnerability_service.go`, `internal/api/handler/vulnerability_handler.go`

---

### P1: Response Format Inconsistency

#### Bug 11: AI Analysis endpoints use {"error":"..."} format
- **Symptom**: Multiple AI analysis endpoints return `{"error":"..."}` instead of `{"code":N,"message":"..."}`
- **Affected Endpoints**: history, message, conclusion, similar, rag-context, execution-result
- **Fix**: Updated all error responses to use standard `{"code":400/404/503,"message":"..."}` format
- **Files**: `internal/api/handler/ai_analysis_handler.go`

#### Bug 12: Pause/cancel/delete non-existent session returns 200
- **Symptom**: POST pause/cancel and DELETE on non-existent session return 200 silently
- **Root Cause**: No existence check before performing operations
- **Fix**: 
  1. `stopActiveAnalysis`: Added session existence check (memory + database) before proceeding; returns 404 if not found
  2. `DeleteSession`: Added session existence check; returns 404 if not found
- **Files**: `internal/api/handler/ai_analysis_handler.go`

---

### P1: Error Info Leakage

#### Bug 13: POST /ai-generate — LLM error exposes provider raw error
- **Symptom**: Response includes raw LLM error with vendor-specific info (e.g., "API returned status 403: unsupported_country_region_territory")
- **Fix**: Replace raw error with generic message: "AI生成服务暂时不可用，请检查LLM配置或稍后重试"
- **Files**: `internal/api/handler/detection_package_handler.go`

#### Bug 14: POST /similar and /rag-context — Embedding API error exposed
- **Symptom**: Embedding API 403 error exposed as 500 with internal details
- **Fix**: Changed from 500 with raw error to 503 with generic message: "搜索服务暂时不可用" / "RAG服务暂时不可用"
- **Files**: `internal/api/handler/ai_analysis_handler.go`

---

### P2: Design Improvements

#### Bug 15: GET /settings/ebpf-hooks/allowlist — First query returns 404
- **Symptom**: First query returns 404 "allowlist not found"
- **Fix**: Return default empty config with 200 status when no allowlist exists:
  ```json
  {"code":0, "data":{"version":0, "tracepoints":[], "kprobes":[], "lsm":[], "xdp":[], "tc":[]}}
  ```
- **Files**: `internal/api/handler/detection_package_handler.go`

#### Bug 16: POST /rollback — Error message exposes internal field name
- **Symptom**: Error "Key: 'TargetVersion' Error:Field validation for 'TargetVersion' failed on the 'required' tag"
- **Fix**: Replace with user-friendly message: "target_version is required"
- **Files**: `internal/api/handler/detection_package_handler.go`

#### Bug 17: GET /generation-status — Missing 'mode' param error not user-friendly
- **Symptom**: Error "mode must be 'poc' or 'fix'" not helpful for users
- **Fix**: Replace with: "请指定生成模式：'poc'（POC验证）或 'fix'（修复脚本）"
- **Files**: `internal/api/handler/vulnerability_handler.go`

---

## Files Modified

| File | Bugs Fixed |
|------|-----------|
| `internal/api/handler/detection_package_handler.go` | 1, 13, 15, 16 |
| `internal/api/handler/vulnerability_handler.go` | 10, 17 |
| `internal/api/handler/ai_analysis_handler.go` | 11, 12, 14 |
| `internal/service/vulnerability_service.go` | 10 |

## Error Response Standards

All API endpoints should use the standard error format:

```json
{"code": 400, "message": "descriptive error message"}
{"code": 404, "message": "resource not found"}
{"code": 500, "message": "internal server error"}
{"code": 503, "message": "service temporarily unavailable"}
```

## Risk and Rollback Plan

- **Risk**: Low — All changes are in error handling paths; successful paths unchanged
- **Rollback**: Revert the 4 modified files to their previous versions
- **Verification**: `cd api-server && make build` — confirmed successful compilation
