# Frontend Module Rollback Recovery Fix

## Bug Description And Symptoms

Multiple frontend modules were overwritten by older implementations:

- Baseline workbench showed the old upload-first layout instead of the V6.1 rule management page.
- Baseline task center lost pass-rate cards, live refresh status, and report export entry points.
- Host list lost the right-side asset drawer for software and application assets.
- Application asset pages lost category tabs, PID/container labels, and runtime details.
- Weak password frontend files and real E2E cases were removed.
- Model settings reintroduced image-model configuration that the current frontend baseline removed.
- Some E2E cases still asserted old "模板上传" baseline UI text.

## Root Cause Analysis

The frontend working tree diverged from `HEAD` with thousands of deleted lines and several old implementations replacing newer V6.1 UI files. The rollback was broad enough that page-by-page repair would be fragile. The stable source of the intended frontend baseline is the current repository `HEAD`, with the current baseline auto-healing wording patch reapplied on top.

## Fix Design

- Restore `frontend/` from `HEAD` to recover the current V6.1 frontend baseline.
- Reapply only the current baseline task-detail behavior changes:
  - Replace user-facing "脚本修复" and "ReAct 修复" text with "大模型修复".
  - Do not show the direct "修复" action for a CHECK task that is merely non-compliant.
  - Keep large-model repair actions limited to task execution failure, timeout, or prior repair failure states.
- Update stale E2E assertions that still target the old baseline upload-first page.
- Keep the newly added real E2E for CHECK exit code `1` non-compliance behavior.

## Code Changes

- `frontend/`: restored to the current repository baseline.
- `frontend/src/views/TaskDetail.vue`: reapplied large-model repair wording and removed the direct non-compliance repair action.
- `frontend/e2e/assistant-real-business.spec.ts`: updated baseline upload flow to open "文件解析" from the V6.1 rule management page.
- `frontend/e2e/baseline-react-auto-healing-real.spec.ts`: retained as the real regression for non-compliant CHECK tasks not entering large-model repair.

## Verification Steps

- `cd frontend && npm run build`
- `docker compose up -d --build frontend`
- Real Playwright checks:
  - `/baseline/workbench` shows "规则管理", "文件解析", and "任务下发".
  - `/baseline/tasks` shows "平均通过率", live refresh, and "合规报告".
  - `/hosts` IP link opens the asset drawer with software and application sections.
  - `/settings/models` does not show "图片模型配置".
  - A CHECK task with `exit_code=1` exposes "未通过" without direct "修复" or old "脚本修复/ReAct" text.

## Risk And Rollback Plan

The restore intentionally discards stale frontend working-tree changes and returns the frontend to the current repository baseline. If a removed frontend change is later confirmed as desired, reintroduce it as a small feature patch with its own design and test coverage instead of mixing it with rollback recovery.
