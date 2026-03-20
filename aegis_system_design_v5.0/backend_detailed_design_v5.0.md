# 后端详细设计文档 - V5.0

**版本**: 5.0  
**状态**: 设计中  
**日期**: 2026-03-20

---

## 1. 概述

V5.0 后端负责 AI 运行时检测闭环：接收 Agent 上报事件（已由 Sigma 初筛）、按主机进行 2 分钟聚合、调用 LLM 进行降噪与研判、根据阻断策略执行自动/手动阻断、推送告警到前端，并管理规则生命周期。

**核心结论（已确认）**：

1. 事件按主机聚合，窗口固定 2 分钟。  
2. 原始事件不落库，仅在窗口内内存/Kafka 中转。  
3. 告警去重按 `host_id + pid + mitre_id`，命中后只增加次数。  
4. 阻断动作默认为 `kill_process`，是否自动执行由页面开关决定。  
5. 规则来源于 LLM（可由人工对话驱动生成），24 小时人工未审核则自动下发为 experimental。  
6. LLM 可调用 Agent 工具（最多 10 次/研判）。

---

## 2. 后端项目结构（V5.0）

```text
/backend
|-- /internal
|   |-- /api
|   |   |-- /handler
|   |   |   |-- runtime_handler.go           # 运行时事件、告警、阻断、规则
|   |-- /service
|   |   |-- runtime_pipeline_service.go      # 2分钟聚合 + LLM分析主流程
|   |   |-- llm_analysis_service.go          # LLM调用、工具调用回路
|   |   |-- alert_service.go                 # 告警去重、状态流转、推送
|   |   |-- block_service.go                 # 自动/手动阻断
|   |   |-- rule_service.go                  # 规则生命周期、下发
|   |   |-- websocket_service.go             # 实时推送
|   |-- /repository
|   |   |-- alert_repo.go
|   |   |-- block_repo.go
|   |   |-- block_policy_repo.go
|   |   |-- sigma_rule_repo.go
|   |   |-- tool_call_repo.go
|   |-- /model
|   |   |-- alert.go
|   |   |-- block_record.go
|   |   |-- block_policy.go
|   |   |-- sigma_rule.go
|   |   |-- sigma_rule_version.go
|   |   |-- tool_call.go
|   |-- /queue
|   |   |-- kafka_consumer.go
|   |   |-- kafka_producer.go
|   |-- /pipeline
|   |   |-- host_window_aggregator.go
|   |   |-- llm_prompt_builder.go
|   |   |-- llm_response_parser.go
|-- /pkg/api/v1
|   |-- agent_comm.proto                      # Agent工具调用/阻断/规则下发扩展
```

---

## 3. 核心流程设计

### 3.1 事件处理主流程

```text
Agent命中Sigma规则后上报事件
    ↓
Kafka Topic: raw-events
    ↓
RuntimePipelineConsumer 消费
    ↓
按 host_id 进入 2分钟窗口聚合
    ↓
窗口到期 -> 调用 LLM 分析
    ↓
LLM需要补充信息？
    ├─ 是：调用 Agent 工具（最多10次）-> 带工具结果重试分析
    └─ 否：直接输出告警/阻断/规则调整
    ↓
告警去重（host_id + pid + mitre_id）
    ↓
检查阻断策略 auto_block
    ├─ true: 自动下发 kill_process
    └─ false: 只告警，等待用户手动下发阻断
    ↓
WebSocket 推送结果到页面
    ↓
丢弃窗口原始事件（不入库）
```

### 3.2 聚合策略

- 窗口长度：2 分钟（固定）
- 维度：每个主机独立窗口（`host_id`）
- 窗口结束即触发分析
- 原始事件只在窗口内保留

---

## 4. Kafka 设计

### 4.1 Topic 定义

| Topic | 用途 | 备注 |
|:---|:---|:---|
| `raw-events` | Agent上报事件 | 主输入队列 |
| `analysis-results` | LLM分析结果 | 可选审计用途 |
| `block-commands` | 阻断指令 | 后端发往Agent |
| `rule-updates` | 规则下发事件 | 全量/增量下发 |
| `tool-calls` | 工具调用链路日志 | 审计/排障 |

### 4.2 分区策略

- `raw-events` 按 `host_id` 分区，保证单主机事件顺序。
- `block-commands` 按 `host_id` 分区，保证阻断顺序。

---

## 5. LLM 分析与工具调用

### 5.1 LLM 输入数据

LLM 输入为**某主机 2 分钟聚合事件集**，每条事件包含：

- `event_type`
- `pid`
- `command_line`
- `matched_rule_id`
- `mitre_id`
- `severity`
- `timestamp`

### 5.2 LLM 输出结构（约定）

```json
{
  "alerts": [
    {
      "mitre_id": "T1059.004",
      "severity": "critical",
      "pid": 12345,
      "description": "检测到反弹shell行为",
      "block_action": "kill_process",
      "block_target": "12345"
    }
  ],
  "tool_calls": [
    {
      "tool": "get_process_tree",
      "params": {"pid": 12345},
      "reason": "确认父子进程链"
    }
  ],
  "rule_adjustments": [
    {
      "rule_id": "reverse_shell_t1059_004",
      "action": "tighten",
      "reason": "降低误报"
    }
  ]
}
```

### 5.3 工具调用限制

- 单次研判最多 10 次工具调用。
- 超过 10 次直接终止工具回路并输出当前最佳结论。

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

- `GET /api/v1/alerts`
- `GET /api/v1/alerts/:id`
- `POST /api/v1/alerts/:id/resolve`
- `POST /api/v1/alerts/:id/block`  （手动阻断）

### 9.2 阻断策略

- `GET /api/v1/block-policies`
- `PUT /api/v1/block-policies/:mitre_id`  （更新 auto_block 开关）

### 9.3 规则

- `GET /api/v1/rules`
- `GET /api/v1/rules/:id`
- `PUT /api/v1/rules/:id/status`

### 9.4 工具调用审计

- `GET /api/v1/tool-calls`
- `GET /api/v1/tool-calls/:id`

### 9.5 WebSocket

- `GET /api/v1/runtime/ws`

推送消息类型：
- `alert`
- `block_status`
- `rule_update`

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

### 11.1 聚合与分析调度

```go
func (s *RuntimePipelineService) Tick() {
    batches := s.aggregator.FlushReady(2 * time.Minute) // host_id -> []events
    for hostID, events := range batches {
        result := s.llm.Analyze(hostID, events)
        s.executor.Apply(result)
        // 原始events直接丢弃
    }
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

- ✅ Kafka + 2分钟主机窗口聚合  
- ✅ LLM 分析 + 最多 10 次工具调用  
- ✅ 告警去重（host_id + pid + mitre_id）  
- ✅ 自动阻断开关 + 手动阻断入口  
- ✅ 规则 pending/experimental/active 流转  
- ✅ 规则全量/增量下发

---

**文档结束**
