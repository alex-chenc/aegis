# Aegis智能主机安全系统 V5.5 后端微服务架构详细设计

**版本**: 5.5
**日期**: 2026-03-30
**状态**: 定稿

---

## 1. 架构设计

### 1.1 服务架构

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              Frontend (Vue 3)                                   │
│                           localhost:8081 (Nginx)                                │
└─────────────────────────────────┬───────────────────────────────────────────────┘
                                  │ HTTP/WebSocket
┌─────────────────────────────────▼───────────────────────────────────────────────┐
│                           API Gateway (Nginx)                                  │
│                              localhost:8080                                     │
└─────────────────────────────────┬───────────────────────────────────────────────┘
                                  │
                                  ↓
┌─────────────────────────────────────────────────────────────────────────────────┐
│                               API Server (Backend)                               │
│                           端口: 8082 (HTTP) / 19093 (gRPC)                      │
│  职责: REST API + WebSocket + 认证授权 + gRPC调用Server                        │
└─────────────────────────────────┬───────────────────────────────────────────────┘
                                  │ gRPC
                                  ↓
┌─────────────────────────────────────────────────────────────────────────────────┐
│                                 Server                                          │
│                    端口: 8083 (HTTP) / 19090 (gRPC)                             │
│  职责: Agent管理(gRPC) + 命令下发 + 事件接收 + Kafka发送                        │
└─────────────────────────────────┬───────────────────────────────────────────────┘
                                  │ gRPC (Agent连接)
                                  ↓
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              Agent (目标主机)                                   │
│                    运行在目标主机上 (资源限制: 1C1G)                            │
└───────────────────────────────────────────────────────────────────────────────┘
                                  ↑ gRPC
                                  │
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           Kafka Cluster                                         │
│                    Topic: aegis.security.events                                 │
└─────────────────────────────────┬───────────────────────────────────────────────┘
                                  │ Kafka Consumer
                                  ↓
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          DC (Data Consumer)                                     │
│                    端口: 19092 (gRPC)                                           │
│  职责: Kafka消费 + 事件聚合 + LLM分析 + 告警生成 + 数据入库                    │
└─────────────────────────────────┬───────────────────────────────────────────────┘
                                  │ PostgreSQL
                                  ↓
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          PostgreSQL                                             │
│                    端口: 5432                                                   │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 服务间通信

| 通信路径 | 协议 | 说明 |
|---------|------|------|
| Frontend ↔ API Server | HTTP/WebSocket | 用户请求 |
| API Server ↔ Server | gRPC | 命令转发、状态查询 |
| Server ↔ Agent | gRPC | Agent连接、命令下发、事件上报 |
| Server ↔ Kafka | Kafka Producer | 事件发送 |
| Kafka ↔ DC | Kafka Consumer | 事件消费 |
| DC ↔ PostgreSQL | PostgreSQL | 数据存储 |

---

## 2. 三个独立服务（各自独立文件夹、独立编译、独立镜像）

### 2.1 API Server

```
api-server/
├── cmd/
│   └── main.go
│
├── internal/
│   ├── server.go              # HTTP服务器启动
│   ├── router.go              # 路由定义
│   │
│   ├── handler/               # HTTP处理器
│   │   ├── config_handler.go
│   │   ├── host_handler.go
│   │   ├── template_handler.go
│   │   ├── task_handler.go
│   │   ├── task_handler_with_healing.go
│   │   ├── agent_handler.go
│   │   ├── rule_handler.go
│   │   ├── vulnerability_handler.go
│   │   ├── detection_handler.go
│   │   └── websocket_handler.go
│   │
│   ├── service/               # 业务服务
│   │   ├── template_service.go
│   │   ├── task_service.go
│   │   ├── self_healing_service.go
│   │   ├── vulnerability_service.go
│   │   ├── custom_cve_service.go
│   │   ├── host_vulnerability_script_service.go
│   │   ├── alert_service.go
│   │   ├── sigma_rule_service.go
│   │   ├── false_positive_service.go
│   │   ├── llm_service.go
│   │   ├── script_generation_service.go
│   │   └── ws_service.go
│   │
│   ├── client/                # gRPC客户端 (调用Server)
│   │   └── server_client.go
│   │
│   ├── middleware/            # 中间件
│   │   ├── cors.go
│   │   ├── request_logger.go
│   │   └── recovery.go
│
│   ├── repository/            # 数据访问层
│   ├── model/                # 数据模型
│   ├── storage/              # 存储层
│   └── llm/                  # LLM客户端
│
├── pkg/
│   ├── api/v1/               # Protobuf生成代码
│   └── logger/
│
├── config/
│   └── api-server.yaml
├── go.mod
└── Makefile
```

### 2.2 Server

```
server/
├── cmd/
│   └── main.go
│
├── internal/
│   ├── server.go              # 主服务启动
│   │
│   ├── grpc_server/          # gRPC服务 (Agent连接)
│   │   └── server.go
│   │
│   ├── agent_registry.go     # Agent注册表
│   ├── heartbeat.go         # 心跳监控
│   ├── command_queue.go    # 命令队列
│   ├── event_processor.go  # 事件处理
│   │
│   ├── kafka_producer.go    # Kafka生产者
│   │   - aegis.security.events
│   │   - aegis.block.commands
│   │   - aegis.rule.updates
│   │
│   ├── client/              # gRPC客户端 (调用API Server)
│   │   └── api_server_client.go
│   │
│   ├── repository/            # 数据访问层
│   ├── model/                # 数据模型
│   └── storage/              # 存储层
│
├── pkg/
│   ├── api/v1/               # Protobuf生成代码
│   └── logger/
│
├── config/
│   └── server.yaml
├── go.mod
└── Makefile
```

### 2.3 DC

```
dc/
├── cmd/
│   └── main.go
│
├── internal/
│   ├── server.go              # 主服务启动
│   │
│   ├── kafka_consumer.go    # Kafka消费者
│   │   - aegis.security.events
│   │
│   ├── event_handler.go     # 事件处理
│   ├── aggregator.go        # 事件聚合 (2分钟窗口)
│   │
│   ├── pipeline/           # 事件处理管道
│   │   ├── host_window_aggregator.go
│   │   ├── llm_prompt_builder.go
│   │   ├── llm_response_parser.go
│   │   └── host_window_aggregator_test.go
│   │
│   ├── llm_analyzer.go     # LLM分析
│   ├── alert_generator.go   # 告警生成
│   ├── block_manager.go     # 阻断管理
│   │
│   ├── client/              # gRPC客户端 (调用API Server)
│   │   └── api_server_client.go
│   │
│   ├── repository/            # 数据访问层
│   ├── model/                # 数据模型
│   └── llm/                  # LLM客户端
│
├── pkg/
│   ├── api/v1/               # Protobuf生成代码
│   └── logger/
│
├── config/
│   └── dc.yaml
├── go.mod
└── Makefile
```

---

## 3. 保留的所有API端点

```yaml
# /api/v1/config/* (4个)
GET    /api/v1/config/llm
POST   /api/v1/config/llm
POST   /api/v1/config/llm/test
GET    /api/v1/config/llm/full-key

# /api/v1/hosts/* (2个)
GET    /api/v1/hosts
GET    /api/v1/hosts/:id

# /api/v1/templates/* (7个)
POST   /api/v1/templates/upload
GET    /api/v1/templates
GET    /api/v1/templates/check-md5
GET    /api/v1/templates/:id/status
GET    /api/v1/templates/:id/rules
POST   /api/v1/templates/:id/generate-scripts
DELETE /api/v1/templates/:id

# /api/v1/rules/* (5个)
GET    /api/v1/rules/:id
GET    /api/v1/rules/:id/has-tasks
POST   /api/v1/rules/:id/scripts/generate
PUT    /api/v1/rules/:id/scripts
DELETE /api/v1/rules/:id

# /api/v1/tasks/* (12个)
GET    /api/v1/tasks
POST   /api/v1/tasks/run-check
POST   /api/v1/tasks/run-fix
GET    /api/v1/tasks/:id/status
GET    /api/v1/tasks/:id/logs
GET    /api/v1/tasks/:id
POST   /api/v1/tasks/:id/redispatch
DELETE /api/v1/tasks/:id
DELETE /api/v1/tasks/group/:id
DELETE /api/v1/tasks/batch
POST   /api/v1/tasks/:id/heal
GET    /api/v1/tasks/:id/healing-status

# /api/v1/agent/* (4个)
GET    /api/v1/agent/install-command
GET    /api/v1/agent/install.sh
GET    /api/v1/agent/uninstall.sh
GET    /api/v1/agent/download

# /api/v1/vulnerability/* (15个)
POST   /api/v1/vulnerability/scan
GET    /api/v1/vulnerability/scan/:scan_id/status
GET    /api/v1/vulnerability
GET    /api/v1/vulnerability/:cve_id/affected-hosts
POST   /api/v1/vulnerability/custom-query
GET    /api/v1/vulnerability/custom-query/:query_id/status
GET    /api/v1/vulnerability/custom-query/current
POST   /api/v1/vulnerability/:cve_id/scripts/generate
GET    /api/v1/vulnerability/:cve_id/host-scripts
POST   /api/v1/vulnerability/:cve_id/scripts/execute
POST   /api/v1/vulnerability/:cve_id/fix
POST   /api/v1/vulnerability/:cve_id/poc
GET    /api/v1/vulnerability/:cve_id/generation-status
GET    /api/v1/vulnerability/:cve_id/task-status
GET    /api/v1/vulnerability/scripts/:script_id/status

# /api/v1/detection/* (40+个)
# alerts (6个), blocks (1个), block-policies (4个), attack-matrix (1个)
# llm (2个), rules (7个), tool-calls (1个), statistics (2个), ws (1个)
```

---

## 4. gRPC通信协议

### 4.1 API Server ↔ Server (gRPC)

```protobuf
service APIServerToServer {
    rpc ForwardCommand(CommandRequest) returns (CommandResponse);
    rpc GetAgentStatus(AgentStatusRequest) returns (AgentStatusResponse);
    rpc ListAgents(ListAgentsRequest) returns (ListAgentsResponse);
    rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);
}
```

### 4.2 Server ↔ Agent (gRPC)

```protobuf
service ServerAgent {
    rpc AgentStream(stream AgentMessage) returns (stream ServerMessage);
    rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);
}
```

---

## 5. 端口分配

| 服务 | HTTP端口 | gRPC端口 | 说明 |
|------|----------|----------|------|
| Frontend | 8081 | - | 前端界面 |
| API Server | 8082 | 19093 | 后端API服务 |
| Server | 8083 | 19090 | 核心服务，Agent连接 |
| DC | - | 19092 | 数据消费者 |

---

## 6. 构建和部署

### 6.1 项目结构

```
/code/ai-benchmark/
├── api-server/          # 独立服务
│   ├── cmd/
│   ├── internal/
│   ├── pkg/
│   ├── config/
│   ├── go.mod
│   └── Makefile
│
├── server/              # 独立服务
│   ├── cmd/
│   ├── internal/
│   ├── pkg/
│   ├── config/
│   ├── go.mod
│   └── Makefile
│
├── dc/                  # 独立服务
│   ├── cmd/
│   ├── internal/
│   ├── pkg/
│   ├── config/
│   ├── go.mod
│   └── Makefile
│
├── frontend/
├── agent/
└── docker-compose.yml
```

### 6.2 Makefile (每个服务独立)

```makefile
# api-server/Makefile
build:
	go build -o bin/api-server ./cmd/...

# server/Makefile
build:
	go build -o bin/server ./cmd/...

# dc/Makefile
build:
	go build -o bin/dc ./cmd/...
```

### 6.3 Docker Compose

```yaml
version: '3.8'

services:
  api-server:
    build: ./api-server
    command: ./bin/api-server
    ports:
      - "8082:8082"
      - "19093:19093"
    depends_on: [postgres, redis]

  server:
    build: ./server
    command: ./bin/server
    ports:
      - "8083:8083"
      - "19090:19090"
    depends_on: [postgres, redis, kafka]

  dc:
    build: ./dc
    command: ./bin/dc
    ports:
      - "19092:19092"
    depends_on: [postgres, kafka]

  postgres:
    image: postgres:15
  redis:
    image: redis:7
  kafka:
    image: confluentinc/cp-kafka:7.5.0
```

---

**文档结束**