# 基线模块增强与修复设计文档

## 问题描述

基线模块存在以下5个问题需要修复和增强：

1. **检测类型任务的"重新下发"按钮应删除** — 检测类型的任务不应允许手动重新下发
2. **通过率计算方式和显示需要调整** — 检测通过率=通过状态/总数，修复通过率=修复成功/总数；任务详情删除通过率，任务中心显示数字
3. **修复后再次检测仍显示未通过** — FIX修复成功后，再次CHECK仍显示未通过
4. **新增自动验证功能** — 检测未通过→自动修复→再检测，直到通过或达到最大轮数
5. **合规报告只导出XLS** — 删除PDF/每周/每月选项，导出内容包含每个任务的数据

## 根因分析

### 问题1：检测类型任务重新下发按钮
- **位置**: `frontend/src/views/TaskDetail.vue` 第444-453行
- **原因**: `canReExecute()` 函数未区分任务类型，CHECK和FIX任务都显示重新下发按钮
- **影响**: 检测任务不应手动重新下发，应通过自动验证流程处理

### 问题2：通过率计算和显示
- **位置**: `frontend/src/views/TaskDetail.vue` 第331-337行、476-479行；`frontend/src/views/TaskCenter.vue` 第324-328行
- **原因**: 当前通过率计算为 passed/terminal_tasks * 100，不区分检测和修复
- **需求**: 
  - 检测通过率 = CHECK任务中exit_code=0的数量 / CHECK任务总数
  - 修复通过率 = FIX任务中exit_code=0的数量 / FIX任务总数
  - 任务详情页面删除通过率显示
  - 任务中心列表显示数字而非进度条

### 问题3：修复后再次检测仍显示未通过
- **位置**: `api-server/internal/service/self_healing_service.go` 第510-519行
- **原因**: 自愈流程中FIX修复成功后，更新了规则的FIX脚本（`updateRuleScript`），但CHECK脚本未更新。当FIX修复成功后再次下发CHECK任务时，CHECK脚本可能仍然检测到问题。
- **根本原因**: FIX修复后，系统状态已改变，但CHECK脚本可能需要重新生成或更新以适应修复后的系统状态。
- **解决方案**: FIX修复成功后，触发CHECK任务重新检测时，应使用最新的检测逻辑。如果CHECK仍然不通过，应该触发自愈流程来修复CHECK脚本。

### 问题4：自动验证功能
- **位置**: 需要新增后端服务和前端UI
- **设计**: 
  - 检测任务：CHECK失败 → 自动触发FIX → FIX成功 → 再CHECK → 直到通过或达到最大轮数
  - 修复任务：FIX成功 → CHECK验证 → CHECK失败 → 再FIX → 直到通过或达到最大轮数
  - 需要跟踪验证轮数和状态

### 问题5：合规报告导出
- **位置**: `frontend/src/views/TaskCenter.vue` 第339-380行
- **原因**: 当前有PDF/Excel/每周/每月四个选项，需求是只保留XLS导出
- **需求**: 导出内容应包含每个任务的详细数据，而非仅任务组汇总

## 修复设计

### 修复1：删除检测类型任务的重新下发按钮

**前端修改**: `frontend/src/views/TaskDetail.vue`

修改 `canReExecute()` 函数，排除CHECK类型任务：

```typescript
function canReExecute(row: any): boolean {
  const taskType = normalizeType(row.task_type)
  // 检测类型任务不允许手动重新下发
  if (taskType === 'CHECK') return false
  return row.displayState === '修复失败' ||
         row.displayState === 'POC验证失败' ||
         row.displayState === '漏洞修复失败' ||
         row.displayState === '大模型修复成功' ||
         row.displayState === '修复超时'
}
```

### 修复2：通过率计算和显示调整

**前端修改1**: `frontend/src/views/TaskDetail.vue`
- 删除整体通过率显示（第29-31行的el-statistic）
- 删除单任务通过率列（第103-111行）

**前端修改2**: `frontend/src/views/TaskCenter.vue`
- 修改 `getPassRate()` 函数，区分检测和修复通过率：
  - 检测通过率 = CHECK任务中exit_code=0的数量 / CHECK任务总数
  - 修复通过率 = FIX任务中exit_code=0的数量 / FIX任务总数
- 将通过率显示从进度条改为数字
- 任务组通过率 = 通过的任务数 / 总任务数

**后端修改**: `api-server/internal/repository/task_log_repo.go`
- 在 `ListTaskGroups` SQL查询中添加通过率计算字段

### 修复3：修复后再次检测仍显示未通过

**后端修改**: `api-server/internal/service/self_healing_service.go`

在FIX修复成功后，如果需要再次检测，应触发CHECK脚本的重新生成或更新：

```go
// 在 processHealing 函数中，FIX修复成功后
if IsTaskExecutionSuccessful(resultTask.TaskType, resultTask.Status, resultTask.ExitCode, resultStderr) {
    // 修复成功后，触发CHECK脚本的重新检测
    // 如果CHECK脚本也需要更新，可以在这里处理
    // ...
}
```

**更优方案**: 在自动验证流程中处理（见修复4）

### 修复4：新增自动验证功能

**后端新增**: `api-server/internal/service/auto_verify_service.go`

创建自动验证服务，处理检测/修复的循环验证逻辑：

```go
type AutoVerifyService struct {
    taskService    *TaskService
    taskLogRepo    *repository.TaskLogRepository
    ruleRepo       *repository.RuleRepository
    healingService *SelfHealingService
    maxRounds      int
}

// VerifyTaskGroup 自动验证任务组
func (s *AutoVerifyService) VerifyTaskGroup(taskGroupID uuid.UUID, taskType string) {
    // 根据任务类型执行不同的验证逻辑
    switch taskType {
    case "CHECK":
        s.verifyCheckTaskGroup(taskGroupID)
    case "FIX":
        s.verifyFixTaskGroup(taskGroupID)
    }
}
```

**前端修改**: `frontend/src/views/Workbench.vue`

在任务下发对话框中添加"自动验证"开关：

```vue
<el-form-item label="自动验证">
  <el-switch v-model="autoVerifyEnabled" />
  <span style="font-size: 12px; color: #666; margin-left: 8px;">
    检测未通过时自动修复并重新检测
  </span>
</el-form-item>
```

**API修改**: `api-server/internal/api/handler/task_handler.go`

修改 `RunCheckRequest` 和 `RunFixRequest` 添加 `auto_verify` 字段：

```go
type RunCheckRequest struct {
    RuleIDs      []string `json:"rule_ids"`
    HostIDs      []string `json:"host_ids"`
    AutoVerify   bool     `json:"auto_verify"`
    MaxRounds    int      `json:"max_rounds"`
}
```

### 修复5：合规报告只导出XLS

**前端修改**: `frontend/src/views/TaskCenter.vue`

1. 删除PDF/每周/每月选项，只保留XLS导出
2. 修改导出内容，包含每个任务的详细数据：

```typescript
const exportExcelReport = async () => {
  // 获取所有任务组的详细任务数据
  const allTaskDetails = []
  for (const group of taskGroups.value) {
    const tasks = await getTaskLogs(group.task_group_id)
    for (const task of tasks) {
      allTaskDetails.push({
        任务组ID: group.task_group_id,
        规则标题: task.rule_title,
        主机: task.hostname,
        任务类型: getTaskTypeLabel(task.task_type),
        状态: getDisplayState(task.task_type, task.status, task.exit_code),
        退出码: task.exit_code,
        执行时间: task.finished_at,
      })
    }
  }
  // 生成XLS文件
  // ...
}
```

## 数据流变化

### 自动验证数据流

```
用户下发检测任务(自动验证=true)
    ↓
TaskService.CreateAndDispatchTasks() → 创建CHECK任务
    ↓
CHECK任务执行完成 → ProcessTaskResult()
    ↓
如果CHECK失败(exit_code=1)且auto_verify=true
    ↓
AutoVerifyService.TriggerAutoVerify()
    ↓
创建FIX任务 → 执行修复
    ↓
FIX完成 → 检查结果
    ↓
如果FIX成功 → 再次创建CHECK任务
    ↓
循环直到CHECK通过或达到最大轮数
```

## 接口变更

### 新增接口

无新增接口，修改现有接口。

### 修改接口

1. `POST /api/v1/tasks/run-check`
   - 新增字段: `auto_verify` (bool), `max_rounds` (int)

2. `POST /api/v1/tasks/run-fix`
   - 新增字段: `auto_verify` (bool), `max_rounds` (int)

3. `GET /api/v1/tasks`
   - 响应新增字段: `pass_rate` (float64) - 通过率百分比

## 数据库变更

无数据库表结构变更。

## 兼容性影响

- 前端通过率显示变化需要用户适应
- 自动验证功能为可选功能，默认关闭
- 合规报告导出格式变化（只保留XLS）

## 测试用例

### 测试用例1：检测类型任务无重新下发按钮
1. 下发检测任务
2. 等待任务完成
3. 在任务详情页面，检测类型任务不应显示"重新下发"按钮

### 测试用例2：通过率计算正确性
1. 下发包含3个规则的检测任务到2个主机（共6个检测任务）
2. 其中4个通过，2个未通过
3. 验证通过率显示为 66.7% (4/6)

### 测试用例3：自动验证流程
1. 下发检测任务，开启自动验证
2. 模拟检测失败
3. 验证系统自动触发修复
4. 验证修复成功后再次检测
5. 验证最终检测通过

### 测试用例4：合规报告导出
1. 创建包含多个任务的任务组
2. 点击"合规报告" → "导出 Excel"
3. 验证只导出XLS格式
4. 验证导出内容包含每个任务的详细数据

## 回滚计划

如果出现问题，可以通过以下方式回滚：
1. 前端代码回滚到git的上一个版本
2. 后端代码回滚到git的上一个版本
3. 重新构建和部署

## 风险评估

- **低风险**: 删除重新下发按钮、通过率显示调整、合规报告导出
- **中风险**: 自动验证功能（新增功能，需要充分测试）
- **中风险**: 修复后再次检测问题（涉及自愈流程逻辑）
