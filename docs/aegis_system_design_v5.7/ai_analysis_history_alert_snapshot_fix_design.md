# AI 分析历史会话告警事件快照修复设计文档

## 概述

修复 AI 分析历史会话加载时，左侧告警事件面板未同步加载被分析告警的问题。

## 问题分析

### 问题 7：历史会话加载后左侧告警事件未显示

**现象：** 加载历史 AI 分析会话（如 `06b89c4b-4c1e-4a74-bf57-b6fe3e18e57e`）后，左侧面板不显示该会话分析的告警事件。用户无法看到该会话分析了哪些告警。

**根因：** 左侧告警面板由 `visibleAlertRows` 计算属性驱动：

```typescript
const isAnalysisSnapshotActive = computed(() => Boolean(sessionId.value && analysisAlertSnapshot.value.length > 0))
const visibleAlertRows = computed(() => {
  return isAnalysisSnapshotActive.value ? analysisAlertSnapshot.value : filteredAlerts.value
})
```

`analysisAlertSnapshot` 在 `loadSession()` 中被清空为 `[]`（line 964），但从未从历史数据中重新填充。

**数据流断裂点：**

1. `getSessionHistory` API 返回 `messages`、`execution_plan`、`audits`、`reflections`、`corrections`、`status`、`conclusion`，但**不包含告警数据**。
2. 会话模型 `AISession` 只存储 `alert_ids`（UUID 数组），不存储告警详情。
3. 后端 `GetSessionHistory` handler 未查询告警表，也未返回告警快照。
4. 前端 `loadSession()` 没有逻辑来恢复 `analysisAlertSnapshot`。

**影响范围：**
- 所有通过历史会话列表加载的会话均受影响
- 页面刷新后从 localStorage 恢复的会话不受影响（因为 localStorage 缓存了快照）

## 修复方案

### 后端修改

**文件：** `api-server/internal/api/handler/ai_analysis_handler.go`

修改 `GetSessionHistory` handler，在返回数据中增加 `alerts` 字段：

1. 从已加载的 session 中获取 `AlertIDs`
2. 使用 `alertRepo.FindByIDs()` 查询告警记录
3. 使用 `buildAlertSnapshots()` 构建告警快照
4. 将快照添加到响应的 `alerts` 字段
5. 如果告警已被删除（查询返回空），`alerts` 为空数组

```go
// 在 GetSessionHistory 中增加告警快照加载
var alertSnapshots []AlertContextSnapshot
if h.alertRepo != nil && len(session.AlertIDs) > 0 {
    alerts, err := h.alertRepo.FindByIDs(session.AlertIDs)
    if err == nil {
        alertSnapshots = buildAlertSnapshots(alerts)
    }
}
```

响应结构变更：
```json
{
  "success": true,
  "data": {
    "session_id": "...",
    "messages": [...],
    "execution_plan": {...},
    "audits": [...],
    "reflections": [...],
    "corrections": [...],
    "status": "completed",
    "conclusion": {...},
    "alerts": [...]  // 新增字段
  }
}
```

### 前端修改

**文件 1：** `frontend/src/api/aiAnalysis.ts`

更新 `getSessionHistory` 返回类型，增加 `alerts` 字段。

**文件 2：** `frontend/src/views/detection/AIAnalysis.vue`

在 `loadSession()` 中，从 history 响应的 `alerts` 字段填充 `analysisAlertSnapshot`：

```typescript
// 在 loadSession() 中，从 history 响应加载告警快照
if (payload.alerts && payload.alerts.length > 0) {
  analysisAlertSnapshot.value = payload.alerts
}
```

### 边界情况处理

1. **告警已被删除：** 后端 `FindByIDs` 返回空或部分结果，前端收到空数组，`isAnalysisSnapshotActive` 为 false，左侧面板显示空状态（正常行为）
2. **告警数据变更：** 后端查询的是当前告警状态（非快照），如果告警在分析后被修改（如状态、阻断状态变更），显示的是最新数据而非分析时的数据。这是可接受的行为，因为告警快照本身不持久化。
3. **内存 fallback 路径：** 内存中的 session 已有 `AlertSnapshots`，直接使用。

## 不修改的部分

- 不修改会话创建流程（已正常工作）
- 不修改 localStorage 恢复流程（已正常工作）
- 不修改 `GetSessionList` 接口（列表页不需要告警详情）
- 不持久化告警快照到数据库（当前设计不需要）
