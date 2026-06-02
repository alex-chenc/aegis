# Detection Package Agent Sync And Status Dedupe Fix

## Bug Description And Symptoms

For package `b1c4300a-d050-4b12-8b0f-b41fce167b1e`, newly connected agents did not receive the enabled dynamic detection package. Existing agents also showed two host-status rows for the same package and host, such as `1.0.1 active` plus old `1.0.0 uninstalled`.

## Reproduction Steps

1. Enable dynamic detection package version `1.0.1`.
2. Connect a new Agent after the enable operation has already completed.
3. Open the package host-status detail page.
4. Observe that the new Agent is not installed, and the original Agent can show multiple rows across old and current package versions.

## Root Cause Analysis

`EnablePackage` sends install commands only to agents currently connected to Server. When a new Agent establishes the bidirectional stream later, Server pushes active configs but not enabled dynamic detection packages.

Host-status records were keyed by `(package_id, version, host_id)`, and the frontend query requested all versions for a package. Version transitions therefore left old host rows visible instead of showing one current state per Agent.

## Fix Design

- Add Server-side enabled detection package repository.
- On Agent stream establishment, push active configs and then all enabled dynamic detection packages to that Agent.
- Build package download URLs with `MINIO_ARTIFACT_BASE_URL`, matching API Server behavior.
- Default package host-status listing to the latest package version.
- When a host reports a package status for a newer version, remove older status rows for the same `(package_id, host_id)`.

## Code Changes

- `server/internal/model/detection_package.go`
- `server/internal/repository/detection_package_repo.go`
- `server/internal/grpc_server/server.go`
- `server/cmd/main.go`
- `docker-compose.yml`
- `api-server/internal/service/detection_package_service.go`
- `api-server/internal/repository/detection_package_repo.go`

## Verification Steps

- `go test ./internal/service -run 'TestListHostStatusDefaultsToLatestPackageVersion|TestReportHostStatusRemovesPreviousVersionForHost|TestSyncDetectionPackageRules|TestUpdateDraftResetsBuildStatus'`
- `go test ./internal/grpc_server`
- Rebuild and restart `api-server` and `server`.
- Confirm service health.
- Clean existing duplicate host-status rows for the affected package.

## Risk And Rollback Plan

Risk is limited to dynamic detection package startup synchronization and host-status presentation. Roll back by reverting the listed files. If needed, old status history can still be queried directly from the database before cleanup.
