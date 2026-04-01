# Aegis智能主机安全系统 V5.5 后端详细设计文档

**版本**: 5.5
**日期**: 2026-03-30
**状态**: 定稿

---

## 1. 项目结构

### 1.1 V5.5后端整体结构

```
/backend
├── cmd/
│   ├── server/                   # 单体服务入口 (兼容旧版)
│   │   └── main.go
│   ├── api-service/              # API服务入口 (V5.5新增)
│   │   └── main.go
│   ├── agent-hub/                # Agent Hub服务入口 (V5.5新增)
│   │   └── main.go
│   └── pipeline/                 # Pipeline服务入口 (V5.5新增)
│       └── main.go
│
├── internal/
│   ├── api/                      # HTTP层 (API Service使用)
│   │   ├── router.go
│   │   └── handler/
│   │       ├── host_handler.go
│   │       ├── rule_handler.go
│   │       ├── alert_handler.go
│   │       └── ...
│   │
│   ├── service/                  # 业务服务层
│   │   ├── template_service.go
│   │   ├── task_service.go
│   │   ├── alert_service.go
│   │   ├── rule_service.go
│   │   └── ...
│   │
│   ├── agent_hub/                # Agent通信层 (V5.5新增)
│   │   ├── server.go             # gRPC Server
│   │   ├── manager.go            # Agent管理器
│   │   ├── registry.go           # 注册表
│   │   ├── heartbeat.go          # 心跳监控
│   │   ├── dispatcher.go         # 命令下发
│   │   └── collector.go          # 结果回收
│   │
│   ├── pipeline/                 # 事件处理管道 (V5.5新增)
│   │   ├── consumer.go           # Kafka消费者
│   │   ├── processor.go          # 事件处理器
│   │   ├── llm_analyzer.go       # LLM分析器
│   │   ├── alert_generator.go   # 告警生成器
│   │   └── block_manager.go     # 阻断管理器
│   │
│   ├── repository/               # 数据访问层
│   │   ├── host_repo.go
│   │   ├── rule_repo.go
│   │   ├── alert_repo.go
│   │   └── ...
│   │
│   ├── model/                    # 数据模型
│   │   ├── host.go
│   │   ├── rule.go
│   │   ├── alert.go
│   │   └── ...
│   │
│   ├── queue/                    # Kafka队列
│   │   ├── producer.go
│   │   └── consumer.go
│   │
│   ├── storage/                  # 存储层
│   │   ├── redis_client.go
│   │   └── minio_client.go
│   │
│   └── llm/                      # LLM客户端
│       ├── client.go
│       └── prompts.go
│
├── pkg/                          # 公共包
│   ├── api/v1/                   # 生成protobuf
│   └── logger/
│
└── config/
    └── config.yaml
```

---

## 2. 微服务详细设计

### 2.1 API Service

```go
// cmd/api-service/main.go
package main

func main() {
    // 初始化配置
    cfg := config.Load()

    // 初始化日志
    logger.Init(cfg.Logger)

    // 初始化数据库
    db := repository.NewDB(&cfg.Database)

    // 初始化Redis
    redis := storage.NewRedisClient(&cfg.Redis)

    // 初始化MinIO
    minio := storage.NewMinIOClient(&cfg.MinIO)

    // 初始化Repository
    hostRepo := repository.NewHostRepository(db)
    ruleRepo := repository.NewRuleRepository(db)
    alertRepo := repository.NewAlertRepository(db)

    // 初始化Service
    alertService := service.NewAlertService(alertRepo)
    wsService := service.NewWebSocketService()

    // 初始化Handler
    hostHandler := handler.NewHostHandler(hostRepo, redis)
    alertHandler := handler.NewAlertHandler(alertRepo, wsService)

    // 初始化Router
    router := api.NewRouter(hostHandler, alertHandler, ...)

    // 启动HTTP服务器
    router.Run(fmt.Sprintf(":%d", cfg.Server.HTTPPort))
}
```

**端口**: 8080
**职责**:
- HTTP REST API
- WebSocket实时推送
- 认证授权
- 请求路由

### 2.2 Agent Hub

```go
// cmd/agent-hub/main.go
package main

func main() {
    cfg := config.Load()
    logger.Init(cfg.Logger)

    // 初始化数据库
    db := repository.NewDB(&cfg.Database)

    // 初始化Redis
    redis := storage.NewRedisClient(&cfg.Redis)

    // 初始化Kafka Producer
    kafkaProducer := queue.NewKafkaProducer(cfg.Kafka.Brokers)

    // 初始化Repository
    hostRepo := repository.NewHostRepository(db)
    runtimeEventRepo := repository.NewRuntimeEventRepository(db)

    // 初始化Agent Manager
    manager := agent_hub.NewAgentManager(hostRepo, redis)

    // 初始化gRPC Server
    grpcServer := agent_hub.NewGRPCServer(
        manager,
        kafkaProducer,
        cfg.Server.GRPCPort,
    )

    // 启动服务
    if err := grpcServer.Start(); err != nil {
        logger.Fatal("failed to start gRPC server", zap.Error(err))
    }
}
```

**端口**: 19090 (gRPC)
**职责**:
- Agent注册管理
- 心跳监控
- 命令下发
- 结果回收
- 事件路由到Kafka

### 2.3 Pipeline Service

```go
// cmd/pipeline/main.go
package main

func main() {
    cfg := config.Load()
    logger.Init(cfg.Logger)

    // 初始化数据库
    db := repository.NewDB(&cfg.Database)

    // 初始化Redis
    redis := storage.NewRedisClient(&cfg.Redis)

    // 初始化Kafka Consumer
    kafkaConsumer := queue.NewKafkaConsumer(
        cfg.Kafka.Brokers,
        "pipeline-group",
        []string{"aegis.security.events"},
    )

    // 初始化LLM Client
    llmClient := llm.NewClient(&cfg.LLM)

    // 初始化Repository
    alertRepo := repository.NewAlertRepository(db)
    sigmaRuleRepo := repository.NewSigmaRuleRepository(db)
    runtimeEventRepo := repository.NewRuntimeEventRepository(db)

    // 初始化Pipeline
    pipeline := pipeline.NewPipeline(
        kafkaConsumer,
        llmClient,
        alertRepo,
        sigmaRuleRepo,
        runtimeEventRepo,
    )

    // 启动处理
    pipeline.Start(context.Background())
}
```

**端口**: 19091 (内部gRPC)
**职责**:
- Kafka事件消费
- LLM智能分析
- 告警生成
- 阻断策略管理

---

## 3. Agent Manager 核心设计

### 3.1 Agent注册表

```go
// internal/agent_hub/registry.go
package agent_hub

import (
    "sync"
    "time"
)

type AgentRegistry struct {
    agents map[string]*AgentInfo
    mu     sync.RWMutex
}

type AgentInfo struct {
    AgentID     string
    Hostname    string
    IP          string
    OS          string
    Version     string
    Status      AgentStatus
    RegisteredAt time.Time
    LastHeartbeat time.Time
    Capabilities []string
}

type AgentStatus string

const (
    AgentStatusOnline  AgentStatus = "online"
    AgentStatusOffline AgentStatus = "offline"
    AgentStatusUnknown AgentStatus = "unknown"
)

// 注册Agent
func (r *AgentRegistry) Register(req *RegisterRequest) (*AgentInfo, error) {
    r.mu.Lock()
    defer r.mu.Unlock()

    info := &AgentInfo{
        AgentID:     req.AgentID,
        Hostname:    req.Hostname,
        IP:          req.IP,
        OS:          req.OS,
        Version:     req.Version,
        Status:      AgentStatusOnline,
        RegisteredAt: time.Now(),
        LastHeartbeat: time.Now(),
        Capabilities: req.Capabilities,
    }

    r.agents[req.AgentID] = info
    return info, nil
}

// 更新心跳
func (r *AgentRegistry) UpdateHeartbeat(agentID string) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    if agent, ok := r.agents[agentID]; ok {
        agent.LastHeartbeat = time.Now()
        agent.Status = AgentStatusOnline
        return nil
    }
    return fmt.Errorf("agent not found: %s", agentID)
}

// 获取所有Agent
func (r *AgentRegistry) GetAllAgents() []*AgentInfo {
    r.mu.RLock()
    defer r.mu.RUnlock()

    agents := make([]*AgentInfo, 0, len(r.agents))
    for _, agent := range r.agents {
        agents = append(agents, agent)
    }
    return agents
}

// 检查离线Agent
func (r *AgentRegistry) CheckOfflineAgents(timeout time.Duration) []string {
    r.mu.Lock()
    defer r.mu.Unlock()

    var offline []string
    now := time.Now()

    for agentID, agent := range r.agents {
        if now.Sub(agent.LastHeartbeat) > timeout {
            agent.Status = AgentStatusOffline
            offline = append(offline, agentID)
        }
    }
    return offline
}
```

### 3.2 心跳监控

```go
// internal/agent_hub/heartbeat.go
package agent_hub

type HeartbeatMonitor struct {
    registry    *AgentRegistry
    timeout     time.Duration
    checkInterval time.Duration
    stopCh      chan struct{}
    wg          sync.WaitGroup
}

func NewHeartbeatMonitor(registry *AgentRegistry, timeout time.Duration) *HeartbeatMonitor {
    return &HeartbeatMonitor{
        registry:     registry,
        timeout:      timeout,
        checkInterval: 10 * time.Second,
        stopCh:       make(chan struct{}),
    }
}

func (m *HeartbeatMonitor) Start(ctx context.Context) {
    m.wg.Add(1)
    go m.run(ctx)
}

func (m *HeartbeatMonitor) run(ctx context.Context) {
    defer m.wg.Done()

    ticker := time.NewTicker(m.checkInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            offline := m.registry.CheckOfflineAgents(m.timeout)
            if len(offline) > 0 {
                // 记录离线日志或触发告警
                logger.Warn("agents offline", zap.Strings("agents", offline))
            }
        }
    }
}
```

### 3.3 命令下发

```go
// internal/agent_hub/dispatcher.go
package agent_hub

type CommandDispatcher struct {
    registry *AgentRegistry
    streams  map[string]AgentStream
    mu       sync.RWMutex
}

type AgentStream struct {
    AgentID string
    Send    chan *CommandExecute
    Done    chan struct{}
}

// 下发命令到指定Agent
func (d *CommandDispatcher) DispatchCommand(agentID string, cmd *CommandExecute) error {
    d.mu.RLock()
    stream, ok := d.streams[agentID]
    d.mu.RUnlock()

    if !ok {
        return fmt.Errorf("agent stream not found: %s", agentID)
    }

    select {
    case stream.Send <- cmd:
        return nil
    case <-time.After(5 * time.Second):
        return fmt.Errorf("command dispatch timeout: %s", agentID)
    }
}

// 广播命令到所有在线Agent
func (d *CommandDispatcher) BroadcastCommand(cmd *CommandExecute) {
    d.mu.RLock()
    defer d.mu.RUnlock()

    for _, stream := range d.streams {
        select {
        case stream.Send <- cmd:
        default:
            // 非阻塞，如果通道满则跳过
        }
    }
}
```

---

## 4. Pipeline 核心设计

### 4.1 事件处理器

```go
// internal/pipeline/processor.go
package pipeline

type EventProcessor struct {
    llmClient        *llm.Client
    alertRepo        repository.AlertRepository
    sigmaRuleRepo    repository.SigmaRuleRepository
    runtimeEventRepo repository.RuntimeEventRepository
}

type ProcessResult struct {
    AlertID       string
    Severity      string
    IsBlock       bool
    LLMSummary    string
    BlockAction   string
}

// 处理单个事件
func (p *EventProcessor) ProcessEvent(event *model.RuntimeEvent) (*ProcessResult, error) {
    // 1. 查询匹配规则
    rule, err := p.sigmaRuleRepo.FindMatchedRule(event)
    if err != nil {
        return nil, err
    }

    if rule == nil {
        // 无匹配规则，可能是正常行为
        return nil, nil
    }

    // 2. 构建LLM分析上下文
    context := p.buildAnalysisContext(event, rule)

    // 3. 调用LLM分析 (如果是关键事件)
    var llmSummary string
    if p.needLLMAnalysis(rule.Severity) {
        summary, err := p.llmClient.AnalyzeEvent(context)
        if err != nil {
            logger.Warn("LLM analysis failed", zap.Error(err))
            // 降级到规则判断
        } else {
            llmSummary = summary
        }
    }

    // 4. 生成结果
    result := &ProcessResult{
        AlertID:    generateAlertID(),
        Severity:   rule.Severity,
        IsBlock:    rule.AutoBlock,
        LLMSummary: llmSummary,
    }

    // 5. 保存告警
    alert := &model.Alert{
        AlertID:    result.AlertID,
        HostID:     event.HostID,
        Severity:   result.Severity,
        RuleID:     rule.RuleID,
        LLMSummary: result.LLMSummary,
    }

    if err := p.alertRepo.Create(alert); err != nil {
        return nil, err
    }

    return result, nil
}

// 批量处理事件
func (p *EventProcessor) ProcessBatch(events []*model.RuntimeEvent) ([]*ProcessResult, error) {
    results := make([]*ProcessResult, 0, len(events))

    for _, event := range events {
        result, err := p.ProcessEvent(event)
        if err != nil {
            logger.Warn("process event failed", zap.Error(err))
            continue
        }
        if result != nil {
            results = append(results, result)
        }
    }

    return results, nil
}
```

### 4.2 阻断策略管理

```go
// internal/pipeline/block_manager.go
package pipeline

type BlockManager struct {
    blockPolicyRepo repository.BlockPolicyRepository
    kafkaProducer   queue.KafkaProducer
    grpcServer      *grpc.Server
}

// 广播阻断策略到所有Agent
func (bm *BlockManager) BroadcastBlockPolicy(policy *model.BlockPolicy) error {
    // 1. 保存到数据库
    if err := bm.blockPolicyRepo.Upsert(policy); err != nil {
        return err
    }

    // 2. 发送到Kafka (供Pipeline消费)
    blockEvent := &BlockPolicyEvent{
        Type:      "block_policy_update",
        Policy:    policy,
        Timestamp: time.Now(),
    }

    data, err := json.Marshal(blockEvent)
    if err != nil {
        return err
    }

    return bm.kafkaProducer.Send("aegis.control", data)
}

// 处理阻断事件
func (bm *BlockManager) HandleBlockEvent(event *BlockPolicyEvent) error {
    switch event.Type {
    case "block_policy_update":
        return bm.handlePolicyUpdate(event.Policy)
    case "block_policy_delete":
        return bm.handlePolicyDelete(event.PolicyID)
    default:
        return fmt.Errorf("unknown block event type: %s", event.Type)
    }
}
```

---

## 5. 服务间通信

### 5.1 API Service → Agent Hub

```go
// 通过gRPC调用Agent Hub
type AgentHubClient struct {
    conn *grpc.ClientConn
    client pb.AgentHubClient
}

// 下发命令
func (c *AgentHubClient) ExecuteCommand(ctx context.Context, req *ExecuteCommandRequest) (*ExecuteCommandResponse, error) {
    return c.client.ExecuteCommand(ctx, req)
}

// 获取Agent状态
func (c *AgentHubClient) GetAgentStatus(ctx context.Context, agentID string) (*AgentStatus, error) {
    return c.client.GetAgentStatus(ctx, &AgentStatusRequest{AgentID: agentID})
}
```

### 5.2 Agent Hub → Pipeline Service

```go
// 通过Kafka发送事件
func (ah *AgentHub) sendEventToPipeline(event *model.RuntimeEvent) error {
    data, err := json.Marshal(event)
    if err != nil {
        return err
    }

    return ah.kafkaProducer.Send("aegis.security.events", data)
}
```

---

## 6. 配置设计

### 6.1 统一配置文件

```yaml
# config/config.yaml

# 服务器通用配置
server:
  http_port: 8080
  grpc_port: 19090
  external_ip: ""

# 数据库配置
database:
  host: postgres
  port: 5432
  user: aegis
  password: aegis
  dbname: aegis

# Redis配置
redis:
  host: redis
  port: 6379
  password: ""
  db: 0

# MinIO配置
minio:
  endpoint: minio:9000
  access_key: minioadmin
  secret_key: minioadmin
  use_ssl: false
  bucket: aegis

# Kafka配置
kafka:
  brokers:
    - kafka:29092
  topics:
    events: aegis.security.events
    control: aegis.control

# LLM配置
llm:
  provider: openai
  api_key: ""
  model: gpt-4
  timeout_seconds: 30
  max_retries: 3
```

### 6.2 微服务独立配置

```yaml
# config/api-service.yaml
service:
  name: api-service
  port: 8080

# config/agent-hub.yaml
service:
  name: agent-hub
  port: 19090

agent:
  heartbeat_interval: 30s
  heartbeat_timeout: 90s

# config/pipeline.yaml
service:
  name: pipeline
  port: 19091

kafka:
  consumer_group: pipeline-group
  batch_size: 100
  batch_timeout: 1s
```

---

## 7. 健康检查与监控

### 7.1 健康检查接口

```go
// API Service
router.GET("/health", func(c *gin.Context) {
    // 检查数据库
    if err := db.Exec("SELECT 1").Error; err != nil {
        c.JSON(503, gin.H{"status": "unhealthy", "error": "database"})
        return
    }

    // 检查Redis
    if err := redis.Ping().Err(); err != nil {
        c.JSON(503, gin.H{"status": "unhealthy", "error": "redis"})
        return
    }

    c.JSON(200, gin.H{"status": "healthy"})
})

// Agent Hub (gRPC Health Check)
func (s *GRPCServer) Check(ctx context.Context, req *health.CheckRequest) (*health.CheckResponse, error) {
    return &health.CheckResponse{Status: health.HealthCheckResponse_SERVING}, nil
}
```

### 7.2 监控指标

| 指标 | 描述 |
|------|------|
| agent_online_count | 在线Agent数量 |
| agent_offline_count | 离线Agent数量 |
| event_processed_total | 已处理事件总数 |
| event_process_duration | 事件处理延迟 |
| alert_generated_total | 生成的告警总数 |
| llm_call_total | LLM调用次数 |
| llm_call_duration | LLM调用延迟 |

---

## 8. 部署配置

### 8.1 Docker Compose (微服务模式)

```yaml
version: '3.8'

services:
  # API Service
  api-service:
    build: ./backend
    command: ./api-service
    ports:
      - "8080:8080"
    environment:
      - CONFIG_PATH=/config/api-service.yaml
    depends_on:
      - postgres
      - redis

  # Agent Hub
  agent-hub:
    build: ./backend
    command: ./agent-hub
    ports:
      - "19090:19090"
    environment:
      - CONFIG_PATH=/config/agent-hub.yaml
    depends_on:
      - postgres
      - redis
      - kafka

  # Pipeline Service
  pipeline:
    build: ./backend
    command: ./pipeline
    environment:
      - CONFIG_PATH=/config/pipeline.yaml
    depends_on:
      - postgres
      - redis
      - kafka

  # 共享服务
  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: aegis
      POSTGRES_USER: aegis
      POSTGRES_PASSWORD: aegis

  redis:
    image: redis:7

  kafka:
    image: confluentinc/cp-kafka:7.5.0

  minio:
    image: minio/minio
    command: server /data

  nginx:
    image: nginx:alpine
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
    ports:
      - "80:80"
```

---

## 9. 迁移策略

### 9.1 从单体到微服务

**阶段1**: 保持单体代码结构，添加服务入口
- 添加 `cmd/api-service/main.go`
- 添加 `cmd/agent-hub/main.go`
- 添加 `cmd/pipeline/main.go`

**阶段2**: 代码拆分
- 将 `internal/agent_hub/` 抽取为独立模块
- 将 `internal/pipeline/` 抽取为独立模块
- 共享 `internal/repository/`、`internal/model/`

**阶段3**: 配置分离
- 配置文件拆分
- 环境变量配置

**阶段4**: 独立部署
- Docker镜像拆分
- Kubernetes部署

---

**文档结束**