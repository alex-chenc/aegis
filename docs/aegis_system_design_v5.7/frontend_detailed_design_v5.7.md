# V5.7 前端详细设计

**版本**: 5.7
**日期**: 2026-05-07
**状态**: 设计中

---

## 1. 变更概述

V5.7前端新增两个页面：**命令审计配置** 和 **审计日志**，以及任务状态展示增强。

---

## 2. 新增页面

### 2.1 命令审计配置页

**路由**: `/settings/command-audit`

**组件结构**:
```
src/views/settings/CommandAudit/
├── index.vue
├── components/
│   ├── AuditPolicyCard.vue       # 审计策略开关（2列网格布局）
│   ├── RuleTable.vue             # 规则列表
│   └── RuleFormDialog.vue        # 新增/编辑对话框
└── composables/useCommandAudit.ts
```

#### 审计策略卡片
采用2列网格布局，每项策略包含图标、名称、描述和开关：
- 黑名单审计（图标B，红色调）- 基于预置规则的确定性检查，命中即拦截
- AI 审计（图标AI，紫色调）- 基于大模型的上下文风险分析，检测隐蔽威胁
- 下发前校验（图标P，橙色调）- 脚本从 API Server 下发前再次校验黑名单
- Agent 侧校验（图标A，绿色调）- Agent 执行前的最后一道防线
- 最大重试次数（1-5）- AI审计失败后重新生成脚本的最大尝试次数

#### 规则列表
- 搜索（名称+模式）
- 筛选：分类、严重等级、匹配类型、状态
- 操作：新增、编辑、删除（预置不可删）、启停
- 预置规则有明显标识

### 2.2 审计日志页

**路由**: `/settings/audit-logs`

**组件结构**:
```
src/views/settings/AuditLogs/
├── index.vue
├── components/
│   ├── AuditStatsCard.vue        # 统计卡片
│   ├── AuditLogTable.vue         # 日志列表
│   └── AuditDetailDrawer.vue     # 详情抽屉
└── composables/useAuditLogs.ts
```

#### 统计卡片
- 总审计次数
- 通过/失败次数
- 通过率（颜色编码：>=90%绿色，<90%黄色）
- 重试分布（1次/2次/3次/失败）

#### 日志列表
- 时间、脚本类型、审计来源、尝试次数
- 结果（通过/失败标签）
- 风险等级（颜色编码）
- 耗时
- 详情按钮

#### 详情抽屉
- 脚本内容（代码高亮）
- 黑名单命中详情（规则名、行号、匹配文本）
- AI审计结果（issues列表）
- 审计时间线（多次尝试的记录）

---

## 3. 现有页面改造

### 3.1 全局按钮可读性修复

**设计文档**: `button_readability_design.md`

全局主题对 Element Plus primary 按钮做样式隔离：

- 普通 `.el-button--primary:not(.is-link)` 保持深蓝实色背景与白字，显式设置文字色、行高和字重。
- 移除 primary 按钮上的 `text-shadow` 与额外 `letter-spacing`，避免短中文按钮在小字号下发糊。
- `.el-button.is-link.el-button--primary` 保持文字链接形态，不继承蓝底白字填充样式。
- hover、active、focus 状态继续通过颜色、浅底和焦点环提供反馈。

### 3.2 任务状态增强

```vue
<!-- 新增audit_blocked状态 -->
<el-tag v-else-if="row.status === 'audit_blocked'" type="danger">
  脚本审计未通过
</el-tag>
```

任务详情页增加审计信息展示：
```
状态: 脚本审计未通过
错误信息: 脚本存在恶意命令，下发已阻止。
命中规则:
  1. [critical] curl管道执行 (第5行)
[查看审计日志] [重新生成脚本]
```

#### API响应格式

`GET /api/v1/tasks/:id/logs` 和 `GET /api/v1/tasks/:id` 对 `AUDIT_BLOCKED` 状态的任务返回 `audit_info` 字段：

```json
{
  "id": "task-uuid",
  "status": "AUDIT_BLOCKED",
  "stderr": "脚本存在恶意命令，下发已阻止。",
  "audit_info": {
    "hit_rules": [
      {"rule_name": "curl管道执行", "severity": "critical", "line_number": 5, "matched_text": "curl | bash"}
    ],
    "error_message": "脚本存在恶意命令，下发已阻止。",
    "audit_log_id": "audit-log-uuid"
  }
}
```

`audit_info` 字段仅在 `status === "AUDIT_BLOCKED"` 时存在，其他状态为 `null`。
```

### 3.3 导航菜单

```typescript
{
  path: '/settings/command-audit',
  name: 'CommandAudit',
  meta: { title: '命令审计配置', icon: 'shield-check' }
},
{
  path: '/settings/audit-logs',
  name: 'AuditLogs',
  meta: { title: '审计日志', icon: 'document-checked' }
}
```

---

## 4. API封装

```typescript
// src/api/command-audit.ts
export const commandAuditApi = {
  getRules: (params) => request.get('/api/v1/settings/command-audit/rules', { params }),
  createRule: (data) => request.post('/api/v1/settings/command-audit/rules', data),
  updateRule: (id, data) => request.put(`/api/v1/settings/command-audit/rules/${id}`, data),
  deleteRule: (id) => request.delete(`/api/v1/settings/command-audit/rules/${id}`),
  toggleRule: (id) => request.put(`/api/v1/settings/command-audit/rules/${id}/toggle`),
  testPattern: (data) => request.post('/api/v1/settings/command-audit/rules/test', data),
  getSettings: () => request.get('/api/v1/settings/command-audit/settings'),
  updateSettings: (data) => request.put('/api/v1/settings/command-audit/settings', data),
}

// src/api/audit-logs.ts
export const auditLogApi = {
  getLogs: (params) => request.get('/api/v1/settings/audit-logs', { params }),
  getLog: (id) => request.get(`/api/v1/settings/audit-logs/${id}`),
  getStats: () => request.get('/api/v1/settings/audit-logs/stats'),
}
```

---

## 5. AI分析页面 — Agent-Runtime 集成改造

**版本**: 5.7
**日期**: 2026-05-12
**关联设计**: `agent_optimization_design.md`

### 5.1 背景

V5.7将AI分析的后端执行引擎从手写ReAct循环替换为 `agent-runtime` SDK。agent-runtime提供完整的 Plan → Execute → Reflect → Audit → Correct → Summarize 生命周期，通过HookSink接口实时推送18种事件。前端需要扩展SSE事件处理和UI展示以支持新的执行流程。

### 5.2 SSE事件类型扩展

**文件**: `frontend/src/api/aiAnalysis.ts`

```typescript
// 改造前
export type SSEEventType = 'thinking' | 'tool_call' | 'tool_result' | 'tool_error'
  | 'content' | 'flowchart_image' | 'done' | 'error'

// 改造后 — 新增6种事件类型
export type SSEEventType = 'thinking' | 'tool_call' | 'tool_result' | 'tool_error'
  | 'content' | 'flowchart_image' | 'done' | 'error'
  | 'plan' | 'step_started' | 'step_completed' | 'audit' | 'reflection' | 'correction'
```

### 5.3 新增接口定义

```typescript
// 执行计划步骤
export interface PlanStep {
  step_id: string
  title: string
  objective: string
  expected_output: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped' | 'replaced' | 'invalidated'
  suggested_tools?: string[]
  dependencies?: string[]
}

// 执行计划事件 (SSE type: 'plan')
export interface PlanEvent {
  plan_id: string
  goal: string
  steps: PlanStep[]
  assumptions?: string[]
}

// 步骤开始事件 (SSE type: 'step_started')
export interface StepStartedEvent {
  step_id: string
  title: string
  objective: string
}

// 步骤完成事件 (SSE type: 'step_completed')
export interface StepCompletedEvent {
  step_id: string
  result?: string
  evidence?: string[]
  duration_ms: number
}

// 审计事件 (SSE type: 'audit')
export interface AuditEvent {
  audit_id: string
  trigger: string
  findings: string[]
  decision: 'continue' | 'minor_adjustment' | 'correct_plan' | 'summarize_now' | 'fail'
  risk_level: 'read_only' | 'low' | 'high' | 'dangerous'
  correction_hint?: string
}

// 反思事件 (SSE type: 'reflection')
export interface ReflectionEvent {
  reflection_id: string
  trigger: string
  root_cause: string
  impact: string
  recoverable: boolean
  recommendation: 'retry_step' | 'skip_step' | 'correct_plan' | 'summarize_now' | 'fail'
}

// 纠正事件 (SSE type: 'correction')
export interface CorrectionEvent {
  correction_id: string
  trigger: string
  reason: string
  actions: Array<{
    type: 'add_step' | 'skip_step' | 'replace_step' | 'split_step' | 'merge_steps' | 'reorder_steps'
    step_id?: string
    new_step_id?: string
    reason: string
  }>
  from_plan_version: number
  to_plan_version: number
}
```

### 5.4 新增 ExecutionPlan 组件

**文件**: `frontend/src/components/ExecutionPlan.vue`

**组件结构**:
```
src/components/
├── ExecutionPlan.vue          # 执行计划面板（新增）
├── AttackGraph.vue            # 攻击图谱（已有）
└── ...
```

**功能设计**:

```
┌─ 执行计划 ──────────────────────────────────┐
│ 目标: 分析反弹Shell攻击链路                    │
│                                              │
│ ① ✓ 收集进程信息          [GetProcessTree]   │
│ ② ✓ 分析网络连接          [GetNetworkConns]  │
│ ③ ⟳ 检查文件操作          [GetOpenFiles]     │
│ ④ ○ 关联历史日志          [QueryHistorical]  │
│ ⑤ ○ 生成攻击图谱          —                  │
│                                              │
│ 审计: 1次 | 反思: 0次 | 纠正: 0次             │
└──────────────────────────────────────────────┘
```

**状态徽章**:
- `pending` — 灰色圆圈 ○
- `running` — 蓝色旋转 ⟳
- `completed` — 绿色勾号 ✓
- `failed` — 红色叉号 ✗
- `skipped` — 灰色跳过 ⏭
- `replaced` — 橙色替换 ↻
- `invalidated` — 灰色失效 ⊘

**折叠行为**:
- 默认展开（首次收到 `plan` 事件时）
- 可折叠为单行摘要："执行计划: 3/5 步骤完成"
- 支持拖拽调整宽度

### 5.5 AIAnalysis.vue 改造

#### 5.5.1 新增响应式数据

```typescript
// 执行计划
const executionPlan = ref<PlanEvent | null>(null)
const currentStepId = ref<string | null>(null)

// 分析过程数据
const auditResults = ref<AuditEvent[]>([])
const reflectionResults = ref<ReflectionEvent[]>([])
const correctionResults = ref<CorrectionEvent[]>([])

// 运行时指标
const runtimeMetrics = ref({
  totalToolCalls: 0,
  totalModelCalls: 0,
  completedSteps: 0,
  totalSteps: 0,
  duration: 0,
})
```

#### 5.5.2 SSE事件处理扩展

```typescript
const createSSEHandler = (sessionId: string) => {
  return (event: SSEEvent) => {
    switch (event.type) {
      // 现有事件处理（不变）
      case 'thinking':   // ...
      case 'tool_call':  // ...
      case 'tool_result': // ...
      case 'tool_error':  // ...
      case 'content':    // ...
      case 'done':       // ...
      case 'error':      // ...

      // 新增事件处理
      case 'plan':
        executionPlan.value = event.content as PlanEvent
        break
      case 'step_started':
        currentStepId.value = (event.content as StepStartedEvent).step_id
        updateStepStatus(currentStepId.value, 'running')
        addThinkingMessage(`步骤开始: ${(event.content as StepStartedEvent).title}`)
        break
      case 'step_completed':
        updateStepStatus((event.content as StepCompletedEvent).step_id, 'completed')
        break
      case 'audit':
        auditResults.value.push(event.content as AuditEvent)
        addThinkingMessage(`审计完成: ${(event.content as AuditEvent).decision}`)
        break
      case 'reflection':
        reflectionResults.value.push(event.content as ReflectionEvent)
        addThinkingMessage(`反思: ${(event.content as ReflectionEvent).root_cause}`)
        break
      case 'correction':
        correctionResults.value.push(event.content as CorrectionEvent)
        addThinkingMessage(`计划纠正: ${(event.content as CorrectionEvent).reason}`)
        break
    }
  }
}
```

#### 5.5.3 左侧面板改造

新增"执行计划"折叠区域，位于告警选择区域下方：

```vue
<template>
  <!-- 现有告警选择区域 -->
  <div class="alert-selection">...</div>

  <!-- 新增：执行计划面板 -->
  <div v-if="executionPlan" class="execution-plan-section">
    <ExecutionPlan
      :plan="executionPlan"
      :current-step-id="currentStepId"
      :audit-results="auditResults"
      :reflection-results="reflectionResults"
      :correction-results="correctionResults"
    />
  </div>

  <!-- 现有会话历史区域 -->
  <div class="session-history">...</div>
</template>
```

#### 5.5.4 右侧消息流改造

新增消息气泡类型：

```vue
<!-- 审计结果气泡 -->
<div v-if="msg.type === 'audit'" class="message-bubble audit-bubble">
  <div class="audit-icon">🔍</div>
  <div class="audit-content">
    <div class="audit-title">进度审计</div>
    <div class="audit-findings">{{ msg.auditResult.findings.join(', ') }}</div>
    <div class="audit-decision">决策: {{ msg.auditResult.decision }}</div>
  </div>
</div>

<!-- 反思结果气泡 -->
<div v-if="msg.type === 'reflection'" class="message-bubble reflection-bubble">
  <div class="reflection-icon">🤔</div>
  <div class="reflection-content">
    <div class="reflection-title">失败反思</div>
    <div class="reflection-cause">根因: {{ msg.reflectionResult.root_cause }}</div>
    <div class="reflection-recommendation">建议: {{ msg.reflectionResult.recommendation }}</div>
  </div>
</div>

<!-- 纠正结果气泡 -->
<div v-if="msg.type === 'correction'" class="message-bubble correction-bubble">
  <div class="correction-icon">🔧</div>
  <div class="correction-content">
    <div class="correction-title">计划纠正</div>
    <div class="correction-reason">{{ msg.correctionResult.reason }}</div>
    <div class="correction-actions">
      <span v-for="action in msg.correctionResult.actions" :key="action.step_id">
        {{ action.type }}: {{ action.reason }}
      </span>
    </div>
  </div>
</div>
```

#### 5.5.5 Message接口扩展

```typescript
interface Message {
  // 现有字段
  role: 'user' | 'assistant' | 'system'
  content: string
  thought?: string
  action?: string
  callId?: string
  actionInput?: Record<string, any>
  observation?: string
  observationError?: string
  toolCalls?: ToolCall[]
  isError?: boolean
  isLoading?: boolean

  // 新增字段
  type?: 'normal' | 'plan' | 'step' | 'audit' | 'reflection' | 'correction'
  planStepId?: string
  auditResult?: AuditEvent
  reflectionResult?: ReflectionEvent
  correctionResult?: CorrectionEvent
}
```

### 5.6 样式设计

```scss
// 执行计划面板
.execution-plan-section {
  border-top: 1px solid var(--el-border-color-lighter);
  padding: 12px 0;
}

// 审计气泡 — 紫色调
.audit-bubble {
  background: linear-gradient(135deg, #f3e8ff, #ede9fe);
  border-left: 3px solid #8b5cf6;
}

// 反思气泡 — 橙色调
.reflection-bubble {
  background: linear-gradient(135deg, #fff7ed, #ffedd5);
  border-left: 3px solid #f97316;
}

// 纠正气泡 — 蓝色调
.correction-bubble {
  background: linear-gradient(135deg, #eff6ff, #dbeafe);
  border-left: 3px solid #3b82f6;
}

// 步骤状态徽章
.step-badge {
  &.pending    { color: #9ca3af; }
  &.running    { color: #3b82f6; animation: spin 1s linear infinite; }
  &.completed  { color: #22c55e; }
  &.failed     { color: #ef4444; }
  &.skipped    { color: #9ca3af; }
  &.replaced   { color: #f97316; }
  &.invalidated { color: #6b7280; }
}
```

### 5.7 向后兼容

- 旧SSE事件类型（thinking, tool_call, tool_result, tool_error, content, done, error）完全不变
- 新增事件类型为可选，前端对未知事件类型做忽略处理
- 无 `plan` 事件时，不显示执行计划面板（兼容旧版后端）
- 无 `audit`/`reflection`/`correction` 事件时，不显示对应气泡

---

## 6. 实施状态

### 6.1 已完成 (V5.7 agent-runtime集成)

| 文件 | 变更 |
|:---|:---|
| `src/api/aiAnalysis.ts` | SSEEventType新增6个类型；新增PlanStep/PlanEvent/AuditEvent/ReflectionEvent/CorrectionEvent接口 |
| `src/components/ExecutionPlan.vue` | 新建组件：可折叠执行计划面板，步骤状态徽章，审计/反思/纠正事件时间线 |
| `src/views/detection/AIAnalysis.vue` | 导入ExecutionPlan组件；新增executionPlan/auditResults/reflectionResults/correctionResults响应式引用；createSSEHandler新增plan/step_started/step_completed/audit/reflection/correction事件处理；Message接口扩展type/planStepId/auditResult/reflectionResult/correctionResult字段；左侧面板添加ExecutionPlan组件；右侧消息流添加审计(紫色)/反思(橙色)/纠正(蓝色)气泡；localStorage持久化新增字段 |

### 6.2 向后兼容验证

- 旧SSE事件类型完全不变，新事件类型为可选
- 无plan事件时ExecutionPlan组件不渲染（v-if="plan"）
- 未知事件类型在switch default中被忽略
