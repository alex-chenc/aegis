# V5.0 运行时检测系统实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** 完成 Aegis V5.0 运行时检测闭环：Kafka 事件队列 → 2分钟主机窗口聚合 → LLM 分析 → 告警/阻断 → WebSocket 推送

**Architecture:** 基于 Kafka 的事件驱动架构，Agent 上报 Sigma 初筛事件，Backend 进行窗口聚合和 LLM 深度分析，结果推送前端并可自动/手动阻断

**Tech Stack:** Go, segmentio/kafka-go, gorilla/websocket, Gin, gRPC, PostgreSQL, Redis

---

## 当前状态

### ✅ 已实现
- 数据库 Schema (migrations/005_v5_detection_tables.sql)
- Models: Alert, SigmaRule, BlockPolicy, BlockRecord, ToolCall
- Repositories: AlertRepository, BlockRepository, BlockPolicyRepository, SigmaRuleRepository, ToolCallRepository
- Services: AlertService (去重逻辑), SigmaRuleService (生命周期)
- Handlers: DetectionHandler (REST API)
- Routes: /api/v1/detection/*

### ❌ 待实现
1. Kafka 基础设施 (docker-compose, config, dependencies)
2. Kafka 队列层 (kafka_consumer.go, kafka_producer.go)
3. 管道组件 (host_window_aggregator.go, llm_prompt_builder.go, llm_response_parser.go)
4. 核心服务 (runtime_pipeline_service.go, llm_analysis_service.go, block_service.go, rule_service.go)
5. WebSocket (websocket_service.go, websocket_handler.go)
6. 规则加载器 (rule_loader.go)

---

## 技术选型

| 组件 | 选型 | 理由 |
|------|------|------|
| Kafka 客户端 | segmentio/kafka-go | 轻量、纯 Go、无 CGO |
| WebSocket | gorilla/websocket | 成熟、稳定、标准 |
| 窗口时长 | 2 分钟 | 设计规范 |
| 最大工具调用 | 10 次 | 设计规范 |
| 分区策略 | 按 host_id | 保证单主机事件顺序 |
| 去重键 | host_id:pid:mitre_id | 符合现有 AlertService 模式 |

---

## Kafka Topics 设计

| Topic | 用途 | 分区策略 |
|-------|------|----------|
| raw-events | Agent 上报事件 | 按 host_id |
| analysis-results | LLM 分析结果 | 审计用途 |
| block-commands | 阻断指令 | 按 host_id |
| rule-updates | 规则下发 | 广播 |
| tool-calls | 工具调用日志 | 审计 |

---

## LLM 输入/输出格式

### 输入 (2分钟窗口聚合事件)
```json
{
  "host_id": "host-001",
  "window_start": "2026-03-20T10:00:00Z",
  "window_end": "2026-03-20T10:02:00Z",
  "events": [
    {
      "event_type": "process_exec",
      "pid": 12345,
      "command_line": "/bin/bash -i >& /dev/tcp/...",
      "matched_rule_id": "reverse_shell_t1059_004",
      "mitre_id": "T1059.004",
      "severity": "critical",
      "timestamp": "2026-03-20T10:00:15Z"
    }
  ]
}
```

### 输出 (告警 + 工具调用 + 规则调整)
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

---

## 实现任务

### Task 1: Kafka 基础设施

**Files:**
- Modify: `docker-compose.yml` (添加 Kafka + Zookeeper 服务)
- Modify: `backend/config/config.go` (添加 KafkaConfig 结构体)
- Modify: `backend/config/config.yaml` (添加 Kafka 配置)
- Modify: `backend/go.mod` (添加 segmentio/kafka-go 和 gorilla/websocket 依赖)

**Step 1: 添加 Kafka 和 Zookeeper 到 docker-compose.yml**

在 services 部分添加：
```yaml
# Kafka 依赖的 Zookeeper
zookeeper:
  image: confluentinc/cp-zookeeper:7.5.0
  container_name: aegis-zookeeper
  restart: unless-stopped
  environment:
    ZOOKEEPER_CLIENT_PORT: 2181
    ZOOKEEPER_TICK_TIME: 2000
  ports:
    - "2181:2181"
  networks:
    - aegis-network
  healthcheck:
    test: ["CMD", "echo", "ruok", "|", "nc", "localhost", "2181"]
    interval: 10s
    timeout: 5s
    retries: 5

# Kafka 消息队列
kafka:
  image: confluentinc/cp-kafka:7.5.0
  container_name: aegis-kafka
  restart: unless-stopped
  depends_on:
    zookeeper:
      condition: service_started
  environment:
    KAFKA_BROKER_ID: 1
    KAFKA_ZOOKEEPER_CONNECT: zookeeper:2181
    KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://kafka:9092,PLAINTEXT_HOST://localhost:29092
    KAFKA_LISTENER_SECURITY_PROTOCOL_MAP: PLAINTEXT:PLAINTEXT,PLAINTEXT_HOST:PLAINTEXT
    KAFKA_INTER_BROKER_LISTENER_NAME: PLAINTEXT
    KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
    KAFKA_AUTO_CREATE_TOPICS_ENABLE: "true"
  ports:
    - "29092:9092"
  networks:
    - aegis-network
  healthcheck:
    test: ["CMD", "kafka-topics", "--bootstrap-server", "localhost:9092", "--list"]
    interval: 15s
    timeout: 10s
    retries: 5
    start_period: 30s
```

**Step 2: 添加 KafkaConfig 到 config.go**

```go
type KafkaConfig struct {
    Brokers []string `mapstructure:"brokers"`
    GroupID string   `mapstructure:"group_id"`
}

// 在 Config 结构体中添加
type Config struct {
    // ... existing fields
    Kafka KafkaConfig `mapstructure:"kafka"`
}
```

**Step 3: 添加 Kafka 配置到 config.yaml**

```yaml
kafka:
  brokers:
    - kafka:9092
  group_id: aegis-backend-consumer
```

**Step 4: 更新 go.mod**

```bash
cd backend && go get github.com/segmentio/kafka-go github.com/gorilla/websocket
```

**Verification:**
- docker compose up -d zookeeper kafka
- docker compose ps (确认 kafka healthy)
- go mod tidy (确认依赖)

---

### Task 2: Kafka Producer

**Files:**
- Create: `backend/internal/queue/kafka_producer.go`

**Implementation:**
```go
package queue

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/segmentio/kafka-go"
    "go.uber.org/zap"
)

type KafkaProducer struct {
    writers map[string]*kafka.Writer
    logger  *zap.Logger
}

func NewKafkaProducer(brokers []string, logger *zap.Logger) *KafkaProducer {
    topics := []string{"raw-events", "analysis-results", "block-commands", "rule-updates", "tool-calls"}
    writers := make(map[string]*kafka.Writer)
    
    for _, topic := range topics {
        writers[topic] = &kafka.Writer{
            Addr:     kafka.TCP(brokers...),
            Topic:    topic,
            Balancer: &kafka.Hash{}, // 按 key 分区
        }
    }
    
    return &KafkaProducer{writers: writers, logger: logger}
}

func (p *KafkaProducer) SendMessage(ctx context.Context, topic, key string, value interface{}) error {
    writer, ok := p.writers[topic]
    if !ok {
        return fmt.Errorf("unknown topic: %s", topic)
    }
    
    data, err := json.Marshal(value)
    if err != nil {
        return fmt.Errorf("failed to marshal message: %w", err)
    }
    
    err = writer.WriteMessages(ctx, kafka.Message{
        Key:   []byte(key),
        Value: data,
    })
    if err != nil {
        p.logger.Error("failed to write message", zap.String("topic", topic), zap.Error(err))
        return err
    }
    
    return nil
}

func (p *KafkaProducer) Close() error {
    for topic, writer := range p.writers {
        if err := writer.Close(); err != nil {
            p.logger.Error("failed to close writer", zap.String("topic", topic), zap.Error(err))
        }
    }
    return nil
}
```

**Verification:**
- go build ./internal/queue/
- 写单元测试验证消息发送

---

### Task 3: Kafka Consumer

**Files:**
- Create: `backend/internal/queue/kafka_consumer.go`

**Implementation:**
```go
package queue

import (
    "context"
    "encoding/json"

    "github.com/segmentio/kafka-go"
    "go.uber.org/zap"
)

type MessageHandler func(ctx context.Context, key, value []byte) error

type KafkaConsumer struct {
    reader  *kafka.Reader
    handler MessageHandler
    logger  *zap.Logger
}

func NewKafkaConsumer(brokers []string, topic, groupID string, handler MessageHandler, logger *zap.Logger) *KafkaConsumer {
    reader := kafka.NewReader(kafka.ReaderConfig{
        Brokers:  brokers,
        Topic:    topic,
        GroupID:  groupID,
        MinBytes: 10e3, // 10KB
        MaxBytes: 10e6, // 10MB
    })
    
    return &KafkaConsumer{reader: reader, handler: handler, logger: logger}
}

func (c *KafkaConsumer) Start(ctx context.Context) error {
    c.logger.Info("starting consumer", zap.String("topic", c.reader.Config().Topic))
    
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            msg, err := c.reader.ReadMessage(ctx)
            if err != nil {
                if ctx.Err() != nil {
                    return nil // graceful shutdown
                }
                c.logger.Error("failed to read message", zap.Error(err))
                continue
            }
            
            if err := c.handler(ctx, msg.Key, msg.Value); err != nil {
                c.logger.Error("handler error", zap.Error(err))
                // TODO: send to DLQ
            }
        }
    }
}

func (c *KafkaConsumer) Close() error {
    return c.reader.Close()
}
```

**Verification:**
- go build ./internal/queue/
- 写单元测试验证消息消费

---

### Task 4: Host Window Aggregator

**Files:**
- Create: `backend/internal/pipeline/host_window_aggregator.go`

**Implementation:**
```go
package pipeline

import (
    "sync"
    "time"
)

type RuntimeEvent struct {
    EventType    string    `json:"event_type"`
    PID          int       `json:"pid"`
    CommandLine  string    `json:"command_line"`
    MatchedRuleID string  `json:"matched_rule_id"`
    MitreID      string    `json:"mitre_id"`
    Severity     string    `json:"severity"`
    Timestamp    time.Time `json:"timestamp"`
}

type HostWindow struct {
    HostID      string
    Events      []RuntimeEvent
    WindowStart time.Time
    WindowEnd   time.Time
    mu          sync.Mutex
}

type HostWindowAggregator struct {
    windows     map[string]*HostWindow
    windowSize  time.Duration
    mu          sync.RWMutex
    onFlush     func(window *HostWindow)
}

func NewHostWindowAggregator(windowSize time.Duration, onFlush func(window *HostWindow)) *HostWindowAggregator {
    return &HostWindowAggregator{
        windows:    make(map[string]*HostWindow),
        windowSize: windowSize,
        onFlush:    onFlush,
    }
}

func (a *HostWindowAggregator) AddEvent(hostID string, event RuntimeEvent) {
    a.mu.Lock()
    defer a.mu.Unlock()
    
    window, ok := a.windows[hostID]
    if !ok {
        now := time.Now()
        window = &HostWindow{
            HostID:      hostID,
            Events:      []RuntimeEvent{},
            WindowStart: now,
            WindowEnd:   now.Add(a.windowSize),
        }
        a.windows[hostID] = window
    }
    
    window.mu.Lock()
    window.Events = append(window.Events, event)
    window.mu.Unlock()
}

func (a *HostWindowAggregator) FlushReady() map[string][]RuntimeEvent {
    a.mu.Lock()
    defer a.mu.Unlock()
    
    now := time.Now()
    ready := make(map[string][]RuntimeEvent)
    
    for hostID, window := range a.windows {
        if now.After(window.WindowEnd) {
            ready[hostID] = window.Events
            delete(a.windows, hostID)
            
            if a.onFlush != nil {
                go a.onFlush(window)
            }
        }
    }
    
    return ready
}

func (a *HostWindowAggregator) StartTicker(ctx context.Context, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            ready := a.FlushReady()
            // ready windows will be processed by onFlush callback
            _ = ready
        }
    }
}
```

**Verification:**
- go build ./internal/pipeline/
- 写单元测试验证窗口聚合逻辑

---

### Task 5: LLM Prompt Builder

**Files:**
- Create: `backend/internal/pipeline/llm_prompt_builder.go`

**Implementation:**
```go
package pipeline

import (
    "encoding/json"
    "fmt"
)

type LLMAnalysisInput struct {
    HostID      string         `json:"host_id"`
    WindowStart string         `json:"window_start"`
    WindowEnd   string         `json:"window_end"`
    Events      []RuntimeEvent `json:"events"`
}

type ToolDefinition struct {
    Name        string `json:"name"`
    Description string `json:"description"`
    Parameters  map[string]interface{} `json:"parameters"`
}

func BuildAnalysisPrompt(window *HostWindow) (string, error) {
    input := LLMAnalysisInput{
        HostID:      window.HostID,
        WindowStart: window.WindowStart.Format(time.RFC3339),
        WindowEnd:   window.WindowEnd.Format(time.RFC3339),
        Events:      window.Events,
    }
    
    inputJSON, err := json.MarshalIndent(input, "", "  ")
    if err != nil {
        return "", fmt.Errorf("failed to marshal input: %w", err)
    }
    
    systemPrompt := `你是一个主机安全分析专家。分析以下2分钟窗口内的安全事件，判断是否存在真实威胁。

可用工具：
1. get_process_tree(pid) - 获取进程树
2. get_network_connections(pid) - 获取网络连接
3. get_file_info(path) - 获取文件信息
4. get_user_info(username) - 获取用户信息

请返回 JSON 格式：
{
  "alerts": [
    {
      "mitre_id": "T1059.004",
      "severity": "critical|high|medium|low",
      "pid": 12345,
      "description": "威胁描述",
      "block_action": "kill_process",
      "block_target": "12345"
    }
  ],
  "tool_calls": [
    {
      "tool": "get_process_tree",
      "params": {"pid": 12345},
      "reason": "调用原因"
    }
  ],
  "rule_adjustments": [
    {
      "rule_id": "rule_id",
      "action": "tighten|loosen",
      "reason": "调整原因"
    }
  ]
}

注意：
- 最多调用10次工具
- 如果事件是误报，alerts数组可以为空
- severity应根据MITRE ATT&CK技术的危险程度判断`

    return fmt.Sprintf("%s\n\n事件数据：\n%s", systemPrompt, string(inputJSON)), nil
}
```

**Verification:**
- go build ./internal/pipeline/
- 写单元测试验证 prompt 构建

---

### Task 6: LLM Response Parser

**Files:**
- Create: `backend/internal/pipeline/llm_response_parser.go`

**Implementation:**
```go
package pipeline

import (
    "encoding/json"
    "fmt"
    "regexp"
    "strings"
)

type LLMAnalysisOutput struct {
    Alerts           []AlertPayload       `json:"alerts"`
    ToolCalls        []ToolCallPayload    `json:"tool_calls"`
    RuleAdjustments  []RuleAdjustment     `json:"rule_adjustments"`
}

type AlertPayload struct {
    MitreID     string `json:"mitre_id"`
    Severity    string `json:"severity"`
    PID         int    `json:"pid"`
    Description string `json:"description"`
    BlockAction string `json:"block_action"`
    BlockTarget string `json:"block_target"`
}

type ToolCallPayload struct {
    Tool   string                 `json:"tool"`
    Params map[string]interface{} `json:"params"`
    Reason string                 `json:"reason"`
}

type RuleAdjustment struct {
    RuleID string `json:"rule_id"`
    Action string `json:"action"`
    Reason string `json:"reason"`
}

func ParseLLMResponse(response string) (*LLMAnalysisOutput, error) {
    // 清理响应，提取 JSON
    cleaned := extractJSON(response)
    
    var output LLMAnalysisOutput
    if err := json.Unmarshal([]byte(cleaned), &output); err != nil {
        return nil, fmt.Errorf("failed to parse LLM response: %w", err)
    }
    
    // 验证必填字段
    for i, alert := range output.Alerts {
        if alert.MitreID == "" {
            return nil, fmt.Errorf("alert[%d]: mitre_id is required", i)
        }
        if alert.Severity == "" {
            return nil, fmt.Errorf("alert[%d]: severity is required", i)
        }
    }
    
    // 限制工具调用次数
    if len(output.ToolCalls) > 10 {
        output.ToolCalls = output.ToolCalls[:10]
    }
    
    return &output, nil
}

func extractJSON(text string) string {
    // 尝试提取 ```json ... ``` 块
    re := regexp.MustCompile("(?s)```json\\s*(.*?)\\s*```")
    matches := re.FindStringSubmatch(text)
    if len(matches) > 1 {
        return strings.TrimSpace(matches[1])
    }
    
    // 尝试提取 { ... } 块
    start := strings.Index(text, "{")
    end := strings.LastIndex(text, "}")
    if start != -1 && end != -1 && end > start {
        return text[start : end+1]
    }
    
    return text
}
```

**Verification:**
- go build ./internal/pipeline/
- 写单元测试验证 JSON 解析和清理

---

### Task 7: Runtime Pipeline Service

**Files:**
- Create: `backend/internal/service/runtime_pipeline_service.go`

**Implementation:**
```go
package service

import (
    "context"
    "encoding/json"
    "time"

    "aegis-system/internal/model"
    "aegis-system/internal/pipeline"
    "aegis-system/internal/queue"
    "aegis-system/internal/repository"
    "aegis-system/pkg/logger"

    "github.com/google/uuid"
    "go.uber.org/zap"
)

type RuntimePipelineService struct {
    consumer    *queue.KafkaConsumer
    aggregator  *pipeline.HostWindowAggregator
    llmService  *LLMAnalysisService
    alertService *AlertService
    blockService *BlockService
    ruleService  *RuleService
    wsService    *WebSocketService
    logger       *zap.Logger
}

func NewRuntimePipelineService(
    brokers []string,
    groupID string,
    llmService *LLMAnalysisService,
    alertService *AlertService,
    blockService *BlockService,
    ruleService *RuleService,
    wsService *WebSocketService,
) *RuntimePipelineService {
    s := &RuntimePipelineService{
        llmService:   llmService,
        alertService: alertService,
        blockService: blockService,
        ruleService:  ruleService,
        wsService:    wsService,
        logger:       logger.Get(),
    }
    
    // 创建聚合器，2分钟窗口
    s.aggregator = pipeline.NewHostWindowAggregator(2*time.Minute, s.onWindowFlush)
    
    // 创建消费者
    s.consumer = queue.NewKafkaConsumer(
        brokers,
        "raw-events",
        groupID,
        s.handleRawEvent,
        s.logger,
    )
    
    return s
}

func (s *RuntimePipelineService) Start(ctx context.Context) error {
    s.logger.Info("starting runtime pipeline service")
    
    // 启动聚合器定时器
    go s.aggregator.StartTicker(ctx, 10*time.Second)
    
    // 启动消费者
    return s.consumer.Start(ctx)
}

func (s *RuntimePipelineService) handleRawEvent(ctx context.Context, key, value []byte) error {
    var event pipeline.RuntimeEvent
    if err := json.Unmarshal(value, &event); err != nil {
        return err
    }
    
    hostID := string(key)
    s.aggregator.AddEvent(hostID, event)
    
    return nil
}

func (s *RuntimePipelineService) onWindowFlush(window *pipeline.HostWindow) {
    ctx := context.Background()
    
    logger.Info("processing window",
        zap.String("host_id", window.HostID),
        zap.Int("event_count", len(window.Events)),
    )
    
    // 调用 LLM 分析
    result, err := s.llmService.Analyze(ctx, window)
    if err != nil {
        logger.Error("LLM analysis failed", zap.Error(err))
        return
    }
    
    // 处理告警
    for _, alert := range result.Alerts {
        hostUUID, _ := uuid.Parse(window.HostID)
        createdAlert, err := s.alertService.UpsertByDedupe(
            hostUUID,
            alert.PID,
            alert.MitreID,
            "", // mitre_name
            alert.Severity,
            alert.Description,
        )
        if err != nil {
            logger.Error("failed to create alert", zap.Error(err))
            continue
        }
        
        // 检查是否需要自动阻断
        s.alertService.CheckAndAutoBlock(createdAlert)
        
        // WebSocket 推送
        s.wsService.BroadcastAlert(createdAlert)
    }
    
    // 处理工具调用
    for _, toolCall := range result.ToolCalls {
        // TODO: 通过 gRPC 调用 Agent 工具
        logger.Info("tool call requested",
            zap.String("tool", toolCall.Tool),
            zap.String("reason", toolCall.Reason),
        )
    }
    
    // 处理规则调整
    for _, adj := range result.RuleAdjustments {
        logger.Info("rule adjustment",
            zap.String("rule_id", adj.RuleID),
            zap.String("action", adj.Action),
            zap.String("reason", adj.Reason),
        )
    }
}

func (s *RuntimePipelineService) Close() error {
    return s.consumer.Close()
}
```

**Verification:**
- go build ./internal/service/
- 写集成测试验证完整流程

---

### Task 8: LLM Analysis Service

**Files:**
- Create: `backend/internal/service/llm_analysis_service.go`

**Implementation:**
```go
package service

import (
    "context"
    "fmt"

    "aegis-system/internal/llm"
    "aegis-system/internal/pipeline"
    "aegis-system/pkg/logger"

    "go.uber.org/zap"
)

type LLMAnalysisService struct {
    client *llm.Client
    logger *zap.Logger
}

func NewLLMAnalysisService(client *llm.Client) *LLMAnalysisService {
    return &LLMAnalysisService{
        client: client,
        logger: logger.Get(),
    }
}

func (s *LLMAnalysisService) Analyze(ctx context.Context, window *pipeline.HostWindow) (*pipeline.LLMAnalysisOutput, error) {
    // 构建 prompt
    prompt, err := pipeline.BuildAnalysisPrompt(window)
    if err != nil {
        return nil, fmt.Errorf("failed to build prompt: %w", err)
    }
    
    // 调用 LLM
    response, err := s.client.Complete(ctx, prompt)
    if err != nil {
        return nil, fmt.Errorf("LLM call failed: %w", err)
    }
    
    // 解析响应
    output, err := pipeline.ParseLLMResponse(response)
    if err != nil {
        return nil, fmt.Errorf("failed to parse response: %w", err)
    }
    
    // 工具调用循环（最多10次）
    toolCallCount := 0
    for len(output.ToolCalls) > 0 && toolCallCount < 10 {
        toolCall := output.ToolCalls[0]
        output.ToolCalls = output.ToolCalls[1:]
        
        // 执行工具调用
        result, err := s.executeToolCall(ctx, window.HostID, toolCall)
        if err != nil {
            s.logger.Error("tool call failed", zap.Error(err))
            continue
        }
        
        // 将工具结果加入下一轮分析
        prompt = fmt.Sprintf("%s\n\n工具调用结果：\n%s\n\n请继续分析。", prompt, result)
        response, err = s.client.Complete(ctx, prompt)
        if err != nil {
            return nil, fmt.Errorf("LLM call failed: %w", err)
        }
        
        nextOutput, err := pipeline.ParseLLMResponse(response)
        if err != nil {
            return nil, fmt.Errorf("failed to parse response: %w", err)
        }
        
        // 合并结果
        output.Alerts = append(output.Alerts, nextOutput.Alerts...)
        output.ToolCalls = append(output.ToolCalls, nextOutput.ToolCalls...)
        output.RuleAdjustments = append(output.RuleAdjustments, nextOutput.RuleAdjustments...)
        
        toolCallCount++
    }
    
    return output, nil
}

func (s *LLMAnalysisService) executeToolCall(ctx context.Context, hostID string, call pipeline.ToolCallPayload) (string, error) {
    // TODO: 通过 gRPC 调用 Agent 工具
    // 暂时返回模拟结果
    return fmt.Sprintf("工具 %s 执行结果（待实现）", call.Tool), nil
}
```

**Verification:**
- go build ./internal/service/
- 写单元测试验证 LLM 调用和工具循环

---

### Task 9: Block Service

**Files:**
- Create: `backend/internal/service/block_service.go`

**Implementation:**
```go
package service

import (
    "fmt"

    "aegis-system/internal/grpc_server"
    "aegis-system/internal/model"
    "aegis-system/internal/repository"
    "aegis-system/pkg/logger"

    "github.com/google/uuid"
    "go.uber.org/zap"
)

type BlockService struct {
    blockRepo  *repository.BlockRepository
    grpcServer *grpc_server.GRPCServer
    logger     *zap.Logger
}

func NewBlockService(blockRepo *repository.BlockRepository, grpcServer *grpc_server.GRPCServer) *BlockService {
    return &BlockService{
        blockRepo:  blockRepo,
        grpcServer: grpcServer,
        logger:     logger.Get(),
    }
}

func (s *BlockService) ExecuteBlock(hostID string, action string, target string, alertID *uuid.UUID, issuedBy string) (*model.BlockRecord, error) {
    // 创建阻断记录
    record := &model.BlockRecord{
        BlockID:  "BLK-" + uuid.New().String()[:8],
        AlertID:  alertID,
        HostID:   uuid.MustParse(hostID),
        Action:   action,
        Target:   target,
        IssuedBy: issuedBy,
    }
    
    // 通过 gRPC 下发阻断指令
    err := s.dispatchBlockCommand(hostID, action, target)
    if err != nil {
        record.Success = false
        record.Message = err.Error()
        s.logger.Error("block command failed",
            zap.String("host_id", hostID),
            zap.String("action", action),
            zap.Error(err),
        )
    } else {
        record.Success = true
        record.Message = "阻断指令已下发"
    }
    
    // 保存记录
    if err := s.blockRepo.Create(record); err != nil {
        return nil, fmt.Errorf("failed to create block record: %w", err)
    }
    
    return record, nil
}

func (s *BlockService) dispatchBlockCommand(hostID, action, target string) error {
    // TODO: 实现 gRPC 调用
    // 当前 gRPC server 支持 ExecuteCommand 流
    // 需要添加专门的 BlockCommand RPC
    s.logger.Info("dispatching block command",
        zap.String("host_id", hostID),
        zap.String("action", action),
        zap.String("target", target),
    )
    return nil
}
```

**Verification:**
- go build ./internal/service/
- 写单元测试验证阻断逻辑

---

### Task 10: Rule Service

**Files:**
- Create: `backend/internal/service/rule_service.go`

**Implementation:**
```go
package service

import (
    "context"
    "fmt"
    "time"

    "aegis-system/internal/model"
    "aegis-system/internal/queue"
    "aegis-system/internal/repository"
    "aegis-system/pkg/logger"

    "go.uber.org/zap"
)

type RuleService struct {
    ruleRepo  *repository.SigmaRuleRepository
    producer  *queue.KafkaProducer
    logger    *zap.Logger
}

func NewRuleService(ruleRepo *repository.SigmaRuleRepository, producer *queue.KafkaProducer) *RuleService {
    return &RuleService{
        ruleRepo: ruleRepo,
        producer: producer,
        logger:   logger.Get(),
    }
}

func (s *RuleService) DistributeAllRules(ctx context.Context) error {
    rules, _, err := s.ruleRepo.List(1, 10000, map[string]interface{}{})
    if err != nil {
        return fmt.Errorf("failed to list rules: %w", err)
    }
    
    for _, rule := range rules {
        if rule.Status == "active" || rule.Status == "experimental" {
            if err := s.publishRuleUpdate(ctx, "add", &rule); err != nil {
                s.logger.Error("failed to publish rule",
                    zap.String("rule_id", rule.RuleID),
                    zap.Error(err),
                )
            }
        }
    }
    
    return nil
}

func (s *RuleService) DistributeRuleChange(ctx context.Context, action string, rule *model.SigmaRule) error {
    return s.publishRuleUpdate(ctx, action, rule)
}

func (s *RuleService) publishRuleUpdate(ctx context.Context, action string, rule *model.SigmaRule) error {
    update := map[string]interface{}{
        "action":    action,
        "rule_id":   rule.RuleID,
        "content":   rule.Content,
        "status":    rule.Status,
        "mitre_id":  rule.MitreID,
        "severity":  rule.Severity,
        "timestamp": time.Now().Format(time.RFC3339),
    }
    
    return s.producer.SendMessage(ctx, "rule-updates", rule.RuleID, update)
}

func (s *RuleService) CheckAndActivatePending() error {
    rules, _, err := s.ruleRepo.List(1, 10000, map[string]interface{}{"status": "pending"})
    if err != nil {
        return err
    }
    
    for _, rule := range rules {
        if time.Since(rule.CreatedAt) >= 24*time.Hour {
            if err := s.ruleRepo.UpdateStatus(rule.RuleID, "experimental"); err != nil {
                s.logger.Error("failed to activate pending rule",
                    zap.String("rule_id", rule.RuleID),
                    zap.Error(err),
                )
            } else {
                s.logger.Info("pending rule activated as experimental",
                    zap.String("rule_id", rule.RuleID),
                )
                // 发布规则更新
                s.DistributeRuleChange(context.Background(), "update", &rule)
            }
        }
    }
    
    return nil
}

func (s *RuleService) CheckAndPromoteExperimental() error {
    rules, err := s.ruleRepo.GetActiveAndExperimental()
    if err != nil {
        return err
    }
    
    for _, rule := range rules {
        if rule.Status == "experimental" && rule.ActivatedAt != nil {
            if time.Since(*rule.ActivatedAt) >= 7*24*time.Hour {
                if err := s.ruleRepo.UpdateStatus(rule.RuleID, "active"); err != nil {
                    s.logger.Error("failed to promote rule",
                        zap.String("rule_id", rule.RuleID),
                        zap.Error(err),
                    )
                } else {
                    s.logger.Info("rule promoted to active",
                        zap.String("rule_id", rule.RuleID),
                    )
                    // 发布规则更新
                    s.DistributeRuleChange(context.Background(), "update", &rule)
                }
            }
        }
    }
    
    return nil
}
```

**Verification:**
- go build ./internal/service/
- 写单元测试验证规则分发逻辑

---

### Task 11: WebSocket Service

**Files:**
- Create: `backend/internal/service/websocket_service.go`

**Implementation:**
```go
package service

import (
    "sync"

    "aegis-system/internal/model"
    "aegis-system/pkg/logger"

    "github.com/gorilla/websocket"
    "go.uber.org/zap"
)

type WSMessage struct {
    Type string      `json:"type"`
    Data interface{} `json:"data"`
}

type WebSocketService struct {
    clients map[*websocket.Conn]bool
    mu      sync.RWMutex
    logger  *zap.Logger
}

func NewWebSocketService() *WebSocketService {
    return &WebSocketService{
        clients: make(map[*websocket.Conn]bool),
        logger:  logger.Get(),
    }
}

func (s *WebSocketService) AddClient(conn *websocket.Conn) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.clients[conn] = true
    s.logger.Info("WebSocket client connected", zap.Int("total_clients", len(s.clients)))
}

func (s *WebSocketService) RemoveClient(conn *websocket.Conn) {
    s.mu.Lock()
    defer s.mu.Unlock()
    delete(s.clients, conn)
    s.logger.Info("WebSocket client disconnected", zap.Int("total_clients", len(s.clients)))
}

func (s *WebSocketService) Broadcast(msg WSMessage) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    for conn := range s.clients {
        if err := conn.WriteJSON(msg); err != nil {
            s.logger.Error("failed to broadcast message", zap.Error(err))
            conn.Close()
            delete(s.clients, conn)
        }
    }
}

func (s *WebSocketService) BroadcastAlert(alert *model.Alert) {
    s.Broadcast(WSMessage{
        Type: "alert",
        Data: alert,
    })
}

func (s *WebSocketService) BroadcastBlockStatus(record *model.BlockRecord) {
    s.Broadcast(WSMessage{
        Type: "block_status",
        Data: record,
    })
}

func (s *WebSocketService) BroadcastRuleUpdate(rule *model.SigmaRule) {
    s.Broadcast(WSMessage{
        Type: "rule_update",
        Data: rule,
    })
}
```

**Verification:**
- go build ./internal/service/
- 写单元测试验证广播逻辑

---

### Task 12: WebSocket Handler

**Files:**
- Create: `backend/internal/api/handler/websocket_handler.go`
- Modify: `backend/internal/api/router.go` (添加 WebSocket 路由)

**Implementation:**
```go
package handler

import (
    "net/http"

    "aegis-system/internal/service"
    "aegis-system/pkg/logger"

    "github.com/gin-gonic/gin"
    "github.com/gorilla/websocket"
    "go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
    CheckOrigin: func(r *http.Request) bool {
        return true // 允许所有来源，生产环境应限制
    },
}

type WebSocketHandler struct {
    wsService *service.WebSocketService
    logger    *zap.Logger
}

func NewWebSocketHandler(wsService *service.WebSocketService) *WebSocketHandler {
    return &WebSocketHandler{
        wsService: wsService,
        logger:    logger.Get(),
    }
}

func (h *WebSocketHandler) HandleConnection(c *gin.Context) {
    conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
    if err != nil {
        h.logger.Error("failed to upgrade to websocket", zap.Error(err))
        return
    }
    defer conn.Close()
    
    h.wsService.AddClient(conn)
    defer h.wsService.RemoveClient(conn)
    
    // 保持连接，读取客户端消息（心跳等）
    for {
        _, _, err := conn.ReadMessage()
        if err != nil {
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
                h.logger.Error("websocket read error", zap.Error(err))
            }
            break
        }
    }
}
```

**Router 更新:**
```go
// 在 router.go 的 detection group 中添加
websocketHandler := handler.NewWebSocketHandler(wsService)
detection.GET("/runtime/ws", websocketHandler.HandleConnection)
```

**Verification:**
- go build ./internal/api/handler/
- 启动服务后用 wscat 测试连接

---

### Task 13: Rule Loader

**Files:**
- Create: `backend/internal/service/rule_loader.go`

**Implementation:**
```go
package service

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "aegis-system/internal/model"
    "aegis-system/internal/repository"
    "aegis-system/pkg/logger"

    "go.uber.org/zap"
    "gopkg.in/yaml.v3"
)

type SigmaRuleYAML struct {
    Title       string   `yaml:"title"`
    ID          string   `yaml:"id"`
    Status      string   `yaml:"status"`
    Description string   `yaml:"description"`
    Logsource   struct {
        Category string `yaml:"category"`
        Product  string `yaml:"product"`
    } `yaml:"logsource"`
    Detection struct {
        Selection map[string]interface{} `yaml:"selection"`
        Condition string                 `yaml:"condition"`
    } `yaml:"detection"`
    Level string   `yaml:"level"`
    Tags  []string `yaml:"tags"`
}

type RuleLoader struct {
    ruleRepo *repository.SigmaRuleRepository
    logger   *zap.Logger
}

func NewRuleLoader(ruleRepo *repository.SigmaRuleRepository) *RuleLoader {
    return &RuleLoader{
        ruleRepo: ruleRepo,
        logger:   logger.Get(),
    }
}

func (l *RuleLoader) LoadFromDirectory(ctx context.Context, dirPath string) error {
    files, err := filepath.Glob(filepath.Join(dirPath, "*.yml"))
    if err != nil {
        return fmt.Errorf("failed to glob rules: %w", err)
    }
    
    yamlFiles, _ := filepath.Glob(filepath.Join(dirPath, "*.yaml"))
    files = append(files, yamlFiles...)
    
    for _, file := range files {
        if err := l.LoadFromFile(ctx, file); err != nil {
            l.logger.Error("failed to load rule",
                zap.String("file", file),
                zap.Error(err),
            )
        }
    }
    
    return nil
}

func (l *RuleLoader) LoadFromFile(ctx context.Context, filePath string) error {
    data, err := os.ReadFile(filePath)
    if err != nil {
        return fmt.Errorf("failed to read file: %w", err)
    }
    
    var rule SigmaRuleYAML
    if err := yaml.Unmarshal(data, &rule); err != nil {
        return fmt.Errorf("failed to parse YAML: %w", err)
    }
    
    // 检查是否已存在
    existing, err := l.ruleRepo.FindByID(rule.ID)
    if err == nil && existing != nil {
        l.logger.Info("rule already exists, skipping", zap.String("rule_id", rule.ID))
        return nil
    }
    
    // 创建规则记录
    sigmaRule := &model.SigmaRule{
        RuleID:      rule.ID,
        Title:       rule.Title,
        Description: rule.Description,
        Content:     string(data),
        Status:      "active",
        MitreID:     extractMitreID(rule.Tags),
        Severity:    rule.Level,
        GeneratedBy: "manual",
        Version:     "1.0",
    }
    
    if err := l.ruleRepo.Create(sigmaRule); err != nil {
        return fmt.Errorf("failed to create rule: %w", err)
    }
    
    l.logger.Info("rule loaded",
        zap.String("rule_id", rule.ID),
        zap.String("title", rule.Title),
    )
    
    return nil
}

func extractMitreID(tags []string) string {
    for _, tag := range tags {
        if strings.HasPrefix(tag, "attack.") {
            return strings.TrimPrefix(tag, "attack.")
        }
    }
    return ""
}
```

**Verification:**
- go build ./internal/service/
- 写单元测试验证规则加载

---

### Task 14: 集成 Wiring

**Files:**
- Modify: `backend/cmd/server/main.go` (添加新服务初始化和启动)

**Changes:**
1. 添加 Kafka producer 初始化
2. 添加 WebSocket service 初始化
3. 添加 RuntimePipelineService 初始化和启动
4. 添加 RuleLoader 并在启动时加载规则
5. 添加定时任务（规则生命周期检查）

**Verification:**
- go build ./cmd/server/
- 启动服务验证所有组件正常运行

---

### Task 15: 端到端测试

**Test Steps:**
1. 启动所有服务：docker compose up -d --build
2. 验证 Kafka topics 创建
3. 模拟发送事件到 raw-events topic
4. 验证告警创建
5. 测试 WebSocket 连接
6. 测试手动阻断 API
7. 验证规则加载

**Verification Commands:**
```bash
# 检查服务状态
docker compose ps

# 检查 Kafka topics
docker exec aegis-kafka kafka-topics --bootstrap-server localhost:9092 --list

# 发送测试事件
docker exec aegis-kafka kafka-console-producer --bootstrap-server localhost:9092 --topic raw-events <<< '{"event_type":"process_exec","pid":12345,"command_line":"/bin/bash -i","mitre_id":"T1059.004","severity":"critical","timestamp":"2026-03-20T10:00:00Z"}'

# 查询告警
curl http://localhost:8080/api/v1/detection/alerts

# 测试 WebSocket
wscat -c ws://localhost:8080/api/v1/detection/runtime/ws
```

---

## 完成标准

- [ ] Kafka 服务正常运行
- [ ] 事件能从 Kafka 消费并聚合
- [ ] LLM 分析能正常调用
- [ ] 告警能正确创建和去重
- [ ] 阻断指令能通过 gRPC 下发
- [ ] WebSocket 能实时推送
- [ ] 规则能从 YAML 文件加载
- [ ] 所有 API 端点正常响应
- [ ] 单元测试和集成测试通过
