---
name: aegis-release-packaging
description: Aegis 离线部署包构建技能 - Docker 镜像导出、数据库初始化、一键启动脚本、MinIO Agent 预置
version: 1.0.0
source: manual-creation
---

# Aegis 离线部署包构建技能

本技能用于构建 Aegis 系统的离线部署包（zip 格式），可在任何支持 Docker 的 Linux 机器上一键部署。

## 一、核心概念

### 1.1 离线部署包结构

```
release/
└── {version}/
    ├── images/                    # Docker 镜像 tar.gz 文件
    │   ├── api-server.tar.gz
    │   ├── server.tar.gz
    │   ├── dc.tar.gz
    │   ├── frontend.tar.gz
    │   ├── postgres.tar.gz
    │   ├── redis.tar.gz
    │   ├── kafka.tar.gz
    │   ├── zookeeper.tar.gz
    │   ├── minio-with-agent.tar.gz   # 自定义镜像（预置 Agent 包）
    │   └── ...
    ├── build-context/            # MinIO 镜像构建上下文
    │   ├── aegis-agent-linux-amd64
    │   ├── aegis-agent-linux-amd64.tar.gz
    │   ├── bpf/
    │   └── minio-entrypoint.sh
    ├── backend/
    │   └── scripts/
    │       └── init.sql
    ├── docker-compose.yml
    ├── .env.example
    ├── start.sh
    └── README.md
```

### 1.2 服务架构

| 服务 | 端口 | 职责 |
|------|------|------|
| api-server | 8082, 19093 | HTTP API，gRPC 客户端 |
| server | 19090, 19094 | Agent Hub，命令下发 |
| dc | 19092 | Kafka 消费，LLM 分析 |
| frontend | 8081 | Vue 3 UI |
| postgres | 5432 | 主数据库 |
| redis | 6379 | 缓存，消息队列 |
| minio | 9000, 9001 | 对象存储（Agent 包存储） |
| kafka | 29092 | 事件流 |

### 1.3 数据流

```
Agent → Server → Kafka (aegis.security.events) → DC → PostgreSQL (runtime_events, alerts)
                                                    ↓
                                              WebSocket → Frontend
```

## 二、构建流程

### 阶段一：准备构建目录

```bash
# 创建版本目录
VERSION=v5.5
RELEASE_DIR=/code/ai-benchmark/release/${VERSION}
mkdir -p ${RELEASE_DIR}/{images,build-context/bpf,backend/scripts}
```

### 阶段二：构建 Agent（如修改过 Agent）

```bash
cd agent
make bpf    # 编译 eBPF 程序
make build  # 构建 Agent 二进制
```

### 阶段三：准备 Agent 打包

```bash
# 复制 Agent 二进制和 eBPF 程序
cp agent/bin/agent-linux-amd64 ${RELEASE_DIR}/build-context/aegis-agent-linux-amd64
cp agent/internal/ebpf/*.bpf.o ${RELEASE_DIR}/build-context/bpf/

# 打包 Agent
tar -czvf ${RELEASE_DIR}/build-context/aegis-agent-linux-amd64.tar.gz \
    -C ${RELEASE_DIR}/build-context \
    aegis-agent-linux-amd64 bpf/
```

### 阶段四：构建所有 Docker 镜像

```bash
# 业务服务
docker build -f api-server/Dockerfile -t aegis-system/api-server:latest api-server/
docker build -f server/Dockerfile -t aegis-system/server:latest server/
docker build -f dc/Dockerfile -t aegis-system/dc:latest dc/
docker build -f frontend/Dockerfile -t aegis-system/frontend:latest frontend/

# 基础镜像
docker pull postgres:14-alpine
docker pull redis:7-alpine
docker pull confluentinc/cp-kafka:7.5.0
docker pull confluentinc/cp-zookeeper:7.5.0
docker pull minio/minio:latest
docker pull minio/mc:latest
```

### 阶段五：制作自定义 MinIO 镜像

**关键**：Agent 安装时从 MinIO 下载，需要预置 Agent 包。

```bash
# 创建 MinIO 入口脚本
cat > ${RELEASE_DIR}/build-context/minio-entrypoint.sh << 'EOF'
#!/bin/bash
set -e

mc alias set myminio http://localhost:9000 ${MINIO_ROOT_USER} ${MINIO_ROOT_PASSWORD} 2>/dev/null || true
mc mb myminio/aegis-templates --ignore-existing 2>/dev/null || true
mc mb myminio/agent-artifacts --ignore-existing 2>/dev/null || true
mc mb myminio/generated-scripts --ignore-existing 2>/dev/null || true
mc anonymous set download myminio/agent-artifacts 2>/dev/null || true

# 上传 Agent 包（文件名必须与 api-server handler 预期一致）
if [ -f /agent-artifacts/aegis-agent-linux-amd64.tar.gz ]; then
    mc cp /agent-artifacts/aegis-agent-linux-amd64.tar.gz myminio/agent-artifacts/aegis-agent.tar.gz
fi

exec minio server /data --console-address ":9001" "$@"
EOF

# 创建 Dockerfile.minio
cat > ${RELEASE_DIR}/Dockerfile.minio << 'EOF'
FROM minio/minio:latest
COPY build-context/aegis-agent-linux-amd64.tar.gz /agent-artifacts/
COPY build-context/minio-entrypoint.sh /usr/bin/minio-entrypoint.sh
RUN chmod +x /usr/bin/minio-entrypoint.sh
ENTRYPOINT ["/usr/bin/minio-entrypoint.sh"]
CMD ["server", "/data", "--console-address", ":9001"]
EOF

# 构建
docker build -f ${RELEASE_DIR}/Dockerfile.minio -t aegis-system/minio-with-agent:latest ${RELEASE_DIR}
```

### 阶段六：导出镜像

```bash
cd ${RELEASE_DIR}/images

# 业务服务镜像
docker save aegis-system/api-server:latest | gzip > api-server.tar.gz
docker save aegis-system/server:latest | gzip > server.tar.gz
docker save aegis-system/dc:latest | gzip > dc.tar.gz
docker save aegis-system/frontend:latest | gzip > frontend.tar.gz
docker save aegis-system/minio-with-agent:latest | gzip > minio-with-agent.tar.gz

# 基础镜像
docker save postgres:14-alpine | gzip > postgres.tar.gz
docker save redis:7-alpine | gzip > redis.tar.gz
docker save confluentinc/cp-kafka:7.5.0 | gzip > kafka.tar.gz
docker save confluentinc/cp-zookeeper:7.5.0 | gzip > zookeeper.tar.gz
docker save minio/minio:latest | gzip > minio.tar.gz
docker save minio/mc:latest | gzip > minio-mc.tar.gz
```

### 阶段七：创建数据库初始化脚本

```bash
# init.sql 必须包含所有表，且字段与 GORM model 一致
cat > ${RELEASE_DIR}/backend/scripts/init.sql << 'EOF'
-- PostgreSQL 初始化脚本
-- 注意：字段必须与服务的 GORM model 定义一致

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- runtime_events（关键表，易遗漏字段）
CREATE TABLE IF NOT EXISTS runtime_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id VARCHAR(64) NOT NULL UNIQUE,
    host_id UUID NOT NULL,
    event_type VARCHAR(32) NOT NULL,
    event_data JSONB NOT NULL DEFAULT '{}',
    matched_rule_id VARCHAR(128),
    rule_title VARCHAR(255),
    mitre_id VARCHAR(20),
    severity VARCHAR(16),
    pid INTEGER DEFAULT 0,
    command_line TEXT,
    process_name VARCHAR(255),  -- 必须有！与 DC GORM model 一致
    timestamp BIGINT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    aggregated BOOLEAN DEFAULT FALSE
);

-- 其他表：hosts, alerts, sigma_rules, block_policies, vulnerabilities 等
-- 参考 database_structure_design_{version}.md
EOF
```

### 阶段八：创建 docker-compose.yml

关键配置点：

```yaml
# 1. 服务依赖（必须按顺序）
api-server:
  depends_on:
    postgres:
      condition: service_healthy
    redis:
      condition: service_healthy
    minio:
      condition: service_healthy
    server:        # 必须添加
      condition: service_healthy
    kafka:         # 必须添加
      condition: service_healthy

# 2. MinIO init 容器（上传 Agent 包）
minio-init:
  image: aegis-system/mc:latest
  volumes:
    - ./build-context:/agent-artifacts:ro
  entrypoint: >
    /bin/sh -c "
    mc alias set myminio http://minio:9000 $${MINIO_ACCESS_KEY} $${MINIO_SECRET_KEY};
    mc mb myminio/agent-artifacts --ignore-existing;
    mc anonymous set download myminio/agent-artifacts;
    # 文件名 aegis-agent.tar.gz 与 api-server handler 预期一致
    mc cp /agent-artifacts/aegis-agent-linux-amd64.tar.gz myminio/agent-artifacts/aegis-agent.tar.gz;
    exit 0;
    "

# 3. 环境变量引用统一
api-server:
  environment:
    SERVER_EXTERNAL_IP: ${EXTERNAL_IP:-}  # 使用 EXTERNAL_IP
```

### 阶段九：创建 start.sh

核心功能：
1. 检查 Docker 和 Docker Compose
2. 加载镜像
3. 检测主机外部 IP（排除 Docker/Kubernetes 内部网段）
4. 验证 .env 配置
5. 启动服务
6. 健康检查

```bash
#!/bin/bash
set -e
VERSION="v5.5"
ARCH="linux/amd64"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IMAGES_DIR="${SCRIPT_DIR}/images"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()    { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# 1. 检查 Docker
command -v docker &>/dev/null || { error "Docker 未安装"; exit 1; }

# 2. 加载镜像
for tarball in "$IMAGES_DIR"/*.tar.gz; do
    [ -e "$tarball" ] || continue
    IMAGE_NAME=$(basename "$tarball" .tar.gz)
    info "加载镜像: $IMAGE_NAME ..."
    gunzip -c "$tarball" | docker load &>/dev/null
done

# 3. 检测主机外部 IP（排除 Docker/Kubernetes 内部网段）
DETECTED_IP=$(hostname -I 2>/dev/null | awk '{
    for(i=1;i<=NF;i++)
        if($i !~ /^172\.(17|1[89]|2[0-9]|3[01])\./ &&
           $i !~ /^192\.168\./ &&
           $i !~ /^10\./ &&
           $i !~ /^127\./)
            print $i; exit
}')

# 4. 更新 .env 中的 EXTERNAL_IP
if [ -n "$DETECTED_IP" ]; then
    if grep -q "^EXTERNAL_IP=" .env; then
        sed -i "s/^EXTERNAL_IP=.*/EXTERNAL_IP=$DETECTED_IP/" .env
    else
        echo "EXTERNAL_IP=$DETECTED_IP" >> .env
    fi
fi

# 5. 启动服务
cd "${SCRIPT_DIR}"
docker compose up -d

# 6. 健康检查
info "等待服务就绪..."
sleep 10
docker compose ps
```

### 阶段十：打包

```bash
cd /code/ai-benchmark/release
rm -f aegis-${VERSION}-linux-amd64-release.zip
zip -r aegis-${VERSION}-linux-amd64-release.zip ${VERSION}/
```

## 三、关键配置要点

### 3.1 环境变量映射

| .env 定义 | docker-compose.yml 引用 | 说明 |
|-----------|----------------------|------|
| `EXTERNAL_IP=192.168.x.x` | `SERVER_EXTERNAL_IP: ${EXTERNAL_IP:-}` | Agent 安装脚本显示的 IP |

### 3.2 服务启动顺序

```
1. postgres → 2. redis → 3. minio → 4. kafka/zookeeper
                                            ↓
5. server → 6. api-server → 7. dc → 8. frontend
```

### 3.3 Agent 下载流程

```
前端调用 GET /api/v1/agent/install.sh
  ↓
api-server 返回安装脚本（包含 SERVER_ADDR="${EXTERNAL_IP}:8082"）
  ↓
目标主机执行安装脚本
  ↓
curl GET /api/v1/agent/download?os=linux&arch=amd64
  ↓
api-server 从 MinIO 下载（MinIO 预置了 aegis-agent.tar.gz）
```

## 四、部署验证

```bash
# 1. 解压并启动
unzip aegis-*-linux-amd64-release.zip
cd v5.5
bash start.sh

# 2. 验证服务状态
docker compose ps

# 3. 验证健康检查
curl http://localhost:8082/health

# 4. 验证 Agent 安装脚本 IP
curl -s http://localhost:8082/api/v1/agent/install.sh | grep SERVER_ADDR
# 预期：192.168.x.x（不是 172.x.x.x）

# 5. 验证 Agent 下载
curl -s "http://localhost:8082/api/v1/agent/download?os=linux&arch=amd64" -o /tmp/test.tar.gz
ls -lh /tmp/test.tar.gz  # 预期：非空文件

# 6. 验证数据库
docker exec aegis-postgres psql -U aegis_user -d aegis_db -c "SELECT COUNT(*) FROM sigma_rules;"
```

## 五、常见问题排查

### 问题 1：Agent 安装脚本 IP 是 Docker 内部地址

**检查**：
```bash
grep SERVER_ADDR <(curl -s http://localhost:8082/api/v1/agent/install.sh)
```

**原因**：docker-compose.yml 中 `SERVER_EXTERNAL_IP` 引用了错误的环境变量

**解决**：确认使用 `${EXTERNAL_IP:-}`，`start.sh` 将检测到的 IP 写入 `.env`

### 问题 2：Agent 下载失败 (unexpected end of file)

**检查**：
```bash
curl -s "http://localhost:8082/api/v1/agent/download?os=linux&arch=amd64" -o /tmp/test.tar.gz
file /tmp/test.tar.gz  # 是否为空
```

**原因**：
1. MinIO 中文件名与 api-server handler 预期不一致
2. MinIO init 容器未正确上传文件

**解决**：
1. 确认 MinIO 中文件名为 `aegis-agent.tar.gz`
2. 检查 minio-init 容器日志：`docker compose logs minio-init`

### 问题 3：DC 服务报 `column does not exist`

**检查**：
```bash
docker logs aegis-dc 2>&1 | grep "column.*does not exist"
```

**原因**：init.sql 表定义与 DC GORM model 不一致

**解决**：对比 `dc/internal/model/` 中的 GORM model 补全 init.sql 字段

### 问题 4：服务启动后 API 返回 500

**检查**：
```bash
docker logs aegis-api-server 2>&1 | tail -50
docker exec aegis-postgres psql -U aegis_user -d aegis_db -c "\d"
```

**原因**：
1. 数据库表未创建（init.sql 未执行）
2. GORM AutoMigrate 未调用

**解决**：
1. 确认 `docker-compose.yml` 中 postgres 有 volume mount init.sql
2. 确认 init.sql 被正确执行

## 六、相关文件位置

| 文件 | 位置 |
|------|------|
| API Server | `api-server/` |
| Server | `server/` |
| DC | `dc/` |
| Frontend | `frontend/` |
| Agent | `agent/` |
| 数据库设计 | `aegis_system_design_*/database_structure_design_*.md` |
| 架构设计 | `aegis_system_design_*/architecture_design_*.md` |
