# Weak Password Online Host And Task Detail UI Fix

## Bug Description

The weak password module allowed analysis candidates from hosts that were not confirmed online at runtime. The task detail UI also exposed low-level Agent tool metrics, showed host UUIDs instead of operator-friendly host identity, and kept the original reveal approval wording even though the desired flow is local system-password verification.

## Reproduction Steps

1. Log in as an administrator.
2. Open `/risk/weak-password`.
3. Analyze application assets and create a weak password check task.
4. Open the task detail page.

Observed issues:

- Offline or non-connected hosts could be selected for detection.
- The host execution table showed host IDs instead of IP and hostname.
- The page showed Agent tool count, latest Agent tool, and risk/reveal approval text.
- Full password viewing used a reveal approval request instead of asking the current user to re-enter the system password.
- Weak password tasks had no delete operation in the module UI.

## Root Cause Analysis

- The frontend exposed an `online/all` selector and sent `online_agents_only` based on that selector.
- The repository accepted `OnlineAgentsOnly` but did not apply a host online filter.
- Task creation performed Agent connectivity checks only after creating a task, which produced failed task records for non-connected agents.
- Findings stored only a masked value; the reveal endpoint created an approval record and never returned the verified password.
- The task detail component rendered implementation-level Agent tool fields directly.

## Fix Design

- Force weak password analysis to online hosts only.
- Filter application assets by host heartbeat freshness and confirm runtime Agent connectivity when a server client is available.
- Reject task creation before persistence if the target Agent is not connected.
- Store matched weak passwords encrypted server-side; list APIs still expose only masked values.
- Change reveal from approval creation to current-user password verification followed by one-time plaintext response.
- Add task deletion API and UI actions.
- Update task detail UI to show host IP and hostname, remove Agent tool/latest tool/risk explanation UI, and replace "申请明文" with "详情".

## Code Changes

- Backend:
  - `WeakPasswordRepository.ListApplicationAssets` now filters online hosts.
  - `WeakPasswordService.AnalyzeAssetApplications` enforces online-only scope and runtime connectivity.
  - `WeakPasswordService.CreateTaskByApplication` rejects offline targets before task creation.
  - `WeakPasswordService.MatchCredentialRecords` stores encrypted matched passwords and all-star masks.
  - `WeakPasswordService.RevealFinding` verifies the current user's password and returns plaintext without approval records.
  - `DELETE /api/v1/weak-password/tasks/:id` deletes terminal weak-password tasks.
- Frontend:
  - Removed the online/all selector and risk explanation column.
  - Task list and task detail now support deletion.
  - Task detail host table shows IP and hostname.
  - Findings show masked passwords and reveal plaintext after system password input.

## Verification Steps

- `cd api-server && go test ./internal/service -run TestWeakPassword`
- `cd frontend && npm run test -- --run src/store/weakPassword.test.ts`
- `cd frontend && npm run build`
- `cd frontend && PLAYWRIGHT_BASE_URL=http://127.0.0.1:8081 npx playwright test e2e/weak-password.spec.ts --reporter=line`
- `cd frontend && PLAYWRIGHT_REAL=1 PLAYWRIGHT_BASE_URL=http://127.0.0.1:8081 AEGIS_E2E_USERNAME=admin AEGIS_E2E_PASSWORD='Admin@123' npx playwright test e2e/weak-password-real.spec.ts --reporter=line`

## Affected Components

- `api-server`
- `frontend`

## Risk And Rollback Plan

The online-only behavior may reduce visible weak password candidates when no Agent is connected. Roll back by reverting the changed weak-password backend service/repository files and frontend weak-password views/tests.
