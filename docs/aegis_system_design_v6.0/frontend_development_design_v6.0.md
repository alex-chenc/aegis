# Aegis V6.0 前端开发文档: 智能模式工作台

**版本**: 6.0  
**日期**: 2026-05-29  
**状态**: 设计中

---

## 1. 前端目标

新增智能模式工作台，并把当前普通页面与智能体会话打通。前端重点不是做一个普通聊天框，而是展示“任务计划、工具调用、审批、上下文对象、业务结果”的安全运营工作流。

---

## 2. 视觉与交互基准

参考现有截图：

- `docs/screenshots/ui-refresh/hosts.png`
- `docs/screenshots/ui-refresh/ai_analysis.png`
- `docs/screenshots/ui-refresh/ai_trace.png`

V6.0 继续沿用：

- 深色左侧导航。
- 顶部 `SECURITY OPERATIONS` 面包屑。
- 右上角 API 状态、通知、刷新、用户菜单。
- 浅色主内容区。
- 卡片容器、表格、状态标签。
- 操作按钮使用 Element Plus。
- 智能执行过程采用步骤卡片、标签和时间线。

---

## 3. 路由设计

新增路由：

```ts
{
  path: '/assistant',
  name: 'AssistantWorkspace',
  component: AssistantWorkspace,
  meta: { title: '智能模式' }
}
```

推荐文件：

```text
frontend/src/views/assistant/AssistantWorkspace.vue
frontend/src/views/assistant/components/AssistantSessionSidebar.vue
frontend/src/views/assistant/components/AssistantConversation.vue
frontend/src/views/assistant/components/AssistantComposer.vue
frontend/src/views/assistant/components/AssistantPlanPanel.vue
frontend/src/views/assistant/components/AssistantToolSelectionPanel.vue
frontend/src/views/assistant/components/AssistantToolCallCard.vue
frontend/src/views/assistant/components/AssistantApprovalCard.vue
frontend/src/views/assistant/components/AssistantContextRail.vue
frontend/src/views/assistant/components/AssistantObjectCard.vue
frontend/src/views/assistant/components/AssistantResultRenderer.vue
frontend/src/views/assistant/components/HostAttackInvestigationPanel.vue
frontend/src/views/assistant/components/CompromiseScoreCard.vue
frontend/src/views/assistant/components/EvidenceMatrixTable.vue
frontend/src/views/assistant/components/EntryPointCandidateList.vue
frontend/src/views/assistant/components/AttackTimelineCard.vue
frontend/src/views/assistant/components/AttackPathGraph.vue
frontend/src/views/assistant/components/SourceCoveragePanel.vue
frontend/src/views/assistant/composables/useAssistantStream.ts
frontend/src/views/assistant/composables/useAssistantActions.ts
frontend/src/views/settings/AssistantToolPolicySettings.vue
frontend/src/views/settings/ExternalMCPDataSourceSettings.vue
frontend/src/components/settings/MCPSourceForm.vue
frontend/src/components/settings/MCPSourceToolList.vue
frontend/src/components/settings/MCPQueryLogDrawer.vue
frontend/src/api/assistant.ts
frontend/src/store/assistant.ts
```

---

## 4. App 导航改造

### 4.1 侧边栏新增智能模式

在 [frontend/src/App.vue](/Users/chenchen/Documents/code/aegis/frontend/src/App.vue) 侧边栏加入：

```vue
<el-menu-item index="/assistant">
  <el-icon><ChatDotRound /></el-icon>
  <span>智能模式</span>
</el-menu-item>
```

### 4.2 顶部模式切换

顶部 header 右侧新增 segmented control：

```text
普通模式 | 智能模式
```

设计函数：

```ts
function switchMode(mode: 'normal' | 'assistant'): void
function getCurrentModeByRoute(path: string): 'normal' | 'assistant'
function rememberLastNormalRoute(route: RouteLocationNormalizedLoaded): void
function restoreNormalMode(): void
```

交互：

- 从普通模式切到智能模式：进入 `/assistant`。
- 从智能模式切回普通模式：回到上次普通模式路由，默认 `/hosts`。
- 如果会话中有未审批动作，切换前提示但不阻止。

---

## 5. 智能模式整体布局

```text
+----------------------+--------------------------------+----------------------+
| SessionSidebar        | Conversation                   | ContextRail          |
| - 新建会话            | - 消息流                       | - 上下文对象         |
| - 历史会话            | - 计划卡片                     | - 待审批动作         |
| - 快捷任务            | - 工具调用卡片                 | - 关联任务状态       |
| - 最近对象            | - 结果渲染                     | - 普通模式跳转       |
|                       | - 输入框                       |                      |
+----------------------+--------------------------------+----------------------+
```

组件尺寸建议：

| 区域 | 宽度 |
|:---|:---|
| 左侧会话栏 | 280px |
| 中间对话区 | 自适应，最小 640px |
| 右侧上下文栏 | 360px |

---

## 6. TypeScript 类型

新增文件：`frontend/src/api/assistant.ts`

```ts
export type AssistantTaskType =
  | 'investigation'
  | 'host_attack_investigation'
  | 'operations'
  | 'generation'
  | 'remediation'
  | 'configuration'
  | 'explanation'

export type AssistantSessionStatus =
  | 'active'
  | 'running'
  | 'waiting_approval'
  | 'completed'
  | 'cancelled'
  | 'failed'

export type AssistantRiskLevel =
  | 'readonly'
  | 'low'
  | 'medium'
  | 'high'
  | 'critical'

export type AssistantToolApprovalMode =
  | 'request_approval'
  | 'whitelist'
  | 'full_access'

export interface AssistantSession {
  session_id: string
  title: string
  task_type: AssistantTaskType
  status: AssistantSessionStatus
  mode_source: 'assistant' | 'host' | 'alert' | 'task' | 'package' | 'vulnerability'
  message_count: number
  tool_call_count: number
  approval_count: number
  created_at: string
  updated_at: string
}

export interface AssistantMessage {
  message_id: string
  session_id: string
  role: 'user' | 'assistant' | 'system' | 'tool'
  content: string
  thinking?: string
  plan?: AssistantPlan
  tool_calls?: AssistantToolCall[]
  approvals?: AssistantApproval[]
  result_cards?: AssistantResultCard[]
  created_at: string
}

export interface AssistantContextRef {
  id: string
  session_id: string
  object_type: 'host' | 'alert' | 'task' | 'vulnerability' | 'package' | 'rule' | 'audit_log'
  object_id: string
  title: string
  summary?: string
  route_path?: string
  snapshot?: Record<string, unknown>
  created_at: string
}

export interface AssistantIntentResult {
  domains: string[]
  operations: string[]
  object_types: string[]
  object_ids: string[]
  keywords: string[]
  risk_hint: AssistantRiskLevel
  confidence: number
  reason: string
}

export interface AssistantSelectedTool {
  name: string
  domain: string
  operation: string
  risk: AssistantRiskLevel
  reason: string
}

export interface AssistantToolSelection {
  run_id: string
  stage: 'initial' | 'expanded' | 'approval_resume' | 'retry'
  intent: AssistantIntentResult
  tools: AssistantSelectedTool[]
  tool_count: number
}

export interface AssistantToolSearchResult {
  matches: Array<{
    name: string
    domain: string
    operation: string
    risk: AssistantRiskLevel
    description: string
    args_summary: string
    tags: string[]
  }>
}

export interface AssistantToolPolicy {
  name: string
  domain: string
  operation: string
  risk_level: AssistantRiskLevel
  description: string
  args_summary: string
  default_whitelisted: boolean
  whitelisted: boolean
  enabled: boolean
  updated_at?: string
}

export interface AssistantToolApprovalPolicy {
  mode: AssistantToolApprovalMode
  whitelist_version?: number
  updated_by?: string
  updated_at?: string
}

export interface AssistantPlan {
  plan_id: string
  goal: string
  steps: AssistantPlanStep[]
  status: 'planning' | 'running' | 'completed' | 'failed' | 'cancelled'
}

export interface AssistantPlanStep {
  step_id: string
  title: string
  objective: string
  suggested_tools: string[]
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped' | 'waiting_approval'
  result_summary?: string
}

export interface AssistantToolCall {
  call_id: string
  session_id: string
  tool_name: string
  domain: string
  risk_level: AssistantRiskLevel
  status: 'pending' | 'running' | 'success' | 'failed' | 'approval_required' | 'cancelled'
  args_summary: string
  result_summary?: string
  error_message?: string
  duration_ms?: number
  created_at: string
}

export interface AssistantApproval {
  approval_id: string
  session_id: string
  tool_call_id: string
  tool_name: string
  risk_level: 'medium' | 'high' | 'critical'
  title: string
  impact_summary: string
  params_preview: Record<string, unknown>
  rollback_hint?: string
  status: 'pending' | 'approved' | 'rejected' | 'expired' | 'executed' | 'failed'
  created_at: string
}

export interface AssistantResultCard {
  type: 'host_list' | 'alert_list' | 'task_status' | 'package_summary' | 'attack_graph' | 'host_attack_investigation' | 'evidence_matrix' | 'markdown' | 'json'
  title: string
  payload: Record<string, unknown>
}

export interface HostAttackInvestigationCardPayload {
  investigation_id: string
  host_id: string
  hostname?: string
  verdict: 'confirmed_compromised' | 'suspicious' | 'likely_benign' | 'insufficient_evidence'
  score: number
  confidence: number
  entry_point_candidates: Array<{
    candidate_id: string
    entry_type: string
    title: string
    score: number
    confidence: number
    evidence_ids: string[]
    counter_evidence_ids?: string[]
    explanation: string
  }>
  attack_timeline: Array<{
    event_id: string
    time: string
    phase: string
    title: string
    summary: string
    evidence_ids: string[]
    confidence: number
  }>
  attack_path: {
    nodes: Array<{ node_id: string; node_type: string; label: string; risk_level: string; evidence_ids: string[] }>
    edges: Array<{ from: string; to: string; relation: string; evidence_ids: string[]; confidence: number }>
  }
  evidence_count: number
  missing_evidence: Array<{ source_type: string; reason: string; suggested_tool?: string }>
}
```

---

## 7. API 客户端函数

```ts
export interface CreateAssistantSessionRequest {
  title?: string
  task_type?: AssistantTaskType
  initial_message?: string
  context_refs?: Array<{
    object_type: AssistantContextRef['object_type']
    object_id: string
  }>
}

export interface SendAssistantMessageRequest {
  content: string
  context_refs?: Array<{
    object_type: AssistantContextRef['object_type']
    object_id: string
  }>
}

export interface CreateHostAttackInvestigationRequest {
  session_id?: string
  host_id: string
  alert_ids?: string[]
  cve_ids?: string[]
  time_range?: {
    from: string
    to: string
  }
  include_agent_live?: boolean
  include_external_mcp?: boolean
  mcp_source_ids?: string[]
  max_evidence_items?: number
}

export const assistantApi = {
  listSessions(params?: { page?: number; page_size?: number; status?: string }): Promise<{ data: AssistantSession[]; total: number }>
  createSession(data: CreateAssistantSessionRequest): Promise<AssistantSession>
  getSession(sessionId: string): Promise<AssistantSession>
  getMessages(sessionId: string): Promise<AssistantMessage[]>
  sendMessage(sessionId: string, data: SendAssistantMessageRequest): Promise<{ message_id: string; run_id: string }>
  cancelRun(sessionId: string): Promise<{ session_id: string; status: string }>
  listContextRefs(sessionId: string): Promise<AssistantContextRef[]>
  listToolCalls(sessionId: string): Promise<AssistantToolCall[]>
  listApprovals(sessionId: string): Promise<AssistantApproval[]>
  createHostAttackInvestigation(data: CreateHostAttackInvestigationRequest): Promise<HostAttackInvestigationCardPayload>
  getInvestigation(investigationId: string): Promise<HostAttackInvestigationCardPayload>
  listInvestigationEvidence(investigationId: string, params?: { page?: number; page_size?: number; source_type?: string }): Promise<{ data: unknown[]; total: number }>
  rebuildInvestigationReport(investigationId: string): Promise<HostAttackInvestigationCardPayload>
  approve(approvalId: string, comment?: string): Promise<AssistantApproval>
  reject(approvalId: string, comment?: string): Promise<AssistantApproval>
}
```

SSE 连接函数：

```ts
export function openAssistantStream(
  sessionId: string,
  onEvent: (event: AssistantStreamEvent) => void,
  onError?: (error: Error) => void
): EventSource
```

---

## 8. SSE 事件类型

```ts
export type AssistantStreamEventType =
  | 'message_delta'
  | 'thinking'
  | 'intent_detected'
  | 'tools_selected'
  | 'tool_search'
  | 'tool_expansion'
  | 'plan'
  | 'step_started'
  | 'step_completed'
  | 'tool_call'
  | 'tool_result'
  | 'tool_error'
  | 'approval_required'
  | 'approval_updated'
  | 'context_ref_added'
  | 'result_card'
  | 'done'
  | 'error'

export interface AssistantStreamEvent {
  type: AssistantStreamEventType
  session_id: string
  run_id?: string
  message_id?: string
  payload?: unknown
  error?: string
}
```

处理函数：

```ts
function handleAssistantStreamEvent(event: AssistantStreamEvent): void
function appendAssistantDelta(messageId: string, delta: string): void
function setIntentResult(result: AssistantIntentResult): void
function upsertToolSelection(selection: AssistantToolSelection): void
function appendToolSearchResult(result: AssistantToolSearchResult): void
function upsertPlan(plan: AssistantPlan): void
function upsertToolCall(call: AssistantToolCall): void
function upsertApproval(approval: AssistantApproval): void
function addResultCard(card: AssistantResultCard): void
function markRunDone(runId: string): void
function markRunError(runId: string, error: string): void
```

---

## 9. Pinia Store 设计

文件：`frontend/src/store/assistant.ts`

```ts
export const useAssistantStore = defineStore('assistant', {
  state: () => ({
    sessions: [] as AssistantSession[],
    currentSession: null as AssistantSession | null,
    messages: [] as AssistantMessage[],
    contextRefs: [] as AssistantContextRef[],
    intentResult: null as AssistantIntentResult | null,
    toolSelections: [] as AssistantToolSelection[],
    toolSearchResults: [] as AssistantToolSearchResult[],
    toolCalls: [] as AssistantToolCall[],
    approvals: [] as AssistantApproval[],
    resultCards: [] as AssistantResultCard[],
    streaming: false,
    loading: false,
    error: ''
  }),
  actions: {
    async fetchSessions(params?: SessionQuery): Promise<void> {}
    async createSession(req: CreateAssistantSessionRequest): Promise<AssistantSession> {}
    async openSession(sessionId: string): Promise<void> {}
    async sendMessage(content: string): Promise<void> {}
    async cancelCurrentRun(): Promise<void> {}
    async approveAction(approvalId: string, comment?: string): Promise<void> {}
    async rejectAction(approvalId: string, comment?: string): Promise<void> {}
    applyStreamEvent(event: AssistantStreamEvent): void {}
    resetCurrentSession(): void {}
  }
})
```

---

## 10. 核心组件设计

### 10.1 AssistantWorkspace.vue

职责：

- 加载当前会话。
- 管理三栏布局。
- 连接 SSE。
- 处理 route query 中的上下文对象。

函数：

```ts
function bootstrapWorkspace(): Promise<void>
function createSessionFromRouteContext(): Promise<void>
function handleSessionSelected(sessionId: string): Promise<void>
function handleNewSession(taskType?: AssistantTaskType): Promise<void>
function handleSend(content: string): Promise<void>
```

### 10.2 AssistantSessionSidebar.vue

职责：

- 展示历史会话。
- 新建会话。
- 按任务类型筛选。

Props：

```ts
interface Props {
  sessions: AssistantSession[]
  activeSessionId?: string
  loading: boolean
}
```

Emits：

```ts
defineEmits<{
  select: [sessionId: string]
  create: [taskType?: AssistantTaskType]
  search: [keyword: string]
}>()
```

### 10.3 AssistantConversation.vue

职责：

- 展示消息。
- 展示 plan、tool calls、approval、result cards。
- 管理滚动到底部。

函数：

```ts
function scrollToBottom(): void
function renderMessageContent(message: AssistantMessage): VNode
function groupToolCallsByMessage(messageId: string): AssistantToolCall[]
function groupApprovalsByMessage(messageId: string): AssistantApproval[]
```

### 10.4 AssistantToolSelectionPanel.vue

职责：

- 展示 `intent_detected` 和 `tools_selected` 事件。
- 让用户知道本轮智能体识别到的业务域、动作和风险。
- 展示被注入 agent-runtime 的工具清单。
- 展示 `Tool.Search` 的结果和 `tool_expansion` 的扩展记录。

Props：

```ts
interface Props {
  intent?: AssistantIntentResult | null
  selections: AssistantToolSelection[]
  searches: AssistantToolSearchResult[]
  collapsed?: boolean
}
```

函数：

```ts
function getDomainLabel(domain: string): string
function getRiskTagType(risk: AssistantRiskLevel): 'info' | 'success' | 'warning' | 'danger'
function groupToolsByDomain(selection: AssistantToolSelection): Record<string, AssistantSelectedTool[]>
```

显示规则：

- 默认折叠，避免干扰对话主线。
- 当工具扩展发生时自动展开一次。
- critical 工具用危险色标签，但不显示完整敏感参数。

### 10.5 AssistantApprovalCard.vue

职责：

- 展示待审批动作。
- 批准或拒绝。
- 跳转查看影响对象。

Props：

```ts
interface Props {
  approval: AssistantApproval
  disabled?: boolean
}
```

Emits：

```ts
defineEmits<{
  approve: [approvalId: string, comment?: string]
  reject: [approvalId: string, comment?: string]
  inspect: [approval: AssistantApproval]
}>()
```

函数：

```ts
function riskTagType(level: AssistantRiskLevel): 'info' | 'warning' | 'danger'
function confirmApprove(): Promise<void>
function confirmReject(): Promise<void>
```

### 10.6 AssistantToolPolicySettings.vue

位置：系统配置页新增“智能体工具权限”Tab。

职责：

- 展示三种审批模式。
- 展示全部智能体工具。
- 支持按工具名、详情、领域、风险、白名单状态搜索过滤。
- 支持单个工具加入/移出白名单。
- 支持批量加入/移出白名单。
- 支持恢复默认低危工具白名单。
- 展示每个工具的名称、详情、参数摘要、风险等级和当前白名单状态。

布局：

```text
+---------------------------------------------------------------+
| 工具审批模式:  请求批准 | 白名单 | 完全权限                    |
+---------------------------------------------------------------+
| 搜索工具  领域筛选  风险筛选  白名单筛选  恢复默认白名单       |
+---------------------------------------------------------------+
| 工具名称 | 领域 | 操作 | 风险 | 工具详情 | 参数摘要 | 白名单 |
+---------------------------------------------------------------+
```

函数：

```ts
function fetchToolPolicies(): Promise<void>
function updateApprovalMode(mode: AssistantToolApprovalMode): Promise<void>
function updateToolWhitelist(toolName: string, whitelisted: boolean): Promise<void>
function batchUpdateWhitelist(items: Array<{ tool_name: string; whitelisted: boolean }>): Promise<void>
function resetDefaultWhitelist(): Promise<void>
function getApprovalModeDescription(mode: AssistantToolApprovalMode): string
function getToolRiskTagType(risk: AssistantRiskLevel): 'info' | 'success' | 'warning' | 'danger'
```

交互要求：

- 选择 `request_approval` 时，提示“所有工具调用都将等待人工批准，包含只读查询”。
- 选择 `whitelist` 时，表格白名单开关可用。
- 选择 `full_access` 时，提示“所有被本轮智能体选中的工具将直接执行，仍会记录审计”。
- critical 工具加入白名单时必须二次确认。
- 恢复默认白名单不会改变审批模式。

### 10.7 AssistantContextRail.vue

职责：

- 展示当前会话已关联对象。
- 展示待审批列表。
- 展示最近工具调用状态。
- 提供普通模式跳转。

函数：

```ts
function routeForContextRef(ref: AssistantContextRef): string
function openInNormalMode(ref: AssistantContextRef): void
function removeContextRef(refId: string): Promise<void>
```

### 10.8 ExternalMCPDataSourceSettings.vue

位置：系统配置页新增“外接 MCP 数据源”Tab，和“智能体工具权限”并列。

职责：

- 展示所有已配置外接 MCP 数据源。
- 支持新增、编辑、禁用、删除数据源。
- 支持测试连接。
- 支持同步 schema/tool 摘要。
- 支持查看最近查询日志。
- 明确展示“外部数据进入大模型前会脱敏、截断、标注来源”。

布局：

```text
+--------------------------------------------------------------------+
| 外接 MCP 数据源          新增数据源                                  |
+--------------------------------------------------------------------+
| 搜索  类型筛选  状态筛选                                            |
+--------------------------------------------------------------------+
| 名称 | 类型 | Transport | Endpoint | 状态 | Tool数 | 最近测试 | 操作 |
+--------------------------------------------------------------------+
| prod-siem | siem | streamable_http | https://*** | enabled | 8 | 成功 |
+--------------------------------------------------------------------+
```

类型定义：

```ts
type MCPSourceType =
  | 'siem'
  | 'cmdb'
  | 'edr'
  | 'ticket'
  | 'threat_intel'
  | 'log_warehouse'
  | 'custom'

type MCPTransport = 'streamable_http' | 'sse'

interface ExternalMCPSource {
  source_id: string
  name: string
  source_type: MCPSourceType
  transport: MCPTransport
  endpoint_url_masked: string
  auth_type: 'none' | 'api_key' | 'bearer' | 'basic' | 'oauth2'
  credential_configured: boolean
  enabled: boolean
  description?: string
  schema_cache?: Record<string, unknown>
  query_limits?: {
    max_rows?: number
    timeout_seconds?: number
    max_context_chars?: number
  }
  last_test_status?: 'success' | 'failed'
  last_test_error?: string
  last_test_at?: string
}
```

函数：

```ts
function fetchMCPSources(): Promise<void>
function openCreateSourceDialog(): void
function createMCPSource(input: CreateMCPSourceRequest): Promise<void>
function updateMCPSource(sourceId: string, input: UpdateMCPSourceRequest): Promise<void>
function deleteMCPSource(sourceId: string): Promise<void>
function testMCPSource(sourceId: string): Promise<void>
function syncMCPSchema(sourceId: string): Promise<void>
function openQueryLogDrawer(source: ExternalMCPSource): void
```

交互要求：

- Endpoint 和 credential 不在表格中明文展示。
- credential 输入框只在创建或重置凭据时出现，不回显。
- 点击测试连接时展示 latency、tool_count、错误摘要。
- 点击同步 schema 后展示外部 MCP tools/fields 摘要。
- 删除数据源必须二次确认。
- 禁用数据源后智能体不能再选择该 source。

### 10.9 HostAttackInvestigationPanel.vue

职责：

- 渲染 `host_attack_investigation` result card。
- 展示是否被攻击判断、分数、置信度。
- 展示攻击入口候选、证据矩阵、攻击时间线、攻击路径图和缺失证据。
- 支持从证据跳转普通模式对应页面。

布局：

```text
+---------------------------------------------------------------+
| 主机攻击研判  verdict/score/confidence                         |
+----------------------------+----------------------------------+
| 入口候选                    | 数据源覆盖                       |
+----------------------------+----------------------------------+
| 攻击时间线                                                     |
+---------------------------------------------------------------+
| 攻击路径图                                                     |
+---------------------------------------------------------------+
| 证据矩阵                                                       |
+---------------------------------------------------------------+
| 建议动作                                                       |
+---------------------------------------------------------------+
```

Props：

```ts
interface Props {
  payload: HostAttackInvestigationCardPayload
  evidenceExpanded?: boolean
}
```

函数：

```ts
function verdictLabel(verdict: HostAttackInvestigationCardPayload['verdict']): string
function verdictTagType(verdict: HostAttackInvestigationCardPayload['verdict']): 'success' | 'warning' | 'danger' | 'info'
function scoreColor(score: number): string
function openEvidence(evidenceId: string): void
function routeForEvidence(evidenceId: string): string
function groupTimelineByPhase(events: HostAttackInvestigationCardPayload['attack_timeline']): Record<string, typeof events>
function renderAttackPathGraph(graph: HostAttackInvestigationCardPayload['attack_path']): void
```

交互要求：

- `confirmed_compromised` 使用危险态，但仍必须展示证据引用。
- `suspicious` 使用告警态，突出“需继续确认”。
- `likely_benign` 使用正常态，但保留告警证据和反证。
- `insufficient_evidence` 使用信息态，不渲染为已失陷。
- 入口候选必须同时展示支持证据和反证数量。
- 高风险建议动作只展示“发起审批”或“查看建议”，不能在卡片里直接执行。

---

## 11. 普通页面接入“交给智能体”

通用组件：

```text
frontend/src/components/assistant/AskAssistantButton.vue
```

Props：

```ts
interface AskAssistantButtonProps {
  objectType: AssistantContextRef['object_type']
  objectIds: string[]
  promptTemplate?: string
  buttonText?: string
  size?: 'small' | 'default'
}
```

函数：

```ts
function buildAssistantRoute(): RouteLocationRaw
function createContextRefs(): Array<{ object_type: string; object_id: string }>
```

接入页面：

| 页面 | object_type | 默认提示词 |
|:---|:---|:---|
| Dashboard.vue | host | 分析这些主机的安全态势 |
| TaskDetail.vue | task | 解释这个任务的执行结果 |
| Vulnerability.vue | vulnerability | 分析这个 CVE 的影响和修复路径 |
| Alerts.vue | alert | 对这些告警做攻击溯源 |
| PackageDetail.vue | package | 分析这个检测包的构建和运行状态 |
| AuditLogs | audit_log | 总结这些审计日志中的风险 |

---

## 12. 前端测试用例

| 测试 | 文件 | 断言 |
|:---|:---|:---|
| API client | `frontend/src/api/assistant.test.ts` | endpoint、参数、响应类型 |
| Store | `frontend/src/store/assistant.test.ts` | SSE event 合并、审批更新 |
| Workspace | `AssistantWorkspace.test.ts` | 会话创建、消息发送、三栏渲染 |
| ApprovalCard | `AssistantApprovalCard.test.ts` | 批准、拒绝、风险标签 |
| AskAssistantButton | `AskAssistantButton.test.ts` | 正确生成 route context |
| MCP settings | `ExternalMCPDataSourceSettings.test.ts` | 数据源列表、新增、测试连接、schema 同步、凭据不回显 |

---

## 13. 可访问性与状态

- 所有 icon button 必须有 tooltip。
- 审批按钮必须明确显示动作和风险。
- SSE 断开时显示“连接中断，可重试”。
- LLM 未配置时展示模型配置跳转。
- 工具调用失败时展示错误摘要和“复制参数”按钮。
- 结果卡片必须提供普通模式跳转入口。

