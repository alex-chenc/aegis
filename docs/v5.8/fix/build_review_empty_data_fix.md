# Bug Fix: Build Review (构建审核) Empty Data

## Bug Description

The detection package detail page's "构建审核" (Build Review) tab displayed empty data for multiple fields when viewing a completed build:

- **Artifacts**: Empty (no perf/ringbuf .o files listed)
- **Hook Summary**: Wrong field names (Go exported names instead of frontend-expected names)
- **Build Log Tail**: Empty (no build log content)
- **Builder Image Digest**: Empty
- **Review Metadata**: After approving/rejecting, reviewer name/timestamp/comment silently lost
- **Hook 列表 tab**: Empty because it read from `currentPackage?.hook_summary` (always null for drafts) instead of `currentBuild?.hook_summary`
- **Event Schema tab**: Same issue - read from package instead of build

## Root Cause

Six interconnected bugs:

1. **Builder Service**: `StartBuild` compiled perf/ringbuf BPF objects but never populated `result.Artifacts`
2. **gRPC Client**: `BuilderStartBuildResponse` struct missing `Artifacts`, `HookSummary`, `EventSchemaJSON` fields
3. **Service Layer**: `executeBuild` never set `build.BuildLog` or `build.ArtifactSummary` from builder response
4. **Repository**: `UpdateBuild` refactored from GORM to raw SQL but omitted `builder_digest`, `reviewed_by`, `reviewed_at`, `review_comment`
5. **Model**: JSON tags mismatched frontend field names (`builder_digest` vs `builder_image_digest`, etc.)
6. **Frontend**: "Hook 列表" and "Event Schema" tabs read from `currentPackage` (draft, always null) instead of `currentBuild`

## Fix

- Added artifact population in builder `StartBuild`
- Added `BuildArtifactItem` struct and mapping in gRPC client
- Added `BuildLog`/`ArtifactSummary` population in `executeBuild`
- Added missing SET clauses in `UpdateBuild` (builder_digest, reviewed_by, reviewed_at, review_comment)
- Restored early `UpdateBuild` call for "running" status persistence
- Aligned JSON tags with frontend: `builder_image_digest`, `artifacts`, `build_log_tail`
- Updated `HookSummaryItem` JSON tags: `attach_type`, `attach`, `program`, `name`

## Verification

```bash
TOKEN=$(curl -s -X POST http://localhost:8082/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"Cc&324511"}' | jq -r '.token')

# Trigger build
curl -s -X POST http://localhost:8082/api/v1/detection/packages/cve-2026_31431/build \
  -H "Authorization: Bearer $TOKEN"

# Check build data (after build completes)
curl -s http://localhost:8082/api/v1/detection/packages/cve-2026_31431/latest-build \
  -H "Authorization: Bearer $TOKEN" | jq '.data | {artifacts, hook_summary, build_log_tail, clang_version}'

# Approve build and verify review metadata
curl -s -X POST http://localhost:8082/api/v1/detection/packages/builds/{build_id}/review \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"approved": true, "comment": "Approved"}'
```

## Affected Components

- `builder/internal/service/builder_service.go`
- `api-server/internal/grpc/builder_client.go`
- `api-server/internal/service/detection_package_service.go`
- `api-server/internal/repository/detection_package_repo.go`
- `api-server/internal/model/detection_package.go`
- `frontend/src/views/detection/DetectionPackages/PackageDetail.vue`
