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
    store.rebuildAssistantHistoryCycles()

    expect(store.messages).toHaveLength(8)
    expect(store.messages[0].thinking).toEqual(['思考 A: 确认目标主机'])
    expect(store.messages[1].thinking).toEqual(['正在调用工具: Host.List'])
    expect(store.messages[2].tool_calls?.[0]).toMatchObject({
      call_id: 'call-1',
      tool_name: 'Host.List',
    })
    expect(store.messages[3].thinking).toEqual(['思考 B: 读取告警信息'])
    expect(store.messages[4].thinking).toEqual(['正在调用工具: Detection.Alert.List'])
    expect(store.messages[5].tool_calls?.[0]).toMatchObject({
      call_id: 'call-2',
      tool_name: 'Detection.Alert.List',
    })
    expect(store.messages[6].thinking).toEqual(['思考 C: 综合分析与结论'])
    expect(store.messages[7].content).toBe('最终分析报告')

    const visibleThinking = store.messages.flatMap(message =>
      Array.isArray(message.thinking) ? message.thinking : []
    )
    expect(visibleThinking).toContain('正在调用工具: Host.List')
    expect(visibleThinking).toContain('正在调用工具: Detection.Alert.List')
  })

  it('刷新历史时优先使用后端运行事件流按 call_id 重建重复工具顺序', async () => {
    const api = await import('@/api/assistant')
    const store = useAssistantStore()
    store.currentSession = {
      ...session,
      status: 'completed',
      metadata: {
        assistant_runtime_events: {
          'msg-run-dup': [
            { type: 'thinking', message_id: 'msg-run-dup', payload: { content: '第一轮主机查询' } },
            { type: 'tool_call', message_id: 'msg-run-dup', payload: { call_id: 'call-b', tool_name: 'Host.List' } },
            { type: 'thinking', message_id: 'msg-run-dup', payload: { content: '第二轮主机查询' } },
            { type: 'tool_call', message_id: 'msg-run-dup', payload: { call_id: 'call-a', tool_name: 'Host.List' } },
          ],
        },
      },
    }

    const persistedMessage: AssistantMessage = {
      id: 'db-msg-dup',
      session_id: 'session-1',
      message_id: 'msg-run-dup',
      role: 'assistant',
      content: '最终分析报告',
      thinking: ['旧的降级思考顺序'],
      created_at: '2026-06-07T00:00:01Z',
    }
    const toolCalls: AssistantToolCall[] = [
      {
        id: 'tool-a',
        session_id: 'session-1',
        message_id: 'msg-run-dup',
        call_id: 'call-a',
        tool_name: 'Host.List',
        risk_level: 'readonly',
        status: 'completed',
        result: { page: 1 },
        created_at: '2026-06-07T00:00:02Z',
      },
      {
        id: 'tool-b',
        session_id: 'session-1',
        message_id: 'msg-run-dup',
        call_id: 'call-b',
        tool_name: 'Host.List',
        risk_level: 'readonly',
        status: 'completed',
        result: { page: 2 },
        created_at: '2026-06-07T00:00:03Z',
      },
    ]

    vi.mocked(api.getMessages).mockResolvedValue([persistedMessage])
    vi.mocked(api.getToolCalls).mockResolvedValue({ items: toolCalls, total: 2 } as any)

    await store.fetchMessages('session-1')
    await store.fetchToolCalls('session-1')
    store.rebuildAssistantHistoryCycles()

    expect(store.messages).toHaveLength(7)
    expect(store.messages[0].thinking).toEqual(['第一轮主机查询'])
    expect(store.messages[1].thinking).toEqual(['正在调用工具: Host.List'])
    expect(store.messages[2].tool_calls?.[0]).toMatchObject({ call_id: 'call-b', tool_name: 'Host.List' })
    expect(store.messages[3].thinking).toEqual(['第二轮主机查询'])
    expect(store.messages[4].thinking).toEqual(['正在调用工具: Host.List'])
    expect(store.messages[5].tool_calls?.[0]).toMatchObject({ call_id: 'call-a', tool_name: 'Host.List' })
    expect(store.messages[6].content).toBe('最终分析报告')
  })

  it('刷新历史时隐藏没有工具记录的孤儿工具思考，并为未标记工具补齐思考', async () => {
    const api = await import('@/api/assistant')
    const store = useAssistantStore()
    store.currentSession = { ...session, status: 'completed' }

    const persistedMessage: AssistantMessage = {
      id: 'db-msg-orphan',
      session_id: 'session-1',
      message_id: 'msg-run-orphan',
      role: 'assistant',
      content: '最终结果',
      thinking: [
        '开始收集证据',
        '正在调用工具: Host.List',
        '正在调用工具: Host.List',
        '完成平台证据收集',
      ],
      created_at: '2026-06-07T00:00:01Z',
    }
    const toolCalls: AssistantToolCall[] = [
      {
        id: 'tool-host',
        session_id: 'session-1',
        message_id: 'msg-run-orphan',
        call_id: 'call-host',
        tool_name: 'Host.List',
        risk_level: 'readonly',
        status: 'completed',
        result: { total: 2 },
        created_at: '2026-06-07T00:00:02Z',
      },
      {
        id: 'tool-alert',
        session_id: 'session-1',
        message_id: 'msg-run-orphan',
        call_id: 'call-alert',
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
    store.rebuildAssistantHistoryCycles()

    expect(store.messages).toHaveLength(7)
    expect(store.messages[0].thinking).toEqual(['开始收集证据'])
    expect(store.messages[1].thinking).toEqual(['正在调用工具: Host.List'])
    expect(store.messages[2].tool_calls?.[0]).toMatchObject({ call_id: 'call-host', tool_name: 'Host.List' })
    expect(store.messages[3].thinking).toEqual(['完成平台证据收集'])
    expect(store.messages[4].thinking).toEqual(['正在调用工具: Detection.Alert.List'])
    expect(store.messages[5].tool_calls?.[0]).toMatchObject({ call_id: 'call-alert', tool_name: 'Detection.Alert.List' })
    expect(store.messages[6].content).toBe('最终结果')

    const hostMarkers = store.messages.filter(message =>
      Array.isArray(message.thinking) && message.thinking[0] === '正在调用工具: Host.List'
    )
    expect(hostMarkers).toHaveLength(1)
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

  it('刷新拆分历史后最终消息保留完整计划供计划栏展示', async () => {
    const api = await import('@/api/assistant')
    const store = useAssistantStore()
    store.currentSession = { ...session, status: 'completed' }

    vi.mocked(api.getMessages).mockResolvedValue([{
      id: 'db-msg-plan-full',
      session_id: 'session-1',
      message_id: 'msg-plan-full',
      role: 'assistant',
      content: '最终分析报告',
      thinking: [
        '开始执行步骤: 定位全部主机',
        '步骤完成: 定位全部主机',
        '开始执行步骤: 逐台 Agent 取证',
        '步骤完成: 逐台 Agent 取证',
      ],
      plan: {
        goal: '分析全部主机风险',
        status: 'completed',
        steps: [
          { step_id: 'step-1', title: '定位全部主机', status: 'completed', result_summary: '发现 2 台目标主机' },
          { step_id: 'step-2', title: '逐台 Agent 取证', status: 'completed', result_summary: '2 台主机均已尝试取证' },
        ],
      },
      created_at: '2026-06-07T00:00:01Z',
    }])
    vi.mocked(api.getToolCalls).mockResolvedValue({ items: [], total: 0 } as any)

    await store.fetchMessages('session-1')
    await store.fetchToolCalls('session-1')
    store.rebuildAssistantHistoryCycles()

    const finalMessage = store.messages[store.messages.length - 1]
    expect(finalMessage.content).toBe('最终分析报告')
    expect(finalMessage.plan?.steps).toHaveLength(2)
    expect(finalMessage.plan?.steps.map(step => step.title)).toEqual(['定位全部主机', '逐台 Agent 取证'])

    const stepResultPlans = store.messages
      .filter(message => Array.isArray(message.thinking) && message.thinking[0]?.startsWith('步骤完成'))
      .map(message => message.plan?.steps.length)
    expect(stepResultPlans).toEqual([1, 1])
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
    store.rebuildAssistantHistoryCycles()

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

  // ========== 刷新重建竞态条件测试 ==========

  it('openSession 并行加载后正确重建消息顺序', async () => {
    const api = await import('@/api/assistant')
    const store = useAssistantStore()

    const persistedMessage: AssistantMessage = {
      id: 'db-msg-1',
      session_id: 'session-1',
      message_id: 'msg-run-1',
      role: 'assistant',
      content: '最终报告',
      thinking: [
        '开始分析',
        '正在调用工具: Host.List',
        '分析主机数据',
        '正在调用工具: Alert.List',
        '总结结论',
      ],
      created_at: '2026-06-07T00:00:01Z',
    }
    const toolCalls: AssistantToolCall[] = [
      {
        id: 'tc-1',
        session_id: 'session-1',
        message_id: 'msg-run-1',
        call_id: 'call-1',
        tool_name: 'Host.List',
        risk_level: 'readonly',
        status: 'completed',
        result: { hosts: 5 },
        created_at: '2026-06-07T00:00:02Z',
      },
      {
        id: 'tc-2',
        session_id: 'session-1',
        message_id: 'msg-run-1',
        call_id: 'call-2',
        tool_name: 'Alert.List',
        risk_level: 'readonly',
        status: 'completed',
        result: { alerts: 3 },
        created_at: '2026-06-07T00:00:03Z',
      },
    ]

    vi.mocked(api.getMessages).mockResolvedValue([persistedMessage])
    vi.mocked(api.getToolCalls).mockResolvedValue({ items: toolCalls, total: 2 } as any)
    vi.mocked(api.getSession).mockResolvedValue({ ...session, status: 'completed' })
    vi.mocked(api.getContextRefs).mockResolvedValue([] as any)
    vi.mocked(api.getApprovals).mockResolvedValue({ items: [], total: 0 } as any)

    // 模拟 openSession 的并行加载
    await Promise.all([
      store.fetchMessages('session-1'),
      store.fetchContextRefs('session-1'),
      store.fetchToolCalls('session-1'),
      store.fetchApprovals('session-1'),
    ])
    store.rebuildAssistantHistoryCycles()

    // 验证消息顺序：thinking → tool marker → tool → thinking → tool marker → tool → thinking → content
    expect(store.messages).toHaveLength(8)
    expect(store.messages[0].thinking).toEqual(['开始分析'])
    expect(store.messages[1].thinking).toEqual(['正在调用工具: Host.List'])
    expect(store.messages[2].tool_calls?.[0]).toMatchObject({ call_id: 'call-1', tool_name: 'Host.List' })
    expect(store.messages[3].thinking).toEqual(['分析主机数据'])
    expect(store.messages[4].thinking).toEqual(['正在调用工具: Alert.List'])
    expect(store.messages[5].tool_calls?.[0]).toMatchObject({ call_id: 'call-2', tool_name: 'Alert.List' })
    expect(store.messages[6].thinking).toEqual(['总结结论'])
    expect(store.messages[7].content).toBe('最终报告')
  })

  it('fetchToolCalls 完成后不触发提前重建', async () => {
    const api = await import('@/api/assistant')
    const store = useAssistantStore()

    const persistedMessage: AssistantMessage = {
      id: 'db-msg-1',
      session_id: 'session-1',
      message_id: 'msg-run-1',
      role: 'assistant',
      content: '结果',
      thinking: ['思考', '正在调用工具: Script.Run'],
      created_at: '2026-06-07T00:00:01Z',
    }
    const toolCalls: AssistantToolCall[] = [{
      id: 'tc-1',
      session_id: 'session-1',
      message_id: 'msg-run-1',
      call_id: 'call-1',
      tool_name: 'Script.Run',
      risk_level: 'readonly',
      status: 'completed',
      result: {},
      created_at: '2026-06-07T00:00:02Z',
    }]

    vi.mocked(api.getMessages).mockResolvedValue([persistedMessage])
    vi.mocked(api.getToolCalls).mockResolvedValue({ items: toolCalls, total: 1 } as any)

    // 先加载消息
    await store.fetchMessages('session-1')
    // 消息已加载但工具调用未加载，此时不应有 _history_ 克隆
    expect(store.messages).toHaveLength(1)
    expect(store.messages[0].message_id).toBe('msg-run-1')

    // 加载工具调用（不应触发重建）
    await store.fetchToolCalls('session-1')
    // 工具调用已加载，但 fetchToolCalls 不应触发重建
    // 消息仍应为原始消息（未重建）
    expect(store.messages).toHaveLength(1)
    expect(store.messages[0].message_id).toBe('msg-run-1')

    // 手动触发重建（模拟 openSession 的行为）
    store.rebuildAssistantHistoryCycles()

    // 现在应该正确重建
    expect(store.messages).toHaveLength(4)
    expect(store.messages[0].thinking).toEqual(['思考'])
    expect(store.messages[1].thinking).toEqual(['正在调用工具: Script.Run'])
    expect(store.messages[2].tool_calls?.[0]).toMatchObject({ tool_name: 'Script.Run' })
    expect(store.messages[3].content).toBe('结果')
  })

  it('提前重建遇到工具标记时等待工具调用数据', async () => {
    const api = await import('@/api/assistant')
    const store = useAssistantStore()

    const persistedMessage: AssistantMessage = {
      id: 'db-msg-early',
      session_id: 'session-1',
      message_id: 'msg-run-early',
      role: 'assistant',
      content: '最终结果',
      thinking: ['准备执行脚本', '正在调用工具: Script.Run', '分析脚本结果'],
      created_at: '2026-06-07T00:00:01Z',
    }
    const toolCalls: AssistantToolCall[] = [{
      id: 'tc-early',
      session_id: 'session-1',
      message_id: 'msg-run-early',
      call_id: 'call-early',
      tool_name: 'Script.Run',
      risk_level: 'readonly',
      status: 'completed',
      result: { ok: true },
      created_at: '2026-06-07T00:00:02Z',
    }]

    vi.mocked(api.getMessages).mockResolvedValue([persistedMessage])
    vi.mocked(api.getToolCalls).mockResolvedValue({ items: toolCalls, total: 1 } as any)

    await store.fetchMessages('session-1')
    store.rebuildAssistantHistoryCycles()

    expect(store.messages).toHaveLength(1)
    expect(store.messages[0].message_id).toBe('msg-run-early')

    await store.fetchToolCalls('session-1')
    store.rebuildAssistantHistoryCycles()

    expect(store.messages).toHaveLength(5)
    expect(store.messages[0].thinking).toEqual(['准备执行脚本'])
    expect(store.messages[1].thinking).toEqual(['正在调用工具: Script.Run'])
    expect(store.messages[2].tool_calls?.[0]).toMatchObject({ call_id: 'call-early', tool_name: 'Script.Run' })
    expect(store.messages[3].thinking).toEqual(['分析脚本结果'])
    expect(store.messages[4].content).toBe('最终结果')
  })

  it('刷新历史时按运行事件顺序原样重放 thinking 和工具卡', async () => {
    const api = await import('@/api/assistant')
    const store = useAssistantStore()

    const persistedMessage: AssistantMessage = {
      id: 'db-msg-process',
      session_id: 'session-1',
      message_id: 'msg-run-process',
      role: 'assistant',
      content: '最终分析报告',
      thinking: [
        '正在分析您的问题...',
        '开始执行任务...',
        '开始执行步骤: 获取平台概况',
        '正在调用工具: Host.List',
        '步骤失败: 获取平台概况',
        '步骤完成: 获取平台概况',
        '需要结合主机与漏洞数据判断风险',
        '审计完成: null',
        '正在反思执行过程...',
      ],
      plan: {
        goal: '获取平台概况',
        status: 'completed',
        steps: [{
          step_id: 'step-1',
          title: '获取平台概况',
          status: 'completed',
          result_summary: '已获取 2 台主机资产',
        }],
      },
      created_at: '2026-06-07T00:00:01Z',
    }
    const toolCalls: AssistantToolCall[] = [{
      id: 'tc-process',
      session_id: 'session-1',
      message_id: 'msg-run-process',
      call_id: 'call-process',
      tool_name: 'Host.List',
      risk_level: 'readonly',
      status: 'completed',
      result: { total: 2 },
      created_at: '2026-06-07T00:00:02Z',
    }]

    vi.mocked(api.getMessages).mockResolvedValue([persistedMessage])
    vi.mocked(api.getToolCalls).mockResolvedValue({ items: toolCalls, total: 1 } as any)

    await store.fetchMessages('session-1')
    await store.fetchToolCalls('session-1')
    store.rebuildAssistantHistoryCycles()

    expect(store.messages).toHaveLength(8)
    expect(store.messages[0].thinking).toEqual(['正在分析您的问题...'])
    expect(store.messages[1].thinking).toEqual(['开始执行任务...'])
    expect(store.messages[2].thinking).toEqual(['开始执行步骤: 获取平台概况'])
    expect(store.messages[3].thinking).toEqual(['正在调用工具: Host.List'])
    expect(store.messages[4].tool_calls?.[0]).toMatchObject({ call_id: 'call-process', tool_name: 'Host.List' })
    expect(store.messages[5].thinking).toEqual([
      '步骤完成: 获取平台概况',
    ])
    expect(store.messages[5].plan?.steps[0]).toMatchObject({
      title: '获取平台概况',
      result_summary: '已获取 2 台主机资产',
    })
    expect(store.messages[6].thinking).toEqual(['需要结合主机与漏洞数据判断风险'])
    expect(store.messages[7].content).toBe('最终分析报告')

    const visibleText = store.messages.flatMap(message =>
      Array.isArray(message.thinking) ? message.thinking : []
    ).join('\n')
    expect(visibleText).toContain('开始执行步骤')
    expect(visibleText).toContain('正在调用工具: Host.List')
    expect(visibleText).toContain('步骤完成')
    expect(visibleText).not.toContain('审计完成')
    expect(visibleText).not.toContain('步骤失败')
    expect(visibleText).not.toContain('正在反思执行过程')
    expect(store.messages[store.messages.length - 1].content).toBe('最终分析报告')
  })

  it('openSession 刷新运行中会话时用已落库工具调用恢复时间线', async () => {
    const api = await import('@/api/assistant')
    const store = useAssistantStore()

    const runningSession: AssistantSession = {
      ...session,
      status: 'running',
      metadata: {
        current_run_id: 'run-refresh',
        current_message_id: 'msg-run-refresh',
        current_run_status: 'running',
      },
    }
    const userMessage: AssistantMessage = {
      id: 'user-msg-1',
      session_id: 'session-1',
      message_id: 'user-msg-1',
      role: 'user',
      content: '分析平台安全态势',
      created_at: '2026-06-07T00:00:00Z',
    }
    const toolCalls: AssistantToolCall[] = [{
      id: 'tc-host',
      session_id: 'session-1',
      message_id: 'msg-run-refresh',
      call_id: 'call-host',
      tool_name: 'Host.List',
      risk_level: 'readonly',
      status: 'completed',
      result: { total: 2 },
      created_at: '2026-06-07T00:00:01Z',
    }, {
      id: 'tc-stat',
      session_id: 'session-1',
      message_id: 'msg-run-refresh',
      call_id: 'call-stat',
      tool_name: 'Detection.Statistics.Get',
      risk_level: 'readonly',
      status: 'completed',
      result: { active_rules: 4 },
      created_at: '2026-06-07T00:00:02Z',
    }]

    vi.mocked(api.getSession).mockResolvedValue(runningSession)
    vi.mocked(api.getMessages).mockResolvedValue([userMessage])
    vi.mocked(api.getContextRefs).mockResolvedValue([])
    vi.mocked(api.getToolCalls).mockResolvedValue({ items: toolCalls, total: 2 } as any)
    vi.mocked(api.getApprovals).mockResolvedValue({ items: [], total: 0 } as any)
    vi.mocked(api.createAssistantStream).mockReturnValue({ close: vi.fn() } as any)

    await store.openSession('session-1')

    const assistantMessage = store.messages.find(message => message.role === 'assistant')
    expect(assistantMessage).toBeTruthy()
    expect(assistantMessage?.message_id).toBe('msg-run-refresh')
    expect(assistantMessage?.thinking).toEqual([
      '正在调用工具: Host.List',
      '正在调用工具: Detection.Statistics.Get',
    ])
    expect(assistantMessage?.tool_calls).toHaveLength(2)
    expect(assistantMessage?.tool_calls?.[0]).toMatchObject({ call_id: 'call-host', result: { total: 2 } })
    expect(assistantMessage?.tool_calls?.[1]).toMatchObject({ call_id: 'call-stat', result: { active_rules: 4 } })
    expect(api.createAssistantStream).toHaveBeenCalledWith('session-1')

    store.stopStream()
  })

  it('工具调用超过 20 个时通过 page_size=100 获取全部', async () => {
    const api = await import('@/api/assistant')
    const store = useAssistantStore()

    // 生成 30 个工具调用
    const manyToolCalls: AssistantToolCall[] = Array.from({ length: 30 }, (_, i) => ({
      id: `tc-${i}`,
      session_id: 'session-1',
      message_id: 'msg-run-1',
      call_id: `call-${i}`,
      tool_name: `Tool.${i}`,
      risk_level: 'readonly',
      status: 'completed',
      result: {},
      created_at: `2026-06-07T00:00:${String(i).padStart(2, '0')}Z`,
    }))

    // 生成对应的 thinking 步骤
    const thinkingSteps: string[] = []
    for (let i = 0; i < 30; i++) {
      thinkingSteps.push(`步骤 ${i}`)
      thinkingSteps.push(`正在调用工具: Tool.${i}`)
    }
    thinkingSteps.push('最终分析')

    const persistedMessage: AssistantMessage = {
      id: 'db-msg-1',
      session_id: 'session-1',
      message_id: 'msg-run-1',
      role: 'assistant',
      content: '最终结果',
      thinking: thinkingSteps,
      created_at: '2026-06-07T00:00:01Z',
    }

    vi.mocked(api.getMessages).mockResolvedValue([persistedMessage])
    // 模拟 API 返回全部 30 个工具调用（page_size=100）
    vi.mocked(api.getToolCalls).mockResolvedValue({ items: manyToolCalls, total: 30 } as any)
    vi.mocked(api.getSession).mockResolvedValue({ ...session, status: 'completed' })
    vi.mocked(api.getContextRefs).mockResolvedValue([] as any)
    vi.mocked(api.getApprovals).mockResolvedValue({ items: [], total: 0 } as any)

    await Promise.all([
      store.fetchMessages('session-1'),
      store.fetchToolCalls('session-1'),
    ])
    store.rebuildAssistantHistoryCycles()

    // 验证所有工具调用都被正确关联
    const toolSegments = store.messages.filter(m => m.tool_calls?.length)
    expect(toolSegments).toHaveLength(30)

    // 验证 fetchToolCalls 使用了 page_size=100
    expect(api.getToolCalls).toHaveBeenCalledWith('session-1', expect.objectContaining({ page_size: 100 }))
  })

  it('工具调用超过 100 个时继续拉取后续分页', async () => {
    const api = await import('@/api/assistant')
    const store = useAssistantStore()

    const firstPageCalls: AssistantToolCall[] = Array.from({ length: 100 }, (_, i) => ({
      id: `tc-${i}`,
      session_id: 'session-1',
      message_id: 'msg-run-1',
      call_id: `call-${i}`,
      tool_name: `Tool.${i}`,
      risk_level: 'readonly',
      status: 'completed',
      result: {},
      created_at: `2026-06-07T00:00:${String(i % 60).padStart(2, '0')}Z`,
    }))
    const secondPageCalls: AssistantToolCall[] = Array.from({ length: 30 }, (_, i) => {
      const index = i + 100
      return {
        id: `tc-${index}`,
        session_id: 'session-1',
        message_id: 'msg-run-1',
        call_id: `call-${index}`,
        tool_name: `Tool.${index}`,
        risk_level: 'readonly',
        status: 'completed',
        result: {},
        created_at: `2026-06-07T00:01:${String(i % 60).padStart(2, '0')}Z`,
      }
    })

    vi.mocked(api.getToolCalls)
      .mockResolvedValueOnce({ items: firstPageCalls, total: 130, page: 1, page_size: 100 } as any)
      .mockResolvedValueOnce({ items: secondPageCalls, total: 130, page: 2, page_size: 100 } as any)

    await store.fetchToolCalls('session-1')

    expect(store.toolCalls).toHaveLength(130)
    expect(api.getToolCalls).toHaveBeenNthCalledWith(1, 'session-1', expect.objectContaining({ page: 1, page_size: 100 }))
    expect(api.getToolCalls).toHaveBeenNthCalledWith(2, 'session-1', expect.objectContaining({ page: 2, page_size: 100 }))
  })

  it('does not display an accepted asynchronous operation as success', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    store.applyStreamEvent({
      type: 'tool_call',
      session_id: 'session-1',
      run_id: 'run-async',
      payload: {
        call_id: 'call-generate',
        tool_name: 'Vulnerability.Script.Generate',
        args: { cve_id: 'CVE-2021-45340', script_type: 'fix' },
      },
    })
    store.applyStreamEvent({
      type: 'tool_result',
      session_id: 'session-1',
      run_id: 'run-async',
      payload: {
        call_id: 'call-generate',
        result: { status: 'accepted', generation_id: 'gen-1' },
        operation_status: 'accepted',
        terminal: false,
      },
    })

    expect(store.toolCalls[0].status).toBe('accepted')
    expect(store.toolCalls[0].result_summary).toContain('尚未完成')
  })

  it('displays a business failure even when the tool transport returned a result', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    store.applyStreamEvent({
      type: 'tool_result',
      session_id: 'session-1',
      run_id: 'run-failed',
      payload: {
        call_id: 'call-dispatch',
        result: { success: true },
        operation_status: 'failed',
        terminal: true,
      },
    })

    expect(store.toolCalls[0].status).toBe('failed')
    expect(store.toolCalls[0].error_message).toContain('业务操作执行失败')
  })

  it('displays a terminal partial weak-password result without marking the query as failed', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    store.applyStreamEvent({
      type: 'tool_call',
      session_id: 'session-1',
      run_id: 'run-weak-password',
      payload: {
        call_id: 'call-weak-password-progress',
        tool_name: 'Credential.WeakPassword.QueryProgress',
        args: { task_ids: ['task-1', 'task-2'] },
      },
    })
    store.applyStreamEvent({
      type: 'tool_result',
      session_id: 'session-1',
      run_id: 'run-weak-password',
      payload: {
        call_id: 'call-weak-password-progress',
        result: {
          status: 'partial_failed',
          task_total: 6,
          task_completed: 2,
          task_failed: 4,
          task_running: 0,
          matched_findings: 2,
        },
        operation_status: 'succeeded',
        terminal: true,
      },
    })

    expect(store.toolCalls).toHaveLength(1)
    expect(store.toolCalls[0].status).toBe('completed')
    expect(store.toolCalls[0].error_message).toBeFalsy()
    expect(store.toolCalls[0].result_summary).toContain('部分失败')
    expect(store.toolCalls[0].result_summary).toContain('总计 6')
    expect(store.toolCalls[0].result_summary).toContain('失败 4')
    expect(store.toolCalls[0].result_summary).toContain('命中 2')
  })

  it('does not mark a failed goal as a completed session when the stream closes', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    store.applyStreamEvent({
      type: 'done',
      session_id: 'session-1',
      run_id: 'run-goal-failed',
      payload: {
        status: 'failed',
        goal_outcome: 'failed',
      },
    })

    expect(store.currentSession?.status).toBe('failed')
    expect(store.currentSession?.metadata?.current_run_status).toBe('failed')
    expect(store.currentSession?.metadata?.goal_outcome).toBe('failed')
  })

  it('keeps a needs-input goal active when the stream closes', () => {
    const store = useAssistantStore()
    store.currentSession = { ...session }

    store.applyStreamEvent({
      type: 'done',
      session_id: 'session-1',
      run_id: 'run-needs-input',
      payload: {
        status: 'needs_input',
        goal_outcome: 'needs_input',
      },
    })

    expect(store.currentSession?.status).toBe('active')
  })
})
