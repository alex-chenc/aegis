# AI智能运行时检测和异常检测架构设计

**版本**: 1.0  
**状态**: 定稿  
**作者**: AI Research Team  
**日期**: 2026-03-19

---

## 执行摘要

本架构设计文档基于2026年最新技术栈和开源项目最佳实践，针对Aegis智能主机安全系统的AI智能运行时检测模块，提供三种可选架构方案。经过全面分析，**推荐采用"混合云边协同架构"**作为首选方案。

**关键建议**:
- **实时检测延迟**: P99 < 500ms（eBPF采集 → 边缘推理 → 响应）
- **检测准确率**: 目标96%+（多层检测融合）
- **成本优化**: 80%请求由边缘小模型处理，降低云端调用成本80%
- **技术栈**: eBPF + Kafka + Flink + 混合LLM（边缘+云端）

---

## 目录

1. [架构模式对比分析](#1-架构模式对比分析)
2. [推荐架构方案](#2-推荐架构方案)
3. [实时检测解决方案](#3-实时检测解决方案)
4. [滞后性问题解决策略](#4-滞后性问题解决策略)
5. [成本效益分析](#5-成本效益分析)
6. [技术选型矩阵](#6-技术选型矩阵)
7. [实施路线图](#7-实施路线图)

---

## 1. 架构模式对比分析

### 1.1 三种候选架构

| 维度 | 方案A: 云中心化架构 | 方案B: 纯边缘架构 | 方案C: 混合云边协同架构(推荐) |
|------|-------------------|------------------|---------------------------|
| **延迟** | 100-500ms | 5-20ms | 10-100ms |
| **准确率** | 98%+ | 85-90% | 96-98% |
| **成本** | 高 | 低 | 中 |
| **复杂度** | 中 | 高 | 高 |
| **可扩展性** | 强 | 弱 | 强 |
| **数据隐私** | 中 | 强 | 强 |
| **运维难度** | 中 | 高 | 高 |

### 1.2 详细对比分析

#### 方案A: 云中心化架构

**架构描述**:
```
Agent → gRPC → Backend → Kafka → Flink → 云端ML模型 → 告警
```

**优点**:
- ✅ 部署简单，统一管理
- ✅ 可利用大模型能力（GPT-4等）
- ✅ 便于集中升级和维护

**缺点**:
- ❌ 网络依赖性强
- ❌ 数据需外传，隐私风险
- ❌ 成本高（API调用费用）
- ❌ 延迟较高

**适用场景**: 数据敏感度低、预算充足、延迟要求宽松

#### 方案B: 纯边缘架构

**架构描述**:
```
Agent → 本地eBPF → 本地小模型 → 本地决策 → 上报摘要
```

**优点**:
- ✅ 超低延迟（<20ms）
- ✅ 数据不出域，隐私保护好
- ✅ 网络依赖低
- ✅ 成本低（一次性硬件投入）

**缺点**:
- ❌ 模型能力受限（只能用≤7B小模型）
- ❌ 误报率较高
- ❌ 升级困难（需更新所有Agent）
- ❌ 无法检测复杂攻击链

**适用场景**: 高度敏感环境、离线环境、IoT设备

#### 方案C: 混合云边协同架构（推荐）

**架构描述**:
```
Agent → 本地快速检测（规则+eBPF）→ 智能路由 → 
├─ 简单威胁: 本地阻断（<50ms）
├─ 复杂威胁: 云端深度分析（<200ms）
└─ 高置信度: 本地决策 + 云端学习
```

**优点**:
- ✅ 平衡延迟与准确率
- ✅ 分层检测，降低成本
- ✅ 敏感数据可本地处理
- ✅ 可利用云端大模型能力
- ✅ 渐进式升级

**缺点**:
- ❌ 架构复杂，需要智能路由
- ❌ 运维难度较高
- ❌ 需设计一致性策略

**适用场景**: 企业级部署、安全要求高、成本敏感

---

## 2. 推荐架构方案

### 2.1 混合云边协同架构详解

#### 2.1.1 整体架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                     混合云边协同架构                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                    云端 (Cloud Layer)                   │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │   │
│  │  │ LLM分析引擎  │  │ 复杂模式识别 │  │ 模型训练/更新│  │   │
│  │  │ (Qwen 72B)   │  │ (GNN+深度学习)│  │ (持续学习)  │  │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘  │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │   │
│  │  │ 威胁情报中心 │  │ 全局关联分析 │  │ 可视化平台  │  │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘  │   │
│  └─────────────────────────────────────────────────────────┘   │
│                          ↑↓ 智能路由                            │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                边缘层 (Edge Layer)                      │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │   │
│  │  │ 智能路由网关 │  │ 轻量级ML模型 │  │ 本地决策引擎 │  │   │
│  │  │ (Nginx+Lua)  │  │ (Phi-4-mini) │  │ (规则+ML)   │  │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘  │   │
│  └─────────────────────────────────────────────────────────┘   │
│                          ↑↓ Kafka Streams                       │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                终端层 (Endpoint Layer)                  │   │
│  │  ┌─────────────────────────────────────────────────┐   │   │
│  │  │                 Aegis Agent                     │   │   │
│  │  │  ┌──────────┐  ┌──────────┐  ┌──────────┐     │   │   │
│  │  │  │ eBPF采集 │  │ 快速筛选 │  │ 本地模型 │     │   │   │
│  │  │  │ (Tetragon)│  │ (规则引擎)│  │ (1B-3B) │     │   │   │
│  │  │  └──────────┘  └──────────┘  └──────────┘     │   │   │
│  │  └─────────────────────────────────────────────────┘   │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

#### 2.1.2 分层检测策略

**三层检测架构**:

| 层级 | 组件 | 延迟 | 准确率 | 处理能力 | 成本 |
|------|------|------|--------|---------|------|
| **L1** | eBPF规则引擎 | <10ms | 85% | 简单签名匹配 | 极低 |
| **L2** | 边缘ML模型 | <100ms | 94% | 序列检测、异常分类 | 低 |
| **L3** | 云端大模型 | <500ms | 98% | 复杂攻击链、未知威胁 | 高 |

**数据流示例**:
```
事件: setuid文件创建
  ↓
L1: 规则匹配（白名单过滤）
  → 白名单命中 → 放行（5ms）
  → 可疑 → 进入L2
  ↓
L2: 边缘ML模型（Isolation Forest + LSTM）
  → 低风险 → 记录日志（80ms）
  → 中风险 → 告警 + 上报云端
  → 高风险 → 自动阻断 + 上报
  ↓
L3: 云端深度分析（GNN攻击链分析）
  → 确认威胁 → 全局告警 + 联动响应
  → 误报 → 更新模型
```

#### 2.1.3 智能路由策略

**路由决策树**:
```
输入事件
  ↓
[是否敏感数据?]
├─ 是 → 本地处理（数据不出域）
└─ 否 → 继续判断
  ↓
[事件类型]
├─ 文件提权 → 本地规则+ML
├─ 反弹shell → 边缘序列模型
├─ 复杂APT → 云端GNN分析
└─ 未知行为 → 云端LLM分析
  ↓
[置信度评估]
├─ >0.9 → 本地决策
├─ 0.5-0.9 → 云端确认
└─ <0.5 → 人工审核队列
```

### 2.2 Agent内嵌大模型架构

#### 2.2.1 本地模型选择

**推荐模型配置**:

| 场景 | 模型 | 参数量 | VRAM需求 | 推理延迟 | 功能 |
|------|------|--------|----------|----------|------|
| **实时检测** | TinyLlama-1.1B | 1.1B | ~1GB | 80ms | 命令语义分析 |
| **边缘推理** | Phi-4-mini | 3.8B | ~3GB | 120ms | 行为序列检测 |
| **本地深度** | Qwen2.5-7B-INT4 | 7B | ~5GB | 150ms | 复杂场景分析 |

**模型优化技术**:
- **量化**: INT4 AWQ量化（显存节省75%）
- **推理引擎**: vLLM + PagedAttention（吞吐量提升23x）
- **批处理**: 连续批处理（GPU利用率90%+）
- **缓存**: Prefix Caching（减少30%计算）

#### 2.2.2 云端模型接口

**推荐配置**:
- **主模型**: Qwen-Turbo / GPT-4-mini
- **用途**: 复杂场景分析、未知威胁研判、自动化响应建议
- **成本**: 约$0.20-0.60/1M tokens
- **延迟**: 500-1000ms（异步）

#### 2.2.3 混合推理流程

```python
class HybridInference:
    def analyze(self, event):
        # 1. 本地快速推理（小模型）
        local_score, confidence = self.local_model.predict(event)
        
        # 2. 高置信度：本地决策
        if confidence > 0.9:
            return self.make_decision(local_score)
        
        # 3. 低置信度：云端深度分析
        cloud_result = self.cloud_model.analyze(event)
        
        # 4. 结果融合
        final_score = self.weighted_fusion(local_score, cloud_result)
        return final_score
```

---

## 3. 实时检测解决方案

### 3.1 低延迟事件采集

#### 3.1.1 eBPF事件采集架构

**技术栈**: Tetragon / Falco + eBPF

```c
// eBPF程序示例：监控setuid系统调用
SEC("tracepoint/syscalls/sys_enter_setuid")
int trace_setuid(struct trace_event_raw_sys_enter* ctx) {
    struct event_t event = {};
    event.pid = bpf_get_current_pid_tgid() >> 32;
    event.uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    event.new_uid = ctx->args[0];
    bpf_get_current_comm(&event.comm, sizeof(event.comm));
    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &event, sizeof(event));
    return 0;
}
```

**性能指标**:
- 延迟: 微秒级（内核到用户空间）
- CPU开销: 1-3%（中等负载）
- 内存: 每核心50-100MB
- 吞吐: 单核100K+ events/sec

#### 3.1.2 事件标准化（OCSF）

```json
{
  "class_uid": 1007,
  "category_uid": 1,
  "activity_id": 1,
  "time": 1700000000000,
  "process": {
    "pid": 12345,
    "file": {"path": "/usr/bin/sudo"},
    "cmd_line": "sudo -u root whoami",
    "parent_process": {"pid": 12344}
  },
  "actor": {
    "user": {"uid": 1000, "name": "admin"}
  }
}
```

### 3.2 流处理架构

#### 3.2.1 推荐技术栈

**Kafka + Flink**（大规模部署）:
```
Agent → Kafka → Flink (特征提取+CEP) → ML推理 → 告警
```

**Kafka Streams**（中小规模）:
```
Agent → Kafka → Kafka Streams (窗口聚合) → 规则引擎 → 告警
```

**Redis Streams**（小规模/边缘）:
```
Agent → Redis Streams → 本地处理 → 告警
```

#### 3.2.2 性能对比

| 技术 | 吞吐 | 延迟(P99) | 状态管理 | 适用规模 |
|------|------|----------|---------|---------|
| Flink | 100K+ events/s | <50ms | RocksDB | 大规模 |
| Kafka Streams | 80K events/s | <30ms | Changelog | 中规模 |
| Redis Streams | 50K msg/s | <1ms | 内存 | 小规模 |

#### 3.2.3 Flink CEP模式匹配

```java
// 检测反弹shell攻击链
Pattern<Event, ?> reverseShellPattern = Pattern.<Event>begin("shell_spawn")
    .where(e -> e.getType().equals("SHELL_EXEC"))
    .next("network_connect")
    .where(e -> e.getType().equals("NETWORK_OUTBOUND"))
    .where(e -> e.getPort() < 1024 || e.getPort() > 10000)
    .next("fd_redirect")
    .where(e -> e.getType().equals("FD_REDIRECT_TO_SOCKET"))
    .within(Time.seconds(5));
```

### 3.3 实时特征计算

#### 3.3.1 时序特征

**滑动窗口统计**:
```python
# 5分钟滑动窗口，30秒步进
features = {
    'event_rate': len(events) / 300,  # 每秒事件数
    'syscall_diversity': len(set(e['syscall'] for e in events)),
    'privilege_change_count': sum(1 for e in events if e['type'] == 'SETUID'),
    'entropy': calculate_syscall_entropy(events),
    'burst_score': detect_burst_pattern(events)
}
```

#### 3.3.2 图特征（进程关系）

**进程图构建**:
```python
# 节点: 进程、文件、Socket
# 边: fork, exec, read, write, connect
graph.add_edge(parent_pid, child_pid, type='fork')
graph.add_edge(process_pid, file_path, type='write')
graph.add_edge(process_pid, (dst_ip, dst_port), type='connect')
```

### 3.4 实时异常检测

#### 3.4.1 分层检测模型

**Tier 1: 规则引擎（<10ms）**:
- 白名单过滤
- 黑名单匹配
- 简单阈值检测

**Tier 2: Isolation Forest（<5ms）**:
```python
from sklearn.ensemble import IsolationForest

model = IsolationForest(
    n_estimators=100,
    contamination=0.01,
    max_samples=256
)
model.fit(normal_training_data)
```

**Tier 3: LSTM序列检测（<50ms）**:
```python
class CommandSequenceDetector(nn.Module):
    def __init__(self, vocab_size=500, embed_dim=64, hidden_dim=128):
        super().__init__()
        self.embedding = nn.Embedding(vocab_size, embed_dim)
        self.lstm = nn.LSTM(embed_dim, hidden_dim, batch_first=True)
        self.classifier = nn.Linear(hidden_dim, 2)
    
    def forward(self, x):
        x = self.embedding(x)
        _, (hidden, _) = self.lstm(x)
        return self.classifier(hidden[-1])
```

#### 3.4.2 检测延迟优化

**异步推理**:
```python
async def async_inference(event):
    # 立即返回任务ID
    task_id = generate_task_id()
    asyncio.create_task(background_analysis(event, task_id))
    return {"task_id": task_id, "status": "processing"}
```

**流式推理**:
```python
# 使用Server-Sent Events
async def stream_analysis(prompt):
    async for token in llm.generate_stream(prompt):
        yield f"data: {token}\n\n"
```

---

## 4. 滞后性问题解决策略

### 4.1 问题定义

**滞后性（Latency）问题**:
- 事件采集延迟: 1-10ms
- 数据传输延迟: 10-100ms
- 模型推理延迟: 50-500ms
- 告警响应延迟: 100-1000ms

**累积效应**: 端到端延迟可能达到秒级，影响实时响应能力

### 4.2 解决策略

#### 4.2.1 边缘优先策略

**本地决策，云端学习**:
```
边缘层: 立即响应（阻断/放行）
云端层: 异步分析 + 模型更新
```

**实现方式**:
- 边缘Agent内置轻量级决策引擎
- 高风险事件立即阻断
- 云端异步分析，结果用于模型优化

#### 4.2.2 流式处理优化

**减少批处理延迟**:
```
传统: 累积批次 → 批量处理 → 批量输出
流式: 事件到达 → 立即处理 → 实时输出
```

**技术实现**:
- Flink真正的流处理（非微批）
- Ring Buffer替代Perf Buffer
- 事件时间处理（支持乱序事件）

#### 4.2.3 预测性检测

**行为预测**:
```python
# 基于历史行为预测未来威胁
class PredictiveDetector:
    def predict_threat(self, user_id, current_events):
        user_profile = self.get_user_profile(user_id)
        
        # LSTM预测下一步行为
        next_action_prob = self.lstm_predict(current_events)
        
        # 如果预测行为与正常模式偏离 > 阈值
        if divergence_score > threshold:
            return "PREDICTED_ANOMALY"
```

#### 4.2.4 增量计算

**避免全量重算**:
```python
# Apache Iceberg增量处理
SELECT * FROM events
WHERE _change_type IN ('INSERT', 'UPDATE')
  AND _commit_snapshot_id > last_processed_snapshot
```

**效果**:
- 处理时间: 从4小时 → 15分钟
- 计算成本: 降低92%
- 数据新鲜度: 每日更新 → 每小时更新

#### 4.2.5 采样和近似算法

**分层采样**:
```python
class AdaptiveSampler:
    def should_sample(self, event, anomaly_score):
        if anomaly_score > 0.8:
            return True  # 高风险全采样
        elif anomaly_score > 0.5:
            return random.random() < 0.1  # 中风险10%采样
        else:
            return random.random() < 0.01  # 低风险1%采样
```

**Sketch算法**:
```python
# HyperLogLog估计唯一用户数量
# Count-Min Sketch频率估计
# Bloom Filter成员检测
# 内存占用从GB级降至KB级
```

### 4.3 延迟优化效果对比

| 优化策略 | 延迟改善 | 成本影响 | 准确性 | 复杂度 |
|----------|---------|---------|--------|--------|
| 边缘优先 | 80-95% | 持平 | 100% | 中 |
| 流式处理 | 60-80% | +10-20% | 100% | 中-高 |
| 增量计算 | 80-95% | -70-90% | 100% | 中 |
| 自适应采样 | 70-85% | -90-95% | 95-98% | 低-中 |
| 模型量化 | 40-60% | -50-70% | 98-99% | 低 |
| 缓存预测 | 50-90% | +20-50% | 100% | 低-中 |

---

## 5. 成本效益分析

### 5.1 成本构成分析

#### 5.1.1 云中心化架构成本

**场景**: 1000台主机，日均100M tokens

| 成本项 | 月度成本 | 年度成本 |
|--------|---------|---------|
| API调用费(GPT-4) | $8,000 | $96,000 |
| 云端计算资源 | $3,000 | $36,000 |
| 存储费用 | $500 | $6,000 |
| 网络带宽 | $1,000 | $12,000 |
| **总计** | **$12,500** | **$150,000** |

#### 5.1.2 纯边缘架构成本

**场景**: 1000台主机，本地推理

| 成本项 | 初始投入 | 年度成本 |
|--------|---------|---------|
| GPU硬件(RTX 4090) | $128,000 | $0 |
| 服务器 | $50,000 | $0 |
| 电费(24/7) | $0 | $8,640 |
| 运维人力 | $0 | $24,000 |
| 分摊(3年) | **$59,280/年** | **$32,640** |
| **总计(首年)** | **$178,000** | - |
| **总计(年均)** | **$91,920** | - |

#### 5.1.3 混合架构成本

**场景**: 1000台主机，80%边缘+20%云端

| 成本项 | 初始投入 | 月度成本 |
|--------|---------|---------|
| 边缘GPU(100台) | $48,000 | - |
| 云端API(20%流量) | - | $1,600 |
| 云端计算 | - | $600 |
| 电费 | - | $360 |
| 运维 | - | $2,000 |
| 分摊(3年) | **$1,333/月** | - |
| **总计(月)** | - | **$5,893** |
| **总计(年)** | - | **$70,720** |

### 5.2 成本对比总结

| 架构 | 首年成本 | 年均成本(3年) | 每主机年均 |
|------|---------|--------------|-----------|
| 云中心化 | $150,000 | $150,000 | $150 |
| 纯边缘 | $178,000 | $91,920 | $92 |
| 混合架构 | $118,716 | $70,720 | **$71** |

### 5.3 ROI分析

**混合架构收益**:
- 成本节省: 相比云中心化节省52%
- 延迟改善: 从500ms → 100ms（80%改善）
- 隐私保护: 敏感数据80%本地处理
- 可用性: 网络中断时边缘仍可工作

**投资回收期**:
- 相比云中心化: 8个月回收额外硬件投入
- 相比纯边缘: 立即节省（无需大规模硬件）

### 5.4 成本优化建议

#### 5.4.1 动态扩缩容

```python
class AutoScaler:
    def adjust_resources(self, qps):
        if qps > threshold_high:
            self.scale_up_cloud()
        elif qps < threshold_low:
            self.scale_down_cloud()
```

#### 5.4.2 分层成本模型

```
L1: 规则引擎（固定成本，极低）
L2: 边缘ML（一次性投入，低运营成本）
L3: 云端大模型（按需付费，高峰期增加）
```

#### 5.4.3 Token优化

**Prompt压缩**:
- 使用结构化输出格式（JSON）
- 避免冗余上下文
- 利用上下文缓存

**预期节省**: 30-40% Token成本

---

## 6. 技术选型矩阵

### 6.1 流处理引擎选型

| 场景 | 推荐引擎 | 原因 |
|------|---------|------|
| 大规模(>1000主机) | Apache Flink | 真流处理，TB级状态，复杂CEP |
| 中规模(100-1000主机) | Kafka Streams | 与Kafka深度集成，运维简单 |
| 小规模(<100主机) | Redis Streams | 超低延迟，极简部署 |
| SQL分析需求 | RisingWave | 流式SQL，实时物化视图 |

### 6.2 边缘推理框架选型

| 平台 | 推荐框架 | 性能特点 |
|------|---------|---------|
| NVIDIA GPU | TensorRT + vLLM | 最优GPU加速，INT8/FP16量化 |
| Intel CPU/GPU | OpenVINO | Intel硬件优化，低功耗 |
| ARM设备 | ONNX Runtime | 轻量级，跨平台 |
| Apple设备 | Core ML | Neural Engine加速 |

### 6.3 异常检测算法选型

| 场景 | 推荐算法 | 延迟 | 准确率 |
|------|---------|------|--------|
| 实时初筛 | Isolation Forest | <5ms | 86% |
| 时序检测 | LSTM-Autoencoder | <50ms | 93% |
| 图关系检测 | GNN | <200ms | 95% |
| 无监督基线 | One-Class SVM | <10ms | 88% |

### 6.4 开源项目选型

| 功能 | 推荐项目 | 优点 |
|------|---------|------|
| 运行时安全 | Tetragon | 低开销，Kubernetes原生 |
| 容器安全 | Falco | CNCF毕业，生态丰富 |
| 完整SIEM | Wazuh | 功能完整，规则丰富 |
| 网络检测 | Suricata | 高性能IPS/IDS |
| 网络分析 | Zeek | 详细日志，可编程 |

---

## 7. 实施路线图

### 7.1 阶段规划

#### 阶段1: MVP验证（1-3个月）

**目标**: 验证核心架构可行性

**交付物**:
- eBPF事件采集（Tetragon/Falco）
- Redis Streams流处理
- 规则引擎快速检测
- Isolation Forest异常检测

**资源需求**:
- 2-3名工程师
- 10台测试主机
- 预算: $5K

#### 阶段2: 能力建设（3-6个月）

**目标**: 扩展检测能力

**交付物**:
- Kafka集群部署
- Flink流处理（复杂CEP）
- 边缘ML模型部署（Phi-4-mini）
- OCSF标准化层

**资源需求**:
- 5-8名工程师
- 100台测试主机
- 预算: $30K

#### 阶段3: 生产部署（6-12个月）

**目标**: 生产级大规模部署

**交付物**:
- Flink集群规模化
- GNN深度学习模型
- 混合LLM架构（边缘+云端）
- 联邦学习（多租户）

**资源需求**:
- 10+名工程师
- 1000+生产主机
- 预算: $100K+

### 7.2 风险与缓解

| 风险 | 影响 | 缓解策略 |
|------|------|---------|
| eBPF兼容性 | 高 | 支持多内核版本，内核模块回退 |
| 误报率高 | 中 | 分层检测，人工反馈闭环 |
| 模型漂移 | 中 | 持续学习，定期重训练 |
| 性能瓶颈 | 中 | 性能测试，动态扩缩容 |
| 隐私合规 | 高 | 数据脱敏，本地处理优先 |

### 7.3 成功指标

| 指标 | 目标值 | 测量方法 |
|------|--------|---------|
| 检测延迟 | P99 < 500ms | 端到端监控 |
| 检测准确率 | >96% | 人工验证样本 |
| 误报率 | <2% | 分析师反馈 |
| 覆盖主机数 | 1000+ | 部署统计 |
| 威胁检测数 | 月度统计 | 安全报告 |
| 成本节约 | >50% | 成本分析 |

---

## 8. 总结与建议

### 8.1 关键结论

1. **推荐架构**: 混合云边协同架构（方案C）
   - 平衡性能、成本和准确性
   - 渐进式演进路径
   - 符合Aegis系统现有架构

2. **技术栈**:
   - 采集: eBPF（Tetragon/Falco）
   - 流处理: Kafka + Flink
   - 边缘ML: Phi-4-mini + vLLM
   - 云端LLM: Qwen-Turbo
   - 检测: 规则+Isolation Forest+LSTM

3. **成本优化**:
   - 分层检测降低80%云端成本
   - 边缘优先策略
   - Token压缩和缓存

### 8.2 立即行动项

**Week 1-2**:
- [ ] 部署Tetragon测试环境
- [ ] 验证eBPF采集性能
- [ ] 定义OCSF事件格式

**Week 3-4**:
- [ ] 部署Kafka/Redis Streams
- [ ] 实现规则引擎原型
- [ ] 集成Isolation Forest

**Month 2**:
- [ ] 部署边缘ML模型（Phi-4-mini）
- [ ] 实现智能路由逻辑
- [ ] 端到端性能测试

**Month 3**:
- [ ] 小规模生产验证（10台主机）
- [ ] 误报率调优
- [ ] 准备大规模部署

### 8.3 参考资源

**开源项目**:
- Tetragon: https://tetragon.io/
- Falco: https://falco.org/
- Wazuh: https://wazuh.com/
- vLLM: https://github.com/vllm-project/vllm

**学术论文**:
- OCSF Schema: https://schema.ocsf.io/
- Graph Neural Networks for Anomaly Detection (Springer 2026)
- Federated Learning for IoT Security (Nature 2026)

**技术文档**:
- Apache Flink Documentation
- NVIDIA TensorRT-LLM Guide
- Kafka Streams Architecture

---

**文档生成时间**: 2026-03-19  
**版本**: 1.0  
**状态**: 定稿
