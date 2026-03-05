# AGENTS.md

This is the AI Benchmark Baseline System project. It's an automated baseline check and self-healing system using LLM (Tongyi Qianwen) to parse documents, generate scripts, and manage agents.

## Project Structure

The codebase is located in `/code/ai-benchmark/` with the following main components:

| Directory | Language | Description |
| :--- | :--- | :--- |
| `backend/` | Go 1.25 | Core API server (Gin) and gRPC server for agent communication. |
| `agent/` | Go 1.24 | Lightweight probe deployed on target hosts for execution and reporting. |
| `frontend/` | Vue 3 / TS | User interface built with Vite, Pinia, and Element Plus. |
| `baseline_system_design/` | Markdown | Detailed design documents and PRDs. |

## Build Commands

### Backend (Go)

```bash
# Build backend server (single binary with HTTP + gRPC)
cd backend && go build -o server ./cmd/main.go

# Build all packages
cd backend && go build ./...
```

### Agent (Go)

```bash
# Build Agent
cd agent && go build -o agent main.go
```

### Frontend (Vue 3 / Vite)

```bash
# Install dependencies
cd frontend && npm install

# Development mode (hot reload)
npm run dev

# Build for production
npm run build

# Preview production build
npm run preview
```

## Test Commands

### Go Tests (Backend & Agent)

```bash
# Run all tests in a module
cd backend && go test ./...

# Run tests with verbose output
cd backend && go test -v ./...

# Run a single test file
cd backend && go test -v ./internal/api/hosts_test.go

# Run a specific test function
cd backend && go test -v -run TestGetHosts ./internal/api/

# Run tests with coverage
cd backend && go test -cover ./...

# Run agent tests
cd agent && go test -v ./...
```

### Frontend Tests

```bash
# Run unit tests (if Vitest is configured)
cd frontend && npm run test:unit

# Run tests in watch mode
cd frontend && npm run test:unit -- --watch
```

## Lint & Format

### Go

```bash
# Format code
gofmt -w .
goimports -w .

# Vet code
go vet ./...

# Run golangci-lint (if configured)
golangci-lint run
```

### Frontend

```bash
# TypeScript check
cd frontend && vue-tsc --noEmit

# Lint (if ESLint is configured)
cd frontend && npm run lint
```

## Code Style Guidelines

### Go Naming Conventions

| Element | Convention | Example |
| :--- | :--- | :--- |
| Package | `lowercase` | `api`, `models`, `handler` |
| Exported type | `PascalCase` | `HostHandler`, `AgentService` |
| Exported function | `PascalCase` | `NewHostHandler`, `GetHosts` |
| Unexported function | `camelCase` | `handleMessage`, `addConnection` |
| Interface | `PascalCase` + `-er` | `Handler`, `Service` |
| Constant | `PascalCase` or `SCREAMING_SNAKE` | `DefaultTimeout`, `MAX_RETRIES` |
| Private member | `camelCase` | `connections`, `mu` |

### Go Import Order

Group imports into three sections separated by blank lines:
1. Standard library
2. Third-party packages
3. Local project packages

```go
import (
    // Standard library
    "context"
    "log"
    "net/http"

    // Third-party
    "github.com/gin-gonic/gin"
    "gorm.io/gorm"

    // Local project
    "ai-benchmark/backend/internal/models"
)
```

### Go Error Handling

```go
// Return errors as last value
func (h *HostHandler) GetHosts(c *gin.Context) {
    var hosts []models.Host
    if err := h.DB.Find(&hosts).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
        return
    }
    // ... success case
}

// Wrap errors with context where appropriate
if err := db.AutoMigrate(&models.Host{}); err != nil {
    log.Printf("Failed to auto migrate models: %v", err)
}
```

### Go Concurrency

```go
// Use sync.RWMutex for read-heavy workloads
type AgentServiceHandler struct {
    mu          sync.RWMutex
    connections map[string]*AgentConnection
}

// Lock for write
h.mu.Lock()
h.connections[hostID] = conn
h.mu.Unlock()

// Lock for read
h.mu.RLock()
conn, ok := h.connections[hostID]
h.mu.RUnlock()
```

### Frontend (Vue 3 + TypeScript)

#### Component Structure

```vue
<template>
  <!-- Template with kebab-case component tags -->
  <el-card class="box-card">
    <el-button type="primary">Click</el-button>
  </el-card>
</template>

<script setup lang="ts">
// Imports first
import { ref, computed, onMounted } from 'vue';
import { useHostStore } from '@/store/hosts';
import type { Host } from '@/types';

// Store instances
const hostStore = useHostStore();

// Reactive state
const loading = ref(false);
const hosts = ref<Host[]>([]);

// Computed properties
const onlineCount = computed(() => 
  hosts.value.filter(h => h.status === 'online').length
);

// Methods
const fetchData = async () => {
  loading.value = true;
  try {
    await hostStore.fetchHosts(1, 10);
  } finally {
    loading.value = false;
  }
};

// Lifecycle hooks
onMounted(() => {
  fetchData();
});
</script>

<style scoped>
/* Scoped styles */
.box-card {
  margin-bottom: 20px;
}
</style>
```

#### Frontend Naming Conventions

| Element | Convention | Example |
| :--- | :--- | :--- |
| Component file | `PascalCase.vue` | `Dashboard.vue`, `LogTerminal.vue` |
| Composable file | `camelCase.ts` with `use` prefix | `useWebSocket.ts` |
| Store file | `camelCase.ts` | `hosts.ts`, `tasks.ts`, `config.ts` |
| Type file | `index.ts` in `types/` | `types/index.ts` |
| API file | `camelCase.ts` | `hosts.ts`, `request.ts` |
| Interface/Type | `PascalCase` | `Host`, `Task`, `ApiResponse` |

#### TypeScript Patterns

```typescript
// Interface for data models
export interface Host {
  id: string;
  ip_address: string;
  hostname: string;
  status: string;
  [key: string]: any; // Allow additional properties
}

// Generic API response wrapper
export interface ApiResponse<T = any> {
  code: number;
  message: string;
  data: T;
}

// Paginated response
export interface PaginatedResponse<T> {
  total: number;
  items: T[];
}
```

#### Pinia Store Pattern

```typescript
import { defineStore } from 'pinia';

export const useHostStore = defineStore('hosts', {
  state: () => ({
    hosts: [] as Host[],
    total: 0,
    isLoading: false,
    error: null as string | null,
  }),
  
  actions: {
    async fetchHosts(page: number, pageSize: number) {
      this.isLoading = true;
      try {
        const response = await getHosts({ page, pageSize });
        this.hosts = response.items;
        this.total = response.total;
      } catch (err: any) {
        this.error = err.message;
      } finally {
        this.isLoading = false;
      }
    },
  },
  
  getters: {
    onlineCount: (state) => 
      state.hosts.filter(h => h.status === 'online').length,
  },
});
```

## Architecture & Communication

### Agent-Backend (gRPC)

Communication uses bidirectional streaming via gRPC:
- **Protobuf Definitions**: `backend/proto/agent_comm.proto`
- **Heartbeat**: Agent sends every 30 seconds
- **Asset Collection**: Full system info on registration
- **Command Execution**: Real-time script execution with log streaming

### Frontend-Backend (RESTful)

- **API Base**: `/api/v1`
- **Endpoints**:
  - `GET /hosts` - List all hosts
  - `GET /hosts/:id` - Host details
  - `GET /templates` - List templates
  - `POST /templates` - Create template
  - `GET /tasks` - List tasks
  - `GET /settings` - System settings
- **Response Format**: `{ code: number, message: string, data: T }`

### Database Schema

PostgreSQL with the following main tables:
- `hosts` - Agent host information
- `templates` - Baseline check templates
- `baseline_rules` - Individual check/fix rules
- `task_logs` - Execution history

See `init.sql` for complete schema.

## Deployment

The system uses Docker Compose for orchestration:

```bash
# Start all services
docker compose up -d

# Check logs
docker compose logs -f

# Stop and remove containers
docker compose down
```

Services: `backend_api`, `backend_grpc`, `frontend`, `agent1`, `postgres`, `redis`, `minio`

## Configuration & Environment

| Variable | Description | Default |
| :--- | :--- | :--- |
| `DATABASE_URL` | PostgreSQL connection string | Required |
| `REDIS_ADDR` | Redis server address | `localhost:6379` |
| `REDIS_PASSWORD` | Redis password | Required |
| `MINIO_ENDPOINT` | MinIO server endpoint | `localhost:9000` |
| `MINIO_ROOT_USER` | MinIO access key | Required |
| `MINIO_ROOT_PASSWORD` | MinIO secret key | Required |
| `LLM_API_KEY` | API key for Tongyi Qianwen | Required |
| `BACKEND_GRPC_PORT` | Port for gRPC server | `9090` |
| `BACKEND_HTTP_PORT` | Port for API server | `8080` |
| `AGENT_AUTH_TOKEN` | Shared secret for agent auth | Required |

## Design References

Detailed specifications in `baseline_system_design/`:
- `full_system_design_document_v1.4.md` - System architecture
- `agent_detailed_design.md` - Agent internals
- `frontend_detailed_design.md` - Frontend structure
- `api_communication_design.md` - API & WebSocket design
- `database_structure_design_detailed.md` - Schema design
- `docker_deployment_detailed.md` - Containerization

---
**Version**: 1.1
**Project**: AI Benchmark Baseline System