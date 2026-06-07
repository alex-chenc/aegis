import { defineStore } from 'pinia'
import { ref } from 'vue'
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
  type CreateSessionRequest,
  type SendMessageRequest,
  type RunHandle,
  type SessionsQueryParams,
  type ToolCallsQueryParams,
  type ApprovalsQueryParams,
} from '@/api/assistant'

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
  /** 是否正在流式接收 */
  const streaming = ref(false)
  /** 全局加载状态 */
  const loading = ref(false)
  /** 错误信息 */
  const error = ref<string | null>(null)

  /** SSE 连接实例 */
  let eventSource: EventSource | null = null

  // ============================================
  // Actions
  // ============================================

  /**
   * 获取会话列表
   */
  async function fetchSessions(params?: SessionsQueryParams) {
    loading.value = true
    error.value = null
    try {
      const result = await getSessions(params)
      // API 返回 { sessions: [...], total: N }
      sessions.value = result?.sessions || result?.items || []
      return result
    } catch (err: any) {
      // 不抛出错误，避免页面崩溃
      error.value = err.message || '获取会话列表失败'
      sessions.value = []
      return { sessions: [], total: 0 }
    } finally {
      loading.value = false
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
        messages.value = []
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

  /**
   * 获取会话消息列表
   */
  async function fetchMessages(sessionId: string) {
    loading.value = true
    error.value = null
    try {
      const result = await getMessages(sessionId)
      messages.value = result
      return result
    } catch (err: any) {
      error.value = err.message || '获取消息列表失败'
      throw err
    } finally {
      loading.value = false
    }
  }

  /**
   * 发送消息并启动 SSE 流式连接
   */
  async function sendMessage(sessionId: string, content: string, contextRefsData?: Array<{ object_type: string; object_id: string }>) {
    error.value = null
    try {
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
      await apiSendMessage(sessionId, data)

      // 启动 SSE 流式接收助手回复
      startStream(sessionId)
    } catch (err: any) {
      error.value = err.message || '发送消息失败'
      throw err
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
  function applyStreamEvent(event: { type: string; payload?: any; error?: string; session_id?: string }) {
    const payload = event.payload || {}

    switch (event.type) {
      case 'thinking':
        // 思考状态（可选：显示 loading 指示器）
        break

      case 'message_delta': {
        // 增量更新助手消息内容
        // payload: { message_id, delta }
        const { message_id, delta } = payload
        if (!delta) break
        const existing = messages.value.find(m => m.id === message_id)
        if (existing) {
          existing.content += delta
        } else {
          messages.value.push({
            id: message_id,
            session_id: currentSession.value?.session_id || '',
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
        const toolCall: AssistantToolCall = {
          id: payload.call_id,
          session_id: currentSession.value?.session_id || '',
          message_id: '',
          call_id: payload.call_id,
          tool_name: payload.tool_name,
          args: payload.args || {},
          status: 'running',
          risk_level: 'readonly',
          created_at: new Date().toISOString(),
        }
        toolCalls.value.push(toolCall)
        break
      }

      case 'tool_result': {
        // 工具调用完成
        // payload: { call_id, result }
        const idx = toolCalls.value.findIndex(tc => tc.call_id === payload.call_id || tc.id === payload.call_id)
        if (idx > -1) {
          toolCalls.value[idx].status = 'completed'
          toolCalls.value[idx].result = payload.result
          toolCalls.value[idx].result_summary = typeof payload.result === 'string'
            ? payload.result
            : JSON.stringify(payload.result).slice(0, 500)
        }
        break
      }

      case 'tool_error': {
        // 工具调用错误
        // payload: { call_id, error }
        const errIdx = toolCalls.value.findIndex(tc => tc.call_id === payload.call_id || tc.id === payload.call_id)
        if (errIdx > -1) {
          toolCalls.value[errIdx].status = 'failed'
          toolCalls.value[errIdx].error_message = payload.error
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
        // 结果卡片
        const card = payload as AssistantResultCard
        resultCards.value.push(card)
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
        }
        break
      }

      case 'error': {
        // 错误事件
        error.value = event.error || payload.message || '助手运行出错'
        streaming.value = false
        eventSource?.close()
        eventSource = null
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
      await fetchSession(sessionId)
      await Promise.all([
        fetchMessages(sessionId),
        fetchContextRefs(sessionId),
        fetchToolCalls(sessionId),
        fetchApprovals(sessionId),
      ])
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
    streaming.value = false
    loading.value = false
    error.value = null
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
    streaming,
    loading,
    error,

    // Actions
    fetchSessions,
    createSession,
    fetchSession,
    fetchMessages,
    sendMessage,
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
  }
})
