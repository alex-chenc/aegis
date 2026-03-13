# Agent 详细设计文档 - V1.6 完整版

**版本**: 2.1
**状态**: 定稿
**作者**: Manus AI

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 1.6 | 2026-03-05 | Manus AI | **完整重写**。确保文档独立、完整，包含所有模块的详细设计、代码结构和配置规范，移除所有外部引用。 |
| 1.5 | 2026-03-05 | Manus AI | 精简采集信息，明确 root 安装，优化构建与分发流程。 |

## 2. 概述

Agent 是部署在目标主机上的轻量级 Go 语言程序，作为自动化基线检查与自愈系统的执行端点。它被设计为无依赖的静态二进制文件，以实现轻量、高效、跨平台和易于部署的目标。

## 3. 核心功能

Agent 的核心功能被拆分为三个独立的模块：

1.  **心跳与注册模块 (Heartbeat & Register)**: 负责与后端 gRPC 服务器建立并维持长连接，上报自身身份信息。
2.  **资产信息收集模块 (Asset Collector)**: 负责在启动时采集主机的核心身份信息。
3.  **命令执行模块 (Command Executor)**: 负责安全地执行后端下发的 Shell 脚本，并回传结果。

## 4. 目录与代码结构

建议的 Agent 项目目录结构如下：

```
/agent
|-- /cmd/agent         # 主程序入口
|   |-- main.go
|-- /internal
|   |-- asset          # 资产信息收集模块
|   |   |-- collector.go
|   |-- client         # gRPC 客户端与心跳模块
|   |   |-- client.go
|   |-- executor       # 命令执行模块
|   |   |-- executor.go
|   |-- config         # 配置文件加载模块
|   |   |-- config.go
|-- /pkg/api/v1        # Protobuf 生成的 Go 代码
|   |-- agent_comm.pb.go
|   |-- agent_comm_grpc.pb.go
|-- go.mod
|-- go.sum
|-- Makefile
|-- build.sh
```

## 5. 模块详细设计

### 5.1 配置模块 (`internal/config`)

*   **功能**: 负责加载和解析 Agent 的配置文件。
*   **配置文件路径**: `/etc/aegis-agent/config.toml`
*   **配置文件格式 (TOML)**:
    ```toml
    # 后端 gRPC 服务器地址
    ServerAddr = "127.0.0.1:9090"
    
    # 用于与后端认证的 Token
    AuthToken = "a_very_secret_token"
    
    # Agent 的唯一标识符，如果文件不存在或为空，Agent 首次启动时会生成一个 UUID 并回写
    HostID = "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    ```
*   **实现细节**: `config.go` 中应定义一个 `LoadConfig()` 函数，该函数读取 TOML 文件，如果 `HostID` 为空，则生成一个新的 UUID 并更新配置文件。

### 5.2 资产信息收集模块 (`internal/asset`)

*   **功能**: 采集主机的核心身份信息。
*   **采集清单 (精简)**:
    | 采集项 | 数据类型 | 获取方式 (Go) |
    |:---|:---|:---|
| **IP 地址** | `string` | 遍历 `net.Interfaces()`，获取第一个非环回的有效 IPv4 地址。 |
| **主机名** | `string` | `os.Hostname()` |
| **系统类型** | `string` | `runtime.GOOS` (e.g., "linux") |
*   **实现细节**: `collector.go` 中应定义一个 `Collect()` 函数，该函数返回一个包含上述信息的结构体。此函数仅在 Agent 启动时被调用一次。

### 5.3 gRPC 客户端与心跳模块 (`internal/client`)

*   **功能**: 负责与后端建立 gRPC 双向流，并维持心跳。
*   **工作流程**:
    1.  **启动连接**: Agent 启动后，该模块使用从配置中读取的 `ServerAddr` 和 `AuthToken` (作为 gRPC metadata) 尝试与后端建立连接。
    2.  **指数退避重连**: 如果连接失败或意外断开，启动重连机制。初始间隔 5 秒，每次失败后翻倍，最大间隔 5 分钟。连接成功后重置间隔。
    3.  **注册 (首次通讯)**: 连接成功后，立即调用资产收集模块获取信息，组装成 `AssetInfo` 消息，作为第一条消息发送给后端。
    4.  **心跳维持**: 注册成功后，启动一个 Ticker，以 **30 秒** 为固定周期，定时向后端发送空的 `HeartbeatRequest` 消息。
    5.  **接收指令**: 在一个独立的 goroutine 中循环接收从服务端流中下发的 `ServerMessage`。如果收到 `ServerCommand`，则将其交给命令执行模块处理。
*   **实现细节**: `client.go` 中应包含一个 `NewClient` 构造函数和一个 `Run` 方法。`Run` 方法内包含主循环，处理连接、重连、心跳和消息接收。

### 5.4 命令执行模块 (`internal/executor`)

*   **功能**: 安全地执行后端下发的 Shell 脚本。
*   **工作流程**:
    1.  **接收任务**: 从 gRPC 客户端模块接收 `ServerCommand` 指令。
    2.  **创建脚本**: 在临时目录 (`/tmp/aegis-agent/`) 中创建一个唯一的子目录，并将脚本内容写入文件（例如 `script.sh`），赋予 `0700` 权限。
    3.  **执行与捕获**: 使用 `os/exec` 启动 `bash script.sh` 子进程。通过 `io.Pipe` 实时捕获 `stdout` 和 `stderr` 的输出。
    4.  **超时控制**: 使用 `context.WithTimeout` 为每个脚本执行设置超时时间（从 `ServerCommand` 中获取）。
    5.  **结果回传**: 脚本执行结束后（正常结束、出错或超时），将最终状态（退出码、是否超时）和完整的 `stdout`, `stderr` 日志打包成 `CommandResult` 消息，通过 gRPC 客户端模块发送回后端。
    6.  **并发限制**: 使用一个带缓冲的 channel (大小为 2) 作为信号量，确保 Agent 最多同时执行 **2** 个脚本。在执行脚本前获取信号量，结束后释放。
    7.  **文件清理**: 使用 `defer` 语句确保无论任务成功与否，创建的临时脚本文件和目录都会被清理。
*   **实现细节**: `executor.go` 中应定义一个 `NewExecutor` 构造函数和一个 `ExecuteCommand` 方法。

## 6. 安装与部署

*   **安装用户**: 必须由 **`root`** 用户执行安装脚本。
*   **安装路径**:
    *   二进制文件: `/usr/local/bin/aegis-agent`
    *   配置文件: `/etc/aegis-agent/config.toml`
    *   Systemd 服务文件: `/etc/systemd/system/aegis-agent.service`
*   **一键安装**: 通过 `curl -sSL http://<Server_IP>/api/v1/agent/install.sh | sudo bash` 命令完成。安装脚本的具体内容见 `build_system_design_v1.6_complete.md`。
*   **服务守护**: 通过 Systemd 实现开机自启和进程守护。

## 7. 构建与分发

*   **构建**: 在 Docker 容器内使用 `golang:1.20-alpine` 镜像进行交叉编译，生成 `linux/amd64` 和 `linux/arm64` 两个平台的静态二进制文件。
*   **分发**: 构建脚本在编译完成后，会自动将产物上传到 MinIO 的 `agent-artifacts` Bucket 中。安装脚本会根据目标主机的架构，从后端获取对应的预签名下载链接进行下载。
