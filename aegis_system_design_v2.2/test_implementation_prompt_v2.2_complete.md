# 测试实现提示词 - V2.2 完整版

**版本**: 2.2
**状态**: 定稿
**作者**: Manus AI

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 2.2 | 2026-03-06 | Manus AI | **全新文档**。独立的测试提示词，覆盖后端单元测试、集成测试、API 连通性测试、Agent 连通性测试、前端组件测试和端到端测试。 |

---

## 2. 角色与目标

你是一位精通 Go 测试（`testing`、`testify`、`gomock`）、Vue 3 测试（`vitest`、`@vue/test-utils`）和 API 测试（`httptest`、`Postman Collection`）的高级测试工程师。你的任务是为「自动化基线检查与自愈系统」编写全面、可执行的测试代码，确保系统的正确性、稳定性和可靠性。

---

## 3. 核心强制指令

1. **调用 `superpower` skill**：在编写任何后端或 Agent 测试代码前，必须先调用 `superpower` skill。
2. **测试覆盖率目标**：核心业务逻辑（Service 层、Repository 层）的测试覆盖率不低于 80%。
3. **测试独立性**：每个测试用例必须独立，不依赖其他测试用例的执行顺序或状态。
4. **测试数据隔离**：集成测试必须使用独立的测试数据库（通过 Docker 启动），测试结束后清理所有测试数据。
5. **Mock 优先**：单元测试必须使用 Mock 替代所有外部依赖（数据库、Redis、MinIO、LLM API），不允许在单元测试中发起真实的网络请求。
6. **提供完整文件**：每个测试文件必须提供完整内容，禁止输出代码片段或省略号。

---

## 4. 测试上下文（设计文档索引）

在编写测试前，请阅读以下文档以理解被测系统的行为规范：

| 文档名 | 描述 |
|:---|:---|
| `communication_structure_design_v2.2_complete.md` | API 接口定义，包含请求/响应格式和错误码 |
| `backend_detailed_design_v2.2_complete.md` | 后端业务逻辑，包含各模块的行为规范 |
| `database_structure_design_v2.2_complete.md` | 数据库表结构，用于集成测试的数据准备 |
| `agent_detailed_design_v2.2_complete.md` | Agent 模块设计，用于 Agent 单元测试 |

---

## 第一部分：后端单元测试

### 测试任务 1.1：LLM 模块单元测试

> **调用 `superpower` skill 后再开始编写。**

**文件路径**：`backend/internal/llm/`

**测试文件**：

1. **`validator_test.go`**：测试三层连通性校验逻辑。
   - `TestValidate_FormatCheck_EmptyAPIKey`：API Key 为空时，第一层校验应返回错误，错误信息包含 `api_key`。
   - `TestValidate_FormatCheck_InvalidBaseURL`：Base URL 格式不合法时，第一层校验应返回错误。
   - `TestValidate_FormatCheck_ValidInput`：所有格式正确时，第一层校验应通过。
   - `TestValidate_NetworkCheck_Unreachable`：使用 `httptest.NewServer` 模拟一个立即关闭的服务器，第二层校验应返回网络不可达错误。
   - `TestValidate_ModelCheck_InvalidAPIKey`：使用 `httptest.NewServer` 模拟返回 401 的服务器，第三层校验应返回认证失败错误。
   - `TestValidate_ModelCheck_Success`：使用 `httptest.NewServer` 模拟返回正常响应的服务器，三层校验均应通过。

2. **`parser_test.go`**：测试 LLM 返回结果解析逻辑。
   - `TestParseRules_ValidJSON`：输入包含合法 JSON 的 LLM 响应，应成功解析出规则列表。
   - `TestParseRules_JSONInMarkdownBlock`：输入被 Markdown 代码块（` ```json ... ``` `）包裹的 JSON，应成功提取并解析。
   - `TestParseRules_InvalidJSON`：输入无法解析的文本，应返回错误。
   - `TestParseRules_Deduplication`：输入包含重复标题的规则，应去重后返回唯一规则列表。
   - `TestParseRules_MissingRequiredFields`：输入缺少必填字段（如 `title`）的规则，应过滤该条规则并记录警告。

3. **`client_test.go`**：测试 LLM 客户端重试逻辑。
   - `TestChat_RetryOn429`：使用 `httptest.NewServer` 模拟前两次返回 429、第三次返回 200 的服务器，客户端应在重试后成功返回结果。
   - `TestChat_MaxRetriesExceeded`：使用 `httptest.NewServer` 模拟始终返回 500 的服务器，客户端应在达到最大重试次数后返回错误。
   - `TestChat_Timeout`：使用 `httptest.NewServer` 模拟响应延迟超过超时时间的服务器，客户端应返回超时错误。

---

### 测试任务 1.2：文件解析模块单元测试

> **调用 `superpower` skill 后再开始编写。**

**文件路径**：`backend/internal/fileparser/`

**测试文件**：

1. **`parser_test.go`**：测试各类型文件解析器。
   - `TestNewParser_UnsupportedType`：传入不支持的 MIME 类型，工厂函数应返回错误。
   - `TestTextParser_Parse`：传入包含已知内容的文本文件 Reader，应返回完整的文本内容。
   - `TestYAMLParser_Parse`：传入合法的 YAML 内容，应返回提取的文本内容。
   - `TestYAMLParser_InvalidYAML`：传入不合法的 YAML 内容，应返回解析错误。
   - `TestExcelParser_Parse`：传入包含多个 Sheet 的 Excel 文件，应返回所有 Sheet 的文本内容。

---

### 测试任务 1.3：IP 检测模块单元测试

> **调用 `superpower` skill 后再开始编写。**

**文件路径**：`backend/internal/ipdetect/`

**测试文件**：

1. **`detector_test.go`**：测试 IP 检测逻辑。
   - `TestDetectServerIP_ConfiguredIP`：当 `configuredIP` 非空时，应直接返回配置的 IP，不发起任何网络请求。
   - `TestDetectServerIP_PublicIPFallback`：当配置 IP 为空时，使用 `httptest.NewServer` 模拟 `api.ipify.org`，应返回模拟服务器返回的 IP。
   - `TestDetectServerIP_AllFailed_Fallback`：当所有公网 IP 查询均失败时，应返回通过出站连接检测或网卡枚举获得的本地 IP（非 `127.0.0.1`）。

---

### 测试任务 1.4：Service 层单元测试

> **调用 `superpower` skill 后再开始编写。**

**文件路径**：`backend/internal/service/`

使用 `gomock` 生成所有 Repository 接口的 Mock 实现。

**测试文件**：

1. **`task_service_test.go`**：测试任务编排逻辑。
   - `TestRunTask_OfflineHost`：当 Redis Mock 返回主机离线时，应直接创建状态为 `failed` 的任务日志，不调用 gRPC 下发命令。
   - `TestRunTask_OnlineHost`：当 Redis Mock 返回主机在线时，应创建状态为 `running` 的任务日志，并调用 gRPC 下发命令。
   - `TestRunTask_ScriptNotReady`：当规则的脚本状态不为 `ready` 时，应返回错误，不创建任何任务日志。
   - `TestHandleTaskResult_TriggerHealing`：当任务结果为修复脚本执行失败（退出码非 0）时，应调用 `SelfHealingService.ShouldHeal` 并触发自愈。
   - `TestHandleTaskResult_NoHealing`：当任务结果为检查脚本执行完成时，不应触发自愈。

2. **`self_healing_service_test.go`**：测试自愈修复逻辑。
   - `TestShouldHeal_FixScriptFailed`：修复脚本执行失败时，`ShouldHeal` 应返回 `true`。
   - `TestShouldHeal_CheckScript`：检查脚本执行失败时，`ShouldHeal` 应返回 `false`（检查脚本失败不触发自愈）。
   - `TestShouldHeal_AlreadyHealing`：当该任务已有进行中的自愈记录时，`ShouldHeal` 应返回 `false`（避免重复触发）。
   - `TestRunHealingLoop_SuccessOnFirstAttempt`：LLM Mock 返回有效脚本，Agent 执行成功，自愈日志状态应更新为 `success`，循环应在第一次尝试后退出。
   - `TestRunHealingLoop_MaxRetriesExceeded`：LLM Mock 始终返回有效脚本，但 Agent 执行始终失败，自愈日志状态应在 3 次尝试后更新为 `failed`。
   - `TestBuildHealingPrompt_WithHistory`：当有历史修复尝试时，构建的 Prompt 应包含之前的修复脚本和错误信息。

---

## 第二部分：后端集成测试

### 测试任务 2.1：Repository 层集成测试

> **调用 `superpower` skill 后再开始编写。**

**文件路径**：`backend/internal/repository/`

**前置条件**：使用 `testcontainers-go` 在测试时自动启动一个 PostgreSQL Docker 容器，并在测试结束后自动销毁。

**测试文件**：

1. **`host_repo_integration_test.go`**：
   - `TestHostRepo_Upsert_Insert`：插入一条新主机记录，应成功写入数据库，并能通过 `FindByID` 查询到。
   - `TestHostRepo_Upsert_Update`：对已存在的 host_id 再次 Upsert，应更新 IP 和主机名，不创建新记录。
   - `TestHostRepo_FindAll_Pagination`：插入 25 条记录，查询第 2 页（每页 10 条），应返回 10 条记录，total 为 25。
   - `TestHostRepo_FindAll_SearchQuery`：插入包含特定 IP 的记录，使用该 IP 作为搜索条件，应只返回匹配的记录。

2. **`rule_repo_integration_test.go`**：
   - `TestRuleRepo_BatchCreate_Transaction`：批量插入 10 条规则，应全部成功写入数据库。
   - `TestRuleRepo_BatchCreate_RollbackOnError`：模拟批量插入中途发生错误，事务应回滚，数据库中不应有任何新增记录。

3. **`config_repo_integration_test.go`**：
   - `TestConfigRepo_Upsert_EncryptsAPIKey`：保存 LLM 配置，数据库中存储的 API Key 应为加密后的密文，而非明文。
   - `TestConfigRepo_GetActive_MasksAPIKey`：获取 LLM 配置，返回的 API Key 应为脱敏格式（`sk-xxxx...1234`）。

---

### 测试任务 2.2：API 层集成测试

> **调用 `superpower` skill 后再开始编写。**

**文件路径**：`backend/internal/api/`

使用 `net/http/httptest` 包创建测试服务器，使用 Mock 替代所有 Service 层依赖。

**测试文件**：

1. **`config_handler_test.go`**：
   - `TestGetLLMConfig_Success`：Mock `ConfigService.GetActive()` 返回配置，`GET /api/v1/config/llm` 应返回 200 和脱敏后的配置数据。
   - `TestSaveLLMConfig_Success`：发送合法的配置数据，`POST /api/v1/config/llm` 应返回 200。
   - `TestSaveLLMConfig_InvalidBody`：发送格式错误的请求体，应返回 400 和错误信息。
   - `TestTestLLMConfig_Success`：Mock `LLMValidator.Validate()` 返回成功，`POST /api/v1/config/llm/test` 应返回 200 和三层校验通过的详细信息。
   - `TestTestLLMConfig_NetworkError`：Mock `LLMValidator.Validate()` 返回网络错误，应返回 200（业务层成功）但响应体中 `success` 为 `false`，`message` 包含具体错误原因。

2. **`host_handler_test.go`**：
   - `TestGetHosts_Success`：Mock `HostRepo.FindAll()` 返回主机列表，Mock `RedisClient.BatchCheckOnline()` 返回在线状态，`GET /api/v1/hosts` 应返回 200 和包含在线状态的主机列表。
   - `TestGetHosts_Pagination`：传入 `page=2&page_size=10` 参数，应将正确的分页参数传递给 Mock。
   - `TestGetHosts_InvalidPageParam`：传入非数字的 `page` 参数，应返回 400。

3. **`template_handler_test.go`**：
   - `TestUploadTemplate_Success`：上传合法的文件，`POST /api/v1/templates/upload` 应返回 201 和新创建的模板信息。
   - `TestUploadTemplate_UnsupportedFileType`：上传不支持的文件类型（如 `.exe`），应返回 400 和错误信息。
   - `TestUploadTemplate_FileTooLarge`：上传超过 50MB 的文件，应返回 400 和文件大小超限的错误信息。
   - `TestGetTemplateStatus_FromRedis`：Mock `RedisClient.GetParseStatus()` 返回 `processing`，`GET /api/v1/templates/:id/status` 应返回 200 和状态信息。

4. **`task_handler_test.go`**：
   - `TestRunCheck_Success`：发送合法的任务下发请求，Mock `TaskService.RunTask()` 返回 task_group_id，应返回 202 和 task_group_id。
   - `TestRunCheck_EmptyHostIDs`：发送空的 `host_ids` 列表，应返回 400。
   - `TestGetTaskLogs_WithOffset`：传入 `offset=5` 参数，应将正确的 offset 传递给 Mock `RedisClient.GetTaskLogs()`。

5. **`agent_handler_test.go`**：
   - `TestGetInstallCommand_ReturnsCorrectIP`：使用已知 IP 初始化 `AgentHandler`，`GET /api/v1/agent/install-command` 应返回包含该 IP 的安装命令。
   - `TestGetInstallScript_ContainsServerAddr`：`GET /api/v1/agent/install.sh` 应返回 `text/plain` 内容，且脚本中包含正确的 `SERVER_ADDR` 和 `GRPC_ADDR`。

---

## 第三部分：Agent 单元测试

### 测试任务 3.1：Agent 配置模块测试

> **调用 `superpower` skill 后再开始编写。**

**文件路径**：`agent/internal/config/`

**测试文件**：

1. **`config_test.go`**：
   - `TestLoadConfig_ValidFile`：使用 `os.CreateTemp` 创建包含合法 TOML 内容的临时文件，`LoadConfig` 应成功解析并返回正确的配置。
   - `TestLoadConfig_FileNotFound`：传入不存在的文件路径，`LoadConfig` 应返回错误。
   - `TestLoadConfig_GeneratesHostID`：使用 `HostID` 为空的 TOML 文件，`LoadConfig` 应自动生成 UUID 并回写到文件，再次调用 `LoadConfig` 时应返回相同的 UUID。

---

### 测试任务 3.2：Agent 命令执行模块测试

> **调用 `superpower` skill 后再开始编写。**

**文件路径**：`agent/internal/executor/`

**测试文件**：

1. **`executor_test.go`**：
   - `TestExecuteCommand_Success`：执行一个简单的 `echo "hello"` 脚本，应返回退出码 0，stdout 包含 `hello`，stderr 为空。
   - `TestExecuteCommand_ScriptFailure`：执行一个 `exit 1` 脚本，应返回退出码 1。
   - `TestExecuteCommand_Timeout`：执行一个 `sleep 100` 脚本，设置 1 秒超时，应在超时后返回超时错误，退出码为非 0。
   - `TestExecuteCommand_CleansUpTempFiles`：执行任意脚本后，临时目录 `/tmp/aegis-agent/{taskID}/` 应被清理，不留下任何文件。
   - `TestExecuteCommand_ConcurrencyLimit`：同时启动 5 个执行任务（并发限制为 2），应确保同一时刻最多只有 2 个脚本在执行（通过计数器验证）。

---

### 测试任务 3.3：Agent 资产收集模块测试

> **调用 `superpower` skill 后再开始编写。**

**文件路径**：`agent/internal/asset/`

**测试文件**：

1. **`collector_test.go`**：
   - `TestCollect_ReturnsValidInfo`：调用 `Collect()` 函数，返回的 `IPAddress` 应为合法的 IPv4 地址（非 `127.0.0.1`），`Hostname` 非空，`OSType` 为 `linux`。
   - `TestCollect_IPNotLoopback`：返回的 `IPAddress` 不应为回环地址（`127.x.x.x`）。

---

## 第四部分：连通性测试

### 测试任务 4.1：基础设施连通性测试脚本

> **调用 `superpower` skill 后再开始编写。**

**文件路径**：`scripts/test_connectivity.sh`

编写一个 Bash 脚本，用于在部署后验证所有基础设施组件的连通性。

**脚本功能**：

```bash
#!/bin/bash
# 基础设施连通性测试脚本
# 用法: ./test_connectivity.sh [--host <backend_host>] [--pg-port <port>] [--redis-port <port>] [--minio-port <port>]

# 默认参数
BACKEND_HOST="${BACKEND_HOST:-localhost}"
PG_HOST="${PG_HOST:-localhost}"
PG_PORT="${PG_PORT:-5432}"
PG_USER="${PG_USER:-aegis_user}"
PG_DB="${PG_DB:-aegis_db}"
REDIS_HOST="${REDIS_HOST:-localhost}"
REDIS_PORT="${REDIS_PORT:-6379}"
MINIO_HOST="${MINIO_HOST:-localhost}"
MINIO_PORT="${MINIO_PORT:-9000}"
BACKEND_HTTP_PORT="${BACKEND_HTTP_PORT:-8080}"
BACKEND_GRPC_PORT="${BACKEND_GRPC_PORT:-9090}"
```

脚本必须包含以下检查项，每项检查结果以 `[PASS]` 或 `[FAIL]` 标注：

1. **PostgreSQL 连通性**：使用 `pg_isready` 或 `psql` 命令检查 PostgreSQL 是否可达，并验证能否成功连接到目标数据库。
2. **Redis 连通性**：使用 `redis-cli ping` 命令检查 Redis 是否返回 `PONG`。
3. **MinIO 连通性**：使用 `curl -sf http://{MINIO_HOST}:{MINIO_PORT}/minio/health/live` 检查 MinIO 健康状态。
4. **后端 HTTP 服务连通性**：使用 `curl -sf http://{BACKEND_HOST}:{BACKEND_HTTP_PORT}/health` 检查后端 HTTP 服务是否正常。
5. **后端 gRPC 端口连通性**：使用 `nc -z` 或 `curl` 检查 gRPC 端口是否监听。
6. **后端 API 基本功能**：调用 `GET /api/v1/config/llm` 接口，检查是否返回 200 状态码。
7. **Agent 安装命令接口**：调用 `GET /api/v1/agent/install-command` 接口，检查返回的 `server_ip` 是否为非 `127.0.0.1` 的有效 IP。

脚本最后汇总所有检查结果，如果有任何 `[FAIL]` 项，以退出码 1 退出；全部通过则以退出码 0 退出。

---

### 测试任务 4.2：LLM 连通性测试脚本

> **调用 `superpower` skill 后再开始编写。**

**文件路径**：`scripts/test_llm_connectivity.sh`

编写一个 Bash 脚本，用于快速验证 LLM API 的连通性，无需启动后端服务。

**脚本功能**：

```bash
#!/bin/bash
# LLM 连通性测试脚本
# 用法: ./test_llm_connectivity.sh --api-key <key> --base-url <url> --model <model>
```

脚本必须包含以下检查项：

1. **参数校验**：检查 `--api-key`、`--base-url`、`--model` 参数是否均已提供，否则打印使用说明并退出。
2. **URL 格式校验**：检查 `base-url` 是否以 `http://` 或 `https://` 开头。
3. **网络连通性测试**：使用 `curl -sf --max-time 5 {base-url}` 检查服务器是否可达（允许 4xx 响应，只要不是连接超时）。
4. **API Key 有效性测试**：向 `{base-url}/v1/chat/completions` 发送最小化的 Chat Completion 请求，检查是否返回 200，并打印 token 使用量。
5. **模型可用性测试**：检查响应中的 `model` 字段是否与请求的模型名称一致。

---

### 测试任务 4.3：Agent 连通性测试脚本

> **调用 `superpower` skill 后再开始编写。**

**文件路径**：`scripts/test_agent_connectivity.sh`

编写一个 Bash 脚本，用于在 Agent 安装后验证 Agent 与后端的连通性。

**脚本功能**：

```bash
#!/bin/bash
# Agent 连通性测试脚本
# 用法: ./test_agent_connectivity.sh [--backend-host <host>] [--grpc-port <port>]
```

脚本必须包含以下检查项：

1. **Agent 二进制文件检查**：检查 `/usr/local/bin/aegis-agent` 是否存在且可执行。
2. **Agent 配置文件检查**：检查 `/etc/aegis-agent/config.toml` 是否存在，并验证 `ServerAddr` 和 `HostID` 字段非空。
3. **Systemd 服务状态检查**：使用 `systemctl is-active aegis-agent` 检查 Agent 服务是否处于 `active` 状态。
4. **gRPC 端口连通性检查**：使用 `nc -z` 检查后端 gRPC 端口是否可达。
5. **Agent 日志检查**：读取 `/opt/baseline/logs/agent/agent.log` 的最后 20 行，检查是否包含 `已启动` 或 `connected` 等成功连接的关键字，检查是否包含 `Fatal` 或 `连接失败` 等错误关键字。
6. **后端主机注册验证**：调用后端 `GET /api/v1/hosts` 接口，检查当前主机（通过 `hostname` 命令获取）是否出现在主机列表中，且在线状态为 `true`。

---

## 第五部分：前端组件测试

### 测试任务 5.1：前端组件单元测试

> **调用 `ui-ux-pro-max` skill 后再开始编写。**

**文件路径**：`frontend/src/`

使用 `vitest` + `@vue/test-utils` 编写前端组件测试。

**测试文件**：

1. **`components/base/BaseButton.test.ts`**：
   - `renders correctly`：渲染组件，检查按钮文本和样式是否正确。
   - `throttles click events`：在 300ms 内连续点击两次，`click` 事件只应触发一次。
   - `shows loading state`：传入 `loading: true` prop，按钮应显示加载状态且不可点击。
   - `is disabled when disabled prop is true`：传入 `disabled: true` prop，按钮应不可点击。

2. **`components/common/LogTerminal.test.ts`**：
   - `renders log entries`：传入包含 stdout 和 stderr 的日志数组，应正确渲染所有日志条目。
   - `applies different colors for stdout and stderr`：stdout 条目应有白色样式，stderr 条目应有红色样式。
   - `auto-scrolls to bottom on new logs`：新增日志条目时，容器应自动滚动到底部。
   - `clears logs when clear is called`：调用清空方法后，日志列表应为空。

3. **`store/config.test.ts`**：
   - `fetchConfig calls API and updates state`：Mock API 返回配置数据，调用 `fetchConfig()` 后，store 的 `config` state 应更新为返回的数据。
   - `testConfig sets status to success on pass`：Mock API 返回测试成功，调用 `testConfig()` 后，store 的 `status` 应为 `success`。
   - `testConfig sets status to error on fail`：Mock API 返回测试失败，调用 `testConfig()` 后，store 的 `status` 应为 `error`。

4. **`store/tasks.test.ts`**：
   - `uploadTemplate calls API and refreshes templates`：Mock 文件上传 API 成功，`uploadTemplate()` 后应自动调用 `getTemplates()` 刷新列表。
   - `pollLogs stops when task is complete`：Mock `getTaskLogs()` 在第二次调用时返回包含 `status: completed` 的响应，轮询应在第二次调用后停止。
   - `stopPolling clears the timer`：调用 `stopPolling()` 后，轮询定时器应被清除，不再发起新的 API 请求。

---

## 第六部分：端到端测试（E2E）

### 测试任务 6.1：核心业务流程 E2E 测试

> **调用 `superpower` skill 后再开始编写。**

**文件路径**：`e2e/`

**前置条件**：所有服务（后端、数据库、Redis、MinIO）必须已启动。使用真实的 LLM API（需要配置 `E2E_LLM_API_KEY` 环境变量）。

编写 Go 语言的 E2E 测试文件（`e2e/e2e_test.go`），使用 `net/http` 客户端直接调用后端 API，验证以下核心业务流程：

1. **`TestE2E_LLMConfigAndTest`**：
   - 调用 `POST /api/v1/config/llm` 保存 LLM 配置（使用环境变量中的 API Key）。
   - 调用 `POST /api/v1/config/llm/test` 测试连通性，断言返回 `success: true`。

2. **`TestE2E_TemplateUploadAndParse`**：
   - 上传一个预先准备好的测试基线文档（`e2e/testdata/sample_baseline.txt`）。
   - 轮询 `GET /api/v1/templates/{id}/status`，等待状态变为 `completed`（超时 120 秒）。
   - 调用 `GET /api/v1/templates/{id}/rules`，断言规则列表非空，且每条规则包含 `title`、`check_content` 字段。

3. **`TestE2E_AgentInstallCommand`**：
   - 调用 `GET /api/v1/agent/install-command`，断言返回的 `server_ip` 为非 `127.0.0.1` 的有效 IP。
   - 断言返回的 `command` 包含该 IP 地址。
   - 调用 `GET /api/v1/agent/install.sh`，断言响应 Content-Type 为 `text/plain`，且脚本内容包含 `SERVER_ADDR` 的实际值。

4. **`TestE2E_HostListWithOnlineStatus`**（需要 Agent 已注册）：
   - 调用 `GET /api/v1/hosts`，断言主机列表非空。
   - 断言至少有一台主机的 `online` 字段为 `true`。

---

请等待我的指示，我们将一步一步完成测试的编写。现在，请从 **测试任务 1.1** 开始。
