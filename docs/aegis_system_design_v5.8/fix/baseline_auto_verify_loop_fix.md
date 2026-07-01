# 基线自动验证闭环修复

## Bug 描述与症状

基线任务自动验证功能在页面上已经提供 `auto_verify` 和 `max_rounds` 参数，但实际运行中存在断点：

1. 检测未通过后，修复成功再检测时，后续检测结果仍可能停留在未通过，无法继续按最大轮数闭环。
2. 自动验证子任务未按用户设置的最大轮数继续执行，通常第一轮复检失败后就停止。
3. 修复任务失败时，系统会尝试直接重复下发旧修复脚本，而不是先进入大模型修复脚本流程。

## 复现路径

1. 在规则管理中选择规则和在线主机，设置最大轮数大于 1，并开启自动验证。
2. 下发检测任务，使检测脚本返回 exit code `1`（脚本执行成功，但基线不合规）。
3. 等待自动修复和复检。
4. 观察任务组中后续自动验证任务是否持续执行到检测通过或达到最大轮数。

## 根因分析

### 调用链

```text
Workbench.vue
  -> POST /api/v1/tasks/run-check 或 /api/v1/tasks/run-fix
  -> TaskHandler.RunCheck/RunFix
  -> TaskService.CreateAndDispatchTasks
  -> TaskService.dispatchToAgent
  -> server.ForwardCommand
  -> agent 执行脚本
  -> server.GRPCServer.ExecuteCommand 接收 CommandResult
  -> server TaskLogRepository.UpdateResult
```

### 关键问题

1. `AutoVerifyService` 挂在 `api-server` 的 `TaskService.ProcessTaskResult` 后面，但正常 Agent 最终执行结果由 `server` 直接写入数据库，未回传到 `api-server`。因此自动验证逻辑收不到最终结果事件。
2. `AutoVerifyService.triggerFixForVerify` 和 `triggerCheckForVerify` 创建子任务时把 `MaxRounds` 写死为 `1`，导致后续任务无法继承用户设置的最大轮数。
3. 自动验证的修复失败分支直接创建新的 FIX 任务，容易重复旧脚本；业务期望是修复失败时先进入大模型脚本修复，直到修复任务成功，再继续检测。
4. 自愈服务更新规则脚本时只写旧字段 `script_status=ready`，未同步类型化状态 `check_script_status/fix_script_status=generated`。

## 修复设计

1. 在 api-server 启动自动验证结果扫描器：
   - 定期读取 `task_logs` 中 `auto_verify=true` 且已进入终态的任务。
   - 对 server 已写入的最终结果补触发自动验证逻辑。
   - 使用任务 ID、状态、退出码、尝试次数、完成时间组成内存幂等 key，避免同一结果在一个进程生命周期内重复处理。
   - 创建下一步任务前查询同组、同规则、同主机、同轮次、同类型的自动验证任务是否已存在，避免重启或重复扫描造成重复下发。

2. 避免 RUNNING 误触发：
   - `TaskService.ProcessTaskResult` 只在任务进入终态后触发大模型自愈和自动验证。
   - 派发后写入 `RUNNING` 只更新状态，不参与自动验证判断。

3. 修正自动验证轮次：
   - 自动验证子任务继承原任务 `MaxRounds`。
   - CHECK exit code `1` 表示检测未通过，应继续下发 FIX，直到检测通过或达到最大轮数。
   - FIX 成功后下发 CHECK 复检。
   - FIX 失败时不直接重复旧脚本，而是交给 `TaskService` 的大模型自愈流程处理；自愈成功重派原 FIX 后，再进入自动验证复检。

4. 同步脚本状态：
   - `RuleRepository.UpdateScript` 同步更新 `check_script_status/fix_script_status` 为 `generated`，并清理对应错误信息。

5. 前端保持现有行为：
   - 检测类型任务不展示“重新下发”。
   - 任务详情不展示通过率。
   - 任务中心使用后端返回的数字 `pass_rate`。

## 回归测试用例

1. `AutoVerifyService`：CHECK 非合规结果应创建 FIX 子任务，并继承 `max_rounds`。
2. `AutoVerifyService`：FIX 成功应创建 CHECK 子任务，并继承 `max_rounds`。
3. `AutoVerifyService`：FIX 失败不应直接创建重复 FIX 子任务。
4. `TaskService.ProcessTaskResult`：最终 CHECK exit code `1` 应触发自动 FIX；RUNNING 不触发自动验证。
5. `TaskLogRepository`：可查询终态自动验证任务，并能识别已存在的同轮次后续任务。
6. API smoke：登录后创建带 `auto_verify=true` 的基线检测/修复任务，查询任务组接口确认 `auto_verify/max_rounds/verify_round/pass_rate` 字段可见。

## 影响组件

- `api-server/internal/service/task_service.go`
- `api-server/internal/service/auto_verify_service.go`
- `api-server/internal/repository/rule_repo.go`
- `api-server/internal/repository/task_log_repo.go`
- `api-server/cmd/main.go`

## 风险与回滚

风险较低。扫描器只处理 `auto_verify=true` 的基线任务，不改变 Agent/server 协议，也不改变 server 的结果入库路径。幂等查询会避免重复创建后续任务。回滚时撤销本修复涉及文件，自动验证功能回到原先不完整状态。

## 验证步骤

1. 运行 api-server 和 server 相关单元测试。
2. 构建 api-server 与 server。
3. 使用 `docker compose up -d --build api-server server` 重装服务。
4. 使用 `admin/Admin@123` 登录获取 token。
5. 使用 curl 下发带 `auto_verify=true`、`max_rounds>1` 的基线任务，并查询任务组状态/日志。
