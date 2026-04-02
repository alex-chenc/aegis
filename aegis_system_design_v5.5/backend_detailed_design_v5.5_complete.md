# Aegis智能主机安全系统 V5.5 后端详细设计文档

**版本**: 5.5
**日期**: 2026-03-30
**状态**: 定稿

---

## 1. 项目结构

### 1.1 V5.5后端整体结构

```
/
├── api-server/                   # API Server服务
│   ├── cmd/
│   │   └── main.go
│   └── internal/
│       ├── api/                  # HTTP层
│       ├── service/              # 业务服务层
│       ├── repository/           # 数据访问层
│       ├── storage/              # 存储层
│       ├── queue/                # Kafka生产者
│       ├── grpc/                 # gRPC客户端
│       └── llm/                  # LLM客户端
│
├── server/                       # Server服务 (Agent Hub)
│   ├── cmd/
│   │   └── main.go
│   └── internal/
│       ├── grpc_server/          # gRPC服务端
│       │   ├── server.go
│       │   └── api_server_impl.go
│       ├── repository/
│       ├── storage/
│       └── queue/                # Kafka生产者
│
├── dc/                          # DC服务 (Data Consumer)
│   ├── cmd/
│   │   └── main.go
│   └── internal/
│       ├── server/               # gRPC服务端
│       ├── pipeline/            # 事件处理管道
│       ├── consumer/            # Kafka消费者
│       └── repository/
│
└── frontend/                     # Vue 3前端
```

---

## 2. 微服务详细设计

### 2.1 API Server

```go
// api-server/cmd/main.go
package main

func main() {
    // 初始化配置
    cfg := config.Load("config/api-server.yaml")

    // 初始化日志
    logger.Init(&logger.Config{...})

    // 初始化数据库
    db, err := repository.NewDB(&cfg.Database)

    // 初始化Redis
    redisClient, err := storage.NewRedisClient(&cfg.Redis)

    // 初始化MinIO
    minioClient, err := storage.NewMinIOClient(&cfg.MinIO)

    // 初始化Kafka Producer
    kafkaProducer := queue.NewKafkaProducer(kafkaBrokers, logger.Get())

    // 初始化gRPC Client到Server
    serverClient, err := grpcclient.NewServerClient(serverAddr)

    // 初始化Repository
    hostRepo := repository.NewHostRepository(db)
    taskLogRepo := repository.NewTaskLogRepository(db)
    alertRepo := repository.NewAlertRepository(db)
    ruleRepo := repository.NewRuleRepository(db)
    sigmaRuleRepo := repository.NewSigmaRuleRepository(db)

    // 初始化Service
    taskService := service.NewTaskService(taskLogRepo, hostRepo, ruleRepo, healingLogRepo, redisClient, serverClient)
    alertService := service.NewAlertService(alertRepo, blockPolicyRepo, blockRepo, serverClient)
    wsService := service.NewWebSocketService()

    // 初始化Handler
    hostHandler := handler.NewHostHandler(hostRepo, redisClient, serverClient)
    taskHandler := handler.NewTaskHandler(taskService, taskLogRepo, healingLogRepo, scriptGenService, serverClient, ruleRepo, selfHealingService)
    detectionHandler := handler.NewDetectionHandler(...)

    // 初始化Router
    router := api.NewRouter(configHandler, hostHandler, templateHandler, taskHandler, taskHandlerWithHealing, agentHandler, ruleHandler, vulnerabilityHandler, detectionHandler, websocketHandler)
    router.Setup()

    // 启动HTTP服务器 (端口8082)
    go router.Run(fmt.Sprintf(":%d", cfg.Server.HTTPPort))
}
```

**端口**: 8082 (HTTP), 19093 (gRPC client)
**职责**:
- HTTP REST API
- WebSocket实时推送
- 认证授权
- 请求路由
- gRPC客户端到Server服务 (APIServerToServer:19094)
- gRPC客户端到DC服务

### 2.2 Server (Agent Hub)

```go
// server/cmd/main.go
package main

func main() {
    cfg, err := config.Load("config/server.yaml")
    logger.Init(&logger.Config{...})

    // 初始化数据库
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})

    // 初始化Redis
    redisClient, err := storage.NewRedisClient(&cfg.Redis)

    // 初始化Kafka Producer
    kafkaProducerInstance := queue.NewKafkaProducer(cfg.Kafka.Brokers, logger.Get())

    // 初始化Repository
    hostRepo := repository.NewHostRepository(db)
    sigmaRuleRepo := repository.NewSigmaRuleRepository(db)
    alertRepo := repository.NewAlertRepository(db)
    runtimeEventRepo := repository.NewRuntimeEventRepository(db)
    blockPolicyRepo := repository.NewBlockPolicyRepository(db)

    // 初始化gRPC Server (Agent Hub)
    grpcServer := grpc_server.NewGRPCServer(
        hostRepo,
        redisClient,
        kafkaProducerInstance,
        cfg.Server.GRPCPort,  // 19090
    )

    // 设置额外repository用于事件处理和规则推送
    grpcServer.SetSigmaRuleRepo(sigmaRuleRepo)
    grpcServer.SetAlertRepo(alertRepo, nil)
    grpcServer.SetRuntimeEventRepo(runtimeEventRepo)
    grpcServer.SetBlockPolicyRepo(blockPolicyRepo)

    // 创建APIServerToServer gRPC服务 (端口19094)
    apiServerLis, err := net.Listen("tcp", fmt.Sprintf(":%d", 19094))
    apiServerGRPCServer := grpc.NewServer()
    apiServerImpl := grpc_server.NewAPIServerToServerImpl(grpcServer, hostRepo, redisClient)
    pb.RegisterAPIServerToServerServer(apiServerGRPCServer, apiServerImpl)

    // 启动Agent Hub gRPC服务
    go func() {
        grpcServer.Start()  // 端口19090
    }()

    // 启动APIServerToServer gRPC服务
    go func() {
        apiServerGRPCServer.Serve(apiServerLis)  // 端口19094
    }()
}
```

**端口**: 19090 (Agent Hub), 19094 (APIServerToServer)
**职责**:
- Agent注册管理 (19090)
- 心跳监控 (19090)
- 命令下发 (19090)
- 结果回收 (19090)
- 接收API Server命令 (19094)
- 事件路由到Kafka
- 阻断策略管理

### 2.3 DC (Data Consumer)

```go
// dc/cmd/main.go
package main

func main() {
    cfg := config.Load("config/dc.yaml")
    logger.Init(&logger.Config{...})

    // 初始化数据库
    db, err := repository.NewDB(&cfg.Database)

    // 初始化Kafka Consumer
    kafkaConsumer := queue.NewKafkaConsumer(
        cfg.Kafka.Brokers,
        "aegis-dc-consumer",
        []string{"aegis.security.events", "aegis.block.commands"},
    )

    // 初始化LLM Client
    llmClient := llm.NewClient(&cfg.LLM)

    // 初始化Repository
    alertRepo := repository.NewAlertRepository(db)
    sigmaRuleRepo := repository.NewSigmaRuleRepository(db)
    runtimeEventRepo := repository.NewRuntimeEventRepository(db)
    blockPolicyRepo := repository.NewBlockPolicyRepository(db)

    // 初始化gRPC Server
    grpcServer := dc.NewDCServer(
        alertRepo,
        sigmaRuleRepo,
        runtimeEventRepo,
        blockPolicyRepo,
        kafkaConsumer,
        llmClient,
        cfg.GRPC.ServerPort,
    )

    // 启动gRPC服务和Kafka消费者
    go grpcServer.Start()  // 端口19092
    kafkaConsumer.Start(context.Background())
}
```

**端口**: 19092 (gRPC)
**职责**:
- Kafka事件消费 (aegis.security.events, aegis.block.commands)
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
# docker-compose.yml 环境变量

# API Server配置
DATABASE_HOST: postgres
DATABASE_PORT: 5432
DATABASE_USER: aegis_user
DATABASE_PASSWORD: xxx
DATABASE_DBNAME: aegis_db
REDIS_HOST: redis
REDIS_PORT: 6379
REDIS_PASSWORD: xxx
MINIO_ENDPOINT: minio:9000
SERVER_HTTP_PORT: 8082
SERVER_GRPC_PORT: 19093
GRPC_SERVER_ADDRESS: server:19094

# Server配置
KAFKA_BROKERS: kafka:9092
SERVER_GRPC_PORT: 19090
AGENT_AUTH_TOKEN: xxx
AGENT_HEARTBEAT_TIMEOUT: 90

# DC配置
KAFKA_BROKERS: kafka:9092
KAFKA_GROUP_ID: aegis-dc-consumer
KAFKA_TOPIC: aegis.security.events
GRPC_SERVER_PORT: 19092
LLM_API_KEY: xxx
LLM_BASE_URL: https://api.openai.com/v1
LLM_MODEL_NAME: gpt-4
```

### 6.2 微服务独立配置

```yaml
# api-server/config/api-server.yaml
service:
  name: api-server
  http_port: 8082
  grpc_port: 19093

# server/config/server.yaml
service:
  name: server
  grpc_port: 19090
  api_server_grpc_port: 19094

agent:
  heartbeat_interval: 30s
  heartbeat_timeout: 90s

kafka:
  brokers:
    - kafka:9092

# dc/config/dc.yaml
service:
  name: dc
  grpc_port: 19092

kafka:
  brokers:
    - kafka:9092
  consumer_group: aegis-dc-consumer
  topics:
    - aegis.security.events
    - aegis.block.commands
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
# docker-compose.yml (实际配置)

services:
  # API Server
  api-server:
    build: ./api-server
    ports:
      - "8082:8082"
      - "19093:19093"
    environment:
      DATABASE_HOST: postgres
      SERVER_HTTP_PORT: 8082
      GRPC_SERVER_ADDRESS: server:19094
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      minio:
        condition: service_healthy

  # Server (Agent Hub)
  server:
    build: ./server
    ports:
      - "19090:19090"
      - "19094:19094"
    environment:
      DATABASE_HOST: postgres
      KAFKA_BROKERS: kafka:9092
      SERVER_GRPC_PORT: 19090
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      kafka:
        condition: service_healthy

  # DC (Data Consumer)
  dc:
    build: ./dc
    ports:
      - "19092:19092"
    environment:
      DATABASE_HOST: postgres
      KAFKA_BROKERS: kafka:9092
      KAFKA_GROUP_ID: aegis-dc-consumer
      KAFKA_TOPIC: aegis.security.events
      GRPC_SERVER_PORT: 19092
    depends_on:
      postgres:
        condition: service_healthy
      kafka:
        condition: service_healthy

  # 共享服务
  postgres:
    image: postgres:14-alpine
    ports:
      - "5432:5432"

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

  kafka:
    image: confluentinc/cp-kafka:7.5.0
    ports:
      - "29092:9092"

  minio:
    image: minio/minio:latest
    ports:
      - "9000:9000"
      - "9001:9001"

  frontend:
    image: aegis-system/frontend:latest
    ports:
      - "8081:80"
```

---

## 9. Kafka Topic设计

### 9.1 Topic列表

| Topic | 分区数 | 消费者 | 用途 |
|-------|--------|--------|------|
| aegis.security.events | 3 | DC | Agent上报的运行时安全事件 |
| aegis.block.commands | 3 | Server | DC生成的阻断命令 |
| aegis.rule.updates | 3 | Server | 规则更新通知 |

### 9.2 消息格式

```json
// aegis.security.events
{
  "host_id": "uuid",
  "event_type": "process_create|network_connect|...",
  "timestamp": "2026-04-02T10:00:00Z",
  "data": {
    "process_name": "bash",
    "file_path": "/bin/bash",
    "remote_addr": "",
    "process_tree": "[{\"pid\":1,\"name\":\"init\"},...]"
  }
}

// aegis.block.commands
{
  "type": "block_ip|block_process|unblock",
  "target": "192.168.1.100",
  "rule_id": "uuid",
  "timestamp": "2026-04-02T10:00:00Z"
}
```

---

**文档结束**