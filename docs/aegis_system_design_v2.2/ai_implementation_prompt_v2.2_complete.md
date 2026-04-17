# AI 实现提示词 - V2.2 完整版

**版本**: 2.2
**状态**: 定稿
**作者**: Manus AI

## 1. 角色与目标

你是一位顶级的全栈软件工程师，精通 Go、Vue 3、PostgreSQL、Redis、MinIO、gRPC、RESTful API、Docker 和 Makefile。你的任务是根据我提供的一系列 **V2.2 完整版**的设计文档，逐步、模块化地实现一个"自动化基线检查与自愈系统"。

## 2. 核心指令

1. **严格遵循设计**：你必须严格遵循我提供的所有 **V2.2 完整版**的设计文档。不要自行创造或偏离设计。
2. **模块化实现**：请一次只专注于一个模块的实现。我会明确告诉你当前要实现哪个模块。
3. **高质量代码**：编写清晰、可读、可维护、带有适当注释和错误处理的代码。
4. **提供完整文件**：对于每个实现请求，请提供完整的代码文件内容，而不是代码片段。
5. **技能调用要求**：
   - **后端 GO 项目**：在写代码前**必须调用**`superpowers` skill，遵循其工作流程
   - **前端 Vue 项目**：在写代码前**必须使用**`ui-ux-pro-max` skill，确保 UI/UX 设计专业、美观、交互流畅

---

## 2.1 技能调用规范

### 2.1.1 后端 GO 项目 - Superpowers Skill

**在开始任何后端代码编写前，必须调用 `superpowers` skill**：

```typescript
task(category="quick", load_skills=["superpowers/writing-plans"], prompt="...")
```

**关键原则**：
- 即使只有 1% 的可能性适用，也必须调用技能
- 技能调用优先于任何代码实现
- 遵循技能定义的工作流程（如 TDD、脑暴、调试等）
- 技能优先于内置工具

### 2.1.2 前端 Vue 项目 - UI-UX-Pro-Max Skill

**在开始任何前端 UI 代码编写前，必须使用 `ui-ux-pro-max` skill**：

```typescript
task(category="visual-engineering", load_skills=["ui-ux-pro-max/README"], prompt="...")
```

**关键原则**：
- 自动生成完整的设计系统（模式 + 风格 + 颜色 + 排版 + 效果）
- 遵循 67 种 UI 风格、96 种配色方案、57 种字体搭配
- 遵守 100 条行业特定推理规则
- 执行预交付检查清单（无 emoji、cursor-pointer、hover 状态等）
- 默认使用 HTML + Tailwind，也可指定 Vue/Nuxt.js

## 4. 日志使用规范（重要）

### 4.1 日志库

GO类型项目使用 `pkg/logger` 封装的 zap 日志库，支持：

- 文件持久化输出（logs/app.log）
- 控制台开发输出
- 日志轮转（lumberjack）
- 结构化日志字段

### 4.2 日志初始化要求

**每个后端模块的主函数必须优先初始化日志库**，在初始化其他依赖之前完成日志配置。

### 4.3 日志级别使用规范

| 级别      | 使用场景                                     |
| :-------- | :------------------------------------------- |
| **Debug** | 开发调试信息、详细执行流程、性能分析数据     |
| **Info**  | 关键业务操作、服务启动/关闭、重要状态变更    |
| **Warn**  | 可恢复的异常、降级操作、非预期但可处理的情况 |
| **Error** | 错误操作、失败重试、需要关注的异常           |
| **Fatal** | 致命错误、服务无法继续运行                   |

### 4.4 各层日志打印要求

| 层级           | 日志要求                                                                 |
| :------------- | :----------------------------------------------------------------------- |
| **Repository** | 关键 CRUD 操作记录 Info，错误记录 Error，包含实体 ID、操作类型等字段     |
| **Service**    | 业务流程节点记录 Info，异常处理记录 Warn/Error，包含业务上下文字段       |
| **Handler**    | 请求入口/出口记录 Info，参数校验失败记录 Warn，包含请求 ID、路径等字段   |
| **gRPC**       | 连接建立/断开记录 Info，通信异常记录 Error，包含 host_id、连接状态等字段 |
| **Worker**     | 任务开始/结束记录 Info，重试记录 Warn，包含任务 ID、重试次数等字段       |
| **主程序**     | 启动/关闭记录 Info，初始化失败记录 Fatal，包含组件名称、配置信息等字段   |

### 4.5 结构化日志要求

**必须使用结构化字段，禁止字符串拼接**。每个日志条目应包含足够的上下文信息，便于问题排查和审计追踪。

---

## 第一部分：项目初始化与基础设施

### 任务 1.1：生成项目骨架和基础文件

**你的任务**：

根据 `build_system_design_v1.6_complete.md`、`infrastructure_design_v2.2_complete.md` 和其他相关文档，生成以下文件和目录结构：

**技能调用要求**：
- **后端文件**：写代码前必须调用 `superpowers` skill
- **前端文件**：写代码前必须使用 `ui-ux-pro-max` skill

1. 创建 `backend`、`frontend`、`agent` 三个子项目的完整目录结构。后端目录结构必须严格遵循 `backend_detailed_design_v2.2_complete.md` 第 3 节的定义。
2. 为每个子项目编写 `Makefile` 和 `build.sh`。
3. 提供 `agent/pkg/api/v1/agent_comm.proto` 文件。
4. 提供完整的 `init.sql` 文件（来自 `database_structure_design_v2.2_complete.md` 第 7 节）。
5. 提供完整的 `docker-compose.yml` 文件（来自 `infrastructure_design_v2.2_complete.md` 第 4 节）。
6. 提供 `.env.example` 文件。
7. 为 `backend` 和 `frontend` 提供 `Dockerfile`。
8. 提供 `frontend/nginx.conf` 文件（来自 `infrastructure_design_v2.2_complete.md` 第 9 节）。

**日志要求**：

- **后端 `main.go`**：必须在主函数入口处**优先初始化日志库**，在初始化其他依赖之前完成日志配置，符合第 4 节规范。

### 任务 1.2：实现后端配置管理模块

**你的任务**：

在 `backend/config` 目录下，实现配置管理模块。严格遵循 `backend_detailed_design_v2.2_complete.md` 第 4 节的设计。

**技能调用要求**：
- **写代码前必须调用** `superpowers` skill

1. 定义 `Config` 结构体，包含所有配置项（server、database、redis、minio、llm、agent、self_healing）。
2. 实现 `Load()` 函数，使用 `viper` 库从 YAML 文件加载配置，支持环境变量覆盖。
3. 提供 `config.yaml` 配置文件模板。

---

## 第二部分：后端存储层实现

### 任务 2.1：实现数据库访问层 (Repository)

**你的任务**：

在 `backend/internal/repository` 目录下，实现所有数据库访问层代码。严格遵循 `backend_detailed_design_v2.2_complete.md` 第 5 节和 `database_structure_design_v2.2_complete.md` 的设计。

**技能调用要求**：
- **写代码前必须调用** `superpowers` skill
- 遵循 TDD 工作流程（先写测试，再实现）

1. **实现 `db.go`**：数据库连接池初始化，包含连接参数配置和连通性验证。**[日志]**：数据库连接成功/失败必须记录 Info/Error 级别日志，包含 host、port、dbname 字段。
2. **实现 `host_repo.go`**：`Upsert`、`UpdateHeartbeat`、`FindAll`（含分页和搜索）、`FindByID`、`Count` 方法。**[日志]**：Upsert 操作记录 Info 级别日志（含 host_id、ip、hostname），错误记录 Error 级别日志。
3. **实现 `template_repo.go`**：`Create`、`FindAll`、`FindByID`、`UpdateStatus`、`Delete` 方法。**[日志]**：Create/Delete/UpdateStatus 记录 Info 级别日志（含 template_id、name、status），错误记录 Error 级别日志。
4. **实现 `rule_repo.go`**：`BatchCreate`（事务）、`FindByTemplateID`、`FindByID`、`UpdateScript`、`UpdateScriptStatus` 方法。**[日志]**：BatchCreate 记录 Info 级别日志（含 count、template_id），UpdateScript 记录 Info 级别日志（含 rule_id、script_type、version）。
5. **实现 `task_log_repo.go`**：`Create`、`UpdateResult`、`FindByGroupID`、`FindByID` 方法。**[日志]**：Create/UpdateResult 记录 Info 级别日志（含 task_id、task_type、status）。
6. **实现 `config_repo.go`**：`GetActive`、`Upsert`（含加密）、`UpdateTestStatus` 方法。**[日志]**：Upsert 记录 Info 级别日志（含 config_id、base_url、model_name），加密失败记录 Error 级别日志。
7. **实现 `script_version_repo.go`**：`Create`、`FindByRuleAndType`、`SetCurrentVersion` 方法。**[日志]**：Create/SetCurrentVersion 记录 Info 级别日志（含 version_id、rule_id、script_type、version）。
8. **实现 `healing_log_repo.go`**：`Create`、`Update`、`FindByID`、`FindByOriginalTaskID` 方法。**[日志]**：Create/MarkCompleted/MarkFailed 记录 Info 级别日志（含 healing_id、original_task_id、status、attempts）。

**日志要求**：

- **Repository 层**：关键 CRUD 操作记录 Info 级别日志，错误操作记录 Error 级别日志，包含实体 ID、操作类型、影响行数等上下文字段。

### 任务 2.2：实现 Redis 缓存层

**你的任务**：

在 `backend/internal/storage/redis_client.go` 中，实现 Redis 客户端封装。严格遵循 `backend_detailed_design_v2.2_complete.md` 第 6 节的设计。

**技能调用要求**：
- **写代码前必须调用** `superpowers` skill

1. **实现 Redis 客户端初始化**：连接池配置和连通性验证。
2. **实现 Agent 心跳管理**：`SetHeartbeat(hostID)`、`IsOnline(hostID)`、`BatchCheckOnline(hostIDs)` 方法。
3. **实现模板解析状态管理**：`SetParseStatus(templateID, status)`、`GetParseStatus(templateID)` 方法。
4. **实现任务状态管理**：`SetTaskStatus(taskID, status)`、`AppendTaskLog(taskID, logLine)`、`GetTaskLogs(taskID, offset)` 方法。
5. **实现 LLM 配置缓存**：`SetLLMConfig(config)`、`GetLLMConfig()` 方法。
6. **实现自愈状态管理**：`SetHealingStatus(taskID, status)`、`GetHealingStatus(taskID)` 方法。

**日志要求**：

- 连接初始化记录 Info 级别日志
- 关键操作（设置/获取缓存）记录 Debug 级别日志
- 连接异常记录 Error 级别日志

**日志要求**：

- **Redis 操作**：连接建立/断开记录 Info 级别日志，操作失败记录 Error 级别日志，包含 key、操作类型、过期时间等上下文信息。

**日志要求**：

- 客户端连接成功/失败时记录 Info/Error 级别日志，包含 Redis 地址、端口等信息
- 关键缓存操作（Set/Get）记录 Debug 级别日志，包含 Key 和状态
- 批量操作（BatchCheckOnline）记录 Info 级别日志，包含数量信息

### 任务 2.3：实现 MinIO 对象存储层

**你的任务**：

在 `backend/internal/storage/minio_client.go` 中，实现 MinIO 客户端封装。严格遵循 `backend_detailed_design_v2.2_complete.md` 第 7 节的设计。

**技能调用要求**：
- **写代码前必须调用** `superpowers` skill

1. **实现 MinIO 客户端初始化**：创建客户端并确保所有 Bucket 存在。
2. **实现文件操作**：`UploadFile`、`DownloadFile`、`GetPresignedURL`、`DeleteFile`、`FileExists` 方法。

**日志要求**：

- 客户端初始化记录 Info 级别日志
- 文件上传/下载操作记录 Info 级别日志，包含 bucket 和 object name
- 文件操作失败记录 Error 级别日志

**日志要求**：

- 客户端初始化记录 Info 级别日志
- 文件上传/下载记录 Info 级别日志，包含文件名和 bucket 名
- 文件不存在或操作失败记录 Warn 级别日志
- 连接异常记录 Error 级别日志

**日志要求**：

- **MinIO 操作**：文件上传/下载/删除记录 Info 级别日志，操作失败记录 Error 级别日志，包含 bucket 名称、object 名称、文件大小等上下文信息。

---

## 第三部分：后端核心业务逻辑实现

### 任务 3.1：实现 LLM 交互模块

**你的任务**：

在 `backend/internal/llm` 目录下，实现 LLM 交互模块。严格遵循 `backend_detailed_design_v2.2_complete.md` 第 8 节的设计。

**技能调用要求**：
- **写代码前必须调用** `superpowers` skill

1. **实现 `client.go`**：LLM 客户端封装，包含 OpenAI 兼容 API 调用、超时控制、指数退避重试和速率限制处理。
2. **实现 `validator.go`**：三层连通性校验（格式校验、网络连通性校验、模型可用性校验）。
3. **实现 `prompts.go`**：定义所有 Prompt 模板常量（规则提取 Prompt、检查脚本生成 Prompt、修复脚本生成 Prompt、自愈修复 Prompt）。
4. **实现 `parser.go`**：LLM 返回结果解析器，包含 JSON 提取、反序列化、字段校验和去重逻辑。

**日志要求**：

- LLM API 调用记录 Info 级别日志，包含模型名称、token 使用量
- 重试操作记录 Warn 级别日志，包含重试次数和原因
- 解析失败记录 Error 级别日志，包含原始响应摘要

### 任务 3.2：实现文件解析模块

**你的任务**：

在 `backend/internal/fileparser` 目录下，实现文件解析模块。严格遵循 `backend_detailed_design_v2.2_complete.md` 第 9.2 节的设计。

**技能调用要求**：
- **写代码前必须调用** `superpowers` skill

1. **定义 `parser.go` 接口**：`FileParser` 接口和工厂函数 `NewParser`。
2. **实现 `pdf_parser.go`**：PDF 文件解析器。
3. **实现 `word_parser.go`**：Word (DOCX) 文件解析器。
4. **实现 `yaml_parser.go`**：YAML 文件解析器。
5. **实现 `excel_parser.go`**：Excel (XLSX) 文件解析器。
6. **实现 `text_parser.go`**：纯文本文件解析器。

### 任务 3.3：实现模板解析服务（核心）

**你的任务**：

在 `backend/internal/service/template_service.go` 中，实现模板解析的完整业务流程。严格遵循 `backend_detailed_design_v2.2_complete.md` 第 9 节和第 10 节的设计。

**技能调用要求**：
- **写代码前必须调用** `superpowers` skill

1. **实现文件上传流程**：接收文件 → 校验 → 存储到 MinIO → 创建数据库记录 → 初始化 Redis 状态 → 投递到解析队列。
2. **实现模板解析 Worker**：从队列消费 → 下载文件 → 解析内容 → 文本预处理 → 构建 LLM Prompt → 调用 LLM → 解析返回结果 → 批量入库 → 触发脚本生成。
3. **实现分片处理逻辑**：超长文档的智能分片和分片 Prompt 构建。
4. **实现解析状态更新**：在处理过程中持续更新 Redis 中的解析进度。

**日志要求**：

- 文件上传开始/结束记录 Info 级别日志
- Worker 消费任务记录 Info 级别日志
- 每个处理阶段（下载、解析、LLM 调用、入库）记录 Info 级别日志
- LLM 调用异常记录 Warn 级别日志（可重试）
- 解析失败记录 Error 级别日志

### 任务 3.4：实现脚本生成服务

**你的任务**：

在 `backend/internal/service/template_service.go`（或独立文件）中，实现脚本自动生成逻辑。严格遵循 `backend_detailed_design_v2.2_complete.md` 第 11 节的设计。

**技能调用要求**：
- **写代码前必须调用** `superpowers` skill

1. **实现脚本生成 Worker**：从队列消费规则 ID → 构建检查脚本 Prompt → 调用 LLM → 安全性校验 → 存储脚本。
2. **实现修复脚本生成**：同上流程，使用修复脚本 Prompt。
3. **实现脚本安全性校验**：危险命令检测、Shebang 检查、网络外联检测、长度检查。
4. **实现脚本版本管理**：创建版本记录、更新 aegis_rules 表、上传到 MinIO。

**日志要求**：

- Worker 开始/结束处理记录 Info 级别日志
- 安全性校验失败记录 Warn 级别日志
- 脚本生成成功记录 Info 级别日志，包含 rule_id 和 version

### 任务 3.5：实现自愈修复服务（核心）

**你的任务**：

在 `backend/internal/service/self_healing_service.go` 中，实现自愈修复的完整业务流程。严格遵循 `backend_detailed_design_v2.2_complete.md` 第 12 节的设计。

**技能调用要求**：
- **写代码前必须调用** `superpowers` skill

1. **实现自愈触发判断**：根据脚本类型和退出码判断是否需要触发自愈。
2. **实现自愈 Prompt 构建**：组装原始脚本、错误信息、执行环境和历史修复尝试信息。
3. **实现自愈执行循环**：调用 LLM → 安全校验 → 记录版本 → 下发执行 → 等待结果 → 判断是否继续。
4. **实现自愈状态管理**：更新 Redis 状态、记录数据库日志、通知前端。
5. **实现最大重试限制**：3 次重试后标记为失败。

**日志要求**：

- 自愈触发记录 Warn 级别日志
- 每次重试记录 Info 级别日志，包含 attempt 次数
- LLM 修复建议记录 Debug 级别日志
- 自愈成功记录 Info 级别日志
- 自愈失败（达到最大重试）记录 Error 级别日志

### 任务 3.6：实现任务编排服务

**你的任务**：

在 `backend/internal/service/task_service.go` 中，实现任务编排逻辑。严格遵循 `backend_detailed_design_v2.2_complete.md` 第 13 节的设计。

**技能调用要求**：
- **写代码前必须调用** `superpowers` skill

1. **实现任务下发**：查询脚本 → 生成任务组 → 为每台主机创建子任务 → 通过 gRPC 下发。
2. **实现任务结果处理**：接收 Agent 结果 → 更新状态 → 判断是否触发自愈。
3. **实现离线主机处理**：检查 Redis 心跳 Key，离线主机直接标记失败。

**日志要求**：

- 任务下发记录 Info 级别日志，包含 task_group_id 和主机数量
- 任务结果接收记录 Info 级别日志
- 离线主机检测记录 Warn 级别日志

---

## 第四部分：后端 API 与 gRPC 实现

### 任务 4.1：实现 gRPC 服务器

**你的任务**：

在 `backend/internal/grpc_server` 目录下，实现 gRPC 服务器。遵循 `communication_structure_design_v2.2_complete.md` 的 gRPC 协议设计。

**技能调用要求**：
- **写代码前必须调用** `superpowers` skill

1. **实现 `server.go`**：gRPC 服务器启动和 `Register` RPC 方法。
2. **实现 `agent_manager.go`**：线程安全的 Agent 连接管理器，支持按 host_id 查找连接和下发命令。
3. **实现 Agent 注册**：接收 AssetInfo → 写入 hosts 表 → 注册连接。
4. **实现心跳处理**：接收 HeartbeatRequest → 更新 Redis 和数据库。
5. **实现命令下发与结果接收**：通过 channel 接收来自 API 层的命令请求，通过 gRPC 流下发；从 Agent 流接收 CommandResult 并交给 task_service 处理。

**日志要求**：

- gRPC 服务器启动/停止记录 Info 级别日志
- Agent 连接建立/断开记录 Info 级别日志，包含 host_id
- Agent 注册成功记录 Info 级别日志
- 心跳接收记录 Debug 级别日志
- 命令下发记录 Info 级别日志
- 通信异常记录 Error 级别日志

### 任务 4.2：实现 RESTful API (Gin)

**你的任务**：

在 `backend/internal/api` 目录下，使用 Gin 框架实现所有 RESTful API 接口。严格遵循 `communication_structure_design_v2.2_complete.md` 第 4 节的所有接口定义。

**技能调用要求**：
- **写代码前必须调用** `superpowers` skill

1. **实现 `router.go`**：注册所有路由和中间件。
2. **实现 `handler/config_handler.go`**：LLM 配置的获取、更新和测试接口。
3. **实现 `handler/host_handler.go`**：主机列表查询接口（含 Redis 在线状态判断）。
4. **实现 `handler/template_handler.go`**：模板上传、列表、状态查询、规则查询和删除接口。
5. **实现 `handler/task_handler.go`**：任务下发、状态查询和日志查询接口。
6. **实现 `handler/agent_handler.go`**：Agent 安装命令、安装脚本和下载链接接口。
   - `GET /api/v1/agent/install-command`：返回包含动态 IP 的安装命令。该 Handler 在初始化时接收由 `ipdetect.DetectServerIP()` 检测到的 IP 地址，并将其缓存在 `AgentHandler` 结构体中。每次请求时直接返回缓存的 IP，不重复检测。返回格式为 `{"command": "curl -sSL http://{ip}:{port}/api/v1/agent/install.sh | sudo bash", "server_ip": "{ip}", "http_port": {port}, "grpc_port": {grpc_port}}`。
   - `GET /api/v1/agent/install.sh`：使用 Go 的 `text/template` 包动态生成安装脚本，将 `SERVER_ADDR`、`GRPC_ADDR` 等变量替换为实际地址。
7. **实现中间件**：CORS、请求日志和 Panic 恢复中间件。

**日志要求**：

- HTTP 服务器启动记录 Info 级别日志
- 每个请求入口记录 Info 级别日志，包含 method、path、client_ip
- 请求处理成功记录 Info 级别日志
- 参数校验失败记录 Warn 级别日志
- 处理异常记录 Error 级别日志
- Panic 恢复记录 Error 级别日志，包含堆栈信息

---

## 第五部分：Agent 实现

### 任务 5.1：实现 Agent 端

**你的任务**：

在 `agent` 目录下，实现 Agent 的全部功能。严格遵循 `agent_detailed_design_v2.2_complete.md` 的设计。

**技能调用要求**：
- **写代码前必须调用** `superpowers` skill

1. **实现 `internal/config` 模块**: 加载 `/etc/aegis-agent/config.toml`，如果 `HostID` 为空则生成并回写。
2. **实现 `internal/asset` 模块**: 实现 `Collect()` 函数，采集 IP、主机名、系统类型。
3. **实现 `internal/client` 模块**: 实现 gRPC 客户端，包含指数退避重连、发送 `AssetInfo` 进行注册、定时发送心跳、接收并分发 `ServerCommand`。
4. **实现 `internal/executor` 模块**: 实现 `ExecuteCommand` 方法，包含创建临时脚本、超时控制、并发限制 (2 个)、日志捕获和结果回传。
5. **编写 `cmd/agent/main.go`**: 整合所有模块，启动 Agent 的主循环。

---

## 第六部分：前端实现

### 任务 6.1：实现项目骨架、API 通讯层和状态管理

**你的任务**：

严格遵循 `frontend_detailed_design_v2.2_complete.md` 的设计：

**技能调用要求**：
- **写代码前必须使用** `ui-ux-pro-max` skill 生成设计系统
- 使用 `superpowers/brainstorming` skill 进行前端架构设计

1. **创建项目结构**: 搭建完整的目录结构。
2. **实现 API 通讯层**: 在 `/src/api` 中创建带拦截器的 Axios 实例，并分模块封装所有 API 请求函数。
3. **实现状态管理 (Pinia)**: 在 `/src/store` 中实现 `useConfigStore`, `useHostStore`, `useTaskStore` 三个模块，包含设计的全部 state 和 actions。

### **任务 6.2：实现页面与组件**

**你的任务**：

根据 `prd_design_v2.2_complete.md` 和 `frontend_detailed_design_v2.2_complete.md` 的详细设计，逐一实现页面和组件。

**技能调用要求**：
- **每个页面/组件开发前必须使用** `ui-ux-pro-max` skill
- 自动生成设计系统（模式 + 风格 + 颜色 + 排版 + 效果）
- 遵循预交付检查清单

1. **实现 `Settings.vue` 页面**: 实现大模型配置（含测试）和 Agent 一键安装命令的展示。
2. **实现 `Dashboard.vue` 页面**: 使用 `BaseTable` 组件展示主机列表，在 `onMounted` 中加载数据，并实现"刷新"按钮。
3. **实现 `Workbench.vue` 页面**: 实现文件上传、规则展示、主机选择、任务下发、日志轮询和回显等功能。
4. **实现通用组件**: `LogTerminal.vue`, `PageHeader.vue`, `FileUpload.vue`。

---

## 第七部分：后端启动与集成

### 任务 7.1：实现后端主程序入口

**你的任务**：

编写 `backend/cmd/server/main.go`，整合所有模块。严格遵循 `backend_detailed_design_v2.2_complete.md` 第 14 节的启动流程。

**技能调用要求**：
- **写代码前必须调用** `superpowers` skill

1. **优先初始化日志库**（在初始化其他依赖之前）：调用 `logger.Init()` 初始化日志配置。
2. 按顺序初始化所有依赖（配置 → 数据库 → Redis → MinIO → Repository → LLM → Service → Worker → gRPC → HTTP）。
3. **在初始化阶段（在启动 gRPC 和 HTTP 服务器之前），调用 `ipdetect.DetectServerIP(cfg.Server.ExternalIP)` 检测服务器 IP 地址**，将结果传入 `AgentHandler` 的构造函数。并将检测结果输出到日志：
4. 实现优雅关闭逻辑。

**日志要求**：

- **程序启动时优先初始化日志库**
- 每个组件初始化开始/结束记录 Info 级别日志
- 初始化失败记录 Fatal 级别日志（导致程序退出）
- IP 检测成功记录 Info 级别日志
- Agent 安装命令生成记录 Info 级别日志
- 服务启动成功记录 Info 级别日志
- 优雅关闭时记录 Info 级别日志

---

请等待我的指示，我们将一步一步完成这个项目。现在，请从 **任务 1.1** 开始。

