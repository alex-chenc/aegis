<template>
  <div class="ai-analysis-page">
    <el-row :gutter="20">
      <!-- 左侧：告警列表选择 -->
      <el-col :span="8">
        <el-card class="alert-selection-card">
          <template #header>
            <div class="card-header">
              <span>选择要分析的告警</span>
              <el-button size="small" @click="loadAlerts()">刷新</el-button>
            </div>
          </template>

          <div class="alert-selection-scroll">
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
                <el-select v-model="hostFilter" multiple filterable placeholder="选择在线主机" clearable :loading="hostLoading">
                  <el-option v-for="host in hosts" :key="host" :label="host" :value="host" />
                </el-select>
              </el-form-item>
              <el-form-item label="最大轮数">
                <el-input-number v-model="maxIterations" :min="1" :max="1000" size="default" />
              </el-form-item>
            </el-form>

            <div v-if="isAnalysisSnapshotActive" class="analysis-snapshot-hint">
              当前展示的是本次 AI 分析保留的事件快照，共 {{ analysisAlertSnapshot.length }} 条。
            </div>

            <el-table
              ref="alertTableRef"
              v-loading="alertLoading"
              :data="visibleAlertRows"
              border
              stripe
              height="400"
              row-key="id"
              @selection-change="handleAlertSelection"
            >
              <el-table-column type="selection" width="40" />
              <el-table-column prop="hostname" label="主机" min-width="100" />
              <el-table-column prop="rule_title" label="规则" min-width="150" show-overflow-tooltip>
                <template #default="{ row }">
                  {{ row.rule_title || row.mitre_id || '-' }}
                </template>
              </el-table-column>
              <el-table-column label="MITRE" width="120">
                <template #default="{ row }">
                  {{ row.mitre_id || '-' }}
                </template>
              </el-table-column>
              <el-table-column prop="last_seen_at" label="最近时间" min-width="150">
                <template #default="{ row }">
                  {{ formatTime(row.last_seen_at) }}
                </template>
              </el-table-column>
              <el-table-column prop="severity" label="级别" width="80" align="center">
                <template #default="{ row }">
                  <el-tag :type="severityTagType(row.severity)" size="small">
                    {{ severityLabel(row.severity) }}
                  </el-tag>
                </template>
              </el-table-column>
            </el-table>

            <div
              v-if="!isAnalysisSnapshotActive && alertTotal > alertPageSize"
              class="alert-pagination"
              aria-label="告警分页"
            >
              <div class="alert-pagination-summary">
                <span class="alert-pagination-total">共 {{ alertTotal }} 条</span>
                <el-select
                  v-model="alertPageSize"
                  class="alert-page-size-select"
                  size="small"
                  @change="handleAlertSizeChange"
                >
                  <el-option label="10 条/页" :value="10" />
                  <el-option label="20 条/页" :value="20" />
                  <el-option label="50 条/页" :value="50" />
                </el-select>
              </div>
              <el-pagination
                class="alert-pagination-pager"
                background
                small
                layout="prev, pager, next"
                :total="alertTotal"
                :page-size="alertPageSize"
                :current-page="alertPage"
                :pager-count="5"
                @current-change="handleAlertPageChange"
              />
            </div>

            <div class="selection-info">
              已选择 {{ selectedAlertIds.length }} 个告警
              <el-button type="primary" :disabled="selectedAlertIds.length === 0 || isAnalysisSnapshotActive" @click="startAnalysis">
                开始 AI 分析
              </el-button>
            </div>

            <!-- Execution plan (agent-runtime) -->
            <ExecutionPlan
              :plan="executionPlan"
              :audits="auditResults"
              :reflections="reflectionResults"
              :corrections="correctionResults"
            />
          </div>
        </el-card>
      </el-col>

      <!-- 右侧：AI 分析对话 -->
      <el-col :span="16">
        <el-card class="chat-card">
          <template #header>
            <div class="card-header">
              <span>AI 安全分析助手</span>
              <div class="header-actions">
                <el-button size="small" @click="showSessionHistory">
                  历史会话
                </el-button>
              </div>
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

<!-- Thought 独立显示 -->
                    <div v-if="msg.thought" class="thought-block">
                      <div class="block-header">
                        <el-icon><Aim /></el-icon>
                        <span>Thought</span>
                      </div>
                      <div class="thought-text">{{ msg.thought }}</div>
                    </div>

                    <!-- Action 独立显示 -->
                    <div v-if="msg.action" class="action-block">
                      <div class="block-header">
                        <el-icon><Tools /></el-icon>
                        <span>Action: {{ msg.action }}</span>
                      </div>
                    </div>

                    <!-- Action Input 独立显示 -->
                    <div v-if="msg.actionInput" class="action-input-block">
                      <div class="block-header">
                        <el-icon><Edit /></el-icon>
                        <span>Action Input</span>
                      </div>
                      <div class="block-content">
                        <pre>{{ typeof msg.actionInput === 'string' ? msg.actionInput : JSON.stringify(msg.actionInput, null, 2) }}</pre>
                      </div>
                    </div>

                    <!-- Observation 独立显示 -->
                    <div v-if="msg.observation" class="observation-block">
                      <div class="block-header">
                        <el-icon><View /></el-icon>
                        <span>Observation</span>
                        <el-icon class="is-loading" v-if="msg.isLoading"><Loading /></el-icon>
                        <el-icon color="#67c23a" v-else-if="msg.observation"><Check /></el-icon>
                        <el-icon color="#f56c6c" v-else-if="msg.observationError"><CircleClose /></el-icon>
                      </div>
                      <div class="block-content" v-if="msg.observationError">
                        <pre class="error-text">{{ msg.observationError }}</pre>
                      </div>
                      <div class="block-content" v-else>
                        <pre>{{ typeof msg.observation === 'string' ? msg.observation : JSON.stringify(msg.observation, null, 2) }}</pre>
                      </div>
                    </div>

                    <!-- 最终回复 -->
                    <div v-if="msg.executionResult && !msg.thought" class="final-execution-result">
                      <TaskExecutionResult :result="msg.executionResult" />
                    </div>
                    <div v-else-if="msg.content && !msg.thought && !msg.isError" class="final-content">
                      <pre>{{ msg.content }}</pre>
                    </div>

                    <!-- 审计结果气泡 -->
                    <div v-if="msg.type === 'audit' && msg.auditResult" class="audit-block">
                      <div class="block-header">
                        <el-icon><Warning /></el-icon>
                        <span>审计结果</span>
                        <el-tag :type="msg.auditResult.risk_level === 'high' ? 'danger' : msg.auditResult.risk_level === 'medium' ? 'warning' : 'info'" size="small">
                          {{ msg.auditResult.risk_level }}
                        </el-tag>
                      </div>
                      <div class="block-content">
                        <div><strong>决策:</strong> {{ msg.auditResult.decision }}</div>
                        <div v-if="msg.auditResult.findings?.length">
                          <strong>发现:</strong>
                          <ul>
                            <li v-for="(f, i) in msg.auditResult.findings" :key="i">{{ f }}</li>
                          </ul>
                        </div>
                      </div>
                    </div>

                    <!-- 反思结果气泡 -->
                    <div v-if="msg.type === 'reflection' && msg.reflectionResult" class="reflection-block">
                      <div class="block-header">
                        <el-icon><RefreshRight /></el-icon>
                        <span>反思</span>
                      </div>
                      <div class="block-content">
                        <div><strong>根因:</strong> {{ msg.reflectionResult.root_cause }}</div>
                        <div><strong>影响:</strong> {{ msg.reflectionResult.impact }}</div>
                        <div><strong>建议:</strong> {{ msg.reflectionResult.recommendation }}</div>
                      </div>
                    </div>

                    <!-- 纠正结果气泡 -->
                    <div v-if="msg.type === 'correction' && msg.correctionResult" class="correction-block">
                      <div class="block-header">
                        <el-icon><CircleCheck /></el-icon>
                        <span>纠正</span>
                      </div>
                      <div class="block-content">
                        <div><strong>原因:</strong> {{ msg.correctionResult.reason }}</div>
                        <div v-if="msg.correctionResult.actions?.length">
                          <strong>操作:</strong>
                          <ul>
                            <li v-for="(a, i) in msg.correctionResult.actions" :key="i">{{ a }}</li>
                          </ul>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              <!-- 加载中指示器 -->
              <div v-if="isLoading && (!messages.length || !messages[messages.length - 1]?.thought)" class="message assistant loading">
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
                @keydown.enter.ctrl="handleEnterKey"
              />
              <el-button
                v-if="getActionButtonType(isLoading, !!inputMessage.trim()) === 'pause'"
                type="warning"
                @click="pauseAnalysis"
              >
                暂停
              </el-button>
              <el-button
                v-else
                type="primary"
                :disabled="!inputMessage.trim()"
                @click="sendMessage"
              >
                发送 (Ctrl+Enter)
              </el-button>
            </div>
          </div>

          <!-- 执行结果展示区域 -->
          <div v-if="executionResult && !hasMessageExecutionResult" class="execution-result-panel">
            <div class="execution-result-header">
              <span>任务执行结果</span>
              <el-button size="small" type="danger" @click="executionResult = null">
                关闭
              </el-button>
            </div>
            <TaskExecutionResult :result="executionResult" />
          </div>

          <!-- 溯源图展示区域 -->
          <el-card v-if="attackGraph" class="attack-graph-card" style="margin-top: 16px;">
            <template #header>
              <div class="card-header">
                <span>攻击溯源图</span>
                <div class="header-actions">
                  <el-button size="small" @click="downloadFlowchartImage">
                    <el-icon><Download /></el-icon>
                    下载流程图
                  </el-button>
                  <el-button size="small" type="danger" @click="closeFlowchart">
                    关闭
                  </el-button>
                </div>
              </div>
            </template>
            <AttackGraph :graph-data="attackGraph" />
          </el-card>
        </el-card>

        <!-- 历史会话对话框 -->
        <el-dialog v-model="sessionHistoryVisible" title="历史会话" width="800px">
          <el-table
            v-loading="sessionListLoading"
            :data="sessionList"
            border
            stripe
            @row-click="handleSelectSession"
            style="cursor: pointer;"
          >
            <el-table-column prop="session_id" label="会话ID" width="200" />
            <el-table-column prop="alert_ids" label="关联告警" width="120">
              <template #default="{ row }">
                {{ Array.isArray(row.alert_ids) ? row.alert_ids.length : 0 }} 个
              </template>
            </el-table-column>
            <el-table-column prop="status" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="getDisplayStatus(row) === 'completed' ? 'success' : 'info'" size="small">
                  {{ getDisplayStatus(row) === 'completed' ? '已完成' : '未完成' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="max_iterations" label="最大轮数" width="100" />
            <el-table-column prop="message_count" label="消息数" width="80" />
            <el-table-column prop="created_at" label="创建时间" width="180">
              <template #default="{ row }">
                {{ formatTime(row.created_at) }}
              </template>
            </el-table-column>
            <el-table-column label="操作" width="150" fixed="right">
              <template #default="{ row }">
                <div style="display: flex; gap: 8px; justify-content: center;">
                  <el-button size="small" type="primary" @click.stop="loadSession(row)">
                    加载
                  </el-button>
                  <el-button size="small" type="danger" @click.stop="deleteSessionById(row)">
                    删除
                  </el-button>
                </div>
              </template>
            </el-table-column>
          </el-table>
          <el-pagination
            v-if="sessionTotal > sessionPageSize"
            layout="prev, pager, next"
            :total="sessionTotal"
            :page-size="sessionPageSize"
            :current-page="sessionPage"
            @current-change="handleSessionPageChange"
            style="margin-top: 16px; text-align: center;"
          />
          <template #footer>
            <el-button @click="sessionHistoryVisible = false">关闭</el-button>
          </template>
        </el-dialog>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { User, ChatDotRound, Tools, Loading, Aim, Check, CircleClose, View, Edit, Download, Warning, RefreshRight, CircleCheck } from '@element-plus/icons-vue'
import { getAlerts } from '@/api/detection'
import { getHosts } from '@/api/hosts'
import { createAISession, createAISessionStream, getSessionList, getSessionHistory, deleteSession, pauseSession, getExecutionResult, type SSEEvent, type PlanEvent, type AuditEvent, type ReflectionEvent, type CorrectionEvent, type ExecutionResult } from '@/api/aiAnalysis'
import AttackGraph from '@/components/AttackGraph.vue'
import ExecutionPlan from '@/components/ExecutionPlan.vue'
import TaskExecutionResult from '@/components/TaskExecutionResult.vue'
import {
  buildAttackGraphDisplayText,
  buildAttackGraphSvgDataUrl,
  extractAttackGraphFinalAnswer,
  isLikelyAttackGraphFinalAnswer,
  type AttackGraphData
} from '@/utils/attackGraph'
import { buildInitialAnalysisMessage, normalizeAIAnalysisErrorMessage } from '@/utils/aiAnalysisView'
import { buildAnalysisAlertQuery, buildAnalysisAlertSnapshot, filterOnlineHostnames, pruneSelectedAlertIds } from '@/utils/aiAnalysisFilters'
import { applyPlanStepStatus, getActionButtonType, normalizePlanEvent } from '@/utils/aiAnalysisRuntime'
import { parseExecutionResultText } from '@/utils/taskExecutionResult'
import { getDisplayStatus, isFalsePositive, getRemediationSuggestion, getVerdictType, getVerdictText } from '@/utils/sessionStatus'

const route = useRoute()
const router = useRouter()

// Types
interface Alert {
  id: string
  alert_id?: string
  hostname?: string
  rule_title?: string
  mitre_id: string
  severity: string
  status: string
  description?: string
  last_seen_at: string
}

interface Message {
  role: 'user' | 'assistant'
  content?: string
  thought?: string
  action?: string
  callId?: string
  actionInput?: any
  observation?: any
  observationError?: string
  toolCalls?: Array<{
    call_id: string
    tool: string
    args?: any
    result?: any
  }>
  isError?: boolean
  isLoading?: boolean
  // agent-runtime message types
  type?: 'audit' | 'reflection' | 'correction'
  planStepId?: string
  auditResult?: AuditEvent
  reflectionResult?: ReflectionEvent
  correctionResult?: CorrectionEvent
  executionResult?: ExecutionResult | null
}

// State
const alerts = ref<Alert[]>([])
const alertLoading = ref(false)
const hostLoading = ref(false)
const selectedAlertIds = ref<string[]>([])
const hostFilter = ref<string[]>([])
const hosts = ref<string[]>([])
const timeRange = ref<[string, string] | null>(null)
const analysisAlertSnapshot = ref<Alert[]>([])
let alertLoadSeq = 0

// Pagination state
const alertPage = ref(1)
const alertPageSize = ref(10)
const alertTotal = ref(0)

const sessionId = ref<string | null>(null)
const messages = ref<Message[]>([])
const inputMessage = ref('')
const attackGraph = ref<AttackGraphData | null>(null)
const finalAnswerContent = ref<string>('')
const generatedFlowchartImageUrl = ref('')
const isLoading = ref(false)
const maxIterations = ref(500)
const executionPlan = ref<PlanEvent | null>(null)
const auditResults = ref<AuditEvent[]>([])
const reflectionResults = ref<ReflectionEvent[]>([])
const correctionResults = ref<CorrectionEvent[]>([])
const executionResult = ref<ExecutionResult | null>(null)
const messageListRef = ref<HTMLElement | null>(null)
const alertTableRef = ref<any>(null)
const currentEventSource = ref<EventSource | null>(null)

// LocalStorage keys
const CURRENT_SESSION_KEY = 'aegis_current_session_id'
const STRUCTURED_FINAL_PENDING_TEXT = '正在整理最终结论与溯源图...'
const savedSessionId = ref<string | null>(null)
const getStorageKey = () => `aegis_ai_session_${savedSessionId.value}`

// Save current session ID to a fixed key for page reload recovery
function saveCurrentSessionId() {
  if (sessionId.value) {
    localStorage.setItem(CURRENT_SESSION_KEY, sessionId.value)
  }
}

// Load current session ID from localStorage
function loadCurrentSessionId(): string | null {
  return localStorage.getItem(CURRENT_SESSION_KEY)
}

// Clear current session ID
function clearCurrentSessionId() {
  localStorage.removeItem(CURRENT_SESSION_KEY)
}

// Save current conversation to localStorage
function saveConversation() {
  if (savedSessionId.value && messages.value.length > 0) {
    try {
      const data = {
        messages: messages.value,
        attackGraph: attackGraph.value,
        analysisAlertSnapshot: analysisAlertSnapshot.value,
        finalAnswerContent: finalAnswerContent.value,
        generatedFlowchartImageUrl: generatedFlowchartImageUrl.value,
        executionPlan: executionPlan.value,
        auditResults: auditResults.value,
        reflectionResults: reflectionResults.value,
        correctionResults: correctionResults.value,
        maxIterations: maxIterations.value,
        savedAt: new Date().toISOString()
      }
      localStorage.setItem(getStorageKey(), JSON.stringify(data))
    } catch (e) {
      // Handle localStorage quota exceeded silently
      console.warn('Failed to save conversation to localStorage:', e)
    }
  }
}

// Load conversation from localStorage
function loadConversation(): boolean {
  if (!savedSessionId.value) return false
  const stored = localStorage.getItem(getStorageKey())
  if (stored) {
    try {
      const data = JSON.parse(stored)
      messages.value = data.messages || []
      attackGraph.value = data.attackGraph || null
      analysisAlertSnapshot.value = data.analysisAlertSnapshot || []
      finalAnswerContent.value = data.finalAnswerContent || ''
      generatedFlowchartImageUrl.value = data.generatedFlowchartImageUrl || ''
      executionPlan.value = data.executionPlan || null
      auditResults.value = data.auditResults || []
      reflectionResults.value = data.reflectionResults || []
      correctionResults.value = data.correctionResults || []
      maxIterations.value = data.maxIterations || 500
      applyStructuredFinalAnswer()
      applyParsedExecutionResultFromContent()
      return true
    } catch (e) {
      console.error('Failed to load conversation from localStorage:', e)
    }
  }
  return false
}

// Clear saved conversation from localStorage
function clearSavedConversation() {
  if (savedSessionId.value) {
    localStorage.removeItem(getStorageKey())
  }
  savedSessionId.value = null
}

// Computed
const filteredAlerts = computed(() => {
  if (!hasAlertSearchCondition.value) return []
  return alerts.value
})

const hasAlertSearchCondition = computed(() => {
  return hostFilter.value.length > 0 || Boolean(timeRange.value?.[0] && timeRange.value?.[1])
})

const isAnalysisSnapshotActive = computed(() => Boolean(sessionId.value && analysisAlertSnapshot.value.length > 0))

const visibleAlertRows = computed(() => {
  return isAnalysisSnapshotActive.value ? analysisAlertSnapshot.value : filteredAlerts.value
})

const hasMessageExecutionResult = computed(() => {
  return messages.value.some(msg => Boolean(msg.executionResult))
})

// Methods
async function loadHosts() {
  hostLoading.value = true
  try {
    const response = await getHosts({ page: 1, pageSize: 1000 })
    hosts.value = filterOnlineHostnames(response)
  } catch (error: any) {
    ElMessage.error(error.message || '加载在线主机失败')
  } finally {
    hostLoading.value = false
  }
}

async function loadAlerts(force = false) {
  const query = buildAnalysisAlertQuery(hostFilter.value, timeRange.value, alertPage.value, alertPageSize.value)

  if (!force && !query) {
    alertLoadSeq += 1
    alerts.value = []
    selectedAlertIds.value = []
    alertTotal.value = 0
    alertLoading.value = false
    return
  }

  const currentSeq = ++alertLoadSeq
  alertLoading.value = true
  try {
    const response = await getAlerts(query || { page: alertPage.value, pageSize: alertPageSize.value })
    if (currentSeq !== alertLoadSeq) {
      return
    }

    alerts.value = response.data || []
    alertTotal.value = response.total || 0
  } catch (error: any) {
    if (currentSeq === alertLoadSeq) {
      ElMessage.error(error.message || '加载告警失败')
    }
  } finally {
    if (currentSeq === alertLoadSeq) {
      alertLoading.value = false
    }
  }
}

function handleAlertSelection(selection: Alert[]) {
  selectedAlertIds.value = selection.map(a => a.id)
}

function handleTimeRangeChange() {
  pruneSelectionToVisibleAlerts()
}

function pruneSelectionToVisibleAlerts() {
  if (isAnalysisSnapshotActive.value) return
  selectedAlertIds.value = pruneSelectedAlertIds(selectedAlertIds.value, filteredAlerts.value)
}

function handleAlertPageChange(page: number) {
  alertPage.value = page
  loadAlerts()
}

function handleAlertSizeChange(size: number) {
  alertPageSize.value = size
  alertPage.value = 1
  loadAlerts()
}

// Reset page when filters change
watch([hostFilter, timeRange], () => {
  alertPage.value = 1
}, { deep: true })

// Restore selection state when alerts change (for cross-page selection)
watch(alerts, () => {
  nextTick(() => {
    if (alertTableRef.value) {
      alerts.value.forEach((alert: Alert) => {
        if (selectedAlertIds.value.includes(alert.id)) {
          alertTableRef.value.toggleRowSelection(alert, true)
        }
      })
    }
  })
})

function formatTime(timestamp: string): string {
  if (!timestamp) return '-'
  return new Date(timestamp).toLocaleString('zh-CN')
}

// Session history functions
const sessionHistoryVisible = ref(false)
const sessionListLoading = ref(false)
const sessionList = ref<SessionListItem[]>([])
const sessionTotal = ref(0)
const sessionPage = ref(1)
const sessionPageSize = ref(10)

async function loadSessionList(page: number = 1) {
  sessionListLoading.value = true
  sessionPage.value = page
  try {
    const response = await getSessionList(page, sessionPageSize.value)
    const payload = (response as any).data || response
    sessionList.value = payload.sessions || []
    sessionTotal.value = payload.total || 0
  } catch (error: any) {
    ElMessage.error(error.message || '加载历史会话失败')
  } finally {
    sessionListLoading.value = false
  }
}

function showSessionHistory() {
  sessionHistoryVisible.value = true
  loadSessionList(1)
}

function handleSessionPageChange(page: number) {
  loadSessionList(page)
}

function isFinalAssistantMessage(msg?: Message) {
  return Boolean(
    msg &&
      msg.role === 'assistant' &&
      !msg.thought &&
      !msg.action &&
      !msg.observation &&
      !msg.type &&
      !msg.isError
  )
}

function findLatestFinalAssistantMessageIndex() {
  const index = messages.value.length - 1
  return isFinalAssistantMessage(messages.value[index]) ? index : -1
}

function upsertFinalAssistantMessage(content: string, append = false) {
  const index = findLatestFinalAssistantMessageIndex()
  if (index >= 0) {
    messages.value[index].content = append ? (messages.value[index].content || '') + content : content
    if (append || content) {
      messages.value[index].executionResult = null
    }
    return
  }

  messages.value.push({
    role: 'assistant',
    content,
    thought: '',
    action: '',
    actionInput: null,
    observation: null,
    executionResult: null
  })
}

function applyStructuredFinalAnswer(content = finalAnswerContent.value) {
  const finalAnswer = extractAttackGraphFinalAnswer(content)
  if (!finalAnswer) return false

  attackGraph.value = finalAnswer.graph
  upsertFinalAssistantMessage(buildAttackGraphDisplayText(finalAnswer))
  return true
}

function attachExecutionResultToLatestMessage(result: ExecutionResult) {
  const index = findLatestFinalAssistantMessageIndex()
  if (index >= 0) {
    messages.value[index].executionResult = result
    return
  }

  messages.value.push({
    role: 'assistant',
    content: '',
    executionResult: result
  })
}

function applyParsedExecutionResultFromContent(content = finalAnswerContent.value) {
  const parsed = parseExecutionResultText(content)
  if (!parsed) return false

  executionResult.value = parsed
  attachExecutionResultToLatestMessage(parsed)
  return true
}

async function loadExecutionResultForSession(targetSessionId: string, attachToMessage = false) {
  try {
    const result = await getExecutionResult(targetSessionId)
    executionResult.value = result
    if (attachToMessage) {
      attachExecutionResultToLatestMessage(result)
    }
    return result
  } catch {
    return null
  }
}

async function deleteSessionById(session: SessionListItem) {
  try {
    await deleteSession(session.session_id)
    if (sessionId.value === session.session_id) {
      closeCurrentStreamSilently()
      sessionId.value = null
      messages.value = []
      executionPlan.value = null
      auditResults.value = []
      reflectionResults.value = []
      correctionResults.value = []
      executionResult.value = null
      clearCurrentSessionId()
      clearSavedConversation()
    }
    ElMessage.success('会话已删除')
    loadSessionList(sessionPage.value)
  } catch (error: any) {
    ElMessage.error(error.message || '删除会话失败')
  }
}

interface SessionListItem {
  id: string
  session_id: string
  alert_ids: string[]
  host_ids: string[]
  status: string
  max_iterations: number
  message_count: number
  created_at: string
}

async function loadSession(session: SessionListItem) {
  // Guard against double execution
  if (sessionId.value === session.session_id && messages.value.length > 0) {
    sessionHistoryVisible.value = false
    if (!executionResult.value) {
      void loadExecutionResultForSession(session.session_id, true)
    }
    return
  }

  // Clear current session ID when switching to history session
  clearCurrentSessionId()
  clearSavedConversation()

  // Set savedSessionId for localStorage key stability
  savedSessionId.value = session.session_id
  sessionId.value = session.session_id
  sessionHistoryVisible.value = false
  messages.value = []
  finalAnswerContent.value = ''
  attackGraph.value = null
  generatedFlowchartImageUrl.value = ''
  executionResult.value = null
  executionPlan.value = null
  auditResults.value = []
  reflectionResults.value = []
  correctionResults.value = []
  analysisAlertSnapshot.value = []
  maxIterations.value = session.max_iterations || 500

  // 获取会话显示状态
  const sessionStatus = getDisplayStatus(session)

  // Load messages from history
  try {
    const response = await getSessionHistory(session.session_id)
    // Backend returns {success: true, data: {session_id, messages, execution_plan}}
    const payload = (response as any).data || response
    const msgs = payload.messages || []
    if (msgs.length > 0) {
      messages.value = rebuildMessagesFromHistory(msgs)

      // 只有已完成的会话才应用结论相关逻辑
      if (sessionStatus === 'completed') {
        applyStructuredFinalAnswer()
        applyParsedExecutionResultFromContent()
      }
    }
    // Load execution plan from history
    if (payload.execution_plan) {
      executionPlan.value = normalizePlanEvent(payload.execution_plan)
    }

    // 审计和反思：如果为空则显示空数组
    auditResults.value = payload.audits || []
    reflectionResults.value = payload.reflections || []
    correctionResults.value = payload.corrections || []

    // 只有已完成的会话才追加运行时事件消息
    if (sessionStatus === 'completed') {
      appendHistoryRuntimeEventMessages(auditResults.value, reflectionResults.value, correctionResults.value)
    }
  } catch (error: any) {
    ElMessage.error(error.message || '加载会话消息失败')
  }

  // 加载执行结果（仅已完成会话）
  if (sessionStatus === 'completed') {
    await loadExecutionResultForSession(session.session_id, true)
  }

  ElMessage.success('已加载会话')
}

function handleSelectSession(row: SessionListItem) {
  loadSession(row)
}

function appendHistoryRuntimeEventMessages(
  audits: AuditEvent[] = [],
  reflections: ReflectionEvent[] = [],
  corrections: CorrectionEvent[] = []
) {
  audits.forEach((audit) => {
    messages.value.push({
      role: 'assistant',
      content: '',
      type: 'audit',
      auditResult: audit
    })
  })
  reflections.forEach((reflection) => {
    messages.value.push({
      role: 'assistant',
      content: '',
      type: 'reflection',
      reflectionResult: reflection
    })
  })
  corrections.forEach((correction) => {
    messages.value.push({
      role: 'assistant',
      content: '',
      type: 'correction',
      correctionResult: correction
    })
  })
}

function unwrapHistoryItems<T = any>(value: any): T[] {
  if (!value) return []
  if (Array.isArray(value)) return value
  if (Array.isArray(value.items)) return value.items
  return []
}

function findToolResult(toolResults: any[], callId?: string) {
  if (!callId) return null
  return toolResults.find(item => item?.call_id === callId) || null
}

function rebuildMessagesFromHistory(historyMessages: any[]): Message[] {
  const rebuilt: Message[] = []

  historyMessages.forEach((msg) => {
    if (msg.role === 'user') {
      rebuilt.push({
        role: 'user',
        content: msg.content || ''
      })
      return
    }

    const steps = unwrapHistoryItems<any>(msg.steps)
    const toolCalls = unwrapHistoryItems<any>(msg.tool_calls)
    const toolResults = unwrapHistoryItems<any>(msg.tool_results)

    if (steps.length > 0) {
      steps.forEach((step) => {
        if (step.thought) {
          rebuilt.push({
            role: 'assistant',
            thought: step.thought,
            content: ''
          })
        }
        if (step.action || step.action_input || step.observation) {
          rebuilt.push({
            role: 'assistant',
            content: '',
            action: step.action || '',
            actionInput: step.action_input || null,
            observation: step.observation || null
          })
        }
      })
    } else {
      if (msg.thinking) {
        rebuilt.push({
          role: 'assistant',
          thought: msg.thinking,
          content: ''
        })
      }

      toolCalls.forEach((call) => {
        const result = findToolResult(toolResults, call.call_id)
        rebuilt.push({
          role: 'assistant',
          content: '',
          action: call.tool || '',
          callId: call.call_id,
          actionInput: call.args || call.arguments || null,
          observation: result?.result || null,
          observationError: result?.error || ''
        })
      })
    }

    if (msg.content) {
      rebuilt.push({
        role: 'assistant',
        content: msg.content
      })
      finalAnswerContent.value += msg.content
    }
  })

  return rebuilt
}

async function startAnalysis() {
  if (selectedAlertIds.value.length === 0) {
    ElMessage.warning('请先选择要分析的告警')
    return
  }

  try {
    const analysisSnapshot = buildAnalysisAlertSnapshot(filteredAlerts.value, selectedAlertIds.value)
    if (analysisSnapshot.length === 0) {
      ElMessage.warning('当前筛选条件下没有可分析的告警')
      return
    }

    // Convert time range to RFC3339 format for backend
    const timeRangeFormatted = timeRange.value ? {
      start: new Date(timeRange.value[0]).toISOString(),
      end: new Date(timeRange.value[1]).toISOString()
    } : undefined

    const response = await createAISession({
      alert_ids: analysisSnapshot.map(alert => alert.id),
      time_range: timeRangeFormatted,
      host_filter: hostFilter.value.length > 0 ? hostFilter.value : undefined,
      max_iterations: maxIterations.value
    })

    // Clear any saved conversation for previous session
    clearSavedConversation()

    // Set savedSessionId for localStorage key stability
    savedSessionId.value = response.session_id
    sessionId.value = response.session_id
    messages.value = []
    finalAnswerContent.value = ''
    attackGraph.value = null
    generatedFlowchartImageUrl.value = ''
    executionPlan.value = null
    auditResults.value = []
    reflectionResults.value = []
    correctionResults.value = []
    executionResult.value = null
    analysisAlertSnapshot.value = analysisSnapshot
    selectedAlertIds.value = analysisSnapshot.map(alert => alert.id)

    // Save current session ID for page reload recovery
    saveCurrentSessionId()

    // Try to load saved conversation if exists
    if (!loadConversation()) {
      ElMessage.success('AI 分析会话已创建')

      // Automatically send initial analysis request
      const initialMessage = buildInitialAnalysisMessage(
        analysisSnapshot.map(alert => ({
          id: alert.alert_id || alert.id,
          hostname: alert.hostname,
          rule_title: alert.rule_title,
          severity: alert.severity,
          description: alert.description,
          last_seen_at: alert.last_seen_at
        })),
        timeRange.value
      )
      sendInitialMessage(initialMessage)
    } else {
      ElMessage.success('已恢复会话')
    }
  } catch (error: any) {
    ElMessage.error(error.message || '创建 AI 分析会话失败')
  }
}

function createSSEHandler(message: string) {
  let rafId: number | null = null
  let currentThought = ''
  let thoughtMsgIndex: number = -1 // Index of the thought message in messages array
  let lastCompletedThought = ''
  // Track current action for association with observation
  let currentAction = ''
  let currentCallId = ''
  let currentArgs: any = null
  let structuredFinalCandidate = false

  const normalizeThought = (value: string) => value.replace(/\s+/g, ' ').trim()

  const mergeThoughtChunk = (chunk: string) => {
    if (!chunk.trim()) return false
    const nextChunk = chunk

    const normalizedChunk = normalizeThought(nextChunk)
    const normalizedCurrent = normalizeThought(currentThought)

    if (!normalizedCurrent && normalizedChunk === lastCompletedThought) {
      return false
    }

    if (!normalizedCurrent) {
      currentThought = nextChunk
      return true
    }

    if (normalizedChunk === normalizedCurrent || normalizedCurrent.endsWith(normalizedChunk)) {
      return false
    }

    if (normalizedChunk.startsWith(normalizedCurrent)) {
      currentThought = nextChunk
    } else {
      currentThought += nextChunk
    }

    return true
  }

  const flushThought = (complete = false) => {
    const thought = currentThought.trim()
    if (!thought) return

    if (thoughtMsgIndex >= 0 && thoughtMsgIndex < messages.value.length) {
      messages.value[thoughtMsgIndex].thought = thought
    } else {
      // Create new thought message and record its index
      thoughtMsgIndex = messages.value.length
      messages.value.push({
        role: 'assistant',
        thought,
        content: '',
        action: '',
        actionInput: null,
        observation: null
      })
    }
    if (complete) {
      lastCompletedThought = normalizeThought(thought)
      currentThought = ''
      thoughtMsgIndex = -1
    }
    scrollToBottom()
  }

  const scheduleFlush = () => {
    if (rafId) cancelAnimationFrame(rafId)
    rafId = requestAnimationFrame(() => {
      flushThought()
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
        if (mergeThoughtChunk(event.content || '')) {
          scheduleFlush()
        }
        break

      case 'tool_call':
        cleanup()
        flushThought(true)
        // Store the action info
        currentAction = event.tool || ''
        currentCallId = event.call_id || ''
        currentArgs = event.args
        // Push a message with Action and Action Input
        messages.value.push({
          role: 'assistant',
          content: '',
          thought: '',
          action: currentAction,
          callId: currentCallId,
          actionInput: currentArgs,
          observation: null,
          isLoading: true
        })
        scrollToBottom()
        break

      case 'tool_result':
        // Find the latest loading observation message and update it
        // Find message with matching call_id or the most recent loading message
        for (let i = messages.value.length - 1; i >= 0; i--) {
          const msg = messages.value[i]
          if (msg.action && msg.isLoading && (!event.call_id || msg.callId === event.call_id)) {
            msg.observation = event.result
            msg.isLoading = false
            break
          }
        }
        scrollToBottom()
        break

      case 'tool_error':
        // Find the latest loading observation message and update it with error
        for (let i = messages.value.length - 1; i >= 0; i--) {
          const msg = messages.value[i]
          if (msg.action && msg.isLoading && (!event.call_id || msg.callId === event.call_id)) {
            msg.observationError = event.error || 'Tool execution failed'
            msg.isLoading = false
            break
          }
        }
        scrollToBottom()
        break

      case 'content':
        cleanup()
        flushThought(true)
        // Store final answer content for attack graph parsing
        if (event.content) {
          finalAnswerContent.value += event.content
        }
        structuredFinalCandidate = structuredFinalCandidate || isLikelyAttackGraphFinalAnswer(finalAnswerContent.value)
        if (structuredFinalCandidate) {
          if (!applyStructuredFinalAnswer()) {
            upsertFinalAssistantMessage(STRUCTURED_FINAL_PENDING_TEXT)
          }
        } else {
          upsertFinalAssistantMessage(event.content || '', true)
        }
        // Don't set isLoading=false here; wait for 'done' event
        scrollToBottom()
        break

      case 'flowchart_image':
        if (event.result?.url) {
          generatedFlowchartImageUrl.value = event.result.url
        } else if (event.error) {
          ElMessage.warning(event.error)
        }
        scrollToBottom()
        break

      case 'plan':
        try {
          const planData = typeof event.content === 'string' ? JSON.parse(event.content) : event.result
          if (planData) {
            executionPlan.value = normalizePlanEvent(planData)
          }
        } catch {
          // ignore parse errors
        }
        scrollToBottom()
        break

      case 'step_started':
        applyPlanStepStatus(executionPlan.value, event.call_id, 'running')
        break

      case 'step_completed':
        applyPlanStepStatus(executionPlan.value, event.call_id, 'completed', event.content || '')
        break

      case 'step_failed':
        applyPlanStepStatus(executionPlan.value, event.call_id, 'failed', event.error || event.content || '')
        break

      case 'step_retrying':
        applyPlanStepStatus(executionPlan.value, event.call_id, 'retrying', '正在重试...')
        break

      case 'step_skipped':
        applyPlanStepStatus(executionPlan.value, event.call_id, 'skipped', '已跳过')
        break

      case 'audit':
        try {
          const auditData = typeof event.content === 'string' ? JSON.parse(event.content) : event.result
          if (auditData) {
            auditResults.value.push(auditData as AuditEvent)
            messages.value.push({
              role: 'assistant',
              content: '',
              type: 'audit',
              auditResult: auditData as AuditEvent
            })
          }
        } catch {
          // ignore parse errors
        }
        scrollToBottom()
        break

      case 'reflection':
        try {
          const reflData = typeof event.content === 'string' ? JSON.parse(event.content) : event.result
          if (reflData) {
            reflectionResults.value.push(reflData as ReflectionEvent)
            messages.value.push({
              role: 'assistant',
              content: '',
              type: 'reflection',
              reflectionResult: reflData as ReflectionEvent
            })
          }
        } catch {
          // ignore parse errors
        }
        scrollToBottom()
        break

      case 'correction':
        try {
          const corrData = typeof event.content === 'string' ? JSON.parse(event.content) : event.result
          if (corrData) {
            correctionResults.value.push(corrData as CorrectionEvent)
            messages.value.push({
              role: 'assistant',
              content: '',
              type: 'correction',
              correctionResult: corrData as CorrectionEvent
            })
          }
        } catch {
          // ignore parse errors
        }
        scrollToBottom()
        break

      case 'done':
        cleanup()
        flushThought(true)
        currentEventSource.value?.close()
        currentEventSource.value = null
        if (!applyStructuredFinalAnswer() && structuredFinalCandidate) {
          upsertFinalAssistantMessage(finalAnswerContent.value)
        }
        applyParsedExecutionResultFromContent()
        if (sessionId.value) {
          void loadExecutionResultForSession(sessionId.value, true)
        }
        isLoading.value = false
        scrollToBottom()
        break

      case 'error':
        const normalizedError = normalizeAIAnalysisErrorMessage(event.content || 'AI 分析出错')
        ElMessage.error(normalizedError)
        cleanup()
        flushThought(true)
        currentEventSource.value?.close()
        currentEventSource.value = null
        currentThought = ''
        thoughtMsgIndex = -1
        messages.value.push({
          role: 'assistant',
          content: `AI 分析失败: ${normalizedError}`,
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

  // Close previous EventSource if exists
  if (currentEventSource.value) {
    currentEventSource.value.close()
    currentEventSource.value = null
  }

  messages.value.push({
    role: 'user',
    content: message
  })
  finalAnswerContent.value = ''
  attackGraph.value = null
  generatedFlowchartImageUrl.value = ''
  executionResult.value = null

  scrollToBottom()
  isLoading.value = true
  currentEventSource.value = createSSEHandler(message)
}

function sendMessage() {
  if (!inputMessage.value.trim() || !sessionId.value || isLoading.value) return

  // Close previous EventSource if exists
  if (currentEventSource.value) {
    currentEventSource.value.close()
    currentEventSource.value = null
  }

  const userMessage = inputMessage.value.trim()
  inputMessage.value = ''

  messages.value.push({
    role: 'user',
    content: userMessage
  })
  finalAnswerContent.value = ''
  attackGraph.value = null
  generatedFlowchartImageUrl.value = ''
  executionResult.value = null

  scrollToBottom()
  isLoading.value = true
  currentEventSource.value = createSSEHandler(userMessage)
}

function closeCurrentStreamSilently() {
  if (!currentEventSource.value) return
  currentEventSource.value.onmessage = null
  currentEventSource.value.onerror = null
  currentEventSource.value.close()
  currentEventSource.value = null
}

async function pauseAnalysis() {
  if (!sessionId.value) return
  try {
    await pauseSession(sessionId.value)
    ElMessage.success('AI 分析已暂停')
  } catch (error: any) {
    ElMessage.error(error.message || '暂停 AI 分析失败')
  } finally {
    closeCurrentStreamSilently()
    isLoading.value = false
  }
}

async function handleEnterKey(e: KeyboardEvent) {
  if (isLoading.value) {
    // During analysis, Ctrl+Enter pauses current analysis and re-sends with new input
    e.preventDefault()
    await pauseAnalysis()
    nextTick(() => {
      if (inputMessage.value.trim()) {
        sendMessage()
      }
    })
  } else {
    sendMessage()
  }
}

function scrollToBottom() {
  nextTick(() => {
    if (messageListRef.value) {
      messageListRef.value.scrollTop = messageListRef.value.scrollHeight
    }
  })
}

function downloadFlowchartImage() {
  if (!attackGraph.value) return

  const link = document.createElement('a')
  link.href = buildAttackGraphSvgDataUrl(attackGraph.value)
  link.download = `${attackGraph.value.graphId || 'attack-graph'}.svg`
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

function closeFlowchart() {
  attackGraph.value = null
  generatedFlowchartImageUrl.value = ''
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

// Auto-save conversation when messages change
watch(messages, () => {
  saveConversation()
}, { deep: true })

// Cleanup on unmount
onBeforeUnmount(() => {
  if (loadAlertsTimer) clearTimeout(loadAlertsTimer)
  clearSavedConversation()
})

// Prune selection only when filters change, not on page change
// The watch on [hostFilter, timeRange] above handles page reset + reload

let loadAlertsTimer: ReturnType<typeof setTimeout> | null = null
watch([hostFilter, timeRange], () => {
  if (loadAlertsTimer) clearTimeout(loadAlertsTimer)
  loadAlertsTimer = setTimeout(() => loadAlerts(), 300)
})

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

  // Parallel load hosts and alerts for better performance
  Promise.all([
    loadHosts(),
    loadAlerts(Boolean(alertIdsParam))
  ]).then(() => {
    // If alert_ids are provided, select them
    if (alertIdsParam) {
      const ids = alertIdsParam.split(',')
      selectedAlertIds.value = ids

      // Auto-start analysis with selected alerts
      nextTick(() => {
        startAnalysis()
      })
    } else {
      // Try to restore session from localStorage after page reload
      const savedId = loadCurrentSessionId()
      if (savedId) {
        savedSessionId.value = savedId
        sessionId.value = savedId
        if (loadConversation()) {
          nextTick(() => {
            ElMessage.success('已恢复之前的会话')
          })
        }
      }
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

.alert-selection-card {
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

.alert-selection-card :deep(.el-card__header) {
  flex: 0 0 auto;
}

.alert-selection-card :deep(.el-card__body) {
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  min-height: 0;
  overflow: hidden;
}

.alert-selection-scroll {
  flex: 1 1 auto;
  min-height: 0;
  padding-right: 4px;
  overflow-x: hidden;
  overflow-y: scroll;
  scrollbar-gutter: stable;
}

.alert-selection-scroll::-webkit-scrollbar {
  width: 8px;
}

.alert-selection-scroll::-webkit-scrollbar-thumb {
  border-radius: 999px;
  background: rgba(100, 116, 139, 0.28);
}

.alert-selection-scroll::-webkit-scrollbar-track {
  background: transparent;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.filter-form {
  margin-bottom: 16px;
}

.analysis-snapshot-hint {
  margin-bottom: 12px;
  padding: 10px 12px;
  border: 1px solid rgba(37, 99, 235, 0.18);
  border-radius: 10px;
  background: rgba(37, 99, 235, 0.07);
  color: #1d4ed8;
  font-size: 13px;
  line-height: 1.5;
}

.alert-pagination {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid rgba(15, 23, 42, 0.08);
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  justify-items: center;
  gap: 10px;
  max-width: 100%;
}

.alert-pagination-summary {
  display: inline-flex;
  max-width: 100%;
  align-items: center;
  justify-content: center;
  gap: 12px;
  min-width: 0;
}

.alert-pagination-total {
  flex: 0 0 auto;
  color: var(--aegis-text-muted);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
  line-height: 32px;
  white-space: nowrap;
}

.alert-page-size-select {
  width: 104px;
  flex: 0 0 auto;
}

.alert-page-size-select :deep(.el-select__wrapper) {
  min-height: 32px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.92);
  box-shadow: 0 0 0 1px rgba(15, 23, 42, 0.1) inset;
}

.alert-pagination-pager {
  --el-pagination-button-width: 28px;
  --el-pagination-button-height: 32px;
  --el-pagination-button-width-small: 28px;
  --el-pagination-button-height-small: 32px;
  --el-pagination-item-gap: 0;
  display: inline-flex;
  max-width: 100%;
  justify-content: center;
  margin-left: 0;
  overflow: hidden;
}

.alert-pagination-pager :deep(.el-pager) {
  display: flex;
  min-width: 0;
}

.alert-pagination-pager :deep(.btn-prev),
.alert-pagination-pager :deep(.btn-next),
.alert-pagination-pager :deep(.el-pager li) {
  min-width: 28px;
  width: 28px;
  height: 32px;
  margin: 0 1px;
  padding: 0;
  border-radius: 8px;
  font-weight: 650;
}

.alert-pagination-pager :deep(.el-pager li.is-active) {
  box-shadow: 0 8px 18px rgba(37, 99, 235, 0.22);
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
  max-width: 100%;
  min-width: 0;
  flex: 1;
  padding: 12px 16px;
  border-radius: 8px;
  word-break: break-word;
  overflow: visible;
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
  overflow: visible;
  display: block;
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
  font-family: var(--aegis-font-sans);
}

/* Thought block - 独立显示AI思考过程 */
.thought-block {
  background: linear-gradient(135deg, #f0f9ff 0%, #e0f2fe 100%);
  border: 1px solid #0ea5e9;
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 12px;
  max-height: clamp(180px, 42vh, 520px);
  overflow-y: auto;
  transition: max-height 0.3s ease-out;
}

.thought-block .block-header {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #0ea5e9;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 8px;
}

.thought-content {
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 13px;
  color: #1e293b;
  line-height: 1.6;
  margin: 0;
}

.thought-text {
  background: white;
  border-radius: 4px;
  padding: 8px 12px;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 13px;
  color: #1e293b;
  line-height: 1.6;
  max-height: clamp(120px, 34vh, 420px);
  overflow-y: auto;
}

/* Action block - 独立显示工具调用 */
.action-block {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  border: 1px solid #f59e0b;
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 12px;
}

.action-block .block-header {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #b45309;
  font-size: 13px;
  font-weight: 600;
}

/* Action Input block - 独立显示工具参数 */
.action-input-block {
  background: linear-gradient(135deg, #f3e8ff 0%, #e9d5ff 100%);
  border: 1px solid #a855f7;
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 12px;
}

.action-input-block .block-header {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #7e22ce;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 8px;
}

.action-input-block .block-content {
  background: white;
  border-radius: 4px;
  padding: 8px 12px;
}

.action-input-block pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
  color: #1e293b;
  max-height: clamp(96px, 26vh, 260px);
  overflow-y: auto;
}

/* Observation block - 独立显示工具执行结果 */
.observation-block {
  background: linear-gradient(135deg, #f0fdf4 0%, #dcfce7 100%);
  border: 1px solid #22c55e;
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 12px;
}

.observation-block .block-header {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #15803d;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 8px;
}

.observation-block .block-content {
  background: white;
  border-radius: 4px;
  padding: 8px 12px;
}

.observation-block pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
  color: #1e293b;
  max-height: clamp(120px, 36vh, 360px);
  overflow-y: auto;
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
  word-break: break-word;
}

.final-execution-result {
  width: 100%;
}

.final-execution-result :deep(.task-execution-result) {
  padding: 0;
}

.execution-result-panel {
  margin-top: 16px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  background: var(--el-bg-color);
}

.execution-result-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--el-border-color-extra-light);
  font-weight: 600;
}

.final-content pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
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

/* Audit block - purple tone */
.audit-block {
  background: linear-gradient(135deg, #f5f3ff 0%, #ede9fe 100%);
  border: 1px solid #8b5cf6;
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 12px;
}

.audit-block .block-header {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #7c3aed;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 8px;
}

.audit-block .block-content {
  background: white;
  border-radius: 4px;
  padding: 8px 12px;
  font-size: 13px;
  line-height: 1.6;
}

.audit-block .block-content ul {
  margin: 4px 0 0 0;
  padding-left: 18px;
}

/* Reflection block - orange tone */
.reflection-block {
  background: linear-gradient(135deg, #fff7ed 0%, #ffedd5 100%);
  border: 1px solid #f97316;
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 12px;
}

.reflection-block .block-header {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #c2410c;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 8px;
}

.reflection-block .block-content {
  background: white;
  border-radius: 4px;
  padding: 8px 12px;
  font-size: 13px;
  line-height: 1.6;
}

/* Correction block - blue tone */
.correction-block {
  background: linear-gradient(135deg, #eff6ff 0%, #dbeafe 100%);
  border: 1px solid #3b82f6;
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 12px;
}

.correction-block .block-header {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #1d4ed8;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 8px;
}

.correction-block .block-content {
  background: white;
  border-radius: 4px;
  padding: 8px 12px;
  font-size: 13px;
  line-height: 1.6;
}

.correction-block .block-content ul {
  margin: 4px 0 0 0;
  padding-left: 18px;
}
</style>
