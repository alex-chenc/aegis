# 通讯层设计文档 - V1.6 完整版

**版本**: 1.6
**状态**: 定稿
**作者**: Manus AI

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 1.6 | 2026-03-05 | Manus AI | **完整重写**。确保文档独立、完整，包含完整的 gRPC Protobuf 定义和所有 RESTful API 的详细 JSON Schema，移除所有外部引用。 |
| 1.5 | 2026-03-05 | Manus AI | 补全 Agent gRPC 通讯，移除 WebSocket，新增 Agent 安装接口。 |

## 2. 概述

本文档定义了系统内所有组件间的通讯协议，分为两大部分：

1.  **gRPC 通讯协议**: 用于后端 (Backend) 与 Agent 之间的高性能、双向流式通讯，承载了 Agent 注册、心跳维持、命令下发和结果上报等核心控制流。
2.  **HTTP/RESTful API**: 用于前端 (Frontend) 与后端 (Backend) 之间的业务数据交互，遵循标准的无状态请求-响应模式。

## 3. gRPC 通讯协议 (后端 <-> Agent)

### 3.1 Protobuf 完整定义 (`agent_comm.proto`)

```protobuf
syntax = "proto3";

package agent_comm;

import "google/protobuf/timestamp.proto";

option go_package = "./agent_comm";

// AgentService 定义 Agent 与服务端之间的 gRPC 服务
service AgentService {
  // Register 建立双向流通道用于持续通讯。Agent 在建立连接后，
  // 必须发送第一条包含 AssetInfo 的消息进行注册。
  rpc Register(stream AgentMessage) returns (stream ServerMessage);
}

// ServerMessage 定义服务端下发给 Agent 的消息信封
message ServerMessage {
  string message_id = 1; // 消息唯一ID (UUID)
  google.protobuf.Timestamp timestamp = 2; // 消息发送时间

  oneof payload {
    ServerCommand command = 3; // 下发指令
    HeartbeatResponse heartbeat_response = 4; // 心跳响应
  }
}

// AgentMessage 定义 Agent 上报给服务端的消息信封
message AgentMessage {
  string host_id = 1;    // Agent 所在主机ID (由 Agent 本地生成并持久化)
  string message_id = 2; // 消息唯一ID (UUID)
  google.protobuf.Timestamp timestamp = 3; // 消息发送时间

  oneof payload {
    AssetInfo asset_info = 4; // 资产信息 (仅在首次连接时发送)
    HeartbeatRequest heartbeat_request = 5; // 心跳请求
    CommandResult command_result = 6; // 指令执行结果
  }
}

// AssetInfo 定义 Agent 首次上报的资产信息 (精简版)
message AssetInfo {
  string ip_address = 1;
  string hostname = 2;
  string os_type = 3;
  string agent_version = 4;
}

// HeartbeatRequest 定义 Agent 定时上报的心跳信息 (空消息，仅用于保活)
message HeartbeatRequest {}

// HeartbeatResponse 定义服务端对心跳的响应 (空消息)
message HeartbeatResponse {}

// ServerCommand 定义服务端下发的指令
message ServerCommand {
  string task_id = 1; // 任务唯一ID
  string script = 2; // 要执行的脚本内容
  int64 timeout_seconds = 3; // 执行超时时间（秒）
}

// CommandResult 定义 Agent 上报的指令执行结果
message CommandResult {
  string task_id = 1; // 对应的任务ID
  int32 exit_code = 2; // 脚本退出码
  string stdout = 3; // 标准输出
  string stderr = 4; // 标准错误
  bool timed_out = 5; // 是否超时
}
```

## 4. HTTP/RESTful API (前端 <-> 后端)

### 4.1 通用约定

*   **Base URL**: `/api/v1`
*   **认证**: 部分接口需要通过 HTTP Header `Authorization: Bearer <token>` 传递认证令牌。
*   **成功响应**: `200 OK` 或 `201 Created`。
*   **错误响应**: `4xx` 或 `5xx`，响应体格式如下：
    ```json
    {
      "error": {
        "code": "ERROR_CODE",
        "message": "A human-readable error message."
      }
    }
    ```

### 4.2 API 接口详述

#### 4.2.1 系统配置 (Settings)

*   **`GET /api/v1/config/llm`**
    *   **描述**: 获取当前的大模型配置。
    *   **响应体 (200 OK)**:
        ```json
        {
          "api_key": "sk-xxxx...1234",
          "base_url": "https://dashscope.aliyuncs.com/compatible-mode/v1"
        }
        ```

*   **`POST /api/v1/config/llm`**
    *   **描述**: 更新大模型配置。
    *   **请求体**:
        ```json
        {
          "api_key": "sk-real-api-key",
          "base_url": "https://dashscope.aliyuncs.com/compatible-mode/v1"
        }
        ```
    *   **响应体 (200 OK)**: (同 `GET` 响应体，API Key 脱敏)

*   **`POST /api/v1/config/llm/test`**
    *   **描述**: 测试大模型配置的连通性。
    *   **请求体**: (同 `POST /api/v1/config/llm` 请求体)
    *   **响应体 (200 OK)**:
        ```json
        {
          "status": "ok",
          "message": "Connection successful."
        }
        ```

#### 4.2.2 Agent 安装与分发

*   **`GET /api/v1/agent/install.sh`**
    *   **描述**: 动态生成并返回一键安装脚本。
    *   **Query Parameters**: `token` (string, required): Agent 认证 Token。
    *   **响应体 (200 OK)**: `Content-Type: text/plain`，内容为 Shell 脚本。

*   **`GET /api/v1/agent/download`**
    *   **描述**: 获取 Agent 二进制文件的预签名下载链接。
    *   **Query Parameters**: `os` (string, required), `arch` (string, required)。
    *   **响应体 (200 OK)**:
        ```json
        {
          "download_url": "http://minio:9000/agent-artifacts/baseline-agent-linux-amd64?X-Amz-Algorithm=..."
        }
        ```

#### 4.2.3 资产管理 (Hosts)

*   **`GET /api/v1/hosts`**
    *   **描述**: 获取主机列表，支持搜索和分页。
    *   **Query Parameters**: `page` (int), `pageSize` (int), `query` (string)。
    *   **响应体 (200 OK)**:
        ```json
        {
          "items": [
            {
              "id": "uuid-1",
              "status": "online",
              "ip_address": "192.168.1.10",
              "hostname": "web-server-01",
              "os_type": "linux",
              "agent_version": "v1.6.0",
              "last_heartbeat_at": "2026-03-05T14:30:00Z"
            }
          ],
          "total": 1
        }
        ```

#### 4.2.4 模板与规则 (Templates & Rules)

*   **`POST /api/v1/templates/upload`**
    *   **描述**: 上传基线模板文件。
    *   **请求体**: `Content-Type: multipart/form-data`，包含文件本身。
    *   **响应体 (201 Created)**:
        ```json
        {
          "id": "uuid-template-1",
          "name": "CIS_Ubuntu_Linux_22.04_LTS_Benchmark_v1.0.0.pdf",
          "status": "parsing"
        }
        ```

*   **`GET /api/v1/templates`**
    *   **描述**: 获取所有已上传的模板列表。
    *   **响应体 (200 OK)**: (结构为包含多个 `POST /upload` 响应体中对象的数组)

*   **`GET /api/v1/templates/{id}/rules`**
    *   **描述**: 获取指定模板解析出的所有规则。
    *   **响应体 (200 OK)**:
        ```json
        {
          "items": [
            {
              "id": "uuid-rule-1",
              "title": "Ensure password creation requirements are configured",
              "check_content": "...",
              "fix_content": "..."
            }
          ]
        }
        ```

#### 4.2.5 任务执行 (Tasks)

*   **`POST /api/v1/tasks/run-check`**
    *   **描述**: 下发检查任务。
    *   **请求体**:
        ```json
        {
          "rule_id": "uuid-rule-1",
          "host_ids": ["uuid-host-1", "uuid-host-2"]
        }
        ```
    *   **响应体 (202 Accepted)**:
        ```json
        {
          "task_group_id": "uuid-task-group-1"
        }
        ```

*   **`GET /api/v1/tasks/{group_id}/logs`**
    *   **描述**: 获取任务组的执行日志。
    *   **响应体 (200 OK)**:
        ```json
        {
          "logs_by_host": {
            "uuid-host-1": [
              { "timestamp": "...", "stream": "stdout", "line": "Running check..." }
            ]
          }
        }
        ```

#### 4.2.6 健康检查

*   **`GET /health`**
    *   **描述**: 用于 Docker 和负载均衡器的健康检查端点。
    *   **响应体 (200 OK)**:
        ```json
        {
          "status": "healthy"
        }
        ```
