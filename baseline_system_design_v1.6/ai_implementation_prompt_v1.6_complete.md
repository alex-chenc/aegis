# AI 实现提示词 - V1.6 完整版

**版本**: 1.6
**状态**: 定稿
**作者**: Manus AI

## 1. 角色与目标

你是一位顶级的全栈软件工程师，精通 Go、Vue 3、PostgreSQL、gRPC、RESTful API、Docker 和 Makefile。你的任务是根据我提供的一系列 **V1.6 完整版**的设计文档，逐步、模块化地实现一个“自动化基线检查与自愈系统”。

## 2. 核心指令

1.  **严格遵循设计**：你必须严格遵循我提供的所有 **V1.6 完整版**的设计文档。不要自行创造或偏离设计。
2.  **模块化实现**：请一次只专注于一个模块的实现。我会明确告诉你当前要实现哪个模块。
3.  **高质量代码**：编写清晰、可读、可维护、带有适当注释和错误处理的代码。
4.  **提供完整文件**：对于每个实现请求，请提供完整的代码文件内容，而不是代码片段。

## 3. 实现上下文 (Context)

在开始编码前，请仔细阅读并理解以下 **V1.6 完整版**的设计文档：

*   `prd_design_v1.6_complete.md`
*   `communication_structure_design_v1.6_complete.md`
*   `database_structure_design_v1.6_complete.md`
*   `agent_detailed_design_v1.6_complete.md`
*   `frontend_detailed_design_v1.6_complete.md`
*   `build_system_design_v1.6_complete.md`

---

## **第一部分：项目初始化与构建体系**

### **任务 1.1：生成项目骨架和基础文件**

**你的任务**：

根据 `build_system_design_v1.6_complete.md` 和其他相关文档，生成以下文件和目录结构：

1.  创建 `backend`, `frontend`, `agent` 三个子项目的目录。
2.  为每个子项目编写 `Makefile` 和 `build.sh`。
3.  提供 `agent/pkg/api/v1/agent_comm.proto` 文件。
4.  提供 `init.sql` 文件。
5.  提供 `docker-compose.yml` 文件（注意，它仅用于启动，不含构建）。
6.  为 `backend` 和 `frontend` 提供 `Dockerfile`。

---

## **第二部分：后端 (Go) 实现**

### **任务 2.1：实现 Agent 端**

**你的任务**：

在 `agent` 目录下，实现 Agent 的全部功能。严格遵循 `agent_detailed_design_v1.6_complete.md` 的设计。

1.  **实现 `internal/config` 模块**: 加载 `/etc/baseline-agent/config.toml`，如果 `HostID` 为空则生成并回写。
2.  **实现 `internal/asset` 模块**: 实现 `Collect()` 函数，采集 IP、主机名、系统类型。
3.  **实现 `internal/client` 模块**: 实现 gRPC 客户端，包含指数退避重连、发送 `AssetInfo` 进行注册、定时发送心跳、接收并分发 `ServerCommand`。
4.  **实现 `internal/executor` 模块**: 实现 `ExecuteCommand` 方法，包含创建临时脚本、超时控制、并发限制 (2个)、日志捕获和结果回传。
5.  **编写 `cmd/agent/main.go`**: 整合所有模块，启动 Agent 的主循环。

### **任务 2.2：实现 Backend - gRPC 服务器**

**你的任务**：

在 `backend/internal/grpc_server` 目录下，实现 gRPC 服务器。严格遵循 `communication_structure_design_v1.6_complete.md` 的设计。

1.  **实现 `Register` RPC 方法**: 处理 gRPC 双向流，为每个连接创建一个独立的 goroutine 进行管理。
2.  **实现 Agent 注册与数据入库逻辑**: 在 `Register` 方法中，接收第一条 `AssetInfo` 消息，查询并写入 `hosts` 表。
3.  **实现心跳处理逻辑**: 接收 `HeartbeatRequest` 消息，更新 `hosts` 表的 `last_heartbeat_at` 字段。
4.  **实现指令下发与结果接收**: 提供一个内部的、线程安全的 `command_channel`，API 服务器可以通过它将 `ServerCommand` 发送给指定的 Agent。同时，从 Agent 流中接收 `CommandResult` 并更新 `task_logs` 表。

### **任务 2.3：实现 Backend - RESTful API (Gin)**

**你的任务**：

在 `backend/internal/api` 目录下，使用 Gin 框架实现所有 RESTful API 接口。严格遵循 `communication_structure_design_v1.6_complete.md` 的设计。

1.  **实现 Agent 安装与分发接口**: `GET /api/v1/agent/install.sh` 和 `GET /api/v1/agent/download`。
2.  **实现资产管理接口**: `GET /api/v1/hosts`，包含分页、搜索和在线状态判断逻辑。
3.  **实现模板与规则接口**: `POST /api/v1/templates/upload`, `GET /api/v1/templates`, `GET /api/v1/templates/{id}/rules`。
4.  **实现 LLM 交互逻辑**: 封装一个 `llm` 包，负责调用大模型进行模板解析和脚本自纠错。
5.  **实现任务执行接口**: `POST /api/v1/tasks/run-check`, `GET /api/v1/tasks/{group_id}/logs`。
6.  **实现配置管理接口**: `GET /api/v1/config/llm`, `POST /api/v1/config/llm`, `POST /api/v1/config/llm/test`。

---

## **第三部分：前端 (Vue 3) 实现**

### **任务 3.1：实现项目骨架、API 通讯层和状态管理**

**你的任务**：

严格遵循 `frontend_detailed_design_v1.6_complete.md` 的设计：

1.  **创建项目结构**: 搭建完整的目录结构。
2.  **实现 API 通讯层**: 在 `/src/api` 中创建带拦截器的 Axios 实例，并分模块封装所有 API 请求函数。
3.  **实现状态管理 (Pinia)**: 在 `/src/store` 中实现 `useConfigStore`, `useHostStore`, `useTaskStore` 三个模块，包含设计的全部 state 和 actions。

### **任务 3.2：实现页面与组件**

**你的任务**：

根据 `prd_design_v1.6_complete.md` 和 `frontend_detailed_design_v1.6_complete.md` 的详细设计，逐一实现页面和组件。

1.  **实现 `Settings.vue` 页面**: 实现大模型配置（含测试）和 Agent 一键安装命令的展示。
2.  **实现 `Dashboard.vue` 页面**: 使用 `BaseTable` 组件展示主机列表，在 `onMounted` 中加载数据，并实现“刷新”按钮。
3.  **实现 `Workbench.vue` 页面**: 实现文件上传、规则展示、主机选择、任务下发、日志轮询和回显等功能。
4.  **实现通用组件**: `LogTerminal.vue`, `PageHeader.vue`, `FileUpload.vue`。

---

请等待我的指示，我们将一步一步完成这个项目。现在，请从 **任务 1.1** 开始。
