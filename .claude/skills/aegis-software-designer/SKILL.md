---
name: aegis-software-designer
description: Aegis Software Designer skill - design-driven development workflow with TDD, documentation-first, and mandatory build/test verification
version: 1.0.0
source: manual-creation
---

# Aegis Software Designer Skill

This skill defines the workflow and constraints for developing the Aegis platform as a **Software Designer** role. It enforces a design-first, test-driven, documentation-driven development process.

## Role: Software Designer

You are a Software Designer working on the Aegis security platform. You must follow strict engineering discipline: design docs before code, tests before implementation, and mandatory build verification.

## Knowledge Sources

- **Design documents**: `docs/` directory contains current design specs and architecture docs
- **Full codebase**: All source code in the project root
- **Local agent path**: `/opt/aegis-agent` (agent-runtime on host machines)

## Core Workflow (Mandatory Order)

```
1. Task Plan → 2. Design Document → 3. Test Cases → 4. Implementation → 5. Build & Test (aegis-build-test skill) → 6. Code Review (code-review skill) → 7. Doc Update
```

## Task Plan (Mandatory First Step)

Before ANY work begins, you MUST create a structured task plan using TodoWrite. The plan must follow this format:

### Plan Structure

```
1. [Analysis] Understand requirements and explore codebase
2. [Design] Write/update design document
3. [Tests] Write test cases (TDD red phase)
4. [Implementation] Implement code (TDD green phase)
5. [Build & Test - /aegis-build-test] Build verification + curl API testing
6. [Code Review - /code-review] Code quality and security review
7. [Documentation] Update docs (including bug fix doc if applicable)
```

### Plan Requirements

- Every task plan MUST include step 5 with `/aegis-build-test` in the step name, and step 6 with `/code-review` in the step name
- The plan MUST be created as a TodoWrite list before starting any work
- Each step must be marked as `in_progress` when active and `completed` when done
- Step names MUST explicitly include the skill invocation tag (e.g. `/aegis-build-test`, `/code-review`) so it is clear which skill to invoke

### Bug Fix Plan Additional Requirements

When the task is a **bug fix**, the plan MUST include an additional step:

```
1. [Analysis] Understand bug, reproduce issue, identify root cause
2. [Design] Write/update design document with bug analysis
3. [Tests] Write regression test cases (TDD red phase)
4. [Implementation] Fix bug (TDD green phase)
5. [Build & Test - aegis-build-test skill] Build verification + curl API testing
6. [Code Review - code-review skill] Code quality and security review
7. [Bug Fix Doc] Create bug fix document in version fix folder
8. [Documentation] Update related docs
```

**Bug fix document location**: `docs/v{version}/fix/{bug_description}_fix.md`

The bug fix document must contain:
- Bug description and symptoms
- Root cause analysis
- Fix description (what code was changed and why)
- Verification steps (how to confirm the fix works)
- Affected components

Example: `docs/v5.8/fix/login_timeout_fix.md`

### Step 1: Design Document First

Before writing ANY code:

1. Read existing design docs in `docs/` directory to understand current architecture
2. Use an Explore subagent to analyze existing codebase patterns before designing
3. Write or update the design document describing:
   - Problem statement and requirements
   - Component design (interfaces, data flow, dependencies)
   - API contract changes (if applicable)
   - Database schema changes (if applicable)
4. Get user approval on the design before proceeding

### Step 2: Test Cases Before Implementation

After design is approved:

1. Write test cases FIRST (TDD approach)
2. Test types by component:
   - **Go backend** (`api-server/`, `server/`, `dc/`, `agent/`): Go unit tests with `_test.go` files
   - **Frontend** (`frontend/`): Use `ui-ux-pro-max` skill for UI component tests
   - **API integration**: Write curl-based test commands with JWT auth token
3. Tests must define expected behavior, not implementation details
4. Run tests to confirm they FAIL (red phase) before implementing

### Step 3: Implementation

Only after tests are written:

1. Implement the minimal code to make tests pass
2. Follow existing code patterns in the project
3. Use subagents for code exploration and analysis when understanding existing patterns

### Step 5: Build & Test Verification (skill: `/aegis-build-test`)

After implementation, you MUST invoke the `aegis-build-test` skill:

```
/aegis-build-test
```

This performs:
- Component compilation (api-server, server, dc, agent, frontend)
- Docker image builds
- Service startup and health checks
- gRPC data flow verification

**Never skip this step. Never mark a task as complete without build verification.**

### Step 6: Code Review (skill: `/code-review`)

After build passes, invoke the `code-review` skill:

```
/code-review
```

This checks:
- Code quality and consistency
- Security vulnerabilities
- Performance concerns
- Adherence to project patterns

### Step 7: Documentation Update

After code review passes:

1. Update design docs to reflect actual implementation
2. If a bug was fixed, update related documentation with new information
3. Keep `docs/` directory in sync with code changes

## API Testing with curl

**IMPORTANT**: Credentials must NEVER be hardcoded in this file or any source code. Users MUST provide their credentials in the conversation prompt when API testing is needed.

Before performing any API testing, you MUST ask the user to provide:
- Username
- Password

Once provided, use them for testing:

```bash
# 1. Obtain JWT token (use credentials provided by user)
TOKEN=$(curl -s -X POST http://localhost:8082/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"<USER_PROVIDED_USERNAME>","password":"<USER_PROVIDED_PASSWORD>"}' | jq -r '.token')

# 2. Test the modified endpoint
curl -s -X <METHOD> http://localhost:8082/api/v1/<endpoint> \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d '<payload>' | jq .
```

Always test:
- Happy path (valid input)
- Error cases (invalid input, missing fields)
- Auth failures (no token, expired token)

## Agent-Runtime Development Workflow

When changes involve the agent-runtime component:

1. Make changes to the agent-runtime code at the local path `/opt/aegis-agent`
2. Push the agent-runtime changes to its repository
3. Update Aegis to import the new agent-runtime code
4. Then run the full build & test cycle

```bash
# In agent-runtime repo (e.g., /opt/aegis-agent)
git add . && git commit -m "your changes" && git push

# In Aegis repo, update dependency
cd agent
go get github.com/aegis-agent-runtime@latest
go mod tidy
```

## Subagent Usage Policy

You MUST use subagents for the following tasks:

| Task | Subagent Type | When to Use |
|------|---------------|-------------|
| Code exploration | Explore agent | Understanding existing patterns, finding related code |
| Documentation editing | General-purpose agent | Updating design docs, creating new docs |
| Code review | Code-reviewer agent | Reviewing code changes |
| Build error resolution | Build-error-resolver agent | Fixing compilation errors |
| Architecture analysis | Architect agent | Designing system components |

Do NOT use subagents for:
- Simple file reads (use Read tool directly)
- Single-line edits (use Edit tool directly)
- Running build commands (use Bash tool directly)

## Forbidden Actions

1. **No working without a plan** - Must create TodoWrite task plan before ANY work begins
2. **No fake tests** - All tests must be real, runnable tests with actual assertions
3. **No pseudocode** - All code must be real, compilable, executable code
4. **No skipping build verification** - Must use `/aegis-build-test` skill after implementation (step name must include `/aegis-build-test`)
5. **No skipping code review** - Must use `/code-review` skill before marking complete (step name must include `/code-review`)
6. **No implementing before designing** - Design doc must exist and be approved first
7. **No implementing before testing** - Test cases must be written and confirmed failing first
8. **No bug fixes without fix documentation** - Bug fixes MUST include a fix document in `docs/v{version}/fix/`

## Design Document Location

Design documents should be placed in:
```
docs/
├── v5.8/                          # Version-specific designs
│   ├── <feature>_design.md        # Feature design document
│   └── ...
├── api/                           # API documentation
└── architecture/                  # Architecture docs
```

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
[Write Test Cases → confirm RED]
    ↓
[Implement Code → confirm GREEN]
    ↓
[Build & Test - /aegis-build-test + curl API testing]
    ↓
[Code Review - /code-review]
    ↓
[Bug Fix Doc (if bug fix)] → docs/v{version}/fix/{bug}_fix.md
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
