# Aegis V5.0 核心架构设计方案

**版本**: 5.0  
**状态**: 设计中  
**日期**: 2026-03-20

---

## 1. 设计约束（对齐版）

| 约束条件 | 要求 | 影响 |
|:---|:---|:---|
| Agent资源 | 1C1G | Agent 仅做 eBPF + Sigma 初筛，不做本地LLM |
| 内核版本 | >= 4.17 | 使用 cilium/ebpf 采集事件 |
| 检测范围 | ATT&CK 14战术 | Sigma 初筛 + LLM 深度研判 |
| 事件分析窗口 | 2分钟/主机 | 按 host_id 聚合后批量送 LLM |
| 原始事件存储 | 不存储 | 分析后丢弃，仅保留告警与阻断记录 |
| 阻断动作 | kill_process | 自动/手动由策略开关控制 |
| 工具调用 | 最多10次/研判 | LLM 可按需向 Agent 拉取补充证据 |

---

## 2. 核心架构

### 2.1 架构图

```text
┌──────────────────────────────────────────────────────────────────┐
│ Frontend (Vue 3)                                                │
│  - 告警中心 - 阻断策略开关 - 规则中心 - ATT&CK矩阵             │
└──────────────────────────────────────────────────────────────────┘
                              ▲ WebSocket + REST
                              │
┌──────────────────────────────────────────────────────────────────┐
│ Backend (Go)                                                    │
│  - Kafka Consumer                                                │
│  - Host Window Aggregator (2分钟/主机)                          │
│  - LLM Analyzer（降噪/研判/阻断建议）                           │
│  - Tool Call Orchestrator（最多10次）                           │
│  - Alert Deduper（host_id + pid + mitre_id）                    │
│  - Block Executor（自动/手动）                                  │
│  - Rule Lifecycle Manager（pending/experimental/active）        │
└──────────────────────────────────────────────────────────────────┘
                              ▲ gRPC + Kafka
                              │
┌──────────────────────────────────────────────────────────────────┐
│ Agent (Go, 1C1G)                                                │
│  - eBPF Collector（4.17+）                                      │
│  - Sigma Matcher（命中才上报）                                  │
│  - Block Executor（执行 kill_process）                          │
│  - Tool Provider（get_process_tree / execute_command 等）        │
└──────────────────────────────────────────────────────────────────┘
```

---

## 3. 关键闭环

### 3.1 事件闭环

```text
Agent采集事件 -> Sigma命中后上报 -> Kafka
    -> Backend按主机聚合2分钟 -> LLM分析
    -> 告警去重 -> 自动/手动阻断 -> WebSocket推送
    -> 丢弃原始事件
```

### 3.2 阻断闭环

```text
LLM返回阻断建议(kill_process)
    -> 查询策略 auto_block
        -> true: 后端自动下发阻断
        -> false: 仅告警，等待用户手动触发
```

### 3.3 规则闭环

```text
LLM生成规则 -> status=pending
    -> 24小时人工审核?
       -> 是: active/disabled
       -> 否: 自动下发为 experimental
    -> experimental 运行满7天无干预 -> active
```

---

## 4. 分层职责（最终版）

### 4.1 Agent层

- eBPF 采集（process_exec/fork/exit, file_access, network_connect, privilege_change）
- Sigma 规则匹配（仅命中事件上报）
- 执行阻断命令（kill_process）
- 执行工具调用并返回结果

### 4.2 Backend层

- Kafka 消费与主机窗口聚合（2分钟）
- LLM 分析与工具调用编排（<=10次）
- 告警去重与状态管理
- 自动/手动阻断编排
- 规则生命周期管理与下发

### 4.3 Frontend层

- 告警实时展示
- 自动阻断策略开关
- 手动阻断入口
- 规则状态与版本信息展示

---

## 5. 数据存储原则（最终版）

| 数据类型 | 是否存储 | 说明 |
|:---|:---|:---|
| 原始事件 | 否 | 窗口分析后丢弃 |
| 告警 | 是 | 去重后落库，累计命中次数 |
| 阻断记录 | 是 | 自动/手动阻断审计 |
| 规则与版本 | 是 | 生命周期与回溯 |
| 工具调用记录 | 是 | 审计与排障 |

---

## 6. 对齐结论

1. 不再使用 Redis Streams 作为主事件队列，统一使用 Kafka。  
2. 不再使用 YARA 作为主规则格式，统一 Sigma。  
3. 不再要求端到端 <3 秒实时判定，改为 2 分钟主机窗口研判。  
4. 阻断统一为 kill_process，是否自动执行由开关控制。  
5. 原始事件不入库，仅保留告警和策略性结果。

---

**文档结束**
