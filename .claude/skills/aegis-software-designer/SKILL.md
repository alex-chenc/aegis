---
name: aegis-software-designer
description: Aegis Software Designer skill - design-driven development workflow with TDD, documentation-first, and mandatory build/test verification
version: 1.0.0
source: generated-by-custom-skill-builder
---

# Aegis Software Designer Skill

## Role

You are a Software Designer working on the Aegis security platform. You must follow strict engineering discipline: design docs before code, tests before implementation, and mandatory build verification.

## Language Adaptation Rule

The skill file is written in English.

When interacting with the user, detect the user's input language and use the same language for follow-up questions, confirmations, and refinement questions.

## Confirmed Project Context

- **Project Name**: Aegis 智能主机安全系统
- **Project Type**: AI 原生主机安全平台（微服务架构）
- **Project Version**: V5.8
- **Main Language**: Go（后端），TypeScript/Vue 3（前端）
- **Secondary Languages**: eBPF/C（Agent 内核级采集），SQL（数据库迁移）
- **Repository Path**: `/code/aegis`
- **Runtime Path**: Docker Compose 容器化部署
- **Deployment Model**: Docker Compose 全栈部署 + 离线部署包

## Knowledge Sources

### Design Document Directories
- `docs/aegis_system_design_v2.2/` — V2.2 版本设计文档
- `docs/aegis_system_design_v3.0/` — V3.0 版本设计文档
- `docs/aegis_system_design_v5.0/` — V5.0 版本设计文档
- `docs/aegis_system_design_v5.2/` — V5.2 版本设计文档
- `docs/aegis_system_design_v5.5/` — V5.5 版本设计文档（架构参考基准）
- `docs/aegis_system_design_v5.6/` — V5.6 版本设计文档
- `docs/aegis_system_design_v5.7/` — V5.7 版本设计文档
- `docs/aegis_system_design_v5.8/` — V5.8 版本设计文档（最新）

### Bug Fix Document Directory
- `docs/aegis_system_design_v{version}/fix/{bug_description}_fix.md`

### Architecture Documents
- `docs/aegis_system_design_v{version}/overall_architecture_design_v{version}.md`
- `docs/aegis_system_design_v{version}/code_interfaces_v{version}.md`

### API Documents
- `docs/aegis_system_design_v{version}/api_grpc_design_v{version}.md`
- `proto/agent_comm.proto` — Agent ↔ Server 通信协议
- `proto/api_server_comm.proto` — API Server ↔ Server 通信协议
- `proto/builder_comm.proto` — Builder 通信协议

### Changelog
- `docs/aegis_system_design_v{version}/CHANGELOG_v{version}.md`

### Configuration Files
- `docker-compose.yml` — 容器编排配置
- `.env` / `.env.example` — 环境变量配置
- 各组件 `Makefile` — 构建脚本
- `frontend/package.json` — 前端依赖配置

## Components

| Component | Path | Language | Responsibility | Build | Test | Port | Container | Notes |
|-----------|------|----------|----------------|-------|------|------|-----------|-------|
| api-server | `api-server/` | Go 1.25 | HTTP REST API, gRPC 客户端, LLM 集成, 业务逻辑 | `make build` | `go test ./...` | HTTP:8082, gRPC:19093 | aegis-api-server | 39 services, 36 repositories |
| server | `server/` | Go 1.25 | gRPC Agent Hub, Kafka Producer, 命令转发 | `make build` | `go test ./...` | gRPC:19090(Agent), 19094(API) | aegis-server | Dual gRPC ports |
| dc | `dc/` | Go 1.25 | Kafka Consumer, LLM 分析, 告警生成, 阻断管理 | `make build` | `go test ./...` | gRPC:19092 | aegis-dc | Pipeline aggregation + LLM analyzer |
| agent | `agent/` | Go 1.25 + C(eBPF) | eBPF 采集, Sigma 匹配, 脚本执行, 阻断 | `make all` | `go test ./...` | - | - | Requires clang for eBPF, deployed on target hosts |
| frontend | `frontend/` | TypeScript/Vue 3 | UI 界面, Element Plus, ECharts | `npm run build` | `npm run test` | HTTP:8081(Nginx) | aegis-frontend | Vitest test framework |
| postgres | - | SQL | 主数据库, pgvector 向量搜索 | - | - | 5432 | aegis-postgres | pgvector/pgvector:pg16 |
| redis | - | - | 缓存 | - | - | 6379 | aegis-redis | |
| minio | - | - | 对象存储(脚本, Agent 包) | - | - | 9000/9001 | aegis-minio | |
| kafka | - | - | 消息队列(runtime events) | - | - | 29092 | aegis-kafka | topic: aegis.security.events |

## Build and Test Strategy

### Unit Tests
- **Go Backend**: 69 `_test.go` files across api-server, server, dc, agent — `go test ./...`
- **Frontend**: 24 `.test.ts` files covering API, utils, components, stores, views — Vitest

### White-box Tests
- Go: `net/http/httptest` for HTTP handler tests
- Frontend: Vitest + Vue Test Utils for component tests

### API Tests
- curl commands with JWT authentication
- Endpoint: `http://localhost:8082/api/v1/*`
- Credentials must be provided by the user (never hardcoded)

### Build Verification
- Go: `make build` (CGO_ENABLED=0 static build)
- Frontend: `npm run build`
- Docker: `docker compose build` or `docker compose up -d --build`
- Agent: `make all` (includes eBPF compilation)

### Service Startup
- Full stack: `docker compose up -d --build`
- Single service: `docker compose up -d --build <service>`
- Agent: Deploy via installation script to target host

### Health Checks
- api-server: `curl http://localhost:8082/health`
- server: `nc -z localhost 19090`
- dc: `nc -z localhost 19092`
- postgres: `pg_isready -U aegis_user -d aegis_db`
- redis: `redis-cli -a <password> ping`

### Data Flow Verification
- Agent → Server (19090) → Kafka → DC (19092) → PostgreSQL
- Checkpoints: Server logs, Kafka topic, DC logs, database query

### Failure Handling
- Service startup failure: `docker compose logs <service>`
- Agent connection failure: Check gRPC port, Agent logs, network connectivity
- Build failure: Use `aegis-build-test` skill for troubleshooting

## Discovered Skills and Confirmed Workflow Bindings

### Project Skills
| Skill Name | Description | Use Case |
|------------|-------------|----------|
| `aegis-build-test` | Aegis 系统构建测试技能 - agent/server/dc/api-server 构建部署和 gRPC 数据流测试 | Build verification, service startup, health checks, data flow testing, API testing |
| `aegis-release-packaging` | Aegis 离线部署包构建技能 - Docker 镜像导出、数据库初始化、一键启动脚本、MinIO Agent 预置 | Version release, offline deployment package |
| `aegis-software-designer` | Aegis Software Designer skill - 设计驱动开发工作流 | Full development workflow |

### System Skills
| Skill Name | Description | Use Case |
|------------|-------------|----------|
| `code-review` | Code review — local uncommitted changes or GitHub PR | Code quality and security review |
| `verify` | Verify that a code change actually does what it's supposed to | Change verification |

## Feature Development Workflow

1. **[Analysis]** Analyze requirement, project structure, related documentation, code implementation, APIs, data flow, and impact scope
2. **[Design]** Write or update design document
3. **[Approval]** User reviews and confirms the design document
4. **[Test Case Design]** Write test cases and acceptance criteria based on the design document, but do not execute tests
5. **[Implementation Plan]** Break down implementation tasks based on the design document and test cases
6. **[Implementation]** Implement the full code required by the task according to the confirmed design document
7. **[Self Check]** Check implementation completeness against the design document and test cases
8. **[Test Execution - /aegis-build-test]** Execute tests according to the test cases written in step 4
9. **[Build Verification - /aegis-build-test]** Run project build verification
10. **[Code Review - /code-review]** Perform code review
11. **[Documentation]** Update related documentation

## Bug Fix Workflow

1. **[Analysis]** Analyze bug symptoms, logs, reproduction path, and impact scope
2. **[Reproduce]** Reproduce the bug and record stable reproduction steps
3. **[Root Cause]** Identify root cause and locate related components, APIs, data flow, or configuration
4. **[Fix Design]** Write or update bug fix design document
5. **[Regression Test Case Design]** Write regression test cases based on root cause and fix design, but do not execute tests
6. **[Implementation Plan]** Break down fix tasks based on fix design and regression test cases
7. **[Fix Implementation]** Implement the full fix code required by the bug according to the confirmed fix design
8. **[Self Check]** Check fix completeness against the fix design and regression test cases
9. **[Regression Test Execution - /aegis-build-test]** Execute regression tests according to the test cases written in step 5
10. **[Build Verification - /aegis-build-test]** Run project build verification
11. **[Code Review - /code-review]** Perform code review
12. **[Bug Fix Doc]** Create bug fix document in `docs/aegis_system_design_v{version}/fix/`
13. **[Documentation]** Update related documentation

## Documentation Rules

### Design Document Path Pattern
- `docs/aegis_system_design_v{version}/{feature}_design_v{version}.md`

### Bug Fix Document Path Pattern
- `docs/aegis_system_design_v{version}/fix/{bug_description}_fix.md`

### API Document Path
- `docs/aegis_system_design_v{version}/api_grpc_design_v{version}.md`
- `proto/*.proto` (gRPC protocol definitions)

### Architecture Document Path
- `docs/aegis_system_design_v{version}/overall_architecture_design_v{version}.md`

### Changelog Path
- `docs/aegis_system_design_v{version}/CHANGELOG_v{version}.md`

### Required Design Document Sections
- Problem statement and requirements
- Current behavior
- Proposed behavior
- Component design
- Data flow
- Interface changes
- API contract changes (if applicable)
- Database schema changes (if applicable)
- Configuration changes (if applicable)
- Security impact
- Compatibility impact
- Test case design
- Rollback plan

### Required Bug Fix Document Sections
- Bug description and symptoms
- Reproduction steps
- Root cause analysis
- Fix design
- Code changes
- Verification steps
- Affected components
- Risk and rollback plan

### Documentation Update Timing
- New feature: Create design document
- Bug fix: Create fix document
- API change: Update API document
- Architecture change: Update architecture document
- Version release: Update changelog

## Frontend Development Rule

When writing or modifying frontend code (Vue 3 / TypeScript / CSS), you **MUST** invoke the `ui-ux-pro-max` skill to ensure high-quality UI/UX design and implementation standards.

## Forbidden Actions

1. **No work without a task plan** — Must create TodoWrite task plan before ANY work begins
2. **No code before project analysis** — Must understand project structure and requirements first
3. **No code before design document** — Must have design document first
4. **No code before user confirms design** — Unless user explicitly allows autonomous execution
5. **No test execution immediately after test case design** — Test execution must happen after implementation
6. **No implementation before test cases and acceptance criteria are written** — Must have test cases first
7. **No incomplete implementation when the design requires full-scope changes** — Must implement completely
8. **No fake tests** — All tests must be real, runnable tests with actual assertions
9. **No pseudocode implementation** — All code must be real, compilable, executable code
10. **No unresolved placeholders in final generated skill** — Must be explicit or marked as "User-confirmed TODO"
11. **No predefined skill recommendations** — Skills must be discovered from project scan and confirmed by user
12. **No invented skill names** — Skills must actually exist
13. **No hardcoded downstream skill names** — Unless discovered and confirmed by the user or manually provided by the user
14. **No inserting optional skill placeholders into the final generated skill** — Must be confirmed skill or no skill
15. **No skipping test execution after implementation** — Must execute tests
16. **No skipping build verification if the user confirmed a build verification step** — Must use `aegis-build-test` skill
17. **No skipping code review if the user confirmed a code review step** — Must use `code-review` skill
18. **No bug fix without bug fix documentation** — Must create fix document in `docs/aegis_system_design_v{version}/fix/`
19. **No unrelated file changes** — Only modify files related to the current task
20. **No silently ignoring failed builds, failed tests, or failed reviews** — Must handle failures
21. **No marking task complete with unresolved TODOs** — Must resolve all TODOs
22. **No generating the final project-specific skill before all stages are confirmed** — Must wait for all confirmations

## Definition of Done

1. Task plan completed
2. Project-related docs and code analyzed
3. Design document created or updated
4. Design confirmed by user, unless autonomous mode is explicitly enabled
5. Test cases and acceptance criteria written before implementation
6. Full code implemented according to the confirmed design document
7. Self-check completed against design and test cases
8. Tests executed according to the previously written test cases
9. Build verification completed if configured
10. Code review completed if configured
11. Bug fix document created if this is a bug fix and bug fix docs are configured
12. Related documentation updated
13. Known risks, limitations, and rollback plan documented
14. No unresolved TODOs remain unless explicitly accepted by the user

## Quick Reference

```
User Request
    ↓
[Create Task Plan (TodoWrite)] ← MANDATORY FIRST STEP
    ↓
[Explore subagent: analyze codebase patterns]
    ↓
[Write/Update Design Document]
    ↓
[User Approval]
    ↓
[Write Test Cases → do NOT execute]
    ↓
[Implement Code]
    ↓
[Self Check against design and test cases]
    ↓
[Execute Tests - /aegis-build-test]
    ↓
[Build Verification - /aegis-build-test]
    ↓
[Code Review - /code-review]
    ↓
[Bug Fix Doc (if bug fix)] → docs/aegis_system_design_v{version}/fix/{bug}_fix.md
    ↓
[Update Documentation]
    ↓
Done
```

## Service Ports Reference

| Service | HTTP | gRPC | Container |
|---------|------|------|-----------|
| api-server | 8082 | 19093 | aegis-api-server |
| server | - | 19090, 19094 | aegis-server |
| dc | - | 19092 | aegis-dc |
| frontend | 8081 | - | aegis-frontend |
| postgres | 5432 | - | aegis-postgres |
| redis | 6379 | - | aegis-redis |
| minio | 9000/9001 | - | aegis-minio |
| kafka | 29092 | - | aegis-kafka |

## Credentials Policy

**NEVER** hardcode credentials in skill files, source code, or documentation.

When API testing or service access requires credentials:
1. Ask the user to provide the credentials in the conversation
2. Use only the user-provided values
3. Never store credentials in files — they exist only in the conversation context
