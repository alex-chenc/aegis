---
name: aegis-build-test
description: Aegis 系统构建测试技能 - agent/server/dc/api-server 构建部署和 gRPC 数据流测试
version: 1.2.0
source: manual-creation
---

# Aegis 系统构建测试技能

本技能用于 AI 学习 Aegis 系统架构、构建部署流程和 API 测试。

## 服务端口总览

| 服务 | HTTP 端口 | gRPC 端口 | 容器名 |
|------|-----------|-----------|--------|
| api-server | 8082 | 19093 | aegis-api-server |
| server | - | 19090, 19094 | aegis-server |
| dc | - | 19092 | aegis-dc |
| frontend | 8081 | - | aegis-frontend |
| postgres | 5432 | - | aegis-postgres |
| redis | 6379 | - | aegis-redis |
| minio | 9000/9001 | - | aegis-minio |
| kafka | 29092 | - | aegis-kafka |

---

## 一、AI 必读文档

### 架构设计文档 (v5.5)
```bash
cat aegis_system_design_v5.5/README.md
cat aegis_system_design_v5.5/architecture_design_v5.5.md
cat aegis_system_design_v5.5/communication_protocol_design_v5.5.md
cat aegis_system_design_v5.5/backend_detailed_design_v5.5_complete.md
cat aegis_system_design_v5.5/agent_detailed_design_v5.5_complete.md
cat aegis_system_design_v5.5/frontend_detailed_design_v5.5_complete.md
cat aegis_system_design_v5.5/database_structure_design_v5.5_complete.md
```

---

## 二、构建命令

### 单组件镜像构建

适用于：修改单个服务代码后快速构建部署

```bash
# 仅构建单个服务镜像（不启动）
docker compose build <service>

# 构建并启动单个服务（会重建容器）
docker compose up -d --build <service>

# 可用服务: api-server, server, dc, frontend
```

**示例：修改 api-server 后快速部署**
```bash
docker compose up -d --build api-server
docker compose logs -f api-server   # 查看日志确认启动
```

### 完整组件构建

```bash
# API Server
cd api-server && make build

# Server
cd server && make build

# DC
cd dc && make build

# Agent（必须在 aegis-agent-builder-ubi8:5.8.0 容器内构建）
# 步骤1: 检查本地是否已存在构建镜像
docker image inspect aegis-agent-builder-ubi8:5.8.0 > /dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "构建基础镜像 aegis-agent-builder-ubi8:5.8.0..."
  cd agent && make docker-base-image
fi

# 步骤2: 使用容器化构建 Agent
cd agent && make docker-build
```

**Agent 构建说明**：
- Agent 的 eBPF 编译依赖 `aegis-agent-builder-ubi8:5.8.0` 镜像（UBI 8 + Go + clang + llvm + libbpf headers）
- 该镜像基于 `docker/ebpf-builder-base/Dockerfile` 构建
- `make docker-build` 会自动先检查并构建基础镜像，然后在容器内完成 eBPF 编译和 Agent 构建
- 构建产物镜像为 `aegis-agent-artifacts:local`
- 如需单独构建基础镜像：`cd agent && make docker-base-image`

### Agent 容器化构建详解

**为什么必须用容器构建？**
Agent 包含 eBPF 程序，编译需要特定的内核头文件、clang/llvm 工具链和 libbpf 头文件。`aegis-agent-builder-ubi8:5.8.0` 镜像提供了完整且一致的构建环境。

**构建流程**：

```bash
# 1. 检查基础镜像是否存在
docker image inspect aegis-agent-builder-ubi8:5.8.0 > /dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "基础镜像不存在，开始构建..."
  # 方式A: 使用 Makefile（推荐）
  cd agent && make docker-base-image

  # 方式B: 直接使用 docker build
  # docker build -f docker/ebpf-builder-base/Dockerfile -t aegis-agent-builder-ubi8:5.8.0 .
fi

# 2. 容器内构建 Agent（eBPF + Go 二进制 + 打包）
cd agent && make docker-build

# 3. 从构建产物镜像中提取 Agent 包（可选）
# docker create --name agent-extract aegis-agent-artifacts:local
# docker cp agent-extract:/out/ ./dist/
# docker rm agent-extract
```

**构建产物**：
- `aegis-agent-artifacts:local`：包含编译好的 Agent 二进制和 eBPF 对象文件
- 产物路径在容器内为 `/out/`，包含 `aegis-agent-linux-amd64` 和 `bpf/*.bpf.o`

**常用 Makefile Target**：
| Target | 说明 |
|--------|------|
| `make docker-base-image` | 构建 `aegis-agent-builder-ubi8:5.8.0` 基础镜像 |
| `make docker-build` | 容器内完整构建 Agent（依赖基础镜像） |
| `make bpf` | 仅编译 eBPF 程序（需在容器内执行） |
| `make build` | 仅编译 Go 二进制（需在容器内执行） |
| `make package` | 打包构建产物 |
| `make upload` | 上传到 MinIO |

---

## 三、服务管理命令

### 3.1 基础生命周期管理

```bash
# 启动/停止/重启单个服务
docker compose start <service>
docker compose stop <service>
docker compose restart <service>

# 重建并重启（代码修改后使用）
docker compose up -d --build <service>
```

### 3.2 日志与状态查看

```bash
# 查看实时日志
docker compose logs -f <service>

# 查看服务状态
docker compose ps

# 检查服务健康
docker compose exec <service> wget --spider -q localhost:<port>/health 2>/dev/null && echo "OK" || echo "FAIL"
```

### 3.3 完整栈管理

```bash
# 启动/停止所有服务
docker compose up -d
docker compose down

# 停止并清除数据卷（慎用！）
docker compose down -v

# 重新构建所有镜像
docker compose build --no-cache
docker compose up -d
```

---

## 四、服务健康检查矩阵

| 服务 | 检查命令 | 期望输出 |
|------|---------|---------|
| api-server | `curl -s http://localhost:8082/health` | `{"status":"ok"}` |
| server | `nc -z localhost 19090 && echo OK` | `OK` |
| dc | `nc -z localhost 19092 && echo OK` | `OK` |
| postgres | `pg_isready -U aegis_user -d aegis_db` | `accepting connections` |
| redis | `redis-cli -a <password> ping` | `PONG` |
| minio | `mc ready local` | `OK` |

---

## 五、Agent 包上传到 MINIO

### 5.1 环境变量配置

MINIO 服务地址: `http://localhost:9000`
MINIO Console: `http://localhost:9001`

**IMPORTANT**: Credentials must NEVER be hardcoded. Users MUST provide their MINIO credentials in the conversation when needed.

在 `.env` 文件中配置（使用用户提供的凭证）：
```bash
MINIO_ACCESS_KEY=<USER_PROVIDED_ACCESS_KEY>
MINIO_SECRET_KEY=<USER_PROVIDED_SECRET_KEY>
```

### 5.2 上传 Agent 包

```bash
# 1. 设置环境变量（使用用户提供的凭证）
export MINIO_ENDPOINT=localhost:9000
export MINIO_ACCESS_KEY=<USER_PROVIDED_ACCESS_KEY>
export MINIO_SECRET_KEY=<USER_PROVIDED_SECRET_KEY>

# 2. 在容器内构建 Agent 并上传（一次性完成）
cd agent && make docker-build && make upload

# 3. 验证上传
# 访问 http://localhost:9001 → agent-artifacts bucket
```

### 5.3 远程 MINIO 服务器

```bash
MINIO_ENDPOINT=<server>:9000 \
MINIO_ACCESS_KEY=<your_key> \
MINIO_SECRET_KEY=<your_secret> \
make upload
```

### 5.4 Agent 卸载重装完整流程

#### 开发机：构建并上传
```bash
# 容器内构建并上传
cd agent && make docker-build && make upload
```

#### 目标机：卸载旧 Agent
```bash
sudo /opt/aegis-agent/uninstall.sh
```

#### 目标机：重新安装
```bash
curl -sSL http://<API_SERVER_IP>:8082/api/v1/agent/install.sh | sudo bash
```

#### 验证
```bash
# 检查服务状态
sudo systemctl status aegis-agent

# 查看实时日志
sudo journalctl -u aegis-agent -f

# 检查进程
ps aux | grep aegis-agent
```

---

## 六、API 测试 (curl)

### 健康检查
```bash
curl http://localhost:8082/health
```

### 示例：创建漏洞扫描任务
```bash
curl -X POST http://localhost:8082/api/v1/vulnerability/scan \
  -H "Content-Type: application/json" \
  -d '{"host_id":1,"scan_type":"full"}'
```

---

## 七、数据流测试检查点

```
Agent → Server (19090) → Kafka → DC (19092) → PostgreSQL
```

**测试步骤：**
1. Agent 是否连接 Server: `docker compose logs server | grep agent`
2. Kafka 消息: `docker compose exec kafka kafka-console-consumer --topic aegis.security.events --from-beginning`
3. DC 处理: `docker compose logs dc`
4. 数据库写入: `docker compose exec postgres psql -U aegis_user -d aegis_db -c "SELECT COUNT(*) FROM alerts;"`

---

## 八、常见问题排查

### 8.1 服务启动失败
```bash
# 1. 查看详细日志
docker compose logs <service>

# 2. 检查端口占用
netstat -tlnp | grep <port>

# 3. 检查依赖服务是否就绪
docker compose ps
```

### 8.2 Agent 无法连接 Server
```bash
# 1. 检查 Server gRPC 端口
docker compose exec server nc -z localhost 19090

# 2. 检查 Agent 日志
sudo journalctl -u aegis-agent | grep -i error

# 3. 验证网络连通性
telnet <server_ip> 19090
```

### 8.3 MINIO 上传失败
```bash
# 1. 验证 MINIO 服务
docker compose ps minio

# 2. 检查凭证
mc alias list

# 3. 手动测试上传
mc cp dist/aegis-agent.tar.gz local/agent-artifacts/
```

---

## 九、Docker 构建并启动

### 完整栈
```bash
docker compose up -d --build
```

### 单服务迭代开发
```bash
# 修改代码后快速重建
docker compose up -d --build <service>

# 查看日志
docker compose logs -f <service>
```
