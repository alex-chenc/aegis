# AI 分析告警列表分页功能设计文档

## 1. 问题描述

AI 分析页面的告警选择列表存在以下问题：

1. **数据截断**：`buildAnalysisAlertQuery()` 硬编码 `pageSize = 200`，超过 200 条的告警被静默丢弃
2. **无分页 UI**：告警表格使用固定高度 400px 滚动展示，没有分页控件
3. **total 丢弃**：后端 API 返回 `{ data: Alert[], total: number }`，但前端 `loadAlerts()` 只使用 `response.data`，忽略 `total`

## 2. 设计目标

| 目标 | 说明 |
|------|------|
| 显示全部告警 | 移除 200 条硬编码限制，通过分页加载全部匹配告警 |
| 分页控件 | 默认每页 10 条，可选 20、50 条 |
| 筛选联动 | 切换主机/时间范围时重置到第 1 页 |
| 保持选择 | 跨页选择的告警 ID 保持不变 |

## 3. 修改范围

**仅前端修改**，后端 `ListAlerts` API 已支持 `page`/`pageSize` 分页参数和 `total` 返回。

### 3.1 文件修改清单

| 文件 | 修改内容 |
|------|---------|
| `frontend/src/utils/aiAnalysisFilters.ts` | `buildAnalysisAlertQuery` 默认 pageSize 从 200 改为 10 |
| `frontend/src/views/detection/AIAnalysis.vue` | 新增分页状态、分页 UI、分页事件处理 |
| `frontend/src/utils/aiAnalysisFilters.test.ts` | 更新测试用例适配新默认值 |

### 3.2 不修改的文件

| 文件 | 原因 |
|------|------|
| `api-server/internal/api/handler/detection_handler.go` | 已支持分页，pageSize 上限 1000 |
| `api-server/internal/repository/alert_repo.go` | 已支持 Offset/Limit 分页查询 |
| `frontend/src/api/detection.ts` | `getAlerts` 已返回 `{ data, total }` 类型 |

## 4. 详细设计

### 4.1 状态变更（AIAnalysis.vue）

新增以下响应式状态：

```typescript
const alertPage = ref(1)          // 当前页码
const alertPageSize = ref(10)     // 每页条数，默认 10
const alertTotal = ref(0)         // 总条数
```

### 4.2 `buildAnalysisAlertQuery` 修改（aiAnalysisFilters.ts）

将默认 `pageSize` 从 200 改为 10：

```typescript
export function buildAnalysisAlertQuery(
  hostFilter: string[],
  timeRange?: [string | Date, string | Date] | null,
  page = 1,
  pageSize = 10   // 原为 200
): AnalysisAlertQuery | null {
```

### 4.3 `loadAlerts` 修改（AIAnalysis.vue）

```typescript
async function loadAlerts(force = false) {
  const query = buildAnalysisAlertQuery(
    hostFilter.value, timeRange.value,
    alertPage.value, alertPageSize.value  // 传入分页参数
  )
  // ...
  const response = await getAlerts(query || {
    page: alertPage.value,
    pageSize: alertPageSize.value
  })
  alerts.value = response.data || []
  alertTotal.value = response.total || 0  // 保存 total
}
```

### 4.4 筛选联动

当主机过滤或时间范围变化时，重置到第 1 页：

```typescript
watch([hostFilter, timeRange], () => {
  alertPage.value = 1
}, { deep: true })
```

### 4.5 分页事件处理

```typescript
function handleAlertPageChange(page: number) {
  alertPage.value = page
  loadAlerts()
}

function handleAlertSizeChange(size: number) {
  alertPageSize.value = size
  alertPage.value = 1
  loadAlerts()
}
```

### 4.6 UI 变更（AIAnalysis.vue template）

在告警表格下方、选择信息上方添加 `el-pagination`：

```html
<el-pagination
  v-if="alertTotal > alertPageSize"
  class="alert-pagination"
  background
  layout="total, sizes, prev, pager, next"
  :total="alertTotal"
  :page-size="alertPageSize"
  :current-page="alertPage"
  :page-sizes="[10, 20, 50]"
  @current-change="handleAlertPageChange"
  @size-change="handleAlertSizeChange"
/>
```

### 4.7 选择状态跨页保持

`selectedAlertIds` 存储的是告警 ID 数组，不依赖当前页数据。`el-table` 的 `selection-change` 事件只在当前页触发，因此需要：

- 使用 `el-table` 的 `row-key` 属性确保行标识唯一
- 在页面切换后恢复已选行的选中状态
- 在 `watch(alerts)` 中调用恢复逻辑

```typescript
watch(alerts, () => {
  nextTick(() => {
    if (alertTableRef.value) {
      alerts.value.forEach((alert: Alert) => {
        if (selectedAlertIds.value.includes(alert.id)) {
          alertTableRef.value.toggleRowSelection(alert, true)
        }
      })
    }
  })
})
```

## 5. API 接口不变

现有 API 完全满足需求，无需修改：

```
GET /api/v1/detection/alerts?page=1&pageSize=10&hostnames=host-a&start_time=...&end_time=...

Response:
{
  "code": 0,
  "message": "success",
  "data": {
    "data": [/* alerts */],
    "total": 156
  }
}
```

## 6. 测试计划

### 6.1 单元测试（aiAnalysisFilters.test.ts）

| 用例 | 验证点 |
|------|--------|
| `buildAnalysisAlertQuery` 默认 pageSize | pageSize 应为 10 |
| `buildAnalysisAlertQuery` 自定义 pageSize | 传入 20/50 应正确返回 |
| 分页参数传递 | page 和 pageSize 应正确构建到 query 中 |

### 6.2 API 测试（curl）

```bash
# 获取 token
TOKEN=$(curl -s -X POST http://localhost:8082/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"Cc&324511"}' | jq -r '.data.token')

# 测试分页 - 第 1 页，每页 10 条
curl -s "http://localhost:8082/api/v1/detection/alerts?page=1&pageSize=10" \
  -H "Authorization: Bearer $TOKEN" | jq '.data.total, (.data.data | length)'

# 测试分页 - 第 2 页
curl -s "http://localhost:8082/api/v1/detection/alerts?page=2&pageSize=10" \
  -H "Authorization: Bearer $TOKEN" | jq '.data.total, (.data.data | length)'
```

## 7. UI 布局变更

```
┌─────────────────────────────────────┐
│ 选择要分析的告警              [刷新] │
├─────────────────────────────────────┤
│ 时间范围: [________] ~ [________]   │
│ 主机过滤: [host-a ▼]               │
│ 最大轮数: [500]                     │
├─────────────────────────────────────┤
│ □ | 主机   | 规则   | 时间   | 级别 │
│ □ | host-a | 可疑.. | 01:05  | 高   │
│ □ | host-b | 异常.. | 01:30  | 严重 │
│ ...                                 │
├─────────────────────────────────────┤  <- 新增
│ 共 156 条  [<] [1] [2] [3] ... [>] │
│ 每页: [10 ▼]                        │
├─────────────────────────────────────┤
│ 已选择 3 个告警    [开始 AI 分析]   │
└─────────────────────────────────────┘
```
