# 后端详细设计文档 - V2.6 完整版

**版本**: 2.6
**状态**: 定稿
**作者**: Manus AI, Sisyphus

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 2.6 | 2026-03-09 | Sisyphus | **LLM 超时优化**。超时时间从 60 秒优化为 120 秒，解决 PDF 解析超时问题。所有服务（TemplateService, ScriptGenerationService, SelfHealingService）使用配置值。添加获取完整 API Key 接口。 |
| 2.5 | 2026-03-09 | Sisyphus | **动态 LLM 配置**。所有服务 (TemplateService, ScriptGenerationService, SelfHealingService) 现在从数据库动态获取 LLM 配置，解决 API Key 刷新后丢失的问题。修复 Agent 下载 URL 使用容器内部地址的问题。 |
| 2.4 | 2026-03-09 | Sisyphus | **修复实现问题**。实现真实的 LLM 配置保存和连接测试，修复模板上传后不触发解析的问题 (QueueTemplate)，修复 Agent 下载返回 JSON 改为重定向，添加 encryptionKey 参数到 ConfigHandler。 |
| 2.3 | 2026-03-09 | Sisyphus | **IP 检测策略调整**。将 IP 检测优先级从"优先公网 IP"改为"优先本地 IP"，确保 Agent 能从本地网络访问后端服务。增加 Docker 网段过滤范围（172.17-31.x.x），修复模板上传 API 实现。 |
| 2.1 | 2026-03-05 | Manus AI | **补充 IP 检测模块**。新增第 8 节「服务器 IP 自动检测模块」，设计了优先公网 IP 的多策略自动检测逻辑，更新 Agent 安装命令 API 的返回内容，使前端可直接复制粘贴安装命令。 |
| 2.0 | 2026-03-05 | Manus AI | **全新文档**。补充 V1.6 中缺失的后端详细设计，涵盖项目结构、数据库访问层、Redis 缓存层、MinIO 对象存储层、LLM 交互模块、文件上传与解析模块、提示词自动生成与数据入库模块、脚本自动生成模块、自愈修复模块等全部后端核心逻辑。 |

## 2. 概述

本文档为自动化基线检查与自愈系统的后端服务提供全面、可执行的详细设计规范。后端服务采用 Go 语言开发，基于 Gin 框架提供 RESTful API，基于 gRPC 提供与 Agent 的双向流通讯。后端是整个系统的中枢，负责编排前端请求、Agent 通讯、LLM 调用、数据库读写、缓存管理和对象存储等所有核心业务逻辑。

V1.6 版本的设计文档中，后端部分仅在通讯层文档中定义了 API 接口的请求/响应格式，以及在 AI 提示词文档中列出了实现任务清单，但缺少一份独立、完整的后端详细设计文档。本文档旨在填补这一空白，为开发者提供从项目结构到每个模块内部实现逻辑的完整指导。

## 3. 后端项目结构

后端项目采用 Go 语言标准的分层架构，遵循关注点分离原则。以下是完整的目录结构及各目录的职责说明。

```
/backend
|-- /cmd
|   |-- /server
|   |   |-- main.go                  # 主程序入口，负责初始化和启动所有服务
|-- /config
|   |-- config.yaml                  # 应用配置文件（数据库、Redis、MinIO、服务端口等）
|   |-- config.go                    # 配置加载与解析模块
|-- /internal
|   |-- /api                         # RESTful API 层 (Gin)
|   |   |-- /handler                 # HTTP 请求处理器
|   |   |   |-- config_handler.go    # LLM 配置相关接口
|   |   |   |-- host_handler.go      # 资产管理相关接口
|   |   |   |-- template_handler.go  # 模板上传与规则相关接口
|   |   |   |-- task_handler.go      # 任务执行与日志相关接口
|   |   |   |-- agent_handler.go     # Agent 安装与下载相关接口（含动态IP）
|   |   |-- /middleware              # HTTP 中间件
|   |   |   |-- cors.go              # 跨域处理
|   |   |   |-- logger.go            # 请求日志
|   |   |   |-- recovery.go          # Panic 恢复
|   |   |-- router.go                # 路由注册
|   |-- /grpc_server                 # gRPC 服务层
|   |   |-- server.go                # gRPC 服务器实现
|   |   |-- agent_manager.go         # Agent 连接管理器（线程安全的连接池）
|   |-- /service                     # 业务逻辑层
|   |   |-- config_service.go        # LLM 配置业务逻辑
|   |   |-- host_service.go          # 资产管理业务逻辑
|   |   |-- template_service.go      # 模板解析业务逻辑（核心）
|   |   |-- task_service.go          # 任务编排业务逻辑
|   |   |-- self_healing_service.go  # 自愈修复业务逻辑（核心）
|   |-- /repository                  # 数据访问层 (DAL)
|   |   |-- db.go                    # 数据库连接池初始化
|   |   |-- host_repo.go             # hosts 表 CRUD
|   |   |-- template_repo.go         # templates 表 CRUD
|   |   |-- rule_repo.go             # baseline_rules 表 CRUD
|   |   |-- task_log_repo.go         # task_logs 表 CRUD
|   |   |-- config_repo.go           # llm_configs 表 CRUD
|   |   |-- script_version_repo.go   # script_versions 表 CRUD
|   |   |-- healing_log_repo.go      # self_healing_logs 表 CRUD
|   |-- /llm                         # LLM 交互模块
|   |   |-- client.go                # OpenAI 兼容 API 客户端封装
|   |   |-- prompts.go               # 所有 Prompt 模板定义
|   |   |-- parser.go                # LLM 返回结果解析器
|   |   |-- validator.go             # LLM 连通性校验
|   |-- /fileparser                  # 文件解析模块
|   |   |-- parser.go                # 解析器接口定义
|   |   |-- pdf_parser.go            # PDF 文件解析器
|   |   |-- word_parser.go           # Word (DOCX) 文件解析器
|   |   |-- yaml_parser.go           # YAML 文件解析器
|   |   |-- excel_parser.go          # Excel (XLSX) 文件解析器
|   |   |-- text_parser.go           # 纯文本文件解析器
|   |-- /storage                     # 存储层封装
|   |   |-- minio_client.go          # MinIO 客户端封装
|   |   |-- redis_client.go          # Redis 客户端封装
|   |-- /ipdetect                    # 服务器 IP 自动检测模块
|   |   |-- detector.go              # IP 检测器（多策略，优先公网 IP）
|   |-- /model                       # 数据模型定义
|   |   |-- host.go
|   |   |-- template.go
|   |   |-- rule.go
|   |   |-- task_log.go
|   |   |-- config.go
|   |   |-- script_version.go
|   |   |-- self_healing_log.go
|-- /pkg
|   |-- /api/v1                      # Protobuf 生成的 Go 代码
|   |   |-- agent_comm.pb.go
|   |   |-- agent_comm_grpc.pb.go
|-- /scripts
|   |-- init.sql                     # 数据库初始化脚本
|   |-- seed.sql                     # 测试数据种子脚本
|-- go.mod
|-- go.sum
|-- Makefile
|-- build.sh
|-- Dockerfile
```

## 4. 服务器 IP 自动检测模块

### 4.1 设计目标

后端需要在启动时自动检测自身对外可达的 IP 地址，并将其嵌入到 Agent 安装命令和安装脚本中，使前端用户可以直接复制粘贴命令完成 Agent 安装，无需手动填写服务器地址。

**重要说明**：V2.3 版本将检测策略从"优先公网IP"改为"优先本地IP"，原因是：
1. 大多数部署场景下，Agent 和后端在同一局域网内通信
2. 公网IP可能无法从内网访问（NAT、防火墙等原因）
3. 本地IP更稳定可靠，适合实际生产环境

检测策略遵循「优先本地IP」的原则，按以下优先级依次尝试，直到获取到有效 IP 为止。

| 优先级 | 策略 | 说明 |
|:--:|:---|:---|
| 1 | **读取配置文件** | 如果 `config.yaml` 中 `server.external_ip` 字段不为空，或环境变量 `SERVER_EXTERNAL_IP` 已设置，直接使用该值，跳过所有自动检测步骤。适用于管理员明确知道服务器 IP 的场景。 |
| 2 | **枚举本机网卡** | 遍历所有网络接口，过滤掉回环地址（`127.x.x.x`）、Docker 虚拟网卡地址（`172.17-31.x.x` 段），返回第一个有效的 IPv4 地址。这是最可靠的方式，优先使用。 |
| 3 | **出站连接本机 IP** | 通过向 `8.8.8.8:80` 建立 UDP 连接（不实际发送数据），获取本机用于对外通讯的网卡 IP 地址。此方法可以准确识别默认路由对应的网卡 IP。 |
| 4 | **兜底值** | 如果所有策略均失败，返回 `127.0.0.1`，并在日志中输出 `WARN` 级别警告，提示管理员手动配置 `server.external_ip`。 |

### 4.2 IP 检测器实现 (`internal/ipdetect/detector.go`)

```go
package ipdetect

import (
    "context"
    "io"
    "net"
    "net/http"
    "strings"
    "time"
)

// 公网 IP 查询服务列表（保留备用，但默认不使用）
var publicIPServices = []string{
    "https://api.ipify.org",
    "https://ifconfig.me/ip",
    "https://icanhazip.com",
    "https://api4.my-ip.io/ip",
}

// DetectServerIP 按优先级检测服务器对外可达的 IP 地址
// configuredIP: 来自配置文件的 external_ip 字段，非空则直接返回
// 优先级：配置文件 > 本地网卡 > 出站连接IP > 兜底值
func DetectServerIP(configuredIP string) string {
    // 策略 1：使用配置文件中的显式配置
    if configuredIP != "" {
        return configuredIP
    }

    // 策略 2：枚举本机网卡（优先）
    if ip := getLocalIP(); ip != "" {
        return ip
    }

    // 策略 3：通过出站连接获取本机 IP
    if ip := getOutboundIP(); ip != "" {
        return ip
    }

    // 策略 4：兜底
    return "127.0.0.1"
}

// getLocalIP 枚举网卡，过滤回环和 Docker 虚拟网卡，返回第一个有效 IPv4
func getLocalIP() string {
    ifaces, err := net.Interfaces()
    if err != nil {
        return ""
    }
    for _, iface := range ifaces {
        // 跳过未启用或回环接口
        if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
            continue
        }
        addrs, _ := iface.Addrs()
        for _, addr := range addrs {
            var ip net.IP
            switch v := addr.(type) {
            case *net.IPNet:
                ip = v.IP
            case *net.IPAddr:
                ip = v.IP
            }
            if ip == nil || ip.IsLoopback() || ip.To4() == nil {
                continue
            }
            ipStr := ip.String()
            // 过滤 Docker 默认网段 172.17.x.x - 172.31.x.x
            if isDockerBridgeIP(ipStr) {
                continue
            }
            return ipStr
        }
    }
    return ""
}

// isDockerBridgeIP 检查是否为 Docker 桥接网络 IP
func isDockerBridgeIP(ipStr string) bool {
    // Docker 默认使用 172.17.0.0/16 到 172.31.0.0/16
    prefixes := []string{
        "172.17.", "172.18.", "172.19.", "172.20.",
        "172.21.", "172.22.", "172.23.", "172.24.",
        "172.25.", "172.26.", "172.27.", "172.28.",
        "172.29.", "172.30.", "172.31.",
    }
    for _, prefix := range prefixes {
        if strings.HasPrefix(ipStr, prefix) {
            return true
        }
    }
    return false
}

// getOutboundIP 通过建立 UDP 连接获取本机出站 IP
func getOutboundIP() string {
    conn, err := net.Dial("udp", "8.8.8.8:80")
    if err != nil {
        return ""
    }
    defer conn.Close()
    localAddr := conn.LocalAddr().(*net.UDPAddr)
    return localAddr.IP.String()
}

// queryPublicIP 依次请求公网 IP 查询服务（保留备用）
func queryPublicIP() string {
    client := &http.Client{
        Timeout: 3 * time.Second,
    }
    for _, serviceURL := range publicIPServices {
        ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
        req, _ := http.NewRequestWithContext(ctx, "GET", serviceURL, nil)
        resp, err := client.Do(req)
        cancel()
        if err != nil {
            continue
        }
        defer resp.Body.Close()
        body, err := io.ReadAll(resp.Body)
        if err != nil {
            continue
        }
        ip := strings.TrimSpace(string(body))
        if net.ParseIP(ip) != nil {
            return ip
        }
    }
    return ""
}
```

### 4.3 配置方式

推荐在 `.env` 文件或 `docker-compose.yml` 中设置 `SERVER_PUBLIC_IP` 环境变量：

```bash
# .env
SERVER_PUBLIC_IP=192.168.1.100
```

或在 `docker-compose.yml` 中：

```yaml
environment:
  SERVER_EXTERNAL_IP: ${SERVER_PUBLIC_IP:-}
```

### 4.4 IP 检测结果的缓存与使用

IP 检测在后端服务**启动时执行一次**，结果存储在内存中（作为 `AgentHandler` 的字段）。后续每次调用 `/api/v1/agent/install-command` 和 `/api/v1/agent/install.sh` 接口时，直接使用缓存的 IP 值，不重复检测。

如果管理员在运行时修改了 `server.external_ip` 配置，需要重启服务才能生效。

### 4.4 安装命令格式

后端返回的安装命令格式如下，其中 `{SERVER_IP}` 由 IP 检测器自动填充，`{HTTP_PORT}` 来自配置文件的 `server.http_port` 字段：

```bash
curl -sSL http://{SERVER_IP}:{HTTP_PORT}/api/v1/agent/install.sh | sudo bash
```

**示例**（服务器公网 IP 为 `203.0.113.10`，HTTP 端口为 `8080`）：

```bash
curl -sSL http://203.0.113.10:8080/api/v1/agent/install.sh | sudo bash
```

前端将此命令展示在一个带有「复制」按钮的代码框中，用户点击复制后，在目标主机上粘贴执行即可完成 Agent 的自动下载和安装。

### 4.5 动态生成的安装脚本内容

`GET /api/v1/agent/install.sh` 接口返回的是一个动态生成的 Shell 脚本，其中的后端地址和 Agent 下载链接均由服务器 IP 自动填充：

```bash
#!/bin/bash
# 自动化基线检查与自愈系统 - Agent 一键安装脚本
# 由服务端动态生成，请勿手动修改
set -e

SERVER_ADDR="{SERVER_IP}:{HTTP_PORT}"
GRPC_ADDR="{SERVER_IP}:{GRPC_PORT}"
INSTALL_DIR="/opt/baseline-agent"
SERVICE_NAME="baseline-agent"

# 检测系统架构
ARCH=$(uname -m)
case $ARCH in
    x86_64)  ARCH_SUFFIX="amd64" ;;
    aarch64) ARCH_SUFFIX="arm64" ;;
    *) echo "不支持的架构: $ARCH"; exit 1 ;;
esac

DOWNLOAD_URL="http://$SERVER_ADDR/api/v1/agent/download?os=linux&arch=$ARCH_SUFFIX"

echo "[INFO] 正在从 $SERVER_ADDR 下载 Agent..."
mkdir -p $INSTALL_DIR
curl -sSL -o $INSTALL_DIR/baseline-agent "$DOWNLOAD_URL"
chmod +x $INSTALL_DIR/baseline-agent

echo "[INFO] 正在创建 systemd 服务..."
cat > /etc/systemd/system/$SERVICE_NAME.service <<EOF
[Unit]
Description=Baseline Check Agent
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/baseline-agent --server $GRPC_ADDR
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable $SERVICE_NAME
systemctl start $SERVICE_NAME

echo "[INFO] Agent 安装完成！服务已启动。"
systemctl status $SERVICE_NAME
```

## 5. 配置管理模块

### 4.1 配置文件设计 (`config/config.yaml`)

后端服务的所有外部依赖配置均通过一个统一的 YAML 配置文件管理，同时支持环境变量覆盖以适配容器化部署场景。

```yaml
# config/config.yaml

server:
  http_port: 8080          # Gin HTTP 服务端口
  grpc_port: 9090          # gRPC 服务端口
  server_ip: "0.0.0.0"     # 服务绑定地址（用于生成安装命令）
  external_ip: ""          # 外部可访问IP（用于Agent安装脚本，为空时自动检测）

database:
  host: "postgres"
  port: 5432
  user: "baseline_user"
  password: "a_strong_db_password"
  dbname: "baseline_db"
  sslmode: "disable"
  max_open_conns: 25       # 最大打开连接数
  max_idle_conns: 10       # 最大空闲连接数
  conn_max_lifetime: 300   # 连接最大存活时间（秒）

redis:
  host: "redis"
  port: 6379
  password: "a_strong_redis_password"
  db: 0
  pool_size: 20            # 连接池大小
  min_idle_conns: 5        # 最小空闲连接数

minio:
  endpoint: "minio:9000"
  access_key: "minio_admin"
  secret_key: "a_third_strong_secret_password"
  use_ssl: false
  buckets:
    templates: "baseline-templates"       # 存储上传的基线模板文件
    agent_artifacts: "agent-artifacts"    # 存储Agent二进制构建产物
    scripts: "generated-scripts"          # 存储LLM生成的脚本文件

llm:
  default_base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1"
  default_model: "qwen-plus"
  request_timeout: 120     # LLM API 请求超时时间（秒）
  max_retries: 3           # LLM API 最大重试次数
  retry_interval: 2        # 重试间隔（秒）

agent:
  heartbeat_timeout: 90    # Agent 心跳超时判定时间（秒），超过此时间未收到心跳视为离线
  auth_token: "a_very_secret_agent_token"  # Agent 认证 Token
  script_timeout: 300      # 脚本默认执行超时时间（秒）

self_healing:
  max_retries: 3           # 自愈最大重试次数
  enabled: true            # 是否启用自愈功能
```

### 4.2 配置加载逻辑 (`config/config.go`)

配置加载模块使用 `viper` 库实现，支持从 YAML 文件加载配置，并允许通过环境变量进行覆盖。环境变量的命名规则为：将 YAML 路径中的层级用下划线连接并转为大写，例如 `database.host` 对应环境变量 `DATABASE_HOST`。

在 `main.go` 中，配置加载是第一个执行的初始化步骤。加载完成后，配置对象以依赖注入的方式传递给各个模块的初始化函数，避免全局变量的使用。

## 5. 数据库访问层 (Repository)

### 5.1 数据库连接池初始化 (`repository/db.go`)

数据库连接池使用 GORM ORM 框架配合 `gorm.io/driver/postgres` 驱动实现。GORM 提供了类型安全的链式查询 API，自动处理连接池管理，并支持事务、钩子和迁移等高级功能。初始化时根据配置文件设置连接池参数，并执行一次 `Ping()` 确认数据库连通性。如果连接失败，程序应以明确的错误信息退出，而非静默重试。

```go
// 伪代码示意
func NewDB(cfg *config.DatabaseConfig) (*gorm.DB, error) {
    dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
        cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)
    
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        return nil, fmt.Errorf("failed to connect database: %w", err)
    }
    
    sqlDB, _ := db.DB()
    sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
    sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
    sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)
    
    if err := sqlDB.Ping(); err != nil {
        return nil, fmt.Errorf("failed to ping database: %w", err)
    }
    
    return db, nil
}
```

### 5.2 Repository 模式

每张数据库表对应一个 Repository 结构体，封装该表的所有 CRUD 操作。Repository 接受 `*gorm.DB` 作为构造参数，所有数据库操作均通过 GORM 的链式 API 执行，GORM 内部自动处理参数化查询以防止 SQL 注入。对于需要事务支持的场景（如批量插入规则），使用 `db.Transaction()` 方法。

以 `host_repo.go` 为例，其核心方法包括：

| 方法名 | 功能描述 | GORM 操作 |
|:---|:---|:---|
| `Upsert(host *model.Host)` | 注册或更新主机信息，以 `ip_address` 为冲突判断键 | `db.Clauses(clause.OnConflict{...}).Create(host)` |
| `UpdateHeartbeat(hostID uuid.UUID)` | 更新主机的最后心跳时间 | `db.Model(&Host{}).Where("id = ?", hostID).Updates(...)` |
| `FindAll(page, pageSize int, query string)` | 分页查询主机列表，支持按 IP 或主机名模糊搜索 | `db.Where("ip_address LIKE ? OR hostname LIKE ?", ...).Find(&hosts)` |
| `FindByID(id uuid.UUID)` | 根据 ID 查询单个主机 | `db.First(&host, "id = ?", id)` |
| `Count(query string)` | 统计符合条件的主机总数 | `db.Model(&Host{}).Where(...).Count(&count)` |

其他 Repository（`template_repo.go`、`rule_repo.go`、`task_log_repo.go`、`config_repo.go`、`script_version_repo.go`、`healing_log_repo.go`）均遵循相同的设计模式。

### 5.3 日志记录

所有数据库操作均使用 `pkg/logger` 包中的 zap 日志器进行日志记录。关键操作（如创建、更新）记录 Info 级别日志，错误操作记录 Error 级别日志。日志包含操作类型、实体 ID、错误信息等上下文字段。

```go
// 日志示例
logger.Info("host upserted successfully",
    zap.String("id", host.ID.String()),
    zap.String("ip", host.IPAddress),
    zap.String("hostname", host.Hostname),
)
```

## 6. Redis 缓存层

### 6.1 Redis 客户端初始化 (`storage/redis_client.go`)

Redis 客户端使用 `go-redis/redis/v9` 库实现。初始化时根据配置文件设置连接池参数，并执行 `Ping` 命令验证连通性。

```go
// 伪代码示意
func NewRedisClient(cfg *config.RedisConfig) (*redis.Client, error) {
    rdb := redis.NewClient(&redis.Options{
        Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
        Password:     cfg.Password,
        DB:           cfg.DB,
        PoolSize:     cfg.PoolSize,
        MinIdleConns: cfg.MinIdleConns,
    })
    
    if err := rdb.Ping(context.Background()).Err(); err != nil {
        return nil, fmt.Errorf("failed to connect to Redis: %w", err)
    }
    
    return rdb, nil
}
```

### 6.2 Redis Key 设计与用途

Redis 在本系统中承担三个核心职责：Agent 在线状态缓存、模板解析任务状态缓存和 LLM 配置缓存。以下是完整的 Key 设计规范。

| Key 模式 | 数据类型 | TTL | 用途说明 |
|:---|:---|:---|:---|
| `agent:heartbeat:{host_id}` | `STRING` | 90s | 存储 Agent 最后心跳时间戳。每次收到心跳时 `SET` 并重置 TTL。判断在线状态时，Key 存在即为在线，不存在即为离线。 |
| `agent:session:{host_id}` | `STRING` | 无 (手动删除) | 存储 Agent 的 gRPC 流标识符，用于后端向指定 Agent 下发命令时定位连接。Agent 断开时删除。 |
| `template:parse:status:{template_id}` | `HASH` | 1h | 存储模板解析任务的实时状态。字段包括 `status`（parsing/completed/failed）、`progress`（0-100）、`message`（状态描述）。前端轮询此 Key 获取解析进度。 |
| `task:status:{task_id}` | `HASH` | 2h | 存储任务执行的实时状态和日志。字段包括 `status`、`stdout`、`stderr`、`exit_code`。前端轮询此 Key 获取执行进度。 |
| `task:logs:{task_id}` | `LIST` | 2h | 以列表形式追加存储任务执行的实时日志行，支持前端增量拉取。 |
| `config:llm` | `HASH` | 无 (持久) | 缓存当前生效的 LLM 配置（api_key、base_url、model），避免每次 LLM 调用都查询数据库。配置更新时同步刷新。 |
| `self_healing:{task_id}` | `HASH` | 1h | 存储自愈修复流程的状态。字段包括 `attempt`（当前重试次数）、`status`、`last_error`。 |

### 6.3 Agent 在线状态判断逻辑

在 V1.6 的设计中，主机的在线状态通过查询数据库 `hosts.last_heartbeat_at` 字段与当前时间的差值来判断。V2.2 引入 Redis 后，在线状态判断改为直接检查 Redis Key 是否存在，极大地提升了查询性能。

具体流程如下：当 gRPC 服务器收到 Agent 的心跳消息时，执行两个操作——第一，调用 `SET agent:heartbeat:{host_id} {timestamp} EX 90` 在 Redis 中设置带 TTL 的心跳标记；第二，异步更新数据库 `hosts.last_heartbeat_at` 字段。当前端请求主机列表时，后端从数据库查询主机基本信息后，批量通过 Redis 的 `EXISTS` 命令检查每台主机的心跳 Key 是否存在，从而确定在线状态。这种设计将高频的心跳写入操作主要落在 Redis 上，减轻了数据库的写入压力。

## 7. MinIO 对象存储层

### 7.1 MinIO 客户端初始化 (`storage/minio_client.go`)

MinIO 客户端使用官方 `minio-go/v7` SDK 实现。初始化时根据配置文件创建客户端实例，并确保所有必需的 Bucket 已存在。

```go
// 伪代码示意
func NewMinIOClient(cfg *config.MinIOConfig) (*minio.Client, error) {
    client, err := minio.New(cfg.Endpoint, &minio.Options{
        Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
        Secure: cfg.UseSSL,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to create MinIO client: %w", err)
    }
    
    // 确保所有必需的 Bucket 存在
    buckets := []string{cfg.Buckets.Templates, cfg.Buckets.AgentArtifacts, cfg.Buckets.Scripts}
    for _, bucket := range buckets {
        exists, err := client.BucketExists(context.Background(), bucket)
        if err != nil {
            return nil, fmt.Errorf("failed to check bucket %s: %w", bucket, err)
        }
        if !exists {
            err = client.MakeBucket(context.Background(), bucket, minio.MakeBucketOptions{})
            if err != nil {
                return nil, fmt.Errorf("failed to create bucket %s: %w", bucket, err)
            }
        }
    }
    
    return client, nil
}
```

### 7.2 Bucket 规划与访问策略

系统使用三个独立的 Bucket，分别存储不同类型的文件，并配置不同的访问策略。

| Bucket 名称 | 存储内容 | 访问策略 | 对象命名规则 |
|:---|:---|:---|:---|
| `baseline-templates` | 用户上传的基线模板文件（PDF、Word、YAML、Excel） | 私有（仅后端服务可访问） | `{template_id}/{original_filename}` |
| `agent-artifacts` | Agent 交叉编译后的二进制文件 | 私有（通过预签名 URL 提供下载） | `baseline-agent-{os}-{arch}` |
| `generated-scripts` | LLM 生成的检查脚本和修复脚本 | 私有（仅后端服务可访问） | `{rule_id}/{version}/{check\|fix}.sh` |

### 7.3 核心操作封装

MinIO 客户端封装层提供以下核心方法：

| 方法名 | 功能描述 |
|:---|:---|
| `UploadFile(bucket, objectName string, reader io.Reader, size int64, contentType string)` | 上传文件到指定 Bucket |
| `DownloadFile(bucket, objectName string) (io.ReadCloser, error)` | 下载文件，返回文件流 |
| `GetPresignedURL(bucket, objectName string, expiry time.Duration)` | 生成预签名下载 URL（用于 Agent 下载） |
| `DeleteFile(bucket, objectName string)` | 删除指定文件 |
| `FileExists(bucket, objectName string) (bool, error)` | 检查文件是否存在 |

## 8. LLM 交互模块

LLM 交互模块是后端的核心智能组件，负责与大语言模型（通义千问）进行所有交互。该模块采用 OpenAI 兼容 API 格式，通过统一的客户端封装实现模板解析、脚本生成和自愈修复等功能。

### 8.1 LLM 客户端封装 (`llm/client.go`)

LLM 客户端基于标准的 HTTP 客户端封装，兼容 OpenAI API 格式。客户端从 Redis 缓存中读取当前生效的 LLM 配置（API Key 和 Base URL），如果缓存未命中则回退到数据库查询。

```go
// LLMClient 结构体
type LLMClient struct {
    httpClient  *http.Client
    redisClient *redis.Client
    configRepo  *repository.ConfigRepo
}

// ChatCompletion 发送聊天补全请求
// 参数:
//   - systemPrompt: 系统提示词，定义LLM的角色和行为约束
//   - userPrompt: 用户提示词，包含具体的任务内容
//   - temperature: 生成温度，脚本生成建议使用0.1，文本解析建议使用0.3
// 返回:
//   - LLM的响应文本内容
//   - 错误信息（包含重试逻辑）
func (c *LLMClient) ChatCompletion(ctx context.Context, systemPrompt, userPrompt string, temperature float64) (string, error)
```

客户端内置了以下容错机制：第一，**请求超时控制**，每次 API 调用设置 120 秒超时；第二，**指数退避重试**，遇到网络错误或 5xx 响应时，最多重试 3 次，间隔分别为 2 秒、4 秒、8 秒；第三，**速率限制处理**，遇到 429 响应时，读取 `Retry-After` Header 并等待相应时间后重试。

### 8.2 LLM 连通性校验 (`llm/validator.go`)

LLM 连通性校验模块为前端"系统配置"页面的"连通性测试"功能提供后端支持。校验流程分为三个层次，逐步验证配置的正确性。

**第一层：格式校验**。在发送任何网络请求之前，先对用户输入的 API Key 和 Base URL 进行基本的格式校验。API Key 不能为空且长度应大于 10 个字符；Base URL 必须是合法的 HTTP/HTTPS URL 格式。如果格式校验失败，立即返回明确的错误信息（如"API Key 格式不正确"），避免不必要的网络请求。

**第二层：网络连通性校验**。使用用户提供的 Base URL，向 `{base_url}/models` 端点发送一个 GET 请求（这是 OpenAI 兼容 API 的标准模型列表接口）。此步骤验证网络是否可达以及 API Key 是否有效。如果请求超时，返回"网络不通，请检查 Base URL 和服务器网络"；如果返回 401/403，返回"认证失败，请检查 API Key"。

**第三层：模型可用性校验**。在网络连通性校验通过后，向 `{base_url}/chat/completions` 端点发送一个最小化的聊天补全请求（例如 `{"model": "qwen-plus", "messages": [{"role": "user", "content": "ping"}], "max_tokens": 5}`）。此步骤验证配置的模型是否可用且 API Key 是否有足够的调用额度。如果模型不存在，返回"模型不可用，请确认模型名称"；如果额度不足，返回"API 额度不足"。

```go
// ValidateConfig 执行三层校验
func (v *LLMValidator) ValidateConfig(ctx context.Context, apiKey, baseURL string) (*ValidationResult, error) {
    // 第一层：格式校验
    if err := v.validateFormat(apiKey, baseURL); err != nil {
        return &ValidationResult{Status: "failed", Message: err.Error()}, nil
    }
    
    // 第二层：网络连通性校验
    if err := v.validateConnectivity(ctx, apiKey, baseURL); err != nil {
        return &ValidationResult{Status: "failed", Message: err.Error()}, nil
    }
    
    // 第三层：模型可用性校验
    if err := v.validateModelAvailability(ctx, apiKey, baseURL); err != nil {
        return &ValidationResult{Status: "failed", Message: err.Error()}, nil
    }
    
    return &ValidationResult{Status: "ok", Message: "连接成功，模型可用"}, nil
}
```

### 8.3 LLM 配置持久化与缓存同步

当用户在前端保存 LLM 配置时，后端执行以下操作序列：第一步，将完整的 API Key 和 Base URL 写入数据库 `llm_configs` 表（API Key 使用 AES-256 加密存储）；第二步，将配置同步写入 Redis 的 `config:llm` Hash Key 中；第三步，向前端返回脱敏后的 API Key（仅显示前4位和后4位，中间用星号替代）。

当 LLM 客户端需要读取配置时，优先从 Redis 缓存读取。如果缓存未命中（例如 Redis 重启后），则从数据库读取并重新写入缓存。

## 9. 文件上传与解析模块

### 9.1 文件上传流程

文件上传是基线模板管理的入口。当用户在前端"基线检查工作台"页面上传文件时，后端执行以下完整流程。

**步骤一：接收与校验**。Gin Handler 接收 `multipart/form-data` 请求，提取上传的文件。首先进行前置校验：文件大小不超过 50MB；文件扩展名必须为 `.pdf`、`.docx`、`.yaml`、`.yml`、`.xlsx` 或 `.txt` 之一；通过读取文件头部的 Magic Bytes 验证文件的真实类型，防止伪造扩展名。

**步骤二：存储到 MinIO**。校验通过后，生成一个新的 UUID 作为 `template_id`，将文件上传到 MinIO 的 `baseline-templates` Bucket 中，对象名为 `{template_id}/{original_filename}`。

**步骤三：创建数据库记录**。在 `templates` 表中插入一条新记录，状态设为 `parsing`，同时将 MinIO 对象名存入 `minio_object_name` 字段。

**步骤四：初始化 Redis 状态**。在 Redis 中创建 `template:parse:status:{template_id}` Hash Key，设置初始状态为 `{"status": "parsing", "progress": 0, "message": "文件已上传，等待解析..."}`。

**步骤五：触发异步解析**。将 `template_id` 投递到一个 Go channel（内部任务队列）中，由后台的解析 Worker 协程异步处理。前端收到 `201 Created` 响应后，开始轮询解析状态。

### 9.2 文件内容解析器设计 (`fileparser/`)

文件解析器模块采用策略模式设计，为每种支持的文件类型提供独立的解析器实现，所有解析器实现统一的 `FileParser` 接口。

```go
// FileParser 文件解析器接口
type FileParser interface {
    // Parse 解析文件内容，返回提取出的纯文本
    Parse(reader io.Reader) (string, error)
    // SupportedTypes 返回该解析器支持的文件扩展名列表
    SupportedTypes() []string
}
```

各解析器的实现方案如下表所示。

| 解析器 | 支持类型 | Go 依赖库 | 解析策略 |
|:---|:---|:---|:---|
| `PDFParser` | `.pdf` | `ledongthuc/pdf` 或调用外部 `pdftotext` 命令 | 逐页提取文本内容，保留段落结构。对于扫描版 PDF，记录警告并返回空文本。 |
| `WordParser` | `.docx` | `unidoc/unioffice` | 遍历文档中的所有段落（Paragraph）和表格（Table），按顺序提取文本。保留标题层级信息。 |
| `YAMLParser` | `.yaml`, `.yml` | `gopkg.in/yaml.v3` | 将 YAML 内容直接作为结构化文本返回，保留完整的键值对结构。 |
| `ExcelParser` | `.xlsx` | `excelize/v2` | 遍历所有工作表（Sheet），逐行逐列提取单元格内容，以制表符分隔列，换行符分隔行。 |
| `TextParser` | `.txt` | 标准库 `io` | 直接读取全部文本内容。 |

解析器工厂函数根据文件扩展名自动选择对应的解析器实例：

```go
// NewParser 根据文件类型返回对应的解析器
func NewParser(fileType string) (FileParser, error) {
    switch strings.ToLower(fileType) {
    case ".pdf":
        return &PDFParser{}, nil
    case ".docx":
        return &WordParser{}, nil
    case ".yaml", ".yml":
        return &YAMLParser{}, nil
    case ".xlsx":
        return &ExcelParser{}, nil
    case ".txt":
        return &TextParser{}, nil
    default:
        return nil, fmt.Errorf("unsupported file type: %s", fileType)
    }
}
```

### 9.3 文本预处理

文件解析器提取出原始文本后，需要经过预处理步骤以提高 LLM 解析的准确性。预处理包括以下操作：移除连续的空白行（保留最多一个空行）；移除页眉页脚中的页码信息；将制表符统一替换为空格；如果文本总长度超过 LLM 的上下文窗口限制（例如 32K tokens），则按章节或段落进行智能分片，每个分片保留适当的上下文重叠（约 500 字符）。

## 10. 大模型自动生成提示词与数据入库模块

### 10.1 模板解析 Worker

模板解析 Worker 是一个后台运行的协程，从内部任务队列中消费 `template_id`，执行完整的"文件解析 → LLM 提取规则 → 数据入库"流水线。

```
[任务队列] → [从MinIO下载文件] → [文件内容解析] → [文本预处理] → [构建LLM提示词]
    → [调用LLM提取规则] → [解析LLM返回的JSON] → [批量写入baseline_rules表]
    → [触发脚本生成] → [更新模板状态为completed]
```

Worker 在处理过程中，持续更新 Redis 中的解析状态，使前端能够通过轮询获取实时进度。

### 10.2 规则提取 Prompt 设计

规则提取是整个系统的核心智能环节。后端根据解析出的文件内容，自动构建结构化的 Prompt 发送给 LLM，要求其从文档中提取出所有基线检查规则。

**System Prompt（系统提示词）**：

```
你是一位专业的信息安全基线分析专家。你的任务是从用户提供的基线安全文档中，
精确地提取出每一条独立的基线检查规则。

你必须严格遵循以下要求：
1. 每条规则必须包含三个字段：title（规则标题）、check_content（检查方法描述）、
   fix_content（修复方法描述）。
2. check_content 应详细描述如何在 Linux 系统上验证该规则是否合规，包括需要检查的
   配置文件路径、命令输出、参数值等。
3. fix_content 应详细描述如何修复不合规的配置，包括需要修改的文件、命令、参数等。
4. 如果文档中某条规则只有检查方法没有修复方法（或反之），你应根据专业知识补充缺失的部分。
5. 你的输出必须是一个严格的 JSON 数组，不要包含任何 Markdown 格式标记或额外说明文字。

输出格式：
[
  {
    "title": "规则标题",
    "check_content": "详细的检查方法描述",
    "fix_content": "详细的修复方法描述"
  }
]
```

**User Prompt（用户提示词）模板**：

```
请从以下基线安全文档内容中提取所有基线检查规则。

--- 文档内容开始 ---
{extracted_text}
--- 文档内容结束 ---

请严格按照 JSON 数组格式输出所有提取到的规则。
```

对于超长文档（分片处理的情况），User Prompt 会附加分片上下文信息：

```
这是文档的第 {current_part} 部分（共 {total_parts} 部分）。
请仅从本部分内容中提取规则，不要重复之前已提取的规则。

--- 文档内容（第 {current_part}/{total_parts} 部分）开始 ---
{extracted_text_chunk}
--- 文档内容（第 {current_part}/{total_parts} 部分）结束 ---
```

### 10.3 LLM 返回结果解析与校验 (`llm/parser.go`)

LLM 返回的文本内容需要经过严格的解析和校验，才能写入数据库。解析流程如下。

**步骤一：JSON 提取**。LLM 的返回内容可能包含 Markdown 代码块标记（如 ` ```json ... ``` `），解析器首先使用正则表达式提取出纯 JSON 内容。如果未找到 JSON 代码块，则尝试直接将整个返回内容作为 JSON 解析。

**步骤二：JSON 反序列化**。将提取出的 JSON 字符串反序列化为 `[]RuleExtraction` 结构体数组。

```go
type RuleExtraction struct {
    Title        string `json:"title"`
    CheckContent string `json:"check_content"`
    FixContent   string `json:"fix_content"`
}
```

**步骤三：字段完整性校验**。遍历每个 `RuleExtraction` 对象，验证 `title`、`check_content`、`fix_content` 三个字段均不为空。对于字段缺失的规则，记录警告日志并跳过该条规则，不中断整体流程。

**步骤四：去重处理**。以 `title` 字段为键进行去重，避免 LLM 在分片处理时产生重复规则。

### 10.4 规则数据入库

规则入库操作在一个数据库事务中完成，确保原子性。具体流程如下。

**步骤一**：开启数据库事务。

**步骤二**：遍历校验通过的规则列表，逐条插入 `baseline_rules` 表。每条记录的 `template_id` 关联到当前正在处理的模板。

**步骤三**：更新 `templates` 表的状态字段为 `completed`。

**步骤四**：提交事务。如果任何步骤失败，回滚事务并将模板状态更新为 `failed`。

**步骤五**：更新 Redis 中的解析状态为 `{"status": "completed", "progress": 100, "message": "解析完成，共提取 N 条规则"}`。

**步骤六**：将规则列表投递到脚本生成队列，触发后续的检查脚本和修复脚本自动生成流程。

### 10.5 自动生成 Prompt 模板的逻辑

系统在将规则入库的同时，会为每个模板自动生成一个 `llm_prompt_template` 并回写到 `templates` 表的对应字段中。该 Prompt 模板是根据实际提取出的规则特征动态生成的，用于后续用户对同类型文档的快速解析。

生成逻辑如下：分析已提取规则的共性特征（如规则标题的命名模式、检查内容的结构特点），构建一个更精确的提示词模板。例如，如果提取出的规则标题均以"确保"开头，生成的 Prompt 模板会包含"请重点关注以'确保'开头的条目"等引导信息。

## 11. 脚本自动生成模块

### 11.1 脚本生成 Worker

脚本生成 Worker 是另一个后台协程，负责为每条基线规则自动生成可执行的 Shell 检查脚本和修复脚本。该 Worker 从脚本生成队列中消费规则 ID，逐条调用 LLM 生成脚本。

### 11.2 检查脚本生成 Prompt

**System Prompt**：

```
你是一位资深的 Linux 系统运维工程师和 Shell 脚本专家。你的任务是根据用户提供的
基线检查规则描述，编写一个可在 Linux 系统上直接执行的 Bash 检查脚本。

你必须严格遵循以下要求：
1. 脚本必须以 #!/bin/bash 开头。
2. 脚本必须使用 set -e 确保遇到错误时立即退出。
3. 脚本的退出码约定：0 表示检查通过（合规），1 表示检查不通过（不合规），
   2 表示检查过程中发生错误。
4. 脚本必须将检查过程和结果输出到 stdout，错误信息输出到 stderr。
5. 脚本不能进行任何修改操作，只能读取和检查。
6. 脚本应包含充分的注释说明检查逻辑。
7. 脚本应处理目标文件或配置不存在的边界情况。
8. 只输出纯脚本内容，不要包含任何 Markdown 格式标记。
```

**User Prompt 模板**：

```
请根据以下基线检查规则，编写一个 Bash 检查脚本。

规则标题：{rule_title}
检查内容：{check_content}

请直接输出可执行的 Bash 脚本内容。
```

### 11.3 修复脚本生成 Prompt

**System Prompt**：

```
你是一位资深的 Linux 系统运维工程师和 Shell 脚本专家。你的任务是根据用户提供的
基线修复规则描述，编写一个可在 Linux 系统上直接执行的 Bash 修复脚本。

你必须严格遵循以下要求：
1. 脚本必须以 #!/bin/bash 开头。
2. 脚本必须使用 set -e 确保遇到错误时立即退出。
3. 脚本的退出码约定：0 表示修复成功，1 表示修复失败，2 表示修复过程中发生错误。
4. 在执行任何修改操作之前，脚本必须先备份原始配置文件（备份到 /tmp/baseline-backup/ 目录）。
5. 脚本必须将修复过程和结果输出到 stdout，错误信息输出到 stderr。
6. 脚本应包含充分的注释说明修复逻辑。
7. 脚本应处理目标文件或配置不存在的边界情况。
8. 修复完成后，脚本应验证修复是否生效。
9. 只输出纯脚本内容，不要包含任何 Markdown 格式标记。
```

**User Prompt 模板**：

```
请根据以下基线修复规则，编写一个 Bash 修复脚本。

规则标题：{rule_title}
修复内容：{fix_content}

请直接输出可执行的 Bash 脚本内容。
```

### 11.4 脚本安全性校验

LLM 生成的脚本在存储和下发之前，必须经过安全性校验。校验规则如下表所示。

| 校验项 | 校验规则 | 处理方式 |
|:---|:---|:---|
| 危险命令检测 | 检查脚本中是否包含 `rm -rf /`、`mkfs`、`dd if=`、`:(){ :\|:& };:` 等破坏性命令 | 拒绝该脚本，标记为生成失败，记录告警日志 |
| Shebang 检查 | 确认脚本第一行为 `#!/bin/bash` | 如果缺失，自动补充 |
| 网络外联检测 | 检查脚本中是否包含 `curl`、`wget`、`nc` 等网络命令（检查脚本中不应出现） | 记录警告日志，但不阻止（修复脚本可能需要下载补丁） |
| 脚本长度检查 | 脚本内容不超过 64KB | 拒绝该脚本，要求 LLM 重新生成 |
| 语法检查 | 使用 `bash -n script.sh` 进行语法检查（如果后端环境支持） | 语法错误则要求 LLM 重新生成 |

### 11.5 脚本版本管理

每次 LLM 生成或修复脚本时，都会在 `script_versions` 表中创建一条新记录，保留完整的版本历史。同时，最新版本的脚本内容会更新到 `baseline_rules` 表的 `generated_check_script` 或 `generated_fix_script` 字段中，并上传到 MinIO 的 `generated-scripts` Bucket 中进行持久化存储。

## 12. 自愈修复模块

### 12.1 自愈流程概述

自愈修复是本系统的核心差异化功能。当 Agent 执行检查脚本或修复脚本失败时（退出码非 0 且非预期的检查不通过），后端会自动启动自愈流程：将脚本内容、执行环境信息和错误输出发送给 LLM，由 LLM 分析错误原因并生成修复后的脚本，然后重新下发执行。整个过程最多重试 3 次。

### 12.2 自愈触发条件

并非所有脚本执行失败都会触发自愈。触发条件的判断逻辑如下。

对于**检查脚本**：退出码为 0（合规）或 1（不合规）均为正常结果，不触发自愈；退出码为 2（检查过程出错）或其他非预期退出码时，触发自愈。

对于**修复脚本**：退出码为 0（修复成功）为正常结果，不触发自愈；退出码为 1（修复失败）或 2（修复出错）或执行超时时，触发自愈。

### 12.3 自愈 Prompt 设计

**System Prompt**：

```
你是一位资深的 Linux 系统运维工程师和 Shell 脚本调试专家。一个自动化脚本在目标
主机上执行失败了，你需要根据提供的错误信息分析失败原因，并生成修复后的脚本。

你必须严格遵循以下要求：
1. 仔细分析 stderr 和 stdout 中的错误信息，找出根本原因。
2. 根据错误原因修改原始脚本，确保修复后的脚本能够正确执行。
3. 常见的错误原因包括但不限于：
   - 命令不存在（需要安装对应的软件包或使用替代命令）
   - 文件路径不存在（需要创建目录或调整路径）
   - 权限不足（需要添加 sudo 或调整权限）
   - 语法错误（需要修正 Shell 语法）
   - 配置文件格式不同（需要适配目标系统的实际格式）
4. 修复后的脚本必须保持与原始脚本相同的功能目标和退出码约定。
5. 在修复的位置添加注释说明修复原因。
6. 只输出修复后的完整脚本内容，不要包含任何 Markdown 格式标记或额外说明。
```

**User Prompt 模板**：

```
以下脚本在目标主机上执行失败，请分析错误并生成修复后的脚本。

--- 原始脚本 ---
{original_script}

--- 执行环境 ---
操作系统：{os_type}
主机名：{hostname}

--- 标准输出 (stdout) ---
{stdout}

--- 标准错误 (stderr) ---
{stderr}

--- 退出码 ---
{exit_code}

{previous_attempts_context}

请输出修复后的完整脚本。
```

其中 `{previous_attempts_context}` 在第 2 次及以后的重试中会包含之前的修复尝试信息，帮助 LLM 避免重复相同的错误修复方向：

```
--- 之前的修复尝试 ---
第 1 次修复尝试同样失败，错误信息为：{previous_stderr_1}
第 2 次修复尝试同样失败，错误信息为：{previous_stderr_2}
请尝试不同的修复方案。
```

### 12.4 自愈执行流程

完整的自愈执行流程如下图所示（以文字描述）。

**步骤一：错误捕获**。gRPC 服务器从 Agent 接收到 `CommandResult`，`task_service` 判断退出码满足自愈触发条件。

**步骤二：初始化自愈状态**。在 Redis 中创建 `self_healing:{task_id}` Hash Key，设置 `attempt=1, status=healing, last_error={stderr}`。

**步骤三：构建自愈 Prompt**。`self_healing_service` 从数据库中获取原始脚本内容，结合 `CommandResult` 中的 stdout、stderr、exit_code 以及主机的 os_type 信息，构建完整的自愈 Prompt。

**步骤四：调用 LLM 生成修复脚本**。将构建好的 Prompt 发送给 LLM，获取修复后的脚本内容。

**步骤五：脚本安全性校验**。对 LLM 返回的修复脚本执行与初始脚本相同的安全性校验。

**步骤六：记录修复版本**。将修复后的脚本作为新版本写入 `script_versions` 表，并在 `self_healing_logs` 表中记录本次修复尝试的详细信息。

**步骤七：重新下发执行**。通过 gRPC Agent Manager 将修复后的脚本作为新的 `ServerCommand` 下发给目标 Agent。

**步骤八：等待结果**。等待 Agent 返回新的 `CommandResult`。如果执行成功，更新自愈状态为 `healed`，更新 `baseline_rules` 表中的脚本为修复后的版本；如果仍然失败且重试次数未达上限（3 次），回到步骤三继续；如果达到重试上限，更新自愈状态为 `failed`，记录最终错误信息。

**步骤九：通知前端**。无论自愈成功还是失败，都通过 Redis 的 `task:logs:{task_id}` 列表追加日志行，前端通过轮询获取自愈过程的实时状态。日志格式示例：

```
[SYSTEM] 脚本执行失败，正在尝试 AI 自我修复... (第 1/3 次)
[SYSTEM] AI 已生成修复脚本，正在重新执行...
[SYSTEM] 修复成功！脚本已更新为最新版本。
```

或：

```
[SYSTEM] 脚本执行失败，正在尝试 AI 自我修复... (第 3/3 次)
[SYSTEM] AI 自我修复已达最大重试次数，修复失败。请手动检查脚本。
```

### 12.5 自愈流程状态机

自愈流程的状态转换可以用以下状态机描述。

| 当前状态 | 触发事件 | 目标状态 | 动作 |
|:---|:---|:---|:---|
| `idle` | 脚本执行失败且满足自愈条件 | `healing` | 构建 Prompt，调用 LLM |
| `healing` | LLM 返回修复脚本 | `executing` | 安全校验，下发执行 |
| `healing` | LLM 调用失败 | `failed` | 记录错误，通知前端 |
| `executing` | Agent 返回成功结果 | `healed` | 更新脚本版本，通知前端 |
| `executing` | Agent 返回失败结果且 attempt < 3 | `healing` | attempt++，重新构建 Prompt |
| `executing` | Agent 返回失败结果且 attempt >= 3 | `failed` | 记录最终错误，通知前端 |

## 13. 任务编排与执行模块

### 13.1 任务下发流程

当前端调用 `POST /api/v1/tasks/run-check` 或 `POST /api/v1/tasks/run-fix` 接口时，`task_service` 执行以下编排逻辑。

**步骤一**：根据 `rule_id` 从数据库查询对应的基线规则，获取 `generated_check_script` 或 `generated_fix_script`。如果脚本尚未生成（字段为 NULL），返回错误提示"脚本尚未生成，请等待 AI 生成完成"。

**步骤二**：生成一个 `task_group_id`（UUID），用于关联本次批量任务中的所有子任务。

**步骤三**：遍历 `host_ids` 列表，为每台主机创建一个独立的子任务。对于每个子任务：生成唯一的 `task_id`；在 `task_logs` 表中插入一条状态为 `RUNNING` 的记录；在 Redis 中创建 `task:status:{task_id}` 和 `task:logs:{task_id}` Key；通过 gRPC Agent Manager 查找目标主机的活跃连接，构建 `ServerCommand` 消息（包含 `task_id`、脚本内容和超时时间）并通过 gRPC 流下发。

**步骤四**：如果目标主机当前不在线（Redis 中不存在对应的心跳 Key），则将该子任务标记为 `FAILED`，错误信息为"目标主机离线"。

**步骤五**：返回 `task_group_id` 给前端，前端使用此 ID 轮询任务状态。

### 13.2 任务结果处理

gRPC 服务器从 Agent 接收到 `CommandResult` 后，执行以下处理逻辑。

**步骤一**：根据 `task_id` 更新 Redis 中的任务状态和日志。

**步骤二**：更新数据库 `task_logs` 表中对应记录的 `status`、`stdout`、`stderr`、`exit_code` 和 `finished_at` 字段。

**步骤三**：判断是否需要触发自愈流程（参见第 12.2 节的触发条件）。

**步骤四**：如果不需要自愈，将最终状态写入 Redis，前端下次轮询时即可获取完整结果。

## 14. 后端服务启动流程

`main.go` 中的服务启动遵循严格的初始化顺序，确保所有依赖在使用前已就绪。

**第一步：加载配置**。调用 `config.Load()` 从 YAML 文件和环境变量加载配置。

**第二步：初始化数据库连接**。调用 `repository.NewDB()` 创建 PostgreSQL 连接池并验证连通性。

**第三步：初始化 Redis 连接**。调用 `storage.NewRedisClient()` 创建 Redis 客户端并验证连通性。

**第四步：初始化 MinIO 客户端**。调用 `storage.NewMinIOClient()` 创建 MinIO 客户端并确保 Bucket 存在。

**第五步：初始化 Repository 层**。创建所有 Repository 实例，注入数据库连接。

**第六步：初始化 LLM 客户端**。创建 LLM 客户端实例，注入 Redis 和 Config Repository。

**第七步：初始化 Service 层**。创建所有 Service 实例，注入 Repository、LLM 客户端和存储客户端。

**第八步：启动后台 Worker**。启动模板解析 Worker 和脚本生成 Worker 协程。

**第九步：启动 gRPC 服务器**。在独立的 goroutine 中启动 gRPC 服务器，监听配置的端口。

**第十步：启动 HTTP 服务器**。在主 goroutine 中启动 Gin HTTP 服务器，监听配置的端口。

**第十一步：优雅关闭**。监听系统信号（SIGINT、SIGTERM），收到信号后依次关闭 HTTP 服务器、gRPC 服务器、后台 Worker、数据库连接和 Redis 连接。

## 15. 错误处理与日志规范

### 15.1 错误处理策略

后端采用分层错误处理策略。Repository 层返回原始的数据库错误，由 Service 层进行业务语义的包装（例如将"record not found"转换为"主机不存在"）。Handler 层负责将 Service 层的错误转换为标准的 HTTP 错误响应格式。

所有对外返回的错误响应遵循统一格式：

```json
{
  "error": {
    "code": "TEMPLATE_PARSE_FAILED",
    "message": "模板解析失败：LLM 返回的内容不是有效的 JSON 格式"
  }
}
```

### 15.2 日志规范

后端使用结构化日志库（如 `zap` 或 `logrus`），所有日志输出为 JSON 格式，便于日志收集和分析。日志级别使用规范如下。

| 级别 | 使用场景 |
|:---|:---|
| `DEBUG` | 开发调试信息，如 SQL 语句、LLM Prompt 内容 |
| `INFO` | 正常业务流程，如"模板解析完成"、"Agent 注册成功" |
| `WARN` | 非致命异常，如"LLM 返回的某条规则字段缺失，已跳过" |
| `ERROR` | 业务错误，如"数据库写入失败"、"LLM API 调用失败" |
| `FATAL` | 启动失败，如"无法连接数据库"，程序将退出 |
