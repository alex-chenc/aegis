# Aegis智能主机安全系统 V5.5 设计文档

**版本**: 5.5
**日期**: 2026-03-30
**状态**: 定稿

---

## 文档目录

| 文档 | 说明 |
|------|------|
| [prd_design_v5.5_complete.md](./prd_design_v5.5_complete.md) | 产品需求文档 |
| [architecture_design_v5.5.md](./architecture_design_v5.5.md) | 系统架构设计 |
| [backend_detailed_design_v5.5_complete.md](./backend_detailed_design_v5.5_complete.md) | 后端详细设计 |
| [agent_detailed_design_v5.5_complete.md](./agent_detailed_design_v5.5_complete.md) | Agent详细设计 |
| [frontend_detailed_design_v5.5_complete.md](./frontend_detailed_design_v5.5_complete.md) | 前端详细设计 |
| [database_structure_design_v5.5_complete.md](./database_structure_design_v5.5_complete.md) | 数据库结构设计 |
| [communication_protocol_design_v5.5.md](./communication_protocol_design_v5.5.md) | 通信协议设计 |

---

## 系统概述

Aegis智能主机安全系统是一个面向企业级运维和安全团队的全方位主机安全管理平台。

### 核心功能

| 模块 | 功能 |
|------|------|
| 智能基线检查与修复 | 自动化解析安全基线文档，生成可执行检查脚本 |
| 智能漏洞检查与修复 | 一键扫描软件漏洞，LLM生成精准修复方案 |
| 智能异常检测 | eBPF实时采集 + Sigma规则匹配 + LLM智能研判 |
| Agent智能预处理 | 本地轻量级智能分析，减少后端压力 |
| 微服务架构 | Backend拆分为独立服务，提高可维护性和扩展性 |

### V5.5核心特性

1. **微服务架构重构**: Backend拆分为API服务、Agent Hub服务、Pipeline服务
2. **Agent轻量级智能**: 本地预处理、统计异常检测、智能压缩上报
3. **通信协议优化**: 增量上报、分级处理、本地决策
4. **资源高效利用**: Agent端资源占用 < 1C1G，后端集中智能处理

---

## 技术栈

| 组件 | 技术 |
|------|------|
| 后端服务 | Go 1.21+, Gin, gRPC, GORM |
| 前端 | Vue 3, TypeScript, Vite, Element Plus |
| Agent | Go, eBPF, gRPC |
| 数据库 | PostgreSQL 15+ |
| 消息队列 | Kafka |
| 缓存 | Redis |
| 对象存储 | MinIO |
| 服务治理 | Nginx (API Gateway) |

---

## 架构概览

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Frontend (Vue 3)                               │
│                           localhost:8081 (Nginx)                           │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │ HTTP/WebSocket
┌─────────────────────────────────▼───────────────────────────────────────────┐
│                           API Gateway (Nginx)                              │
│                              localhost:8080                                │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
          ┌───────────────────────┼───────────────────────┐
          ↓                       ↓                       ↓
┌───────────────────┐   ┌───────────────────┐   ┌───────────────────┐
│   API Service     │   │   Agent Hub       │   │  Pipeline Service │
│   (HTTP API)      │   │   (gRPC Server)   │   │  (Kafka Consumer) │
│   - REST API      │   │   - Agent管理      │   │  - 事件处理        │
│   - WebSocket     │   │   - 命令下发       │   │  - LLM分析        │
│   - 认证授权       │   │   - 心跳监控       │   │  - 告警生成        │
└───────────────────┘   └───────────────────┘   └───────────────────┘
          │                       │                       │
          └───────────────────────┼───────────────────────┘
                                  ↓
                    ┌───────────────────────────┐
                    │     Shared Services       │
                    │  - PostgreSQL (主数据库)   │
                    │  - Redis (缓存/消息队列)   │
                    │  - MinIO (文件存储)       │
                    │  - LLM Client (AI推理)    │
                    │  - Kafka (事件流)         │
                    └───────────────────────────┘
                                  ↑
                                  │ gRPC (bi-directional streaming)
                                  ↓
┌───────────────────────────────────────────────────────────────────────────────┐
│                              Agent (Go)                                      │
│                    运行在目标主机上 (资源限制: 1C1G)                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │  eBPF       │→ │ Local       │→ │ Decision    │→ │  Smart              │
│  │  Collector  │  │ Intelligence│  │ Engine      │  │  Communicator       │
│  │             │  │ (轻量算法)   │  │ (本地决策)   │  │  (压缩/增量上报)     │  │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────────┘
```

---

## V5.5主要变更

### 架构变更

1. **Backend微服务拆分**
   - API Service: 负责HTTP REST API和WebSocket
   - Agent Hub: 负责Agent注册、心跳、命令下发
   - Pipeline Service: 负责Kafka消费、LLM分析、告警生成

2. **服务间通信**
   - API Service ↔ Agent Hub: gRPC
   - Agent Hub ↔ Pipeline Service: Kafka
   - 所有服务共享PostgreSQL、Redis、MinIO

### Agent变更

1. **新增本地智能模块**
   - LocalIntelligence: 本地轻量级智能处理
   - SlidingWindowStats: 滑动窗口统计异常检测
   - PriorityEngine: 规则优先级引擎
   - FeatureExtractor: 特征提取器

2. **通信协议优化**
   - 增量上报: 只上报特征而非原始数据
   - 分级处理: 紧急事件立即上报，普通事件批量上报
   - 本地决策: 高危事件本地阻断，无需等待后端

### 性能优化

| 指标 | V5.2 | V5.5 | 改善 |
|------|------|------|------|
| Agent网络带宽 | 100条/秒原始事件 | 10条/秒特征数据 | ↓90% |
| 紧急事件延迟 | 3-5秒 | <0.5秒 | ↑90% |
| Backend事件处理压力 | 100% | 30% | ↓70% |
| Agent内存占用 | 200MB | 150MB | ↓25% |

---

## 快速开始

### 构建后端服务

```bash
# 方式一：构建单体服务（兼容旧版）
cd backend && make build

# 方式二：分别构建微服务
cd backend && make build-api      # API Service
cd backend && make build-agent-hub # Agent Hub
cd backend && make build-pipeline # Pipeline Service
```

### 构建前端

```bash
cd frontend && npm run build
```

### 构建Agent

```bash
cd agent && make all
```

### 部署

```bash
# 使用docker-compose (单体模式，兼容旧版)
docker compose up -d

# 使用docker-compose (微服务模式)
docker compose -f docker-compose.microservices.yml up -d
```

---

## 项目结构

```
/ai-benchmark
├── backend/                          # 后端服务（单体）
│   ├── cmd/
│   │   ├── server/                   # 主服务入口
│   │   ├── api-service/              # API服务入口 (V5.5新增)
│   │   ├── agent-hub/                # Agent Hub服务入口 (V5.5新增)
│   │   └── pipeline/                 # Pipeline服务入口 (V5.5新增)
│   ├── internal/
│   │   ├── api/                      # HTTP层
│   │   ├── service/                  # 业务服务
│   │   ├── repository/               # 数据访问
│   │   ├── model/                    # 数据模型
│   │   ├── grpc_server/              # gRPC服务
│   │   ├── queue/                    # Kafka队列
│   │   └── pipeline/                 # 事件处理管道
│   ├── pkg/                          # 公共包
│   └── config/                       # 配置
├── frontend/                         # 前端服务
│   └── src/views/
├── agent/                            # Agent
│   ├── cmd/agent/
│   ├── internal/
│   │   ├── ebpf/                     # eBPF事件采集
│   │   ├── intelligence/             # 本地智能 (V5.5新增)
│   │   ├── decision/                 # 本地决策引擎 (V5.5新增)
│   │   └── communicator/             # 智能通信器 (V5.5新增)
│   └── dist/
├── aegis_system_design_v5.5/         # V5.5设计文档（本目录）
└── docker-compose.yml
```

---

## 版本历史

| 版本 | 日期 | 主要变更 |
|------|------|----------|
| 5.5 | 2026-03-30 | 微服务架构重构、Agent轻量级智能、通信协议优化、资源高效利用 |
| 5.2 | 2026-03-26 | MITRE ID统一、AI降噪增强、智能误报检测、AI规则生成 |
| 5.1 | 2026-03-25 | Agent日志配置、阻断策略初始化 |
| 5.0 | 2026-03-20 | 智能异常检测模块、eBPF事件采集、LLM研判 |

---

## 数据流对比

### V5.2 数据流

```
Agent: eBPF采集 → 实时上报所有事件 → Backend: Kafka → LLM分析 → 告警
问题: 网络流量大、后端压力大、延迟高
```

### V5.5 数据流

```
Agent: eBPF采集 → 本地智能预处理 → 分级上报
  ├── 紧急事件 → 实时上报 → Backend优先处理
  └── 普通事件 → 批量上报 → Backend聚合处理
Backend: LLM分析 → 告警/阻断策略 → 下发Agent
```

---

## API变更

### 新增API

| API | 说明 |
|-----|------|
| GET `/api/v1/agents` | 获取Agent列表 |
| GET `/api/v1/agents/:id/status` | 获取Agent状态 |
| POST `/api/v1/agents/:id/command` | 下发命令到Agent |
| GET `/api/v1/pipeline/status` | 获取Pipeline状态 |

### 服务发现

| 服务 | 端口 | 协议 |
|------|------|------|
| API Service | 8080 | HTTP |
| Agent Hub | 19090 | gRPC |
| Pipeline Service | 19091 | gRPC (内部) |

---

## 构建部署

### 后端微服务构建

```bash
# 构建所有服务
cd backend && make build-all

# 分别构建
cd backend && make build-api      # API Service
cd backend && make build-agent-hub # Agent Hub
cd backend && make build-pipeline # Pipeline Service
```

### Docker部署

```bash
# 单体模式
docker compose up -d

# 微服务模式
docker compose -f docker-compose.microservices.yml up -d
```

---

## 后续演进路线

### V6.0 目标

1. **部署拆分**: 微服务独立部署
2. **Agent增强**: 更多本地智能算法
3. **多Agent协同**: 威胁情报共享

### V7.0 目标

1. **Service Mesh**: 引入Istio服务治理
2. **分布式智能**: Agent间协同工作
3. **预测性安全**: 基于历史数据的威胁预测

---

**文档结束**