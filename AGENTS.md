# AGENTS.md

This file gives Codex project guidance for the Aegis (`ai-benchmark`) repository.
`CLAUDE.md` and `.claude/` are kept for Claude compatibility; the Codex-native
copy of the operational guidance lives here and under `.agents/codex-skills/`.

## Project Overview

Aegis is an AI-native host security platform for automated baseline auditing,
vulnerability management, and runtime threat detection. The system is split into:

- `api-server/`: Go HTTP control plane, LLM services, repositories, MinIO, Redis,
  Kafka producer, and gRPC client.
- `server/`: Go gRPC agent hub, command forwarding, Kafka producer, host storage.
- `dc/`: Go data consumer for Kafka events, aggregation, LLM analysis, alerts,
  and blocking.
- `agent/`: Go data-plane agent with eBPF collection, Sigma matching, command
  execution, and gRPC streaming to `server`.
- `frontend/`: Vue 3 UI with Element Plus, Pinia, Axios, and ECharts.

Primary services and ports:

| Service | Ports | Purpose |
| --- | --- | --- |
| `frontend` | `8081` | Vue UI via Nginx |
| `api-server` | `8082`, `19093` | REST API and gRPC client |
| `server` | `19090`, `19094` | Agent hub and API-server command forwarding |
| `dc` | `19092` | Kafka consumer and alert pipeline |
| `postgres` | `5432` | Primary database |
| `redis` | `6379` | Cache and queues |
| `minio` | `9000`, `9001` | Script and agent artifact storage |
| `kafka` | `29092` | Runtime event stream |

## Codex Skills

Use project skills from `.agents/codex-skills/` when their trigger matches the task.
Read the relevant `SKILL.md` before running commands or changing related files.

- Build, test, service verification, docker compose checks, agent packaging, API
  smoke tests: read `.agents/codex-skills/aegis-build-test/SKILL.md`.
- Offline release package creation, Docker image export, MinIO-with-agent image,
  release zip/start script/database init packaging: read
  `.agents/codex-skills/aegis-release-packaging/SKILL.md`.

After development work, run the narrowest useful verification from
`aegis-build-test` when feasible. If verification cannot run because of sandbox,
missing dependencies, Docker availability, or time, report that explicitly.

## Architecture Notes

Agent registration and command execution use bidirectional gRPC defined in
`proto/agent_comm.proto`.

Task command flow:

```text
Frontend -> api-server:8082 -> server:19094 -> agent stream on server:19090
```

Runtime detection flow:

```text
agent eBPF/Sigma -> server -> Kafka topic aegis.security.events -> dc -> PostgreSQL/WebSocket
```

Important Kafka topics:

| Topic | Purpose |
| --- | --- |
| `aegis.security.events` | Runtime security events from agent hub to DC |
| `aegis.block.commands` | Block commands from DC to agent hub |

The LLM integration follows a worker-queue pattern in `api-server/internal/service/`
with `api-server/internal/llm/client.go` handling API calls and retries.

## Repository Patterns

- Repositories follow GORM patterns with `NewXxxRepository(db *gorm.DB)`.
- Service config is loaded from service-specific config files.
- SQL migrations live in `migrations/`.
- Generated protobuf Go code must stay consistent with `proto/*.proto`.
- Build eBPF programs before building the agent when agent runtime collection
  code changes.

## Common Commands

Use these as starting points; prefer the skill-specific procedure for verification.

```bash
cd api-server && make build
cd server && make build
cd dc && make build
cd agent && make bpf && make build
cd frontend && npm run build
docker compose up -d --build
```

Frontend checks:

```bash
cd frontend
npm run test -- --grep "test name"
npm run test -- src/components/FooBar.test.ts
npm run lint
npm run type-check
```

## Important Files

| Concern | Location |
| --- | --- |
| API server entry | `api-server/cmd/main.go` |
| Server entry | `server/cmd/main.go` |
| DC entry | `dc/cmd/main.go` |
| Agent entry | `agent/cmd/agent/main.go` |
| Agent/server proto | `proto/agent_comm.proto` |
| API-server/server proto | `proto/api_server_comm.proto` |
| LLM prompts | `api-server/internal/llm/prompts.go` |
| File parsers | `api-server/internal/fileparser/` |
| Sigma rules | `agent/internal/sigma/` |
| DC pipeline | `dc/internal/pipeline/` |
| Docker stack | `docker-compose.yml` |
| Current architecture docs | `docs/aegis_system_design_v5.5/` |
| Newer draft architecture docs | `docs/aegis_system_design_v5.6/` |

## Safety Notes

- Do not revert unrelated user changes.
- Treat commands that remove data, reset git state, or stop/delete volumes as
  destructive and ask before running them unless the user explicitly requested it.
- Docker, package installation, and network-dependent verification may require
  user approval in sandboxed environments.
