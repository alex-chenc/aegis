import { defineStore } from 'pinia'
import { ref } from 'vue'
import request from '@/api'
import {
  getSessions,
  createSession as apiCreateSession,
  getSession,
  getMessages,
  sendMessage as apiSendMessage,
  cancelRun as apiCancelRun,
  getContextRefs as apiGetContextRefs,
  uploadAssistantFile as apiUploadAssistantFile,
  getToolCalls as apiGetToolCalls,
  getApprovals as apiGetApprovals,
  approveApproval as apiApproveApproval,
  rejectApproval as apiRejectApproval,
  createAssistantStream,
  type AssistantSession,
  type AssistantMessage,
  type AssistantContextRef,
  type AssistantToolCall,
  type AssistantApproval,
  type AssistantResultCard,
  type AssistantFileUploadPurpose,
  type AssistantFileUploadResult,
  type AssistantIntentResult,
  type AssistantToolSelection,
  type AssistantToolSearchResult,
  type AssistantPlan,
  type AssistantTaskType,
  type CreateSessionRequest,
  type SendMessageRequest,
  type RunHandle,
  type SessionsQueryParams,
  type ToolCallsQueryParams,
  type ApprovalsQueryParams,
} from '@/api/assistant'
import type { ContextBudgetEvent, ContextCompressedEvent } from '@/api/aiAnalysis'
import { getCurrentLocale, translate } from '@/i18n'

export const useAssistantStore = defineStore('assistant', () => {
  // ============================================
  // 状态
  // ============================================

  /** 会话列表 */
  const sessions = ref<AssistantSession[]>([])
  /** 当前会话 */
  const currentSession = ref<AssistantSession | null>(null)
  /** 当前会话消息列表 */
  const messages = ref<AssistantMessage[]>([])
  /** 当前会话上下文引用 */
  const contextRefs = ref<AssistantContextRef[]>([])
  /** 当前会话工具调用列表 */
  const toolCalls = ref<AssistantToolCall[]>([])
  /** 当前会话审批列表 */
  const approvals = ref<AssistantApproval[]>([])
  /** 当前会话结果卡片 */
  const resultCards = ref<AssistantResultCard[]>([])
  /** 意图识别结果 */
  const intentResult = ref<AssistantIntentResult | null>(null)
  /** 工具选择列表 */
  const toolSelections = ref<AssistantToolSelection[]>([])
  /** 工具搜索结果 */
  const toolSearchResults = ref<AssistantToolSearchResult[]>([])
  /** 当前会话上下文预算 */
  const contextBudget = ref<ContextBudgetEvent | null>(null)
  /** 当前会话上下文压缩记录 */
  const compressionRecords = ref<ContextCompressedEvent[]>([])
  /** 当前会话累计 prompt tokens */
  const totalPromptTokens = ref(0)
  /** 当前会话累计 completion tokens */
  const totalCompletionTokens = ref(0)
  /** 是否正在流式接收 */
  const streaming = ref(false)
  /** 全局加载状态 */
  const loading = ref(false)
  /** 错误信息 */
  const error = ref<string | null>(null)
  /** 待创建会话的任务类型（点击快捷任务后设置，发送消息时才真正创建会话） */
  const pendingTaskType = ref<AssistantTaskType | null>(null)

  /** SSE 连接实例 */
  let eventSource: EventSource | null = null

  // 循环分割状态跟踪
  /** 上一个事件类型，用于检测循环边界 */
  let lastEventType: string | null = null
  /** 当前循环的消息ID */
  let currentCycleMessageId: string | null = null
  /** 当前运行内的循环序号，用于生成稳定的前端消息ID */
  let currentCycleIndex = 0

  // ============================================
  // Actions
  // ============================================

  // 分页状态
  const sessionPage = ref(1)
  const sessionTotal = ref(0)
  const hasMoreSessions = ref(false)
  const loadingMore = ref(false)
  const sessionQuery = ref<SessionsQueryParams>({})

  function normalizeSessionQuery(params?: SessionsQueryParams): SessionsQueryParams {
    if (!params) return {}
    const query = { ...params }
    delete query.page
    delete query.page_size
    return query
  }

  function normalizeMetadata(metadata?: Record<string, any> | string | null): Record<string, any> {
    if (!metadata) return {}
    if (typeof metadata === 'string') {
      try {
        const parsed = JSON.parse(metadata)
        return parsed && typeof parsed === 'object' ? parsed : {}
      } catch {
        return {}
      }
    }
    return metadata
  }

  function applySessionRuntimeMetadata(session?: AssistantSession | null) {
    const metadata = normalizeMetadata(session?.metadata)
    contextBudget.value = metadata.context_budget || null
    compressionRecords.value = Array.isArray(metadata.compression_records)
      ? metadata.compression_records
      : []
    totalPromptTokens.value = Number(metadata.total_prompt_tokens || 0)
    totalCompletionTokens.value = Number(metadata.total_completion_tokens || 0)
  }

  function updateCurrentSessionMetadata(updates: Record<string, any>) {
    if (!currentSession.value) return
    const cleanUpdates = Object.fromEntries(
      Object.entries(updates).filter(([, value]) => value !== undefined)
    )
    currentSession.value.metadata = {
      ...normalizeMetadata(currentSession.value.metadata),
      ...cleanUpdates,
    }
  }

  function resetRuntimeMetrics() {
    contextBudget.value = null
    compressionRecords.value = []
    totalPromptTokens.value = 0
    totalCompletionTokens.value = 0
  }

  function inferSessionTitle(content: string) {
    const trimmed = content.trim()
    if (!trimmed) return translate('assistant.session.new')
    return Array.from(trimmed).slice(0, 18).join('')
  }

  function normalizeMessages(input: AssistantMessage[]) {
    const normalized: AssistantMessage[] = []
    for (const msg of input || []) {
      const current = normalizeMessageForCurrentSession(msg)
      const prev = normalized[normalized.length - 1]
      if (
        prev &&
        prev.role === 'user' &&
        current.role === 'user' &&
        prev.content === current.content &&
        Math.abs(new Date(current.created_at).getTime() - new Date(prev.created_at).getTime()) <= 10_000
      ) {
        continue
      }
      normalized.push(current)
    }
    return normalized
  }

  function normalizeMessageForCurrentSession(msg: AssistantMessage): AssistantMessage {
    if (!msg.plan) return msg
    return {
      ...msg,
      plan: normalizePlanForCurrentSession(msg.plan),
    }
  }

  function normalizePlanForCurrentSession(plan: NonNullable<AssistantMessage['plan']>): NonNullable<AssistantMessage['plan']> {
    const sessionStatus = currentSession.value?.status
    const terminalStatus = sessionStatus === 'completed' || sessionStatus === 'failed' || sessionStatus === 'cancelled'
      ? sessionStatus
      : ''
    if (!terminalStatus) {
      return {
        ...plan,
        steps: (plan.steps || []).map(step => ({ ...step })),
      }
    }

    return {
      ...plan,
      status: terminalStatus,
      steps: (plan.steps || []).map(step => ({
        ...step,
        status: step.status === 'pending' || step.status === 'running'
          ? terminalStatus
          : step.status,
      })),
    }
  }

  function normalizeThinkingSteps(thinking: AssistantMessage['thinking']): string[] {
    if (Array.isArray(thinking)) {
      return thinking.map(step => String(step).trim()).filter(Boolean).filter(step => !isHiddenInternalThinkingStep(step))
    }
    return (thinking || '')
      .split('\n')
      .map(step => step.trim())
      .filter(Boolean)
      .filter(step => !isHiddenInternalThinkingStep(step))
  }

  function matchToolCallThinkingStep(step: string) {
    return step.match(/^(?:正在调用工具|Calling tool)[:：]\s*(.+)$/i)
  }

  function matchStepCompletedThinkingStep(step: string) {
    const match = step.match(/^(?:步骤完成|Step completed)[:：]\s*(.+)$/i)
    return match?.[1]?.trim() || ''
  }

  function isHiddenInternalThinkingStep(step: string) {
    return /^(?:正在反思执行过程|Reflecting on execution)/i.test(step) ||
      /^(?:反思结果|Reflection result)[:：]/i.test(step) ||
      /^(?:步骤失败|Step failed)[:：]/i.test(step) ||
      /^(?:正在重试步骤|Retrying step)[:：]/i.test(step) ||
      /^(?:正在审计执行进度|Auditing execution progress)/i.test(step) ||
      /^(?:审计完成|Audit complete)(?:[:：]|$)/i.test(step)
  }

  function buildStepResultPlan(source: AssistantMessage, completedStepTitle: string): AssistantMessage['plan'] | undefined {
    const title = completedStepTitle.trim()
    if (!title) return undefined

    const steps = source.plan?.steps || []
    const matchedStep = steps.find(step => step.title === title)
    const resultSummary = matchedStep?.result_summary || translate('assistant.progress.stepCompleted', { title })

    return {
      goal: source.plan?.goal || '',
      status: source.plan?.status || 'running',
      steps: [{
        step_id: matchedStep?.step_id || `step-result-${title}`,
        title,
        status: 'completed',
        result_summary: resultSummary,
      }],
    }
  }

  function extractPlanFromRuntimeEvents(events: RuntimeDisplayEvent[]): AssistantMessage['plan'] | undefined {
    let plan: NonNullable<AssistantMessage['plan']> | undefined

    for (const event of events) {
      const payload = event.payload || {}
      if (event.type === 'plan') {
        plan = normalizeAssistantPlan(payload as AssistantPlan)
        continue
      }

      if (!plan || !['step_started', 'step_completed', 'step_failed', 'step_retrying'].includes(event.type)) {
        continue
      }

      const stepId = String(payload.step_id || payload.id || '').trim()
      if (!stepId) continue

      const step = plan.steps.find(item => item.step_id === stepId)
      if (!step) continue

      if (event.type === 'step_started') {
        step.status = payload.status || 'running'
      } else if (event.type === 'step_completed') {
        step.status = payload.status || 'completed'
        const resultSummary = payload.result_summary || payload.summary
        if (resultSummary) {
          step.result_summary = resultSummary
        }
      } else if (event.type === 'step_retrying') {
        step.status = 'retrying'
      } else {
        step.status = 'failed'
        if (payload.error) {
          step.result_summary = payload.error
        }
      }

      if (payload.title && !step.title) {
        step.title = payload.title
      }
    }

    return plan
  }

  type RuntimeDisplayEvent = {
    type: string
    run_id?: string
    message_id?: string
    timestamp?: string
    payload?: Record<string, any>
  }

  function normalizeRuntimeDisplayEvents(input: any): RuntimeDisplayEvent[] {
    if (!Array.isArray(input)) return []
    return input
      .map(item => item && typeof item === 'object' ? item as RuntimeDisplayEvent : null)
      .filter((item): item is RuntimeDisplayEvent => Boolean(item?.type))
  }

  function getRuntimeDisplayEventsForMessage(msg: AssistantMessage): RuntimeDisplayEvent[] {
    const metadata = normalizeMetadata(currentSession.value?.metadata)
    const timeline = metadata.assistant_runtime_events
    if (Array.isArray(timeline)) {
      return normalizeRuntimeDisplayEvents(timeline).filter(event =>
        !event.message_id || event.message_id === msg.message_id || event.message_id === msg.id
      )
    }
    if (timeline && typeof timeline === 'object') {
      return normalizeRuntimeDisplayEvents(timeline[msg.message_id] || timeline[msg.id])
    }
    return []
  }

  function isHistoryDisplayMessage(msg: AssistantMessage) {
    return msg.message_id.includes('_history_')
  }

  function getRelatedToolCalls(msg: AssistantMessage) {
    const toolCallMap = new Map<string, AssistantToolCall>()
    const addToolCall = (tc: AssistantToolCall) => {
      toolCallMap.set(tc.call_id || tc.id, tc)
    }

    for (const tc of msg.tool_calls || []) {
      addToolCall(tc)
    }
    for (const tc of toolCalls.value) {
      const sameSession = !msg.session_id || !tc.session_id || tc.session_id === msg.session_id
      const sameMessage = tc.message_id === msg.message_id || tc.message_id === msg.id
      if (sameSession && sameMessage) {
        addToolCall(tc)
      }
    }

    return Array.from(toolCallMap.values()).sort((left, right) => {
      const leftTime = new Date(left.created_at || '').getTime() || 0
      const rightTime = new Date(right.created_at || '').getTime() || 0
      return leftTime - rightTime
    })
  }

  function isSettledToolCallStatus(status: AssistantToolCall['status']) {
    return [
      'accepted',
      'completed',
      'success',
      'failed',
      'cancelled',
      'approval_required',
      'rejected',
    ].includes(status)
  }

  function displayStatusForToolOutcome(
    operationStatus: AssistantToolCall['operation_status'],
    terminal: boolean | undefined,
    fallback: AssistantToolCall['status']
  ): AssistantToolCall['status'] {
    switch (operationStatus) {
      case 'accepted':
        return 'accepted'
      case 'running':
        return 'running'
      case 'failed':
        return 'failed'
      case 'succeeded':
      case 'skipped':
        return terminal === false ? 'running' : 'completed'
      default:
        return fallback
    }
  }

  function normalizeToolCallBusinessOutcome(call: AssistantToolCall): AssistantToolCall {
    return {
      ...call,
      status: displayStatusForToolOutcome(call.operation_status, call.terminal, call.status),
    }
  }

  function summarizeOperationOutcome(payload: any): string {
    switch (payload.operation_status) {
      case 'accepted':
        return translate('assistant.outcome.accepted')
      case 'running':
        return translate('assistant.outcome.running')
      case 'failed':
        return translate('assistant.outcome.failed')
      case 'skipped':
        return translate('assistant.outcome.skipped')
      default:
        return summarizeToolResult(payload.result)
    }
  }

  function sortedToolCallsForMessage(messageId: string) {
    return toolCalls.value
      .filter(tc => {
        const sameSession = !currentSession.value?.session_id || tc.session_id === currentSession.value.session_id
        return sameSession && tc.message_id === messageId
      })
      .sort((left, right) => {
        const leftTime = new Date(left.created_at || '').getTime() || 0
        const rightTime = new Date(right.created_at || '').getTime() || 0
        return leftTime - rightTime
      })
  }

  function resolveRunningMessageIdFromSession() {
    const metadata = normalizeMetadata(currentSession.value?.metadata)
    const messageId = String(metadata.current_message_id || '').trim()
    if (messageId) return messageId
    const runId = String(metadata.current_run_id || '').trim()
    return runId ? `msg_${runId}` : ''
  }

  function restoreRunningAssistantTimeline() {
    const metadata = normalizeMetadata(currentSession.value?.metadata)
    const isRunning = currentSession.value?.status === 'running' || metadata.current_run_status === 'running'
    if (!isRunning) return

    const messageId = resolveRunningMessageIdFromSession()
    if (!messageId) return

    const relatedToolCalls = sortedToolCallsForMessage(messageId)
    if (relatedToolCalls.length === 0) {
      currentCycleMessageId = messageId
      return
    }

    let message = messages.value.find(msg =>
      msg.role === 'assistant' &&
      !isHistoryDisplayMessage(msg) &&
      (msg.message_id === messageId || msg.id === messageId)
    )

    if (!message) {
      const hasHistoryFragments = messages.value.some(msg =>
        msg.role === 'assistant' &&
        isHistoryDisplayMessage(msg) &&
        msg.message_id.startsWith(`${messageId}_history_`)
      )
      if (!hasHistoryFragments) {
        message = {
          id: messageId,
          session_id: currentSession.value?.session_id || relatedToolCalls[0]?.session_id || '',
          message_id: messageId,
          role: 'assistant',
          content: '',
          thinking: relatedToolCalls.map(tc => translate('assistant.progress.callingTool', { name: tc.tool_name })),
          tool_calls: relatedToolCalls.map(tc => ({
            ...tc,
            message_id: messageId,
          })),
          created_at: relatedToolCalls[0]?.created_at || currentSession.value?.updated_at || new Date().toISOString(),
        }
        messages.value.push(message)
      }
    } else {
      const toolCallMap = new Map<string, AssistantToolCall>()
      for (const tc of message.tool_calls || []) {
        toolCallMap.set(tc.call_id || tc.id, tc)
      }
      for (const tc of relatedToolCalls) {
        toolCallMap.set(tc.call_id || tc.id, {
          ...tc,
          message_id: message.message_id,
        })
      }
      message.tool_calls = Array.from(toolCallMap.values()).sort((left, right) => {
        const leftTime = new Date(left.created_at || '').getTime() || 0
        const rightTime = new Date(right.created_at || '').getTime() || 0
        return leftTime - rightTime
      })

      const steps = normalizeThinkingSteps(message.thinking)
      const hasToolMarker = steps.some(step => matchToolCallThinkingStep(step))
      if (!hasToolMarker) {
        message.thinking = [
          ...steps,
          ...relatedToolCalls.map(tc => translate('assistant.progress.callingTool', { name: tc.tool_name })),
        ]
      }
    }

    currentCycleMessageId = messageId
    const lastToolCall = relatedToolCalls[relatedToolCalls.length - 1]
    lastEventType = lastToolCall && isSettledToolCallStatus(lastToolCall.status)
      ? 'tool_result'
      : 'tool_call'
  }

  function cloneHistoryMessage(
    source: AssistantMessage,
    index: number,
    overrides: Partial<AssistantMessage>
  ): AssistantMessage {
    const baseId = source.message_id || source.id || 'assistant'
    const messageId = `${baseId}_history_${index}`
    return {
      ...source,
      id: messageId,
      message_id: messageId,
      content: '',
      thinking: undefined,
      plan: undefined,
      tool_calls: undefined,
      approvals: undefined,
      result_cards: undefined,
      context_refs: undefined,
      ...overrides,
    }
  }

  function rebuildAssistantHistoryCycles() {
    const rebuilt: AssistantMessage[] = []
    let changed = false

    for (const msg of messages.value) {
      if (msg.role !== 'assistant' || isHistoryDisplayMessage(msg)) {
        rebuilt.push(msg)
        continue
      }

      const relatedToolCalls = getRelatedToolCalls(msg)
      const thinkingSteps = normalizeThinkingSteps(msg.thinking)
      const runtimeEvents = getRuntimeDisplayEventsForMessage(msg)
      const hasToolMarker = thinkingSteps.some(step => matchToolCallThinkingStep(step))
      const runtimePlan = extractPlanFromRuntimeEvents(runtimeEvents)
      const sourcePlan = msg.plan || runtimePlan
      const planSourceMessage = sourcePlan ? { ...msg, plan: sourcePlan } : msg
      const hasRenderableResult = Boolean(
        msg.content ||
        sourcePlan ||
        msg.approvals?.length ||
        msg.result_cards?.length ||
        msg.context_refs?.length
      )
      if (runtimeEvents.length > 0) {
        changed = true
        const displayMessages: AssistantMessage[] = []
        const usedToolCalls = new Set<string>()
        let displayIndex = 1

        const pushThinkingStep = (step: string) => {
          const content = step.trim()
          if (!content || isHiddenInternalThinkingStep(content)) return
          const completedStepTitle = matchStepCompletedThinkingStep(content)
          displayMessages.push(cloneHistoryMessage(msg, displayIndex++, {
            thinking: [content],
            plan: completedStepTitle ? buildStepResultPlan(planSourceMessage, completedStepTitle) : undefined,
          }))
        }

        const pushToolCall = (toolCall: AssistantToolCall) => {
          const toolMessageId = `${msg.message_id || msg.id}_history_${displayIndex}`
          displayMessages.push(cloneHistoryMessage(msg, displayIndex++, {
            tool_calls: [{
              ...toolCall,
              message_id: toolMessageId,
            }],
          }))
        }

        const takeToolCallByCallID = (callID?: string) => {
          const normalizedCallID = callID?.trim()
          if (!normalizedCallID) return undefined
          const match = relatedToolCalls.find(tc =>
            !usedToolCalls.has(tc.call_id || tc.id) &&
            (tc.call_id === normalizedCallID || tc.id === normalizedCallID)
          )
          if (match) {
            usedToolCalls.add(match.call_id || match.id)
          }
          return match
        }

        const takeToolCallByName = (toolName?: string) => {
          const normalizedName = toolName?.trim()
          if (!normalizedName) return undefined
          const match = relatedToolCalls.find(tc =>
            !usedToolCalls.has(tc.call_id || tc.id) &&
            tc.tool_name === normalizedName
          )
          if (match) {
            usedToolCalls.add(match.call_id || match.id)
          }
          return match
        }

        for (const event of runtimeEvents) {
          const payload = event.payload || {}
          if (event.type === 'thinking') {
            const content = String(payload.content || payload.message || '').trim()
            const toolMatch = matchToolCallThinkingStep(content)
            if (toolMatch) {
              const eventCallID = String(payload.call_id || '').trim()
              const toolCall = eventCallID
                ? takeToolCallByCallID(eventCallID)
                : takeToolCallByName(String(payload.tool_name || toolMatch[1] || ''))
              if (toolCall) {
                pushThinkingStep(content)
                pushToolCall(toolCall)
              }
              continue
            }
            pushThinkingStep(content)
          } else if (event.type === 'tool_call') {
            const eventCallID = String(payload.call_id || '').trim()
            const toolCall = eventCallID
              ? takeToolCallByCallID(eventCallID)
              : takeToolCallByName(String(payload.tool_name || ''))
            if (toolCall) {
              pushThinkingStep(translate('assistant.progress.callingTool', { name: toolCall.tool_name }))
              pushToolCall(toolCall)
            }
          }
        }

        for (const toolCall of relatedToolCalls) {
          if (usedToolCalls.has(toolCall.call_id || toolCall.id)) continue
          pushThinkingStep(translate('assistant.progress.callingTool', { name: toolCall.tool_name }))
          pushToolCall(toolCall)
        }

        const hasRenderableFinalResult = Boolean(
          msg.content ||
          msg.approvals?.length ||
          msg.result_cards?.length ||
          msg.context_refs?.length
        )

        if (hasRenderableFinalResult || sourcePlan) {
          displayMessages.push(cloneHistoryMessage(msg, displayIndex++, {
            content: msg.content,
            plan: sourcePlan,
            approvals: msg.approvals,
            result_cards: msg.result_cards,
            context_refs: msg.context_refs,
          }))
        }

        rebuilt.push(...(displayMessages.length ? displayMessages : [msg]))
        continue
      }
      if (hasToolMarker && relatedToolCalls.length === 0) {
        rebuilt.push(msg)
        continue
      }
      const shouldRebuild = relatedToolCalls.length > 0 ||
        hasToolMarker ||
        thinkingSteps.length > 1 ||
        (thinkingSteps.length === 1 && hasRenderableResult)
      if (!shouldRebuild) {
        rebuilt.push(msg)
        continue
      }

      changed = true
      const displayMessages: AssistantMessage[] = []
      const usedToolCalls = new Set<string>()
      let displayIndex = 1

      const pushThinkingStep = (step: string) => {
        const completedStepTitle = matchStepCompletedThinkingStep(step)
        displayMessages.push(cloneHistoryMessage(msg, displayIndex++, {
          thinking: [step],
          plan: completedStepTitle ? buildStepResultPlan(msg, completedStepTitle) : undefined,
        }))
      }

      const pushToolCall = (toolCall: AssistantToolCall) => {
        const toolMessageId = `${msg.message_id || msg.id}_history_${displayIndex}`
        displayMessages.push(cloneHistoryMessage(msg, displayIndex++, {
          tool_calls: [{
            ...toolCall,
            message_id: toolMessageId,
          }],
        }))
      }

      const takeToolCall = (toolName?: string) => {
        const normalizedName = toolName?.trim()
        let match: AssistantToolCall | undefined
        if (normalizedName) {
          match = relatedToolCalls.find(tc =>
            !usedToolCalls.has(tc.call_id || tc.id) &&
            tc.tool_name === normalizedName
          )
        } else {
          match = relatedToolCalls.find(tc => !usedToolCalls.has(tc.call_id || tc.id))
        }
        if (match) {
          usedToolCalls.add(match.call_id || match.id)
        }
        return match
      }

      for (const step of thinkingSteps) {
        const toolMatch = matchToolCallThinkingStep(step)
        if (toolMatch) {
          const toolCall = takeToolCall(toolMatch[1])
          if (toolCall) {
            pushThinkingStep(step)
            pushToolCall(toolCall)
          }
          continue
        }
        pushThinkingStep(step)
      }

      for (const toolCall of relatedToolCalls) {
        if (usedToolCalls.has(toolCall.call_id || toolCall.id)) continue
        pushThinkingStep(translate('assistant.progress.callingTool', { name: toolCall.tool_name }))
        pushToolCall(toolCall)
      }

      const hasRenderableFinalResult = Boolean(
        msg.content ||
        msg.approvals?.length ||
        msg.result_cards?.length ||
        msg.context_refs?.length
      )

      if (hasRenderableFinalResult || msg.plan) {
        displayMessages.push(cloneHistoryMessage(msg, displayIndex++, {
          content: msg.content,
          plan: msg.plan,
          approvals: msg.approvals,
          result_cards: msg.result_cards,
          context_refs: msg.context_refs,
        }))
      }

      rebuilt.push(...(displayMessages.length ? displayMessages : [msg]))
    }

    if (changed) {
      messages.value = rebuilt
    }
  }

  /**
   * 获取会话列表
   * @param params 查询参数
   * @param append 是否追加模式（加载更多），默认 false（替换模式）
   */
  async function fetchSessions(params?: SessionsQueryParams, append = false) {
    if (append) {
      loadingMore.value = true
    } else {
      loading.value = true
      sessionPage.value = 1
      sessionQuery.value = normalizeSessionQuery(params)
    }
    error.value = null
    try {
      const queryPage = append ? sessionPage.value + 1 : 1
      const query = append ? sessionQuery.value : normalizeSessionQuery(params)
      const result = await getSessions({ ...query, page: queryPage, page_size: 10 })
      // API 返回 { sessions: [...], total: N } 或 { items: [...], total: N }
      // 兼容两种格式
      const items = result?.sessions || result?.items || []
      const total = result?.total || 0

      if (append) {
        sessions.value = [...sessions.value, ...items]
        sessionPage.value = queryPage
      } else {
        sessions.value = items
      }
      sessionTotal.value = total
      hasMoreSessions.value = sessions.value.length < total
      return result
    } catch (err: any) {
      // 不抛出错误，避免页面崩溃
      error.value = err.message || translate('assistant.errors.fetchSessions')
      if (!append) sessions.value = []
      return { sessions: [], total: 0 }
    } finally {
      loading.value = false
      loadingMore.value = false
    }
  }

  /**
   * 跳转到指定页
   */
  async function goToSessionPage(page: number) {
    sessionPage.value = page
    loading.value = true
    error.value = null
    try {
      const result = await getSessions({ ...sessionQuery.value, page, page_size: 10 })
      const items = result?.sessions || result?.items || []
      sessions.value = items
      sessionTotal.value = result?.total || 0
      hasMoreSessions.value = sessions.value.length < sessionTotal.value
      return result
    } catch (err: any) {
      error.value = err.message || translate('assistant.errors.fetchSessions')
      sessions.value = []
      return { sessions: [], total: 0 }
    } finally {
      loading.value = false
    }
  }

  /**
   * 删除会话
   */
  async function deleteSession(sessionId: string) {
    try {
      await request.delete(`/assistant/sessions/${sessionId}`)
      // 从列表中移除
      sessions.value = sessions.value.filter(s => s.session_id !== sessionId)
      sessionTotal.value = Math.max(0, sessionTotal.value - 1)
      // 如果删除的是当前会话，清空当前会话
      if (currentSession.value?.session_id === sessionId) {
        currentSession.value = null
        messages.value = []
        resetRuntimeMetrics()
      }
      return true
    } catch (err: any) {
      error.value = err.message || translate('assistant.errors.deleteSession')
      return false
    }
  }

  /**
   * 创建会话
   */
  async function createSession(data: CreateSessionRequest) {
    loading.value = true
    error.value = null
    try {
      const session = await apiCreateSession(data)
      if (session) {
        sessions.value.unshift(session)
        currentSession.value = session
        // 清空当前会话的所有状态
        messages.value = []
        toolCalls.value = []
        approvals.value = []
        resultCards.value = []
        contextRefs.value = []
        intentResult.value = null
        toolSelections.value = []
        toolSearchResults.value = []
        resetRuntimeMetrics()
      }
      return session
    } catch (err: any) {
      error.value = err.message || translate('assistant.errors.createSession')
      // 不抛出错误，避免页面崩溃
      return null
    } finally {
      loading.value = false
    }
  }

  /**
   * 获取会话详情
   */
  async function fetchSession(sessionId: string) {
    loading.value = true
    error.value = null
    try {
      const session = await getSession(sessionId)
      currentSession.value = session
      applySessionRuntimeMetadata(session)
      // 同步更新列表中的会话
      const index = sessions.value.findIndex(s => s.session_id === sessionId)
      if (index > -1) {
        sessions.value[index] = session
      }
      return session
    } catch (err: any) {
      error.value = err.message || translate('assistant.errors.fetchSession')
      throw err
    } finally {
      loading.value = false
    }
  }

  async function refreshSessionSnapshot(sessionId: string) {
    try {
      const session = await getSession(sessionId)
      if (currentSession.value?.session_id === sessionId) {
        currentSession.value = session
        applySessionRuntimeMetadata(session)
      }
      const index = sessions.value.findIndex(s => s.session_id === sessionId)
      if (index > -1) {
        sessions.value[index] = session
      }
      return session
    } catch {
      return null
    }
  }

  /**
   * 获取会话消息列表
   */
  async function fetchMessages(sessionId: string) {
    loading.value = true
    error.value = null
    try {
      const result = await getMessages(sessionId)
      const normalized = normalizeMessages(result)
      messages.value = normalized
      return messages.value
    } catch (err: any) {
      error.value = err.message || translate('assistant.errors.fetchMessages')
      throw err
    } finally {
      loading.value = false
    }
  }

  /**
   * 发送消息并启动 SSE 流式连接
   * 如果没有当前会话，会先创建一个新会话
   */
  async function sendMessage(content: string, contextRefsData?: Array<{ object_type: string; object_id: string }>) {
    error.value = null
    try {
      // 如果没有当前会话，先创建一个
      let session = currentSession.value
      if (!session) {
        const createData: CreateSessionRequest = {
          title: inferSessionTitle(content),
          task_type: pendingTaskType.value || 'explanation',
          context_refs: contextRefsData,
          locale: getCurrentLocale(),
        }
        session = await createSession(createData)
        if (!session) {
          throw new Error(translate('assistant.errors.createSession'))
        }
        pendingTaskType.value = null
      }

      const sessionId = session.session_id

      // 先添加用户消息到本地列表
      const tempId = `temp-${Date.now()}`
      const userMessage: AssistantMessage = {
        id: tempId,
        session_id: sessionId,
        message_id: tempId,
        role: 'user',
        content,
        created_at: new Date().toISOString(),
      }
      messages.value.push(userMessage)

      // 发送消息到后端
      const data: SendMessageRequest = { content, locale: getCurrentLocale() }
      if (contextRefsData?.length) {
        data.context_refs = contextRefsData
      }
      const handle = await apiSendMessage(sessionId, data)
      if (handle?.message_id) {
        userMessage.id = handle.message_id
        userMessage.message_id = handle.message_id
      }
      if (currentSession.value) {
        currentSession.value.status = 'running'
      }
      const sessionIdx = sessions.value.findIndex(s => s.session_id === sessionId)
      if (sessionIdx > -1) {
        sessions.value[sessionIdx].status = 'running'
        sessions.value[sessionIdx].message_count = (sessions.value[sessionIdx].message_count || 0) + 1
      }

      // 启动 SSE 流式接收助手回复
      startStream(sessionId)
    } catch (err: any) {
      messages.value = messages.value.filter(message => !String(message.message_id).startsWith('temp-'))
      error.value = err.message || translate('assistant.errors.sendMessage')
      throw err
    }
  }

  /**
   * 设置待创建会话的任务类型
   */
  function setPendingTaskType(taskType: AssistantTaskType | null) {
    pendingTaskType.value = taskType
    // 清空当前会话，进入"新会话"模式
    if (taskType) {
      currentSession.value = null
      messages.value = []
      toolCalls.value = []
      approvals.value = []
      resultCards.value = []
      contextRefs.value = []
      intentResult.value = null
      toolSelections.value = []
      toolSearchResults.value = []
      resetRuntimeMetrics()
    }
  }

  /**
   * 取消运行
   */
  async function cancelRun(sessionId: string) {
    error.value = null
    try {
      await apiCancelRun(sessionId)
      // 更新当前会话状态
      if (currentSession.value?.session_id === sessionId) {
        currentSession.value.status = 'cancelled'
      }
      stopStream()
    } catch (err: any) {
      error.value = err.message || translate('assistant.errors.cancelRun')
      throw err
    }
  }

  /**
   * 获取上下文引用
   */
  async function fetchContextRefs(sessionId: string) {
    error.value = null
    try {
      const result = await apiGetContextRefs(sessionId)
      contextRefs.value = result
      return result
    } catch (err: any) {
      error.value = err.message || translate('assistant.errors.fetchContextRefs')
      throw err
    }
  }

  async function uploadSessionFile(file: File, purpose: AssistantFileUploadPurpose = 'analysis'): Promise<AssistantFileUploadResult> {
    if (!currentSession.value?.session_id) {
      throw new Error(translate('assistant.errors.sessionRequired'))
    }
    const result = await apiUploadAssistantFile(currentSession.value.session_id, file, purpose)
    if (result?.context_ref) {
      const index = contextRefs.value.findIndex(ref => ref.id === result.context_ref.id)
      if (index >= 0) {
        contextRefs.value[index] = result.context_ref
      } else {
        contextRefs.value.push(result.context_ref)
      }
    }
    return result
  }

  /**
   * 获取工具调用列表
   */
  async function fetchToolCalls(sessionId: string, params?: ToolCallsQueryParams) {
    error.value = null
    try {
      const pageSize = params?.page_size || 100
      const query: ToolCallsQueryParams = { ...params, page_size: pageSize }
      if (params?.page) {
        const result = await apiGetToolCalls(sessionId, query)
        toolCalls.value = result.items.map(normalizeToolCallBusinessOutcome)
        return result
      }

      const firstPage = await apiGetToolCalls(sessionId, { ...query, page: 1 })
      const pageItems = [...(firstPage.items || [])]
      const effectivePageSize = firstPage.page_size || pageSize
      const total = firstPage.total || pageItems.length
      const totalPages = Math.ceil(total / effectivePageSize)

      if (totalPages > 1) {
        const restPages = await Promise.all(
          Array.from({ length: totalPages - 1 }, (_, index) =>
            apiGetToolCalls(sessionId, { ...query, page: index + 2, page_size: effectivePageSize })
          )
        )
        for (const pageResult of restPages) {
          pageItems.push(...(pageResult.items || []))
        }
      }

      const result = {
        ...firstPage,
        items: pageItems,
        total,
        page: 1,
        page_size: effectivePageSize,
      }
      toolCalls.value = result.items.map(normalizeToolCallBusinessOutcome)
      // 不在此处调用 rebuildAssistantHistoryCycles()
      // 由调用方（openSession）在所有数据加载完成后统一重建，避免竞态条件
      return result
    } catch (err: any) {
      error.value = err.message || translate('assistant.errors.fetchToolCalls')
      throw err
    }
  }

  /**
   * 获取审批列表
   */
  async function fetchApprovals(sessionId: string, params?: ApprovalsQueryParams) {
    error.value = null
    try {
      const result = await apiGetApprovals(sessionId, params)
      approvals.value = result.items
      return result
    } catch (err: any) {
      error.value = err.message || translate('assistant.errors.fetchApprovals')
      throw err
    }
  }

  /**
   * 通过审批
   */
  async function approveApproval(approvalId: string, comment?: string) {
    error.value = null
    try {
      const result = await apiApproveApproval(approvalId, comment ? { comment } : undefined)
      const approval = (result as any).approval || result
      upsertApproval(approval)
      updateMessageApproval(approval)
      return result
    } catch (err: any) {
      error.value = err.message || translate('assistant.errors.approval')
      throw err
    }
  }

  /**
   * 拒绝审批
   */
  async function rejectApproval(approvalId: string, comment?: string) {
    error.value = null
    try {
      const result = await apiRejectApproval(approvalId, comment ? { comment } : undefined)
      upsertApproval(result)
      updateMessageApproval(result)
      return result
    } catch (err: any) {
      error.value = err.message || translate('assistant.errors.approval')
      throw err
    }
  }

  /**
   * 启动 SSE 流式连接
   */
  function startStream(sessionId: string) {
    stopStream()
    streaming.value = true

    // 重置循环分割状态
    lastEventType = null
    currentCycleMessageId = null
    currentCycleIndex = 0

    eventSource = createAssistantStream(sessionId)

    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        applyStreamEvent(data)
      } catch (err) {
        console.error('解析 SSE 事件失败:', err)
      }
    }

    eventSource.onerror = (event) => {
      console.error('SSE 连接错误:', event)
      streaming.value = false
      eventSource?.close()
      eventSource = null
    }
    // done 事件已在 applyStreamEvent 的 'done' case 中处理
  }

  /**
   * 停止 SSE 流式连接
   */
  function stopStream() {
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }
    streaming.value = false
  }

  function findMessage(messageId: string) {
    return messages.value.find(m => m.id === messageId || m.message_id === messageId)
  }

  function ensureAssistantMessage(messageId: string): AssistantMessage | null {
    if (!messageId) return null
    const existing = findMessage(messageId)
    if (existing) return existing

    const message: AssistantMessage = {
      id: messageId,
      session_id: currentSession.value?.session_id || '',
      message_id: messageId,
      role: 'assistant',
      content: '',
      created_at: new Date().toISOString(),
    }
    messages.value.push(message)
    return message
  }

  function resolveAssistantMessageId(event: { run_id?: string; message_id?: string }, payload: Record<string, any>) {
    return event.message_id || payload.message_id || (event.run_id ? `msg_${event.run_id}` : '')
  }

  function upsertApproval(approval: AssistantApproval) {
    const index = approvals.value.findIndex(a =>
      a.approval_id === approval.approval_id || a.id === approval.id
    )
    if (index > -1) {
      approvals.value[index] = approval
    } else {
      approvals.value.push(approval)
    }
  }

  function updateMessageApproval(approval: AssistantApproval) {
    for (const msg of messages.value) {
      const index = msg.approvals?.findIndex(a =>
        a.approval_id === approval.approval_id || a.id === approval.id
      ) ?? -1
      if (index > -1 && msg.approvals) {
        msg.approvals[index] = approval
      }
    }
  }

  function nextCycleMessageId(event: { run_id?: string; message_id?: string }, payload: Record<string, any>) {
    currentCycleIndex += 1
    const base = resolveAssistantMessageId(event, payload) || 'msg'
    return `${base}_cycle_${currentCycleIndex}`
  }

  /**
   * 检查消息是否有内容或工具调用（用于循环边界检测）
   */
  function hasContentOrToolCalls(messageId: string): boolean {
    const msg = findMessage(messageId)
    if (!msg) return false
    return Boolean(msg.content || msg.tool_calls?.length)
  }

  function hasFinishedToolCalls(messageId: string): boolean {
    const msg = findMessage(messageId)
    return Boolean(msg?.tool_calls?.some(tc =>
      tc.status === 'completed' ||
      tc.status === 'success' ||
      tc.status === 'failed' ||
      tc.status === 'cancelled' ||
      tc.status === 'rejected'
    ))
  }

  /**
   * 检查是否需要创建新的循环消息
   * 循环边界条件：
   * 1. 上一个事件是 message_delta（内容已写入）
   * 2. 上一个事件是 tool_result（工具调用已完成）
   * 3. 当前消息已有内容或工具调用
   */
  function shouldCreateNewCycle(): boolean {
    if (lastEventType === 'message_delta' || lastEventType === 'tool_result' || lastEventType === 'tool_error') {
      return true
    }
    if (currentCycleMessageId && hasContentOrToolCalls(currentCycleMessageId)) {
      return true
    }
    return false
  }

  function isContinuationOfCurrentContent(delta: string): boolean {
    if (!currentCycleMessageId) return false
    const msg = findMessage(currentCycleMessageId)
    if (!msg?.content) return false
    return delta.startsWith(msg.content) || msg.content.endsWith(delta)
  }

  function shouldCreateResultCycle(incomingMessageId: string | undefined, delta: string): boolean {
    if (!currentCycleMessageId) return false
    if (isContinuationOfCurrentContent(delta)) return false
    if (lastEventType === 'tool_result' || lastEventType === 'tool_error') {
      return true
    }
    if (hasFinishedToolCalls(currentCycleMessageId)) {
      return true
    }
    if (
      lastEventType === 'message_delta' &&
      incomingMessageId &&
      incomingMessageId !== currentCycleMessageId
    ) {
      return true
    }
    return false
  }

  function appendThinkingStep(event: { run_id?: string; message_id?: string }, payload: Record<string, any>) {
    const content = String(payload.content || payload.message || '').trim()
    if (!content) return
    if (isHiddenInternalThinkingStep(content)) return

    // 循环边界检测：如果当前消息已有内容或工具调用，创建新消息
    if (shouldCreateNewCycle()) {
      // 创建新消息用于新循环
      currentCycleMessageId = nextCycleMessageId(event, payload)
    }

    const messageId = currentCycleMessageId || resolveAssistantMessageId(event, payload)
    currentCycleMessageId = messageId
    const message = ensureAssistantMessage(messageId)
    if (!message) return

    // 兼容旧格式（字符串）和新格式（数组）
    let steps: string[] = []
    if (Array.isArray(message.thinking)) {
      steps = message.thinking
    } else if (typeof message.thinking === 'string' && message.thinking) {
      steps = message.thinking.split('\n').map(s => s.trim()).filter(Boolean)
    }

    // 去重：如果最后一个步骤和新内容相同，则跳过
    if (steps[steps.length - 1] === content) return
    steps.push(content)
    message.thinking = steps

    // 更新事件类型
    lastEventType = 'thinking'
  }

  function summarizeToolResult(result: any) {
    if (typeof result === 'string') return result
    if (result === undefined || result === null) return ''
    try {
      return JSON.stringify(result).slice(0, 500)
    } catch {
      return String(result).slice(0, 500)
    }
  }

  function normalizeAssistantPlan(plan: Partial<AssistantPlan> & Record<string, any>): NonNullable<AssistantMessage['plan']> {
    const steps = Array.isArray(plan.steps) ? plan.steps : []
    return {
      goal: plan.goal || '',
      status: plan.status || 'running',
      steps: steps.map((step: Record<string, any>, index: number) => ({
        step_id: step.step_id || step.id || `step-${index + 1}`,
        title: step.title || step.description || step.objective || translate('assistant.progress.stepTitle', { number: index + 1 }),
        status: step.status || 'pending',
        result_summary: step.result_summary,
      })),
    }
  }

  // 更新计划状态为已完成
  function updatePlanStatusToCompleted() {
    // 遍历所有消息，找到最新的计划并更新状态
    for (let i = messages.value.length - 1; i >= 0; i--) {
      const msg = messages.value[i]
      if (msg.role === 'assistant' && msg.plan) {
        msg.plan.status = 'completed'
        // 将所有运行中的步骤也标记为完成
        for (const step of msg.plan.steps) {
          if (step.status === 'running' || step.status === 'pending') {
            step.status = 'completed'
          }
        }
        break
      }
    }
  }

  // 更新计划状态为失败
  function updatePlanStatusToFailed() {
    // 遍历所有消息，找到最新的计划并更新状态
    for (let i = messages.value.length - 1; i >= 0; i--) {
      const msg = messages.value[i]
      if (msg.role === 'assistant' && msg.plan) {
        msg.plan.status = 'failed'
        // 将运行中的步骤标记为失败
        for (const step of msg.plan.steps) {
          if (step.status === 'running') {
            step.status = 'failed'
          }
        }
        break
      }
    }
  }

  function updatePlanStatusFromOutcome(outcome?: string) {
    if (!outcome || outcome === 'succeeded') {
      updatePlanStatusToCompleted()
      return
    }
    for (let i = messages.value.length - 1; i >= 0; i--) {
      const msg = messages.value[i]
      if (msg.role !== 'assistant' || !msg.plan) continue
      msg.plan.status = outcome === 'partially_succeeded' ? 'partial' : 'failed'
      for (const step of msg.plan.steps) {
        if (step.status === 'running' || step.status === 'retrying') {
          step.status = 'failed'
        } else if (step.status === 'pending') {
          step.status = 'skipped'
        }
      }
      break
    }
  }

  function sessionStatusFromGoalOutcome(outcome?: string): AssistantSession['status'] {
    switch (outcome) {
      case 'failed':
        return 'failed'
      case 'needs_input':
        return 'active'
      default:
        return 'completed'
    }
  }

  /**
   * 处理 SSE 流式事件并更新状态
   *
   * 后端 AssistantEvent 结构:
   * { type, session_id, run_id, message_id, payload, error, timestamp }
   *
   * 支持的事件类型:
   * - thinking: 思考中
   * - message_delta: 助手消息增量内容（payload.delta）
   * - intent_detected: 意图识别
   * - tools_selected: 工具选择
   * - tool_call: 工具调用开始
   * - tool_result: 工具调用完成
   * - tool_error: 工具调用错误
   * - approval_required: 等待审批
   * - approval_updated: 审批已处理
   * - context_ref_added: 新增上下文引用
   * - result_card: 结果卡片
   * - done: 运行完成
   * - error: 错误事件
   */
  function applyStreamEvent(event: { type: string; payload?: any; error?: string; session_id?: string; run_id?: string; message_id?: string }) {
    const payload = event.payload || {}
    if (event.session_id && currentSession.value?.session_id && event.session_id !== currentSession.value.session_id) {
      return
    }

    switch (event.type) {
      case 'thinking':
        // 思考步骤 — 聚合到当前助手消息，供对话区折叠展示
        appendThinkingStep(event, payload)
        break

      case 'intent_detected': {
        // 意图识别结果
        // payload: AssistantIntentResult
        intentResult.value = payload as AssistantIntentResult
        break
      }

      case 'tools_selected': {
        // 工具选择结果
        // payload: AssistantToolSelection
        const selection = payload as AssistantToolSelection
        updateCurrentSessionMetadata({
          runtime_profile: payload.runtime_profile,
          max_total_turns: payload.max_total_turns,
          current_run_id: event.run_id,
        })
        const existingIdx = toolSelections.value.findIndex(s => s.run_id === selection.run_id)
        if (existingIdx > -1) {
          toolSelections.value[existingIdx] = selection
        } else {
          toolSelections.value.push(selection)
        }
        break
      }

      case 'tool_search': {
        // 工具搜索结果
        // payload: AssistantToolSearchResult
        toolSearchResults.value.push(payload as AssistantToolSearchResult)
        break
      }

      case 'tool_expansion': {
        // 工具扩展 — 更新已有工具选择
        // payload: AssistantToolSelection (expanded stage)
        const expanded = payload as AssistantToolSelection
        const expandIdx = toolSelections.value.findIndex(s => s.run_id === expanded.run_id)
        if (expandIdx > -1) {
          toolSelections.value[expandIdx] = expanded
        } else {
          toolSelections.value.push(expanded)
        }
        break
      }

      case 'plan': {
        // 执行计划 — 嵌入到当前助手消息的 plan 字段
        // payload: AssistantPlan
        const planMsgId = event.message_id || payload.message_id || (event.run_id ? `msg_${event.run_id}` : '')
        const planMsg = ensureAssistantMessage(planMsgId)
        if (planMsg) {
          planMsg.plan = normalizeAssistantPlan(payload as AssistantPlan)
        }
        break
      }

      case 'step_started': {
        // 步骤开始 — 更新对应 plan step 状态
        // payload: { step_id, title? }
        const stepStartedId = event.message_id || payload.message_id || (event.run_id ? `msg_${event.run_id}` : '')
        if (stepStartedId) {
          const stepMsg = findMessage(stepStartedId)
          if (stepMsg?.plan?.steps) {
            const step = stepMsg.plan.steps.find(s => s.step_id === payload.step_id)
            if (step) {
              step.status = 'running'
            }
          }
        }
        break
      }

      case 'step_completed': {
        // 步骤完成 — 更新对应 plan step 状态和结果
        // payload: { step_id, result_summary?, status? }
        const stepDoneId = event.message_id || payload.message_id || (event.run_id ? `msg_${event.run_id}` : '')
        if (stepDoneId) {
          const stepDoneMsg = findMessage(stepDoneId)
          if (stepDoneMsg?.plan?.steps) {
            const stepDone = stepDoneMsg.plan.steps.find(s => s.step_id === payload.step_id)
            if (stepDone) {
              stepDone.status = payload.status || 'completed'
              const resultSummary = payload.result_summary || payload.summary
              if (resultSummary) {
                stepDone.result_summary = resultSummary
              }
            }
          }
        }
        break
      }

      case 'step_failed':
      case 'step_retrying': {
        const stepEventId = event.message_id || payload.message_id || (event.run_id ? `msg_${event.run_id}` : '')
        if (stepEventId) {
          const stepEventMsg = findMessage(stepEventId)
          const step = stepEventMsg?.plan?.steps?.find(s => s.step_id === payload.step_id)
          if (step) {
            step.status = event.type === 'step_retrying' ? 'retrying' : 'failed'
            if (payload.error) {
              step.result_summary = payload.error
            }
          }
        }
        break
      }

      case 'message_delta': {
        // 增量更新助手消息内容
        // payload: { message_id, delta }
        const { message_id, delta } = payload
        if (!delta) break

        if (shouldCreateResultCycle(message_id, delta)) {
          currentCycleMessageId = message_id && !findMessage(message_id)
            ? message_id
            : nextCycleMessageId(event, payload)
        }

        // 使用当前循环的消息ID，如果没有则使用事件中的message_id
        const targetId = currentCycleMessageId || message_id || resolveAssistantMessageId(event, payload)
        const existing = messages.value.find(m => m.id === targetId || m.message_id === targetId)
        if (existing) {
          if (existing.content === delta) break
          existing.content = delta.startsWith(existing.content) ? delta : existing.content + delta
          currentCycleMessageId = targetId
        } else {
          // 创建新消息
          messages.value.push({
            id: targetId,
            session_id: currentSession.value?.session_id || '',
            message_id: targetId,
            role: 'assistant',
            content: delta,
            created_at: new Date().toISOString(),
          })
          // 更新当前循环消息ID
          currentCycleMessageId = targetId
        }

        // 更新事件类型
        lastEventType = 'message_delta'
        break
      }

      case 'tool_call': {
        // 工具调用开始
        // payload: { call_id, tool_name, args }
        // 仅处理属于当前会话的工具调用
        if (event.session_id && event.session_id !== currentSession.value?.session_id) break

        // 使用当前循环的消息ID
        const toolCallMsgId = currentCycleMessageId || resolveAssistantMessageId(event, payload)
        currentCycleMessageId = toolCallMsgId

        const toolCall: AssistantToolCall = {
          id: payload.call_id,
          session_id: event.session_id || currentSession.value?.session_id || '',
          message_id: toolCallMsgId,
          call_id: payload.call_id,
          tool_name: payload.tool_name,
          args: payload.args || {},
          status: 'running',
          risk_level: 'readonly',
          created_at: new Date().toISOString(),
        }

        // 更新全局工具调用列表
        const existingIdx = toolCalls.value.findIndex(tc => tc.call_id === toolCall.call_id || tc.id === toolCall.call_id)
        if (existingIdx > -1) {
          toolCalls.value[existingIdx] = {
            ...toolCalls.value[existingIdx],
            ...toolCall,
            status: toolCalls.value[existingIdx].status === 'running' ? 'running' : toolCalls.value[existingIdx].status,
          }
        } else {
          toolCalls.value.push(toolCall)
        }

        // 关联工具调用到当前循环的消息
        const toolCallMessage = ensureAssistantMessage(toolCallMsgId)
        if (toolCallMessage) {
          if (!toolCallMessage.tool_calls) {
            toolCallMessage.tool_calls = []
          }
          // 检查是否已存在
          const existingInMsg = toolCallMessage.tool_calls.findIndex(tc => tc.call_id === payload.call_id)
          if (existingInMsg > -1) {
            toolCallMessage.tool_calls[existingInMsg] = {
              ...toolCallMessage.tool_calls[existingInMsg],
              ...toolCall,
            }
          } else {
            toolCallMessage.tool_calls.push(toolCall)
          }
        }

        // 更新事件类型
        lastEventType = 'tool_call'
        break
      }

      case 'tool_result': {
        // The tool transport returned. Business completion is represented by
        // operation_status + terminal and may still be accepted/running/failed.
        // payload: { call_id, result, operation_status?, terminal?, outcome? }
        // 仅处理属于当前会话的工具调用
        if (event.session_id && event.session_id !== currentSession.value?.session_id) break

        // 更新全局工具调用列表
        const idx = toolCalls.value.findIndex(tc => tc.call_id === payload.call_id || tc.id === payload.call_id)
        const operationStatus = payload.operation_status as AssistantToolCall['operation_status']
        const displayStatus = displayStatusForToolOutcome(operationStatus, payload.terminal, 'completed')
        const resultSummary = summarizeOperationOutcome(payload)
        if (idx > -1) {
          toolCalls.value[idx].status = displayStatus
          toolCalls.value[idx].result = payload.result
          toolCalls.value[idx].result_summary = resultSummary
          toolCalls.value[idx].operation_status = operationStatus
          toolCalls.value[idx].terminal = payload.terminal
          toolCalls.value[idx].outcome = payload.outcome
          toolCalls.value[idx].outcome_message = payload.outcome_message
          toolCalls.value[idx].capability = payload.capability
          if (displayStatus === 'failed' && !toolCalls.value[idx].error_message) {
            toolCalls.value[idx].error_message = resultSummary
          }
        } else if (payload.call_id) {
          toolCalls.value.push({
            id: payload.call_id,
            session_id: event.session_id || currentSession.value?.session_id || '',
            message_id: resolveAssistantMessageId(event, payload),
            call_id: payload.call_id,
            tool_name: payload.tool_name || 'Unknown.Tool',
            args: payload.args || {},
            status: displayStatus,
            risk_level: 'readonly',
            result: payload.result,
            result_summary: resultSummary,
            operation_status: operationStatus,
            terminal: payload.terminal,
            outcome: payload.outcome,
            outcome_message: payload.outcome_message,
            capability: payload.capability,
            error_message: displayStatus === 'failed' ? resultSummary : undefined,
            created_at: new Date().toISOString(),
          })
        }

        // 更新消息中的工具调用状态
        if (currentCycleMessageId) {
          const toolResultMsg = findMessage(currentCycleMessageId)
          if (toolResultMsg?.tool_calls) {
            const tcInMsg = toolResultMsg.tool_calls.find(tc => tc.call_id === payload.call_id)
            if (tcInMsg) {
              tcInMsg.status = displayStatus
              tcInMsg.result = payload.result
              tcInMsg.result_summary = resultSummary
              tcInMsg.operation_status = operationStatus
              tcInMsg.terminal = payload.terminal
              tcInMsg.outcome = payload.outcome
              tcInMsg.outcome_message = payload.outcome_message
              tcInMsg.capability = payload.capability
              if (displayStatus === 'failed' && !tcInMsg.error_message) {
                tcInMsg.error_message = resultSummary
              }
            }
          }
        }

        // 更新事件类型
        lastEventType = 'tool_result'
        break
      }

      case 'tool_error': {
        // 工具调用错误
        // payload: { call_id, error }
        // 仅处理属于当前会话的工具调用
        if (event.session_id && event.session_id !== currentSession.value?.session_id) break
        const errIdx = toolCalls.value.findIndex(tc => tc.call_id === payload.call_id || tc.id === payload.call_id)
        if (errIdx > -1) {
          toolCalls.value[errIdx].status = 'failed'
          toolCalls.value[errIdx].error_message = payload.error
        } else if (payload.call_id) {
          toolCalls.value.push({
            id: payload.call_id,
            session_id: event.session_id || currentSession.value?.session_id || '',
            message_id: resolveAssistantMessageId(event, payload),
            call_id: payload.call_id,
            tool_name: payload.tool_name || 'Unknown.Tool',
            args: payload.args || {},
            status: 'failed',
            risk_level: 'readonly',
            error_message: payload.error,
            created_at: new Date().toISOString(),
          })
        }

        // 更新消息中的工具调用状态
        if (currentCycleMessageId) {
          const toolErrorMsg = findMessage(currentCycleMessageId)
          if (toolErrorMsg?.tool_calls) {
            const tcInMsg = toolErrorMsg.tool_calls.find(tc => tc.call_id === payload.call_id)
            if (tcInMsg) {
              tcInMsg.status = 'failed'
              tcInMsg.error_message = payload.error
            }
          }
        }

        // 更新事件类型
        lastEventType = 'tool_error'
        break
      }

      case 'approval_required': {
        // 新增待审批项
        const approval = payload as AssistantApproval
        upsertApproval(approval)
        const approvalMsgId = resolveAssistantMessageId(event, payload) || currentCycleMessageId
        const approvalMessage = approvalMsgId ? ensureAssistantMessage(approvalMsgId) : null
        if (approvalMessage) {
          if (!approvalMessage.approvals) {
            approvalMessage.approvals = []
          }
          const existingApprovalIdx = approvalMessage.approvals.findIndex(a =>
            a.approval_id === approval.approval_id || a.id === approval.id
          )
          if (existingApprovalIdx > -1) {
            approvalMessage.approvals[existingApprovalIdx] = approval
          } else {
            approvalMessage.approvals.push(approval)
          }
          if (approval.tool_call_id && approvalMessage.tool_calls) {
            const call = approvalMessage.tool_calls.find(tc =>
              tc.call_id === approval.tool_call_id || tc.id === approval.tool_call_id
            )
            if (call) {
              call.status = 'approval_required'
            }
          }
        }
        if (approval.tool_call_id) {
          const toolIdx = toolCalls.value.findIndex(tc =>
            tc.call_id === approval.tool_call_id || tc.id === approval.tool_call_id
          )
          if (toolIdx > -1) {
            toolCalls.value[toolIdx].status = 'approval_required'
          }
        }
        if (currentSession.value) {
          currentSession.value.status = 'waiting_approval'
        }
        lastEventType = 'approval_required'
        break
      }

      case 'approval_updated': {
        // 审批已处理，更新状态
        const resolved = payload as AssistantApproval
        upsertApproval(resolved)
        for (const msg of messages.value) {
          const approvalIdx = msg.approvals?.findIndex(a =>
            a.approval_id === resolved.approval_id || a.id === resolved.id
          ) ?? -1
          if (approvalIdx > -1 && msg.approvals) {
            msg.approvals[approvalIdx] = resolved
          }
        }
        break
      }

      case 'run_waiting_approval': {
        if (currentSession.value) {
          currentSession.value.status = 'waiting_approval'
          updateCurrentSessionMetadata({
            current_run_status: 'waiting_approval',
            waiting_approval_id: payload.approval_id,
          })
        }
        break
      }

      case 'context_ref_added': {
        // 新增上下文引用
        const ref = payload as AssistantContextRef
        contextRefs.value.push(ref)
        break
      }

      case 'result_card': {
        // 结果卡片 — 后端可能用 card_type/data 或 type/payload，统一映射
        const rawCard = payload as Record<string, any>
        const card: AssistantResultCard = {
          id: rawCard.id || `card-${Date.now()}`,
          type: rawCard.type || rawCard.card_type,
          title: rawCard.title || '',
          payload: rawCard.payload || rawCard.data || {},
          created_at: rawCard.created_at || new Date().toISOString(),
        }
        resultCards.value.push(card)
        break
      }

      case 'context_budget': {
        const budget = payload as ContextBudgetEvent
        contextBudget.value = budget
        const observedPromptTokens = Number(budget.prompt_tokens_observed || budget.estimated_prompt_tokens || 0)
        const totalTokens = Number(budget.total_tokens || 0)
        const completionTokens = Number(budget.completion_tokens || 0)
        const cumulativePromptTokens = totalTokens > completionTokens ? totalTokens - completionTokens : 0
        totalPromptTokens.value = Math.max(totalPromptTokens.value || 0, observedPromptTokens, cumulativePromptTokens)
        totalCompletionTokens.value = Math.max(totalCompletionTokens.value || 0, completionTokens)
        updateCurrentSessionMetadata({
          context_budget: budget,
          total_prompt_tokens: totalPromptTokens.value,
          total_completion_tokens: totalCompletionTokens.value,
          total_tokens: Number(budget.total_tokens || totalPromptTokens.value + totalCompletionTokens.value),
          compression_count: budget.compression_count,
        })
        break
      }

      case 'context_compressed': {
        const record = payload as ContextCompressedEvent & { compression_id?: string }
        const exists = record.compression_id
          ? compressionRecords.value.some(item => (item as any).compression_id === record.compression_id)
          : false
        if (!exists) {
          compressionRecords.value.push(record)
        }
        updateCurrentSessionMetadata({
          compression_records: compressionRecords.value,
          compression_count: compressionRecords.value.length,
        })
        break
      }

      case 'context_compression_failed': {
        error.value = event.error || payload.message || translate('assistant.errors.compression')
        break
      }

      case 'done': {
        // 运行完成
        streaming.value = false
        eventSource?.close()
        eventSource = null
        // A done event closes the stream, but the business goal may have
        // failed or may still need user input.
        if (currentSession.value) {
          currentSession.value.status = sessionStatusFromGoalOutcome(payload.goal_outcome)
          updateCurrentSessionMetadata({
            current_run_status: payload.status || 'completed',
            goal_outcome: payload.goal_outcome,
          })
          refreshSessionSnapshot(currentSession.value.session_id)
        }
        updatePlanStatusFromOutcome(payload.goal_outcome)
        break
      }

      case 'error': {
        // 错误事件
        error.value = event.error || payload.message || translate('assistant.errors.run')
        streaming.value = false
        eventSource?.close()
        eventSource = null
        if (currentSession.value) {
          refreshSessionSnapshot(currentSession.value.session_id)
        }
        // 更新计划状态为失败
        updatePlanStatusToFailed()
        break
      }

      default:
        console.warn('未知的 SSE 事件类型:', event.type)
    }
  }

  /**
   * 打开会话（加载会话详情、消息、上下文、工具调用、审批）
   */
  async function openSession(sessionId: string) {
    loading.value = true
    error.value = null
    try {
      const session = await fetchSession(sessionId)
      await Promise.all([
        fetchMessages(sessionId),
        fetchContextRefs(sessionId),
        fetchToolCalls(sessionId),
        fetchApprovals(sessionId),
      ])
      rebuildAssistantHistoryCycles()
      if (session?.status === 'running' || session?.status === 'waiting_approval') {
        startStream(sessionId)
        restoreRunningAssistantTimeline()
      } else {
        stopStream()
      }
    } catch (err: any) {
      error.value = err.message || translate('assistant.errors.openSession')
      throw err
    } finally {
      loading.value = false
    }
  }

  /**
   * 取消当前运行
   */
  async function cancelCurrentRun() {
    if (!currentSession.value) return
    await cancelRun(currentSession.value.session_id)
  }

  /**
   * 通过审批（别名）
   */
  async function approveAction(approvalId: string, comment?: string) {
    return approveApproval(approvalId, comment)
  }

  /**
   * 拒绝审批（别名）
   */
  async function rejectAction(approvalId: string, comment?: string) {
    return rejectApproval(approvalId, comment)
  }

  /**
   * 重置状态
   */
  function reset() {
    stopStream()
    sessions.value = []
    currentSession.value = null
    messages.value = []
    contextRefs.value = []
    toolCalls.value = []
    approvals.value = []
    resultCards.value = []
    intentResult.value = null
    toolSelections.value = []
    toolSearchResults.value = []
    resetRuntimeMetrics()
    streaming.value = false
    loading.value = false
    error.value = null
    sessionPage.value = 1
    sessionTotal.value = 0
    hasMoreSessions.value = false
    loadingMore.value = false
    sessionQuery.value = {}
    lastEventType = null
    currentCycleMessageId = null
    currentCycleIndex = 0
  }

  return {
    // State
    sessions,
    currentSession,
    messages,
    contextRefs,
    toolCalls,
    approvals,
    resultCards,
    intentResult,
    toolSelections,
    toolSearchResults,
    contextBudget,
    compressionRecords,
    totalPromptTokens,
    totalCompletionTokens,
    streaming,
    loading,
    error,
    sessionTotal,
    hasMoreSessions,
    loadingMore,
    pendingTaskType,

    // Actions
    fetchSessions,
    createSession,
    fetchSession,
    fetchMessages,
    sendMessage,
    setPendingTaskType,
    cancelRun,
    cancelCurrentRun,
    fetchContextRefs,
    uploadSessionFile,
    fetchToolCalls,
    fetchApprovals,
    approveApproval,
    rejectApproval,
    approveAction,
    rejectAction,
    openSession,
    startStream,
    stopStream,
    applyStreamEvent,
    reset,
    rebuildAssistantHistoryCycles,
    goToSessionPage,
    deleteSession,
  }
})
