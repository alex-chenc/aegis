import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAISessionStream, getExecutionResult, resolveExecutionResultPayload } from './aiAnalysis'

const eventSourceMock = vi.fn()
const requestGetMock = vi.hoisted(() => vi.fn())

vi.mock('./index', () => ({
  default: {
    get: requestGetMock
  }
}))

vi.mock('@/utils/auth', () => ({
  getAuthToken: vi.fn(() => 'token-123')
}))

describe('aiAnalysis SSE stream', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal(
      'EventSource',
      eventSourceMock.mockImplementation((url: string) => ({
        url,
        close: vi.fn(),
        onmessage: null,
        onerror: null
      }))
    )
  })

  it('includes auth token in stream url', () => {
    createAISessionStream('session-1', 'hello world', () => undefined)

    expect(eventSourceMock).toHaveBeenCalledWith(
      '/api/v1/detection/alerts/ai-analysis/session-1/stream?message=hello%20world&auth_token=token-123'
    )
  })
})

describe('aiAnalysis execution result API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns the unwrapped ExecutionResult body used by the project request interceptor', async () => {
    requestGetMock.mockResolvedValueOnce({
      execution_id: 'exec-1',
      task_id: 'task-1',
      session_id: 'session-1',
      status: 'completed',
      exit_reason: 'normal_completed',
      started_at: '',
      ended_at: '',
      total_duration_ms: 1,
      steps: [],
      errors: [],
      conclusion: {
        verdict: 'benign',
        summary: 'Benign / False Positive',
        reasoning: ''
      }
    })

    const result = await getExecutionResult('session-1')

    expect(requestGetMock).toHaveBeenCalledWith('/detection/alerts/ai-analysis/session-1/execution-result')
    expect(result.execution_id).toBe('exec-1')
    expect(result.status).toBe('已完成')
    expect('success' in result).toBe(false)
  })

  it('can resolve a legacy wrapped execution result payload defensively', () => {
    const result = resolveExecutionResultPayload({
      success: true,
      data: {
        execution_id: 'exec-2',
        task_id: 'task-2',
        session_id: 'session-2',
        status: 'completed',
        exit_reason: 'normal_completed',
        started_at: '',
        ended_at: '',
        total_duration_ms: 1,
        steps: [],
        errors: [],
        conclusion: {
          verdict: 'benign',
          summary: 'Benign / False Positive',
          reasoning: ''
        }
      }
    })

    expect(result?.execution_id).toBe('exec-2')
    expect(result?.status).toBe('已完成')
  })
})
