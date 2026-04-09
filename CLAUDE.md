# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Aegis (ai-benchmark) is an AI-native host security platform that integrates LLM technology for automated baseline auditing, vulnerability management, and real-time threat detection. The system uses a microservices architecture: API Server (control plane), Server (gRPC agent hub), DC (data consumer), Agent (data plane), and Frontend (UI).

## Build and Test Workflow

**IMPORTANT**: When performing build, test, or verification operations, you **MUST** use the `aegis-build-test` skill. This ensures consistent build/test procedures across all components.

To invoke the skill, use: `/aegis-build-test`

## High-Level Architecture (V5.5)

```
┌─────────────────────────────────────────────────────────────────┐
│                        Frontend (Vue 3)                         │
│                     localhost:8081 (Nginx)                      │
└─────────────────────────────┬───────────────────────────────────┘
                              │ HTTP/WebSocket
┌─────────────────────────────▼───────────────────────────────────┐
│                    API Server (Go)                              │
│               HTTP:8082 / gRPC:19093 (client)                   │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────────┐   │
│  │ Handlers│ │ Services │ │   LLM    │ │ gRPC Client      │   │
│  │ (API)   │ │(Business)│ │ Client   │ │ (DC Comm)        │   │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────────┘   │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────────┐   │
│  │Repository│ │ MinIO    │ │ Redis    │ │ Kafka Producer   │   │
│  │(Postgres)│ │(Scripts) │ │ (Cache)  │ │ (Events)         │   │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────────┘   │
└─────────────────────────────┬───────────────────────────────────┘
                              │ gRPC
┌─────────────────────────────▼───────────────────────────────────┐
│                    DC Server (Go)                              │
│                    gRPC:19092                                   │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────┐   │
│  │ Kafka Consumer│ │ Aggregator   │ │ LLM Analyzer        │   │
│  │              │ │ (Host Window)│ │ (False Positive)     │   │
│  └──────────────┘ └──────────────┘ └──────────────────────┘   │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────┐   │
│  │Alert Generator│ │Block Manager │ │ Repository           │   │
│  │              │ │              │ │ (Postgres)           │   │
│  └──────────────┘ └──────────────┘ └──────────────────────┘   │
└─────────────────────────────┬───────────────────────────────────┘
                              │ gRPC (bi-directional streaming)
┌─────────────────────────────▼───────────────────────────────────┐
│                    Server (Go)                                │
│               gRPC:19090 (Agent Hub + Kafka Producer)          │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────────┐   │
│  │ gRPC     │ │ Kafka    │ │ Repository│ │ Storage          │   │
│  │ Server   │ │ Producer │ │ (Host)   │ │ (Redis)         │   │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────────┘   │
└─────────────────────────────┬───────────────────────────────────┘
                              │ gRPC (bi-directional streaming)
┌─────────────────────────────▼───────────────────────────────────┐
│                       Agent (Go)                               │
│                   Deployed on target hosts                      │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────────┐   │
│  │ eBPF     │ │  Sigma   │ │Executor  │ │ gRPC Client      │   │
│  │Collector │ │ Matcher  │ │(Scripts)│ │ (Server Comm)   │   │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Microservices

| Service    | Port         | Purpose                                        |
| ---------- | ------------ | ---------------------------------------------- |
| frontend   | 8081         | Vue 3 UI (Nginx)                               |
| api-server | 8082         | HTTP REST API, gRPC client to DC               |
| server     | 19090, 19094 | 19090: Agent Hub; 19094: API Server comms      |
| dc         | 19092        | Kafka Consumer, LLM Analysis, Alert Generation |
| postgres   | 5432         | Primary database                               |
| redis      | 6379         | Caching                                        |
| minio      | 9000/9001    | Object storage for scripts                     |
| kafka      | 29092        | Message queue for runtime events               |

## Key Architectural Components

### gRPC Protocol Files

| Proto File              | Purpose                                | Location                      |
| ----------------------- | -------------------------------------- | ----------------------------- |
| `agent_comm.proto`      | Agent ↔ Server bidirectional streaming | `proto/agent_comm.proto`      |
| `api_server_comm.proto` | API Server ↔ Server command forwarding | `proto/api_server_comm.proto` |

### Agent → Server → DC Communication (gRPC)

The Agent registers with the Server via gRPC bi-directional streaming (defined in `proto/agent_comm.proto`):

1. **Registration**: Agent sends `RegisterRequest` with host asset info
2. **Heartbeat**: Periodic `HeartbeatRequest` for liveness (90s timeout)
3. **Command Dispatch**: Server sends `CommandExecute` with script content
4. **Result Reporting**: Agent returns `CommandResult` with exit code, stdout, stderr
5. **Runtime Events (V5.0)**: Agent sends security events via Kafka; DC consumes and analyzes

The gRPC server implementation is in `server/internal/grpc_server/` and the DC server in `dc/internal/server/`.

### Task Command Flow

```
Frontend → API Server (8082) → Server APIServerToServer (19094) → Agent (19090, bidirectional)
```

1. Frontend sends HTTP request to API Server
2. API Server calls `ForwardCommand` via gRPC to Server (port 19094)
3. Server dispatches command to Agent via bidirectional gRPC stream (port 19090)
4. Agent executes and returns result through the same stream
5. Server forwards result back to API Server via gRPC

Note: Server has two distinct gRPC ports:
- **19090**: Agent Hub (bidirectional streaming with Agents)
- **19094**: APIServerToServer (unary RPC from API Server)

### Runtime Threat Detection Pipeline (V5.0)

The real-time threat detection uses eBPF + Sigma rules + LLM analysis:

1. **eBPF Collection** (`agent/internal/ebpf/`): Hooks execve, fork, openat, connect, setuid, setgid, capset
2. **Sigma Matching** (`agent/internal/sigma/`): Local rule-based filtering on agent side
3. **Event Pipeline** (`dc/internal/pipeline/`): Host-window aggregation and LLM analysis
4. **Alert Management** (`dc/internal/alert_generator/`, `dc/internal/block_manager/`): Deduplication, false-positive analysis, auto-block

Build eBPF programs before building the agent: `cd agent && make bpf && make build`

### Kafka Topics

| Topic                   | Partitions | Purpose                                      |
| ----------------------- | ---------- | -------------------------------------------- |
| `aegis.security.events` | 3          | Runtime security events from Agent Hub to DC |
| `aegis.block.commands`  | 3          | Block commands from DC to Agent Hub          |

### LLM Integration Layer

The LLM integration follows a worker-queue pattern:

- **Service Layer**: `api-server/internal/service/` contains LLM-enabled services
- **LLM Client**: `api-server/internal/llm/client.go` handles API calls with retry logic
- **Async Processing**: Services start background workers that consume from Redis queues
- **Progress Tracking**: Frontend polls `/api/v1/templates/{id}/status` for parsing progress

### Self-Healing Flow

When a detection or remediation script fails:

1. `SelfHealingService` receives the error context
2. LLM analyzes the error and generates a corrected script
3. New script is queued for execution (max 3 retries)
4. Healing logs stored via `HealingLogRepository`

## Repository Layer Patterns

All repositories follow GORM patterns:

- `NewXxxRepository(db *gorm.DB)` constructor
- Database config loaded from service-specific config files
- Migrations in `migrations/` (SQL files)

## Build Commands

### API Server

```bash
cd api-server && make
```

### Server (gRPC Agent Hub)

```bash
cd server && make
```

### DC (Data Consumer)

```bash
cd dc && make
```

### Frontend

```bash
cd frontend && npm install && npm run build
```

### Agent

```bash
cd agent && make bpf && make build
```

### Full Stack (Docker Compose)

```bash
cp .env.example .env
docker compose up -d --build
```

## Test Commands

### Go Services

```bash
# Single test
go test -v ./internal/repository -run TestTaskLogRepository_UpdateForRedispatch

# All tests in package
go test -v ./internal/service/...
```

### Frontend

```bash
cd frontend
npm run test -- --grep "test name"
npm run test -- src/components/FooBar.test.ts
npm run lint
npm run type-check
```

## Important File Locations

| Concern                       | Location                             |
| ----------------------------- | ------------------------------------ |
| API Server entry point        | `api-server/cmd/main.go`             |
| Server (gRPC Hub) entry point | `server/cmd/main.go`                 |
| DC entry point                | `dc/cmd/main.go`                     |
| Agent entry point             | `agent/cmd/agent/main.go`            |
| Agent ↔ Server proto          | `proto/agent_comm.proto`             |
| API Server ↔ Server proto     | `proto/api_server_comm.proto`        |
| LLM prompts                   | `api-server/internal/llm/prompts.go` |
| File parsers                  | `api-server/internal/fileparser/`    |
| Sigma rules (agent)           | `agent/internal/sigma/`              |
| DC Pipeline                   | `dc/internal/pipeline/`              |
| Docker orchestration          | `docker-compose.yml`                 |

## Frontend API Integration

Frontend makes requests to `/api/v1/*` which is proxied to `api-server:8082`. The API layer uses:

- Axios with custom interceptors for response handling
- Pinia for state management
- Element Plus for UI components
- ECharts for visualizations

## Agent Installation

On target hosts, run:

```bash
curl -sSL http://<SERVER_IP>:19090/api/v1/agent/install.sh | sudo bash
```

Note: The install script endpoint is on port 19090 (Server), not the API Server.
