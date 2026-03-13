# AGENTS.md — Repository Guide for Coding Agents

This document is the **repo-verified operating guide** for agentic coding in `/code/ai-benchmark`.

## 1) What Exists in This Repo

- Implemented code modules:
  - `backend/` (Go)
  - `frontend/` (Vue 3 + TypeScript + Vite)
  - `agent/` (Go)
- Design/planning docs are extensive (`aegis_system_design_v2.2/`, `aegis_system_design_v3.0/`, `docs/plans/`).
- When instructions conflict, prefer **executable sources** (Makefiles, package scripts).

## 2) Authoritative Command Sources

- `backend/Makefile`
- `frontend/Makefile`
- `agent/Makefile`
- `frontend/package.json` (`scripts`)

Use these as source of truth for build/test/lint behavior.

## 3) Build / Lint / Test Commands

### Backend (Go)

Run from `backend/`:

```bash
make build
make test
make clean
```

Equivalent direct commands:

```bash
go build -o backend ./cmd/server/main.go
go test ./...
```

Single-test commands:

```bash
go test -v ./internal/repository
go test -v ./internal/repository -run TestTaskLogRepository_UpdateForRedispatch
```

### Frontend (Vue + TypeScript)

Run from `frontend/`:

```bash
make install
make build
make test
make clean
```

Equivalent direct commands:

```bash
npm install
npm run dev
npm run build
npm run test
npm run lint
npm run type-check
```

Single-test commands (Vitest):

```bash
npm run test -- --grep "test name"
npm run test -- src/components/FooBar.test.ts
```

### Agent (Go)

Run from `agent/`:

```bash
make build
make test
make clean
```

`make build` cross-compiles Linux binaries for `amd64` + `arm64` into `agent/dist/`.

Single-test commands:

```bash
go test -v ./...
go test -v ./cmd/agent -run TestName
```

## 4) Recommended Validation Sequence

Run checks only for impacted modules, but be strict within those modules.

1. Go modules (`backend`/`agent`)
   - `go test ./...`
   - `go build ...` (or `make build`)
2. Frontend
   - `npm run lint`
   - `npm run type-check`
   - `npm run test`
   - `npm run build`

Do not mark work complete without relevant validation.

## 5) Coding Style Guidelines

### Go (backend + agent)

- Use standard Go formatting (`gofmt` conventions).
- Import groups in this order:
  1. Standard library
  2. External dependencies
  3. Internal packages
- Naming:
  - Local vars/functions: `camelCase`
  - Exported symbols/types: `PascalCase`
  - Layer suffixes: `...Repository`, `...Service`
- Error handling:
  - Wrap propagated errors with context and `%w`
  - Example: `fmt.Errorf("failed to X: %w", err)`
  - Never swallow errors silently
  - Handle errors at appropriate layer boundaries
- Keep transport concerns separate from business logic.

### TypeScript / Vue (frontend)

- Use Vue SFC with `<script setup lang="ts">`.
- Prefer strong typing; avoid `any` unless unavoidable.
- Import grouping:
  1. Vue ecosystem (`vue`, `vue-router`, `pinia`)
  2. External libs (`element-plus`, etc.)
  3. Internal aliases (`@/...`)
- Naming:
  - Components: `PascalCase.vue`
  - Composables: `useXxx.ts`
  - Store files/modules: `camelCase.ts`
  - Types/interfaces: descriptive `PascalCase`
- State management:
  - Pinia for shared/global state
  - `ref`/`reactive` for local state
- UI stack is Element Plus; keep usage and patterns consistent.

## 6) Language & Documentation Conventions

- Repository docs are primarily Chinese.
- Match the local language/style of the file you edit.
- If the surrounding file is English, keep additions English; if Chinese, keep Chinese.

## 7) Cursor / Copilot Rules Status

Checked for:

- `.cursor/rules/`
- `.cursorrules`
- `.github/copilot-instructions.md`

Current status: **none of these files exist** in this repository.

If added later, treat them as higher-priority constraints and update this AGENTS.md.

## 8) Agent Execution Notes

- Prefer focused, minimal edits over broad refactors.
- Mirror existing patterns in the touched module.
- Validate each changed layer/module before finishing.
- Do not invent workflow commands; use Makefile/scripts/package.json.

## 9) Quick Command Cheat Sheet

```bash
# Backend
cd backend && make test && make build

# Frontend
cd frontend && npm run lint && npm run type-check && npm run test && npm run build

# Agent
cd agent && make test && make build
```

For debugging, reproduce with the smallest single-test command first, then scale up to module-level checks.
