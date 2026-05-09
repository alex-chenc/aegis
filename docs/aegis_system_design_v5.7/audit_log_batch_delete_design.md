# V5.7 审计日志批量删除功能设计

**版本**: 5.7
**日期**: 2026-05-08
**状态**: 已完成

---

## 1. 背景与动机

### 1.1 当前问题

审计日志页面（`/settings/audit-logs`）目前仅支持查看和筛选，不支持任何删除操作。随着系统运行，审计日志会持续累积，用户需要手动清理过期或无用的日志记录。

### 1.2 设计目标

在审计日志表格中增加多选删除功能，支持：

1. **批量删除**: 通过复选框多选后批量删除
2. **安全确认**: 删除前弹出确认对话框，显示将删除的记录数
3. **联动刷新**: 删除后自动刷新列表和统计数据

---

## 2. API 设计

### 2.1 批量删除接口

```
DELETE /api/v1/settings/audit-logs
Content-Type: application/json

Request Body:
{
  "ids": ["uuid-1", "uuid-2", "uuid-3"]
}

Response (200):
{
  "code": 0,
  "message": "success",
  "data": {
    "deleted": 3
  }
}
```

**参数校验**:
- `ids` 不能为空数组
- `ids` 长度上限 100
- 每个 id 必须是合法 UUID

### 2.2 路由注册

在 `router.go` 的 `audit-logs` 路由组中新增：

```go
auditLogs.DELETE("", r.auditLogHandler.DeleteLogs)
```

---

## 3. 后端实现

### 3.1 Repository 层

在 `audit_log_repo.go` 中新增 `DeleteByIDs` 方法：

```go
func (r *AuditLogRepo) DeleteByIDs(ids []uuid.UUID) (int64, error) {
    result := r.db.Where("id IN ?", ids).Delete(&model.ScriptAuditLog{})
    if result.Error != nil {
        return 0, result.Error
    }
    return result.RowsAffected, nil
}
```

### 3.2 Handler 层

在 `audit_log_handler.go` 中新增 `DeleteLogs` handler：

```go
type DeleteLogsRequest struct {
    IDs []string `json:"ids" binding:"required,min=1,max=100"`
}

func (h *AuditLogHandler) DeleteLogs(c *gin.Context) {
    // 1. 解析请求体
    // 2. 验证 UUID 格式
    // 3. 调用 repo.DeleteByIDs
    // 4. 返回删除数量
}
```

---

## 4. 前端实现

### 4.1 API 层 (`audit-logs.ts`)

新增 `deleteLogs` 方法：

```typescript
deleteLogs: (ids: string[]) =>
  request.delete('/settings/audit-logs', { data: { ids } })
```

### 4.2 Composable (`useAuditLogs.ts`)

新增 `deleteLogs` 函数：

```typescript
const deleteLogs = async (ids: string[]) => {
  const res = await auditLogApi.deleteLogs(ids)
  await fetchLogs()    // 刷新列表
  await fetchStats()   // 刷新统计
  return res.deleted
}
```

### 4.3 表格组件 (`AuditLogTable.vue`)

变更点：

1. **新增选择列**: 在表格第一列添加 `<el-table-column type="selection" width="55" />`
2. **新增批量删除按钮**: 在表格 header 右侧添加删除按钮，显示选中数量
3. **新增 `selection-change` 事件**: 监听表格选中变化
4. **新增 `delete` 事件**: 向父组件传递删除事件

UI 交互：
- 未选中任何行时，删除按钮禁用
- 选中行后，删除按钮显示选中数量（如 "删除 (3)"）
- 点击删除按钮弹出确认对话框
- 确认后调用 API 删除并刷新

### 4.4 父组件 (`index.vue`)

新增 `handleDelete` 函数，接收选中的 id 列表，调用 composable 的 `deleteLogs`，显示成功/失败提示。

---

## 5. 测试策略

### 5.1 后端测试

| 测试用例 | 描述 |
|:---|:---|
| `TestDeleteLogs_Success` | 批量删除多条记录，验证返回删除数量 |
| `TestDeleteLogs_EmptyIDs` | 空 ids 数组返回 400 |
| `TestDeleteLogs_InvalidUUID` | 非法 UUID 返回 400 |
| `TestDeleteLogs_NotFound` | 不存在的 UUID 不报错，返回 deleted=0 |
| `TestDeleteLogs_PartialMatch` | 部分存在部分不存在，返回实际删除数 |

### 5.2 前端测试

| 测试用例 | 描述 |
|:---|:---|
| API `deleteLogs` | 验证调用 `DELETE /settings/audit-logs` 并传递 ids |
| Composable `deleteLogs` | 验证删除后自动刷新列表和统计 |
| 组件 `renders table with log data` | 验证表格正确渲染（含选择列） |
| 组件 `has a selection column` | 验证第一列为多选列 |
| 组件 `shows delete button in header` | 验证删除按钮存在 |
| 组件 `delete button disabled when no selection` | 未选中时按钮禁用 |
| 组件 `emits delete event with selected ids` | 确认后传递正确的 ID 列表 |
| 组件 `does not emit delete when cancelled` | 取消确认时不触发删除 |
| 组件 `shows selected count in button` | 按钮显示选中数量 |
| 组件 `emits filter event` | 筛选参数正确传递 |
| 组件 `formats timestamps correctly` | 时间格式化正确 |
| 组件 `returns correct risk tag types` | 风险等级标签类型正确 |
| 组件 `clears selection after delete` | 删除后清空选中状态 |
| 组件 `handles pagination` | 分页参数正确传递 |

---

## 6. 文件变更清单

| 文件 | 变更类型 | 说明 |
|:---|:---|:---|
| `api-server/internal/repository/audit_log_repo.go` | 修改 | 新增 `DeleteByIDs` 方法 |
| `api-server/internal/api/handler/audit_log_handler.go` | 修改 | 新增 `DeleteLogs` handler |
| `api-server/internal/api/handler/audit_log_handler_test.go` | 新增 | 5 个后端测试用例 |
| `api-server/internal/api/router.go` | 修改 | 注册 DELETE 路由 |
| `frontend/src/api/audit-logs.ts` | 修改 | 新增 `deleteLogs` API |
| `frontend/src/api/audit-logs.test.ts` | 修改 | 新增 deleteLogs 测试 |
| `frontend/src/views/settings/AuditLogs/composables/useAuditLogs.ts` | 修改 | 新增 `deleteLogs` 函数 |
| `frontend/src/views/settings/AuditLogs/composables/useAuditLogs.test.ts` | 修改 | 新增 deleteLogs 测试 |
| `frontend/src/views/settings/AuditLogs/components/AuditLogTable.vue` | 修改 | 新增多选列和删除按钮 |
| `frontend/src/views/settings/AuditLogs/components/AuditLogTable.test.ts` | 新增 | 11 个组件测试用例 |
| `frontend/src/views/settings/AuditLogs/index.vue` | 修改 | 新增 handleDelete 处理 |
