<template>
  <div class="ai-analysis-page">
    <el-row :gutter="20">
      <!-- 左侧：告警列表选择 -->
      <el-col :span="8">
        <el-card class="alert-selection-card">
          <template #header>
            <div class="card-header">
              <span>选择要分析的告警</span>
              <el-button size="small" @click="loadAlerts">刷新</el-button>
            </div>
          </template>

          <el-form label-width="80px" class="filter-form">
            <el-form-item label="时间范围">
              <el-date-picker
                v-model="timeRange"
                type="datetimerange"
                range-separator="至"
                start-placeholder="开始时间"
                end-placeholder="结束时间"
                :default-time="[new Date(2000, 1, 1, 0, 0, 0), new Date(2000, 1, 1, 23, 59, 59)]"
                @change="handleTimeRangeChange"
              />
            </el-form-item>
            <el-form-item label="主机过滤">
              <el-select v-model="hostFilter" multiple placeholder="选择主机" clearable>
                <el-option v-for="host in hosts" :key="host" :label="host" :value="host" />
              </el-select>
            </el-form-item>
          </el-form>

          <el-table
            ref="alertTableRef"
            v-loading="alertLoading"
            :data="filteredAlerts"
            border
            stripe
            height="400"
            @selection-change="handleAlertSelection"
          >
            <el-table-column type="selection" width="40" />
            <el-table-column prop="hostname" label="主机" min-width="100" />
            <el-table-column prop="rule_title" label="规则" min-width="150" show-overflow-tooltip />
            <el-table-column prop="severity" label="级别" width="80" align="center">
              <template #default="{ row }">
                <el-tag :type="severityTagType(row.severity)" size="small">
                  {{ severityLabel(row.severity) }}
                </el-tag>
              </template>
            </el-table-column>
          </el-table>

          <div class="selection-info">
            已选择 {{ selectedAlertIds.length }} 个告警
            <el-button type="primary" :disabled="selectedAlertIds.length === 0" @click="startAnalysis">
              开始 AI 分析
            </el-button>
          </div>
        </el-card>
      </el-col>

      <!-- 右侧：AI 分析对话 -->
      <el-col :span="16">
        <el-card class="chat-card">
          <template #header>
            <div class="card-header">
              <span>AI 安全分析助手</span>
              <el-tag v-if="sessionId" type="success" size="small">
                会话ID: {{ sessionId }}
              </el-tag>
            </div>
          </template>

          <div v-if="!sessionId" class="no-session">
            <el-empty description="请先在左侧选择告警并点击「开始 AI 分析」">
              <template #image>
                <svg width="64" height="64" viewBox="0 0 24 24" fill="none">
                  <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z" fill="#409EFF"/>
                </svg>
              </template>
            </el-empty>
          </div>

          <div v-else class="chat-container">
            <!-- 消息列表 -->
            <div ref="messageListRef" class="message-list">
              <div
                v-for="(msg, index) in messages"
                :key="index"
                :class="['message', msg.role]"
              >
                <div class="message-avatar">
                  <el-icon v-if="msg.role === 'user'" :size="20"><User /></el-icon>
                  <el-icon v-else :size="20"><ChatDotRound /></el-icon>
                </div>
                <div class="message-content">
                  <!-- 用户消息 -->
                  <div v-if="msg.role === 'user'" class="user-content">
                    {{ msg.content }}
                  </div>

                  <!-- AI 消息 -->
                  <div v-else class="ai-content">
                    <!-- 错误信息 -->
                    <div v-if="msg.isError" class="error-block">
                      <pre>{{ msg.content }}</pre>
                    </div>
                    <!-- 思考过程 - 流式显示 -->
                    <div v-else-if="msg.thinking" class="thinking-block">
                      <div class="thinking-header">
                        <el-icon><Aim /></el-icon>
                        <span>AI 思考中</span>
                        <span class="thinking-cursor" v-if="isLoading"></span>
                      </div>
                      <div class="thinking-content" ref="thinkingContentRef">{{ msg.thinking }}</div>
                    </div>

                    <!-- 工具调用 -->
                    <div v-if="msg.toolCalls && msg.toolCalls.length > 0" class="tool-calls">
                      <div v-for="call in msg.toolCalls" :key="call.call_id" class="tool-call-item">
                        <div class="tool-call-header">
                          <el-icon><Tools /></el-icon>
                          <span>调用工具: {{ call.tool }}</span>
                          <el-icon class="is-loading" v-if="!call.result && !call.error"><Loading /></el-icon>
                          <el-icon color="#67c23a" v-else-if="call.result"><Check /></el-icon>
                          <el-icon color="#f56c6c" v-else-if="call.error"><CircleClose /></el-icon>
                        </div>
                        <div class="tool-call-result" v-if="call.result">
                          <pre>{{ typeof call.result === 'string' ? call.result : JSON.stringify(call.result, null, 2) }}</pre>
                        </div>
                        <div class="tool-call-error" v-if="call.error">
                          <pre>{{ call.error }}</pre>
                        </div>
                      </div>
                    </div>

                    <!-- 最终回复 -->
                    <div v-if="msg.content && !msg.thinking" class="final-content">
                      <pre>{{ msg.content }}</pre>
                    </div>
                  </div>
                </div>
              </div>

              <!-- 加载中指示器 -->
              <div v-if="isLoading && (!messages.length || !messages[messages.length - 1]?.thinking)" class="message assistant loading">
                <div class="message-avatar">
                  <el-icon :size="20"><ChatDotRound /></el-icon>
                </div>
                <div class="message-content">
                  <div class="loading-indicator">
                    <el-icon class="is-loading"><Loading /></el-icon>
                    <span>AI 思考中...</span>
                  </div>
                </div>
              </div>
            </div>

            <!-- 输入框 -->
            <div class="chat-input">
              <el-input
                v-model="inputMessage"
                type="textarea"
                :rows="2"
                placeholder="输入您的问题..."
                :disabled="isLoading"
                @keydown.enter.ctrl="sendMessage"
              />
              <el-button type="primary" :loading="isLoading" :disabled="!inputMessage.trim()" @click="sendMessage">
                发送 (Ctrl+Enter)
              </el-button>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, nextTick, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { User, ChatDotRound, Tools, Loading, Aim, Check, CircleClose } from '@element-plus/icons-vue'
import { getAlerts } from '@/api/detection'
import { createAISession, createAISessionStream, type SSEEvent } from '@/api/aiAnalysis'

const route = useRoute()
const router = useRouter()

// Types
interface Alert {
  id: string
  hostname: string
  rule_title: string
  mitre_id: string
  severity: string
  status: string
  last_seen_at: string
}

interface Message {
  role: 'user' | 'assistant'
  content: string
  thinking?: string
  toolCalls?: Array<{
    call_id: string
    tool: string
    args?: any
    result?: any
  }>
  isError?: boolean
}

// State
const alerts = ref<Alert[]>([])
const alertLoading = ref(false)
const selectedAlertIds = ref<string[]>([])
const hostFilter = ref<string[]>([])
const hosts = ref<string[]>([])
const timeRange = ref<[string, string] | null>(null)

const sessionId = ref<string | null>(null)
const messages = ref<Message[]>([])
const inputMessage = ref('')
const isLoading = ref(false)
const messageListRef = ref<HTMLElement | null>(null)
const alertTableRef = ref<any>(null)

// Computed
const filteredAlerts = computed(() => {
  let result = alerts.value
  if (hostFilter.value.length > 0) {
    result = result.filter(a => hostFilter.value.includes(a.hostname))
  }
  return result
})

// Methods
async function loadAlerts() {
  alertLoading.value = true
  try {
    const params: any = { page: 1, page_size: 100 }
    const response = await getAlerts(params)
    alerts.value = response.data || []

    // Extract unique hosts
    const hostSet = new Set(alerts.value.map(a => a.hostname))
    hosts.value = Array.from(hostSet)
  } catch (error: any) {
    ElMessage.error(error.message || '加载告警失败')
  } finally {
    alertLoading.value = false
  }
}

function handleAlertSelection(selection: Alert[]) {
  selectedAlertIds.value = selection.map(a => a.id)
}

function handleTimeRangeChange() {
  // Time range is optional for filtering
}

async function startAnalysis() {
  if (selectedAlertIds.value.length === 0) {
    ElMessage.warning('请先选择要分析的告警')
    return
  }

  try {
    // Convert time range to RFC3339 format for backend
    const timeRangeFormatted = timeRange.value ? {
      start: new Date(timeRange.value[0]).toISOString(),
      end: new Date(timeRange.value[1]).toISOString()
    } : undefined

    const response = await createAISession({
      alert_ids: selectedAlertIds.value,
      time_range: timeRangeFormatted,
      host_filter: hostFilter.value.length > 0 ? hostFilter.value : undefined
    })

    sessionId.value = response.session_id
    messages.value = []

    ElMessage.success('AI 分析会话已创建')

    // Automatically send initial analysis request
    const initialMessage = `请分析这 ${selectedAlertIds.value.length} 个告警，判断是否为真实威胁，并进行攻击链路溯源。`
    sendInitialMessage(initialMessage)
  } catch (error: any) {
    ElMessage.error(error.message || '创建 AI 分析会话失败')
  }
}

function createSSEHandler(message: string) {
  let currentThinking = ''
  let rafId: number | null = null
  let pendingThinking = ''
  // Track if thinking was already flushed as its own bubble (to prevent duplicates)
  let thinkingFlushedAsBubble = false
  // Map to track tool call indices for quick lookup (avoids race condition)
  const toolCallIndexMap: Record<string, number> = {}

  const flushThinking = () => {
    if (pendingThinking !== currentThinking) {
      currentThinking = pendingThinking
      updateAssistantMessage(currentThinking, undefined)
    }
  }

  const scheduleFlush = () => {
    if (rafId) cancelAnimationFrame(rafId)
    rafId = requestAnimationFrame(() => {
      flushThinking()
      rafId = null
    })
  }

  const cleanup = () => {
    if (rafId) {
      cancelAnimationFrame(rafId)
      rafId = null
    }
  }

  const eventSource = createAISessionStream(sessionId.value!, message, (event: SSEEvent) => {
    switch (event.type) {
      case 'thinking':
        pendingThinking = (pendingThinking + (event.content || '')).trim()
        scheduleFlush()
        break

      case 'tool_call':
        cleanup()
        flushThinking()
        // Push thinking to its own bubble first, but only if not already done
        if (currentThinking && !thinkingFlushedAsBubble) {
          messages.value.push({
            role: 'assistant',
            content: '',
            thinking: currentThinking,
            toolCalls: []
          })
          thinkingFlushedAsBubble = true
        }
        // Push a new bubble for the tool call
        messages.value.push({
          role: 'assistant',
          content: '',
          thinking: '',
          toolCalls: [{
            call_id: event.call_id || '',
            tool: event.tool || '',
            args: event.args
          }]
        })
        // Track the index of this tool call bubble for fast lookup
        if (event.call_id) {
          toolCallIndexMap[event.call_id] = messages.value.length - 1
        }
        currentThinking = ''
        pendingThinking = ''
        break

      case 'tool_result':
        // Find the tool call bubble using the map (fast lookup)
        if (event.call_id && event.result !== undefined) {
          const idx = toolCallIndexMap[event.call_id]
          if (idx !== undefined && messages.value[idx]) {
            const msg = messages.value[idx]
            if (msg.toolCalls && msg.toolCalls.length > 0) {
              msg.toolCalls[0].result = event.result
            }
          }
        }
        break

      case 'tool_error':
        // Find the tool call bubble using the map (fast lookup)
        if (event.call_id && event.error) {
          const idx = toolCallIndexMap[event.call_id]
          if (idx !== undefined && messages.value[idx]) {
            const msg = messages.value[idx]
            if (msg.toolCalls && msg.toolCalls.length > 0) {
              msg.toolCalls[0].error = event.error
            }
          }
        }
        break

      case 'content':
        cleanup()
        pendingThinking = ''
        messages.value.push({
          role: 'assistant',
          content: event.content || '',
          thinking: '',
          toolCalls: []
        })
        isLoading.value = false
        scrollToBottom()
        break

      case 'done':
        cleanup()
        // Only flush thinking if it hasn't been flushed as its own bubble yet
        if (!thinkingFlushedAsBubble) {
          flushThinking()
        }
        isLoading.value = false
        scrollToBottom()
        break

      case 'error':
        ElMessage.error(event.content || 'AI 分析出错')
        cleanup()
        messages.value.push({
          role: 'assistant',
          content: `AI 分析失败: ${event.content || '未知错误'}`,
          isError: true
        })
        isLoading.value = false
        scrollToBottom()
        break
    }
  })

  return eventSource
}

function sendInitialMessage(message: string) {
  if (!sessionId.value || isLoading.value) return

  messages.value.push({
    role: 'user',
    content: message
  })

  scrollToBottom()
  isLoading.value = true
  createSSEHandler(message)
}

function sendMessage() {
  if (!inputMessage.value.trim() || !sessionId.value || isLoading.value) return

  const userMessage = inputMessage.value.trim()
  inputMessage.value = ''

  messages.value.push({
    role: 'user',
    content: userMessage
  })

  scrollToBottom()
  isLoading.value = true
  createSSEHandler(userMessage)
}

function updateAssistantMessage(thinking: string, toolCalls?: Message['toolCalls'], content?: string) {
  const lastMsg = messages.value[messages.value.length - 1]
  // Only update the last message if it's empty (no content and no toolCalls) and no error
  // Otherwise push a new message to create a new bubble
  if (lastMsg && lastMsg.role === 'assistant' && !content && !lastMsg.isError && !lastMsg.content && (!lastMsg.toolCalls || lastMsg.toolCalls.length === 0)) {
    lastMsg.thinking = thinking
    lastMsg.toolCalls = toolCalls
  } else {
    messages.value.push({
      role: 'assistant',
      content: content || '',
      thinking: thinking,
      toolCalls: toolCalls
    })
  }
  scrollToBottom()
}

function scrollToBottom() {
  nextTick(() => {
    if (messageListRef.value) {
      messageListRef.value.scrollTop = messageListRef.value.scrollHeight
    }
  })
}

function severityTagType(severity: string) {
  const map: Record<string, string> = {
    critical: 'danger',
    high: 'warning',
    medium: 'info',
    low: 'success'
  }
  return map[severity] || 'info'
}

function severityLabel(severity: string) {
  const map: Record<string, string> = {
    critical: '严重',
    high: '高危',
    medium: '中危',
    low: '低危'
  }
  return map[severity] || severity
}

// Init
onMounted(() => {
  // Check if we have query parameters from Alerts page
  const alertIdsParam = route.query.alert_ids as string
  const timeRangeStart = route.query.time_range_start as string
  const timeRangeEnd = route.query.time_range_end as string

  if (timeRangeStart && timeRangeEnd) {
    // Set time range from query params
    timeRange.value = [timeRangeStart, timeRangeEnd]
  }

  loadAlerts().then(() => {
    // If alert_ids are provided, select them
    if (alertIdsParam) {
      const ids = alertIdsParam.split(',')
      selectedAlertIds.value = ids

      // Auto-start analysis with selected alerts
      nextTick(() => {
        startAnalysis()
      })
    }
  })
})
</script>

<style scoped>
.ai-analysis-page {
  padding: 20px;
  height: calc(100vh - 120px);
}

.alert-selection-card,
.chat-card {
  height: calc(100vh - 160px);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.filter-form {
  margin-bottom: 16px;
}

.selection-info {
  margin-top: 16px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px;
  background: #f5f7fa;
  border-radius: 4px;
}

.no-session {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  min-height: 400px;
}

.chat-container {
  display: flex;
  flex-direction: column;
  height: calc(100% - 20px);
}

.message-list {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  background: #f5f7fa;
  border-radius: 8px;
}

.message {
  display: flex;
  margin-bottom: 16px;
  gap: 12px;
}

.message.user {
  flex-direction: row-reverse;
}

.message-avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.message.user .message-avatar {
  background: #409eff;
  color: white;
}

.message.assistant .message-avatar {
  background: #67c23a;
  color: white;
}

.message-content {
  max-width: 80%;
  padding: 12px 16px;
  border-radius: 8px;
  word-break: break-word;
}

.message.user .message-content {
  background: #409eff;
  color: white;
}

.message.assistant .message-content {
  background: white;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  min-height: 40px;
  width: 100%;
  box-sizing: border-box;
}

.user-content {
  white-space: pre-wrap;
}

.thinking-block {
  background: #f0f9ff;
  border: 1px solid #b3d8fd;
  border-radius: 4px;
  padding: 8px 12px;
  margin-bottom: 8px;
  min-height: 40px;
  box-sizing: border-box;
}

.thinking-header {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #409eff;
  font-size: 12px;
  margin-bottom: 8px;
  font-weight: 500;
}

.thinking-cursor {
  display: inline-block;
  width: 2px;
  height: 14px;
  background: #409eff;
  margin-left: 4px;
  animation: blink 1s infinite;
  vertical-align: middle;
}

@keyframes blink {
  0%, 50% { opacity: 1; }
  51%, 100% { opacity: 0; }
}

.thinking-content {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 13px;
  color: #303133;
  line-height: 1.6;
  max-height: none;
  overflow-y: visible;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
}

.error-block {
  background: #fef0f0;
  border: 1px solid #fde2e2;
  border-radius: 4px;
  padding: 8px 12px;
  margin-bottom: 8px;
  color: #f56c6c;
}

.error-block pre {
  margin: 0;
  white-space: pre-wrap;
  font-size: 13px;
}

.tool-calls {
  margin-bottom: 8px;
}

.tool-call-item {
  background: #ecf5ff;
  border: 1px solid #d9ecff;
  border-radius: 4px;
  padding: 8px 12px;
  margin-bottom: 8px;
}

.tool-call-header {
  display: flex;
  align-items: center;
  gap: 4px;
  color: #409eff;
  font-size: 12px;
  margin-bottom: 4px;
}

.tool-call-result {
  background: white;
  border-radius: 4px;
  padding: 8px;
  margin-top: 4px;
}

.tool-call-result pre {
  margin: 0;
  font-size: 12px;
  white-space: pre-wrap;
  max-height: 200px;
  overflow-y: auto;
}

.tool-call-error {
  background: #fef0f0;
  border-radius: 4px;
  padding: 8px;
  margin-top: 4px;
  color: #f56c6c;
}

.tool-call-error pre {
  margin: 0;
  font-size: 12px;
  white-space: pre-wrap;
  max-height: 200px;
  overflow-y: auto;
}

.final-content {
  white-space: pre-wrap;
}

.loading-indicator {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #909399;
}

.chat-input {
  display: flex;
  gap: 12px;
  margin-top: 16px;
  align-items: flex-end;
}

.chat-input .el-textarea {
  flex: 1;
}
</style>
