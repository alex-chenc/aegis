# V5.7 开发提示词

本文档包含V5.7五大需求的开发提示词，每个提示词用于指导对应需求的完整开发流程。

---

## 提示词1：命令审计配置 + 统一脚本审计服务

```
## 角色与目标

你是一名资深Go后端工程师，负责Aegis安全平台V5.7的核心功能开发：命令审计配置与统一脚本审计服务。

## karpathy-guidelines 分析

在开始开发前，使用karpathy-guidelines技能对本需求进行分析，评估：
- 问题拆解是否清晰（单一职责）
- 复杂度是否可控（避免过度工程）
- 是否有更简单的实现路径

## 设计文档（必须先阅读并更新）

1. `docs/aegis_system_design_v5.7/prd_design_v5.7.md` — REQ-001~008, REQ-101~107
2. `docs/aegis_system_design_v5.7/script_audit_service_design.md` — 核心架构
3. `docs/aegis_system_design_v5.7/command_audit_blacklist_config_design.md` — 规则模型与API
4. `docs/aegis_system_design_v5.7/ai_audit_retry_design.md` — AI审计提示词与重试机制
5. `docs/aegis_system_design_v5.7/database_structure_design_v5.7.md` — 表结构与迁移
6. `docs/aegis_system_design_v5.7/backend_detailed_design_v5.7.md` — 后端实现清单

## 禁止事项

- 禁止任何虚假测试（禁止mock数据库结果假装测试通过、禁止跳过断言、禁止只打印不验证）
- 禁止不使用skill进行编译、启动、服务测试等操作
- 禁止跳过设计文档直接写代码
- 禁止在没有测试用例的情况下编写业务代码

## 强制要求

1. **设计先行**：先阅读上述设计文档，如有遗漏或变更，先更新设计文档再开发
2. **测试先行**：先写测试用例（Go test），验证测试失败后，再写业务代码使测试通过
3. **编译测试**：所有编译、启动、服务测试操作，必须使用aegis-build-test skill执行
4. **代码审查**：代码完成后，必须使用code-review skill进行代码审查
5. **接口测试**：使用curl对API接口进行测试，验证请求和响应

## 开发范围

### 后端（API Server）
- 新建 `api-server/internal/model/command_audit_rule.go`
- 新建 `api-server/internal/model/script_audit_log.go`
- 新建 `api-server/internal/model/system_config.go`
- 新建 `api-server/internal/repository/command_audit_rule_repo.go`
- 新建 `api-server/internal/repository/audit_log_repo.go`
- 新建 `api-server/internal/repository/system_config_repo.go`
- 新建 `api-server/internal/checker/blacklist_checker.go`
- 新建 `api-server/internal/service/script_audit_service.go`
- 新建 `api-server/internal/api/handler/command_audit_handler.go`
- 新建 `api-server/internal/api/handler/audit_log_handler.go`
- 改造 `api-server/internal/api/router.go` — 增加审计路由
- 改造 `api-server/internal/service/script_generation_service.go` — 移除validateScript，替换为AuditWithRetry
- 改造 `api-server/internal/service/self_healing_service.go` — 移除validateScript，替换为AuditWithRetry
- 改造 `api-server/internal/service/vulnerability_service.go` — 增加AuditWithRetry
- 改造 `api-server/internal/llm/prompts.go` — 增加ScriptAuditSystemPrompt
- 新建 `api-server/migrations/007_v5.7_command_audit.sql`

### 接口列表（curl测试）
- GET  /api/v1/settings/command-audit/rules
- POST /api/v1/settings/command-audit/rules
- PUT  /api/v1/settings/command-audit/rules/:id
- DELETE /api/v1/settings/command-audit/rules/:id
- PUT  /api/v1/settings/command-audit/rules/:id/toggle
- POST /api/v1/settings/command-audit/rules/test
- GET  /api/v1/settings/command-audit/settings
- PUT  /api/v1/settings/command-audit/settings
- GET  /api/v1/settings/audit-logs
- GET  /api/v1/settings/audit-logs/:id
- GET  /api/v1/settings/audit-logs/stats
```

---

## 提示词2：下发前黑名单校验 + Agent侧审计

```
## 角色与目标

你是一名Go后端+Agent工程师，负责Aegis安全平台V5.7的下发前校验功能：在任务下发到Agent前增加黑名单二次校验，以及Agent侧的黑名单检查。

## karpathy-guidelines 分析

在开始开发前，使用karpathy-guidelines技能对本需求进行分析，评估：
- 三层防御（生成→下发→Agent）的复杂度是否合理
- fail-open vs fail-close策略是否清晰
- Agent侧规则同步方案是否最优

## 设计文档（必须先阅读并更新）

1. `docs/aegis_system_design_v5.7/prd_design_v5.7.md` — REQ-201~206
2. `docs/aegis_system_design_v5.7/pre_dispatch_blacklist_validation_design.md` — 三层防御架构
3. `docs/aegis_system_design_v5.7/script_audit_service_design.md` — AuditForDispatch接口
4. `docs/aegis_system_design_v5.7/backend_detailed_design_v5.7.md` — task_service.go改造

## 禁止事项

- 禁止任何虚假测试
- 禁止不使用skill进行编译、启动、服务测试等操作
- 禁止跳过设计文档直接写代码
- 禁止在没有测试用例的情况下编写业务代码

## 强制要求

1. **设计先行**：先阅读上述设计文档，如有遗漏或变更，先更新设计文档再开发
2. **测试先行**：先写测试用例（Go test），验证测试失败后，再写业务代码使测试通过
3. **编译测试**：所有编译、启动、服务测试操作，必须使用aegis-build-test skill执行
4. **代码审查**：代码完成后，必须使用code-review skill进行代码审查
5. **接口测试**：使用curl对API接口进行测试

## 开发范围

### API Server
- 改造 `api-server/internal/service/task_service.go` — dispatchToAgent()增加AuditForDispatch调用
- 新建 task_service 下发前校验的单元测试

### Agent
- 新建 `agent/internal/checker/blacklist.go` — Agent侧黑名单检查器
- 改造 `agent/internal/executor/executor.go` — ExecuteCommand增加BlacklistChecker调用
- 规则同步：复用现有UpdateRules gRPC RPC，扩展审计规则下发

### 接口测试
- 创建任务（含恶意脚本）→ 验证audit_blocked状态
- 创建任务（含正常脚本）→ 验证正常下发
```

---

## 提示词3：eBPF文件事件与网络事件采集

```
## 角色与目标

你是一名eBPF+Go工程师，负责Aegis安全平台V5.7的eBPF增强：接入openat和connect程序，实现文件事件和网络事件采集。

## karpathy-guidelines 分析

在开始开发前，使用karpathy-guidelines技能对本需求进行分析，评估：
- openat/connect的C代码是否已就绪（已确认完成编译）
- 内核态过滤策略是否合理（openat高频必须过滤）
- 事件结构体设计是否足够扩展

## 设计文档（必须先阅读并更新）

1. `docs/aegis_system_design_v5.7/prd_design_v5.7.md` — REQ-301~307, REQ-401~408
2. `docs/aegis_system_design_v5.7/ebpf_file_network_event_design.md` — 事件采集架构
3. `docs/aegis_system_design_v5.7/ebpf_kernel_adaptation_design.md` — 内核适配与降级
4. `docs/aegis_system_design_v5.7/backend_detailed_design_v5.7.md` — Agent变更清单

## 禁止事项

- 禁止任何虚假测试
- 禁止不使用skill进行编译、启动、服务测试等操作
- 禁止跳过设计文档直接写代码
- 禁止在没有测试用例的情况下编写业务代码

## 强制要求

1. **设计先行**：先阅读上述设计文档，如有遗漏或变更，先更新设计文档再开发
2. **测试先行**：先写测试用例（Go test），验证测试失败后，再写业务代码使测试通过
3. **编译测试**：所有编译、启动、服务测试操作，必须使用aegis-build-test skill执行
4. **代码审查**：代码完成后，必须使用code-review skill进行代码审查

## 开发范围

### eBPF C代码
- 改造 `agent/internal/ebpf/bpf/connect.bpf.c` — IPv6支持、源地址、回环过滤
- 改造 `agent/internal/ebpf/bpf/openat.bpf.c` — 敏感路径内核态过滤
- 确认 `exit` 程序加入Makefile的BPF_PROGRAMS

### Go加载层
- 新建 `agent/internal/ebpf/events.go` — FileEvent, ConnEvent结构体
- 改造 `agent/internal/ebpf/loader.go` — LoadAll()扩展openat/connect，processEvent分发
- 改造 `agent/internal/ebpf/pipeline.go` — buildEventMap()扩展file_access/network_connect

### 内核适配
- 新建 `agent/internal/kernel/detector.go` — KernelCapabilities检测
- 改造 `agent/internal/ebpf/loader.go` — 统一EventReader接口（RingbufReader/PerfReader）
- 改造 `agent/Makefile` — bpf-core、bpf-noncore、bpf-all目标

### Sigma规则
- 新增文件事件Sigma规则（敏感文件写入）
- 新增网络事件Sigma规则（高风险端口外联）
```

---

## 提示词4：智能体优化

```
## 角色与目标

你是一名Go后端工程师，负责Aegis安全平台V5.7的智能体（ReAct Agent）优化：修复已知问题，提升分析效率和可靠性。

## karpathy-guidelines 分析

在开始开发前，使用karpathy-guidelines技能对本需求进行分析，评估：
- 当前ReAct agent的8个已知问题的优先级
- 迭代参数调整的影响（50→20是否会丢失复杂分析）
- 工具调用安全边界的合理性

## 设计文档（必须先阅读并更新）

1. `docs/aegis_system_design_v5.7/prd_design_v5.7.md` — REQ-501~506
2. `docs/aegis_system_design_v5.7/agent_optimization_design.md` — 完整优化方案
3. `docs/aegis_system_design_v5.7/backend_detailed_design_v5.7.md` — react_agent.go改造

## 禁止事项

- 禁止任何虚假测试
- 禁止不使用skill进行编译、启动、服务测试等操作
- 禁止跳过设计文档直接写代码
- 禁止在没有测试用例的情况下编写业务代码

## 强制要求

1. **设计先行**：先阅读上述设计文档，如有遗漏或变更，先更新设计文档再开发
2. **测试先行**：先写测试用例（Go test），验证测试失败后，再写业务代码使测试通过
3. **编译测试**：所有编译、启动、服务测试操作，必须使用aegis-build-test skill执行
4. **代码审查**：代码完成后，必须使用code-review skill进行代码审查

## 开发范围

### 核心改造
- 改造 `api-server/internal/llm/react_agent.go`
  - 迭代参数：forceFinalAnswer 50→20，maxNoAction 2→3
  - Observation智能截断（JSON数组头尾各5条）
  - 工具调用安全边界（ToolCallGuard：100/会话，10/分钟/工具）
  - Prometheus指标埋点
  - Session持久化（DB+内存，30分钟超时清理）

### 提示词增强
- 改造 `api-server/internal/llm/prompts.go`
  - 增加工具使用规则（精确名称、单工具/次、无用信息3次强制结束）
  - 增加分析策略（先概览再深入、关注异常父子关系、非标准端口）

### 可观测性
- 新增Prometheus指标：agent_iterations、tool_calls、tool_failures、json_parse_errors、session_duration
- 新建 `api-server/internal/llm/agent_metrics.go`

### Session管理
- 改造 `api-server/internal/api/handler/ai_analysis_handler.go`
  - Session持久化到DB
  - 启动时恢复最近24小时活跃会话
  - 30分钟超时自动清理
```

---

## 提示词5：前端页面开发（命令审计配置 + 审计日志）

```
## 角色与目标

你是一名Vue 3前端工程师，负责Aegis安全平台V5.7的前端开发：命令审计配置页和审计日志页。

## karpathy-guidelines 分析

在开始开发前，使用karpathy-guidelines技能对本需求进行分析，评估：
- 组件拆分是否合理（每个组件单一职责）
- API调用层是否需要封装
- Element Plus组件选择是否最优

## 设计文档（必须先阅读并更新）

1. `docs/aegis_system_design_v5.7/prd_design_v5.7.md` — REQ-001~008, REQ-601~608
2. `docs/aegis_system_design_v5.7/frontend_detailed_design_v5.7.md` — 前端组件与页面设计
3. `docs/aegis_system_design_v5.7/command_audit_blacklist_config_design.md` — API接口定义

## 禁止事项

- 禁止任何虚假测试
- 禁止不使用skill进行编译、启动、服务测试等操作
- 禁止跳过设计文档直接写代码
- 禁止在没有测试用例的情况下编写业务代码

## 强制要求

1. **设计先行**：先阅读上述设计文档，如有遗漏或变更，先更新设计文档再开发
2. **测试先行**：先写测试用例（Vitest），验证测试失败后，再写业务代码使测试通过
3. **UI开发**：写前端代码必须使用ui-ux-pro-max skill
4. **编译测试**：所有编译、启动操作，必须使用aegis-build-test skill执行
5. **代码审查**：代码完成后，必须使用code-review skill进行代码审查
6. **接口测试**：使用curl对API接口进行测试

## 开发范围

### 命令审计配置页 (`/settings/command-audit`)
- 新建 `frontend/src/views/settings/CommandAudit/index.vue`
- 新建 `frontend/src/views/settings/CommandAudit/components/AuditPolicyCard.vue`
- 新建 `frontend/src/views/settings/CommandAudit/components/RuleTable.vue`
- 新建 `frontend/src/views/settings/CommandAudit/components/RuleFormDialog.vue`
- 新建 `frontend/src/views/settings/CommandAudit/components/RegexTestPanel.vue`
- 新建 `frontend/src/views/settings/CommandAudit/composables/useCommandAudit.ts`

### 审计日志页 (`/settings/audit-logs`)
- 新建 `frontend/src/views/settings/AuditLogs/index.vue`
- 新建 `frontend/src/views/settings/AuditLogs/components/AuditStatsCard.vue`
- 新建 `frontend/src/views/settings/AuditLogs/components/AuditLogTable.vue`
- 新建 `frontend/src/views/settings/AuditLogs/components/AuditDetailDrawer.vue`
- 新建 `frontend/src/views/settings/AuditLogs/composables/useAuditLogs.ts`

### API封装
- 新建 `frontend/src/api/command-audit.ts`
- 新建 `frontend/src/api/audit-logs.ts`

### 路由与导航
- 改造路由配置，增加command-audit和audit-logs路由
- 改造导航菜单，增加两个菜单项

### 任务状态增强
- 改造任务详情页，增加audit_blocked状态展示
- 增加审计信息展示区（命中规则、错误信息、审计日志链接）
```
