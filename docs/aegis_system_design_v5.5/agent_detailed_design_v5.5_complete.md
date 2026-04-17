# Aegis智能主机安全系统 V5.5 Agent详细设计文档

**版本**: 5.5
**日期**: 2026-03-30
**状态**: 定稿

---

## 1. Agent概述

### 1.1 Agent定位

Agent是部署在目标主机上的轻量级安全代理程序，负责实时采集系统事件并进行本地智能处理。V5.5版本在保持1C1G资源限制的前提下，实现了本地轻量级智能分析和决策能力。

### 1.2 设计目标

| 目标 | 要求 |
|------|------|
| 资源占用 | CPU < 1核, 内存 < 1GB |
| 事件采集延迟 | < 100ms |
| 紧急事件响应 | < 500ms (本地阻断) |
| 网络带宽降低 | > 90% (通过特征压缩) |

### 1.3 核心能力

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Agent架构                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      Event Collection Layer                         │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                  │   │
│  │  │  eBPF       │  │  Process    │  │  Network    │                  │   │
│  │  │  Collector  │→ │  Monitor    │→ │  Monitor    │                  │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                  │   │
│  └────────────────────────────────────┬────────────────────────────────────┘ │
│                                       │                                      │
│                                       ↓                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Local Intelligence Layer                         │   │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────┐ │ │
│  │  │  SlidingWindow  │  │  Priority       │  │  Feature                │ │ │
│  │  │  Stats          │→ │  Engine         │→ │  Extractor              │ │ │
│  │  │  (统计异常检测)  │  │  (优先级排序)    │  │  (特征提取)             │ │ │
│  │  └─────────────────┘  └─────────────────┘  └─────────────────────────┘ │ │
│  │            │                    │                    │                  │ │
│  │            └────────────────────┼────────────────────┘                  │ │
│  │                                 ↓                                        │ │
│  │                    ┌─────────────────────────┐                         │ │
│  │                    │   Decision Engine       │                         │ │
│  │                    │   (本地决策与阻断)       │                         │ │
│  │                    └─────────────────────────┘                         │ │
│  └────────────────────────────────────┬────────────────────────────────────┘ │
│                                       │                                      │
│                                       ↓                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Smart Communication Layer                        │   │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────┐ │ │
│  │  │  Event          │  │  Batch          │  │  Compression            │ │ │
│  │  │  Classifier     │→ │  Aggregator     │→ │  Encoder                │ │ │
│  │  │  (事件分级)      │  │  (批量聚合)      │  │  (压缩编码)             │ │ │
│  │  └─────────────────┘  └─────────────────┘  └─────────────────────────┘ │ │
│  └────────────────────────────────────┬────────────────────────────────────┘ │
│                                       │                                      │
│                                       ↓                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                           gRPC Client                               │   │
│  │                    连接到 Backend Agent Hub                        │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. 模块详细设计

### 2.1 eBPF事件采集模块

#### 2.1.1 功能描述

基于eBPF技术实时采集系统进程执行事件，是Agent的核心数据来源。

#### 2.1.2 支持的事件类型

| 事件类型 | 说明 | MITRE ID |
|----------|------|----------|
| execve | 进程执行 | T1059 |
| connect | 网络连接 | T1071 |
| open file | 文件访问 | T1083 |
| fork/clone | 进程创建 | T1053 |

#### 2.1.3 核心实现

```go
// internal/ebpf/collector.go
type Collector struct {
    hostID          string
    eventBufferSize int
    ringBuffer      *RingBuffer
    eventChan       chan *RuntimeEvent
}

type RuntimeEvent struct {
    EventID     string    `json:"event_id"`
    HostID      string    `json:"host_id"`
    EventType   string    `json:"event_type"`     // execve, connect, open, fork
    Timestamp   int64     `json:"timestamp"`
    PID         uint32    `json:"pid"`
    PPID        uint32    `json:"ppid"`
    UID         uint32    `json:"uid"`
    ProcessName string    `json:"process_name"`
    CommandLine string    `json:"command_line"`
    ParentName  string    `json:"parent_name"`
    WorkingDir  string    `json:"working_dir"`
    MitreID     string    `json:"mitre_id"`       // 关联的MITRE ID
}

func (c *Collector) Start() error {
    // 初始化eBPF maps
    // 设置ring buffer
    // 启动事件采集
    return nil
}

func (c *Collector) Stop() error {
    // 停止采集
    // 清理资源
    return nil
}
```

---

### 2.2 本地智能模块 (Local Intelligence)

这是V5.5新增的核心模块，负责在Agent端进行轻量级智能处理。

#### 2.2.1 SlidingWindowStats - 滑动窗口统计

**功能**: 检测进程执行的频率异常

```go
// internal/intelligence/window_stats.go
type SlidingWindowStats struct {
    windowSize   time.Duration    // 窗口大小: 5秒/10秒/30秒
    maxEvents    int              // 最大事件数阈值
    counters     map[string]*WindowCounter
    mu           sync.RWMutex
}

type WindowCounter struct {
    events    []time.Time  // 事件时间戳列表
    threshold int          // 阈值
}

// 阈值配置
const (
    MaxNormalForkRate     = 10  // 10次/5秒fork
    MaxNormalExecRate     = 50  // 50次/5秒exec  
    MaxNormalNetworkRate  = 20  // 20次/5秒网络调用
    MaxNormalFileRate     = 30  // 30次/5秒文件操作
)

func NewSlidingWindowStats(windowSize time.Duration, threshold int) *SlidingWindowStats {
    return &SlidingWindowStats{
        windowSize: windowSize,
        maxEvents:  threshold,
        counters:   make(map[string]*WindowCounter),
    }
}

// Record 记录事件
func (s *SlidingWindowStats) Record(key string) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    counter, ok := s.counters[key]
    if !ok {
        counter = &WindowCounter{threshold: s.maxEvents}
        s.counters[key] = counter
    }
    counter.events = append(counter.events, time.Now())
}

// GetCount 获取窗口内事件数
func (s *SlidingWindowStats) GetCount(key string) int {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    if counter, ok := s.counters[key]; ok {
        now := time.Now()
        var valid int
        for _, t := range counter.events {
            if now.Sub(t) < s.windowSize {
                valid++
            }
        }
        return valid
    }
    return 0
}

// IsAnomalous 检查是否异常
func (s *SlidingWindowStats) IsAnomalous(key string) bool {
    return s.GetCount(key) >= s.maxEvents
}
```

#### 2.2.2 PriorityEngine - 优先级引擎

**功能**: 根据规则匹配确定事件优先级

```go
// internal/intelligence/priority_engine.go
type PriorityLevel int

const (
    PriorityCRITICAL PriorityLevel = iota  // 立即阻断
    PriorityHIGH                            // 实时上报
    PriorityMEDIUM                          // 批量上报
    PriorityLOW                             // 忽略
)

type PriorityRule struct {
    Name     string
    Match    func(*RuntimeEvent) bool
    Level    PriorityLevel
    Score    int        // 优先级得分
}

type PriorityEngine struct {
    rules []PriorityRule
}

func NewPriorityEngine() *PriorityEngine {
    return &PriorityEngine{
        rules: []PriorityRule{
            {
                Name:  "mitre_t1059", // 命令行解释器
                Match: func(e *RuntimeEvent) bool { return e.MitreID == "T1059" },
                Level: PriorityCRITICAL,
                Score: 100,
            },
            {
                Name:  "mitre_t1053", // 计划任务
                Match: func(e *RuntimeEvent) bool { return e.MitreID == "T1053" },
                Level: PriorityHIGH,
                Score: 80,
            },
            {
                Name:  "root_exec",
                Match: func(e *RuntimeEvent) bool { return e.UID == 0 && e.ProcessName == "/bin/bash" },
                Level: PriorityHIGH,
                Score: 70,
            },
            {
                Name:  "network_c2",
                Match: func(e *RuntimeEvent) bool { return e.HasNetworkAccess && e.IsExternalConnection },
                Level: PriorityHIGH,
                Score: 85,
            },
            {
                Name:  "system_process",
                Match: func(e *RuntimeEvent) bool { return e.UID < 1000 },
                Level: PriorityLOW,
                Score: 10,
            },
        },
    }
}

// Evaluate 评估事件优先级
func (pe *PriorityEngine) Evaluate(event *RuntimeEvent) PriorityResult {
    result := PriorityResult{Level: PriorityMEDIUM, Score: 0}
    
    for _, rule := range pe.rules {
        if rule.Match(event) {
            if rule.Level < result.Level {
                result.Level = rule.Level
            }
            if rule.Score > result.Score {
                result.Score = rule.Score
                result.RuleName = rule.Name
            }
        }
    }
    
    return result
}

type PriorityResult struct {
    Level    PriorityLevel
    Score    int
    RuleName string
}
```

#### 2.2.3 DecisionEngine - 本地决策引擎

**功能**: 基于规则和白名单进行本地决策，支持本地阻断

```go
// internal/intelligence/decision_engine.go
type DecisionAction int

const (
    DecisionBLOCK DecisionAction = iota  // 本地阻断
    DecisionREPORT_HIGH                   // 高优先级上报
    DecisionREPORT_FEATURE                // 特征上报(批量)
    DecisionSKIP                          // 忽略
)

type Decision struct {
    Action         DecisionAction
    Priority       PriorityLevel
    ShouldBlock    bool
    ShouldNotify   bool
    FeatureData    []byte
}

type DecisionEngine struct {
    blockPolicyPath string
    rules           []BlockRule
    whiteList       map[string]bool
}

type BlockRule struct {
    MitreID    string  // MITRE ID
    ExecMatch  string  // 进程名匹配
    Action     string  // block/report
    Enabled    bool    // 是否启用
}

func NewDecisionEngine() *DecisionEngine {
    de := &DecisionEngine{
        whiteList: make(map[string]bool),
    }
    de.loadWhiteList()
    de.loadBlockRules()
    return de
}

// 加载白名单进程
func (de *DecisionEngine) loadWhiteList() {
    systemProcesses := []string{
        "systemd", "sshd", "dockerd", "containerd",
        "kubelet", "etcd", "kube-proxy", "kube-scheduler",
        "systemd-journal", "systemd-logind",
    }
    for _, p := range systemProcesses {
        de.whiteList[p] = true
    }
}

// 加载阻断规则
func (de *DecisionEngine) loadBlockRules() {
    de.rules = []BlockRule{
        {MitreID: "T1059", Action: "block", Enabled: true},   // bash
        {MitreID: "T1053", Action: "block", Enabled: false},  // at
        {MitreID: "T1021", Action: "block", Enabled: false},  // ssh
        {MitreID: "T1219", Action: "block", Enabled: true},   // 远程管理工具
    }
}

// Decide 决策入口
func (de *DecisionEngine) Decide(event *RuntimeEvent) Decision {
    // 1. 检查白名单
    if de.isWhiteListed(event) {
        return Decision{
            Action:   DecisionSKIP,
            Priority: PriorityLOW,
        }
    }
    
    // 2. 检查阻断规则
    if blockRule := de.checkBlockRule(event); blockRule != nil && blockRule.Enabled {
        return Decision{
            Action:        DecisionBLOCK,
            Priority:      PriorityCRITICAL,
            ShouldBlock:   true,
            ShouldNotify:  true,
        }
    }
    
    // 3. 检查统计异常
    if de.windowStats.IsAnomalous(event.ProcessName) {
        return Decision{
            Action:        DecisionREPORT_HIGH,
            Priority:      PriorityHIGH,
            ShouldNotify:  true,
        }
    }
    
    // 4. 普通事件 - 提取特征后批量上报
    return Decision{
        Action:        DecisionREPORT_FEATURE,
        Priority:      PriorityMEDIUM,
    }
}

func (de *DecisionEngine) isWhiteListed(event *RuntimeEvent) bool {
    return de.whiteList[event.ProcessName]
}

func (de *DecisionEngine) checkBlockRule(event *RuntimeEvent) *BlockRule {
    for _, rule := range de.rules {
        if rule.MitreID == event.MitreID {
            return &rule
        }
    }
    return nil
}
```

#### 2.2.4 FeatureExtractor - 特征提取器

**功能**: 将原始事件压缩为特征数据，大幅降低网络传输量

```go
// internal/intelligence/feature_extractor.go
type EventFeatures struct {
    // 基础特征 (32 bytes)
    EventType   string    `json:"event_type"`
    ProcessHash string    `json:"process_hash"`   // 哈希后的进程名
    UID         uint32    `json:"uid"`
    
    // 统计特征 (16 bytes)
    FrequencyScore   float64 `json:"frequency_score"`
    ProcessTreeScore float64 `json:"process_tree_score"`
    
    // 上下文特征 (32 bytes)
    TimeOfDay        int     `json:"time_of_day"`        // 0-23小时
    IsSystemProcess  bool    `json:"is_system_process"`
    HasNetworkAccess bool    `json:"has_network_access"`
    
    // 原始数据指纹 (32 bytes)
    CommandHash string `json:"command_hash"`
}

type FeatureExtractor struct {
    processNameMap map[string]string
}

func NewFeatureExtractor() *FeatureExtractor {
    return &FeatureExtractor{
        processNameMap: make(map[string]string),
    }
}

// Extract 提取特征
func (fe *FeatureExtractor) Extract(event *RuntimeEvent) EventFeatures {
    return EventFeatures{
        EventType:        event.EventType,
        ProcessHash:      fe.hashString(event.ProcessName),
        UID:              event.UID,
        FrequencyScore:   fe.calcFrequency(event.ProcessName),
        TimeOfDay:        int(event.Timestamp % 86400 / 3600),
        IsSystemProcess:  event.UID < 1000,
        CommandHash:      fe.hashString(event.CommandLine),
    }
}

func (fe *FeatureExtractor) hashString(s string) string {
    h := sha256.Sum256([]byte(s))
    return hex.EncodeToString(h[:8])  // 只取前8字节，16字符
}

func (fe *FeatureExtractor) calcFrequency(processName string) float64 {
    // 简化: 基于滑动窗口计算
    count := globalStats.GetCount(processName)
    return float64(count) / 100.0
}

// Serialize 序列化为紧凑字节数组 (~100 bytes vs 原始2KB)
func (ef *EventFeatures) Serialize() []byte {
    // 使用msgpack或自定义二进制格式
    data, _ := json.Marshal(ef)
    return data
}
```

---

### 2.3 智能通信模块 (Smart Communicator)

#### 2.3.1 EventClassifier - 事件分级器

```go
// internal/intelligence/event_classifier.go
type EventClassifier struct{}

func (ec *EventClassifier) Classify(event *RuntimeEvent) PriorityLevel {
    // 优先级判断逻辑
    switch {
    case event.MitreID == "T1059" && event.UID == 0:  // root执行bash
        return PriorityCRITICAL
    case event.ProcessName == "/bin/bash" && event.UID == 0:
        return PriorityHIGH
    case ec.isSystemCritical(event):
        return PriorityHIGH
    default:
        return PriorityMEDIUM
    }
}

func (ec *EventClassifier) isSystemCritical(event *RuntimeEvent) bool {
    dangerousMitreIDs := map[string]bool{
        "T1059": true,  // 命令行
        "T1053": true,  // 计划任务
        "T1021": true,  // 远程服务
        "T1083": true,  // 文件探测
        "T1071": true,  // 网络外联
    }
    return dangerousMitreIDs[event.MitreID]
}
```

#### 2.3.2 BatchAggregator - 批量聚合器

```go
// internal/intelligence/batch_aggregator.go
type BatchAggregator struct {
    batchSize    int              // 批次大小
    batchTimeout time.Duration    // 批次超时
    buffer       []EventFeatures
    mu           sync.Mutex
}

func NewBatchAggregator(batchSize int, timeout time.Duration) *BatchAggregator {
    ba := &BatchAggregator{
        batchSize:    batchSize,
        batchTimeout: timeout,
        buffer:       make([]EventFeatures, 0, batchSize),
    }
    go ba.flushLoop()
    return ba
}

func (ba *BatchAggregator) Add(feature EventFeatures) {
    ba.mu.Lock()
    defer ba.mu.Unlock()
    
    ba.buffer = append(ba.buffer, feature)
    
    if len(ba.buffer) >= ba.batchSize {
        ba.flush()
    }
}

func (ba *BatchAggregator) flush() {
    // 发送到gRPC通道或积累更多后统一发送
    ba.buffer = ba.buffer[:0]
}

func (ba *BatchAggregator) flushLoop() {
    ticker := time.NewTicker(ba.batchTimeout)
    for range ticker.C {
        ba.mu.Lock()
        if len(ba.buffer) > 0 {
            ba.flush()
        }
        ba.mu.Unlock()
    }
}
```

---

### 2.4 本地阻断模块 (Blocker)

#### 2.4.1 功能描述

对于匹配阻断规则的高危事件，Agent可以在后端响应之前执行本地阻断。

#### 2.4.2 阻断方法

| 方法 | 说明 | 适用场景 |
|------|------|----------|
| kill_process | 终止进程 | 已确认恶意进程 |
| kill_parent | 终止父进程 | 恶意fork链 |
| quarantine | 隔离文件 | 可疑文件 |

```go
// internal/blocker/blocker.go
type Blocker struct {
    quarantineDir string
}

func (b *Blocker) Block(event *RuntimeEvent) error {
    // 根据事件类型选择阻断方法
    switch event.EventType {
    case "execve":
        return b.killProcess(event.PID)
    case "fork":
        return b.killParent(event.PPID)
    default:
        return b.killProcess(event.PID)
    }
}

func (b *Blocker) killProcess(pid uint32) error {
    return syscall.Kill(syscall.Pid(pid), syscall.SIGKILL)
}

func (b *Blocker) killParent(ppid uint32) error {
    return syscall.Kill(syscall.Pid(ppid), syscall.SIGKILL)
}
```

---

## 3. 数据流设计

### 3.1 事件处理流

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Agent数据流                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. eBPF采集                                                                  │
│     ┌────────────────────────────────────────────────────────────────────┐  │
│     │ execve事件 → RuntimeEvent{event_id, pid, command_line, mitre_id}   │  │
│     └────────────────────────────────────────────────────────────────────┘  │
│                                    ↓                                         │
│  2. 本地智能处理                                                              │
│     ┌────────────────────────────────────────────────────────────────────┐  │
│     │ DecisionEngine.Decide(event):                                     │  │
│     │   ├─ 白名单检查 → SKIP                                            │  │
│     │   ├─ 阻断规则匹配 → BLOCK + 本地kill                              │  │
│     │   ├─ 统计异常检测 → REPORT_HIGH                                   │  │
│     │   └─ 普通事件 → REPORT_FEATURE                                    │  │
│     └────────────────────────────────────────────────────────────────────┘  │
│                                    ↓                                         │
│  3. 智能通信                                                                  │
│     ┌────────────────────────────────────────────────────────────────────┐  │
│     │ 根据优先级选择发送策略:                                           │  │
│     │   - CRITICAL: stream.Send() 立即发送                             │  │
│     │   - HIGH: 1秒内发送                                               │  │
│     │   - MEDIUM: BatchAggregator累积后发送                           │  │
│     │   - LOW: 不发送                                                   │  │
│     └────────────────────────────────────────────────────────────────────┘  │
│                                    ↓                                         │
│  4. gRPC发送                                                                  │
│     ┌────────────────────────────────────────────────────────────────────┐  │
│     │ stream.Send(RuntimeEvent / EventFeatures)                        │  │
│     └────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
                                       ↓ gRPC stream
┌─────────────────────────────────────────────────────────────────────────────┐
│                            Backend Agent Hub                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  - 接收事件并转发到Kafka                                                     │
│  - 通过gRPC下发命令和阻断策略                                               │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 4. 配置文件设计

### 4.1 Agent配置文件

```toml
# /etc/aegis-agent/config.toml

[server]
# Backend地址
server_addr = "192.168.1.100:19090"
auth_token = "a_very_secret_token"

[agent]
host_id = ""

[collection]
# eBPF采集配置
event_types = ["execve", "connect", "open", "fork"]
buffer_size = 10000

[intelligence]
# 本地智能配置
enable_local_intelligence = true

# 滑动窗口阈值 (事件数/秒)
fork_threshold = 10
exec_threshold = 50
network_threshold = 20
file_threshold = 30

# 窗口大小 (秒)
window_size = 5

# 特征压缩
enable_feature_extraction = true

[blocker]
# 阻断配置
quarantine_dir = "/opt/aegis-agent/quarantine"
auto_block = true
block_methods = ["kill_process"]

[communication]
# 通信配置
batch_size = 100
batch_timeout = "5s"
compression = true
```

---

## 5. 资源使用

### 5.1 资源占用估算

| 模块 | CPU | 内存 | 说明 |
|------|-----|------|------|
| eBPF采集 | 0.3核 | 30MB | 事件捕获 |
| 本地智能 | 0.2核 | 50MB | 规则匹配+统计 |
| gRPC通信 | 0.1核 | 20MB | 网络IO |
| **总计** | **~0.6核** | **~100MB** | 预留40%余量 |

### 5.2 性能指标

| 指标 | V5.2 | V5.5 | 改善 |
|------|------|------|------|
| 事件采集延迟 | <100ms | <50ms | ↑50% |
| 紧急事件响应 | 3-5秒 | <0.5秒 | ↑90% |
| 网络带宽(原始) | 100条/秒 | - | - |
| 网络带宽(特征) | - | 10条/秒 | ↓90% |
| 内存占用 | 200MB | 100MB | ↓50% |

---

## 6. 目录结构

```
/agent
├── cmd/agent/
│   └── main.go                    # Agent入口
│
├── internal/
│   ├── ebpf/                      # eBPF采集模块
│   │   ├── collector.go           # 采集器
│   │   ├── pipeline.go            # 事件处理管道
│   │   ├── events.go              # 事件定义
│   │   └── loader.go              # eBPF程序加载
│
│   ├── intelligence/              # 本地智能模块 (V5.5新增)
│   │   ├── window_stats.go        # 滑动窗口统计
│   │   ├── anomaly_detector.go    # 异常检测器
│   │   ├── priority_engine.go     # 优先级引擎
│   │   ├── decision_engine.go     # 决策引擎
│   │   ├── feature_extractor.go   # 特征提取器
│   │   ├── smart_communicator.go  # 智能通信器
│   │   └── batch_aggregator.go    # 批量聚合器
│
│   ├── blocker/                   # 阻断模块
│   │   ├── blocker.go             # 阻断器
│   │   ├── process.go             # 进程阻断
│   │   ├── network.go             # 网络阻断
│   │   └── file.go                # 文件隔离
│
│   ├── client/                    # gRPC客户端
│   │   └── client.go              # 客户端实现
│
│   ├── executor/                  # 命令执行模块
│   │   └── executor.go
│
│   ├── asset/                     # 资产信息收集
│   │   └── collector.go
│
│   ├── sigma/                     # Sigma规则
│   │   ├── loader.go              # 规则加载
│   │   ├── parser.go              # 规则解析
│   │   └── matcher.go             # 规则匹配
│
│   ├── config/                    # 配置模块
│   │   └── config.go
│   │
│   └── logger/                    # 日志模块
│       └── logger.go
│
├── pkg/api/v1/                    # Protobuf生成代码
│   ├── agent_comm.pb.go
│   └── agent_comm_grpc.pb.go
│
├── Makefile
└── build.sh
```

---

**文档结束**