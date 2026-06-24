# Repository Guidelines

## Project Structure & Module Organization

Aegis is a multi-service host security platform. Go services are split by role: `api-server/` handles REST, LLM, storage, and orchestration; `server/` is the gRPC agent hub; `dc/` consumes Kafka and drives alert/blocking; `agent/` handles eBPF, Sigma matching, and task execution; `builder/` builds dynamic eBPF packages. Shared protobufs live in `proto/`, generated Go stubs in service `pkg/api/`, migrations in `migrations/` and `api-server/migrations/`, scripts in `scripts/`, and Docker/release assets in `docker/`, `docker-compose.yml`, and `release/`. The Vue 3 frontend is in `frontend/src/`; Playwright specs are in `frontend/e2e/`.

## Build, Test, and Development Commands

- `docker compose up -d --build`: build and run the full local stack.
- `cd api-server && make build && make test`: build and test the API server.
- `cd server && make build && make test`: build and test the gRPC hub.
- `cd dc && make build && make test`: build and test the data consumer.
- `cd agent && make bpf && make build && make test`: compile eBPF artifacts, binaries, and tests.
- `cd frontend && npm install && npm run dev`: run the Vite dev server.
- `cd frontend && npm run build && npm run test`: build assets and run Vitest.

## Coding Style & Naming Conventions

Use `gofmt` for Go and keep package names lowercase. Do not edit generated `*.pb.go` files unless the matching `proto/*.proto` changed. Frontend uses Vue 3, TypeScript, Pinia, and Element Plus; keep two-space indentation, prefer PascalCase component filenames, and use the `@/` alias for `frontend/src`. Run `npm run lint` and `npm run type-check` before UI PRs.

## Testing Guidelines

Backend unit tests use Go's standard runner. Run targeted packages while iterating, for example `go test ./internal/assistant`, then broaden to `go test ./...`. Frontend unit tests use Vitest; place them beside source as `*.test.ts`. Playwright specs may require a running stack and environment flags.

## Commit & Pull Request Guidelines

History mostly follows conventional commits such as `feat: ...` and `fix ...`; prefer `feat:`, `fix:`, `refactor:`, `test:`, or `docs:` with an imperative summary. PRs should explain the change, list backend/frontend/migration impact, link issues, include screenshots for UI work, and note tests run.

## Security & Configuration Tips

Copy `.env.example` to `.env` and never commit real keys. Treat Docker volumes, migrations, blocking policies, and agent task dispatch as high-impact changes that need explicit testing notes.
