# V5.8 API 与 gRPC 设计: 动态 eBPF DetectionPackage

**版本**: 5.8  
**日期**: 2026-05-22  
**状态**: 设计中

---

## 1. API 设计原则

- HTTP API 面向前端页面和管理操作。
- API Server 负责 package 元数据、构建任务、签名发布、启用状态和 MinIO 对象管理。
- Builder 负责动态 eBPF 插件构建、打包、签名，只允许 API Server 通过内部 gRPC 调用。
- Server 负责把安装、卸载、配置同步命令转发给在线 agent。
- Agent 通过现有双向命令流接收动态 eBPF 指令，通过 `ReportEvent` 上报告警和状态。
- 大包不通过 gRPC 直接传输，只下发 URL。

---

## 2. HTTP API 路由

新增路由组：

```text
/api/v1/detection/packages
/api/v1/settings/ebpf-hooks
```

---

## 3. DetectionPackage API

### 3.1 列表

```http
GET /api/v1/detection/packages?page=1&page_size=20&status=enabled&search=copyfail
```

响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "data": [
      {
        "package_id": "cve-2026-31431-copyfail",
        "version": "1.0.0",
        "title": "CVE-2026-31431 CopyFail Runtime Detector",
        "status": "enabled",
        "cve_ids": ["CVE-2026-31431"],
        "hook_count": 3,
        "host_total": 100,
        "host_active": 95,
        "host_failed": 5,
        "updated_at": "2026-05-22T00:00:00Z"
      }
    ],
    "total": 1
  }
}
```

### 3.2 获取详情

```http
GET /api/v1/detection/packages/:package_id?version=1.0.0
```

返回：

- package 元数据
- build 摘要
- hook summary
- event schema
- Sigma rule 摘要
- Correlation 摘要
- host status 统计

### 3.3 AI 生成草稿

```http
POST /api/v1/detection/packages/ai-generate
```

请求：

```json
{
  "cve_id": "CVE-2026-31431",
  "vulnerability_summary": "AF_ALG AEAD + splice page cache corruption local privilege escalation",
  "target_os": ["linux"],
  "preferred_package_id": "cve-2026-31431-copyfail",
  "notes": "Use tracepoint only in first version."
}
```

响应：

```json
{
  "code": 0,
  "data": {
    "draft_id": "uuid",
    "package_id": "cve-2026-31431-copyfail",
    "target_version": "1.0.0",
    "hook_plan_yaml": "...",
    "sigma_rules_yaml": "...",
    "correlation_yaml": "...",
    "ebpf_source": "..."
  }
}
```

### 3.4 创建草稿

```http
POST /api/v1/detection/packages/drafts
```

请求：

```json
{
  "package_id": "cve-2026-31431-copyfail",
  "target_version": "1.0.0",
  "title": "CVE-2026-31431 CopyFail Runtime Detector",
  "description": "...",
  "cve_ids": ["CVE-2026-31431"],
  "hook_plan_yaml": "...",
  "ebpf_source": "...",
  "sigma_rules_yaml": "...",
  "correlation_yaml": "..."
}
```

### 3.5 更新草稿

```http
PUT /api/v1/detection/packages/drafts/:draft_id
```

第一版直接覆盖草稿，不保存 revision。

### 3.6 提交构建

```http
POST /api/v1/detection/packages/:package_id/build
```

请求：

```json
{
  "draft_id": "uuid",
  "version": "1.0.0",
  "builder_profile": "aegis-agent-builder-ubi8"
}
```

响应：

```json
{
  "code": 0,
  "data": {
    "build_id": "uuid",
    "status": "pending"
  }
}
```

说明：

- API Server 创建 build 记录后调用 builder gRPC `StartPackageBuild`。
- Builder 执行编译、打包和构建校验，上传 unsigned package 与构建日志到 MinIO。

### 3.7 获取构建状态

```http
GET /api/v1/detection/packages/builds/:build_id
```

返回：

- status
- build_log
- artifacts
- hooks
- event_schema
- builder_digest
- error_message

### 3.8 签名发布

```http
POST /api/v1/detection/packages/:package_id/sign
```

请求：

```json
{
  "build_id": "uuid",
  "version": "1.0.0",
  "confirm": true
}
```

说明：

- 只有构建成功才允许签名。
- API Server 不持有私钥，只调用 builder 签名动作。
- builder 使用编译进组件内的 Ed25519 私钥对整个 `tar.gz` 签名。
- 签名后上传 package 和 `.sig` 到 MinIO。

### 3.9 启用

```http
POST /api/v1/detection/packages/:package_id/enable
```

请求：

```json
{
  "version": "1.0.0",
  "confirm_global_rollout": true
}
```

行为：

- 设置该 package/version 为 enabled。
- 同 `package_id` 其他 enabled 版本置为 disabled。
- 下发 install command 到全部在线 agent。
- 离线 agent 上线后由 server 自动补发。

### 3.10 禁用

```http
POST /api/v1/detection/packages/:package_id/disable
```

请求：

```json
{
  "version": "1.0.0"
}
```

行为：

- 下发 disable/uninstall command。
- agent detach eBPF links，删除本地 artifact。

### 3.11 卸载

```http
POST /api/v1/detection/packages/:package_id/uninstall
```

第一版卸载等价于禁用并要求 agent 删除本地 artifact。

### 3.12 主机状态

```http
GET /api/v1/detection/packages/:package_id/hosts?version=1.0.0&status=active&page=1&page_size=50
```

响应：

```json
{
  "code": 0,
  "data": {
    "data": [
      {
        "host_id": "uuid",
        "hostname": "host-1",
        "status": "active",
        "active_artifact": "ringbuf",
        "loaded_hooks": ["syscalls/sys_enter_socket"],
        "kernel_release": "5.10.0",
        "arch": "amd64",
        "error_message": "",
        "last_reported_at": "2026-05-22T00:00:00Z"
      }
    ],
    "total": 1
  }
}
```

---

## 4. Hook Allowlist API

### 4.1 获取当前配置

```http
GET /api/v1/settings/ebpf-hooks/allowlist
```

响应：

```json
{
  "code": 0,
  "data": {
    "version": 3,
    "config": {
      "tracepoints": ["syscalls/sys_enter_socket"],
      "kprobes": [],
      "lsm": [],
      "xdp": [],
      "tc": []
    },
    "updated_by": "admin",
    "activated_at": "2026-05-22T00:00:00Z"
  }
}
```

### 4.2 更新配置

```http
PUT /api/v1/settings/ebpf-hooks/allowlist
```

请求：

```json
{
  "config": {
    "tracepoints": [
      "syscalls/sys_enter_socket",
      "syscalls/sys_enter_bind"
    ],
    "kprobes": [],
    "lsm": [],
    "xdp": [],
    "tc": []
  },
  "description": "Enable CopyFail tracepoints"
}
```

行为：

- 保存新版本 allowlist。
- 广播给全部在线 agent。
- agent 收到后重新评估已安装 package。

---

## 5. API Server -> Builder gRPC 新增

V5.8 新增 `builder` 组件。Builder 是控制面内部构建和签名服务，只允许 API Server 调用，不直接和 frontend、server、agent、dc 通信。

通信原则：

- 源码、HookPlan、Sigma、Correlation 由 API Server 传给 builder。
- `.bpf.o`、`package.tar.gz`、`.sig` 等大文件由 builder 上传 MinIO。
- gRPC 只传任务参数、状态、日志摘要、对象 key 和校验信息。
- API Server 是业务状态源，builder 不直接写 PostgreSQL。
- Builder 持有编译进组件内的 Ed25519 私钥；API Server 不持有私钥。

### 5.1 服务定义

```proto
syntax = "proto3";

package aegis.builder.v1;

service BuilderService {
  rpc GetBuilderInfo(GetBuilderInfoRequest) returns (GetBuilderInfoResponse);
  rpc StartPackageBuild(StartPackageBuildRequest) returns (StartPackageBuildResponse);
  rpc GetPackageBuildStatus(GetPackageBuildStatusRequest) returns (GetPackageBuildStatusResponse);
  rpc SignPackage(SignPackageRequest) returns (SignPackageResponse);
}
```

### 5.2 Builder 信息

```proto
message GetBuilderInfoRequest {}

message GetBuilderInfoResponse {
  string builder_version = 1;
  string builder_image = 2;
  string builder_image_digest = 3;
  string clang_version = 4;
  string llvm_version = 5;
  string bpftool_version = 6;
  string libbpf_version = 7;
  repeated string supported_arches = 8;      // amd64, arm64
  repeated string supported_transports = 9;  // perf, ringbuf
  string signing_public_key_fingerprint = 10;
}
```

说明：

- `signing_public_key_fingerprint` 用于页面展示和审计，不泄露私钥。
- API Server 可以在提交构建前调用该接口，记录 builder digest 和工具链版本。

### 5.3 启动构建

```proto
message StartPackageBuildRequest {
  string build_id = 1;
  string package_id = 2;
  string version = 3;
  string title = 4;
  repeated string cve_ids = 5;
  string operator = 6;
  string builder_profile = 7;       // aegis-agent-builder-ubi8
  string target_arch = 8;           // amd64, arm64

  string hook_plan_yaml = 20;
  string ebpf_source = 21;
  string sigma_rules_yaml = 22;
  string correlation_yaml = 23;
  string package_metadata_json = 24;
}

message StartPackageBuildResponse {
  bool accepted = 1;
  string build_id = 2;
  string status = 3;                // pending, running, success, failed
  string message = 4;
}
```

约束：

- `build_id` 由 API Server 生成，便于数据库和 builder 任务关联。
- 第一版直接通过 gRPC 传源码和 YAML；如果后续包体过大，可以改为传 MinIO 草稿对象 key。
- Builder 收到请求后创建隔离工作目录，构建结束后上传 unsigned package 和构建日志。

### 5.4 查询构建状态

```proto
message GetPackageBuildStatusRequest {
  string build_id = 1;
}

message GetPackageBuildStatusResponse {
  string build_id = 1;
  string package_id = 2;
  string version = 3;
  string status = 4;                // pending, running, success, failed
  string error_message = 5;

  string builder_image_digest = 10;
  string clang_version = 11;
  string build_log_object_key = 12;
  string build_log_tail = 13;

  repeated BuildArtifact artifacts = 20;
  repeated HookSummary hooks = 21;
  string event_schema_json = 22;
  string unsigned_package_object_key = 23;
  string unsigned_package_sha256 = 24;
  int64 unsigned_package_size = 25;
}

message BuildArtifact {
  string name = 1;                  // copyfail.ringbuf.bpf.o
  string transport = 2;             // ringbuf, perf
  string object_key = 3;
  string sha256 = 4;
  int64 size = 5;
}

message HookSummary {
  string hook_type = 1;              // tracepoint, kprobe, lsm, xdp, tc
  string attach_point = 2;           // syscalls/sys_enter_socket
  string program_section = 3;
  string risk_level = 4;             // low, medium, high
}
```

说明：

- API Server 将构建结果写回 `detection_package_builds`。
- 页面展示 `hooks`、`event_schema_json`、artifact、build log 和工具链信息。
- `unsigned_package_object_key` 只能用于签名流程，不能下发给 agent。

### 5.5 签名发布

```proto
message SignPackageRequest {
  string build_id = 1;
  string package_id = 2;
  string version = 3;
  string operator = 4;
  bool confirm = 5;
}

message SignPackageResponse {
  bool success = 1;
  string message = 2;
  string package_object_key = 3;
  string signature_object_key = 4;
  string package_sha256 = 5;
  int64 package_size = 6;
  string signature_algorithm = 7;    // Ed25519
  string signing_key_fingerprint = 8;
  int64 signed_at = 9;
}
```

签名约束：

- 只有 `status=success` 的 build 可以签名。
- `confirm=true` 必须来自人工点击“签名发布”。
- Builder 只能签名自己产出的 unsigned package。
- 签名对象是整个 `package.tar.gz`，不是单个文件。
- 签名结果上传到正式发布路径，API Server 只保存 object key、sha256、size 和审计信息。

### 5.6 Builder gRPC 错误码

| code | message |
|:---|:---|
| `BUILD_NOT_FOUND` | build id does not exist in builder workspace |
| `BUILD_ALREADY_RUNNING` | build is already running |
| `BUILD_INPUT_INVALID` | HookPlan/Sigma/Correlation/eBPF source is invalid |
| `BUILD_ARTIFACT_MISSING` | perf or ringbuf artifact is missing |
| `BUILD_VALIDATION_FAILED` | package validation failed |
| `SIGN_BUILD_NOT_SUCCESS` | only successful builds can be signed |
| `SIGN_CONFIRM_REQUIRED` | confirm=true is required |
| `SIGN_UNSIGNED_PACKAGE_MISSING` | unsigned package object is missing |
| `SIGN_FAILED` | package signing failed |

---

## 6. API Server -> Server gRPC 扩展

当前 `UpdateAgentRules` 只适合 Sigma。V5.8 新增 package 命令。

```proto
service APIServerToServer {
  rpc UpdateAgentRules(UpdateAgentRulesRequest) returns (UpdateAgentRulesResponse);
  rpc SyncAgentConfig(SyncAgentConfigRequest) returns (SyncAgentConfigResponse);
  rpc InstallDetectionPackage(InstallDetectionPackageRequest) returns (InstallDetectionPackageResponse);
  rpc UninstallDetectionPackage(UninstallDetectionPackageRequest) returns (UninstallDetectionPackageResponse);
}

message SyncAgentConfigRequest {
  string host_id = 1;       // empty means all online agents
  repeated AgentConfig configs = 2;
}

message AgentConfig {
  string config_type = 1;   // dynamic_ebpf_hook_allowlist
  string action = 2;        // full_sync
  string payload = 3;       // JSON
}

message InstallDetectionPackageRequest {
  string package_id = 1;
  string version = 2;
  string package_url = 3;
  string signature_url = 4;
  int64 package_size = 5;
  bool rollback = 6;
}

message InstallDetectionPackageResponse {
  bool success = 1;
  int32 affected_agents = 2;
  string message = 3;
}

message UninstallDetectionPackageRequest {
  string package_id = 1;
  string version = 2;
}

message UninstallDetectionPackageResponse {
  bool success = 1;
  int32 affected_agents = 2;
  string message = 3;
}
```

说明：

- `host_id` 第一版不暴露到前端，内部可保留为空表示全部 agent。
- 离线 agent 的待安装包由 server 持久化或通过 api-server 在 agent online 后重新计算补发。

---

## 7. Server -> Agent gRPC 扩展

当前 `CommandRequest` 有 `ConfigSync`，V5.8 可复用并扩展 config type，也可新增 oneof。推荐新增明确消息，便于状态管理。

```proto
message CommandRequest {
  oneof request {
    CommandExecute execute = 1;
    CommandResult result = 2;
    RuleUpdateRequest rule_update = 3;
    BlockCommand block = 4;
    ConfigSync config_sync = 5;
    DetectionPackageCommand detection_package = 6;
  }
}

message DetectionPackageCommand {
  string command_id = 1;
  string action = 2;         // install, uninstall, disable, rollback
  string package_id = 3;
  string version = 4;
  string package_url = 5;
  string signature_url = 6;
  int64 package_size = 7;
  bool rollback = 8;
}
```

ConfigSync 新增类型：

```text
dynamic_ebpf_hook_allowlist
```

payload 示例：

```json
{
  "version": 3,
  "tracepoints": ["syscalls/sys_enter_socket"],
  "kprobes": [],
  "lsm": [],
  "xdp": [],
  "tc": []
}
```

---

## 8. Agent 状态上报

可以新增 `ReportDetectionPackageStatus`，也可以复用 `ReportEvent` 发送 status event。推荐新增 RPC，避免状态事件和安全告警混杂。

```proto
service AgentService {
  rpc ReportDetectionPackageStatus(DetectionPackageStatusRequest) returns (DetectionPackageStatusResponse);
}

message DetectionPackageStatusRequest {
  string host_id = 1;
  repeated DetectionPackageHostStatus statuses = 2;
}

message DetectionPackageHostStatus {
  string package_id = 1;
  string version = 2;
  string status = 3;
  string plugin_status = 4;
  string sigma_status = 5;
  string correlation_status = 6;
  string active_artifact = 7; // ringbuf, perf
  repeated string loaded_hooks = 8;
  string kernel_release = 9;
  string arch = 10;
  string error_message = 11;
  string metrics_json = 12;
  int64 reported_at = 13;
}

message DetectionPackageStatusResponse {
  bool success = 1;
  int32 received_count = 2;
}
```

---

## 9. Correlation 告警上报

复用 `RuntimeEvent`，需要保证 proto 已包含 `matched_rule_title` 和 `event_data_json` 字段；如果 agent 当前 proto 不一致，需要同步。

建议字段：

| RuntimeEvent 字段 | 值 |
|:---|:---|
| `event_type` | `correlation_alert` |
| `matched_rule_id` | correlation rule id |
| `severity` | DetectionSpec alert severity |
| `mitre_id` | alert.mitre_id |
| `event_data_json` | evidence chain |

---

## 10. 错误码

| code | message |
|:---|:---|
| `PACKAGE_SIGNATURE_INVALID` | package signature verification failed |
| `PACKAGE_DOWNGRADE_BLOCKED` | package downgrade is blocked |
| `HOOK_NOT_ALLOWED` | package hook is not allowed by current allowlist |
| `BUILD_NOT_SUCCESS` | package cannot be signed before successful build |
| `PACKAGE_NOT_SIGNED` | package must be signed before enable |
| `PLUGIN_LOAD_FAILED` | agent failed to load eBPF plugin |
