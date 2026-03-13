# AGENTS.md - AI Coding Agent Guide

> **Project Status**: Design/Specification Phase - No source code implemented yet.
> This repository contains comprehensive design documents for an "Automated Aegis Check and Self-Healing System" (自动化基线检查与自愈系统).

## Project Overview

A platform for automated aegis security checking and self-healing for server infrastructure. The system uses LLM (Large Language Model) intelligence to parse aegis documents and generate executable scripts, deployed through agents on target hosts.

**Core Features**:
- Upload aegis documents (PDF, Word, YAML) → LLM parses into check/fix rules
- Deploy agents on servers for automated aegis checking
- Self-healing: LLM fixes failed scripts and retries automatically

---

## Architecture

### Tech Stack

| Component | Technology |
|-----------|------------|
| **Backend** | Go 1.20+, Gin (REST API), gRPC (Agent communication) |
| **Frontend** | Vue 3 (Composition API), TypeScript, Vite, Pinia, Element Plus |
| **Agent** | Go (cross-compiled: linux/amd64, linux/arm64) |
| **Database** | PostgreSQL |
| **Cache** | Redis |
| **Storage** | MinIO (file/object storage) |

### Project Structure (Planned)

```
/backend
├── cmd/server/main.go           # Entry point
├── config/config.yaml           # Configuration
├── internal/
│   ├── api/handler/             # HTTP handlers
│   ├── api/middleware/          # CORS, logging, recovery
│   ├── grpc_server/             # Agent communication
│   ├── service/                 # Business logic
│   ├── repository/              # Data access layer
│   ├── llm/                     # LLM client, prompts, parser
│   ├── fileparser/              # PDF, Word, YAML, Excel parsers
│   ├── storage/                 # MinIO, Redis clients
│   ├── ipdetect/                # Server IP auto-detection
│   └── model/                   # Data models
├── pkg/api/v1/                  # Generated protobuf code
├── scripts/                     # DB init scripts
├── Makefile
├── build.sh
└── Dockerfile

/frontend
├── src/
│   ├── api/                     # Axios API layer
│   ├── components/              # Reusable components
│   ├── composables/             # Vue composition functions
│   ├── store/                   # Pinia stores
│   ├── types/                   # TypeScript definitions
│   ├── utils/                   # Helper functions
│   └── views/                   # Page components
├── package.json
├── vite.config.ts
├── tsconfig.json
├── Makefile
├── build.sh
└── Dockerfile

/agent
├── cmd/agent/main.go            # Agent entry point
├── dist/                        # Cross-compiled binaries
├── Makefile
├── build.sh
└── Dockerfile
```

---

## Build Commands

### Backend (Go)

```bash
# Build binary
make build
# or: go build -o backend ./cmd/main.go

# Run tests
make test
# or: go test ./...

# Run single test
go test -v ./internal/service -run TestServiceName

# Build Docker image
./build.sh

# Clean
make clean
```

### Frontend (Vue/TypeScript)

```bash
# Install dependencies
make install
# or: npm install

# Build for production
make build
# or: npm run build

# Development server
npm run dev

# Run tests
make test
# or: npm run test

# Run single test
npm run test -- --grep "test name"

# Lint
npm run lint

# Type check
npm run type-check
# or: vue-tsc --noEmit

# Build Docker image
./build.sh

# Clean
make clean
# removes dist/ and node_modules/
```

### Agent (Go)

```bash
# Cross-compile for all targets (linux/amd64, linux/arm64)
make build

# Cross-compile for specific target
GOOS=linux GOARCH=amd64 go build -o ./dist/aegis-agent-linux-amd64 ./cmd/agent

# Run tests
make test
# or: go test ./...

# Upload artifacts to MinIO
make upload
# or: ./build.sh

# Clean
make clean
```

---

## Code Style Guidelines

### Go (Backend & Agent)

**Imports**: Group imports in 3 sections (stdlib, external, internal):
```go
import (
    // Standard library
    "context"
    "fmt"
    "net/http"
    
    // External packages
    "github.com/gin-gonic/gin"
    "go.uber.org/zap"
    
    // Internal packages
    "aegis-system/internal/model"
    "aegis-system/internal/repository"
)
```

**Naming**:
- Use `camelCase` for local variables, `PascalCase` for exported
- Repository interfaces: `HostRepository`, `TemplateRepository`
- Service structs: `ConfigService`, `TaskService`
- Handler functions: `GetHosts`, `UploadTemplate`

**Error Handling**:
- Always wrap errors with context: `fmt.Errorf("failed to get host: %w", err)`
- Use custom error types for business logic errors
- Return errors up the call stack, handle at handler level

**Database**:
- Use UUID as primary key: `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`
- Always include `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
- Use `updated_at` with trigger for automatic updates

**API Response Format**:
```go
// Success
{
    "code": 0,
    "message": "success",
    "data": { ... }
}

// Error
{
    "code": 1001,
    "message": "host not found",
    "data": null
}
```

### TypeScript/Vue (Frontend)

**Imports**: Group and sort by path alias then external:
```typescript
// Vue ecosystem
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'

// External packages
import { ElMessage, ElMessageBox } from 'element-plus'

// Internal modules
import { useHostStore } from '@/store/hosts'
import { fetchHosts } from '@/api/hosts'
import type { Host } from '@/types'
```

**Naming**:
- Components: `PascalCase.vue` (e.g., `LogTerminal.vue`, `FileUpload.vue`)
- Composables: `use*.ts` (e.g., `usePagination.ts`)
- Store files: `camelCase.ts` (e.g., `hosts.ts`, `config.ts`)
- Types/Interfaces: `PascalCase` with descriptive names

**TypeScript**:
- Prefer `interface` over `type` for object shapes
- Use strict mode, avoid `any`
- Define API response types in `types/api.ts`

**Components**:
- Use Composition API with `<script setup lang="ts">`
- Extract reusable logic to composables
- Use Pinia for global state, `ref`/`reactive` for local state

**Data Fetching**:
- No WebSockets - all updates are user-triggered (refresh button or after operations)
- Use Pinia actions for API calls
- Show loading states during data fetch

**Element Plus**:
- Use `ElInput`, `ElButton`, `ElTable`, `ElPagination`, `ElUpload`
- Wrap form controls with validation
- Use built-in message/notification for feedback

---

## API Endpoints (REST)

| Method | Endpoint | Description |
|--------|----------|-------------|
| **Config** |||
| GET | `/api/v1/config/llm` | Get LLM config (masked API key) |
| POST | `/api/v1/config/llm` | Save LLM config |
| POST | `/api/v1/config/llm/test` | Test LLM connectivity |
| **Hosts** |||
| GET | `/api/v1/hosts` | List hosts (paginated) |
| GET | `/api/v1/hosts/:id` | Get host details |
| **Templates** |||
| POST | `/api/v1/templates/upload` | Upload template file |
| GET | `/api/v1/templates/:id/status` | Get parsing status |
| GET | `/api/v1/templates/:id/rules` | Get parsed rules |
| **Tasks** |||
| POST | `/api/v1/tasks/run-check` | Execute check task |
| POST | `/api/v1/tasks/run-fix` | Execute fix task |
| GET | `/api/v1/tasks/:id/logs` | Get task logs |
| **Agent** |||
| GET | `/api/v1/agent/install-command` | Get install command |
| GET | `/api/v1/agent/install.sh` | Dynamic install script |
| GET | `/api/v1/agent/download` | Download agent binary |

---

## Database Tables

| Table | Purpose |
|-------|---------|
| `hosts` | Agent-managed host information |
| `templates` | Uploaded aegis templates metadata |
| `aegis_rules` | Parsed check/fix rules from templates |
| `task_logs` | Task execution logs (check/fix) |
| `llm_configs` | LLM service configuration |
| `script_versions` | Script version history |
| `self_healing_logs` | Self-healing process logs |

---

## Key Implementation Notes

1. **No WebSocket**: All data updates are user-triggered via refresh buttons or post-operation callbacks

2. **Self-Healing Flow**: When a script fails → LLM analyzes error → generates new script → retries (max 3 attempts)

3. **Agent Communication**: Backend uses gRPC bidirectional streaming for real-time command execution

4. **File Storage**: MinIO stores uploaded templates and agent binaries

5. **IP Detection**: Backend auto-detects server IP at startup (prioritizes public IP) for agent install commands

6. **API Key Security**: LLM API keys are AES-256 encrypted in database, returned masked to frontend

---

## Design Documents Reference

Located in `aegis_system_design_v2.2/`:

- `prd_design_v2.1_complete.md` - Product requirements
- `backend_detailed_design_v2.1_complete.md` - Backend architecture
- `frontend_detailed_design_v2.1_complete.md` - Frontend architecture
- `database_structure_design_v2.1_complete.md` - DB schema
- `agent_detailed_design_v2.1_complete.md` - Agent design
- `build_system_design_v2.1_complete.md` - Build pipeline
- `communication_structure_design_v2.1_complete.md` - API/gRPC specs
- `infrastructure_design_v2.1_complete.md` - Deployment architecture
- `ai_implementation_prompt_v2.1_complete.md` - LLM prompt engineering

---

## Development Workflow

1. Read design documents first to understand architecture decisions
2. Follow the planned project structure
3. Use Makefiles for consistent build commands
4. All code comments and docs in Chinese (中文) to match existing docs
5. Test with `make test` before committing