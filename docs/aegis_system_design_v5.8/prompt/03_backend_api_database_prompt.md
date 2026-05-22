# Prompt 03: API Server 与数据库实现

你是 Aegis API Server 后端工程师，使用 Go、Gin、GORM、PostgreSQL、MinIO。请实现 V5.8 动态 eBPF DetectionPackage 的管理后端。

## 参考文档

- `docs/aegis_system_design_v5.8/overall_architecture_design_v5.8.md`
- `docs/aegis_system_design_v5.8/database_structure_design_v5.8.md`
- `docs/aegis_system_design_v5.8/api_grpc_design_v5.8.md`
- `docs/aegis_system_design_v5.8/code_interfaces_v5.8.md`

## 数据库

新增表：

- `detection_package_drafts`
- `detection_packages`
- `detection_package_builds`
- `detection_package_host_status`
- `detection_package_operations`
- `ebpf_hook_allowlist_configs`
- `correlation_rules`

扩展：

- `sigma_rules` 增加 `package_id`、`package_version`、`package_rule_type`
- `runtime_events` 增加 `package_id`、`correlation_rule_id`

## HTTP API

实现：

```text
GET    /api/v1/detection/packages
GET    /api/v1/detection/packages/:package_id
POST   /api/v1/detection/packages/ai-generate
POST   /api/v1/detection/packages/drafts
PUT    /api/v1/detection/packages/drafts/:draft_id
POST   /api/v1/detection/packages/:package_id/build
GET    /api/v1/detection/packages/builds/:build_id
POST   /api/v1/detection/packages/:package_id/sign
POST   /api/v1/detection/packages/:package_id/enable
POST   /api/v1/detection/packages/:package_id/disable
POST   /api/v1/detection/packages/:package_id/uninstall
GET    /api/v1/detection/packages/:package_id/hosts
GET    /api/v1/settings/ebpf-hooks/allowlist
PUT    /api/v1/settings/ebpf-hooks/allowlist
```

## 服务设计

新增：

```text
DetectionPackageService
DetectionPackageBuildService
BuilderClient
EBPFHookAllowlistService
DetectionPackageDispatchService
```

## 关键规则

- 草稿只保留最后一版。
- API Server 调用 builder gRPC 执行构建、查询构建状态和触发签名。
- 构建成功才允许签名。
- 签名发布只对整个 tar.gz 签名。
- API Server 不持有私钥，只调用 builder 使用内置私钥签名。
- 未签名 package 不允许启用。
- 启用范围第一版为全部 agent。
- 同 `package_id` 同时只能一个 enabled version。
- 默认禁止降级，rollback=true 例外。
- package 上传 MinIO，gRPC 只下发 URL。
- 操作必须写入 `detection_package_operations`。

## AI 生成

AI 生成只写草稿，不触发构建、签名、启用。

AI 输出应包含：

- HookPlan YAML
- eBPF source
- Sigma atomic YAML
- Correlation YAML

## 验收

- 数据库迁移可重复执行。
- API 返回统一 `{code,message,data}`。
- 构建任务状态可轮询。
- 签名包和 `.sig` 能上传 MinIO。
- 启用后能调用 server gRPC 下发 install command。
- allowlist 更新能广播到在线 agent。
