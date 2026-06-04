# V5.8 后端设计: 智能资产采集

**版本**: 5.8  
**日期**: 2026-06-04  
**状态**: 设计中  

---

## 1. 目标

后端负责智能资产采集的控制面能力：

- 触发 Agent 采集软件包和进程快照。
- 保存原始采集快照和归一化资产。
- 调用 LLM 识别应用、分类和版本。
- 在 LLM 需要版本证据时，通过现有 `ExecuteTool` 通道调用 Agent 只读工具。
- 提供前端列表、详情、任务和人工复核 API。

---

## 2. 组件边界

| 组件 | 职责 |
|:---|:---|
| `api-server` | HTTP API、任务编排、LLM 分析、入库、查询 |
| `server` | gRPC Agent Hub，转发采集和工具调用 |
| `agent` | 软件包解析、进程快照、版本工具执行 |
| `postgres` | 资产、任务、快照、AI 证据存储 |
| `redis` | 任务锁、短期进度、去重 key |

---

## 3. 数据流

```text
前端手动触发
  -> api-server 创建 asset_collection_tasks
  -> api-server 调用 server gRPC
  -> server 转发到在线 agent
  -> agent 采集 package inventory + process snapshot
  -> server 返回采集 JSON
  -> api-server 保存 raw snapshot
  -> api-server 归一化软件资产
  -> api-server 构建 LLM 应用识别上下文
  -> LLM 输出应用分类和版本需求
  -> api-server 通过 ExecuteTool 调用 Agent 版本工具
  -> LLM 或规则合并版本证据
  -> api-server upsert application assets
  -> 前端列表查询
```

周期采集由 api-server 内部 worker 触发，流程相同。

---

## 4. 服务设计

新增服务：

```text
api-server/internal/service/asset_collection_service.go
api-server/internal/service/asset_analysis_service.go
api-server/internal/service/asset_query_service.go
```

### 4.1 AssetCollectionService

职责：

- 创建采集任务。
- 按主机在线状态拆分任务。
- 调用 server gRPC 采集。
- 保存原始快照。
- 调用归一化逻辑。
- 推进任务状态。

关键方法：

```go
type AssetCollectionService interface {
    Trigger(ctx context.Context, req TriggerAssetCollectionRequest) (*AssetCollectionTask, error)
    CollectHost(ctx context.Context, taskID uuid.UUID, hostID uuid.UUID, options CollectOptions) error
    RetryFailed(ctx context.Context, taskID uuid.UUID) error
    Cancel(ctx context.Context, taskID uuid.UUID) error
}
```

### 4.2 AssetAnalysisService

职责：

- 构建 LLM prompt。
- 管理 LLM 工具调用。
- 校验 LLM JSON 输出。
- 将 AI 输出和工具证据合并为资产。

关键方法：

```go
type AssetAnalysisService interface {
    AnalyzeHostApplications(ctx context.Context, snapshot HostProcessSnapshot) (*ApplicationAnalysisResult, error)
    ExecuteVersionTool(ctx context.Context, hostID uuid.UUID, tool AssetToolCall) (*AssetToolResult, error)
    UpsertApplications(ctx context.Context, result ApplicationAnalysisResult) error
}
```

### 4.3 AssetQueryService

职责：

- 提供软件资产、应用资产、分类资产查询。
- 拼接主机字段：hostname、ip、group、os。
- 处理分页、筛选、导出。

---

## 5. HTTP API

统一前缀：

```text
/api/v1/host-assets
```

### 5.1 概览

```http
GET /api/v1/host-assets/summary
```

Query：

| 参数 | 说明 |
|:---|:---|
| `host_id` | 可选 |
| `group_id` | 可选 |
| `start_time` | 可选 |
| `end_time` | 可选 |

Response：

```json
{
  "code": 0,
  "data": {
    "software_count": 1203,
    "application_count": 82,
    "database_count": 5,
    "web_service_count": 12,
    "web_framework_count": 18,
    "web_site_count": 9,
    "needs_review_count": 4,
    "last_collection_at": "2026-06-04T11:50:28+08:00"
  }
}
```

### 5.2 软件列表

```http
GET /api/v1/host-assets/software
```

Query：

- `page`
- `page_size`
- `keyword`
- `host_id`
- `group_id`
- `os_type`
- `package_manager`
- `status`
- `start_time`
- `end_time`

Response data：

```json
{
  "items": [
    {
      "id": "uuid",
      "host_id": "uuid",
      "hostname": "hbp-cwpp-metaserver-test01",
      "ip_address": "10.16.140.167",
      "group_name": "默认分组",
      "os_type": "linux",
      "name": "mariadb",
      "version": "10.3.35",
      "package_manager": "rpm",
      "architecture": "x86_64",
      "install_paths": ["/usr/bin/mysql", "/usr/bin/mysqldump"],
      "last_modified_at": "2026-05-20T09:38:41+08:00",
      "collected_at": "2026-06-04T11:50:28+08:00"
    }
  ],
  "total": 1
}
```

### 5.3 应用列表

```http
GET /api/v1/host-assets/applications
```

Query：

- `category`: `database|web_service|web_framework|web_site|other`
- `keyword`
- `host_id`
- `group_id`
- `min_confidence`
- `review_status`
- `status`
- `page`
- `page_size`

Response data：

```json
{
  "items": [
    {
      "id": "uuid",
      "host_id": "uuid",
      "hostname": "hbp-cwpp-metaserver-test01",
      "ip_address": "10.16.140.167",
      "group_name": "默认分组",
      "os_type": "linux",
      "name": "Jar",
      "category": "web_service",
      "version": "unknown",
      "listen_ports": [34068],
      "run_user": "root",
      "start_path": "/home/app/service",
      "config_paths": [],
      "confidence": 0.82,
      "review_status": "pending",
      "status": "active",
      "collected_at": "2026-06-04T11:50:27+08:00"
    }
  ],
  "total": 1
}
```

### 5.4 应用详情

```http
GET /api/v1/host-assets/applications/:id
```

返回：

- 应用基础信息。
- 关联进程。
- 关联软件包。
- AI 输出。
- 工具调用记录。
- 证据列表。

### 5.5 人工复核

```http
PUT /api/v1/host-assets/applications/:id/review
```

Request：

```json
{
  "name": "Spring Boot App",
  "category": "web_service",
  "version": "2.7.18",
  "install_path": "/home/app/service.jar",
  "config_paths": ["/home/app/application.yml"],
  "review_status": "confirmed"
}
```

规则：

- 人工复核字段写入 `manual_overrides`。
- 后续自动采集不得覆盖人工字段，除非用户取消确认。

### 5.6 触发采集

```http
POST /api/v1/host-assets/collections
```

Request：

```json
{
  "scope": "hosts",
  "host_ids": ["uuid"],
  "types": ["software", "process", "application_analysis"],
  "force": false
}
```

Response：

```json
{
  "code": 0,
  "data": {
    "task_id": "uuid",
    "status": "pending"
  }
}
```

### 5.7 采集任务

```http
GET /api/v1/host-assets/collections
GET /api/v1/host-assets/collections/:id
POST /api/v1/host-assets/collections/:id/retry
POST /api/v1/host-assets/collections/:id/cancel
```

### 5.8 周期采集配置

```http
GET /api/v1/host-assets/collection-config
PUT /api/v1/host-assets/collection-config
```

默认配置：

```json
{
  "enabled": true,
  "interval_hours": 12,
  "collect_types": ["software", "process", "application_analysis"],
  "scope": "all_hosts",
  "next_run_at": "2026-06-04T23:50:28+08:00"
}
```

更新请求：

```json
{
  "enabled": true,
  "interval_hours": 12,
  "collect_types": ["software", "process", "application_analysis"]
}
```

校验规则：

- `interval_hours` 范围为 1 到 168。
- 默认值为 12。
- 保存配置不立即创建采集任务。
- `enabled=false` 时清空或暂停下一次计划执行。
- 周期 worker 启动时如果配置不存在，自动创建默认配置。

---

## 6. gRPC 扩展

现有 `CollectSoftware` 只返回软件 JSON，V5.8 建议扩展为资产采集请求。

### 6.1 API Server -> Server

新增：

```proto
rpc CollectHostAssets(CollectHostAssetsRequest) returns (CollectHostAssetsResponse);

message CollectHostAssetsRequest {
  string task_id = 1;
  string host_id = 2;
  repeated string collect_types = 3; // software, process
  int32 timeout_seconds = 4;
}

message CollectHostAssetsResponse {
  bool success = 1;
  string host_id = 2;
  string snapshot_json = 3;
  string error = 4;
  int64 collected_at = 5;
}
```

兼容策略：

- 保留 `CollectSoftware`，作为只采集软件的旧接口。
- 新前端和新服务使用 `CollectHostAssets`。

### 6.2 Server -> Agent

新增：

```proto
rpc CollectHostAssets(HostAssetCollectRequest) returns (HostAssetCollectResponse);

message HostAssetCollectRequest {
  repeated string collect_types = 1;
  bool include_package_files = 2;
  bool include_listen_ports = 3;
  int32 max_process_count = 4;
}

message HostAssetCollectResponse {
  bool success = 1;
  string snapshot_json = 2;
  string error = 3;
}
```

---

## 7. LLM 编排

### 7.1 Prompt 输入

输入必须经过裁剪：

- 软件包只传与进程路径可能相关的包，以及数据库/Web 服务候选包。
- 进程按服务聚合后传入，不传完整环境变量。
- `cmdline` 中疑似 token、password、secret 的参数做脱敏。

### 7.2 工具调用

允许工具：

| 工具 | 用途 |
|:---|:---|
| `AssetGetProcessVersion` | 对指定 pid/exe 尝试获取版本 |
| `AssetReadConfigSummary` | 读取配置文件摘要，不返回完整敏感内容 |
| `AssetListDirectoryHints` | 列出项目目录关键文件名 |
| `AssetResolvePackageByFile` | 通过文件路径匹配软件包 |

所有工具调用走：

```text
api-server AssetAnalysisService
  -> server ExecuteTool
  -> agent ToolManager
```

### 7.3 输出校验

LLM 输出必须满足：

- JSON schema 校验通过。
- `category` 只能是允许枚举。
- `confidence` 在 0 到 1。
- `related_pids` 必须来自原始快照。
- `version` 如来自工具调用，需要引用 tool_call_id。

---

## 8. 漏洞扫描策略变更

V5.8 起，漏洞扫描不再调用 Agent 实时执行软件包采集或进程采集。漏洞扫描服务只读取资产库中的软件资产和应用资产，并将资产上下文传给 LLM 做漏洞匹配和影响面解释。

### 8.1 服务改造

涉及服务：

```text
api-server/internal/service/vulnerability_service.go
api-server/internal/service/asset_query_service.go
api-server/internal/llm/prompts.go
api-server/internal/repository/vulnerability_repo.go
```

改造点：

- 移除或停用扫描流程中的 `CollectSoftware` / `CollectSoftwareList` 调用。
- 扫描输入改为查询 `host_software_assets` 和 `host_application_assets`。
- 每条扫描结果必须关联 `software_asset_id` 或 `application_asset_id`。
- 对资产版本为空的记录，允许 LLM 输出“无法判断”，不允许补造版本。
- 资产数据超过配置周期 2 倍未更新时，扫描接口返回 stale 提示，但不自动触发 Agent 采集。

### 8.2 漏洞真实性校验

LLM 不允许创造漏洞。后端必须校验正式漏洞结果：

| 校验项 | 要求 |
|:---|:---|
| 漏洞编号 | 必须有 `cve_id` 或 advisory id |
| 来源 | 必须有 NVD、CNVD、CNNVD、厂商公告、GitHub Security Advisory、发行版公告或人工录入来源 |
| 资产关联 | 必须关联软件资产或应用资产 |
| 版本依据 | 必须引用资产版本、工具证据或人工复核字段 |
| 置信度 | 低于阈值进入待复核，不进入正式漏洞 |

无法校验真实性的结果：

- 不写入正式 `vulnerabilities`。
- 可写入候选风险表或任务详情的 `rejected_candidates`。
- 前端展示为“待复核风险”，不得计入漏洞总数。

### 8.3 LLM 输出 schema

```json
{
  "matches": [
    {
      "asset_type": "software",
      "asset_id": "uuid",
      "asset_name": "openssl",
      "asset_version": "1.1.1k",
      "vulnerability_id": "CVE-2023-0286",
      "source": {
        "type": "nvd",
        "url": "https://nvd.nist.gov/vuln/detail/CVE-2023-0286",
        "published_at": "2023-02-08"
      },
      "match_reason": "installed version is within affected range",
      "confidence": 0.92
    }
  ],
  "rejected_candidates": [
    {
      "asset_id": "uuid",
      "reason": "missing public advisory source"
    }
  ]
}
```

### 8.4 禁止行为

- 禁止 LLM 生成不存在的 CVE 编号。
- 禁止仅凭“版本较旧”创建漏洞。
- 禁止扫描过程中调用 Agent 采集软件或进程。
- 禁止把 `potential_risk` 当成正式漏洞。

---

## 9. 安全设计

- Agent 工具只读，不允许任意命令。
- 版本命令通过内置模板执行，例如 `nginx -v`、`postgres --version`，不接受 LLM 拼接 shell。
- `cmdline` 脱敏后再发送给 LLM。
- 原始快照中的路径、命令行和配置摘要按保留策略清理。
- 所有人工复核和手动采集写审计日志。
- 漏洞扫描结果必须经过真实来源校验后才能进入正式漏洞表。

---

## 10. 测试设计

| 测试 | 内容 |
|:---|:---|
| Handler 单测 | 列表筛选、分页、触发采集参数校验 |
| Service 单测 | 任务状态流转、Agent 离线、重试失败主机 |
| LLM 输出校验测试 | 非法分类、非法 PID、缺失字段被拒绝 |
| Tool 编排测试 | LLM 请求版本时只调用允许工具 |
| Repository 测试 | upsert 软件和应用资产不产生重复 |
| gRPC 集成测试 | CollectHostAssets 正常返回和超时失败 |
| 周期配置测试 | 默认 12 小时，1 到 168 小时范围校验 |
| 漏洞扫描回归测试 | 扫描不调用 Agent 采集接口 |
| 漏洞真实性测试 | 无来源或编造 CVE 被拒绝入正式漏洞 |

---

## 11. 回滚方案

- 数据库迁移可保留新表，不影响旧模块。
- 前端菜单可隐藏智能资产采集入口。
- 周期任务配置关闭后不再触发采集。
- Agent 新工具不被调用时不影响现有 eBPF、Sigma 和命令执行能力。
- 漏洞扫描可临时回退到旧扫描流程，但回退期间必须在页面提示扫描策略已降级。
