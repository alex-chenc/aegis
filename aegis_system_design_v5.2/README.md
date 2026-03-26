# Aegis智能主机安全系统 V5.2 设计文档

**版本**: 5.2
**日期**: 2026-03-26
**状态**: 定稿

---

## 文档目录

| 文档 | 说明 |
|------|------|
| [prd_design_v5.2_complete.md](./prd_design_v5.2_complete.md) | 产品需求文档 |
| [backend_detailed_design_v5.2_complete.md](./backend_detailed_design_v5.2_complete.md) | 后端详细设计 |
| [frontend_detailed_design_v5.2_complete.md](./frontend_detailed_design_v5.2_complete.md) | 前端详细设计 |
| [agent_detailed_design_v5.2_complete.md](./agent_detailed_design_v5.2_complete.md) | Agent详细设计 |
| [database_structure_design_v5.2_complete.md](./database_structure_design_v5.2_complete.md) | 数据库结构设计 |

---

## 系统概述

Aegis智能主机安全系统是一个面向企业级运维和安全团队的全方位主机安全管理平台。

### 核心功能

| 模块 | 功能 |
|------|------|
| 智能基线检查与修复 | 自动化解析安全基线文档，生成可执行检查脚本 |
| 智能漏洞检查与修复 | 一键扫描软件漏洞，LLM生成精准修复方案 |
| 智能异常检测 | eBPF实时采集 + Sigma规则匹配 + LLM智能研判 |

### V5.2核心特性

1. **MITRE ID统一**: 所有MITRE ID统一为大写T格式（如T1059.004）
2. **AI降噪增强**: 按时间范围直接查询告警，支持精准分析
3. **智能误报检测**: 定时检测高频规则，LLM自动加严规则
4. **AI规则生成**: 描述检测事件，LLM自动生成Sigma规则
5. **规则管理增强**: 多选删除、MITRE点击跳转、搜索功能
6. **阻断策略关联**: 规则标题与阻断策略名称一致，MITRE点击跳转
7. **数据一致性**: 规则、阻断策略、告警三表MITRE ID格式统一

---

## 技术栈

| 组件 | 技术 |
|------|------|
| 后端 | Go 1.21+, Gin, gRPC, GORM |
| 前端 | Vue 3, TypeScript, Vite, Element Plus |
| Agent | Go, eBPF, gRPC |
| 数据库 | PostgreSQL 15+ |
| 消息队列 | Kafka |
| 缓存 | Redis |
| 对象存储 | MinIO |

---

## 快速开始

### 构建后端

```bash
cd backend && make build
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
docker compose up -d
```

---

## 项目结构

```
/ai-benchmark
├── backend/                    # 后端服务
│   ├── internal/
│   │   ├── service/
│   │   │   ├── false_positive_service.go  # 智能误报检测服务
│   │   │   └── ...
│   │   └── repository/
│   │       ├── alert_repo.go
│   │       ├── block_policy_repo.go
│   │       └── sigma_rule_repo.go
│   └── api/handler/
│       └── detection_handler.go
├── frontend/                   # 前端服务
│   └── src/views/detection/
│       ├── Rules.vue          # 规则管理
│       ├── Policies.vue       # 阻断策略
│       └── Alerts.vue         # 告警中心
├── agent/                      # Agent
├── aegis_system_design_v5.2/   # V5.2设计文档（本目录）
└── docker-compose.yml
```

---

## 版本历史

| 版本 | 日期 | 主要变更 |
|------|------|----------|
| 5.2 | 2026-03-26 | MITRE ID统一、AI降噪增强、智能误报检测、AI规则生成、规则管理增强、阻断策略关联 |
| 5.1 | 2026-03-25 | Agent日志配置、阻断策略初始化、分页功能 |
| 5.0 | 2026-03-20 | 智能异常检测模块、eBPF事件采集、LLM研判 |

---

## 数据库变更

### 新增表

**rule_adjustment_histories**: 规则调整历史记录

### 新增约束

```sql
-- sigma_rules表MITRE ID唯一约束
ALTER TABLE sigma_rules ADD CONSTRAINT sigma_rules_mitre_id_unique UNIQUE (mitre_id);
```

### MITRE ID格式规范

- 统一为大写T格式（如T1059.004）
- 规则、阻断策略、告警三表格式一致
- 规则表MITRE ID具有唯一约束

---

## API变更

### 新增API

| API | 说明 |
|-----|------|
| POST `/api/v1/detection/rules/generate` | AI生成Sigma规则 |
| POST `/api/v1/detection/rules/check-delete` | 删除前检查告警关联 |
| DELETE `/api/v1/detection/rules` | 批量删除规则 |
| DELETE `/api/v1/detection/alerts` | 批量删除告警 |
| POST `/api/v1/detection/block-policies/sync` | 同步阻断策略 |
| POST `/api/v1/detection/block-policies/normalize` | MITRE ID规范化 |

### 修改API

| API | 变更 |
|-----|------|
| GET `/api/v1/detection/rules` | 新增query参数支持模糊搜索 |
| GET `/api/v1/detection/block-policies` | 返回rule_title字段 |
| POST `/api/v1/detection/llm/aggregate` | 修复时间范围查询逻辑 |

---

## 前端变更

### 规则管理页面 (Rules.vue)

- 删除规则ID列
- MITRE ID可点击跳转到阻断策略页面
- 添加AI规则生成对话框
- 添加多选删除功能
- 添加搜索功能

### 阻断策略页面 (Policies.vue)

- "策略名称"改为"规则标题"（与规则表一致）
- MITRE ID可点击跳转到规则管理页面
- 添加搜索功能

### 告警列表页面 (Alerts.vue)

- MITRE ID可点击跳转到规则管理页面
- 添加批量删除功能

---

## 构建部署

### 后端

```bash
cd backend && make build
docker build -t aegis-system/backend:latest -f backend/Dockerfile .
docker compose up -d backend
```

### 前端

```bash
cd frontend && npm run build
docker build -t aegis-system/frontend:latest .
docker compose up -d frontend
```

---

**文档结束**