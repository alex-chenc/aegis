# AI基线检查系统

> **版本**: 2.5.0
> **状态**: 开发中

## 项目概述

一个面向运维和安全工程师的内部管理平台，通过结合大语言模型（LLM）的智能解析能力和 Agent 的自动化执行能力，实现服务器基线配置的智能检查与自动修复。

**核心功能**：
- 上传基线文档（PDF、Word、YAML）→ LLM 智能解析为检查/修复规则
- 在服务器上部署 Agent 执行自动化基线检查
- **智能检测脚本生成**：点击检测按钮，LLM 自动生成检测脚本
- **智能修复脚本生成**：点击修复按钮，LLM 自动生成修复脚本
- **任务进度追踪**：实时查看任务执行状态和进度
- **自愈功能**：LLM 自动分析错误并修复失败的脚本

## 技术栈

| 组件 | 技术 |
|------|------|
| **后端** | Go 1.20+, Gin (REST API), gRPC (Agent 通讯) |
| **前端** | Vue 3 (Composition API), TypeScript, Vite, Pinia, Element Plus |
| **Agent** | Go (交叉编译：linux/amd64, linux/arm64) |
| **数据库** | PostgreSQL 14 |
| **缓存** | Redis 7 |
| **存储** | MinIO (文件/对象存储) |

## 快速开始

### 1. 准备环境

```bash
docker --version
docker-compose --version
```

### 2. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env，修改密码和密钥
```

### 3. 构建镜像

```bash
cd backend && ./build.sh && cd ..
cd frontend && ./build.sh && cd ..
```

### 4. 启动服务

```bash
docker-compose up -d
```

### 5. 验证部署

```bash
docker-compose ps
curl http://localhost:8080/health
# 浏览器打开 http://localhost
```

## 项目结构

```
.
├── backend/                 # Go 后端服务
│   ├── cmd/server/         # 入口程序
│   ├── config/             # 配置管理
│   ├── internal/           # 内部包
│   ├── pkg/                # 公共包
│   └── scripts/            # 数据库脚本
├── frontend/               # Vue 3 前端
│   └── src/
├── agent/                  # Go Agent
│   ├── cmd/agent/
│   └── dist/
├── docker-compose.yml
├── .env.example
└── docs/
    └── plans/
```

## 设计文档

所有设计文档位于 `baseline_system_design_v2.1/` 目录。

## 开发指南

```bash
# 后端
cd backend && make build && make test

# 前端
cd frontend && npm run dev

# Agent
cd agent && make build
```

## License

内部项目，未授权。