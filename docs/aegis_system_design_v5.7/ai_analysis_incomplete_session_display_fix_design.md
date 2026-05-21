# AI 分析历史未完成会话显示修复设计文档

**版本**: 5.7
**状态**: 已实现
**适用范围**: `aiAnalysisRuntime.ts` 计划事件规范化；`AIAnalysis.vue` 会话加载与 UI 显示逻辑

## 1. 问题描述

### 1.1 现象

加载历史未完成会话（例如 `43654068-24bf-4bef-8263-516f85a3ac04`）时：

- **左侧计划栏**：显示全部 5 个步骤为"已完成"，但实际只有 3 个步骤完成
- **底部 UI**：未完成会话错误地显示了结论、执行结果、审计、反思、修正等 UI 面板 — 未完成会话应仅显示原始消息和计划（包含实际步骤状态）

### 1.2 期望行为

- 未完成会话的计划栏应反映后端 `FinalPlan` 中的真实步骤状态（如 3 个 completed、1 个 running、1 个 pending）
- 未完成会话不应显示结论、审计、反思、修正等仅适用于已完成会话的 UI 组件
- 已完成会话的行为不受影响

## 2. 根因分析

### 2.1 缺陷 1：`normalizePlanEvent()` 丢弃真实步骤状态

**代码位置**：`frontend/src/utils/aiAnalysisRuntime.ts:41`

```typescript
return {
  ...input,
  steps: (input.steps || []).map(s => ({
    ...s,
    status: 'pending' as const,   // ← 强制覆盖为 pending
  })),
}
```

**问题**：无论后端 `FinalPlan` 中步骤的实际状态是什么，前端一律将 `status` 覆盖为 `'pending'`。后端 `FinalPlan` 包含了每个步骤的真实执行状态（如 `completed`、`running`、`failed`），但前端在规范化时全部丢弃。

**影响**：当后续流程将非 failed 步骤批量设为 `completed` 时（见缺陷 3），所有步骤都从 `pending` 变为 `completed`，掩盖了真实的步骤状态。

### 2.2 缺陷 2：执行结果错误地将会话升级为"已完成"

**代码位置**：`frontend/src/views/detection/AIAnalysis.vue:1053-1071`

```typescript
// persistAgentResult() 在 persistAnalysisOutcome() 之前执行
await persistAgentResult(sessionId, result)
// ...
if (sessionStatus !== 'completed') {
  sessionStatus = 'completed'   // ← 有执行结果就认为完成
}
```

**问题**：

1. `persistAgentResult()` 在 `persistAnalysisOutcome()` 之前调用，意味着执行结果（`agent_executions` 记录）先于结论被保存
2. 未完成会话**可以**有执行结果 — 步骤在执行过程中会产生部分结果
3. 前端将"存在执行结果"等同于"会话已完成"，导致未完成会话被错误标记为 `completed`

**影响**：未完成会话被升级为 `completed` 后，前端渲染完整的已完成 UI（结论、审计、反思、修正），与实际状态不符。

### 2.3 缺陷 3：批量步骤状态覆盖掩盖真实状态

**代码位置**：`frontend/src/views/detection/AIAnalysis.vue:1026-1031`

```typescript
// 批量将非 failed 步骤设为 completed
plan.steps.forEach(s => {
  if (s.status !== 'failed') {
    s.status = 'completed'
  }
})
```

**问题**：在计划事件处理中，所有非 `failed` 的步骤被统一设置为 `completed`。这忽略了 `running`、`pending` 等中间状态，与后端 `FinalPlan` 中的实际状态不一致。

**影响**：即使缺陷 1 被修复（保留了后端状态），此批量覆盖仍会将所有非 failed 步骤设为 completed。

### 2.4 根因总结

| 层级 | 组件 | 问题 | 影响 |
|------|------|------|------|
| 状态规范化 | `normalizePlanEvent()` | 强制覆盖步骤状态为 `pending` | 丢失后端真实状态 |
| 会话状态判断 | `persistAgentResult` 后的状态升级 | 执行结果 ≠ 完成 | 未完成会话被误标为已完成 |
| 步骤状态覆盖 | 批量 `status = 'completed'` | 忽略中间状态 | 计划显示 5/5 完成 |
| UI 渲染 | 无状态守卫 | 不区分完成/未完成 | 未完成会话显示完整 UI |

## 3. 修复方案

### 3.1 修复 1：`normalizePlanEvent()` 保留后端步骤状态

**修改文件**：`frontend/src/utils/aiAnalysisRuntime.ts`

新增 `normalizeStepStatus()` 辅助函数，校验并保留后端传入的步骤状态：

```typescript
const knownStatuses = new Set(['pending', 'running', 'completed', 'failed', 'skipped'])

function normalizeStepStatus(raw: unknown): StepStatus {
  if (typeof raw === 'string' && knownStatuses.has(raw)) {
    return raw as StepStatus
  }
  return 'pending'
}

export function normalizePlanEvent(input: unknown): PlanEvent {
  // ...existing validation...
  return {
    ...validated,
    steps: (validated.steps || []).map(s => ({
      ...s,
      status: normalizeStepStatus(s.status),  // ← 保留后端状态，仅校验
    })),
  }
}
```

**设计决策**：

- 已知状态集合（`knownStatuses`）作为白名单，确保类型安全
- 未知或缺失状态默认为 `pending`（安全降级）
- 不再强制覆盖，后端 `FinalPlan` 的状态被原样保留

### 3.2 修复 2：移除基于执行结果的状态升级

**修改文件**：`frontend/src/views/detection/AIAnalysis.vue`

移除"有执行结果就将会话标记为已完成"的逻辑：

```typescript
// 修改前
if (sessionStatus !== 'completed') {
  sessionStatus = 'completed'
}

// 修改后：移除此逻辑块
// 会话状态由结论是否存在决定，不由执行结果决定
```

**修正后的会话状态判定逻辑**：

- **已完成**：存在 `conclusion`（即后端已调用 `persistAnalysisOutcome`）
- **未完成**：无 `conclusion`，即使存在执行结果

### 3.3 修复 3：移除批量步骤状态覆盖

**修改文件**：`frontend/src/views/detection/AIAnalysis.vue`

删除批量将非 failed 步骤设为 completed 的循环：

```typescript
// 修改前
plan.steps.forEach(s => {
  if (s.status !== 'failed') {
    s.status = 'completed'
  }
})

// 修改后：移除此循环
// FinalPlan 来自后端，步骤状态已是正确的
```

### 3.4 修复 4：未完成会话 UI 保护

**修改文件**：`frontend/src/views/detection/AIAnalysis.vue`

**新增 `currentSessionStatus` 响应式引用**：

```typescript
const currentSessionStatus = ref<string>('pending')
```

在会话加载时更新状态：

```typescript
// loadSession() / loadConversation() 中
currentSessionStatus.value = session.conclusion ? 'completed' : 'incomplete'
```

**ExecutionPlan props 守卫**：

```html
<ExecutionPlan
  :audits="currentSessionStatus === 'completed' ? audits : []"
  :reflections="currentSessionStatus === 'completed' ? reflections : []"
  :corrections="currentSessionStatus === 'completed' ? corrections : []"
/>
```

**执行结果面板守卫**：

```html
<div v-if="executionResult && currentSessionStatus === 'completed'">
  <!-- 执行结果详情 -->
</div>
```

**溯源图卡片守卫**：

```html
<AttackGraphCard v-if="attackGraph && currentSessionStatus === 'completed'" />
```

## 4. 测试用例

### 4.1 单元测试（`aiAnalysisRuntime.test.ts`）

| 编号 | 测试场景 | 输入 | 预期输出 |
|------|---------|------|---------|
| T1 | `normalizePlanEvent` 保留 completed 状态 | `steps: [{status: 'completed'}, {status: 'completed'}]` | 状态原样保留为 `completed` |
| T2 | `normalizePlanEvent` 保留混合状态 | `steps: [{status: 'completed'} x3, {status: 'running'} x1, {status: 'pending'} x1]` | 3 个 `completed`、1 个 `running`、1 个 `pending` |
| T3 | 未知状态默认为 pending | `steps: [{status: 'unknown_value'}]` | 状态为 `pending` |
| T4 | 缺失状态默认为 pending | `steps: [{}]` | 状态为 `pending` |
| T5 | `isPlanTerminal` 对非终态返回 false | `status: 'running'` | `false` |
| T6 | `isPlanTerminal` 对所有终态返回 true | `status: 'completed'` / `'failed'` / `'skipped'` | `true` |

### 4.2 集成验证

```bash
# 获取 token
TOKEN=$(curl -s -X POST http://localhost:8082/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"<password>"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")

# 加载未完成会话
SESSION_ID="43654068-24bf-4bef-8263-516f85a3ac04"

# 验证计划步骤状态
curl -s "http://localhost:8082/api/v1/detection/alerts/ai-analysis/$SESSION_ID" \
  -H "Authorization: Bearer $TOKEN" | python3 -c "
import sys, json
data = json.load(sys.stdin)['data']
status = data.get('status')
assert status != 'completed', f'Session should not be completed, got {status}'
print(f'PASS: Session status is {status}')
"

# 前端验证（手动检查）
# 1. 打开会话页面
# 2. 计划栏应显示 3/5 completed（而非 5/5）
# 3. 不应显示结论、审计、反思、修正 UI
# 4. 不应显示执行结果面板和溯源图卡片
```

## 5. 影响范围

### 5.1 修改文件

| 文件 | 修改内容 |
|------|---------|
| `frontend/src/utils/aiAnalysisRuntime.ts` | 新增 `normalizeStepStatus()` 辅助函数；修改 `normalizePlanEvent()` 保留后端状态 |
| `frontend/src/views/detection/AIAnalysis.vue` | 移除执行结果状态升级；移除批量步骤覆盖；新增 `currentSessionStatus` 守卫 |
| `frontend/src/utils/aiAnalysisRuntime.test.ts` | 新增 6 个单元测试用例 |

### 5.2 不修改文件

| 文件 | 原因 |
|------|------|
| `ExecutionPlan.vue` | 仅消费 props，不负责状态逻辑 |
| `ai_analysis_handler.go` | 后端已正确存储 `FinalPlan` 和步骤状态 |
| `sessionStatus.ts` | 会话状态管理逻辑不变 |

## 6. 关联设计文档

| 文档 | 关系 |
|------|------|
| `ai_analysis_conclusion_storage_fix_design.md` | 修复结论存储格式，本修复依赖结论作为"已完成"判定依据 |
| `ai_analysis_refresh_conclusion_fix_design.md` | 修复刷新后结论丢失，确保结论可用性 |
| `ai_analysis_history_ui_fix_design.md` | 历史会话 UI 通用修复 |
| `session_status_simplification_design.md` | 会话状态简化，本修复与之一致（结论存在 = 已完成） |

## 7. 实现验证结果

### 7.1 单元测试

```
 ✓ src/utils/aiAnalysisRuntime.test.ts  (9 tests) 3ms
   ✓ normalizes agent-runtime plan fields for the execution plan UI
   ✓ preserves mixed step statuses from backend FinalPlan
   ✓ defaults unknown or missing step status to pending
   ✓ isPlanTerminal returns false when steps have non-terminal statuses
   ✓ isPlanTerminal returns true when all steps are in terminal statuses
   ✓ updates steps by either normalized id or runtime step_id and detects terminal state
   ✓ getActionButtonType returns pause when loading and no input
   ✓ getActionButtonType returns send when loading but user has input
   ✓ getActionButtonType returns send when not loading
```

### 7.2 后端测试

```
PASS: TestGetDisplayStatus (8 子测试全部通过)
```

### 7.3 API 验证（会话 43654068-24bf-4bef-8263-516f85a3ac04）

**会话历史 API**:
- Status: `active`（未完成）
- Conclusion: `None`（无结论）
- Messages: 2 条
- Audits: 1 条（但前端不会为未完成会话显示）
- Reflections: 1 条（但前端不会为未完成会话显示）

**执行计划步骤状态**（来自 FinalPlan）:
- Step 1: `completed` — 主机现状与进程存活检查
- Step 2: `completed` — 网络连接与端口监听分析
- Step 3: `completed` — 进程血缘链与用户会话溯源
- Step 4: `retrying` — 历史日志审计与持久化排查
- Step 5: `pending` — 综合研判与处置建议生成

**执行结果 API**:
- Status: `已限制`（limited）
- Exit reason: `max_model_failures`
- 会话未完成，但执行结果存在（确认缺陷 2 的根因）

### 7.4 前端构建

```
✓ 2935 modules transformed.
✓ built in 8.49s
```

### 7.5 修复前后对比

| 场景 | 修复前 | 修复后 |
|------|--------|--------|
| 计划栏完成数 | 5/5（全部标记为完成） | 3/5（保留真实状态） |
| 结论 UI | 显示（错误） | 不显示 |
| 执行结果面板 | 显示（错误） | 不显示 |
| 审计/反思/修正 | 显示（错误） | 不显示 |
| 溯源图 | 显示（错误） | 不显示 |
