# Aegis V6.0 架构升级实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将单体backend拆分为微服务架构，Agent升级为具备本地智能的智能体，实现1C1G约束下的智能威胁检测与响应。

**Architecture:** 采用混合边云架构 - Agent端实现轻量级本地智能(规则引擎+统计异常检测+特征提取)，后端拆分为API Service、Agent Hub、Pipeline Service三个独立服务，通过gRPC和Kafka进行服务间通信。

**Tech Stack:** Go 1.21+, Gin, gRPC, Kafka, Vue 3, eBPF

---

## Part 1: Backend微服务拆分实现

### Task 1.1: 创建API Service入口

**Files:**
- Create: `backend/cmd/api-service/main.go`
- Create: `backend/internal/api_service/server.go`
- Create: `backend/internal/api_service/config.go`
- Modify: `backend/Makefile` (添加api-service构建目标)

**Step 1: 创建api-service主入口**

```go
// backend/cmd/api-service/main.go
package main

import (
	"log"
	"os"

	"aegis-system/internal/api_service"
	"aegis-system/internal/config"
	"aegis-system/internal/logger"
)

func main() {
	cfg, err := config.Load("api-service")
	if err != nil {
		log.Fatal("Failed to load config: ", err)
	}

	if err := logger.Init(cfg.LogLevel); err != nil {
		log.Fatal("Failed to init logger: ", err)
	}

	server := api_service.NewServer(cfg)
	if err := server.Run(); err != nil {
		log.Fatal("Server error: ", err)
	}
}
```

**Step 2: 创建api_service/server.go**

```go
// backend/internal/api_service/server.go
package api_service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"aegis-system/internal/api"
	"aegis-system/internal/api/handler"
	"aegis-system/internal/api/middleware"
	"aegis-system/internal/config"
	"aegis-system/internal/repository"
	"aegis-system/internal/service"
)

type Server struct {
	httpServer *http.Server
	cfg        *config.Config
	grpcConn   *grpc.ClientConn
}

func NewServer(cfg *config.Config) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(middleware.CORS())
	engine.Use(middleware.RequestLogger())

	// 初始化Repository层
	db, err := repository.NewDB(cfg.DatabaseURL)
	if err != nil {
		zap.Fatal("Failed to connect database: ", err)
	}
	
	redisClient, err := repository.NewRedis(cfg.RedisAddr, cfg.RedisPassword)
	if err != nil {
		zap.Fatal("Failed to connect Redis: ", err)
	}

	// 初始化Handler层
	hostHandler := handler.NewHostHandler(repository.NewHostRepo(db))
	ruleHandler := handler.NewRuleHandler(repository.NewRuleRepo(db))
	taskHandler := handler.NewTaskHandler(repository.NewTaskLogRepo(db))
	configHandler := handler.NewConfigHandler(repository.NewConfigRepo(db))
	alertHandler := handler.NewAlertHandler(repository.NewAlertRepo(db))

	// 设置路由
	router := api.NewRouter(
		engine,
		hostHandler,
		ruleHandler,
		taskHandler,
		configHandler,
		alertHandler,
		nil, // websocket handler
	)
	router.Setup(nil)

	return &Server{
		httpServer: &http.Server{
			Addr:    ":8080",
			Handler: engine,
		},
		cfg: cfg,
	}
}

func (s *Server) Run() error {
	// 启动HTTP服务器
	go func() {
		zap.Info("API Service starting on :8080")
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.Fatal("HTTP server error: ", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zap.Info("Shutting down API Service...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return err
	}

	if s.grpcConn != nil {
		s.grpcConn.Close()
	}

	return nil
}
```

**Step 3: 创建Makefile构建目标**

```makefile
# backend/Makefile 添加以下内容

.PHONY: build-api build-agent-hub build-pipeline build-all

build-api:
	go build -o bin/api-service ./cmd/api-service

build-agent-hub:
	go build -o bin/agent-hub ./cmd/agent-hub

build-pipeline:
	go build -o bin/pipeline ./cmd/pipeline

build-all: build-api build-agent-hub build-pipeline

build: build-all

build-single:
	go build -o bin/server ./cmd/server
```

---

### Task 1.2: 创建Agent Hub Service入口

**Files:**
- Create: `backend/cmd/agent-hub/main.go`
- Create: `backend/internal/agent_hub/server.go`
- Create: `backend/internal/agent_hub/config.go`
- Modify: `backend/internal/grpc_server/` (重构现有gRPC服务)

**Step 1: 创建agent-hub主入口和核心服务**

```go
// backend/cmd/agent-hub/main.go
package main

import (
	"log"

	"aegis-system/internal/agent_hub"
	"aegis-system/internal/config"
	"aegis-system/internal/logger"
)

func main() {
	cfg, err := config.Load("agent-hub")
	if err != nil {
		log.Fatal("Failed to load config: ", err)
	}

	if err := logger.Init(cfg.LogLevel); err != nil {
		log.Fatal("Failed to init logger: ", err)
	}

	server := agent_hub.NewServer(cfg)
	if err := server.Run(); err != nil {
		log.Fatal("Agent Hub error: ", err)
	}
}
```

```go
// backend/internal/agent_hub/server.go
package agent_hub

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/types/known/emptypb"

	"aegis-system/internal/config"
	"aegis-system/pkg/api/v1"
	"go.uber.org/zap"
)

type AgentHubServer struct {
	aegisSystem.UnimplementedAgentCommServer
	cfg        *config.Config
	agentMgr   *AgentManager
	eventChan  chan *aegisSystem.RuntimeEvent
}

func NewServer(cfg *config.Config) *AgentHubServer {
	server := &AgentHubServer{
		cfg:        cfg,
		agentMgr:   NewAgentManager(),
		eventChan:  make(chan *aegisSystem.RuntimeEvent, 10000),
	}

	// 启动事件转发协程
	go server.forwardEvents()

	return server
}

func (s *AgentHubServer) Run() error {
	lis, err := net.Listen("tcp", ":19090")
	if err != nil {
		return fmt.Errorf("failed to listen on :19090: %w", err)
	}

	grpcServer := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,
			Timeout:           10 * time.Second,
		}),
	)

	aegisSystem.RegisterAgentCommServer(grpcServer, s)
	zap.Info("Agent Hub starting on :19090")

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			zap.Fatal("gRPC server error: ", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	grpcServer.GracefulStop()
	return nil
}

// gRPC Streaming 实现
func (s *AgentHubServer) AgentStream(stream aegisSystem.AgentComm_AgentStreamServer) error {
	agentID := ""

	defer func() {
		if agentID != "" {
			s.agentMgr.Unregister(agentID)
		}
	}()

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		default:
		}

		in, err := stream.Recv()
		if err != nil {
			s.agentMgr.Unregister(agentID)
			return err
		}

		switch msg := in.Message.(type) {
		case *aegisSystem.AgentMessage_Register:
			agentID = msg.Register.AgentId
			s.agentMgr.Register(agentID, msg.Register)
			if err := stream.Send(&aegisSystem.ServerMessage{
				Message: &aegisSystem.ServerMessage_Ack{
					Ack: &aegisSystem.Ack{Status: "registered"},
				},
			}); err != nil {
				return err
			}

		case *aegisSystem.AgentMessage_Heartbeat:
			s.agentMgr.UpdateHeartbeat(agentID)

		case *aegisSystem.AgentMessage_Event:
			// 转发到Pipeline Service
			select {
			case s.eventChan <- msg.Event:
			default:
				zap.Warn("Event channel full, dropping event")
			}
		}
	}
}

func (s *AgentHubServer) ServerStream(req *emptypb.Empty, server aegisSystem.AgentComm_ServerStreamServer) error {
	// 接收来自后端的命令下发
	return nil
}

// 事件转发到Kafka
func (s *AgentHubServer) forwardEvents() {
	producer := NewKafkaProducer(s.cfg.KafkaBrokers)
	defer producer.Close()

	for event := range s.eventChan {
		if err := producer.SendEvent(event); err != nil {
			zap.Error("Failed to send event to Kafka: ", err)
		}
	}
}
```

---

### Task 1.3: 创建Pipeline Service入口

**Files:**
- Create: `backend/cmd/pipeline/main.go`
- Create: `backend/internal/pipeline_service/server.go`
- Create: `backend/internal/pipeline_service/llm_analyzer.go`

**Step 1: 创建Pipeline Service**

```go
// backend/cmd/pipeline/main.go
package main

import (
	"log"

	"aegis-system/internal/config"
	"aegis-system/internal/logger"
	"aegis-system/internal/pipeline_service"
)

func main() {
	cfg, err := config.Load("pipeline")
	if err != nil {
		log.Fatal("Failed to load config: ", err)
	}

	if err := logger.Init(cfg.LogLevel); err != nil {
		log.Fatal("Failed to init logger: ", err)
	}

	server := pipeline_service.NewServer(cfg)
	if err := server.Run(); err != nil {
		log.Fatal("Pipeline error: ", err)
	}
}
```

```go
// backend/internal/pipeline_service/server.go
package pipeline_service

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"aegis-system/internal/config"
	"aegis-system/internal/llm"
	"aegis-system/internal/pipeline"
	"aegis-system/internal/queue"
	"aegis-system/internal/repository"
	"go.uber.org/zap"
)

type Server struct {
	cfg            *config.Config
	consumer       *queue.KafkaConsumer
	llmClient      *llm.Client
	eventProcessor *pipeline.EventProcessor
}

func NewServer(cfg *config.Config) *Server {
	// 初始化数据库连接
	db, err := repository.NewDB(cfg.DatabaseURL)
	if err != nil {
		zap.Fatal("Failed to connect database: ", err)
	}

	// 初始化LLM客户端
	llmClient, err := llm.NewClient(cfg.LLMConfig)
	if err != nil {
		zap.Fatal("Failed to init LLM client: ", err)
	}

	// 初始化事件处理器
	alertRepo := repository.NewAlertRepo(db)
	blockPolicyRepo := repository.NewBlockPolicyRepo(db)
	runtimeEventRepo := repository.NewRuntimeEventRepo(db)

	eventProcessor := pipeline.NewEventProcessor(
		alertRepo,
		blockPolicyRepo,
		runtimeEventRepo,
		llmClient,
	)

	// 初始化Kafka消费者
	consumer := queue.NewConsumer(
		cfg.KafkaBrokers,
		"pipeline-group",
		[]string{"aegis.security.events"},
		eventProcessor,
	)

	return &Server{
		cfg:            cfg,
		consumer:       consumer,
		llmClient:      llmClient,
		eventProcessor: eventProcessor,
	}
}

func (s *Server) Run() error {
	zap.Info("Pipeline Service starting...")
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动消费
	if err := s.consumer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start consumer: %w", err)
	}

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zap.Info("Shutting down Pipeline Service...")
	cancel()
	s.consumer.Close()

	return nil
}
```

---

## Part 2: Agent本地智能实现

### Task 2.1: 实现SlidingWindowStats统计异常检测

**Files:**
- Create: `agent/internal/intelligence/window_stats.go`
- Create: `agent/internal/intelligence/anomaly_detector.go`
- Modify: `agent/cmd/agent/main.go` (初始化新模块)

**Step 1: 创建滑动窗口统计模块**

```go
// agent/internal/intelligence/window_stats.go
package intelligence

import (
	"sync"
	"time"
)

type SlidingWindowStats struct {
	windowSize   time.Duration
	maxEvents    int
	counters     map[string]*WindowCounter
	mu           sync.RWMutex
}

type WindowCounter struct {
	events    []time.Time
	threshold int
}

func NewSlidingWindowStats(windowSize time.Duration, threshold int) *SlidingWindowStats {
	s := &SlidingWindowStats{
		windowSize: windowSize,
		maxEvents:  threshold,
		counters:   make(map[string]*WindowCounter),
	}

	// 启动清理过期事件的协程
	go s.cleanupLoop()

	return s
}

func (s *SlidingWindowStats) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for key, counter := range s.counters {
			var valid []time.Time
			for _, t := range counter.events {
				if now.Sub(t) < s.windowSize {
					valid = append(valid, t)
				}
			}
			counter.events = valid
			if len(counter.events) == 0 {
				delete(s.counters, key)
			}
		}
		s.mu.Unlock()
	}
}

func (s *SlidingWindowStats) Record(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	counter, ok := s.counters[key]
	if !ok {
		counter = &WindowCounter{
			threshold: s.maxEvents,
		}
		s.counters[key] = counter
	}

	counter.events = append(counter.events, time.Now())
}

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

func (s *SlidingWindowStats) IsAnomalous(key string) bool {
	return s.GetCount(key) >= s.maxEvents
}
```

**Step 2: 创建异常检测器**

```go
// agent/internal/intelligence/anomaly_detector.go
package intelligence

const (
	MaxNormalForkRate     = 10  // 10次/5秒fork
	MaxNormalExecRate     = 50  // 50次/5秒exec  
	MaxNormalNetworkRate  = 20  // 20次/5秒网络调用
	MaxNormalFileRate     = 30  // 30次/5秒文件操作
)

type AnomalyDetector struct {
	forkStats    *SlidingWindowStats
	execStats    *SlidingWindowStats
	networkStats *SlidingWindowStats
	fileStats    *SlidingWindowStats
}

func NewAnomalyDetector() *AnomalyDetector {
	return &AnomalyDetector{
		forkStats:    NewSlidingWindowStats(5*time.Second, MaxNormalForkRate),
		execStats:    NewSlidingWindowStats(5*time.Second, MaxNormalExecRate),
		networkStats: NewSlidingWindowStats(5*time.Second, MaxNormalNetworkRate),
		fileStats:    NewSlidingWindowStats(5*time.Second, MaxNormalFileRate),
	}
}

type AnomalyResult struct {
	IsAnomalous bool
	AnomalyType string
	Severity    int // 1-10
}

func (ad *AnomalyDetector) Detect(event interface{}) AnomalyResult {
	// 类型断言获取事件类型
	// 根据事件类型调用对应的统计检测
	// 返回检测结果
	return AnomalyResult{}
}
```

---

### Task 2.2: 实现PriorityEngine事件优先级引擎

**Files:**
- Create: `agent/internal/intelligence/priority_engine.go`
- Create: `agent/internal/intelligence/decision_engine.go`

```go
// agent/internal/intelligence/priority_engine.go
package intelligence

type PriorityLevel int

const (
	PriorityCRITICAL PriorityLevel = iota
	PriorityHIGH
	PriorityMEDIUM
	PriorityLOW
)

type PriorityRule struct {
	Name     string
	Match    func(Event) bool
	Level    PriorityLevel
	Score    int
}

type PriorityEngine struct {
	rules []PriorityRule
}

func NewPriorityEngine() *PriorityEngine {
	pe := &PriorityEngine{
		rules: []PriorityRule{
			{
				Name:  "mitre_t1059", // 命令行解释器
				Match: func(e Event) bool { return e.MitreID == "T1059" },
				Level: PriorityCRITICAL,
				Score: 100,
			},
			{
				Name:  "mitre_t1053", // 计划任务
				Match: func(e Event) bool { return e.MitreID == "T1053" },
				Level: PriorityHIGH,
				Score: 80,
			},
			{
				Name:  "mitre_t1021", // 远程服务
				Match: func(e Event) bool { return e.MitreID == "T1021" },
				Level: PriorityHIGH,
				Score: 75,
			},
			{
				Name:  "root_exec",
				Match: func(e Event) bool { return e.UID == 0 && e.ProcessName == "/bin/bash" },
				Level: PriorityHIGH,
				Score: 70,
			},
			{
				Name:  "network_c2",
				Match: func(e Event) bool { return e.HasNetworkAccess && e.IsExternalConnection },
				Level: PriorityHIGH,
				Score: 85,
			},
		},
	}
	return pe
}

func (pe *PriorityEngine) Evaluate(event interface{}) PriorityResult {
	result := PriorityResult{Level: PriorityMEDIUM, Score: 0}

	// 如果匹配阻断规则，直接返回CRITICAL
	// 如果匹配任何规则，返回对应优先级

	return result
}

type PriorityResult struct {
	Level    PriorityLevel
	Score    int
	RuleName string
}
```

---

### Task 2.3: 实现DecisionEngine本地决策引擎

**Files:**
- Create: `agent/internal/intelligence/decision_engine.go`

```go
// agent/internal/intelligence/decision_engine.go
package intelligence

type DecisionAction int

const (
	DecisionBLOCK       DecisionAction = iota // 本地阻断
	DecisionREPORT_HIGH                        // 高优先级上报
	DecisionREPORT_FEATURE                     // 特征上报(批量)
	DecisionSKIP                               // 忽略
)

type Decision struct {
	Action           DecisionAction
	Priority         PriorityLevel
	ShouldBlock      bool
	ShouldNotify     bool
	FeatureData      []byte
}

type DecisionEngine struct {
	blockPolicyPath string
	rules           []BlockRule
	whiteList       map[string]bool
	localBlocker    *blocker.Blocker
	communicator    *SmartCommunicator
}

type BlockRule struct {
	MitreID    string
	ExecMatch  string
	Action     string
	Enabled    bool
}

func NewDecisionEngine(cfg *config.Config, blocker *blocker.Blocker, comm *SmartCommunicator) *DecisionEngine {
	de := &DecisionEngine{
		blockPolicyPath: cfg.BlockPolicyDir,
		rules:           make([]BlockRule, 0),
		whiteList:       make(map[string]bool),
		localBlocker:    blocker,
		communicator:    comm,
	}

	// 加载白名单
	de.loadWhiteList()

	// 加载阻断策略
	de.loadBlockRules()

	return de
}

func (de *DecisionEngine) loadWhiteList() {
	// 从配置文件加载白名单进程
	de.whiteList["systemd"] = true
	de.whiteList["sshd"] = true
	de.whiteList["dockerd"] = true
	de.whiteList["containerd"] = true
	// ... 更多系统进程
}

func (de *DecisionEngine) loadBlockRules() {
	// 从远程或本地加载阻断规则
	// 示例规则
	de.rules = []BlockRule{
		{MitreID: "T1059", Action: "block", Enabled: true},  // bash
		{MitreID: "T1053", Action: "block", Enabled: false}, // at
		{MitreID: "T1021", Action: "block", Enabled: false}, // ssh
	}
}

func (de *DecisionEngine) Decide(event interface{}) Decision {
	// 1. 检查白名单
	if de.isWhiteListed(event) {
		return Decision{Action: DecisionSKIP, Priority: PriorityLOW}
	}

	// 2. 检查是否匹配阻断规则
	if blockResult := de.checkBlockRule(event); blockResult.ShouldBlock {
		// 本地阻断
		go de.localBlocker.Block(event)
		return Decision{
			Action:        DecisionBLOCK,
			Priority:      PriorityCRITICAL,
			ShouldBlock:   true,
			ShouldNotify:  true,
		}
	}

	// 3. 检查统计异常
	if anomalyResult := de.checkAnomaly(event); anomalyResult.IsAnomalous {
		return Decision{
			Action:        DecisionREPORT_HIGH,
			Priority:      PriorityHIGH,
			ShouldNotify:  true,
		}
	}

	// 4. 普通事件 - 提取特征后批量上报
	feature := de.extractFeature(event)
	return Decision{
		Action:        DecisionREPORT_FEATURE,
		Priority:      PriorityMEDIUM,
		FeatureData:   feature,
	}
}

func (de *DecisionEngine) isWhiteListed(event interface{}) bool {
	// 检查进程是否在白名单中
	return false
}

func (de *DecisionEngine) checkBlockRule(event interface{}) BlockResult {
	// 检查阻断规则匹配
	return BlockResult{}
}

func (de *DecisionEngine) checkAnomaly(event interface{}) AnomalyResult {
	// 检查统计异常
	return AnomalyResult{}
}

func (de *DecisionEngine) extractFeature(event interface{}) []byte {
	// 提取特征数据用于批量上报
	return nil
}

type BlockResult struct {
	ShouldBlock bool
	RuleName    string
	BlockMethod string // kill_process, kill_parent, quarantine
}
```

---

### Task 2.4: 实现FeatureExtractor特征提取

**Files:**
- Create: `agent/internal/intelligence/feature_extractor.go`

```go
// agent/internal/intelligence/feature_extractor.go
package intelligence

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type EventFeatures struct {
	// 基础特征 (32 bytes)
	EventType   string
	ProcessHash string
	UID         uint32

	// 统计特征 (16 bytes)
	FrequencyScore   float64
	ProcessTreeScore float64

	// 上下文特征 (32 bytes)
	TimeOfDay        int // 0-23
	IsSystemProcess  bool
	HasNetworkAccess bool

	// 原始数据指纹 (32 bytes)
	CommandHash string
}

type FeatureExtractor struct {
	processNameMap map[string]string // 原始名 -> 哈希
}

func NewFeatureExtractor() *FeatureExtractor {
	return &FeatureExtractor{
		processNameMap: make(map[string]string),
	}
}

func (fe *FeatureExtractor) Extract(event interface{}) EventFeatures {
	// 1. 提取进程名哈希
	processHash := fe.hashString(event.GetProcessName())

	// 2. 计算频率得分
	frequencyScore := fe.calcFrequency(event)

	// 3. 计算时间上下文
	timeOfDay := int(time.Now().Unix() % 86400 / 3600)

	// 4. 计算命令哈希
	commandHash := fe.hashString(event.GetCommandLine())

	return EventFeatures{
		EventType:        event.GetEventType(),
		ProcessHash:      processHash,
		UID:              event.GetUID(),
		FrequencyScore:   frequencyScore,
		TimeOfDay:        timeOfDay,
		IsSystemProcess:  event.GetUID() < 1000,
		HasNetworkAccess: event.HasNetworkAccess(),
		CommandHash:      commandHash,
	}
}

func (fe *FeatureExtractor) hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8]) // 只取前8字节，16字符
}

func (fe *FeatureExtractor) calcFrequency(event interface{}) float64 {
	// 基于滑动窗口计算频率得分
	return 0.0
}

// Serialize 序列化为字节数组 (~100 bytes vs 原始2KB)
func (ef *EventFeatures) Serialize() []byte {
	// 实现二进制序列化
	// 使用紧凑格式
	return nil
}
```

---

### Task 2.5: 实现SmartCommunicator智能通信

**Files:**
- Create: `agent/internal/intelligence/smart_communicator.go`
- Create: `agent/internal/intelligence/event_classifier.go`
- Create: `agent/internal/intelligence/batch_aggregator.go`

```go
// agent/internal/intelligence/smart_communicator.go
package intelligence

import (
	"sync"
	"time"
)

type SmartCommunicator struct {
	classifier  *EventClassifier
	aggregator  *BatchAggregator
	compressor  *CompressionEncoder
	client      *client.Client
	eventCh     chan EventFeature
	batchCh     chan []EventFeature
}

func NewSmartCommunicator(client *client.Client) *SmartCommunicator {
	sc := &SmartCommunicator{
		classifier: NewEventClassifier(),
		aggregator: NewBatchAggregator(100, 5*time.Second),
		compressor: NewCompressionEncoder(),
		client:     client,
		eventCh:    make(chan EventFeature, 1000),
		batchCh:    make(chan []EventFeature, 100),
	}

	// 启动各种处理协程
	go sc.priorityForwarder()
	go sc.batchProcessor()

	return sc
}

// 优先级转发 - 根据事件级别选择发送方式
func (sc *SmartCommunicator) priorityForwarder() {
	for feature := range sc.eventCh {
		go sc.sendWithPriority(feature)
	}
}

func (sc *SmartCommunicator) sendWithPriority(feature EventFeature) {
	// CRITICAL: 立即发送
	// HIGH: 1秒内发送
	// MEDIUM: 批量发送 (5秒或100条)
	// LOW: 压缩发送 (30秒或500条)
}

// 批量处理 - 聚合并压缩后发送
func (sc *SmartCommunicator) batchProcessor() {
	for features := range sc.batchCh {
		compressed := sc.compressor.Encode(features)
		sc.client.SendBatch(compressed)
	}
}

type EventClassifier struct{}

func (ec *EventClassifier) Classify(event interface{}) PriorityLevel {
	// 基于MITRE ID、进程名、UID等因素分类
	return PriorityMEDIUM
}
```

```go
// agent/internal/intelligence/batch_aggregator.go
package intelligence

import (
	"sync"
	"time"
)

type BatchAggregator struct {
	batchSize    int
	batchTimeout time.Duration
	buffer       []EventFeature
	mu           sync.Mutex
	timer        *time.Timer
}

func NewBatchAggregator(batchSize int, timeout time.Duration) *BatchAggregator {
	ba := &BatchAggregator{
		batchSize:    batchSize,
		batchTimeout: timeout,
		buffer:       make([]EventFeature, 0, batchSize),
	}
	go ba.flushLoop()
	return ba
}

func (ba *BatchAggregator) Add(feature EventFeature) {
	ba.mu.Lock()
	defer ba.mu.Unlock()

	ba.buffer = append(ba.buffer, feature)

	// 达到批次大小立即发送
	if len(ba.buffer) >= ba.batchSize {
		ba.flush()
	}
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

func (ba *BatchAggregator) flush() {
	// 发送到batchCh由上层处理
	// 清空buffer
	ba.buffer = ba.buffer[:0]
}
```

---

## Part 3: 集成测试与验证

### Task 3.1: 验证Backend微服务拆分

**Step 1: 构建所有服务**

```bash
cd backend
make build-all
# 期望: 成功生成 bin/api-service, bin/agent-hub, bin/pipeline
```

**Step 2: 运行Docker Compose微服务模式**

```bash
docker compose -f docker-compose.microservices.yml up -d
# 期望: 三个服务都启动成功
```

**Step 3: 验证服务间通信**

```bash
# 测试API Service到Agent Hub的gRPC通信
curl http://localhost:8080/health
# 期望: {"status": "ok"}

# 测试Agent Hub gRPC端口
grpc_health_probe -addr=localhost:19090
# 期望: healthy
```

---

### Task 2.6: Agent本地智能集成

**Files:**
- Modify: `agent/cmd/agent/main.go`

```go
// agent/cmd/agent/main.go 集成本地智能模块
func main() {
	// ... 现有初始化代码 ...

	// 新增: 初始化本地智能模块
	windowStats := intelligence.NewSlidingWindowStats(5*time.Second, 50)
	anomalyDetector := intelligence.NewAnomalyDetector()
	priorityEngine := intelligence.NewPriorityEngine()
	featureExtractor := intelligence.NewFeatureExtractor()
	decisionEngine := intelligence.NewDecisionEngine(cfg, blockerInst, nil)
	smartComm := intelligence.NewSmartCommunicator(c)

	// 创建本地智能处理器
	localIntelligence := &intelligence.LocalIntelligence{
		WindowStats:     windowStats,
		AnomalyDetector: anomalyDetector,
		PriorityEngine:  priorityEngine,
		DecisionEngine:  decisionEngine,
		FeatureExtractor: featureExtractor,
		Communicator:    smartComm,
	}

	// 修改pipeline集成本地智能
	pipeline := ebpf.NewPipeline(
		collector,
		ruleLoader,
		c,
		cfg.HostID,
		metrics,
		localIntelligence, // 新增参数
	)

	// ... 现有启动代码 ...
}
```

---

### Task 3.2: 验证Agent智能功能

**Step 1: 构建Agent**

```bash
cd agent
make build
# 期望: 成功生成 agent/dist/aegis-agent
```

**Step 2: 运行Agent**

```bash
# 在测试环境中运行Agent
./agent/dist/aegis-agent -config /etc/aegis-agent/config.toml
# 期望: 正常启动，加载本地智能模块
```

**Step 3: 验证本地决策**

```bash
# 模拟触发阻断规则的事件
# 验证本地阻断生效
```

---

## 注册信息与文件清单

### 创建的新文件

| 文件路径 | 用途 |
|---------|------|
| `backend/cmd/api-service/main.go` | API Service入口 |
| `backend/internal/api_service/server.go` | API Service核心服务 |
| `backend/internal/api_service/config.go` | API Service配置 |
| `backend/cmd/agent-hub/main.go` | Agent Hub入口 |
| `backend/internal/agent_hub/server.go` | Agent Hub核心服务 |
| `backend/internal/agent_hub/agent_manager.go` | Agent管理 |
| `backend/cmd/pipeline/main.go` | Pipeline入口 |
| `backend/internal/pipeline_service/server.go` | Pipeline核心服务 |
| `backend/internal/pipeline_service/llm_analyzer.go` | LLM分析器 |
| `agent/internal/intelligence/window_stats.go` | 滑动窗口统计 |
| `agent/internal/intelligence/anomaly_detector.go` | 异常检测器 |
| `agent/internal/intelligence/priority_engine.go` | 优先级引擎 |
| `agent/internal/intelligence/decision_engine.go` | 决策引擎 |
| `agent/internal/intelligence/feature_extractor.go` | 特征提取器 |
| `agent/internal/intelligence/smart_communicator.go` | 智能通信器 |
| `agent/internal/intelligence/batch_aggregator.go` | 批量聚合器 |

### 修改的文件

| 文件路径 | 修改内容 |
|---------|---------|
| `backend/Makefile` | 添加微服务构建目标 |
| `backend/internal/config/config.go` | 添加服务类型配置 |
| `agent/cmd/agent/main.go` | 集成本地智能模块 |

### 验证命令

```bash
# Backend验证
cd backend && make build-all && docker compose -f docker-compose.microservices.yml up -d

# Agent验证
cd agent && make build

# 集成验证
# 1. curl http://localhost:8080/health
# 2. grpc_health_probe -addr=localhost:19090
```

---

## 实现顺序

1. **Phase 1: Backend微服务拆分** (Task 1.1 → 1.2 → 1.3)
2. **Phase 2: Agent本地智能** (Task 2.1 → 2.2 → 2.3 → 2.4 → 2.5 → 2.6)
3. **Phase 3: 集成测试** (Task 3.1 → 3.2)

**Plan complete and saved to `docs/plans/2026-03-31-aegis-architecture-v6-implementation.md`. Two execution options:**

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

**Which approach?**