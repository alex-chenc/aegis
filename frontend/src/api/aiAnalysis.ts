import request from './index'
import { getAuthToken } from '@/utils/auth'
import { normalizeExecutionResult } from '@/utils/taskExecutionResult'

// AI Analysis Session types
export interface CreateSessionRequest {
  alert_ids: string[]
  time_range?: {
    start: string
    end: string
  }
  host_filter?: string[]
  max_iterations?: number
}

export interface CreateSessionResponse {
  session_id: string
  status: string
  selected_alerts: number
}

export interface SendMessageRequest {
  content: string
}

export interface SendMessageResponse {
  message_id: string
  role: string
  content: string
  tool_calls?: ToolCall[]
  steps?: AgentStep[]
}

export interface ToolCall {
  call_id: string
  tool: string
  arguments: Record<string, any>
}

export interface AgentStep {
  thought: string
  action: string
  action_input: Record<string, any>
  observation: string
}

export interface SimilarCaseRequest {
  query: string
  alert_type?: string
  threshold?: number
  limit?: number
}

export interface SimilarCase {
  id: string
  session_id: string
  alert_ids: string[]
  host_filter: string[]
  initial_query: string
  final_conclusion: Record<string, any>
  summary: string
  similarity: number
}

export interface SimilarCaseResponse {
  similar_cases: SimilarCase[]
}

export interface RAGContextRequest {
  query: string
  alert_type?: string
}

export interface RAGContextResponse {
  context: string
  case_count: number
}

// SSE Event types for streaming
export type SSEEventType =
  | 'thinking' | 'tool_call' | 'tool_result' | 'tool_error'
  | 'content' | 'done' | 'error'
  // agent-runtime new event types
  | 'plan' | 'step_started' | 'step_completed' | 'step_failed' | 'audit'
  | 'reflection' | 'correction' | 'step_retrying' | 'step_skipped'
  // context budget events
  | 'context_budget' | 'context_compressed' | 'context_compression_failed'
  // AI auto block event
  | 'ai_auto_block'

export interface SSEEvent {
  type: SSEEventType
  content?: string
  tool?: string
  call_id?: string
  args?: Record<string, any>
  result?: any
  time_ms?: number
  error?: string
}

// agent-runtime plan step
export interface PlanStep {
  id: string
  step_id?: string
  title?: string
  description: string
  objective?: string
  tool_names?: string[]
  suggested_tools?: string[]
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped' | 'retrying' | 'replaced' | 'invalidated' | 'waiting_approval'
  result_summary?: string
}

// agent-runtime SSE event payloads
export interface PlanEvent {
  id: string
  plan_id?: string
  goal: string
  steps: PlanStep[]
  total_steps: number
  version?: number
  assumptions?: string[]
}

export interface AuditEvent {
  decision: string
  risk_level: string
  findings?: string[]
  recommendations?: string[]
  step_id?: string
}

export interface ReflectionEvent {
  root_cause: string
  impact: string
  recommendation: string
  reusable_lesson?: string
  recoverable?: boolean
}

export interface CorrectionEvent {
  reason: string
  actions?: string[]
  step_id?: string
}

// Context budget event payloads
export interface ContextBudgetEvent {
  max_context_tokens: number
  reserved_output_tokens: number
  estimated_prompt_tokens: number
  context_ratio: number
  prompt_tokens_observed: number
  completion_tokens: number
  total_tokens: number
  compression_count: number
}

export interface ContextCompressedEvent {
  strategy: string
  trigger_ratio: number
  before_tokens: number
  after_tokens: number
}

// AI auto block event payloads
export interface AIAutoBlockResultItem {
  alert_id: string
  mitre_id?: string
  status: 'success' | 'failed' | 'skipped'
  block_id?: string
  action?: string
  target?: string
  message: string
  issued_by?: string
  existing_block_id?: string
  existing_issued_by?: string
}

export interface AIAutoBlockSummary {
  total: number
  executed: number
  success: number
  failed: number
  skipped: number
}

export interface AIAutoBlockPayload {
  triggered: boolean
  summary?: AIAutoBlockSummary
  results?: AIAutoBlockResultItem[]
  reason?: string
}

// AI Analysis API functions

export function createAISession(data: CreateSessionRequest): Promise<CreateSessionResponse> {
  return request.post('/detection/alerts/ai-analysis/session', data)
}

export function sendMessage(sessionId: string, data: SendMessageRequest): Promise<SendMessageResponse> {
  return request.post(`/detection/alerts/ai-analysis/${sessionId}/message`, data)
}

export interface AnalysisControlResponse {
  session_id: string
  status: string
  active_run: boolean
  message: string
}

export function pauseSession(sessionId: string): Promise<AnalysisControlResponse> {
  return request.post(`/detection/alerts/ai-analysis/${sessionId}/pause`)
}

export function cancelSession(sessionId: string): Promise<AnalysisControlResponse> {
  return request.post(`/detection/alerts/ai-analysis/${sessionId}/cancel`)
}

export function getSessionHistory(sessionId: string): Promise<{
  success: boolean
  data: {
    session_id: string
    messages: Array<{
      role: string
      content: string
      thinking?: string
      tool_calls?: { items?: ToolCall[] } | ToolCall[]
      tool_results?: { items?: any[] } | any[]
      steps?: { items?: AgentStep[] } | AgentStep[]
      created_at?: string
    }>
    execution_plan?: PlanEvent | null
    audits?: AuditEvent[]
    reflections?: ReflectionEvent[]
    corrections?: CorrectionEvent[]
    status?: string
    conclusion?: Record<string, any> | null
    alerts?: Array<{
      id: string
      alert_id: string
      hostname?: string
      rule_title?: string
      mitre_id: string
      severity: string
      status: string
      description?: string
      last_seen_at: string
    }>
  }
}> {
  return request.get(`/detection/alerts/ai-analysis/${sessionId}/history`)
}

export function deleteSession(sessionId: string): Promise<{ success: boolean; message: string }> {
  return request.delete(`/detection/alerts/ai-analysis/${sessionId}`)
}

export function findSimilarCases(data: SimilarCaseRequest): Promise<SimilarCaseResponse> {
  return request.post('/detection/alerts/ai-analysis/similar', data)
}

export function getRAGContext(data: RAGContextRequest): Promise<RAGContextResponse> {
  return request.post('/detection/alerts/ai-analysis/rag-context', data)
}

export interface SessionListItem {
  id: string
  session_id: string
  alert_ids: string[]
  host_ids: string[]
  host_filter: string[]
  status: string
  max_iterations: number
  message_count: number
  tool_call_count: number
  created_at: string
  updated_at: string
  concluded_at?: string
  conclusion?: Record<string, any>
}

export interface SessionListResponse {
  sessions: SessionListItem[]
  total: number
  page: number
  page_size: number
}

export function getSessionList(page: number = 1, pageSize: number = 20, status?: string): Promise<SessionListResponse> {
  const params = new URLSearchParams({
    page: String(page),
    page_size: String(pageSize)
  })
  if (status) {
    params.append('status', status)
  }
  return request.get(`/detection/alerts/ai-analysis/sessions?${params.toString()}`)
}

export interface AlertConclusion {
  alert_id: string
  action: 'mark_false_positive' | 'confirm_threat' | 'generate_rule'
  summary: string
}

export interface ApplyConclusionRequest {
  conclusions: AlertConclusion[]
}

export function applyConclusions(sessionId: string, conclusions: AlertConclusion[]): Promise<{ success: boolean; message: string }> {
  return request.post(`/detection/alerts/ai-analysis/${sessionId}/conclusion`, { conclusions })
}

// Execution Result types
export interface ExecutionResult {
  execution_id: string
  task_id: string
  session_id: string
  status: string
  exit_reason: string
  started_at: string
  ended_at: string
  total_duration_ms: number
  steps: StepResult[]
  errors: string[]
  conclusion: Conclusion
  final_answer?: string
  context_budget?: ContextBudgetEvent | null
  compression_records?: ContextCompressedEvent[]
  total_prompt_tokens?: number
  total_completion_tokens?: number
}

export interface StepResult {
  step_id: string
  status: string
  result: string
  started_at: string
  ended_at: string
  duration_ms: number
}

export interface Conclusion {
  verdict: string
  summary: string
  reasoning: string
}

type ExecutionResultPayload = ExecutionResult | { success?: boolean; data?: ExecutionResult } | null | undefined

export function resolveExecutionResultPayload(payload: ExecutionResultPayload): ExecutionResult | null {
  if (!payload || typeof payload !== 'object') return null

  const body = 'data' in payload && payload.data ? payload.data : payload
  if (!body || typeof body !== 'object' || !('status' in body)) return null

  return normalizeExecutionResult(body as ExecutionResult)
}

export async function getExecutionResult(sessionId: string): Promise<ExecutionResult> {
  const payload = await request.get(`/detection/alerts/ai-analysis/${sessionId}/execution-result`)
  const result = resolveExecutionResultPayload(payload as ExecutionResultPayload)
  if (!result) {
    throw new Error('执行记录不存在')
  }
  return result
}

// SSE Streaming function
export function createAISessionStream(
  sessionId: string,
  message: string,
  onEvent: (event: SSEEvent) => void
): EventSource {
  const encodedMessage = encodeURIComponent(message)
  const token = encodeURIComponent(getAuthToken())
  const eventSource = new EventSource(
    `/api/v1/detection/alerts/ai-analysis/${sessionId}/stream?message=${encodedMessage}&auth_token=${token}`
  )

  let streamFinished = false

  eventSource.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data) as SSEEvent
      onEvent(data)

      if (data.type === 'done' || data.type === 'error') {
        streamFinished = true
        eventSource.close()
      }
    } catch (e) {
      console.error('Failed to parse SSE event:', e)
    }
  }

  eventSource.onerror = () => {
    if (streamFinished) return
    streamFinished = true
    onEvent({
      type: 'error',
      content: 'AI 分析连接中断，请稍后重试或查看服务日志'
    })
    eventSource.close()
  }

  return eventSource
}
