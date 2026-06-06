# Aegis V6.0 API 与数据库设计差异分析

**版本**: 6.0
**日期**: 2026-06-05
**状态**: 分析完成

---

## 1. 分析概述

本文档对比 `api_database_design_v6.0.md` 设计文档与当前代码实现的差异，识别出以下类别的缺口：

| 类别 | 设计要求 | 已实现 | 缺口数 |
|:---|:---|:---|:---|
| 数据库表 | 12 | 12 | 0 |
| 数据库 Repository | 12 | 11 | 1 |
| API 端点 | 33 | 31 | 2 |
| SSE 事件类型 | 18 | 18 (2 未使用) | 0 |
| 配置项 | 22 | 0 | 22 |

---

## 2. API 端点差异

### 2.1 缺失端点

| 方法 | 路径 | 说明 | 状态 |
|:---|:---|:---|:---|
| POST | `/investigations/:investigation_id/rebuild-report` | 基于已收集证据重生成研判报告 | **未实现** |
| GET | `/mcp-sources/:source_id/query-logs` | 查询 MCP 数据源调用日志 | **未实现** |

### 2.2 已实现端点 (31/33)

**会话管理 (10/10)**
- `GET /sessions` ✓
- `POST /sessions` ✓
- `GET /sessions/:session_id` ✓
- `GET /sessions/:session_id/messages` ✓
- `POST /sessions/:session_id/message` ✓
- `GET /sessions/:session_id/stream` ✓
- `POST /sessions/:session_id/cancel` ✓
- `GET /sessions/:session_id/context-refs` ✓
- `GET /sessions/:session_id/tool-calls` ✓
- `GET /sessions/:session_id/approvals` ✓

**工具策略 (6/6)**
- `GET /tools` ✓
- `GET /tool-approval-policy` ✓
- `PUT /tool-approval-policy` ✓
- `PUT /tools/:tool_name/whitelist` ✓
- `POST /tools/whitelist/batch` ✓
- `POST /tools/whitelist/reset-defaults` ✓

**审批 (3/3)**
- `GET /approvals/:approval_id` ✓
- `POST /approvals/:approval_id/approve` ✓
- `POST /approvals/:approval_id/reject` ✓

**研判 (2/3)**
- `POST /investigations/host-attack` ✓
- `GET /investigations/:investigation_id` ✓
- `GET /investigations/:investigation_id/evidence` ✓
- `POST /investigations/:investigation_id/rebuild-report` ❌ **未实现**

**MCP 数据源 (6/7)**
- `GET /mcp-sources` ✓
- `POST /mcp-sources` ✓
- `GET /mcp-sources/:source_id` ✓
- `PUT /mcp-sources/:source_id` ✓
- `DELETE /mcp-sources/:source_id` ✓
- `POST /mcp-sources/:source_id/test` ✓
- `POST /mcp-sources/:source_id/sync-schema` ✓
- `GET /mcp-sources/:source_id/query-logs` ❌ **未实现**

---

## 3. 数据库表与 Repository 差异

### 3.1 表实现状态 (12/12)

| 表名 | 模型 | 迁移 | AutoMigrate | Repository |
|:---|:---|:---|:---|:---|
| assistant_sessions | ✓ | ✓ | ✓ | ✓ |
| assistant_messages | ✓ | ✓ | ✓ | ✓ |
| assistant_context_refs | ✓ | ✓ | ✓ | ✓ |
| assistant_tool_calls | ✓ | ✓ | ✓ | ✓ |
| assistant_approvals | ✓ | ✓ | ✓ | ✓ |
| assistant_tool_selections | ✓ | ✓ | ✓ | ❌ **无独立 Repository** |
| assistant_tool_policies | ✓ | ✓ | ✓ | ✓ |
| assistant_memory | ✓ | ✓ | ✓ | ✓ |
| assistant_investigation_reports | ✓ | ✓ | ✓ | ✓ |
| assistant_investigation_evidence | ✓ | ✓ | ✓ | ✓ |
| external_mcp_sources | ✓ | ✓ | ✓ | ✓ |
| external_mcp_query_logs | ✓ | ✓ | ✓ | ✓ |

### 3.2 缺失 Repository

`assistant_tool_selections` 表有模型定义和自动迁移，但没有独立的 Repository 文件。当前由 `tool_selector.go` 直接操作。

**建议**: 创建 `assistant_tool_selection_repo.go` 以保持架构一致性。

---

## 4. SSE 事件类型差异

### 4.1 实现状态 (18/18)

| 事件类型 | 常量 | Payload Helper | 实际使用 |
|:---|:---|:---|:---|
| `thinking` | ✓ | ✓ | ✓ |
| `message_delta` | ✓ | ✓ | ✓ |
| `intent_detected` | ✓ | 内联 | ✓ |
| `tools_selected` | ✓ | 内联 | ✓ |
| `tool_search` | ✓ | ✓ | ✓ |
| `tool_expansion` | ✓ | ✓ | ✓ |
| `plan` | ✓ | ✓ | ✓ |
| `step_started` | ✓ | 内联 | ✓ |
| `step_completed` | ✓ | 内联 | ✓ |
| `tool_call` | ✓ | ✓ | ✓ |
| `tool_result` | ✓ | ✓ | ✓ |
| `tool_error` | ✓ | ✓ | ✓ |
| `approval_required` | ✓ | ✓ | ✓ |
| `approval_updated` | ✓ | ❌ | ❌ **从未触发** |
| `context_ref_added` | ✓ | ❌ | ❌ **从未触发** |
| `result_card` | ✓ | ✓ | ✓ |
| `done` | ✓ | ✓ | ✓ |
| `error` | ✓ | ✓ | ✓ |

### 4.2 额外实现的事件类型

设计文档外新增了 3 个事件类型：

| 事件类型 | 常量 | 用途 |
|:---|:---|:---|
| `run_started` | `EventRunStarted` | 编排开始时触发 |
| `run_waiting_approval` | `EventRunWaitingApproval` | 等待审批时触发 |
| `business_object` | `EventBusinessObject` | 业务对象事件 |

---

## 5. 配置项差异 (0/22)

设计文档 Section 6 指定了 22 个 `assistant.*` 配置项，**全部未实现为可配置项**，当前均硬编码在 Go 源码中。

### 5.1 顶层配置

| 配置键 | 设计默认值 | 当前实现 | 位置 |
|:---|:---|:---|:---|
| `assistant.enabled` | `true` | 始终启用，无开关 | main.go |
| `assistant.max_iterations` | `500` | **硬编码 80** | orchestrator.go:369, runtime_factory.go:183 |
| `assistant.max_selected_tools` | `24` | **硬编码 24** | orchestrator.go:132, tool_selector.go:65 |
| `assistant.max_write_tools` | `6` | **硬编码 6** | tool_selector.go:69 |
| `assistant.tool_approval_mode` | `whitelist` | **硬编码 "whitelist"** | tool_policy_service.go:60 |
| `assistant.require_approval_medium` | `true` | **未实现** | - |
| `assistant.approval_ttl_minutes` | `30` | **硬编码 30** | approval_gate.go:129 |
| `assistant.max_context_refs` | `50` | **未实现，无限制** | - |
| `assistant.max_tool_calls` | `100` | **硬编码 60** | runtime_factory.go:188 |

### 5.2 研判配置

| 配置键 | 设计默认值 | 当前实现 | 位置 |
|:---|:---|:---|:---|
| `assistant.investigation.enabled` | `true` | 始终启用，无开关 | main.go |
| `assistant.investigation.default_time_range_hours` | `24` | **未实现** | - |
| `assistant.investigation.alert_context_before_hours` | `2` | **未实现** | - |
| `assistant.investigation.alert_context_after_hours` | `6` | **未实现** | - |
| `assistant.investigation.max_evidence_items` | `200` | **来自 API 请求** | model:88 |
| `assistant.investigation.agent_live_probe_enabled` | `true` | **来自 API 请求** | model:85 |
| `assistant.investigation.external_mcp_default` | `false` | **来自 API 请求** | model:86 |
| `assistant.investigation.report_prompt_max_chars` | `32000` | **未实现** | - |

### 5.3 外部 MCP 配置

| 配置键 | 设计默认值 | 当前实现 | 位置 |
|:---|:---|:---|:---|
| `assistant.external_mcp.enabled` | `true` | 始终启用，无开关 | main.go |
| `assistant.external_mcp.max_sources_per_run` | `3` | **未实现** | - |
| `assistant.external_mcp.max_query_per_run` | `6` | **未实现** | - |
| `assistant.external_mcp.default_max_rows` | `50` | **来自数据源配置** | model:24 |
| `assistant.external_mcp.max_context_chars` | `24000` | **未实现** | - |
| `assistant.external_mcp.allowed_transports` | `streamable_http,sse` | **未实现** | - |

---

## 6. 设计值与实现值对比

### 6.1 数值差异

| 参数 | 设计值 | 实现值 | 差异 |
|:---|:---|:---|:---|
| max_iterations | 500 | 80 | **-420** |
| max_tool_calls | 100 | 60 | **-40** |
| max_selected_tools | 24 | 24 | 一致 |
| max_write_tools | 6 | 6 | 一致 |
| approval_ttl_minutes | 30 | 30 | 一致 |

### 6.2 建议

1. **max_iterations**: 设计值 500 与实现值 80 差异较大，需确认业务需求
2. **max_tool_calls**: 设计值 100 与实现值 60 差异较大，需确认业务需求

---

## 7. 补充建议

### 7.1 高优先级

1. **实现缺失的 API 端点**
   - `POST /investigations/:investigation_id/rebuild-report`
   - `GET /mcp-sources/:source_id/query-logs`

2. **创建缺失的 Repository**
   - `assistant_tool_selection_repo.go`

3. **实现配置项读取**
   - 使用 `SystemConfigRepo` 读取 `assistant.*` 配置
   - 修改 `ToolPolicyService.GetApprovalMode()` 从数据库读取

### 7.2 中优先级

4. **补齐 SSE 事件触发**
   - 实现 `approval_updated` 事件触发逻辑
   - 实现 `context_ref_added` 事件触发逻辑

5. **添加 Payload Helper 函数**
   - `intent_detected` → `EventIntentDetectedPayload()`
   - `tools_selected` → `EventToolsSelectedPayload()`
   - `step_started` → `EventStepStartedPayload()`
   - `step_completed` → `EventStepCompletedPayload()`

### 7.3 低优先级

6. **添加配置项到 `system_configs` 表**
   - 初始化脚本插入默认配置
   - 提供配置管理 API

7. **数值对齐**
   - 确认 `max_iterations` 和 `max_tool_calls` 的正确值
   - 更新设计文档或代码实现

---

## 8. 关键文件参考

| 类别 | 文件路径 |
|:---|:---|
| 设计文档 | `docs/aegis_system_design_v6.0/api_database_design_v6.0.md` |
| 数据库迁移 | `migrations/015_v6.0_assistant_tables.sql` |
| 模型定义 | `api-server/internal/model/assistant.go` |
| 模型定义 | `api-server/internal/model/assistant_investigation.go` |
| 模型定义 | `api-server/internal/model/external_mcp.go` |
| Repository | `api-server/internal/repository/assistant_*_repo.go` (11 files) |
| Repository | `api-server/internal/repository/external_mcp_*_repo.go` (2 files) |
| Handler | `api-server/internal/api/handler/assistant_handler.go` |
| Service | `api-server/internal/assistant/service.go` |
| Service | `api-server/internal/assistant/orchestrator.go` |
| Service | `api-server/internal/assistant/tool_selector.go` |
| Service | `api-server/internal/assistant/tool_policy_service.go` |
| Service | `api-server/internal/assistant/approval_gate.go` |
| Service | `api-server/internal/assistant/runtime_factory.go` |
| SSE Events | `api-server/internal/assistant/event.go` |
| 主入口 | `api-server/cmd/main.go` |
