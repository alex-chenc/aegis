# 基础设施与部署设计文档 - V2.0 完整版

**版本**: 2.0
**状态**: 定稿
**作者**: Manus AI

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 2.0 | 2026-03-05 | Manus AI | **全新文档**。补充 V1.6 中缺失的基础设施详细设计，包含 Docker Compose 完整配置、PostgreSQL 初始化与调优、Redis 部署与配置、MinIO 部署与 Bucket 策略、网络规划、健康检查、数据持久化与备份策略。 |

## 2. 概述

本文档为自动化基线检查与自愈系统的基础设施层提供完整的部署设计规范。系统采用 Docker Compose 进行容器编排，所有基础设施组件（PostgreSQL、Redis、MinIO）和应用组件（Backend、Frontend）均以容器形式运行。本文档详细定义了每个组件的配置参数、网络规划、数据持久化策略和健康检查机制。

V1.6 版本的设计文档中，虽然在综合设计概览中提到了 Redis 和 MinIO 作为技术栈的一部分，但缺少完整的 `docker-compose.yml` 配置文件，也没有对各基础设施组件的部署参数进行详细说明。本文档旨在填补这一空白。

## 3. 系统部署架构

整个系统的部署架构由以下六个容器组成，运行在同一个 Docker 网络中。

| 容器名称 | 镜像 | 对外端口 | 职责 |
|:---|:---|:---|:---|
| `frontend` | `baseline-system/frontend:latest` | `80:80` | Nginx 托管前端静态资源，反向代理 API 请求到后端 |
| `backend` | `baseline-system/backend:latest` | `8080:8080`, `9090:9090` | Go 后端服务，提供 HTTP API 和 gRPC 服务 |
| `postgres` | `postgres:14-alpine` | `5432:5432` (可选，仅开发环境) | 关系型数据库，持久化所有业务数据 |
| `redis` | `redis:7-alpine` | `6379:6379` (可选，仅开发环境) | 高速缓存，存储 Agent 状态和任务实时数据 |
| `minio` | `minio/minio:latest` | `9000:9000`, `9001:9001` | 对象存储，存储模板文件、Agent 二进制和生成的脚本 |
| `minio-init` | `minio/mc:latest` | 无 | 一次性初始化容器，创建 MinIO Bucket 和访问策略 |

## 4. Docker Compose 完整配置

以下是生产就绪的 `docker-compose.yml` 完整配置文件。

```yaml
# docker-compose.yml
# 自动化基线检查与自愈系统 - 容器编排配置
# 版本: V2.0

version: "3.8"

services:
  # ============================================================
  # 基础设施层
  # ============================================================

  # PostgreSQL 关系型数据库
  postgres:
    image: postgres:14-alpine
    container_name: baseline-postgres
    restart: unless-stopped
    environment:
      POSTGRES_DB: baseline_db
      POSTGRES_USER: baseline_user
      POSTGRES_PASSWORD: ${DB_PASSWORD:-a_strong_db_password}
      POSTGRES_INITDB_ARGS: "--encoding=UTF8 --locale=C"
      # 性能调优参数（通过 command 传递）
    command:
      - "postgres"
      - "-c" 
      - "max_connections=100"
      - "-c"
      - "shared_buffers=256MB"
      - "-c"
      - "effective_cache_size=768MB"
      - "-c"
      - "maintenance_work_mem=128MB"
      - "-c"
      - "checkpoint_completion_target=0.9"
      - "-c"
      - "wal_buffers=16MB"
      - "-c"
      - "default_statistics_target=100"
      - "-c"
      - "random_page_cost=1.1"
      - "-c"
      - "effective_io_concurrency=200"
      - "-c"
      - "work_mem=4MB"
      - "-c"
      - "min_wal_size=1GB"
      - "-c"
      - "max_wal_size=4GB"
      - "-c"
      - "log_statement=mod"
      - "-c"
      - "log_min_duration_statement=1000"
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./scripts/init.sql:/docker-entrypoint-initdb.d/01-init.sql:ro
    ports:
      - "${DB_PORT:-5432}:5432"
    networks:
      - baseline-network
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U baseline_user -d baseline_db"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 30s

  # Redis 高速缓存
  redis:
    image: redis:7-alpine
    container_name: baseline-redis
    restart: unless-stopped
    command:
      - "redis-server"
      - "--requirepass"
      - "${REDIS_PASSWORD:-a_strong_redis_password}"
      - "--maxmemory"
      - "256mb"
      - "--maxmemory-policy"
      - "allkeys-lru"
      - "--appendonly"
      - "yes"
      - "--appendfsync"
      - "everysec"
      - "--tcp-keepalive"
      - "60"
      - "--timeout"
      - "300"
      - "--databases"
      - "4"
    volumes:
      - redis_data:/data
    ports:
      - "${REDIS_PORT:-6379}:6379"
    networks:
      - baseline-network
    healthcheck:
      test: ["CMD", "redis-cli", "-a", "${REDIS_PASSWORD:-a_strong_redis_password}", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s

  # MinIO 对象存储
  minio:
    image: minio/minio:latest
    container_name: baseline-minio
    restart: unless-stopped
    environment:
      MINIO_ROOT_USER: ${MINIO_ACCESS_KEY:-minio_admin}
      MINIO_ROOT_PASSWORD: ${MINIO_SECRET_KEY:-a_third_strong_secret_password}
    command: server /data --console-address ":9001"
    volumes:
      - minio_data:/data
    ports:
      - "${MINIO_API_PORT:-9000}:9000"
      - "${MINIO_CONSOLE_PORT:-9001}:9001"
    networks:
      - baseline-network
    healthcheck:
      test: ["CMD", "mc", "ready", "local"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 15s

  # MinIO 初始化容器（一次性运行，创建 Bucket）
  minio-init:
    image: minio/mc:latest
    container_name: baseline-minio-init
    depends_on:
      minio:
        condition: service_healthy
    entrypoint: >
      /bin/sh -c "
      mc alias set myminio http://minio:9000 $${MINIO_ACCESS_KEY} $${MINIO_SECRET_KEY};
      mc mb myminio/baseline-templates --ignore-existing;
      mc mb myminio/agent-artifacts --ignore-existing;
      mc mb myminio/generated-scripts --ignore-existing;
      mc anonymous set download myminio/agent-artifacts;
      echo 'MinIO initialization complete.';
      exit 0;
      "
    environment:
      MINIO_ACCESS_KEY: ${MINIO_ACCESS_KEY:-minio_admin}
      MINIO_SECRET_KEY: ${MINIO_SECRET_KEY:-a_third_strong_secret_password}
    networks:
      - baseline-network

  # ============================================================
  # 应用层
  # ============================================================

  # Go 后端服务
  backend:
    image: baseline-system/backend:latest
    container_name: baseline-backend
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      minio:
        condition: service_healthy
    environment:
      # 数据库配置
      DATABASE_HOST: postgres
      DATABASE_PORT: 5432
      DATABASE_USER: baseline_user
      DATABASE_PASSWORD: ${DB_PASSWORD:-a_strong_db_password}
      DATABASE_DBNAME: baseline_db
      DATABASE_SSLMODE: disable
      DATABASE_MAX_OPEN_CONNS: 25
      DATABASE_MAX_IDLE_CONNS: 10
      DATABASE_CONN_MAX_LIFETIME: 300
      # Redis 配置
      REDIS_HOST: redis
      REDIS_PORT: 6379
      REDIS_PASSWORD: ${REDIS_PASSWORD:-a_strong_redis_password}
      REDIS_DB: 0
      REDIS_POOL_SIZE: 20
      # MinIO 配置
      MINIO_ENDPOINT: minio:9000
      MINIO_ACCESS_KEY: ${MINIO_ACCESS_KEY:-minio_admin}
      MINIO_SECRET_KEY: ${MINIO_SECRET_KEY:-a_third_strong_secret_password}
      MINIO_USE_SSL: "false"
      # 服务配置
      SERVER_HTTP_PORT: 8080
      SERVER_GRPC_PORT: 9090
      SERVER_EXTERNAL_IP: ${EXTERNAL_IP:-}
      # Agent 配置
      AGENT_AUTH_TOKEN: ${AGENT_TOKEN:-a_very_secret_agent_token}
      AGENT_HEARTBEAT_TIMEOUT: 90
      AGENT_SCRIPT_TIMEOUT: 300
      # 自愈配置
      SELF_HEALING_MAX_RETRIES: 3
      SELF_HEALING_ENABLED: "true"
    ports:
      - "8080:8080"
      - "9090:9090"
    networks:
      - baseline-network
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/health"]
      interval: 15s
      timeout: 5s
      retries: 3
      start_period: 30s

  # Vue 前端 (Nginx)
  frontend:
    image: baseline-system/frontend:latest
    container_name: baseline-frontend
    restart: unless-stopped
    depends_on:
      backend:
        condition: service_healthy
    ports:
      - "80:80"
    networks:
      - baseline-network
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:80"]
      interval: 15s
      timeout: 5s
      retries: 3
      start_period: 10s

# ============================================================
# 网络配置
# ============================================================
networks:
  baseline-network:
    driver: bridge
    ipam:
      config:
        - subnet: 172.28.0.0/16

# ============================================================
# 数据卷配置
# ============================================================
volumes:
  postgres_data:
    driver: local
  redis_data:
    driver: local
  minio_data:
    driver: local
```

## 5. 环境变量配置 (`.env`)

为了安全管理敏感信息，所有密码和密钥通过 `.env` 文件注入，该文件不应提交到版本控制系统中。

```bash
# .env - 环境变量配置文件（请勿提交到 Git）

# 数据库
DB_PASSWORD=your_strong_db_password_here
DB_PORT=5432

# Redis
REDIS_PASSWORD=your_strong_redis_password_here
REDIS_PORT=6379

# MinIO
MINIO_ACCESS_KEY=minio_admin
MINIO_SECRET_KEY=your_strong_minio_password_here
MINIO_API_PORT=9000
MINIO_CONSOLE_PORT=9001

# Agent
AGENT_TOKEN=your_strong_agent_token_here

# 外部访问IP（用于生成Agent安装命令）
# 留空则后端自动检测（优先公网IP）；填写则直接使用该值
EXTERNAL_IP=
```

### 5.1 `EXTERNAL_IP` 配置说明

`EXTERNAL_IP` 环境变量用于控制后端生成的 Agent 安装命令中的服务器地址。该变量的行为如下。

**如果 `EXTERNAL_IP` 为空（推荐）**：后端在启动时自动检测自身对外可达的 IP 地址，优先级顺序为（1）公网 IP 查询服务 → （2）出站连接本机 IP → （3）枚举本机网卡 → （4）兜底 127.0.0.1。这种方式适合大多数部署场景，无需管理员手动干预。

**如果 `EXTERNAL_IP` 已填写**：后端直接使用该值，跳过所有自动检测步骤。这种方式适合管理员明确知道服务器对外 IP 地址的场景（例如已知的公网 IP 或内网固定 IP）。

### 5.2 IP 检测过程

后端在启动时执行以下 IP 检测流程（仅在 `EXTERNAL_IP` 为空时）。

1. **公网 IP 查询**：依次请求 `https://api.ipify.org`、`https://ifconfig.me/ip`、`https://icanhazip.com` 等服务，获取服务器的公网 IP。每个请求超时时间为 3 秒。
2. **出站连接检测**：向 `8.8.8.8:80` 建立 UDP 连接（不实际发送数据），获取本机用于对外通讯的网卡 IP。
3. **本机网卡枚举**：遍历所有网络接口，过滤掉回环地址和 Docker 虚拟网卡，返回第一个有效的 IPv4 地址。
4. **兜底处理**：如果所有方法都失败，返回 `127.0.0.1`，并在日志中输出 WARN 级别警告。

检测结果在后端启动时输出到日志，格式如下。

```
[INFO] Detected server IP: 203.0.113.10
[INFO] Agent install command: curl -sSL http://203.0.113.10:8080/api/v1/agent/install.sh | sudo bash
```

## 6. PostgreSQL 详细配置

### 6.1 初始化流程

PostgreSQL 容器首次启动时，会自动执行 `/docker-entrypoint-initdb.d/` 目录下的 SQL 脚本。系统将 `init.sql` 挂载到该目录，实现数据库的自动初始化。初始化脚本包含创建扩展、建表、建索引和创建触发器等所有操作（详见数据库设计文档 V2.0）。

### 6.2 性能调优参数说明

以下是 `docker-compose.yml` 中 PostgreSQL 性能调优参数的详细说明，这些参数针对 4GB 内存、2 核 CPU 的典型部署环境进行了优化。

| 参数 | 值 | 说明 |
|:---|:---|:---|
| `max_connections` | 100 | 最大并发连接数，后端连接池最大 25 个连接，留有充足余量 |
| `shared_buffers` | 256MB | 共享缓冲区大小，建议设为系统内存的 1/4 |
| `effective_cache_size` | 768MB | 操作系统文件缓存的估计值，建议设为系统内存的 3/4 |
| `maintenance_work_mem` | 128MB | 维护操作（VACUUM、CREATE INDEX）的内存限制 |
| `work_mem` | 4MB | 每个排序/哈希操作的内存限制 |
| `wal_buffers` | 16MB | WAL 写入缓冲区大小 |
| `checkpoint_completion_target` | 0.9 | 检查点完成目标，减少 I/O 峰值 |
| `log_statement` | mod | 记录所有修改数据的 SQL 语句（INSERT/UPDATE/DELETE） |
| `log_min_duration_statement` | 1000 | 记录执行时间超过 1 秒的慢查询 |

### 6.3 数据持久化

PostgreSQL 的数据目录 `/var/lib/postgresql/data` 通过 Docker Named Volume `postgres_data` 进行持久化。即使容器被删除和重建，数据也不会丢失。

### 6.4 备份策略

建议在生产环境中配置以下备份策略。

**日常备份**：使用 `pg_dump` 每天凌晨执行一次全量逻辑备份，备份文件保存到独立的存储卷或远程存储中。可通过在宿主机上配置 crontab 实现：

```bash
# 每天凌晨 2:00 执行数据库备份
0 2 * * * docker exec baseline-postgres pg_dump -U baseline_user -d baseline_db -F c -f /tmp/backup_$(date +\%Y\%m\%d).dump && docker cp baseline-postgres:/tmp/backup_$(date +\%Y\%m\%d).dump /opt/backups/
```

**WAL 归档**：对于需要时间点恢复（PITR）的场景，可启用 WAL 归档功能，将 WAL 日志持续归档到外部存储。

## 7. Redis 详细配置

### 7.1 内存管理

Redis 配置了 256MB 的最大内存限制和 `allkeys-lru` 淘汰策略。这意味着当内存使用达到上限时，Redis 会优先淘汰最近最少使用的 Key。由于系统中 Redis 主要存储的是临时状态数据（心跳、任务状态），这种淘汰策略是合理的。

### 7.2 持久化配置

Redis 启用了 AOF（Append Only File）持久化，配置为 `appendfsync everysec`（每秒同步一次）。这在性能和数据安全性之间取得了平衡——最多丢失 1 秒的数据。对于本系统而言，Redis 中存储的数据均为可重建的缓存数据（心跳状态可通过 Agent 重新上报恢复，任务状态可从数据库重建），因此即使发生少量数据丢失也不会影响系统的业务完整性。

### 7.3 数据库分区

Redis 配置了 4 个数据库（DB 0-3），各数据库的用途规划如下。

| DB 编号 | 用途 | 存储的 Key 类型 |
|:---|:---|:---|
| DB 0 | 主业务数据 | `agent:*`、`template:*`、`task:*`、`config:*`、`self_healing:*` |
| DB 1 | 预留（测试环境） | 与 DB 0 相同的 Key 结构，用于集成测试 |
| DB 2 | 预留（未来扩展） | - |
| DB 3 | 预留（未来扩展） | - |

### 7.4 连接管理

Redis 配置了 `tcp-keepalive 60`（每 60 秒发送一次 TCP keepalive 探测）和 `timeout 300`（空闲连接 300 秒后自动关闭）。这些配置确保了连接的健康管理，避免僵尸连接占用资源。

## 8. MinIO 详细配置

### 8.1 Bucket 初始化

MinIO 的 Bucket 初始化通过一个独立的一次性容器 `minio-init` 完成。该容器在 MinIO 服务健康后自动运行，使用 MinIO Client (`mc`) 创建三个 Bucket 并配置访问策略，然后退出。

| Bucket | 访问策略 | 说明 |
|:---|:---|:---|
| `baseline-templates` | 私有 | 仅后端服务通过 SDK 访问，存储用户上传的基线模板文件 |
| `agent-artifacts` | 公开下载 | 允许匿名下载，Agent 安装脚本直接通过 URL 下载二进制文件 |
| `generated-scripts` | 私有 | 仅后端服务通过 SDK 访问，存储 LLM 生成的脚本文件 |

### 8.2 MinIO 管理控制台

MinIO 提供了一个 Web 管理控制台，通过 `9001` 端口访问。管理员可以通过该控制台查看 Bucket 内容、监控存储使用情况和管理访问策略。在生产环境中，建议将 `9001` 端口的外部访问限制在管理网络中。

### 8.3 数据持久化

MinIO 的数据目录 `/data` 通过 Docker Named Volume `minio_data` 进行持久化。所有上传的文件（模板、Agent 二进制、脚本）都存储在此卷中。

## 9. Nginx 反向代理配置

前端容器使用 Nginx 托管静态资源，同时作为反向代理将 API 请求转发到后端服务。以下是完整的 Nginx 配置文件。

```nginx
# frontend/nginx.conf

server {
    listen 80;
    server_name _;

    # 前端静态资源
    root /usr/share/nginx/html;
    index index.html;

    # Vue Router History 模式支持
    location / {
        try_files $uri $uri/ /index.html;
    }

    # API 反向代理
    location /api/ {
        proxy_pass http://backend:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 文件上传大小限制
        client_max_body_size 50m;

        # 超时配置（LLM 调用可能较慢）
        proxy_connect_timeout 10s;
        proxy_read_timeout 300s;
        proxy_send_timeout 60s;
    }

    # 健康检查端点代理
    location /health {
        proxy_pass http://backend:8080;
    }

    # 静态资源缓存
    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        expires 30d;
        add_header Cache-Control "public, immutable";
    }

    # 禁止访问隐藏文件
    location ~ /\. {
        deny all;
    }
}
```

## 10. 网络安全规划

### 10.1 端口暴露策略

在生产环境中，应严格控制对外暴露的端口。

| 端口 | 服务 | 是否对外暴露 | 说明 |
|:---|:---|:---|:---|
| `80` | Frontend (Nginx) | 是 | 用户通过浏览器访问的入口 |
| `8080` | Backend (HTTP API) | 视情况 | 如果 Nginx 已做反向代理，可不对外暴露 |
| `9090` | Backend (gRPC) | 是 | Agent 需要通过此端口与后端通讯 |
| `5432` | PostgreSQL | 否（仅开发环境） | 生产环境不应对外暴露数据库端口 |
| `6379` | Redis | 否（仅开发环境） | 生产环境不应对外暴露 Redis 端口 |
| `9000` | MinIO API | 否 | 仅内部网络访问 |
| `9001` | MinIO Console | 否（仅管理网络） | 仅管理员通过内部网络访问 |

### 10.2 容器间通讯

所有容器通过 Docker 自定义桥接网络 `baseline-network` 进行通讯。容器间使用服务名（如 `postgres`、`redis`、`minio`）作为主机名进行 DNS 解析，无需使用 IP 地址。

## 11. 健康检查机制

每个容器都配置了健康检查（healthcheck），Docker Compose 通过健康检查结果管理容器的启动顺序和依赖关系。

| 服务 | 检查方式 | 检查间隔 | 超时时间 | 重试次数 | 启动等待 |
|:---|:---|:---|:---|:---|:---|
| PostgreSQL | `pg_isready` 命令 | 10s | 5s | 5 | 30s |
| Redis | `redis-cli ping` 命令 | 10s | 5s | 5 | 10s |
| MinIO | `mc ready local` 命令 | 10s | 5s | 5 | 15s |
| Backend | HTTP GET `/health` | 15s | 5s | 3 | 30s |
| Frontend | HTTP GET `/` | 15s | 5s | 3 | 10s |

## 12. 一键部署流程

### 12.1 首次部署

首次部署系统的完整步骤如下。

**第一步：准备环境**。确保目标服务器已安装 Docker（20.10+）和 Docker Compose（2.0+）。

**第二步：克隆项目**。将项目代码克隆到服务器上。

**第三步：配置环境变量**。复制 `.env.example` 为 `.env`，修改其中的密码和密钥为安全的随机值。关于 `EXTERNAL_IP` 的配置，请参见第 5.1 节说明。

```bash
cp .env.example .env
# 编辑 .env，修改密码和密钥
# EXTERNAL_IP 通常留空，让后端自动检测
```

**第四步：构建镜像**。依次执行各子项目的构建脚本。

```bash
# 构建后端镜像
cd backend && ./build.sh && cd ..

# 构建前端镜像
cd frontend && ./build.sh && cd ..

# 构建 Agent 并上传到 MinIO（需要 MinIO 先启动）
# 此步骤在 docker-compose up 之后执行
```

**第五步：启动服务**。

```bash
docker-compose up -d
```

**第六步：验证部署**。

```bash
# 检查所有容器状态
docker-compose ps

# 检查后端健康状态
curl http://localhost:8080/health

# 查看后端日志，确认 IP 检测成功
docker-compose logs backend | grep "Detected server IP"

# 访问前端页面
# 浏览器打开 http://<server_ip>
```

**第七步：构建并上传 Agent**。

```bash
cd agent && ./build.sh && cd ..
```

**第八步：验证 Agent 安装命令**。

在浏览器中访问后端 API 验证 Agent 安装命令是否已正确生成：

```bash
curl http://<server_ip>:8080/api/v1/agent/install-command
```

返回的 `command` 字段应该包含自动检测到的服务器 IP 地址，例如：

```json
{
  "command": "curl -sSL http://203.0.113.10:8080/api/v1/agent/install.sh | sudo bash",
  "server_ip": "203.0.113.10",
  "http_port": 8080,
  "grpc_port": 9090
}
```

### 12.2 更新部署

更新系统时，只需重新构建变更的组件镜像，然后重启对应的容器。

```bash
# 例如更新后端
cd backend && ./build.sh && cd ..
docker-compose up -d backend
```

### 12.3 数据重置

如需完全重置系统数据（包括数据库、缓存和文件存储），执行以下命令。

```bash
docker-compose down -v
docker-compose up -d
```

> **警告**：`-v` 参数会删除所有 Docker Named Volume，包括数据库数据、Redis 数据和 MinIO 文件。此操作不可逆。

## 13. 监控与运维

### 13.1 日志查看

所有容器的日志可通过 Docker Compose 命令查看。

```bash
# 查看所有服务日志
docker-compose logs -f

# 查看特定服务日志
docker-compose logs -f backend

# 查看最近 100 行日志
docker-compose logs --tail=100 backend

# 查看后端 IP 检测结果
docker-compose logs backend | grep -i "ip\|agent\|install"
```

### 13.2 IP 地址变更

如果服务器的对外 IP 地址发生变化（例如重启后获得新的公网 IP），需要重启后端服务以重新检测 IP 地址。

```bash
# 重启后端服务
docker-compose restart backend

# 验证新的 IP 地址
curl http://localhost:8080/api/v1/agent/install-command
```

如果希望使用固定的 IP 地址而不依赖自动检测，可以修改 `.env` 文件中的 `EXTERNAL_IP` 字段，然后重启后端服务。

### 13.3 资源监控

建议在生产环境中部署 `cAdvisor` 或 `Prometheus + Grafana` 对容器资源使用情况进行监控。关键监控指标包括：各容器的 CPU 和内存使用率、PostgreSQL 的连接数和查询延迟、Redis 的内存使用率和命中率、MinIO 的存储使用量。
