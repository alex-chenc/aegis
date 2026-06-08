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
    expect(wrapper.findAll('.thinking-block')).toHaveLength(2)
    expect(wrapper.text()).toContain('思考一')
    expect(wrapper.text()).toContain('思考二')
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

  it('实时渲染时把工具调用提示和对应结果卡按顺序配对', () => {
    const messages: AssistantMessage[] = [{
      id: 'msg-live-order',
      session_id: 'session-1',
      message_id: 'msg-live-order',
      role: 'assistant',
      content: '最终报告',
      thinking: [
        '正在调用工具: Detection.Statistics.Get',
        '正在调用工具: Detection.Alert.List',
        '步骤完成: 获取平台威胁统计概览',
      ],
      tool_calls: [{
        id: 'tool-stat',
        session_id: 'session-1',
        message_id: 'msg-live-order',
        call_id: 'call-stat',
        tool_name: 'Detection.Statistics.Get',
        risk_level: 'readonly',
        status: 'completed',
        result: { active_rules: 4 },
        created_at: '2026-06-07T00:00:00Z',
      }, {
        id: 'tool-alert',
        session_id: 'session-1',
        message_id: 'msg-live-order',
        call_id: 'call-alert',
        tool_name: 'Detection.Alert.List',
        risk_level: 'readonly',
        status: 'completed',
        result: { total: 0 },
        created_at: '2026-06-07T00:00:01Z',
      }],
      created_at: '2026-06-07T00:00:00Z',
    }]

    const wrapper = mountConversation(messages)
    const renderedSegments = wrapper.findAll('.message.assistant').map(row => {
      const toolCard = row.find('.tool-call-card')
      if (toolCard.exists()) return `tool:${row.find('.tool-name').text()}`

      const stepResult = row.find('.step-result-card')
      if (stepResult.exists()) return `step-result:${stepResult.text()}`

      const thinkingStep = row.find('.thinking-step')
      if (thinkingStep.exists()) return `thinking:${thinkingStep.text()}`

      return `content:${row.text()}`
    })

    expect(renderedSegments).toEqual([
      'thinking:正在调用工具: Detection.Statistics.Get',
      'tool:Detection.Statistics.Get',
      'thinking:正在调用工具: Detection.Alert.List',
      'tool:Detection.Alert.List',
      'thinking:步骤完成: 获取平台威胁统计概览',
      'step-result:获取平台威胁统计概览已完成步骤：获取平台威胁统计概览',
      'content:最终报告',
    ])
  })

  it('刷新历史拆分为相邻消息时仍显示工具调用提示', () => {
    const messages: AssistantMessage[] = [{
      id: 'msg-history-thinking',
      session_id: 'session-1',
      message_id: 'msg-history-thinking',
      role: 'assistant',
      content: '',
      thinking: ['正在调用工具: Host.List'],
      created_at: '2026-06-07T00:00:00Z',
    }, {
      id: 'msg-history-tool',
      session_id: 'session-1',
      message_id: 'msg-history-tool',
      role: 'assistant',
      content: '',
      tool_calls: [{
        id: 'tool-host',
        session_id: 'session-1',
        message_id: 'msg-history-tool',
        call_id: 'call-host',
        tool_name: 'Host.List',
        risk_level: 'readonly',
        status: 'completed',
        result: { total: 2 },
        created_at: '2026-06-07T00:00:01Z',
      }],
      created_at: '2026-06-07T00:00:01Z',
    }]

    const wrapper = mountConversation(messages)
    const renderedSegments = wrapper.findAll('.message.assistant').map(row => {
      const toolCard = row.find('.tool-call-card')
      if (toolCard.exists()) return `tool:${row.find('.tool-name').text()}`

      const thinkingStep = row.find('.thinking-step')
      if (thinkingStep.exists()) return `thinking:${thinkingStep.text()}`

      return `content:${row.text()}`
    })

    expect(renderedSegments).toEqual([
      'thinking:正在调用工具: Host.List',
      'tool:Host.List',
    ])
  })

  it('运行中的工具未完成前不提前显示后续工具调用或思考', () => {
    const messages: AssistantMessage[] = [{
      id: 'msg-live-pending',
      session_id: 'session-1',
      message_id: 'msg-live-pending',
      role: 'assistant',
      content: '',
      thinking: [
        '正在调用工具: Detection.Statistics.Get',
        '正在调用工具: Detection.Alert.List',
        '步骤完成: 获取平台威胁统计概览',
      ],
      tool_calls: [{
        id: 'tool-stat',
        session_id: 'session-1',
        message_id: 'msg-live-pending',
        call_id: 'call-stat',
        tool_name: 'Detection.Statistics.Get',
        risk_level: 'readonly',
        status: 'running',
        created_at: '2026-06-07T00:00:00Z',
      }, {
        id: 'tool-alert',
        session_id: 'session-1',
        message_id: 'msg-live-pending',
        call_id: 'call-alert',
        tool_name: 'Detection.Alert.List',
        risk_level: 'readonly',
        status: 'running',
        created_at: '2026-06-07T00:00:01Z',
      }],
      created_at: '2026-06-07T00:00:00Z',
    }]

    const wrapper = mountConversation(messages)
    const renderedSegments = wrapper.findAll('.message.assistant').map(row => {
      const toolCard = row.find('.tool-call-card')
      if (toolCard.exists()) return `tool:${row.find('.tool-name').text()}`

      const thinkingStep = row.find('.thinking-step')
      if (thinkingStep.exists()) return `thinking:${thinkingStep.text()}`

      return `content:${row.text()}`
    })

    expect(renderedSegments).toEqual([
      'thinking:正在调用工具: Detection.Statistics.Get',
      'tool:Detection.Statistics.Get',
    ])
    expect(wrapper.text()).not.toContain('Detection.Alert.List')
    expect(wrapper.text()).not.toContain('步骤完成')
  })

  it('把步骤完成提示和步骤结果卡紧邻渲染', () => {
    const messages: AssistantMessage[] = [{
      id: 'msg-step-result',
      session_id: 'session-1',
      message_id: 'msg-step-result',
      role: 'assistant',
      content: '最终报告',
      thinking: [
        '开始执行步骤: 获取整体安全态势统计',
        '步骤完成: 获取整体安全态势统计',
      ],
      plan: {
        goal: '分析平台安全态势',
        status: 'completed',
        steps: [{
          step_id: 'step-1',
          title: '获取整体安全态势统计',
          status: 'completed',
          result_summary: '统计完成：当前无今日告警',
        }],
      },
      created_at: '2026-06-07T00:00:00Z',
    }]

    const wrapper = mountConversation(messages)
    const renderedSegments = wrapper.findAll('.message.assistant').map(row => {
      const stepResult = row.find('.step-result-card')
      if (stepResult.exists()) return `step-result:${stepResult.text()}`

      const thinkingStep = row.find('.thinking-step')
      if (thinkingStep.exists()) return `thinking:${thinkingStep.text()}`

      return `content:${row.text()}`
    })

    expect(renderedSegments).toEqual([
      'thinking:开始执行步骤: 获取整体安全态势统计',
      'thinking:步骤完成: 获取整体安全态势统计',
      'step-result:获取整体安全态势统计统计完成：当前无今日告警',
      'content:最终报告',
    ])
  })

  it('历史最终消息保留完整计划时不重复渲染步骤结果卡', () => {
    const messages: AssistantMessage[] = [{
      id: 'msg-step-result',
      session_id: 'session-1',
      message_id: 'msg-run_history_1',
      role: 'assistant',
      content: '',
      thinking: ['步骤完成: 定位全部主机'],
      plan: {
        goal: '分析全部主机风险',
        status: 'completed',
        steps: [{
          step_id: 'step-1',
          title: '定位全部主机',
          status: 'completed',
          result_summary: '发现 2 台目标主机',
        }],
      },
      created_at: '2026-06-07T00:00:00Z',
    }, {
      id: 'msg-final',
      session_id: 'session-1',
      message_id: 'msg-run_history_2',
      role: 'assistant',
      content: '最终报告',
      plan: {
        goal: '分析全部主机风险',
        status: 'completed',
        steps: [{
          step_id: 'step-1',
          title: '定位全部主机',
          status: 'completed',
          result_summary: '发现 2 台目标主机',
        }, {
          step_id: 'step-2',
          title: '逐台 Agent 取证',
          status: 'completed',
          result_summary: '2 台主机均已尝试取证',
        }],
      },
      created_at: '2026-06-07T00:00:01Z',
    }]

    const wrapper = mountConversation(messages)

    expect(wrapper.findAll('.step-result-card')).toHaveLength(1)
    expect(wrapper.text()).toContain('发现 2 台目标主机')
    expect(wrapper.text()).not.toContain('2 台主机均已尝试取证')
    expect(wrapper.text()).toContain('最终报告')
  })

  it('逐条渲染普通 thinking，并隐藏无结果工具提示和内部反思', () => {
    const messages: AssistantMessage[] = [{
      id: 'msg-process',
      session_id: 'session-1',
      message_id: 'msg-process',
      role: 'assistant',
      content: '模型结果',
      thinking: [
        '开始执行任务...',
        '开始执行步骤: 获取平台概况',
        '正在调用工具: Host.List',
        '步骤完成: 获取平台概况',
        '需要结合主机与漏洞数据判断风险',
        '审计完成: null',
        '正在反思执行过程...',
        '反思结果: 工具参数缺失',
      ],
      created_at: '2026-06-07T00:00:00Z',
    }]

    const wrapper = mountConversation(messages)

    expect(wrapper.findAll('.thinking-block')).toHaveLength(4)
    expect(wrapper.text()).toContain('需要结合主机与漏洞数据判断风险')
    expect(wrapper.text()).toContain('开始执行步骤')
    expect(wrapper.text()).toContain('步骤完成')
    expect(wrapper.text()).not.toContain('正在调用工具')
    expect(wrapper.text()).not.toContain('审计完成')
    expect(wrapper.text()).not.toContain('正在反思执行过程')
    expect(wrapper.text()).not.toContain('反思结果')
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
