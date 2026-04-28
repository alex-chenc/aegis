import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAISessionStream } from './aiAnalysis'

const eventSourceMock = vi.fn()

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
