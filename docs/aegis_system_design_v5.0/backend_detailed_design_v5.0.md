# 后端详细设计文档 - V5.0

**版本**: 5.0  
**状态**: 已实现  
**日期**: 2026-03-20
**最后更新**: 2026-03-24

---

## 1. 概述

V5.0 后端负责 AI 运行时检测闭环：接收 Agent 上报事件（已由 Sigma 初筛）、存储告警信息、提供页面手动触发的 AI 降噪功能、根据阻断策略执行自动/手动阻断、推送告警到前端，并管理规则生命周期。

**核心结论（已确认）**：

1. 事件由 Agent 通过 gRPC 直接上报，不经过 Kafka。  
2. 告警去重按 `host_id + pid + mitre_id`，命中后只增加次数。  
3. 阻断动作默认为 `kill_process`，是否自动执行由页面开关决定。  
4. 规则来源于 LLM（可由人工对话驱动生成），24 小时人工未审核则自动下发为 experimental。  
5. LLM 降噪分析由页面手动触发，不再自动定时调用。

### 1.1 实现状态

| 功能 | 状态 | 说明 |
|:---|:---|:---|
| gRPC事件接收 | ✅ 已实现 | Agent直接上报 |
| 告警去重 | ✅ 已实现 | dedupe_key |
| 告警存储 | ✅ 已实现 | PostgreSQL |
| WebSocket推送 | ✅ 已实现 | 实时告警推送 |
| 规则下发 | ✅ 已实现 | gRPC UpdateRules |
| 手动AI降噪 | ✅ 已实现 | 页面触发LLM分析 |
| ~~定时LLM分析~~ | ❌ 已移除 | 改为手动触发 |

### 1.2 已修复问题

| 问题 | 原因 | 解决方案 |
|:---|:---|:---|
| LLMAnalysisService初始化失败 | 传入nil客户端 | 使用configRepo动态获取配置 |
| RuntimeEvent时间戳解析失败 | time.Time vs int64 | 改为int64 Unix毫秒 |
| Alert PID列名不匹配 | GORM映射p_id vs pid | 添加column:pid标签 |
| 定时LLM调用消耗token | 每10秒自动分析 | 移除自动分析，改为手动触发 |

---

## 2. 后端项目结构（V5.0）

```text
/backend
|-- /internal
|   |-- /api
|   |   |-- /handler
|   |   |   |-- detection_handler.go        # 告警、阻断、规则、AI降噪
|   |-- /service
|   |   |-- alert_service.go                # 告警去重、状态流转、推送
|   |   |-- block_service.go                 # 自动/手动阻断
|   |   |-- rule_service.go                  # 规则生命周期、下发
|   |   |-- websocket_service.go             # 实时推送
|   |   |-- llm_analysis_service.go          # LLM调用服务（手动触发）
|   |-- /repository
|   |   |-- alert_repo.go
|   |   |-- block_repo.go
|   |   |-- block_policy_repo.go
|   |   |-- sigma_rule_repo.go
|   |   |-- tool_call_repo.go
|   |   |-- llm_aggregation_repo.go          # AI降噪记录
|   |-- /model
|   |   |-- alert.go
|   |   |-- block_record.go
|   |   |-- block_policy.go
|   |   |-- sigma_rule.go
|   |   |-- sigma_rule_version.go
|   |   |-- tool_call.go
|   |   |-- llm_aggregation.go
|-- /pkg/api/v1
|   |-- agent_comm.proto                      # Agent工具调用/阻断/规则下发扩展
```

---

## 3. 核心流程设计

### 3.1 事件处理主流程

```text
Agent命中Sigma规则后上报事件
    ↓
gRPC ReportEvent 接收事件
    ↓
生成告警（去重判断）
    ├─ 已存在: hit_count + 1, 更新 last_seen_at
    └─ 不存在: 创建新告警
    ↓
存储告警到数据库
    ↓
WebSocket 推送告警到页面
    ↓
用户在页面查看告警
    ↓
用户手动触发 AI 降噪（可选）
    ↓
POST /api/v1/detection/llm/aggregate
    ↓
LLM 分析指定时间范围的告警
    ↓
返回分析结果（威胁判断、处置建议）
    ↓
用户决定是否执行阻断
```

### 3.2 AI 降噪流程（手动触发）

用户在告警页面选择时间范围后手动触发：

```text
用户选择时间范围（最长24小时）
    ↓
POST /api/v1/detection/llm/aggregate
    {
        "start_time": "2026-03-24T10:00:00Z",
        "end_time": "2026-03-24T12:00:00Z",
        "host_ids": ["host-1", "host-2"],  // 可选
        "auto_dispose": false               // 是否自动处置
    }
    ↓
查询 pending 状态告警
    ↓
调用 LLM 分析
    ↓
解析 LLM 返回结果
    ├─ is_threat: 是否为真实威胁
    ├─ llm_summary: 分析摘要
    └─ recommendation: 处置建议
    ↓
更新告警的 LLM 分析结果
    ↓
返回分析结果给页面
```

### 3.3 聚合策略

**已移除自动聚合**，改为：
- 事件实时上报并生成告警
- 用户按需手动触发 AI 降噪分析
- 支持按时间范围（最长24小时）聚合分析

---

## 4. 数据传输设计

### 4.1 gRPC 接口

Agent 通过 gRPC 直接与后端通信，不再使用 Kafka：

| 接口 | 用途 | 备注 |
|:---|:---|:---|
| `Register` | Agent注册 | 首次连接 |
| `Heartbeat` | 心跳保活 | 30秒间隔 |
| `ExecuteCommand` | 双向流命令 | 接收任务/返回结果 |
| `ReportEvent` | 上报运行时事件 | Agent命中规则后上报 |
| `UpdateRules` | 规则下发 | 全量/增量 |

### 4.2 WebSocket 推送

| 消息类型 | 用途 |
|:---|:---|
| `alert` | 新告警通知 |
| `block_status` | 阻断执行结果 |
| `rule_update` | 规则变更通知 |

---

## 5. LLM 分析（手动触发）

### 5.1 触发方式

LLM 分析由用户在页面手动触发，不再自动定时调用：

- **入口**: `POST /api/v1/detection/llm/aggregate`
- **限制**: 时间范围最长 24 小时
- **状态**: 可通过 `GET /api/v1/detection/llm/aggregate/:id` 查询进度

### 5.2 LLM 输入数据

LLM 输入为指定时间范围内的 `pending` 状态告警列表：

- `alert_id`
- `event_type`
- `pid`
- `command_line`
- `matched_rule_id`
- `mitre_id`
- `severity`
- `timestamp`

### 5.3 LLM 输出结构

```json
{
  "alerts": [
    {
      "alert_id": "alert-xxx",
      "is_threat": true,
      "llm_summary": "检测到可疑的反弹shell行为...",
      "recommendation": "建议立即终止进程并检查网络连接"
    }
  ]
}
```

### 5.4 成本优化

相比自动定时分析，手动触发具有以下优势：

| 对比项 | 自动定时 | 手动触发 |
|:---|:---|:---|
| LLM调用频率 | 每10秒检查 | 按需调用 |
| Token消耗 | 高（持续消耗） | 低（按需消耗） |
| 分析精度 | 固定窗口 | 用户选择范围 |
| 可控性 | 低 | 高 |

---

## 6. 告警去重与状态流转

### 6.1 去重键

告警唯一去重键：

`dedupe_key = host_id + pid + mitre_id`

### 6.2 去重行为

- 若不存在：创建新告警
- 若已存在：`hit_count + 1`，更新时间 `last_seen_at`

### 6.3 告警状态

- `active`
- `resolved`

---

## 7. 阻断策略与执行

### 7.1 阻断控制逻辑

每条告警关联策略 `block_policies`：

- `enabled=true` 且 `auto_block=true`：自动阻断
- `enabled=true` 且 `auto_block=false`：仅告警，页面手动阻断

### 7.2 阻断动作

当前固定：`kill_process`

### 7.3 手动阻断

前端在告警详情点击“阻断”后，调用手动阻断 API，后端下发 `kill_process` 指令。

---

## 8. 规则生命周期管理

### 8.1 来源与审核

- 规则来源：LLM 生成（人工可参与提示词交互）
- 创建后状态：`pending`
- 24 小时内人工审核：可直接 `active` / `disabled`
- 24 小时无人干预：自动下发 `experimental`

### 8.2 自动转换

- `experimental` 规则运行满 7 天且无人工干预 -> 自动转 `active`

### 8.3 下发机制

- 首次：全量下发
- 变更：增量下发（add/update/delete）

---

## 9. API 设计（V5.0）

### 9.1 告警

- `GET /api/v1/detection/alerts` - 获取告警列表
- `GET /api/v1/detection/alerts/:id` - 获取告警详情
- `POST /api/v1/detection/alerts/:id/resolve` - 标记告警已处理
- `POST /api/v1/detection/alerts/:id/block` - 手动阻断

### 9.2 阻断策略

- `GET /api/v1/detection/block-policies` - 获取阻断策略列表
- `PUT /api/v1/detection/block-policies/:mitre_id` - 更新 auto_block 开关

### 9.3 规则

- `POST /api/v1/detection/rules/import` - 导入规则
- `GET /api/v1/detection/rules` - 获取规则列表
- `GET /api/v1/detection/rules/:id` - 获取规则详情
- `PUT /api/v1/detection/rules/:id/status` - 更新规则状态

### 9.4 工具调用审计

- `GET /api/v1/detection/tool-calls` - 获取工具调用列表

### 9.5 AI 降噪（手动触发）

- `POST /api/v1/detection/llm/aggregate` - 启动 AI 降噪分析
- `GET /api/v1/detection/llm/aggregate/:id` - 查询分析状态和结果
- `GET /api/v1/detection/llm/aggregate/current` - 获取当前分析任务

### 9.6 WebSocket

- `GET /api/v1/detection/runtime/ws` - 实时告警推送

推送消息类型：
- `alert` - 新告警
- `block_status` - 阻断结果
- `rule_update` - 规则更新

---

## 10. 数据库模型（后端）

> 仅存储告警、阻断、规则与审计，不存储原始事件。

### 10.1 alerts

- `alert_id`
- `host_id`
- `pid`
- `mitre_id`
- `severity`
- `description`
- `dedupe_key`
- `hit_count`
- `auto_blocked`
- `manual_blocked`
- `status`
- `first_seen_at`
- `last_seen_at`

### 10.2 block_records

- `block_id`
- `alert_id`
- `host_id`
- `action`（kill_process）
- `target`（pid）
- `success`
- `message`
- `created_at`

### 10.3 block_policies

- `mitre_id`
- `enabled`
- `auto_block`
- `updated_at`

### 10.4 sigma_rules

- `rule_id`
- `title`
- `content`
- `status`（pending/experimental/active/disabled）
- `generated_by`
- `created_at`
- `activated_at`

### 10.5 sigma_rule_versions

- `rule_id`
- `version`
- `content`
- `change_reason`
- `created_at`

### 10.6 tool_calls

- `call_id`
- `host_id`
- `tool`
- `params_json`
- `result_json`
- `status`
- `created_at`

---

## 11. 关键实现伪代码

### 11.1 AI 降噪（手动触发）

```go
func (h *DetectionHandler) StartLLMAggregation(c *gin.Context) {
    var body struct {
        StartTime   string   `json:"start_time" binding:"required"`
        EndTime     string   `json:"end_time" binding:"required"`
        HostIDs     []string `json:"host_ids"`
        AutoDispose bool     `json:"auto_dispose"`
    }
    
    // 验证时间范围（最长24小时）
    if endTime.Sub(startTime) > 24*time.Hour {
        return Error("time range exceeds maximum of 24 hours")
    }
    
    // 查询 pending 状态告警
    alerts := h.alertRepo.List(status: "pending")
    
    // 调用 LLM 分析
    result := h.callLLMForAlerts(ctx, alerts)
    
    // 更新告警的 LLM 分析结果
    for _, alertResult := range result.Alerts {
        h.alertRepo.UpdateLLMSummary(alertResult.AlertID, alertResult.LLMSummary)
    }
    
    return Success(result)
}
```

### 11.2 告警去重

```go
func (r *AlertRepo) UpsertByDedupe(hostID string, pid int, mitreID string, alert AlertPayload) {
    key := fmt.Sprintf("%s:%d:%s", hostID, pid, mitreID)
    existed := r.FindByDedupeKey(key)
    if existed == nil {
        r.Create(alert.WithDedupeKey(key))
        return
    }
    existed.HitCount += 1
    existed.LastSeenAt = time.Now()
    r.Update(existed)
}
```

### 11.3 自动/手动阻断

```go
func (e *DecisionExecutor) HandleAlert(a Alert) {
    policy := e.policyRepo.GetByMitreID(a.MitreID)
    if policy.Enabled && policy.AutoBlock {
        e.grpc.SendBlockCommand(a.HostID, "kill_process", strconv.Itoa(a.PID))
        e.alertRepo.MarkAutoBlocked(a.AlertID)
        return
    }
    // 仅告警，等待用户手动触发 /alerts/:id/block
}
```

---

## 12. 非功能要求

1. 主机窗口聚合准确，不跨主机串窗。  
2. 单窗口处理失败可重试，不影响其他主机窗口。  
3. LLM 超时、解析失败要落审计日志并可观测。  
4. 工具调用链路全记录（请求、响应、耗时、错误）。  
5. 规则下发具备幂等性（重复下发不重复生效）。

---

## 13. 本版范围（MVP）

- ✅ gRPC 事件接收与告警生成  
- ✅ 告警去重（host_id + pid + mitre_id）  
- ✅ 手动触发 AI 降噪分析  
- ✅ 自动阻断开关 + 手动阻断入口  
- ✅ 规则 pending/experimental/active 流转  
- ✅ 规则全量/增量下发
- ❌ ~~定时 LLM 分析~~（已移除，改为手动触发）

---

## 14. 变更记录

| 日期 | 版本 | 变更内容 |
|:---|:---|:---|
| 2026-03-24 | 5.0.1 | 移除定时 LLM 分析，改为页面手动触发 AI 降噪 |
| 2026-03-20 | 5.0 | 初始版本 |

---

**文档结束**
