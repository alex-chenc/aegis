# Aegis智能主机安全系统 V5.6 架构设计文档

**版本**: 5.6
**日期**: 2026-04-14
**状态**: 设计中

---

## 1. 架构概述

### 1.1 设计目标

V5.6版本在V5.5微服务架构基础上，进行以下核心升级：

| 目标 | 说明 |
|------|------|
| Sigma规则解析 | 支持用户上传Sigma规则文件，系统解析入库后精确下发 |
| LangChain多轮分析 | 引入LangChain模式，AI可进行多轮对话并调用Agent工具 |
| Agent智能体化 | Agent成为真正的智能体，支持工具调用和主动推理 |
| 单Host精确下发 | 所有命令通过host_id精确下发到指定Agent，消除广播模式 |

### 1.2 整体架构图

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              Frontend (Vue 3)                                   │
│                           localhost:8081 (Nginx)                                │
└─────────────────────────────────┬───────────────────────────────────────────────┘
                                  │ HTTP/WebSocket
                                  ↓
┌─────────────────────────────────────────────────────────────────────────────────┐
│                               API Server (Go)                                   │
│                    端口: 8082 (HTTP) / 19093 (gRPC Client)                      │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────────┐  ┌──────────────┐   │
│  │ Handlers│ │ Services │ │   LLM    │ │ gRPC Client      │  │ LangChain   │   │
│  │ (API)   │ │(Business)│ │ Client   │ │ (Server Comm)    │  │ Agent       │   │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────────┘  └──────────────┘   │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────────┐                   │
│  │Repository│ │ MinIO    │ │ Redis    │ │ Kafka Producer   │                   │
│  │(Postgres)│ │(Scripts) │ │ (Cache)  │ │ (Events)         │                   │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────────┘                   │
└─────────────────────────────────┬───────────────────────────────────────────────┘
                                  │ gRPC
                                  ↓
┌─────────────────────────────────────────────────────────────────────────────────┐
│                                 Server (Go)                                     │
│                    端口: 8083 (HTTP) / 19090 (gRPC Agent Hub)                   │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────────────────────┐   │
│  │  Agent       │ │  Command     │ │  Tool Executor                        │   │
│  │  Registry    │ │  Router      │ │  (工具调用路由 - 单Host精确下发)       │   │
│  └──────────────┘ └──────────────┘ └──────────────────────────────────────┘   │
│  ┌──────────────┐ ┌──────────────┐                                               │
│  │ Kafka        │ │ Redis        │                                               │
│  │ Producer     │ │ (Session)    │                                               │
│  └──────────────┘ └──────────────┘                                               │
└─────────────────────────────┬─────────────────────────────────────────────────────┘
                              │ gRPC (Bidirectional Stream)
                              ↓
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              Agent (Go)                                         │
│                   运行在目标主机上 (资源限制: 1C1G)                            │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────────────────────┐   │
│  │  eBPF        │ │  Sigma       │ │  Tool Executor                        │   │
│  │  Collector   │ │  Matcher     │ │  (本地工具执行)                        │   │
│  └──────────────┘ └──────────────┘ └──────────────────────────────────────┘   │
│  ┌──────────────┐ ┌──────────────┐                                               │
│  │  Local       │ │  gRPC        │                                               │
│  │  Intelligence│ │  Client      │                                               │
│  └──────────────┘ └──────────────┘                                               │
└─────────────────────────────────────────────────────────────────────────────────┘
                                  ↑ gRPC
                                  │
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           Kafka Cluster                                         │
│                    Topic: aegis.security.events                                 │
└─────────────────────────────────┬───────────────────────────────────────────────┘
                                  │
                                  ↓
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              DC (Go)                                            │
│                    端口: 19092 (gRPC)                                           │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────────────────────┐   │
│  │  Kafka       │ │  LLM          │ │  Alert Generator                     │   │
│  │  Consumer    │ │  Analyzer     │ │                                      │   │
│  └──────────────┘ └──────────────┘ └──────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ↓
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           PostgreSQL                                            │
│                    端口: 5432                                                   │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. 核心架构变化

### 2.1 LangChain Agent架构

V5.6引入LangChain模式，实现AI多轮分析与Agent工具调用：

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          LangChain Agent (API Server内)                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         LLM (Claude/GPT-4)                           │   │
│  │                      负责对话推理和决策                               │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ↓                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         Memory (上下文管理)                          │   │
│  │  ┌────────────────┐  ┌────────────────┐  ┌────────────────────┐    │   │
│  │  │ Conversation   │  │ Session        │  │ Tool Execution     │    │   │
│  │  │ Memory         │  │ Context        │  │ History            │    │   │
│  │  └────────────────┘  └────────────────┘  └────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ↓                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         Tool Executor                                │   │
│  │  ┌────────────────┐  ┌────────────────┐  ┌────────────────────┐    │   │
│  │  │ GetProcessTree │  │ GetNetwork     │  │ QueryHistorical    │    │   │
│  │  │                │  │ Connections    │  │ Logs               │    │   │
│  │  └────────────────┘  └────────────────┘  └────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ↓                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Server gRPC Client                                │   │
│  │              (通过Server精确调用指定Agent的工具)                     │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 工具调用数据流

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           工具调用完整数据流                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. AI分析请求                                                                │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ User: "分析ALT-003，需要查看进程树"                                │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    │                                          │
│                                    ↓                                          │
│  2. LangChain决定调用工具                                                     │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ LLM决定: 调用 GetProcessTree(host_id, pid)                        │    │
│     │ ToolCall: { call_id: "call_xxx", tool: "GetProcessTree",          │    │
│     │            args: { host_id: "host-123", pid: 12345 } }             │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    │                                          │
│                                    ↓                                          │
│  3. API Server → Server (gRPC)                                                │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ ToolExecuteRequest: {                                             │    │
│     │   call_id: "call_xxx",                                            │    │
│     │   host_id: "host-123",        ← 精确指定目标主机                 │    │
│     │   tool: "GetProcessTree",                                           │    │
│     │   arguments: "{\"pid\":12345}"                                     │    │
│     │ }                                                                  │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    │                                          │
│                                    ↓                                          │
│  4. Server → Agent (gRPC Stream)                                              │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ CommandRequest: {                                                  │    │
│     │   Execute: { host_id: "host-123", script: "#TOOL:GetProcessTree#" }│    │
│     │ }                                                                  │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    │                                          │
│                                    ↓                                          │
│  5. Agent执行工具                                                             │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ 读取 /proc/12345/status                                           │    │
│     │ 读取 /proc/12345/children                                          │    │
│     │ 组装JSON结果                                                       │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    │                                          │
│                                    ↓                                          │
│  6. 返回结果                                                                  │
│     Agent → Server → API Server → LangChain → LLM继续推理                    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. 单Host精确下发机制

### 3.1 问题分析

V5.5及之前版本存在的问题：

| 场景 | V5.5行为 | 问题 |
|------|---------|------|
| `BroadcastRuleUpdate` | `Range`所有连接广播 | 浪费带宽、无差别下发 |
| `UpdateAgentRules` | 无host_id参数，广播 | 无法针对特定主机 |
| 规则审批 | 广播所有Agent | 不需要的规则也被下发 |

### 3.2 V5.6解决方案

**核心原则**: 所有命令下发必须通过`host_id`精确指定目标Agent

```go
// Server层新增方法：精确下发到单Host
func (s *GRPCServer) SendToHost(hostID uuid.UUID, cmd *pb.CommandRequest) error {
    conn, ok := s.agentConnections.Load(hostID)
    if !ok {
        return fmt.Errorf("agent not connected: %s", hostID)
    }
    agentConn := conn.(*AgentConnection)
    return agentConn.Stream.Send(cmd)
}

// 替代BroadcastRuleUpdate
func (s *GRPCServer) SendRuleUpdateToHost(hostID uuid.UUID, update *pb.RuleUpdate) error {
    return s.SendToHost(hostID, &pb.CommandRequest{
        Request: &pb.CommandRequest_RuleUpdate{
            RuleUpdate: &pb.RuleUpdateRequest{
                Action: "incremental",
                Rules:  []*pb.RuleUpdate{update},
            },
        },
    })
}
```

### 3.3 规则精确下发流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        规则精确下发流程                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. 管理员审批规则                                                            │
│     API Server: ruleRepo.UpdateStatus(ruleID, "active")                     │
│                                                                              │
│  2. 确定需要接收规则的主机                                                    │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ 方式A: 根据规则MITRE ID，查找匹配的主机                             │    │
│     │ 方式B: 用户选择特定主机                                            │    │
│     │ 方式C: 全量下发（需要用户确认）                                    │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    │                                          │
│                                    ↓                                          │
│  3. 遍历目标主机，精确下发                                                    │
│     for _, hostID := range targetHosts {                                     │
│         server.SendRuleUpdateToHost(hostID, update)  // 不使用Range广播      │
│     }                                                                         │
│                                                                              │
│  4. Agent接收规则                                                            │
│     Agent收到RuleUpdateRequest，更新本地Sigma规则                            │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 4. Agent工具调用协议

### 4.1 工具列表

| 工具名称 | 参数 | 返回值 | 说明 |
|----------|------|--------|------|
| `GetProcessTree` | `host_id`, `pid` | 进程树JSON | 获取指定进程的完整树状结构 |
| `GetNetworkConnections` | `host_id`, `pid` (可选) | 连接列表 | 获取网络连接情况 |
| `GetOpenFiles` | `host_id`, `pid` | 文件列表 | 获取打开的文件描述符 |
| `GetRunningProcesses` | `host_id`, `filter` (可选) | 进程列表 | 获取运行中的进程 |
| `GetUserSessions` | `host_id` | 会话列表 | 获取登录用户会话 |
| `QueryHistoricalLogs` | `host_id`, `start_time`, `end_time`, `filter` | 日志条目 | 查询历史日志 |

### 4.2 工具执行协议 (V5.6 Callback机制)

**问题**: 传统的Agent→Server单向gRPC流无法直接响应Server→Agent的工具调用请求

**解决方案**: Agent在注册时携带回调端口，启动回调gRPC服务器供Server调用

```protobuf
// agent_comm.proto V5.6新增

// RegisterRequest 新增 callback_port 字段
message RegisterRequest {
    string host_id = 1;
    AssetInfo asset_info = 2;
    string auth_token = 3;
    int32 callback_port = 4;  // V5.6新增: Agent回调端口，默认19095
}

// AgentService 服务定义
service AgentService {
    rpc Register(RegisterRequest) returns (RegisterResponse);
    rpc ExecuteCommand(stream CommandRequest) returns (stream CommandRequest);
    rpc ExecuteTool(ToolRequest) returns (ToolResponse);  // Server通过回调连接调用
    // ...
}

// V5.6新增: 工具请求/响应消息
message ToolRequest {
    string call_id = 1;        // 调用唯一ID
    string host_id = 2;        // 目标主机ID
    string tool = 3;           // 工具名称
    string params_json = 4;   // JSON格式参数 (V5.6改为params_json)
}

message ToolResponse {
    string call_id = 1;
    bool success = 2;
    string result_json = 3;    // JSON格式结果 (V5.6改为result_json)
    string error = 4;
}
```

### 4.3 工具执行流程 (V5.6 Callback模式)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    V5.6 工具执行Callback流程                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. Agent启动时                                                              │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ Agent在注册时携带 callback_port=19095                                │    │
│     │ 启动回调gRPC服务器在19095端口监听                                    │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    │                                          │
│                                    ↓                                          │
│  2. Agent注册                                                                │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ RegisterRequest: {                                                 │    │
│     │   host_id: "host-123",                                             │    │
│     │   callback_port: 19095,                                           │    │
│     │ }                                                                  │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    │                                          │
│                                    ↓                                          │
│  3. Server存储回调地址                                                        │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ callbackPorts[hostID] = 19095                                      │    │
│     │ 创建到 Agent:19095 的gRPC连接                                       │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  4. AI分析工具调用请求                                                        │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ LLM决定: 调用 GetProcessTree(host_id, pid)                          │    │
│     │ ToolCall: { call_id: "call_xxx", tool: "GetProcessTree",          │    │
│     │            args: { host_id: "host-123", pid: 12345 } }             │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    │                                          │
│                                    ↓                                          │
│  5. API Server → Server ExecuteTool (gRPC)                                  │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ ToolExecuteRequest: {                                             │    │
│     │   call_id: "call_xxx",                                            │    │
│     │   host_id: "host-123",        ← 精确指定目标主机                   │    │
│     │   tool: "GetProcessTree",                                           │    │
│     │   arguments: "{\"pid\":12345}"                                     │    │
│     │ }                                                                  │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    │                                          │
│                                    ↓                                          │
│  6. Server → Agent CallbackServer (新建立的gRPC连接)                          │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ ToolRequest: {                                                    │    │
│     │   call_id: "call_xxx",                                            │    │
│     │   host_id: "host-123",                                            │    │
│     │   tool: "GetProcessTree",                                         │    │
│     │   params_json: "{\"pid\":12345}"                                  │    │
│     │ }                                                                  │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    │                                          │
│                                    ↓                                          │
│  7. Agent执行工具并返回结果                                                   │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ ToolResponse: {                                                    │    │
│     │   call_id: "call_xxx",                                            │    │
│     │   success: true,                                                  │    │
│     │   result_json: "{进程树JSON...}"                                   │    │
│     │ }                                                                  │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.4 服务端口映射 (V5.6)

| 服务 | Agent Hub | API Server | Agent回调 |
|------|-----------|------------|-----------|
| Server | 19090 | 19094 | - |
| Agent | client→19090 | - | 19095 (callback) |
| API Server | - | 19093 (client) | - |

### 4.5 Agent工具执行器

```go
// agent/internal/tool_executor.go
type ToolExecutor struct {
    tools map[string]ToolHandler
}

type ToolHandler func(args map[string]interface{}) (interface{}, error)

func NewToolExecutor() *ToolExecutor {
    te := &ToolExecutor{
        tools: make(map[string]ToolHandler),
    }
    te.registerDefaultTools()
    return te
}

func (te *ToolExecutor) registerDefaultTools() {
    te.tools["GetProcessTree"] = te.getProcessTree
    te.tools["GetNetworkConnections"] = te.getNetworkConnections
    te.tools["GetOpenFiles"] = te.getOpenFiles
    te.tools["GetRunningProcesses"] = te.getRunningProcesses
    te.tools["GetUserSessions"] = te.getUserSessions
    te.tools["QueryHistoricalLogs"] = te.queryHistoricalLogs
}

func (te *ToolExecutor) Execute(tool string, args map[string]interface{}) (interface{}, error) {
    handler, ok := te.tools[tool]
    if !ok {
        return nil, fmt.Errorf("unknown tool: %s", tool)
    }
    return handler(args)
}

// GetProcessTree 实现
func (te *ToolExecutor) getProcessTree(args map[string]interface{}) (interface{}, error) {
    pid := int(args["pid"].(float64))  // JSON number转int

    // 读取进程信息
    procPath := fmt.Sprintf("/proc/%d", pid)
    if _, err := os.Stat(procPath); os.IsNotExist(err) {
        return nil, fmt.Errorf("process %d not found", pid)
    }

    // 构建进程树
    tree := te.buildProcessTree(pid)
    return map[string]interface{}{
        "pid":      pid,
        "tree":     tree,
        "captured": time.Now().Unix(),
    }, nil
}
```

---

## 5. 模块依赖关系

### 5.1 依赖图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Frontend                                       │
│                         (无后端依赖)                                         │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ↓
┌─────────────────────────────────────────────────────────────────────────────┐
│                            API Server                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Handler层 → Service层 → Repository层                                │   │
│  │       ↓         ↓           ↓                                       │   │
│  │  Detection   SigmaRule    Postgres                           │   │
│  │  Handler     Service      Repo                                      │   │
│  │       ↓         ↓           ↓                                       │   │
│  │  Alert       LLM          Redis                              │   │
│  │  Service     Service      (Cache)                                  │   │
│  │       ↓         ↓                                               │   │
│  │  LangChain   gRPC                                        │   │
│  │  Agent       Client                                        │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                    ┌───────────────┼───────────────┐
                    ↓               ↓               ↓
              ┌──────────┐   ┌──────────┐   ┌──────────┐
              │  Server  │   │    DC    │   │ Postgres │
              │  (Agent  │   │ (Event   │   │  (Data   │
              │   Hub)   │   │  Filter) │   │  Store)  │
              └──────────┘   └──────────┘   └──────────┘
                    │
                    ↓
              ┌──────────┐
              │  Agent   │
              │(Target   │
              │  Hosts)  │
              └──────────┘
```

### 5.2 API Server新增加载模块

```go
// api-server/internal/llm/langchain/
├── agent.go              // LangChain Agent主类
├── memory.go             // 记忆管理
├── prompt.go             // Prompt模板
├── tools/
│   ├── registry.go       // 工具注册表
│   ├── process_tree.go   // GetProcessTree实现
│   ├── network.go        // GetNetworkConnections实现
│   └── logs.go           // QueryHistoricalLogs实现
└── executor.go           // 工具执行器
```

---

## 6. 端口分配

| 服务 | HTTP端口 | gRPC端口 | 说明 |
|------|----------|----------|------|
| Frontend | 8081 | - | 前端界面 |
| API Server | 8082 | 19093 (client) | 后端API服务 + gRPC客户端(连接Server) |
| Server | 8083 | 19090, 19094 | 19090: Agent Hub; 19094: APIServerToServer |
| DC | - | 19092 | 数据消费者 |
| Agent | - | 19090 (client) | 部署在目标主机，连接Server Agent Hub |

**重要配置说明：**
- Agent 连接到 Server 的 Agent Hub 端口 (**19090**)，不是 API Server 的端口
- API Server 的 `agent_hub_port` 配置项指定了 Agent Hub 端口，用于生成安装脚本

---

## 7. 环境变量配置

### 7.1 docker-compose 环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|---------|
| `EXTERNAL_IP` | 外部可访问IP，用于Agent安装脚本 | - |
| `DB_PASSWORD` | PostgreSQL密码 | a_strong_db_password |
| `REDIS_PASSWORD` | Redis密码 | a_strong_redis_password |
| `AGENT_TOKEN` | Agent认证Token | aegis-agent-token |

### 7.2 Agent 安装脚本IP配置

Agent 安装脚本中的地址由 `EXTERNAL_IP` 环境变量控制：

```
SERVER_ADDR="${EXTERNAL_IP}:8082"   # API Server HTTP端口
GRPC_ADDR="${EXTERNAL_IP}:19090"    # Server Agent Hub端口 (agent_hub_port)
```

**注意**：Agent 需要连接到 Server 服务的 Agent Hub 端口 (19090)，而不是 API Server 的 gRPC 端口 (19093)。

---

## 8. 性能指标

| 指标 | V5.5 | V5.6 | 变化 |
|------|------|------|------|
| 规则下发方式 | 广播所有Agent | 精确单Host下发 | 带宽降低90%+ |
| AI降噪 | 单轮分析 | 多轮对话+工具调用 | 分析深度提升 |
| 工具调用延迟 | N/A | <500ms | 新增能力 |
| Agent内存占用 | <100MB | <150MB | 工具执行器增量 |

---

**文档结束**
