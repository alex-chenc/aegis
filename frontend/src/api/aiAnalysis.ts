import request from './index'

// AI Analysis Session types
export interface CreateSessionRequest {
  alert_ids: string[]
  time_range?: {
    start: string
    end: string
  }
  host_filter?: string[]
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
export type SSEEventType = 'thinking' | 'tool_call' | 'tool_result' | 'tool_error' | 'content' | 'done' | 'error'

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

// AI Analysis API functions

export function createAISession(data: CreateSessionRequest): Promise<CreateSessionResponse> {
  return request.post('/detection/alerts/ai-analysis/session', data)
}

export function sendMessage(sessionId: string, data: SendMessageRequest): Promise<SendMessageResponse> {
  return request.post(`/detection/alerts/ai-analysis/${sessionId}/message`, data)
}

export function getSessionHistory(sessionId: string): Promise<{
  session_id: string
  messages: Array<{
    role: string
    content: string
    created_at?: string
  }>
}> {
  return request.get(`/detection/alerts/ai-analysis/${sessionId}/history`)
}

export function findSimilarCases(data: SimilarCaseRequest): Promise<SimilarCaseResponse> {
  return request.post('/detection/alerts/ai-analysis/similar', data)
}

export function getRAGContext(data: RAGContextRequest): Promise<RAGContextResponse> {
  return request.post('/detection/alerts/ai-analysis/rag-context', data)
}

// SSE Streaming function
export function createAISessionStream(
  sessionId: string,
  message: string,
  onEvent: (event: SSEEvent) => void
): EventSource {
  const encodedMessage = encodeURIComponent(message)
  const eventSource = new EventSource(
    `/api/v1/detection/alerts/ai-analysis/${sessionId}/stream?message=${encodedMessage}`
  )

  eventSource.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data) as SSEEvent
      onEvent(data)

      if (data.type === 'done' || data.type === 'error') {
        eventSource.close()
      }
    } catch (e) {
      console.error('Failed to parse SSE event:', e)
    }
  }

  eventSource.onerror = (error) => {
    console.error('SSE error:', error)
    eventSource.close()
  }

  return eventSource
}
