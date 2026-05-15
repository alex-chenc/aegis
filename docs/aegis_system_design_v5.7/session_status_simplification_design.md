# AI分析历史会话状态简化设计

**版本**: V5.7
**日期**: 2026-05-15
**状态**: 待实现
**适用范围**: AI分析模块历史会话状态管理、会话加载逻辑、结论展示优化

---

## 1. 背景与目标

### 1.1 当前问题

当前AI分析会话存在4种状态：`active`、`completed`、`paused`、`cancelled`，导致：
- 用户难以快速判断会话是否完成
- 状态展示逻辑复杂，需要处理多种边界情况
- 未完成会话加载后显示混乱，包含不该展示的结论UI

### 1.2 设计目标

1. **状态简化**: 仅保留两种状态 - "未完成"和"已完成"
2. **已完成判定**: 只有包含结论(conclusion)的会话才算"已完成"
3. **未完成会话加载**: 保持原始对话，不显示结论相关UI
4. **审计/反思处理**: 未执行的审计和反思在历史加载时显示为空
5. **结论展示优化**: 仅展示结论内容，不展示中间步骤
6. **处置建议**: 非误报场景展示处置建议框

---

## 2. 状态定义与转换

### 2.1 状态值定义

| 状态值 | 中文显示 | 判定条件 | 展示类型 |
|--------|----------|----------|----------|
| `completed` | 已完成 | `conclusion` 字段不为空 | success (绿色) |
| `active` | 未完成 | `conclusion` 字段为空，或状态为 `active`/`paused`/`cancelled` | info (灰色) |

### 2.2 状态判定逻辑

```go
// 后端状态判定
func GetDisplayStatus(session *model.AISession) string {
    // 只有conclusion不为空才是已完成
    if session.Conclusion != nil && len(session.Conclusion) > 0 {
        return "completed"
    }
    return "active"
}
```

```typescript
// 前端状态判定
function getDisplayStatus(session: SessionListItem): 'completed' | 'active' {
    if (session.conclusion && Object.keys(session.conclusion).length > 0) {
        return 'completed'
    }
    return 'active'
}
```

### 2.3 状态转换规则

```
创建会话 -> active (未完成)
AI分析完成且有结论 -> completed (已完成)
用户暂停/取消 -> active (未完成，因为没有结论)
```

---

## 3. 后端改造

### 3.1 Repository层改造

**文件**: `api-server/internal/repository/ai_session_repository.go`

修改 `FindList` 方法，状态过滤逻辑改为基于 `conclusion` 字段判定：

```go
// FindList 根据显示状态过滤会话
func (r *AISessionRepository) FindList(page, pageSize int, status string) ([]*model.AISession, int64, error) {
    var sessions []*model.AISession
    var total int64

    query := r.db.Model(&model.AISession{})
    switch status {
    case "completed":
        // 已完成：conclusion不为空
        query = query.Where("conclusion IS NOT NULL AND conclusion != 'null'::jsonb")
    case "active":
        // 未完成：conclusion为空
        query = query.Where("conclusion IS NULL OR conclusion = 'null'::jsonb")
    }
    // 其他status值不过滤，返回全部

    // ... 其余逻辑不变
}
```

### 3.2 Handler层改造

**文件**: `api-server/internal/api/handler/ai_analysis_handler.go`

修改 `GetSessionList` 返回的状态字段，基于 `conclusion` 判定：

```go
func (h *AIAnalysisHandler) GetSessionList(c *gin.Context) {
    // ... 获取会话列表

    // 转换状态为显示状态
    for _, session := range sessions {
        if session.Conclusion != nil && len(session.Conclusion) > 0 {
            session.Status = "completed"
        } else {
            session.Status = "active"
        }
    }

    // ... 返回响应
}
```

修改 `GetSessionHistory` 返回结构，增加会话状态信息：

```go
func (h *AIAnalysisHandler) GetSessionHistory(c *gin.Context) {
    // ... 获取消息历史

    // 获取会话信息
    session, _ := h.sessionRepo.FindBySessionID(sessionID)
    displayStatus := "active"
    var conclusion model.JSONB
    if session != nil && session.Conclusion != nil && len(session.Conclusion) > 0 {
        displayStatus = "completed"
        conclusion = session.Conclusion
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data": gin.H{
            "session_id":      sessionID,
            "messages":        messages,
            "execution_plan":  artifacts.ExecutionPlan,
            "audits":          artifacts.Audits,
            "reflections":     artifacts.Reflections,
            "corrections":     artifacts.Corrections,
            "status":          displayStatus,
            "conclusion":      conclusion,
        },
    })
}
```

---

## 4. 前端改造

### 4.1 状态显示改造

**文件**: `frontend/src/views/detection/AIAnalysis.vue`

修改历史会话表格的状态列显示：

```vue
<el-table-column prop="status" label="状态" width="100">
  <template #default="{ row }">
    <el-tag :type="getDisplayStatus(row) === 'completed' ? 'success' : 'info'" size="small">
      {{ getDisplayStatus(row) === 'completed' ? '已完成' : '未完成' }}
    </el-tag>
  </template>
</el-table-column>
```

添加状态判定函数：

```typescript
function getDisplayStatus(session: SessionListItem): 'completed' | 'active' {
  if (session.conclusion && Object.keys(session.conclusion).length > 0) {
    return 'completed'
  }
  return 'active'
}
```

### 4.2 会话加载逻辑改造

修改 `loadSession` 函数，根据状态决定显示内容：

```typescript
async function loadSession(session: SessionListItem) {
  // ... 现有逻辑

  // 加载消息历史
  const response = await getSessionHistory(session.session_id)
  const payload = (response as any).data || response
  const msgs = payload.messages || []

  // 获取会话状态
  const sessionStatus = getDisplayStatus(session)

  if (msgs.length > 0) {
    messages.value = rebuildMessagesFromHistory(msgs)

    // 只有已完成的会话才应用结论相关逻辑
    if (sessionStatus === 'completed') {
      applyStructuredFinalAnswer()
      applyParsedExecutionResultFromContent()
    }
  }

  // 加载执行计划、审计、反思、纠正
  if (payload.execution_plan) {
    executionPlan.value = normalizePlanEvent(payload.execution_plan)
  }

  // 审计和反思：如果为空则显示空数组
  auditResults.value = payload.audits || []
  reflectionResults.value = payload.reflections || []
  correctionResults.value = payload.corrections || []

  // 只有已完成的会话才追加运行时事件消息
  if (sessionStatus === 'completed') {
    appendHistoryRuntimeEventMessages(auditResults.value, reflectionResults.value, correctionResults.value)
  }

  // 加载执行结果（仅已完成会话）
  if (sessionStatus === 'completed') {
    await loadExecutionResultForSession(session.session_id, true)
  }

  ElMessage.success('已加载会话')
}
```

### 4.3 结论展示优化

修改结论展示区域，仅显示结论内容：

```vue
<!-- 分析结论展示 - 仅显示结论 -->
<div v-if="executionResult?.conclusion" class="conclusion-section">
  <el-divider content-position="left">分析结论</el-divider>

  <!-- 结论判定 -->
  <div class="conclusion-verdict">
    <el-tag :type="getVerdictType(executionResult.conclusion.verdict)" size="large">
      {{ getVerdictText(executionResult.conclusion.verdict) }}
    </el-tag>
  </div>

  <!-- 结论总结 -->
  <div v-if="executionResult.conclusion.summary" class="conclusion-summary">
    <p>{{ executionResult.conclusion.summary }}</p>
  </div>

  <!-- 处置建议框 - 仅非误报场景显示 -->
  <div v-if="!isFalsePositive(executionResult.conclusion.verdict)" class="remediation-suggestion">
    <el-alert
      title="处置建议"
      type="warning"
      :closable="false"
      show-icon
    >
      <template #default>
        <p>{{ getRemediationSuggestion(executionResult.conclusion.verdict) }}</p>
      </template>
    </el-alert>
  </div>
</div>
```

### 4.4 处置建议生成逻辑

```typescript
function isFalsePositive(verdict: string): boolean {
  return verdict === 'benign' || verdict === 'false_positive'
}

function getRemediationSuggestion(verdict: string): string {
  switch (verdict) {
    case 'malicious':
      return '建议立即隔离受影响主机，进行深入取证分析，并检查横向移动迹象。'
    case 'suspicious':
      return '建议进一步监控相关进程和网络活动，收集更多证据以确认威胁。'
    case 'unknown':
      return '建议人工复核分析结果，结合上下文信息进行判断。'
    default:
      return '建议根据实际情况采取相应措施。'
  }
}
```

---

## 5. 数据库兼容性

### 5.1 现有数据迁移

不需要数据库schema变更，仅修改查询和显示逻辑。

### 5.2 状态兼容处理

对于现有数据中的 `paused` 和 `cancelled` 状态：
- 如果没有 `conclusion`，显示为"未完成"
- 如果有 `conclusion`（理论上不应该），显示为"已完成"

---

## 6. API接口变更

### 6.1 GET /api/v1/detection/alerts/ai-analysis/sessions

**变更**: `status` 参数支持的值从 `active/completed/paused/cancelled` 简化为 `active/completed`

**请求参数**:
- `status`: 可选，`active` 表示未完成，`completed` 表示已完成，不传返回全部

**响应**: 会话列表中的 `status` 字段统一为 `active` 或 `completed`

### 6.2 GET /api/v1/detection/alerts/ai-analysis/{session_id}/history

**变更**: 响应增加 `status` 和 `conclusion` 字段

**响应新增字段**:
- `status`: 会话显示状态 (`active` 或 `completed`)
- `conclusion`: 会话结论（可能为空）

---

## 7. 测试计划

### 7.1 单元测试

#### 后端测试

```go
// TestGetDisplayStatus 测试状态判定逻辑
func TestGetDisplayStatus(t *testing.T) {
    tests := []struct {
        name     string
        session  *model.AISession
        expected string
    }{
        {
            name: "有结论-已完成",
            session: &model.AISession{
                Conclusion: model.JSONB{"verdict": "benign"},
            },
            expected: "completed",
        },
        {
            name: "无结论-未完成",
            session: &model.AISession{
                Conclusion: nil,
            },
            expected: "active",
        },
        {
            name: "空结论-未完成",
            session: &model.AISession{
                Conclusion: model.JSONB{},
            },
            expected: "active",
        },
        {
            name: "状态active无结论-未完成",
            session: &model.AISession{
                Status:     "active",
                Conclusion: nil,
            },
            expected: "active",
        },
        {
            name: "状态paused无结论-未完成",
            session: &model.AISession{
                Status:     "paused",
                Conclusion: nil,
            },
            expected: "active",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := GetDisplayStatus(tt.session)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

#### 前端测试

```typescript
// getDisplayStatus 测试
describe('getDisplayStatus', () => {
  it('有结论应返回completed', () => {
    const session = { conclusion: { verdict: 'benign' } }
    expect(getDisplayStatus(session)).toBe('completed')
  })

  it('无结论应返回active', () => {
    const session = { conclusion: null }
    expect(getDisplayStatus(session)).toBe('active')
  })

  it('空结论应返回active', () => {
    const session = { conclusion: {} }
    expect(getDisplayStatus(session)).toBe('active')
  })
})
```

### 7.2 集成测试

1. **创建会话并完成分析**: 验证状态从 `active` 变为 `completed`
2. **创建会话但取消**: 验证状态保持 `active`
3. **加载未完成会话**: 验证不显示结论UI
4. **加载已完成会话**: 验证显示结论和处置建议
5. **历史会话列表过滤**: 验证按 `active`/`completed` 过滤正确

---

## 8. 验收标准

1. 历史会话列表仅显示"已完成"和"未完成"两种状态
2. 已完成状态仅当会话有结论时显示
3. 未完成会话加载后不显示结论相关UI
4. 审计和反思为空时正确显示为空
5. 结论区域仅显示结论内容，不显示中间步骤
6. 非误报场景显示处置建议框
7. 所有现有测试通过
8. 新增测试覆盖状态判定逻辑

---

## 9. 关联文件

- `api-server/internal/model/ai_session.go` - 会话模型定义
- `api-server/internal/repository/ai_session_repository.go` - 会话仓库
- `api-server/internal/api/handler/ai_analysis_handler.go` - API处理器
- `frontend/src/api/aiAnalysis.ts` - 前端API定义
- `frontend/src/views/detection/AIAnalysis.vue` - 前端页面
- `frontend/src/components/TaskExecutionResult.vue` - 执行结果组件
