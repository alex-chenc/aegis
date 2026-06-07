// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AssistantConversation from './AssistantConversation.vue'
import type { AssistantMessage } from '@/api/assistant'

vi.mock('gsap', () => ({
  gsap: {
    context: (fn: () => void) => {
      fn()
      return {
        add: (callback: () => void) => callback(),
        revert: vi.fn(),
      }
    },
    matchMedia: () => ({
      add: (_query: string, callback: () => void) => callback(),
      revert: vi.fn(),
    }),
    from: vi.fn(),
    set: vi.fn(),
  },
}))

function mountConversation(messages: AssistantMessage[]) {
  return mount(AssistantConversation, {
    props: {
      messages,
      toolCalls: [],
      approvals: [],
      resultCards: [],
      streaming: false,
    },
    global: {
      stubs: {
        ElIcon: { template: '<span><slot /></span>' },
        ElTag: { template: '<span><slot /></span>' },
        User: true,
        Monitor: true,
        InfoFilled: true,
        CircleCheck: true,
        SetUp: true,
        AssistantApprovalCard: true,
        AssistantResultRenderer: true,
      },
    },
  })
}

describe('AssistantConversation', () => {
  it('为 assistant 消息内的每个展示框渲染独立头像', () => {
    const messages: AssistantMessage[] = [{
      id: 'msg-segments',
      session_id: 'session-1',
      message_id: 'msg-segments',
      role: 'assistant',
      content: '模型结果',
      thinking: ['思考一', '思考二'],
      tool_calls: [{
        id: 'tool-1',
        session_id: 'session-1',
        message_id: 'msg-segments',
        call_id: 'call-1',
        tool_name: 'Host.List',
        risk_level: 'readonly',
        status: 'completed',
        result: { total: 2 },
        created_at: '2026-06-07T00:00:00Z',
      }],
      created_at: '2026-06-07T00:00:00Z',
    }]

    const wrapper = mountConversation(messages)
    const assistantRows = wrapper.findAll('.message.assistant')

    expect(assistantRows).toHaveLength(4)
    for (const row of assistantRows) {
      expect(row.find('.message-avatar').exists()).toBe(true)
    }
  })

  it('把工具名称和格式化 JSON 执行结果放在同一个工具框内', () => {
    const messages: AssistantMessage[] = [{
      id: 'msg-json',
      session_id: 'session-1',
      message_id: 'msg-json',
      role: 'assistant',
      content: '',
      tool_calls: [{
        id: 'tool-json',
        session_id: 'session-1',
        message_id: 'msg-json',
        call_id: 'call-json',
        tool_name: 'Detection.Alert.List',
        risk_level: 'readonly',
        status: 'completed',
        result: '{"total":2,"items":[{"severity":"high"}]}',
        created_at: '2026-06-07T00:00:00Z',
      }],
      created_at: '2026-06-07T00:00:00Z',
    }]

    const wrapper = mountConversation(messages)
    const toolCard = wrapper.find('.tool-call-card')

    expect(toolCard.exists()).toBe(true)
    expect(toolCard.text()).toContain('Detection.Alert.List')
    expect(toolCard.text()).toContain('"total": 2')
    expect(toolCard.text()).toContain('"severity": "high"')
    expect(toolCard.find('.tool-call-result.is-json').exists()).toBe(true)
  })

  it('折叠过长的工具执行结果并支持展开', async () => {
    const tailMarker = 'TAIL-MARKER'
    const longResult = `${'A'.repeat(950)}${tailMarker}`
    const messages: AssistantMessage[] = [{
      id: 'msg-1',
      session_id: 'session-1',
      message_id: 'msg-1',
      role: 'assistant',
      content: '',
      tool_calls: [{
        id: 'tool-1',
        session_id: 'session-1',
        message_id: 'msg-1',
        call_id: 'call-1',
        tool_name: 'Host.List',
        risk_level: 'readonly',
        status: 'completed',
        result: longResult,
        created_at: '2026-06-07T00:00:00Z',
      }],
      created_at: '2026-06-07T00:00:00Z',
    }]

    const wrapper = mountConversation(messages)

    expect(wrapper.text()).toContain('展开完整结果')
    expect(wrapper.text()).not.toContain(tailMarker)

    await wrapper.find('button.tool-result-toggle').trigger('click')

    expect(wrapper.text()).toContain('收起结果')
    expect(wrapper.text()).toContain(tailMarker)
  })
})
