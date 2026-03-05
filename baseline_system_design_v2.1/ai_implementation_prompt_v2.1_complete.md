# AI 实现提示词 - V2.0 完整版

**版本**: 2.0
**状态**: 定稿
**作者**: Manus AI

## 1. 角色与目标

你是一位顶级的全栈软件工程师，精通 Go、Vue 3、PostgreSQL、Redis、MinIO、gRPC、RESTful API、Docker 和 Makefile。你的任务是根据我提供的一系列 **V2.0 完整版**的设计文档，逐步、模块化地实现一个"自动化基线检查与自愈系统"。

## 2. 核心指令

1. **严格遵循设计**：你必须严格遵循我提供的所有 **V2.0 完整版**的设计文档。不要自行创造或偏离设计。
2. **模块化实现**：请一次只专注于一个模块的实现。我会明确告诉你当前要实现哪个模块。
3. **高质量代码**：编写清晰、可读、可维护、带有适当注释和错误处理的代码。
4. **提供完整文件**：对于每个实现请求，请提供完整的代码文件内容，而不是代码片段。

## 3. 实现上下文 (Context)

在开始编码前，请仔细阅读并理解以下 **V2.0 完整版**的设计文档：

| 文档名 | 描述 |
|:---|:---|
| `prd_design_v1.6_complete.md` | 产品需求文档（V1.6 保留，未修改） |
| `backend_detailed_design_v2.0_complete.md` | **后端详细设计（V2.0 全新）** |
| `infrastructure_design_v2.0_complete.md` | **基础设施与部署设计（V2.0 全新）** |
| `database_structure_design_v2.0_complete.md` | **数据库设计（V2.0 全面更新）** |
| `communication_structure_design_v2.0_complete.md` | **通讯层设计（V2.0 全面更新）** |
| `agent_detailed_design_v1.6_complete.md` | Agent 详细设计（V1.6 保留，未修改） |
| `frontend_detailed_design_v1.6_complete.md` | 前端项目详细设计（V1.6 保留，未修改） |
| `build_system_design_v1.6_complete.md` | 构建体系设计（V1.6 保留，未修改） |

---

## 第一部分：项目初始化与基础设施

### 任务 1.1：生成项目骨架和基础文件

**你的任务**：

根据 `build_system_design_v1.6_complete.md`、`infrastructure_design_v2.0_complete.md` 和其他相关文档，生成以下文件和目录结构：

1. 创建 `backend`、`frontend`、`agent` 三个子项目的完整目录结构。后端目录结构必须严格遵循 `backend_detailed_design_v2.0_complete.md` 第 3 节的定义。
2. 为每个子项目编写 `Makefile` 和 `build.sh`。
3. 提供 `agent/pkg/api/v1/agent_comm.proto` 文件。
4. 提供完整的 `init.sql` 文件（来自 `database_structure_design_v2.0_complete.md` 第 7 节）。
5. 提供完整的 `docker-compose.yml` 文件（来自 `infrastructure_design_v2.0_complete.md` 第 4 节）。
6. 提供 `.env.example` 文件。
7. 为 `backend` 和 `frontend` 提供 `Dockerfile`。
8. 提供 `frontend/nginx.conf` 文件（来自 `infrastructure_design_v2.0_complete.md` 第 9 节）。

### 任务 1.2：实现后端配置管理模块

**你的任务**：

在 `backend/config` 目录下，实现配置管理模块。严格遵循 `backend_detailed_design_v2.0_complete.md` 第 4 节的设计。

1. 定义 `Config` 结构体，包含所有配置项（server、database、redis、minio、llm、agent、self_healing）。
2. 实现 `Load()` 函数，使用 `viper` 库从 YAML 文件加载配置，支持环境变量覆盖。
3. 提供 `config.yaml` 配置文件模板。

---

## 第二部分：后端存储层实现

### 任务 2.1：实现数据库访问层 (Repository)

**你的任务**：

在 `backend/internal/repository` 目录下，实现所有数据库访问层代码。严格遵循 `backend_detailed_design_v2.0_complete.md` 第 5 节和 `database_structure_design_v2.0_complete.md` 的设计。

1. **实现 `db.go`**：数据库连接池初始化，包含连接参数配置和连通性验证。
2. **实现 `host_repo.go`**：`Upsert`、`UpdateHeartbeat`、`FindAll`（含分页和搜索）、`FindByID`、`Count` 方法。
3. **实现 `template_repo.go`**：`Create`、`FindAll`、`FindByID`、`UpdateStatus`、`Delete` 方法。
4. **实现 `rule_repo.go`**：`BatchCreate`（事务）、`FindByTemplateID`、`FindByID`、`UpdateScript`、`UpdateScriptStatus` 方法。
5. **实现 `task_log_repo.go`**：`Create`、`UpdateResult`、`FindByGroupID`、`FindByID` 方法。
6. **实现 `config_repo.go`**：`GetActive`、`Upsert`（含加密）、`UpdateTestStatus` 方法。
7. **实现 `script_version_repo.go`**：`Create`、`FindByRuleAndType`、`SetCurrentVersion` 方法。
8. **实现 `healing_log_repo.go`**：`Create`、`Update`、`FindByID`、`FindByOriginalTaskID` 方法。

### 任务 2.2：实现 Redis 缓存层

**你的任务**：

在 `backend/internal/storage/redis_client.go` 中，实现 Redis 客户端封装。严格遵循 `backend_detailed_design_v2.0_complete.md` 第 6 节的设计。

1. **实现 Redis 客户端初始化**：连接池配置和连通性验证。
2. **实现 Agent 心跳管理**：`SetHeartbeat(hostID)`、`IsOnline(hostID)`、`BatchCheckOnline(hostIDs)` 方法。
3. **实现模板解析状态管理**：`SetParseStatus(templateID, status)`、`GetParseStatus(templateID)` 方法。
4. **实现任务状态管理**：`SetTaskStatus(taskID, status)`、`AppendTaskLog(taskID, logLine)`、`GetTaskLogs(taskID, offset)` 方法。
5. **实现 LLM 配置缓存**：`SetLLMConfig(config)`、`GetLLMConfig()` 方法。
6. **实现自愈状态管理**：`SetHealingStatus(taskID, status)`、`GetHealingStatus(taskID)` 方法。

### 任务 2.3：实现 MinIO 对象存储层

**你的任务**：

在 `backend/internal/storage/minio_client.go` 中，实现 MinIO 客户端封装。严格遵循 `backend_detailed_design_v2.0_complete.md` 第 7 节的设计。

1. **实现 MinIO 客户端初始化**：创建客户端并确保所有 Bucket 存在。
2. **实现文件操作**：`UploadFile`、`DownloadFile`、`GetPresignedURL`、`DeleteFile`、`FileExists` 方法。

---

## 第三部分：后端核心业务逻辑实现

### 任务 3.1：实现 LLM 交互模块

**你的任务**：

在 `backend/internal/llm` 目录下，实现 LLM 交互模块。严格遵循 `backend_detailed_design_v2.0_complete.md` 第 8 节的设计。

1. **实现 `client.go`**：LLM 客户端封装，包含 OpenAI 兼容 API 调用、超时控制、指数退避重试和速率限制处理。
2. **实现 `validator.go`**：三层连通性校验（格式校验、网络连通性校验、模型可用性校验）。
3. **实现 `prompts.go`**：定义所有 Prompt 模板常量（规则提取 Prompt、检查脚本生成 Prompt、修复脚本生成 Prompt、自愈修复 Prompt）。
4. **实现 `parser.go`**：LLM 返回结果解析器，包含 JSON 提取、反序列化、字段校验和去重逻辑。

### 任务 3.2：实现文件解析模块

**你的任务**：

在 `backend/internal/fileparser` 目录下，实现文件解析模块。严格遵循 `backend_detailed_design_v2.0_complete.md` 第 9.2 节的设计。

1. **定义 `parser.go` 接口**：`FileParser` 接口和工厂函数 `NewParser`。
2. **实现 `pdf_parser.go`**：PDF 文件解析器。
3. **实现 `word_parser.go`**：Word (DOCX) 文件解析器。
4. **实现 `yaml_parser.go`**：YAML 文件解析器。
5. **实现 `excel_parser.go`**：Excel (XLSX) 文件解析器。
6. **实现 `text_parser.go`**：纯文本文件解析器。

### 任务 3.3：实现模板解析服务（核心）

**你的任务**：

在 `backend/internal/service/template_service.go` 中，实现模板解析的完整业务流程。严格遵循 `backend_detailed_design_v2.0_complete.md` 第 9 节和第 10 节的设计。

1. **实现文件上传流程**：接收文件 → 校验 → 存储到 MinIO → 创建数据库记录 → 初始化 Redis 状态 → 投递到解析队列。
2. **实现模板解析 Worker**：从队列消费 → 下载文件 → 解析内容 → 文本预处理 → 构建 LLM Prompt → 调用 LLM → 解析返回结果 → 批量入库 → 触发脚本生成。
3. **实现分片处理逻辑**：超长文档的智能分片和分片 Prompt 构建。
4. **实现解析状态更新**：在处理过程中持续更新 Redis 中的解析进度。

### 任务 3.4：实现脚本生成服务

**你的任务**：

在 `backend/internal/service/template_service.go`（或独立文件）中，实现脚本自动生成逻辑。严格遵循 `backend_detailed_design_v2.0_complete.md` 第 11 节的设计。

1. **实现脚本生成 Worker**：从队列消费规则 ID → 构建检查脚本 Prompt → 调用 LLM → 安全性校验 → 存储脚本。
2. **实现修复脚本生成**：同上流程，使用修复脚本 Prompt。
3. **实现脚本安全性校验**：危险命令检测、Shebang 检查、网络外联检测、长度检查。
4. **实现脚本版本管理**：创建版本记录、更新 baseline_rules 表、上传到 MinIO。

### 任务 3.5：实现自愈修复服务（核心）

**你的任务**：

在 `backend/internal/service/self_healing_service.go` 中，实现自愈修复的完整业务流程。严格遵循 `backend_detailed_design_v2.0_complete.md` 第 12 节的设计。

1. **实现自愈触发判断**：根据脚本类型和退出码判断是否需要触发自愈。
2. **实现自愈 Prompt 构建**：组装原始脚本、错误信息、执行环境和历史修复尝试信息。
3. **实现自愈执行循环**：调用 LLM → 安全校验 → 记录版本 → 下发执行 → 等待结果 → 判断是否继续。
4. **实现自愈状态管理**：更新 Redis 状态、记录数据库日志、通知前端。
5. **实现最大重试限制**：3 次重试后标记为失败。

### 任务 3.6：实现任务编排服务

**你的任务**：

在 `backend/internal/service/task_service.go` 中，实现任务编排逻辑。严格遵循 `backend_detailed_design_v2.0_complete.md` 第 13 节的设计。

1. **实现任务下发**：查询脚本 → 生成任务组 → 为每台主机创建子任务 → 通过 gRPC 下发。
2. **实现任务结果处理**：接收 Agent 结果 → 更新状态 → 判断是否触发自愈。
3. **实现离线主机处理**：检查 Redis 心跳 Key，离线主机直接标记失败。

---

## 第四部分：后端 API 与 gRPC 实现

### 任务 4.1：实现 gRPC 服务器

**你的任务**：

在 `backend/internal/grpc_server` 目录下，实现 gRPC 服务器。遵循 `communication_structure_design_v2.0_complete.md` 的 gRPC 协议设计。

1. **实现 `server.go`**：gRPC 服务器启动和 `Register` RPC 方法。
2. **实现 `agent_manager.go`**：线程安全的 Agent 连接管理器，支持按 host_id 查找连接和下发命令。
3. **实现 Agent 注册**：接收 AssetInfo → 写入 hosts 表 → 注册连接。
4. **实现心跳处理**：接收 HeartbeatRequest → 更新 Redis 和数据库。
5. **实现命令下发与结果接收**：通过 channel 接收来自 API 层的命令请求，通过 gRPC 流下发；从 Agent 流接收 CommandResult 并交给 task_service 处理。

### 任务 4.2：实现 RESTful API (Gin)

**你的任务**：

在 `backend/internal/api` 目录下，使用 Gin 框架实现所有 RESTful API 接口。严格遵循 `communication_structure_design_v2.0_complete.md` 第 4 节的所有接口定义。

1. **实现 `router.go`**：注册所有路由和中间件。
2. **实现 `handler/config_handler.go`**：LLM 配置的获取、更新和测试接口。
3. **实现 `handler/host_handler.go`**：主机列表查询接口（含 Redis 在线状态判断）。
4. **实现 `handler/template_handler.go`**：模板上传、列表、状态查询、规则查询和删除接口。
5. **实现 `handler/task_handler.go`**：任务下发、状态查询和日志查询接口。
6. **实现 `handler/agent_handler.go`**：Agent 安装命令、安装脚本和下载链接接口。
   - `GET /api/v1/agent/install-command`：返回包含动态 IP 的安装命令。该 Handler 在初始化时接收由 `ipdetect.DetectServerIP()` 检测到的 IP 地址，并将其缓存在 `AgentHandler` 结构体中。每次请求时直接返回缓存的 IP，不重复检测。返回格式为 `{"command": "curl -sSL http://{ip}:{port}/api/v1/agent/install.sh | sudo bash", "server_ip": "{ip}", "http_port": {port}, "grpc_port": {grpc_port}}`。
   - `GET /api/v1/agent/install.sh`：使用 Go 的 `text/template` 包动态生成安装脚本，将 `SERVER_ADDR`、`GRPC_ADDR` 等变量替换为实际地址。
7. **实现中间件**：CORS、请求日志和 Panic 恢复中间件。

---

## 第五部分：Agent 实现

### 任务 5.1：实现 Agent 端

**你的任务**：

在 `agent` 目录下，实现 Agent 的全部功能。严格遵循 `agent_detailed_design_v1.6_complete.md` 的设计。此部分与 V1.6 的实现指导一致。

1. 实现 `internal/config` 模块。
2. 实现 `internal/asset` 模块。
3. 实现 `internal/client` 模块。
4. 实现 `internal/executor` 模块。
5. 编写 `cmd/agent/main.go`。

---

## 第六部分：前端实现

### 任务 6.1：实现项目骨架、API 通讯层和状态管理

**你的任务**：

严格遵循 `frontend_detailed_design_v1.6_complete.md` 的设计。此部分与 V1.6 的实现指导一致。

1. 创建项目结构。
2. 实现 API 通讯层（注意新增的 V2.0 API 接口也需要封装）。
3. 实现状态管理 (Pinia)。

### 任务 6.2：实现页面与组件

**你的任务**：

根据 `prd_design_v1.6_complete.md` 和 `frontend_detailed_design_v1.6_complete.md` 的设计，实现所有页面和组件。此部分与 V1.6 的实现指导一致。

1. 实现 `Settings.vue` 页面。
2. 实现 `Dashboard.vue` 页面。
3. 实现 `Workbench.vue` 页面。
4. 实现通用组件。

---

## 第七部分：后端启动与集成

### 任务 7.1：实现后端主程序入口

**你的任务**：

编写 `backend/cmd/server/main.go`，整合所有模块。严格遵循 `backend_detailed_design_v2.0_complete.md` 第 14 节的启动流程。

1. 按顺序初始化所有依赖（配置 → 数据库 → Redis → MinIO → Repository → LLM → Service → Worker → gRPC → HTTP）。
2. **在初始化阶段（在启动 gRPC 和 HTTP 服务器之前），调用 `ipdetect.DetectServerIP(cfg.Server.ExternalIP)` 检测服务器 IP 地址**，将结果传入 `AgentHandler` 的构造函数。并将检测结果输出到日志：
   ```go
   serverIP := ipdetect.DetectServerIP(cfg.Server.ExternalIP)
   logger.Info("Detected server IP", zap.String("ip", serverIP))
   logger.Info("Agent install command", zap.String("command",
       fmt.Sprintf("curl -sSL http://%s:%d/api/v1/agent/install.sh | sudo bash",
           serverIP, cfg.Server.HTTPPort)))
   ```
3. 实现优雅关闭逻辑。
4. 实现结构化日志初始化。

---

请等待我的指示，我们将一步一步完成这个项目。现在，请从 **任务 1.1** 开始。
