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
                  <el-icon v-else :size="20"><Robot /></el-icon>
                </div>
                <div class="message-content">
                  <!-- 用户消息 -->
                  <div v-if="msg.role === 'user'" class="user-content">
                    {{ msg.content }}
                  </div>

                  <!-- AI 消息 -->
                  <div v-else class="ai-content">
                    <!-- 思考过程 -->
                    <div v-if="msg.thinking" class="thinking-block">
                      <div class="thinking-header">
                        <el-icon><Thinking /></el-icon>
                        <span>AI 思考中</span>
                      </div>
                      <pre class="thinking-content">{{ msg.thinking }}</pre>
                    </div>

                    <!-- 工具调用 -->
                    <div v-if="msg.toolCalls && msg.toolCalls.length > 0" class="tool-calls">
                      <div v-for="call in msg.toolCalls" :key="call.call_id" class="tool-call-item">
                        <div class="tool-call-header">
                          <el-icon><Tools /></el-icon>
                          <span>调用工具: {{ call.tool }}</span>
                        </div>
                        <div class="tool-call-result" v-if="call.result">
                          <pre>{{ typeof call.result === 'string' ? call.result : JSON.stringify(call.result, null, 2) }}</pre>
                        </div>
                      </div>
                    </div>

                    <!-- 最终回复 -->
                    <div v-if="msg.content" class="final-content">
                      <pre>{{ msg.content }}</pre>
                    </div>
                  </div>
                </div>
              </div>

              <!-- 加载中 -->
              <div v-if="isLoading" class="message assistant loading">
                <div class="message-avatar">
                  <el-icon :size="20"><Robot /></el-icon>
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
import { ElMessage } from 'element-plus'
import { User, Robot, Tools, Loading } from '@element-plus/icons-vue'
import { getAlerts } from '@/api/detection'
import { createAISession, sendMessage as sendMessageApi, createAISessionStream, type SSEEvent } from '@/api/aiAnalysis'

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
    const response = await createAISession({
      alert_ids: selectedAlertIds.value,
      time_range: timeRange.value ? {
        start: timeRange.value[0],
        end: timeRange.value[1]
      } : undefined,
      host_filter: hostFilter.value.length > 0 ? hostFilter.value : undefined
    })

    sessionId.value = response.session_id
    messages.value = []

    ElMessage.success('AI 分析会话已创建')
  } catch (error: any) {
    ElMessage.error(error.message || '创建 AI 分析会话失败')
  }
}

function sendMessage() {
  if (!inputMessage.value.trim() || !sessionId.value || isLoading.value) return

  const userMessage = inputMessage.value.trim()
  inputMessage.value = ''

  // Add user message
  messages.value.push({
    role: 'user',
    content: userMessage
  })

  scrollToBottom()
  isLoading.value = true

  // Track current thinking for SSE events
  let currentThinking = ''
  let currentToolCalls: Message['toolCalls'] = []

  // Create SSE connection
  const eventSource = createAISessionStream(sessionId.value, userMessage, (event: SSEEvent) => {
    switch (event.type) {
      case 'thinking':
        currentThinking = (currentThinking + (event.content || '')).trim()
        // Update or add thinking message
        updateAssistantMessage(currentThinking, currentToolCalls)
        break

      case 'tool_call':
        currentToolCalls = currentToolCalls || []
        currentToolCalls.push({
          call_id: event.call_id || '',
          tool: event.tool || '',
          args: event.args
        })
        updateAssistantMessage(currentThinking, currentToolCalls)
        break

      case 'tool_result':
        if (currentToolCalls) {
          const lastCall = currentToolCalls[currentToolCalls.length - 1]
          if (lastCall && lastCall.call_id === event.call_id) {
            lastCall.result = event.result
          }
        }
        updateAssistantMessage(currentThinking, currentToolCalls)
        break

      case 'tool_error':
        if (currentToolCalls) {
          const lastCall = currentToolCalls[currentToolCalls.length - 1]
          if (lastCall && lastCall.call_id === event.call_id) {
            lastCall.result = { error: event.error }
          }
        }
        updateAssistantMessage(currentThinking, currentToolCalls)
        break

      case 'content':
        // Final content received
        updateAssistantMessage(currentThinking, currentToolCalls, event.content)
        break

      case 'done':
        // Ensure we have a final message
        if (currentThinking || currentToolCalls?.length > 0) {
          updateAssistantMessage(currentThinking, currentToolCalls)
        }
        isLoading.value = false
        scrollToBottom()
        break

      case 'error':
        ElMessage.error(event.content || 'AI 分析出错')
        isLoading.value = false
        break
    }
  })
}

function updateAssistantMessage(thinking: string, toolCalls?: Message['toolCalls'], content?: string) {
  const lastMsg = messages.value[messages.value.length - 1]
  if (lastMsg && lastMsg.role === 'assistant' && !content) {
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
  loadAlerts()
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
}

.thinking-header {
  display: flex;
  align-items: center;
  gap: 4px;
  color: #409eff;
  font-size: 12px;
  margin-bottom: 4px;
}

.thinking-content {
  margin: 0;
  white-space: pre-wrap;
  font-size: 13px;
  color: #606266;
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
