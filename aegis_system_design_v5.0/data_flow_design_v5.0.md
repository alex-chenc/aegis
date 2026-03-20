# 数据流与事件处理设计 - V5.0

**版本**: 5.0  
**状态**: 设计中  
**日期**: 2026-03-20

---

## 1. 总体数据流（对齐版）

```text
Agent (eBPF采集 -> Sigma匹配)
    -> 命中事件上报
    -> Kafka raw-events
    -> Backend Consumer
    -> Host Window Aggregator (2分钟/主机)
    -> LLM Analyzer
       -> (可选) Tool Call Loop <=10
    -> Alert Deduper(host_id+pid+mitre_id)
    -> Auto/Manual Block Path
    -> WebSocket Push
    -> Drop Raw Events (不落库)
```

---

## 2. 输入输出定义

### 2.1 输入（Agent上报）

输入是**已命中 Sigma 规则**的事件，关键字段：

- `event_id`
- `host_id`
- `hostname`
- `timestamp`
- `event_type`
- `pid`
- `command_line`
- `matched_rule_id`
- `mitre_id`
- `severity`

### 2.2 输出（Backend产物）

- 告警记录（去重后）
- 阻断记录（自动/手动）
- 规则调整（状态与版本）
- 工具调用审计

---

## 3. Kafka 流程

### 3.1 Topic

| Topic | 方向 | 说明 |
|:---|:---|:---|
| `raw-events` | Agent -> Backend | 规则命中事件输入 |
| `analysis-results` | Backend内部 | LLM分析结果（可选持久） |
| `block-commands` | Backend -> Agent | 阻断指令 |
| `rule-updates` | Backend -> Agent | 规则全量/增量下发 |
| `tool-calls` | Backend内部 | 工具调用请求与返回审计 |

### 3.2 分区策略

- `raw-events`：按 `host_id` 分区
- `block-commands`：按 `host_id` 分区

目的：保障同一主机事件和动作顺序一致。

---

## 4. 聚合与分析

### 4.1 聚合器策略

- 维度：`host_id`
- 窗口：2分钟固定窗口
- 窗口输出：`map[host_id][]events`
- 输出后：窗口清空

### 4.2 分析策略

每个主机窗口独立调用 LLM：

1. 首次分析窗口事件
2. 若 LLM 请求补充证据，调用 Agent 工具
3. 工具回路最多 10 次
4. 生成最终 `alerts / block_decisions / rule_adjustments`

---

## 5. 工具调用回路

### 5.1 回路约束

- 单次研判最大调用次数：10
- 超限策略：停止继续调用，输出当前最佳结论并标记 `tool_limit_reached=true`

### 5.2 回路流程

```text
LLM初次分析
  -> 返回 tool_calls?
      -> 否: 输出最终结果
      -> 是: Backend转发到Agent
             -> Agent执行并返回结果
             -> Backend回填结果给LLM
             -> 下一轮分析
             -> 达到10次上限则结束
```

---

## 6. 告警去重流

### 6.1 去重键

`dedupe_key = host_id + pid + mitre_id`

### 6.2 去重规则

- 不存在：创建新告警（`hit_count=1`）
- 已存在：不新增，`hit_count += 1`，更新 `last_seen_at`

### 6.3 状态

- `active`
- `resolved`

---

## 7. 阻断流

### 7.1 自动阻断流

```text
LLM返回 block_action=kill_process
    -> 读取 block_policy(auto_block)
        -> true: 立即下发 block-commands
        -> false: 不下发，仅告警
```

### 7.2 手动阻断流

```text
用户在告警详情点击“阻断”
    -> POST /api/v1/alerts/:id/block
    -> Backend下发 kill_process
    -> 写入 block_records
```

---

## 8. 规则更新流

### 8.1 生命周期驱动

```text
LLM生成规则 -> pending
  -> 24小时人工审核?
      -> 是: active/disabled
      -> 否: experimental（自动下发）
  -> experimental运行7天无干预 -> active
```

### 8.2 下发方式

- 首次：全量同步
- 后续：增量同步（add/update/delete）

---

## 9. 数据保留策略

| 数据 | 保留策略 |
|:---|:---|
| 原始事件 | 不落库，窗口结束即丢弃 |
| 告警 | 持久化 |
| 阻断记录 | 持久化 |
| 规则与版本 | 持久化 |
| 工具调用日志 | 持久化 |

---

## 10. 关键伪代码

### 10.1 Pipeline Tick

```go
func (s *PipelineService) Tick() {
    batches := s.aggregator.FlushReady(2 * time.Minute)
    for hostID, events := range batches {
        res := s.llm.Analyze(hostID, events)
        s.executor.Apply(res)
        // 注意：events不写数据库
    }
}
```

### 10.2 去重 Upsert

```go
func (r *AlertRepo) UpsertByDedupe(hostID string, pid int, mitreID string, payload AlertPayload) {
    key := fmt.Sprintf("%s:%d:%s", hostID, pid, mitreID)
    old := r.FindByDedupeKey(key)
    if old == nil {
        r.Create(payload.WithDedupeKey(key).WithHitCount(1))
        return
    }
    old.HitCount++
    old.LastSeenAt = time.Now()
    r.Update(old)
}
```

### 10.3 自动/手动阻断

```go
func (e *DecisionExecutor) Handle(alert Alert) {
    policy := e.policyRepo.GetByMitreID(alert.MitreID)
    if policy.Enabled && policy.AutoBlock {
        e.grpc.SendBlockCommand(alert.HostID, "kill_process", strconv.Itoa(alert.PID))
        e.alertRepo.MarkAutoBlocked(alert.AlertID)
        return
    }
    // 仅告警，等待手动阻断
}
```

---

## 11. 非功能约束

1. 单主机窗口失败不能影响其他主机窗口处理。  
2. LLM 调用失败要有重试与审计日志。  
3. 工具调用全链路可追踪（请求、参数、响应、耗时、错误）。  
4. 规则下发操作幂等。  
5. WebSocket 推送失败要可重试或降级轮询。

---

**文档结束**
