# Bug Fix: DetectionPackage 审核、启用下发、Event Schema 与告警证据链路

## Bug Description

V5.8 动态检测包当前实现与设计文档存在多处断点：

- 构建审核入口不完整，审核后页面不刷新，拒绝审核请求会因为 `approved=false` 被 Gin `required` 校验拦截。
- 构建成功/待审核状态与前端轮询状态名不一致，导致构建完成后审核、签名入口不稳定。
- 启用时 server 已发送 `DetectionPackageCommand` oneof，但 agent command stream 没有处理该 oneof，也没有处理 `AllowlistUpdate` oneof。
- API Server 到 server 的检测包命令使用 JSON roundtrip，缺少 snake_case JSON tags 时可能丢失 `package_id`、URL 等字段。
- Event Schema 在界面上只显示原始 JSON，缺少按设计文档定义的事件字段表和空态。
- 检测包详情页把 correlation runtime events 展示为“告警证据”，容易与全局告警列表混淆。

## Reproduction Steps

1. 使用管理员账号登录。
2. 创建或打开动态检测包草稿，提交构建。
3. 构建状态进入 `awaiting_review` 后，点击“审核拒绝”。
4. 在签名成功后点击“启用”，观察 agent 不会执行动态包安装处理。
5. 打开详情页 `Event Schema` 和“告警证据”页签，发现字段解释与设计文档不一致。

## Root Cause Analysis

- `DetectionPackageHandler.ReviewBuild` 使用 `binding:"required"` 校验 bool，`false` 被当作缺失值。
- `PackageDetail.vue` 仍轮询旧的 `build_pending/build_running` build 状态。
- `BuildReviewPanel.vue` 触发了 `sign` 事件，但父组件没有监听；审核完成后也没有通知父组件重载 build/package。
- `agent/internal/client/client.go` 只处理 `execute/rule_update/block/config_sync`，忽略 `detection_package_command` 和 `allowlist_update`。
- `DetectionPackageCommand` 服务结构缺少 JSON tags，经 `encoding/json` 转换到 protobuf 请求时字段名不匹配风险高。
- `ListPackagesUnified` 对 status filter 只查 published package，导致 draft/build/review 状态筛选缺失。
- 包详情页的关联告警 API 返回 runtime events，前端却读取不存在的 `title/evidence` 字段。

## Fix Design

- 后端审核 API 改为显式 bool 指针校验，允许拒绝审核。
- `ReviewBuild` 调用 builder review（可用时），并同步 build 与 draft 状态：`awaiting_review -> built` 或 `review_rejected`。
- `SignPackage` 使用 build 关联 draft 的标题、描述、CVE 信息创建 signed package。
- service 与 agent 的 `DetectionPackageCommand` 增加 snake_case JSON tags。
- agent command stream 增加 `DetectionPackageCommand` 与 `AllowlistUpdate` oneof 分支，并复用 ConfigManager 回调。
- 前端构建轮询兼容 `pending/running`，审核后刷新，只有审核通过后的 `success` build 允许签名。
- Event Schema 按 `plugin.yaml` 的 `event_schema.events` 展示为 event/field 表，并保留原 JSON 作为调试信息。
- 包详情页将“告警证据”改为“关联告警”，以 runtime event 表为主，展开行展示 evidence chain；全局告警处置仍在告警中心完成。

## Code Changes

- `api-server/internal/api/handler/detection_package_handler.go`
- `api-server/internal/service/detection_package_service.go`
- `api-server/internal/repository/detection_package_repo.go`
- `agent/internal/client/client.go`
- `agent/internal/configmgr/configmgr.go`
- `agent/internal/dynpkg/types.go`
- `frontend/src/api/detection-packages.ts`
- `frontend/src/views/detection/DetectionPackages/PackageDetail.vue`
- `frontend/src/views/detection/DetectionPackages/components/BuildReviewPanel.vue`
- `frontend/src/views/detection/DetectionPackages/index.vue`
- `frontend/src/views/detection/DetectionPackages/components/PackageStatusTag.vue`

## Verification Steps

- `cd api-server && go test ./internal/service ./internal/api/handler`
- `cd agent && go test ./internal/configmgr`
- `cd frontend && npm run type-check`
- `cd frontend && npm run build`

If Docker is available, additionally verify:

- `docker compose up -d --build api-server server frontend`
- Login with the user-provided admin credential.
- Build -> review -> sign -> enable a detection package.
- Confirm server logs show command sent and agent logs show detection package command received.

## Affected Components

- API Server detection package service, handler, and repository.
- Server-to-agent command stream payload compatibility.
- Agent runtime config manager and dynamic package command dispatcher.
- Frontend dynamic detection package list/detail/review views.

## Risk And Rollback Plan

Risk is limited to V5.8 dynamic detection package flows. Rollback by reverting this fix commit. Existing baseline detection, Sigma rules, vulnerability management, and command audit flows are not changed.
