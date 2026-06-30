# Baseline Asset Workbench Optimization Design v6.1

## Problem Statement And Requirements

This change optimizes three operator workflows:

1. Host list assets: IP addresses in the host list should open a right-side asset drawer with software, databases, Web services, Web sites, Web frameworks, AI LLM, AI Agent, and MCP assets. Each section is vertical and paginated by 10 items.
2. Navigation: the left sidebar should collapse from a footer control and show icons only when collapsed.
3. Baseline rule management and task center: the baseline workbench becomes rule management, removes always-visible template/file side panels, adds a file parse button with upload and real parse progress, improves full-page rule listing, supports searchable rule/host task dispatch with max rounds, improves task-center overview, pass rate, auto refresh, and report export entry points.

The template parse progress must use the backend `/templates/:id/status` progress rather than fake indeterminate progress. Script editing must use CodeMirror 6 with Bash highlighting, default read-only mode, edit toggle, and diff view.

## Current Behavior

- `Workbench.vue` shows a fixed upload card and file sidebar. Parsing progress is hardcoded as an indeterminate 50% bar.
- Script editing uses `el-input` textarea, without syntax highlighting, line numbers, read-only mode, or diff view.
- Rule selection and host selection exist, but hosts are not searchable and dispatch controls are split into side cards.
- Task center requires manual refresh and has no top-level overview, pass-rate display, or export controls.
- Dispatch already performs command audit in `TaskService.dispatchToAgent` through `ScriptAuditService.AuditForDispatch`.
- Agent results are written by `server` into `task_logs`; api-server creates initial task rows and dispatches through server.

## Proposed Behavior

- Host list IP column renders as a link. Clicking opens a right drawer that loads host-scoped software and application assets. Application assets are grouped into database, Web service, Web site, Web framework, AI LLM, AI Agent, and MCP sections. Each section paginates at 10 items.
- Sidebar width toggles between 220px and 64px. Collapsed mode keeps icon navigation and hides text.
- `/baseline/workbench` menu title and route meta display "规则管理".
- Rule management uses a full-page layout:
  - Top toolbar includes "文件解析", "任务下发", refresh, and batch script generation actions.
  - File parse opens an upload dialog. Upload progress comes from Element Plus upload progress. Parse progress polls `getTemplateStatus(id)` every 2 seconds until completed/failed, then refreshes rules.
  - Rule table is full-page, searchable, and paginated. Template metadata remains visible as compact filter/status data instead of a separate file sidebar.
  - Task dispatch opens a dialog with searchable selected-rule and host lists, multi-select checkboxes, and configurable max rounds.
- Script editor dialog uses CodeMirror 6 Shell syntax. Existing generated scripts open read-only; "编辑" toggles editing. A diff tab compares the original loaded script to edited content.
- Task center auto-refreshes every 5 seconds when pending/running tasks exist, shows a live indicator and last refresh time, displays overview cards and per-row pass rate, and exposes PDF/Excel export and weekly/monthly report controls.
- Baseline task dispatch accepts `max_rounds`. api-server stores `attempt_no` and `max_rounds` on `task_logs`. server creates a next-round task in the same group when a failed/timeout/AUDIT_BLOCKED task has remaining rounds. The next task is dispatched through the same command path and therefore preserves pre-dispatch audit behavior.

## Component Design

- Frontend
  - `Dashboard.vue`: host asset drawer and section pagination.
  - `App.vue`: collapsible sidebar and baseline menu labels.
  - `Workbench.vue`: rule management layout, real parse progress polling, CodeMirror script editor, dispatch dialog.
  - `TaskCenter.vue`: overview cards, live polling, pass rate, export controls.
  - API/type updates for `max_rounds`, parse status, and task round fields.
- api-server
  - Add `max_rounds` to run-check/run-fix requests.
  - Add `attempt_no` and `max_rounds` to `TaskLog`.
  - Create first-round task rows with these values.
- server
  - Add the same `TaskLog` fields.
  - After final result update, create and dispatch the next round if current status is not successful and `attempt_no < max_rounds`.
- Database
  - Add migration `020_v6.1_baseline_task_rounds.sql`.
  - Add startup-safe `AutoMigrate`-style column additions where applicable if repository startup migration already manages task schema.

## Data Flow

1. Operator uploads `/root/test-1-2.pdf` from "文件解析".
2. Frontend posts `/templates/upload`, receives `template_id`, then polls `/templates/:id/status` every 2 seconds.
3. When parsing reaches completed, frontend refreshes `/templates` and `/templates/:id/rules`.
4. Operator selects rules and hosts, chooses max rounds, and calls `/tasks/run-check` or `/tasks/run-fix`.
5. api-server creates first-round `task_logs` rows with `attempt_no=1` and `max_rounds`.
6. `TaskService.dispatchToAgent` audits script content with existing command audit rules, then forwards to server.
7. server forwards to Agent and writes final result. If result failed/timed out/audit-blocked and remaining rounds exist, server creates the next round and forwards it.

## Interface Changes

- `POST /api/v1/tasks/run-check`
  - Adds optional `max_rounds`, integer, clamped to 1..10.
- `POST /api/v1/tasks/run-fix`
  - Adds optional `max_rounds`, integer, clamped to 1..10.
- Task log responses add `attempt_no` and `max_rounds`.
- Task group summaries add `pass_rate`.

## Security Impact

- No dispatch bypass is introduced. Each created round uses the same script content and is forwarded through the existing audit path.
- AUDIT_BLOCKED remains a terminal result for the individual round. If max rounds is greater than one, later rounds are still audited and will be blocked again unless audit policy/scripts change.
- No credentials are stored in source or docs.

## Compatibility Impact

- Existing clients that omit `max_rounds` continue to create one round.
- Existing rows receive defaults `attempt_no=1`, `max_rounds=1`.
- Existing UI routes remain unchanged; only display labels change.

## Test Case Design

- Frontend unit/build:
  - `npm run type-check`
  - `npm run build`
- Backend:
  - `cd api-server && make build && go test ./internal/service ./internal/api/handler ./internal/repository`
  - `cd server && make build && go test ./internal/grpc_server ./internal/repository`
- Integration/manual:
  - `docker compose up -d --build api-server server frontend`
  - Health checks for api-server/server.
  - Reinstall Agent with install script.
  - Login as user-provided admin credentials.
  - Playwright checks host IP drawer, sidebar collapse, rule management upload/parse progress with `/root/test-1-2.pdf`, searchable dispatch dialog, task-center live indicator/pass-rate/export controls.

## Rollback Plan

- Revert frontend files, api-server/server model/repository/service changes, and migration `020_v6.1_baseline_task_rounds.sql`.
- Existing added columns are additive and can remain without affecting old code. To fully roll back schema, drop `attempt_no` and `max_rounds` from `task_logs`.
