# 后端详细设计文档 - V3.0 完整版

**版本**: 3.0
**状态**: 定稿
**作者**: 安全产品团队
**日期**: 2026-03-13

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 3.0 | 2026-03-13 | 安全产品团队 | **新增智能漏洞检查与修复模块**。根据PRD v3.0，新增Vulnerability相关服务、仓库和处理器；定义漏洞扫描、分析和修复的完整后端流程；新增漏洞管理相关的API接口和gRPC扩展。 |
| 2.14 | 2026-03-12 | Sisyphus | 修复任务状态映射与自愈状态清理。 |
| 2.13 | 2026-03-12 | Sisyphus | 任务重新下发与状态映射优化。 |
| 2.12 | 2026-03-12 | Sisyphus | 任务超时与删除API。 |

## 2. 概述

本文档为Aegis智能主机安全系统的后端服务提供全面、可执行的详细设计规范。V3.0版本引入了核心的"智能漏洞检查与修复"功能，对后端架构进行了相应扩展，以支持从软件清单采集、CVE分析到智能修复的全流程业务逻辑。

## 3. 后端项目结构 (V3.0 更新)

为支持新功能，后端项目结构扩展如下：

```
/backend
|-- /internal
|   |-- /api
|   |   |-- /handler
|   |   |   |-- ... (existing handlers)
|   |   |   |-- vulnerability_handler.go # V3.0 新增
|   |-- /service
|   |   |-- ... (existing services)
|   |   |-- vulnerability_service.go   # V3.0 新增
|   |-- /repository
|   |   |-- ... (existing repositories)
|   |   |-- vulnerability_repo.go      # V3.0 新增
|   |-- /model
|   |   |-- ... (existing models)
|   |   |-- vulnerability.go           # V3.0 新增
|-- /pkg/api/v1
|   |-- agent_comm.proto             # V3.0 更新
|-- ... (other directories)
```

## 4. API 接口设计 (`/api/handler`)

在V2.2的API基础上，新增`vulnerability_handler.go`，并定义以下RESTful API接口。

### 4.1 `vulnerability_handler.go`

#### 4.1.1 触发漏洞扫描

-   **Endpoint**: `POST /api/v1/vulnerability/scan`
-   **Request Body**:
    ```json
    {
      "host_ids": ["uuid-1", "uuid-2"]
    }
    ```
-   **Response (202 Accepted)**:
    ```json
    {
      "scan_id": "scan-uuid-123"
    }
    ```
-   **Handler Logic**: `VulnerabilityHandler.StartScan()`
    1.  从请求体中解析`host_ids`。
    2.  调用 `VulnerabilityService.StartScan()` 并传入`host_ids`。
    3.  返回服务层生成的唯一`scan_id`。

#### 4.1.2 获取漏洞扫描状态

-   **Endpoint**: `GET /api/v1/vulnerability/scan/:id/status`
-   **Response (200 OK)**:
    ```json
    {
      "status": "analyzing",
      "progress": 60,
      "message": "正在分析漏洞数据..."
    }
    ```
-   **Handler Logic**: `VulnerabilityHandler.GetScanStatus()`
    1.  从URL参数中获取`scan_id`。
    2.  从Redis缓存中查询扫描状态。
    3.  返回状态信息。

#### 4.1.3 获取漏洞列表

-   **Endpoint**: `GET /api/v1/vulnerability`
-   **Query Params**: `page`, `pageSize`, `severity`, `query`, `date_range`
-   **Response (200 OK)**:
    ```json
    {
      "data": [/* Vulnerability objects */],
      "total": 128
    }
    ```
-   **Handler Logic**: `VulnerabilityHandler.GetVulnerabilities()`
    1.  解析查询参数。
    2.  调用 `VulnerabilityService.ListVulnerabilities()`。
    3.  返回分页和筛选后的漏洞列表。

#### 4.1.4 生成并执行修复脚本

-   **Endpoint**: `POST /api/v1/vulnerability/:id/fix`
-   **Request Body**:
    ```json
    {
      "host_ids": ["uuid-1"]
    }
    ```
-   **Response (200 OK)**:
    ```json
    {
      "task_id": "fix-task-uuid-456"
    }
    ```
-   **Handler Logic**: `VulnerabilityHandler.FixVulnerability()`
    1.  从URL参数获取`cve_id`，从请求体获取`host_ids`。
    2.  调用 `VulnerabilityService.InitiateFix()`。
    3.  返回修复任务的ID。

#### 4.1.5 生成并执行POC验证

-   **Endpoint**: `POST /api/v1/vulnerability/:id/poc`
-   **Request Body**:
    ```json
    {
      "host_id": "uuid-1"
    }
    ```
-   **Response (200 OK)**:
    ```json
    {
      "task_id": "poc-task-uuid-789"
    }
    ```
-   **Handler Logic**: `VulnerabilityHandler.VerifyPoc()`
    1.  调用 `VulnerabilityService.InitiatePocVerification()`。
    2.  返回POC验证任务的ID。

## 5. 业务逻辑层 (`/service`)

新增 `vulnerability_service.go` 作为漏洞管理的核心业务逻辑实现。

### 5.1 `vulnerability_service.go`

#### 5.1.1 `StartScan`

1.  **生成扫描ID**: 创建一个唯一的`scan_id`。
2.  **初始化状态**: 在Redis中为`scan_id`设置初始状态（pending）。
3.  **创建后台任务**: 启动一个Go协程来执行异步扫描流程。
4.  **返回扫描ID**: 立即返回`scan_id`给Handler。

#### 5.1.2 异步扫描流程 (协程内)

1.  **采集软件清单**:
    -   更新Redis状态为`scanning`。
    -   并发地向所有目标主机的Agent发送gRPC请求，调用`CollectSoftwareList` RPC。
    -   收集所有Agent返回的软件列表。
2.  **聚合与存储**:
    -   将收集到的软件清单聚合去重。
    -   调用`VulnerabilityRepository.BatchCreateSoftware()`将软件清单存入`installed_software`表。
3.  **LLM分析**:
    -   更新Redis状态为`analyzing`。
    -   构建CVE分析的Prompt（见AI设计文档）。
    -   调用LLM `ChatCompletion` API。
4.  **结果入库**:
    -   解析LLM返回的JSON数据。
    -   调用`VulnerabilityRepository.BatchCreateVulnerabilities()`，使用`ON CONFLICT DO NOTHING`将新的CVE存入`vulnerabilities`表。
    -   调用`VulnerabilityRepository.BatchCreateHostVulnerabilities()`，将漏洞与主机关联，写入`host_vulnerabilities`表。
5.  **完成**:
    -   更新Redis状态为`completed`。

#### 5.1.3 `InitiateFix` & `InitiatePocVerification`

1.  **获取信息**: 从数据库查询CVE详情和目标主机的OS信息。
2.  **生成脚本**: 构建修复/POC脚本生成的Prompt，调用LLM获取脚本内容。
3.  **安全校验**: 对返回的脚本执行安全校验（危险命令检测等）。
4.  **存储脚本**: 调用`VulnerabilityRepository.CreateFixScript()`或`CreatePocScript()`将脚本存入数据库。
5.  **下发任务**: 调用 `TaskService.CreateAndDispatchTasks()` 将脚本下发给Agent执行。

## 6. 数据访问层 (`/repository`)

新增 `vulnerability_repo.go` 用于与V3.0新增的数据库表进行交互。

### 6.1 `vulnerability_repo.go`

-   `BatchCreateSoftware(...)`: 批量插入软件清单到 `installed_software` 表。
-   `BatchCreateVulnerabilities(...)`: 批量插入CVE信息到 `vulnerabilities` 表，处理冲突。
-   `BatchCreateHostVulnerabilities(...)`: 批量插入主机与漏洞的关联关系到 `host_vulnerabilities` 表。
-   `CreateFixScript(...)`: 插入修复脚本到 `vulnerability_fix_scripts` 表。
-   `CreatePocScript(...)`: 插入POC脚本到 `poc_scripts` 表。
-   `ListVulnerabilities(...)`: 分页和筛选查询漏洞列表，需要联查`vulnerabilities`和`host_vulnerabilities`表。

## 7. gRPC 通信 (`/pkg/api/v1/agent_comm.proto`)

更新`.proto`文件，在`AgentService`中增加新的RPC方法。

```protobuf
// agent_comm.proto

service AgentService {
  // ... (existing RPCs like Register, Heartbeat, ExecuteCommand)
  
  // V3.0 新增: 采集软件清单
  rpc CollectSoftwareList(SoftwareListRequest) returns (SoftwareListResponse);
}

message SoftwareListRequest {
  // 可以包含扫描参数，例如扫描路径
}

message SoftwareListResponse {
  repeated SoftwareInfo software_list = 1;
}

message SoftwareInfo {
  string name = 1;
  string version = 2;
  string package_manager = 3; // "rpm" or "dpkg"
}
```

### 7.1 gRPC 服务器实现更新

在 `grpc_server/server.go` 中实现 `CollectSoftwareList` 方法。该方法在Agent端执行相应的命令（`rpm -qa`或`dpkg-query -W`）并返回结果。

## 8. LLM 交互模块 (`/llm`)

在 `prompts.go` 中新增漏洞管理相关的三个核心Prompt模板：
1.  **CVE分析Prompt**: 输入软件清单，输出结构化的CVE JSON数组。
2.  **漏洞修复脚本生成Prompt**: 输入CVE和OS信息，输出安全的Shell修复脚本。
3.  **POC验证脚本生成Prompt**: 输入CVE信息，输出非破坏性的Shell验证脚本。

这些Prompt的设计需严格遵循`ai_implementation_prompt_v3.0_complete.md`文档。

## 9. 数据流图 (漏洞扫描流程)

```
[Frontend]                                   [Backend]                                            [Agent]
   │                                               │                                                  │
   ├─POST /api/v1/vulnerability/scan─────────────→ │                                                  │
   │      (host_ids)                                │                                                  │
   │                                               ├─(Async) gRPC: CollectSoftwareList()────────────→ │
   │                                               │                                                  ├─Exec("rpm -qa")
   │                                               │                                                  │
   │                                               │ ←───SoftwareListResponse─────────────────────────┤
   │                                               │                                                  │
   │                                               ├─Build Prompt & Call LLM for CVE analysis         │
   │                                               │                                                  │
   │                                               ├─Parse Response & Write to DB                     │
   │                                               │  (vulnerabilities, host_vulnerabilities)         │
   │                                               │                                                  │
   │ ←───202 Accepted (scan_id)                     │                                                  │
   │                                               │                                                  │
   ├─(Poll) GET /api/v1/vulnerability/scan/:id/status→                                                │
   │                                               │                                                  │
   │ ←───200 OK (ScanStatus)                       │                                                  │

```

## 10. 脚本修复状态持久化 (V3.0.3)

### 10.1 Redis 状态存储

脚本修复（Self-Healing）状态持久化到 Redis，确保页面刷新后状态不丢失。

**Redis Key 结构**：
```
Key: healing:status:{task_id}
TTL: 10 分钟（比 5 分钟超时长）
```

**数据结构**：
```go
type HealingStatus struct {
    TaskID         string    `json:"task_id"`
    Status         string    `json:"status"` // healing, healed, failed, timeout
    StartedAt      time.Time `json:"started_at"`
    UpdatedAt      time.Time `json:"updated_at"`
    TotalAttempts  int       `json:"total_attempts"`
    MaxAttempts    int       `json:"max_attempts"`
    LastError      string    `json:"last_error,omitempty"`
    UserSuggestion string    `json:"user_suggestion,omitempty"`
    ScriptType     string    `json:"script_type"`
}
```

### 10.2 超时检查器

后端启动独立的 goroutine 每 30 秒扫描所有修复中的任务，检查是否超过 5 分钟：

```go
func (s *SelfHealingService) timeoutChecker(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // 扫描 healing:status:* 键
            // 如果 status == "healing" && started_at > 5分钟
            // 则更新为 timeout
        }
    }
}
```

### 10.3 GetTaskLogs API 增强

`GetTaskLogs` API 返回每个任务的 `healing_status`，前端无需单独请求：

```go
type TaskLogResponse struct {
    // ... 现有字段
    HealingStatus *HealingStatusResponse `json:"healing_status,omitempty"`
}
```
