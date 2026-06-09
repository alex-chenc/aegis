import request from './index'
import { getAuthToken } from '@/utils/auth'

// ============================================
// 类型定义 - V6.0 AI 助手
// ============================================

/** 任务类型 */
export type AssistantTaskType =
  | 'investigation'
  | 'host_attack_investigation'
  | 'operations'
  | 'generation'
  | 'remediation'
  | 'configuration'
  | 'explanation'

/** 会话状态 */
export type AssistantSessionStatus =
  | 'active'
  | 'running'
  | 'waiting_approval'
  | 'completed'
  | 'cancelled'
  | 'failed'

/** 风险等级 */
export type AssistantRiskLevel =
  | 'readonly'
  | 'low'
  | 'medium'
  | 'high'
  | 'critical'

/** 工具审批模式 */
export type AssistantToolApprovalMode =
  | 'request_approval'
  | 'whitelist'
  | 'full_access'

// ============================================
// 接口定义
// ============================================

/** 助手会话 */
export interface AssistantSession {
  id: string
  session_id: string
  title: string
  task_type: AssistantTaskType
  mode_source?: string
  status: AssistantSessionStatus
  created_by?: string
  message_count?: number
  tool_call_count?: number
  approval_count?: number
  metadata?: Record<string, any>
  created_at: string
  updated_at: string
}

/** 助手消息 */
export interface AssistantMessage {
  id: string
  session_id: string
  message_id: string
  role: 'user' | 'assistant' | 'system' | 'tool'
  content: string
  thinking?: string | string[]  // 支持字符串（兼容旧数据）或数组（新格式）
  plan?: {
    goal: string
    status: string
    steps: Array<{
      step_id: string
      title: string
      status: string
      result_summary?: string
    }>
  }
  tool_calls?: AssistantToolCall[]
  approvals?: AssistantApproval[]
  result_cards?: AssistantResultCard[]
  context_refs?: AssistantContextRef[]
  created_at: string
}

/** 上下文引用 */
export interface AssistantContextRef {
  id: string
  session_id: string
  object_type: string
  object_id: string
  title?: string
  summary?: string
  route_path?: string
  snapshot?: Record<string, any>
  created_at: string
}

/** 工具调用 */
export interface AssistantToolCall {
  id: string
  session_id: string
  message_id: string
  call_id: string
  tool_name: string
  domain?: string
  risk_level: AssistantRiskLevel
  status: 'pending' | 'running' | 'completed' | 'success' | 'failed' | 'cancelled' | 'approval_required' | 'rejected'
  args?: Record<string, any>
  args_summary?: string
  result?: any
  result_summary?: string
  error_message?: string
  duration_ms?: number
  created_at: string
  updated_at?: string
}

/** 工具审批 */
export interface AssistantApproval {
  id: string
  approval_id: string
  session_id: string
  tool_call_id: string
  tool_name: string
  risk_level: AssistantRiskLevel
  title: string
  impact_summary?: string
  params_preview?: Record<string, any>
  rollback_hint?: string
  status: 'pending' | 'approved' | 'rejected' | 'expired' | 'executed' | 'failed'
  requested_by: string
  reviewed_by?: string
  review_comment?: string
  expires_at?: string
  created_at: string
  reviewed_at?: string
}

/** 工具策略 */
export interface AssistantToolPolicy {
  mode: AssistantToolApprovalMode
}

/** 工具白名单条目 */
export interface AssistantToolWhitelistEntry {
  tool_name: string
  risk_level: AssistantRiskLevel
  whitelisted: boolean
  default_whitelisted?: boolean
  enabled?: boolean
}

/** 工具信息 */
export interface AssistantTool {
  id?: string
  name?: string
  tool_name?: string
  domain?: string
  operation?: string
  description: string
  risk_level: AssistantRiskLevel
  input_schema?: Record<string, any>
  args_summary?: string
  category?: string
  whitelisted?: boolean
  default_whitelisted?: boolean
  enabled?: boolean
}

export interface AssistantToolsResponse {
  tools?: AssistantTool[]
  items?: AssistantTool[]
  total: number
}

/** MCP 数据源 */
export interface AssistantMCPSource {
  id: string
  name: string
  type: string
  config: Record<string, any>
  status: 'active' | 'inactive' | 'error'
  tool_count?: number
  last_sync_at?: string
  created_at: string
  updated_at: string
}

/** 攻击调查 */
export interface AssistantInvestigation {
  id: string
  session_id: string
  host_id: string
  hostname: string
  status: 'running' | 'completed' | 'failed'
  summary?: string
  evidence_count?: number
  created_at: string
  updated_at: string
}

/** 调查证据 */
export interface AssistantEvidence {
  id: string
  investigation_id: string
  evidence_type: string
  source: string
  content: string
  severity: string
  collected_at: string
}

/** 结果卡片 */
export interface AssistantResultCard {
  id: string
  /** 卡片类型 — 兼容后端 card_type 和 type 两种字段名 */
  type: 'markdown' | 'json' | 'host_list' | 'alert_list' | 'host_attack_investigation' | 'attack_graph' | 'evidence_matrix' | 'task_status' | 'package_summary'
  title: string
  /** 卡片数据 — 兼容后端 data 和 payload 两种字段名 */
  payload: Record<string, any>
  created_at: string
}

/** 入口候选 */
export interface EntryPointCandidate {
  candidate_id: string
  entry_type: string
  title: string
  score: number
  confidence: number
  first_seen_at?: string
  evidence_ids: string[]
  counter_evidence_ids?: string[]
  related_cve_ids?: string[]
  related_baseline_ids?: string[]
  explanation: string
}

/** 攻击时间线事件 */
export interface AttackTimelineEvent {
  event_id: string
  time: string
  phase: string
  title: string
  summary: string
  evidence_ids: string[]
  confidence: number
}

/** 攻击路径图 */
export interface AttackPathGraph {
  nodes: Array<{ node_id: string; node_type: string; label: string; risk_level: string; evidence_ids: string[] }>
  edges: Array<{ from: string; to: string; relation: string; evidence_ids: string[]; confidence: number }>
}

/** 缺失证据 */
export interface MissingEvidence {
  source_type: string
  reason: string
  suggested_tool?: string
}

/** 数据源覆盖 */
export interface SourceCoverage {
  sources: Array<{
    source_type: string
    source_name: string
    available: boolean
    evidence_count: number
    error?: string
  }>
  total_sources: number
  available_sources: number
}

/** 推荐动作 */
export interface RecommendedAction {
  action_id: string
  category: 'immediate_forensics' | 'temporary_containment' | 'remediation' | 'detection_enhancement'
  title: string
  description: string
  risk_level: AssistantRiskLevel
  requires_approval: boolean
  related_evidence_ids: string[]
}

/** 主机攻击研判结果载荷 */
export interface HostAttackInvestigationCardPayload {
  investigation_id: string
  host_id: string
  hostname?: string
  verdict: 'confirmed_compromised' | 'suspicious' | 'likely_benign' | 'insufficient_evidence'
  score: number
  confidence: number
  summary?: string
  key_reasons?: string[]
  contradictions?: string[]
  entry_point_candidates: EntryPointCandidate[]
  attack_timeline: AttackTimelineEvent[]
  attack_path: AttackPathGraph
  evidence_count: number
  missing_evidence: MissingEvidence[]
  source_coverage?: SourceCoverage
  recommended_actions?: RecommendedAction[]
  report_markdown?: string
}

// ============================================
// 请求/响应类型
// ============================================

/** 创建会话请求 */
export interface CreateSessionRequest {
  title?: string
  task_type?: AssistantTaskType
  initial_message?: string
  context_refs?: Array<{
    object_type: string
    object_id: string
  }>
}

/** 发送消息请求 */
export interface SendMessageRequest {
  content: string
  context_refs?: Array<{ object_type: string; object_id: string }>
}

/** 发送消息响应（运行句柄） */
export interface RunHandle {
  run_id: string
  message_id: string
}

/** 创建调查请求 */
export interface CreateInvestigationRequest {
  host_id: string
  time_range?: { start: string; end: string }
  focus_areas?: string[]
}

/** 创建 MCP 数据源请求 */
export interface CreateMCPSourceRequest {
  name: string
  type: string
  config: Record<string, any>
}

/** 更新 MCP 数据源请求 */
export interface UpdateMCPSourceRequest {
  name?: string
  config?: Record<string, any>
  status?: 'active' | 'inactive'
}

/** 更新工具白名单请求 */
export interface UpdateToolWhitelistRequest {
  whitelisted: boolean
}

/** 批量更新白名单请求 */
export interface BatchUpdateWhitelistRequest {
  items: { tool_name: string; whitelisted: boolean }[]
}

/** 审批操作请求 */
export interface ApprovalActionRequest {
  comment?: string
}

export interface ApprovalActionResult {
  approval: AssistantApproval
  tool_result?: {
    success: boolean
    data?: any
    error?: string
    duration_ms?: number
  }
}

/** 分页查询参数 */
export interface PaginationParams {
  page?: number
  page_size?: number
}

/** 会话查询参数 */
export interface SessionsQueryParams extends PaginationParams {
  status?: AssistantSessionStatus
  task_type?: AssistantTaskType
  keyword?: string
}

/** 工具调用查询参数 */
export interface ToolCallsQueryParams extends PaginationParams {
  status?: string
  tool_name?: string
}

/** 审批查询参数 */
export interface ApprovalsQueryParams extends PaginationParams {
  status?: string
}

/** MCP 数据源查询参数 */
export interface MCPSourcesQueryParams extends PaginationParams {
  type?: string
  status?: string
}

/** 证据查询参数 */
export interface EvidenceQueryParams extends PaginationParams {
  evidence_type?: string
  severity?: string
}

/** 分页响应 */
export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

/** 会话列表响应：后端返回 sessions，部分旧接口返回 items */
export interface AssistantSessionsResponse {
  sessions?: AssistantSession[]
  items?: AssistantSession[]
  total: number
  page?: number
  page_size?: number
}

export type AssistantFileUploadPurpose = 'analysis' | 'baseline_template' | 'sigma_rule'

export interface AssistantFileUploadResult {
  purpose: AssistantFileUploadPurpose
  filename: string
  size: number
  context_ref: AssistantContextRef
  data?: Record<string, any>
}

// ============================================
// API 函数
// ============================================

/** 获取会话列表 */
export function getSessions(params?: SessionsQueryParams) {
  return request<any, AssistantSessionsResponse>({
    url: '/assistant/sessions',
    method: 'get',
    params
  })
}

/** 创建会话 */
export function createSession(data: CreateSessionRequest) {
  return request<any, AssistantSession>({
    url: '/assistant/sessions',
    method: 'post',
    data
  })
}

/** 获取会话详情 */
export function getSession(sessionId: string) {
  return request<any, AssistantSession>({
    url: `/assistant/sessions/${sessionId}`,
    method: 'get'
  })
}

/** 获取会话消息列表 */
export function getMessages(sessionId: string) {
  return request<any, AssistantMessage[]>({
    url: `/assistant/sessions/${sessionId}/messages`,
    method: 'get'
  })
}

/** 发送消息 */
export function sendMessage(sessionId: string, data: SendMessageRequest) {
  return request<any, RunHandle>({
    url: `/assistant/sessions/${sessionId}/message`,
    method: 'post',
    data
  })
}

/** 取消运行 */
export function cancelRun(sessionId: string) {
  return request<any, void>({
    url: `/assistant/sessions/${sessionId}/cancel`,
    method: 'post'
  })
}

/** 获取上下文引用 */
export function getContextRefs(sessionId: string) {
  return request<any, AssistantContextRef[]>({
    url: `/assistant/sessions/${sessionId}/context-refs`,
    method: 'get'
  })
}

/** 上传文件并附加到会话上下文 */
export function uploadAssistantFile(sessionId: string, file: File, purpose: AssistantFileUploadPurpose = 'analysis') {
  const formData = new FormData()
  formData.append('file', file)
  formData.append('purpose', purpose)
  return request<any, AssistantFileUploadResult>({
    url: `/assistant/sessions/${sessionId}/files`,
    method: 'post',
    data: formData
  })
}

/** 获取工具调用列表 */
export function getToolCalls(sessionId: string, params?: ToolCallsQueryParams) {
  return request<any, PaginatedResponse<AssistantToolCall>>({
    url: `/assistant/sessions/${sessionId}/tool-calls`,
    method: 'get',
    params
  })
}

/** 获取审批列表 */
export function getApprovals(sessionId: string, params?: ApprovalsQueryParams) {
  return request<any, PaginatedResponse<AssistantApproval>>({
    url: `/assistant/sessions/${sessionId}/approvals`,
    method: 'get',
    params
  })
}

/** 获取可用工具列表 */
export function getTools(params?: PaginationParams) {
  return request<any, AssistantToolsResponse>({
    url: '/assistant/tools',
    method: 'get',
    params
  })
}

/** 获取工具审批策略 */
export function getToolApprovalPolicy() {
  return request<any, AssistantToolPolicy>({
    url: '/assistant/tool-approval-policy',
    method: 'get'
  })
}

/** 更新工具审批策略 */
export function updateToolApprovalPolicy(data: Partial<AssistantToolPolicy>) {
  return request<any, void>({
    url: '/assistant/tool-approval-policy',
    method: 'put',
    data
  })
}

/** 更新工具白名单 */
export function updateToolWhitelist(toolName: string, data: UpdateToolWhitelistRequest) {
  return request<any, AssistantToolWhitelistEntry>({
    url: `/assistant/tools/${toolName}/whitelist`,
    method: 'put',
    data
  })
}

/** 批量更新白名单 */
export function batchUpdateWhitelist(data: BatchUpdateWhitelistRequest) {
  return request<any, AssistantToolWhitelistEntry[]>({
    url: '/assistant/tools/whitelist/batch',
    method: 'post',
    data
  })
}

/** 重置白名单默认值 */
export function resetWhitelistDefaults() {
  return request<any, void>({
    url: '/assistant/tools/whitelist/reset-defaults',
    method: 'post'
  })
}

/** 获取审批详情 */
export function getApproval(approvalId: string) {
  return request<any, AssistantApproval>({
    url: `/assistant/approvals/${approvalId}`,
    method: 'get'
  })
}

/** 通过审批 */
export function approveApproval(approvalId: string, data?: ApprovalActionRequest) {
  return request<any, ApprovalActionResult>({
    url: `/assistant/approvals/${approvalId}/approve`,
    method: 'post',
    data
  })
}

/** 拒绝审批 */
export function rejectApproval(approvalId: string, data?: ApprovalActionRequest) {
  return request<any, AssistantApproval>({
    url: `/assistant/approvals/${approvalId}/reject`,
    method: 'post',
    data
  })
}

/** 创建主机攻击调查 */
export function createInvestigation(data: CreateInvestigationRequest) {
  return request<any, AssistantInvestigation>({
    url: '/assistant/investigations/host-attack',
    method: 'post',
    data
  })
}

/** 获取调查详情 */
export function getInvestigation(investigationId: string) {
  return request<any, AssistantInvestigation>({
    url: `/assistant/investigations/${investigationId}`,
    method: 'get'
  })
}

/** 获取调查证据 */
export function getInvestigationEvidence(investigationId: string, params?: EvidenceQueryParams) {
  return request<any, PaginatedResponse<AssistantEvidence>>({
    url: `/assistant/investigations/${investigationId}/evidence`,
    method: 'get',
    params
  })
}

/** 获取 MCP 数据源列表 */
export function getMCPSources(params?: MCPSourcesQueryParams) {
  return request<any, PaginatedResponse<AssistantMCPSource>>({
    url: '/assistant/mcp-sources',
    method: 'get',
    params
  })
}

/** 创建 MCP 数据源 */
export function createMCPSource(data: CreateMCPSourceRequest) {
  return request<any, AssistantMCPSource>({
    url: '/assistant/mcp-sources',
    method: 'post',
    data
  })
}

/** 更新 MCP 数据源 */
export function updateMCPSource(sourceId: string, data: UpdateMCPSourceRequest) {
  return request<any, AssistantMCPSource>({
    url: `/assistant/mcp-sources/${sourceId}`,
    method: 'put',
    data
  })
}

/** 删除 MCP 数据源 */
export function deleteMCPSource(sourceId: string) {
  return request<any, void>({
    url: `/assistant/mcp-sources/${sourceId}`,
    method: 'delete'
  })
}

/** 测试 MCP 数据源连接 */
export function testMCPSource(sourceId: string) {
  return request<any, { success: boolean; message: string }>({
    url: `/assistant/mcp-sources/${sourceId}/test`,
    method: 'post'
  })
}

/** 同步 MCP Schema */
export function syncMCPSchema(sourceId: string) {
  return request<any, { tool_count: number; synced_at: string }>({
    url: `/assistant/mcp-sources/${sourceId}/sync-schema`,
    method: 'post'
  })
}

// ============================================
// SSE 事件类型 — V6.0 设计规范
// ============================================

/** SSE 事件类型 */
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
  | 'run_waiting_approval'
  | 'context_ref_added'
  | 'result_card'
  | 'context_budget'
  | 'context_compressed'
  | 'context_compression_failed'
  | 'done'
  | 'error'

/** SSE 流式事件 */
export interface AssistantStreamEvent {
  type: AssistantStreamEventType
  session_id: string
  run_id?: string
  message_id?: string
  payload?: any
  error?: string
  timestamp?: string
}

/** 意图识别结果 */
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

/** 智能体选中的工具 */
export interface AssistantSelectedTool {
  name: string
  domain: string
  operation: string
  risk: AssistantRiskLevel
  reason: string
}

/** 工具选择结果 */
export interface AssistantToolSelection {
  run_id: string
  stage: 'initial' | 'expanded' | 'approval_resume' | 'retry'
  intent: AssistantIntentResult
  tools: AssistantSelectedTool[]
  tool_count: number
}

/** 工具搜索结果 */
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

/** 执行计划 */
export interface AssistantPlan {
  plan_id: string
  goal: string
  steps: AssistantPlanStep[]
  status: 'planning' | 'running' | 'completed' | 'failed' | 'cancelled'
}

/** 执行计划步骤 */
export interface AssistantPlanStep {
  step_id: string
  title: string
  objective: string
  suggested_tools: string[]
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped' | 'waiting_approval'
  result_summary?: string
}

// ============================================
// SSE 流式连接
// ============================================

/**
 * 创建助手 SSE 流式连接
 * 使用 EventSource 进行服务端推送，认证 token 作为查询参数传递
 */
export function createAssistantStream(sessionId: string, token?: string): EventSource {
  const authToken = token || getAuthToken()
  const url = `/api/v1/assistant/sessions/${sessionId}/stream?auth_token=${encodeURIComponent(authToken)}`
  return new EventSource(url)
}
