# 构建体系设计文档 - V1.6 完整版

**版本**: 2.1
**状态**: 定稿
**作者**: Manus AI

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 1.6 | 2026-03-05 | Manus AI | **完整重写**。确保文档独立、完整，包含所有组件的完整构建脚本和说明，移除所有外部引用。 |
| 1.5 | 2026-03-05 | Manus AI | 引入 Makefile + build.sh 体系，明确 Agent 构建产物上传 MinIO。 |

## 2. 概述

本文档为系统的所有组件（后端、前端、Agent）提供了一套标准化的构建、打包和分发流程。该体系旨在实现开发与部署的解耦，确保构建过程的一致性和可重复性。

### 核心设计原则

1.  **Makefile 驱动**: 每个子项目（`backend`, `frontend`, `agent`）都包含一个 `Makefile`，作为构建命令的统一入口，定义了 `build`, `clean`, `test` 等标准目标。
2.  **脚本封装构建逻辑**: 每个子项目提供一个 `build.sh` 脚本，该脚本封装了调用 `Makefile` 和执行 Docker 相关操作的逻辑，供 CI/CD 或开发者直接调用。
3.  **构建与运行分离**: `docker-compose.yml` **只负责启动预构建好的镜像**，不包含任何构建过程。开发者或 CI 系统需先执行各子项目的 `build.sh` 来生成 Docker 镜像，然后才能使用 `docker-compose up`。
4.  **Agent 的特殊处理**: Agent 在 Docker 容器中交叉编译，但其产物（二进制文件）不打包成镜像，而是上传到 MinIO 文件服务器进行分发。

## 3. 后端 (Go) 构建体系

**位置**: `/backend`

### 3.1 `Makefile`

```makefile
# backend/Makefile

.PHONY: all build clean test docker

# Go 相关变量
BINARY_NAME=backend
GO_CMD=go

# Docker 相关变量
IMAGE_NAME=baseline-system/backend
IMAGE_TAG?=latest

all: build

# 构建 Go 二进制文件
build:
	@echo "Building Go binary..."
	$(GO_CMD) build -o $(BINARY_NAME) ./cmd/main.go

# 清理构建产物
clean:
	@echo "Cleaning..."
	$(GO_CMD) clean
	rm -f $(BINARY_NAME)

# 运行测试
test:
	@echo "Running tests..."
	$(GO_CMD) test ./...

# 构建 Docker 镜像 (调用 build.sh)
docker:
	@./build.sh $(IMAGE_NAME) $(IMAGE_TAG)
```

### 3.2 `build.sh`

```bash
#!/bin/bash
# backend/build.sh

set -e

IMAGE_NAME=${1:-"baseline-system/backend"}
IMAGE_TAG=${2:-"latest"}

# 基础镜像
BASE_IMAGE="golang:1.20-alpine"

echo "Building Docker image ${IMAGE_NAME}:${IMAGE_TAG}..."

# 使用基础镜像进行构建
docker build -t ${IMAGE_NAME}:${IMAGE_TAG} \
    --build-arg BASE_IMAGE=${BASE_IMAGE} \
    -f Dockerfile .

echo "Build complete: ${IMAGE_NAME}:${IMAGE_TAG}"
```

### 3.3 `Dockerfile`

```dockerfile
# backend/Dockerfile

# --- Build Stage ---
ARG BASE_IMAGE=golang:1.20-alpine
FROM ${BASE_IMAGE} AS builder

WORKDIR /app

# 复制 go.mod 和 go.sum 并下载依赖
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码并构建
COPY . .
RUN make build

# --- Final Stage ---
FROM alpine:latest

WORKDIR /root/

# 从 builder 阶段复制二进制文件和配置文件
COPY --from=builder /app/backend .
COPY --from=builder /app/config/config.yaml .

# 暴露端口
EXPOSE 8080
EXPOSE 9090

# 运行命令
CMD ["./backend"]
```

## 4. 前端 (Vue) 构建体系

**位置**: `/frontend`

### 4.1 `Makefile`

```makefile
# frontend/Makefile

.PHONY: all install build clean test docker

# Node 相关变量
NPM_CMD=npm

# Docker 相关变量
IMAGE_NAME=baseline-system/frontend
IMAGE_TAG?=latest

all: build

# 安装依赖
install:
	@echo "Installing dependencies..."
	$(NPM_CMD) install

# 构建静态文件
build: install
	@echo "Building frontend assets..."
	$(NPM_CMD) run build

# 清理构建产物
clean:
	@echo "Cleaning..."
	rm -rf dist node_modules

# 运行测试
test:
	@echo "Running tests..."
	$(NPM_CMD) run test

# 构建 Docker 镜像 (调用 build.sh)
docker:
	@./build.sh $(IMAGE_NAME) $(IMAGE_TAG)
```

### 4.2 `build.sh`

```bash
#!/bin/bash
# frontend/build.sh

set -e

IMAGE_NAME=${1:-"baseline-system/frontend"}
IMAGE_TAG=${2:-"latest"}

# 基础镜像
BASE_IMAGE="node:18-alpine"

echo "Building Docker image ${IMAGE_NAME}:${IMAGE_TAG}..."

docker build -t ${IMAGE_NAME}:${IMAGE_TAG} \
    --build-arg BASE_IMAGE=${BASE_IMAGE} \
    -f Dockerfile .

echo "Build complete: ${IMAGE_NAME}:${IMAGE_TAG}"
```

### 4.3 `Dockerfile`

```dockerfile
# frontend/Dockerfile

# --- Build Stage ---
ARG BASE_IMAGE=node:18-alpine
FROM ${BASE_IMAGE} AS builder

WORKDIR /app

COPY package*.json ./
RUN npm install

COPY . .
RUN npm run build

# --- Final Stage ---
FROM nginx:1.23-alpine

# 复制构建好的静态文件到 Nginx 的 web 根目录
COPY --from=builder /app/dist /usr/share/nginx/html

# 复制 Nginx 配置文件
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
```

## 5. Agent (Go) 构建与分发体系

**位置**: `/agent`

### 5.1 `Makefile`

```makefile
# agent/Makefile

.PHONY: all build clean test upload

# Go 相关变量
BINARY_NAME=baseline-agent

# 交叉编译目标
TARGETS=linux/amd64 linux/arm64

all: build

# 在 Docker 容器中进行交叉编译
build:
	@echo "Cross-compiling Agent in Docker container..."
	docker run --rm -v "$(PWD)":/usr/src/myapp -w /usr/src/myapp golang:1.20-alpine sh -c ' \
	for target in $(TARGETS); do \
		GOOS=$$(echo $$target | cut -d'/' -f1) GOARCH=$$(echo $$target | cut -d'/' -f2) go build -v -o ./dist/$(BINARY_NAME)-$$GOOS-$$GOARCH ./cmd/agent; \
	done'

# 清理构建产物
clean:
	@echo "Cleaning..."
	rm -rf dist

# 运行测试
test:
	@echo "Running tests..."
	go test ./...

# 上传构建产物到 MinIO (调用 build.sh)
upload:
	@./build.sh
```

### 5.2 `build.sh`

```bash
#!/bin/bash
# agent/build.sh

set -e

# MinIO 配置 (从环境变量读取)
MINIO_ENDPOINT=${MINIO_ENDPOINT:-"http://localhost:9000"}
MINIO_ACCESS_KEY=${MINIO_ACCESS_KEY:-"minio_admin"}
MINIO_SECRET_KEY=${MINIO_SECRET_KEY:-"a_third_strong_secret_password"}
MINIO_BUCKET="agent-artifacts"

# 1. 执行编译
make build

# 2. 配置 MinIO 客户端 (mc)
# 假设 mc 已经安装在系统中
echo "Configuring MinIO client..."
mc alias set myminio ${MINIO_ENDPOINT} ${MINIO_ACCESS_KEY} ${MINIO_SECRET_KEY}

# 3. 确保 Bucket 存在
mc mb myminio/${MINIO_BUCKET} --ignore-existing

# 4. 上传所有构建产物
echo "Uploading artifacts to MinIO bucket: ${MINIO_BUCKET}..."
mc cp --recursive dist/ myminio/${MINIO_BUCKET}/

echo "Upload complete."
```

## 6. Agent 一键安装脚本 (`install.sh`)

此脚本由后端 `/api/v1/agent/install.sh` 接口动态渲染并提供给用户。它负责在目标主机上完成 Agent 的下载、配置和启动。

```bash
#!/bin/bash
# install.sh (Template)

set -e

# --- 配置 (由后端注入) ---
SERVER_IP="{{.ServerIP}}"
AGENT_TOKEN="{{.Token}}"
# ------------------------

# 1. 检查 root 权限
if [ "$(id -u)" -ne 0 ]; then
    echo "Error: This script must be run as root." >&2
    exit 1
fi

# 2. 检测系统架构
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *) echo "Error: Unsupported architecture: $ARCH"; exit 1 ;;
esac

# 3. 从后端获取预签名下载链接
echo "Fetching download URL for ${OS}/${ARCH}..."
DOWNLOAD_URL=$(curl -s "http://${SERVER_IP}:8080/api/v1/agent/download?os=${OS}&arch=${ARCH}" | sed -n 's/.*"download_url":"\([^"]*\)".*/\1/p')

if [ -z "${DOWNLOAD_URL}" ]; then
    echo "Error: Could not get download URL from server."
    exit 1
fi

# 4. 下载 Agent 二进制文件
AGENT_PATH="/usr/local/bin/baseline-agent"
echo "Downloading agent to ${AGENT_PATH}..."
curl -L --progress-bar "${DOWNLOAD_URL}" -o "${AGENT_PATH}"
chmod +x "${AGENT_PATH}"

# 5. 创建配置文件
CONFIG_DIR="/etc/baseline-agent"
CONFIG_PATH="${CONFIG_DIR}/config.toml"
echo "Creating config file at ${CONFIG_PATH}..."
mkdir -p "${CONFIG_DIR}"
cat > "${CONFIG_PATH}" << EOF
# 后端 gRPC 服务器地址
ServerAddr = "${SERVER_IP}:9090"

# 用于与后端认证的 Token
AuthToken = "${AGENT_TOKEN}"

# Agent 的唯一标识符，留空则首次启动时自动生成
HostID = ""
EOF

# 6. 创建 Systemd 服务
SERVICE_PATH="/etc/systemd/system/baseline-agent.service"
echo "Creating systemd service at ${SERVICE_PATH}..."
cat > "${SERVICE_PATH}" << EOF
[Unit]
Description=Baseline Check Agent
After=network.target

[Service]
Type=simple
ExecStart=${AGENT_PATH}
Restart=on-failure
RestartSec=5
User=root
Group=root

[Install]
WantedBy=multi-user.target
EOF

# 7. 启动服务
echo "Starting agent service..."
systemctl daemon-reload
systemctl enable baseline-agent.service
systemctl restart baseline-agent.service

echo "Agent installed and started successfully."
```
