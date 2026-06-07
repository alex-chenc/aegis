import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAssistantStore } from './assistant'
import type { AssistantSession } from '@/api/assistant'

vi.mock('@/api/assistant', () => ({
  getSessions: vi.fn(),
  createSession: vi.fn(),
  getSession: vi.fn(),
  getMessages: vi.fn(),
  sendMessage: vi.fn(),
  cancelRun: vi.fn(),
  getContextRefs: vi.fn(),
  getToolCalls: vi.fn(),
  getApprovals: vi.fn(),
  approveApproval: vi.fn(),
  rejectApproval: vi.fn(),
  createAssistantStream: vi.fn(),
}))

const session: AssistantSession = {
  id: 'id-1',
  session_id: 'session-1',
  title: '安全分析',
  task_type: 'explanation',
  status: 'running',
  message_count: 1,
  tool_call_count: 0,
  approval_count: 0,
  created_at: '2026-06-07T00:00:00Z',
  updated_at: '2026-06-07T00:00:00Z',
}

describe('assistant store stream events', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('collects thinking events into the assistant message', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '正在定位目标主机' },
    })
    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '正在查询告警和漏洞' },
    })

    expect(store.messages).toHaveLength(1)
    expect(store.messages[0].message_id).toBe('msg_run-1')
    expect(store.messages[0].thinking).toContain('正在定位目标主机')
    expect(store.messages[0].thinking).toContain('正在查询告警和漏洞')
  })

  it('updates a running tool call to completed using the same SSE call ID', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    store.applyStreamEvent({
      type: 'tool_call',
      session_id: 'session-1',
      run_id: 'run-1',
      message_id: 'msg_run-1',
      payload: {
        call_id: 'runtime-call-1',
        tool_name: 'Detection.Alert.List',
        args: { page: 1, page_size: 20 },
      },
    })
    store.applyStreamEvent({
      type: 'tool_result',
      session_id: 'session-1',
      run_id: 'run-1',
      message_id: 'msg_run-1',
      payload: {
        call_id: 'runtime-call-1',
        result: { total: 2 },
      },
    })

    expect(store.toolCalls).toHaveLength(1)
    expect(store.toolCalls[0]).toMatchObject({
      call_id: 'runtime-call-1',
      tool_name: 'Detection.Alert.List',
      status: 'completed',
    })
    expect(store.toolCalls[0].result_summary).toContain('"total":2')
  })
})
