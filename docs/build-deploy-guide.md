# 构建部署指南

## 环境要求

- Go 1.21+
- Node.js 18+
- Docker 20.10+
- Docker Compose 2.0+
- Make

## 快速构建

### 后端 (Backend)

```bash
cd backend
make build
```

构建产物：`backend` 可执行文件

### 前端 (Frontend)

```bash
cd frontend
make install
make build
```

构建产物：`frontend/dist/` 目录

### Agent

```bash
cd agent
make build
```

构建产物：`agent/dist/` 目录下的跨平台二进制文件

## Docker 部署

### 方式一：完整构建部署

```bash
# 1. 构建后端
cd backend && make build && cd ..

# 2. 构建前端
cd frontend && make install && make build && cd ..

# 3. 构建并启动容器
docker compose up -d --build
```

### 方式二：直接使用预构建镜像

```bash
docker compose up -d
```

## 服务端口

| 服务 | 端口 |
|------|------|
| 前端 | 8081 |
| 后端 HTTP API | 8080 |
| 后端 gRPC | 9090 |
| PostgreSQL | 5432 |
| Redis | 6379 |
| MinIO API | 9000 |
| MinIO Console | 9001 |
| Kafka | 9092 |
| Zookeeper | 2181 |

## 验证部署

```bash
# 健康检查
curl http://localhost:8080/health

# 预期响应
{"status": "ok"}
```

## 常用命令

```bash
# 查看日志
docker compose logs -f backend
docker compose logs -f frontend

# 重启服务
docker compose restart

# 停止服务
docker compose down

# 清理数据
docker compose down -v
```

## 配置说明

配置文件位于 `backend/config/config.yaml`，主要配置项：

- 数据库连接
- Redis 连接
- MinIO 配置
- LLM API 配置
- Kafka 配置