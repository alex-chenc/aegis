import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAssistantStore } from './assistant'
import type { AssistantMessage, AssistantSession, AssistantToolCall } from '@/api/assistant'

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
    vi.clearAllMocks()
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

  // ========== 循环分割测试用例 ==========

  it('创建单个思考-结果循环', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    // 思考
    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '正在分析主机状态' },
    })
    // 内容
    store.applyStreamEvent({
      type: 'message_delta',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { message_id: 'msg_run-1', delta: '主机状态正常' },
    })
    // 完成
    store.applyStreamEvent({
      type: 'done',
      session_id: 'session-1',
      run_id: 'run-1',
    })

    // 应该只有1个消息
    expect(store.messages).toHaveLength(1)
    // 消息包含思考和内容
    expect(store.messages[0].thinking).toContain('正在分析主机状态')
    expect(store.messages[0].content).toBe('主机状态正常')
  })

  it('识别思考后内容的循环边界', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    // 第一个循环：思考 + 内容
    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '第一次思考' },
    })
    store.applyStreamEvent({
      type: 'message_delta',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { message_id: 'msg_run-1', delta: '第一次结果' },
    })

    // 第二个循环：新的思考（在内容之后）
    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '第二次思考' },
    })
    store.applyStreamEvent({
      type: 'message_delta',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { message_id: 'msg_run-2', delta: '第二次结果' },
    })

    // 应该有2个消息
    expect(store.messages).toHaveLength(2)
    // 第一个消息
    expect(store.messages[0].thinking).toContain('第一次思考')
    expect(store.messages[0].content).toBe('第一次结果')
    // 第二个消息
    expect(store.messages[1].thinking).toContain('第二次思考')
    expect(store.messages[1].content).toBe('第二次结果')
  })

  it('识别工具调用后的循环边界', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    // 第一个循环：思考 + 工具调用
    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '需要查询告警' },
    })
    store.applyStreamEvent({
      type: 'tool_call',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: {
        call_id: 'call-1',
        tool_name: 'Detection.Alert.List',
        args: { page: 1 },
      },
    })
    store.applyStreamEvent({
      type: 'tool_result',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: {
        call_id: 'call-1',
        result: { total: 5 },
      },
    })

    // 第二个循环：新的思考（在工具结果之后）
    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '分析告警结果' },
    })
    store.applyStreamEvent({
      type: 'message_delta',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { message_id: 'msg_run-2', delta: '发现5条告警' },
    })

    // 应该有2个消息
    expect(store.messages).toHaveLength(2)
    // 第一个消息包含思考和工具调用
    expect(store.messages[0].thinking).toContain('需要查询告警')
    expect(store.messages[0].tool_calls).toBeDefined()
    expect(store.messages[0].tool_calls![0].tool_name).toBe('Detection.Alert.List')
    // 第二个消息包含思考和内容
    expect(store.messages[1].thinking).toContain('分析告警结果')
    expect(store.messages[1].content).toBe('发现5条告警')
  })

  it('工具结果后的模型结果会显示为新的结果消息', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '需要查询告警' },
    })
    store.applyStreamEvent({
      type: 'message_delta',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { message_id: 'msg_run-1', delta: '准备查询告警列表' },
    })
    store.applyStreamEvent({
      type: 'tool_call',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: {
        call_id: 'call-1',
        tool_name: 'Detection.Alert.List',
        args: { page: 1 },
      },
    })
    store.applyStreamEvent({
      type: 'tool_result',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: {
        call_id: 'call-1',
        result: { total: 5 },
      },
    })
    store.applyStreamEvent({
      type: 'message_delta',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { message_id: 'msg_run-1', delta: '发现5条告警' },
    })

    expect(store.messages).toHaveLength(2)
    expect(store.messages[0].content).toBe('准备查询告警列表')
    expect(store.messages[0].tool_calls?.[0]).toMatchObject({
      call_id: 'call-1',
      status: 'completed',
    })
    expect(store.messages[1].content).toBe('发现5条告警')
    expect(store.messages[1].thinking).toBeUndefined()
  })

  it('关联工具调用到对应的消息', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    // 思考
    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '需要执行脚本' },
    })
    // 工具调用
    store.applyStreamEvent({
      type: 'tool_call',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: {
        call_id: 'call-1',
        tool_name: 'Script.Execute',
        args: { script: 'check_vuln.sh' },
      },
    })
    // 工具结果
    store.applyStreamEvent({
      type: 'tool_result',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: {
        call_id: 'call-1',
        result: { exit_code: 0, stdout: 'No vulnerabilities found' },
      },
    })

    // 验证工具调用关联到消息
    expect(store.messages).toHaveLength(1)
    expect(store.messages[0].tool_calls).toBeDefined()
    expect(store.messages[0].tool_calls).toHaveLength(1)
    expect(store.messages[0].tool_calls![0]).toMatchObject({
      call_id: 'call-1',
      tool_name: 'Script.Execute',
      status: 'completed',
    })
  })

  it('处理多个工具调用在同一循环', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    // 思考
    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '需要执行多个检查' },
    })
    // 第一个工具调用
    store.applyStreamEvent({
      type: 'tool_call',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: {
        call_id: 'call-1',
        tool_name: 'Script.Execute',
        args: { script: 'check1.sh' },
      },
    })
    store.applyStreamEvent({
      type: 'tool_result',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: {
        call_id: 'call-1',
        result: { exit_code: 0 },
      },
    })
    // 第二个工具调用
    store.applyStreamEvent({
      type: 'tool_call',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: {
        call_id: 'call-2',
        tool_name: 'Script.Execute',
        args: { script: 'check2.sh' },
      },
    })
    store.applyStreamEvent({
      type: 'tool_result',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: {
        call_id: 'call-2',
        result: { exit_code: 0 },
      },
    })

    // 验证两个工具调用都在同一消息
    expect(store.messages).toHaveLength(1)
    expect(store.messages[0].tool_calls).toHaveLength(2)
    expect(store.messages[0].tool_calls![0].call_id).toBe('call-1')
    expect(store.messages[0].tool_calls![1].call_id).toBe('call-2')
  })

  it('处理无思考直接返回内容的情况', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    // 直接内容（无思考）
    store.applyStreamEvent({
      type: 'message_delta',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { message_id: 'msg_run-1', delta: '直接回答' },
    })

    // 应该有1个消息，只有内容
    expect(store.messages).toHaveLength(1)
    expect(store.messages[0].content).toBe('直接回答')
    expect(store.messages[0].thinking).toBeUndefined()
  })

  it('处理旧格式thinking字符串', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    // 模拟旧格式数据（字符串而非数组）
    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '旧格式思考步骤1' },
    })
    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '旧格式思考步骤2' },
    })

    // 应该正确处理为数组
    expect(store.messages).toHaveLength(1)
    expect(Array.isArray(store.messages[0].thinking)).toBe(true)
    expect(store.messages[0].thinking).toContain('旧格式思考步骤1')
    expect(store.messages[0].thinking).toContain('旧格式思考步骤2')
  })

  it('uses context budget total tokens to preserve cumulative runtime usage', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    store.applyStreamEvent({
      type: 'context_budget',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: {
        max_context_tokens: 256000,
        reserved_output_tokens: 8192,
        estimated_prompt_tokens: 24000,
        context_ratio: 0.125,
        prompt_tokens_observed: 24000,
        completion_tokens: 4000,
        total_tokens: 52000,
        compression_count: 0,
      },
    })

    expect(store.contextBudget?.estimated_prompt_tokens).toBe(24000)
    expect(store.totalPromptTokens).toBe(48000)
    expect(store.totalCompletionTokens).toBe(4000)
    expect(store.currentSession?.metadata?.total_tokens).toBe(52000)
  })

  it('刷新历史时按思考、工具调用和最终结果的原始顺序重建展示片段', async () => {
    const api = await import('@/api/assistant')
    const store = useAssistantStore()
    store.currentSession = { ...session, status: 'completed' }

    const persistedMessage: AssistantMessage = {
      id: 'db-msg-1',
      session_id: 'session-1',
      message_id: 'msg-run-1',
      role: 'assistant',
      content: '最终分析报告',
      thinking: [
        '思考 A: 确认目标主机',
        '正在调用工具: Host.List',
        '思考 B: 读取告警信息',
        '正在调用工具: Detection.Alert.List',
        '思考 C: 综合分析与结论',
      ],
      created_at: '2026-06-07T00:00:01Z',
    }
    const toolCalls: AssistantToolCall[] = [
      {
        id: 'tool-1',
        session_id: 'session-1',
        message_id: 'msg-run-1',
        call_id: 'call-1',
        tool_name: 'Host.List',
        risk_level: 'readonly',
        status: 'completed',
        result: { total: 2 },
        created_at: '2026-06-07T00:00:02Z',
      },
      {
        id: 'tool-2',
        session_id: 'session-1',
        message_id: 'msg-run-1',
        call_id: 'call-2',
        tool_name: 'Detection.Alert.List',
        risk_level: 'readonly',
        status: 'completed',
        result: { total: 3 },
        created_at: '2026-06-07T00:00:03Z',
      },
    ]

    vi.mocked(api.getMessages).mockResolvedValue([persistedMessage])
    vi.mocked(api.getToolCalls).mockResolvedValue({ items: toolCalls, total: 2 } as any)

    await store.fetchMessages('session-1')
    await store.fetchToolCalls('session-1')

    expect(store.messages).toHaveLength(6)
    expect(store.messages[0].thinking).toEqual(['思考 A: 确认目标主机'])
    expect(store.messages[1].tool_calls?.[0]).toMatchObject({
      call_id: 'call-1',
      tool_name: 'Host.List',
    })
    expect(store.messages[2].thinking).toEqual(['思考 B: 读取告警信息'])
    expect(store.messages[3].tool_calls?.[0]).toMatchObject({
      call_id: 'call-2',
      tool_name: 'Detection.Alert.List',
    })
    expect(store.messages[4].thinking).toEqual(['思考 C: 综合分析与结论'])
    expect(store.messages[5].content).toBe('最终分析报告')

    const visibleThinking = store.messages.flatMap(message =>
      Array.isArray(message.thinking) ? message.thinking : []
    )
    expect(visibleThinking).not.toContain('正在调用工具: Host.List')
    expect(visibleThinking).not.toContain('正在调用工具: Detection.Alert.List')
  })

  it('刷新已完成会话时将历史计划中的待执行步骤归一为完成', async () => {
    const api = await import('@/api/assistant')
    const store = useAssistantStore()
    store.currentSession = { ...session, status: 'completed' }

    const persistedMessage: AssistantMessage = {
      id: 'db-msg-plan',
      session_id: 'session-1',
      message_id: 'msg-plan-1',
      role: 'assistant',
      content: '任务已完成',
      plan: {
        goal: '排查主机风险',
        status: 'running',
        steps: [
          { step_id: 'step-1', title: '收集告警', status: 'pending' },
          { step_id: 'step-2', title: '汇总结论', status: 'running' },
        ],
      },
      created_at: '2026-06-07T00:00:01Z',
    }

    vi.mocked(api.getMessages).mockResolvedValue([persistedMessage])

    await store.fetchMessages('session-1')

    expect(store.messages[0].plan?.status).toBe('completed')
    expect(store.messages[0].plan?.steps.map(step => step.status)).toEqual(['completed', 'completed'])
  })

  it('刷新无工具历史时也按单条思考拆分展示', async () => {
    const api = await import('@/api/assistant')
    const store = useAssistantStore()
    store.currentSession = { ...session, status: 'completed' }

    vi.mocked(api.getMessages).mockResolvedValue([{
      id: 'db-msg-thinking',
      session_id: 'session-1',
      message_id: 'msg-thinking-1',
      role: 'assistant',
      content: '最终答复',
      thinking: ['思考一', '思考二'],
      created_at: '2026-06-07T00:00:01Z',
    }])
    vi.mocked(api.getToolCalls).mockResolvedValue({ items: [], total: 0 } as any)

    await store.fetchMessages('session-1')
    await store.fetchToolCalls('session-1')

    expect(store.messages).toHaveLength(3)
    expect(store.messages[0].thinking).toEqual(['思考一'])
    expect(store.messages[1].thinking).toEqual(['思考二'])
    expect(store.messages[2].content).toBe('最终答复')
  })

  it('跳过重复的思考步骤', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    // 发送重复的思考步骤
    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '相同内容' },
    })
    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '相同内容' },
    })

    // 应该只保留一个
    expect(store.messages).toHaveLength(1)
    expect(store.messages[0].thinking).toHaveLength(1)
  })

  it('处理多轮思考-工具-结果循环', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    // 第一轮：思考 + 工具
    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '第一轮思考' },
    })
    store.applyStreamEvent({
      type: 'tool_call',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: {
        call_id: 'call-1',
        tool_name: 'Tool.A',
        args: {},
      },
    })
    store.applyStreamEvent({
      type: 'tool_result',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: {
        call_id: 'call-1',
        result: { data: 'result-a' },
      },
    })

    // 第二轮：思考 + 工具
    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '第二轮思考' },
    })
    store.applyStreamEvent({
      type: 'tool_call',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: {
        call_id: 'call-2',
        tool_name: 'Tool.B',
        args: {},
      },
    })
    store.applyStreamEvent({
      type: 'tool_result',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: {
        call_id: 'call-2',
        result: { data: 'result-b' },
      },
    })

    // 第三轮：思考 + 最终内容
    store.applyStreamEvent({
      type: 'thinking',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { content: '最终分析' },
    })
    store.applyStreamEvent({
      type: 'message_delta',
      session_id: 'session-1',
      run_id: 'run-1',
      payload: { message_id: 'msg_run-3', delta: '最终结论' },
    })

    // 应该有3个消息
    expect(store.messages).toHaveLength(3)
    // 验证每个消息的内容
    expect(store.messages[0].thinking).toContain('第一轮思考')
    expect(store.messages[0].tool_calls![0].tool_name).toBe('Tool.A')
    expect(store.messages[1].thinking).toContain('第二轮思考')
    expect(store.messages[1].tool_calls![0].tool_name).toBe('Tool.B')
    expect(store.messages[2].thinking).toContain('最终分析')
    expect(store.messages[2].content).toBe('最终结论')
  })
})
