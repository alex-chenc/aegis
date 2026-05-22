# Prompt 04: Server 与 gRPC 转发实现

你是 Aegis server/gRPC 工程师。请实现 V5.8 DetectionPackage 指令从 api-server 到 agent 的转发。

## 参考文档

- `docs/aegis_system_design_v5.8/overall_architecture_design_v5.8.md`
- `docs/aegis_system_design_v5.8/api_grpc_design_v5.8.md`
- `proto/agent_comm.proto`
- `proto/api_server_comm.proto`

## 需要扩展

API Server -> Server:

```proto
rpc SyncAgentConfig(SyncAgentConfigRequest) returns (SyncAgentConfigResponse);
rpc InstallDetectionPackage(InstallDetectionPackageRequest) returns (InstallDetectionPackageResponse);
rpc UninstallDetectionPackage(UninstallDetectionPackageRequest) returns (UninstallDetectionPackageResponse);
```

Server -> Agent:

```proto
message DetectionPackageCommand {
  string command_id = 1;
  string action = 2;
  string package_id = 3;
  string version = 4;
  string package_url = 5;
  string signature_url = 6;
  int64 package_size = 7;
  bool rollback = 8;
}
```

`CommandRequest` 新增 oneof：

```proto
DetectionPackageCommand detection_package = 6;
```

ConfigSync 新增类型：

```text
dynamic_ebpf_hook_allowlist
```

## 行为要求

- `host_id` 为空表示全部在线 agent。
- 第一版前端只全局启用，但 gRPC 接口保留 host_id 能力。
- 在线 agent 通过现有 command stream 接收 `DetectionPackageCommand`。
- 离线 agent 上线后需要补发当前 enabled package 和最新 allowlist。
- 转发结果需要返回 affected_agents 和 message。
- 单个 agent 发送失败不能阻塞其他 agent。
- server 不直接调用 builder，也不持有签名私钥，只负责转发 API Server 的发布命令。

## 状态上报

新增或实现：

```proto
rpc ReportDetectionPackageStatus(DetectionPackageStatusRequest) returns (DetectionPackageStatusResponse);
```

server 收到后转发/写入 api-server 对应 repository。

## 验收

- proto 生成代码同步到 agent/server/api-server。
- api-server 可以调用 server 安装/卸载 package。
- server 能向在线 agent 发送 detection package command。
- agent 能上报 package host status。
- 失败有日志和错误原因。
