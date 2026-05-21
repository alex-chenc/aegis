# V5.7 自动阻断状态更新修复设计

**版本**: 5.7
**日期**: 2026-05-20
**状态**: 设计中

---

## 1. 问题描述

### 1.1 现象

自动阻断触发后，Agent 已成功执行阻断操作（如 kill_process 终止反弹 shell 进程），但告警列表中的阻断状态始终显示为"阻断中"（`blocking`），未能更新为"已阻断"（`success`）。

### 1.2 影响范围

| 影响项 | 说明 |
|:---|:---|
| 告警状态 | 自动阻断成功后，告警的 `block_status` 停留在 `blocking` |
| 告警生命周期 | `block_status` 不变为 `success`，告警的 `status` 不会自动变为 `resolved` |
| 前端展示 | 前端显示"阻断中"标签，用户误以为阻断未完成 |
| 统计数据 | `today_blocks` 统计可能不准确 |

---

## 2. 根因分析

### 2.1 代码定位

**问题文件**: `server/internal/grpc_server/server.go`
**问题函数**: `checkAutoActions` (第 666-728 行)

### 2.2 当前逻辑

```
checkAutoActions(alert)
├── 查询 BlockPolicy
├── if policy.AutoBlock:
│   ├── 设置 alert.BlockStatus = "blocking"  ← 始终执行
│   ├── 构建 BlockCommand
│   ├── SendBlockCommand(hostID, cmd)
│   │   ├── 成功 → return nil  ← 未更新状态！BUG 在此
│   │   └── 失败 → UpdateBlockStatus("failed", err)  ← 正确处理
│   └── broadcastPolicyUpdate
└── if policy.AutoDispose:
    └── 设置 alert.Status = "resolved"
```

### 2.3 对比：手动阻断路径（正常）

**文件**: `api-server/internal/service/alert_service.go`
**函数**: `ManualBlock` (第 210 行)

手动阻断路径在 gRPC 调用成功后，显式调用 `alertRepo.UpdateBlockStatus(alertID, BlockSuccess, "阻断执行成功")`，状态能正确更新。

### 2.4 根因总结

`checkAutoActions` 函数在 `SendBlockCommand` 成功返回后，缺少将 `block_status` 从 `"blocking"` 更新为 `"success"` 的逻辑。

---

## 3. 修复方案

### 3.1 修改点

在 `server/internal/grpc_server/server.go` 的 `checkAutoActions` 函数中：

1. 当 `SendBlockCommand` 返回 `nil`（成功）时，调用 `alertRepo.UpdateBlockStatus` 将状态更新为 `"success"`
2. 同步更新内存中的 `alert.BlockStatus` 指针，防止后续 `AutoDispose` 的 `Update(alert)` 覆盖数据库值

### 3.2 改造前代码

```go
if err := s.SendBlockCommand(alert.HostID, cmd); err != nil {
    logger.Error("failed to send auto-block command", ...)
    if updateErr := s.alertRepo.UpdateBlockStatus(alert.AlertID, "failed", err.Error()); updateErr != nil {
        logger.Error("failed to update alert block send error", ...)
    }
}
```

### 3.3 改造后代码

```go
if err := s.SendBlockCommand(alert.HostID, cmd); err != nil {
    logger.Error("failed to send auto-block command", ...)
    failedStatus := "failed"
    alert.BlockStatus = &failedStatus          // 同步内存状态
    alert.BlockMessage = err.Error()
    if updateErr := s.alertRepo.UpdateBlockStatus(alert.AlertID, "failed", err.Error()); updateErr != nil {
        logger.Error("failed to update alert block send error", ...)
    }
} else {
    logger.Info("auto-block command executed successfully", ...)
    successStatus := "success"
    alert.BlockStatus = &successStatus         // 同步内存状态
    alert.BlockMessage = "自动阻断执行成功"
    alert.Status = "resolved"
    if updateErr := s.alertRepo.UpdateBlockStatus(alert.AlertID, "success", "自动阻断执行成功"); updateErr != nil {
        logger.Error("failed to update alert block success status", ...)
    }
}
```

### 3.4 内存状态同步说明

当 `AutoBlock` 和 `AutoDispose` 同时启用时，`checkAutoActions` 会依次执行：
1. 阻断逻辑 → 更新 `block_status` 到数据库
2. 处置逻辑 → 调用 `s.alertRepo.Update(alert)` 做全量 Save

如果内存中的 `alert.BlockStatus` 未同步，步骤 2 的 `Save` 会将数据库中的 `block_status` 覆盖回 `"blocking"`。
因此必须在步骤 1 中同步更新内存指针。

### 3.4 状态流转图

```
自动阻断触发
    │
    ▼
BlockStatus = "blocking"
    │
    ├── SendBlockCommand 成功
    │   └── BlockStatus = "success"  ← 修复后新增
    │       └── Alert.Status = "resolved"  ← UpdateBlockStatus 自动处理
    │
    └── SendBlockCommand 失败
        └── BlockStatus = "failed"
```

---

## 4. 测试用例

### 4.1 单元测试

**测试文件**: `server/internal/grpc_server/auto_block_status_test.go`

| 用例 | 输入 | 预期结果 |
|:---|:---|:---|
| 自动阻断成功（Callback） | CallbackClient 返回 Success=true | BlockStatus = "success", Alert.Status = "resolved" |
| 自动阻断失败（Callback） | CallbackClient 返回 Success=false | BlockStatus = "failed", Alert.Status = "pending" |
| 自动阻断成功（Stream） | Stream.Send 返回 nil | BlockStatus = "success" |
| AutoBlock + AutoDispose 组合 | CallbackClient 返回 Success=true, AutoDispose=true | BlockStatus = "success", Alert.Status = "resolved" |

### 4.2 接口测试

通过 curl 验证告警 API 返回正确的 `block_status`。

---

## 5. 涉及文件

| 文件 | 修改类型 | 说明 |
|:---|:---|:---|
| `server/internal/grpc_server/server.go` | 修改 | `checkAutoActions` 增加成功状态更新 |
| `server/internal/grpc_server/auto_block_status_test.go` | 新增 | 单元测试 |

---

## 6. 风险评估

| 风险 | 等级 | 缓解措施 |
|:---|:---|:---|
| 数据库写入失败 | 低 | 已有错误日志，不影响主流程 |
| 重复更新 | 低 | GORM Updates 为幂等操作 |
| WebSocket 广播 | 无 | 状态变更会触发前端刷新 |
