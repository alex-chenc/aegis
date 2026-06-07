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
    if (!trimmed) return '新会话'
    return Array.from(trimmed).slice(0, 18).join('')
  }

  function normalizeMessages(input: AssistantMessage[]) {
    const normalized: AssistantMessage[] = []
    for (const msg of input || []) {
      const prev = normalized[normalized.length - 1]
      if (
        prev &&
        prev.role === 'user' &&
        msg.role === 'user' &&
        prev.content === msg.content &&
        Math.abs(new Date(msg.created_at).getTime() - new Date(prev.created_at).getTime()) <= 10_000
      ) {
        continue
      }
      normalized.push(msg)
    }
    return normalized
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
      error.value = err.message || '获取会话列表失败'
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
      error.value = err.message || '获取会话列表失败'
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
      error.value = err.message || '删除会话失败'
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
      error.value = err.message || '创建会话失败'
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
      error.value = err.message || '获取会话详情失败'
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
      return normalized
    } catch (err: any) {
      error.value = err.message || '获取消息列表失败'
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
        }
        session = await createSession(createData)
        if (!session) {
          throw new Error('创建会话失败')
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
      const data: SendMessageRequest = { content }
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
      error.value = err.message || '发送消息失败'
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
      error.value = err.message || '取消运行失败'
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
      error.value = err.message || '获取上下文引用失败'
      throw err
    }
  }

  /**
   * 获取工具调用列表
   */
  async function fetchToolCalls(sessionId: string, params?: ToolCallsQueryParams) {
    error.value = null
    try {
      const result = await apiGetToolCalls(sessionId, params)
      toolCalls.value = result.items
      return result
    } catch (err: any) {
      error.value = err.message || '获取工具调用列表失败'
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
      error.value = err.message || '获取审批列表失败'
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
      // 更新本地审批列表（兼容 id 和 approval_id 两种查找方式）
      const index = approvals.value.findIndex(a => a.approval_id === approvalId || a.id === approvalId)
      if (index > -1) {
        approvals.value[index] = result
      }
      return result
    } catch (err: any) {
      error.value = err.message || '审批操作失败'
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
      // 更新本地审批列表（兼容 id 和 approval_id 两种查找方式）
      const index = approvals.value.findIndex(a => a.approval_id === approvalId || a.id === approvalId)
      if (index > -1) {
        approvals.value[index] = result
      }
      return result
    } catch (err: any) {
      error.value = err.message || '审批操作失败'
      throw err
    }
  }

  /**
   * 启动 SSE 流式连接
   */
  function startStream(sessionId: string) {
    stopStream()
    streaming.value = true

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

  function appendThinkingStep(event: { run_id?: string; message_id?: string }, payload: Record<string, any>) {
    const content = String(payload.content || payload.message || '').trim()
    if (!content) return
    const messageId = resolveAssistantMessageId(event, payload)
    const message = ensureAssistantMessage(messageId)
    if (!message) return

    const existingSteps = (message.thinking || '')
      .split('\n')
      .map(step => step.trim())
      .filter(Boolean)
    if (existingSteps[existingSteps.length - 1] === content) return
    message.thinking = [...existingSteps, content].join('\n')
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
        title: step.title || step.description || step.objective || `步骤 ${index + 1}`,
        status: step.status || 'pending',
        result_summary: step.result_summary,
      })),
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
              if (payload.result_summary) {
                stepDone.result_summary = payload.result_summary
              }
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
        const existing = messages.value.find(m => m.id === message_id || m.message_id === message_id)
        if (existing) {
          if (existing.content === delta) break
          existing.content = delta.startsWith(existing.content) ? delta : existing.content + delta
        } else {
          messages.value.push({
            id: message_id,
            session_id: currentSession.value?.session_id || '',
            message_id: message_id,
            role: 'assistant',
            content: delta,
            created_at: new Date().toISOString(),
          })
        }
        break
      }

      case 'tool_call': {
        // 工具调用开始
        // payload: { call_id, tool_name, args }
        // 仅处理属于当前会话的工具调用
        if (event.session_id && event.session_id !== currentSession.value?.session_id) break
        const toolCall: AssistantToolCall = {
          id: payload.call_id,
          session_id: event.session_id || currentSession.value?.session_id || '',
          message_id: resolveAssistantMessageId(event, payload),
          call_id: payload.call_id,
          tool_name: payload.tool_name,
          args: payload.args || {},
          status: 'running',
          risk_level: 'readonly',
          created_at: new Date().toISOString(),
        }
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
        break
      }

      case 'tool_result': {
        // 工具调用完成
        // payload: { call_id, result }
        // 仅处理属于当前会话的工具调用
        if (event.session_id && event.session_id !== currentSession.value?.session_id) break
        const idx = toolCalls.value.findIndex(tc => tc.call_id === payload.call_id || tc.id === payload.call_id)
        if (idx > -1) {
          toolCalls.value[idx].status = 'completed'
          toolCalls.value[idx].result = payload.result
          toolCalls.value[idx].result_summary = summarizeToolResult(payload.result)
        } else if (payload.call_id) {
          toolCalls.value.push({
            id: payload.call_id,
            session_id: event.session_id || currentSession.value?.session_id || '',
            message_id: resolveAssistantMessageId(event, payload),
            call_id: payload.call_id,
            tool_name: payload.tool_name || 'Unknown.Tool',
            args: payload.args || {},
            status: 'completed',
            risk_level: 'readonly',
            result: payload.result,
            result_summary: summarizeToolResult(payload.result),
            created_at: new Date().toISOString(),
          })
        }
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
        break
      }

      case 'approval_required': {
        // 新增待审批项
        const approval = payload as AssistantApproval
        approvals.value.push(approval)
        if (currentSession.value) {
          currentSession.value.status = 'waiting_approval'
        }
        break
      }

      case 'approval_updated': {
        // 审批已处理，更新状态
        const resolved = payload as AssistantApproval
        const approvalIdx = approvals.value.findIndex(a => a.id === resolved.id)
        if (approvalIdx > -1) {
          approvals.value[approvalIdx] = resolved
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
        totalPromptTokens.value = Number(budget.prompt_tokens_observed || budget.estimated_prompt_tokens || totalPromptTokens.value || 0)
        totalCompletionTokens.value = Number(budget.completion_tokens || totalCompletionTokens.value || 0)
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
        error.value = event.error || payload.message || '上下文压缩失败'
        break
      }

      case 'done': {
        // 运行完成
        streaming.value = false
        eventSource?.close()
        eventSource = null
        // 更新会话状态为已完成
        if (currentSession.value) {
          currentSession.value.status = 'completed'
          updateCurrentSessionMetadata({ current_run_status: 'completed' })
          refreshSessionSnapshot(currentSession.value.session_id)
        }
        break
      }

      case 'error': {
        // 错误事件
        error.value = event.error || payload.message || '助手运行出错'
        streaming.value = false
        eventSource?.close()
        eventSource = null
        if (currentSession.value) {
          refreshSessionSnapshot(currentSession.value.session_id)
        }
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
      if (session?.status === 'running') {
        startStream(sessionId)
      } else {
        stopStream()
      }
    } catch (err: any) {
      error.value = err.message || '打开会话失败'
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
    goToSessionPage,
    deleteSession,
  }
})
