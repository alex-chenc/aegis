<template>
  <div ref="workspaceRoot" class="assistant-workspace">
    <!-- 左侧会话栏 -->
    <AssistantSessionSidebar
      :sessions="sessions"
      :active-session-id="currentSession?.session_id"
      :loading="loading"
      :loading-more="store.loadingMore"
      :has-more="store.hasMoreSessions"
      :total="store.sessionTotal"
      :current-page="currentPage"
      @select="handleSessionSelected"
      @create="handleNewSession"
      @search="handleSearch"
      @load-more="handleLoadMore"
      @page-change="handlePageChange"
      @delete="handleDeleteSession"
    />

    <!-- 中间对话区 -->
    <div class="conversation-panel">
      <!-- 有会话时显示对话 -->
      <template v-if="currentSession">
        <!-- 对话头部 -->
        <div class="conversation-header">
          <div class="header-left">
            <h3>{{ currentSession.title }}</h3>
            <el-tag :type="getStatusType(currentSession.status)" size="small">
              {{ getStatusLabel(currentSession.status) }}
            </el-tag>
          </div>
          <div class="header-right">
            <div class="runtime-metrics" :aria-label="$t('generated.assistantAssistantWorkspace_agent_operating_indicators_9b1457')">
              <el-tag size="small" effect="plain">
                {{ $t('generated.common_maximum_number_of_rounds_7b621d') }} {{ maxTurnsLabel }}
              </el-tag>
              <el-tag size="small" effect="plain" type="info">
                Tokens {{ tokenUsageLabel }}
              </el-tag>
              <ContextBudgetIndicator
                :budget="contextBudget"
                :compression-records="compressionRecords"
                :total-prompt-tokens="totalPromptTokens"
                :total-completion-tokens="totalCompletionTokens"
              />
            </div>
            <el-button
              v-if="streaming"
              type="danger"
              size="small"
              @click="cancelCurrentRun"
            >
              <el-icon><VideoPause /></el-icon>
              {{ $t('generated.assistantAssistantWorkspace_cancel_run_644b4a') }}
            </el-button>
          </div>
        </div>

        <!-- 消息流 -->
        <AssistantConversation
          :messages="messages"
          :tool-calls="toolCalls"
          :approvals="approvals"
          :result-cards="resultCards"
          :streaming="streaming"
          @approve="handleApprove"
          @reject="handleReject"
        />

        <!-- 输入框在底部 -->
        <AssistantComposer
          :disabled="streaming"
          :approval-mode="approvalMode"
          :mode-loading="approvalModeLoading"
          :uploading="uploadingFile"
          :upload-items="uploadItems"
          @send="handleSend"
          @approval-mode-change="handleApprovalModeChange"
          @upload-file="handleUploadFile"
          @remove-upload="handleRemoveUpload"
        />
      </template>

      <!-- 无会话时：输入框居中 -->
      <div v-else class="welcome-state">
        <div class="welcome-content">
          <el-icon class="welcome-icon"><MagicStick /></el-icon>
          <h2>{{ $t('generated.assistantAssistantWorkspace_intelligent_security_assistant_24ac1c') }}</h2>
          <p>{{ $t('generated.assistantAssistantWorkspace_complete_security_operations_tasks_through_natural_42cec3') }}</p>
          <div class="center-composer">
            <AssistantComposer
              :disabled="false"
              :approval-mode="approvalMode"
              :mode-loading="approvalModeLoading"
              :uploading="uploadingFile"
              :upload-items="uploadItems"
              @send="handleSend"
              @approval-mode-change="handleApprovalModeChange"
              @upload-file="handleUploadFile"
              @remove-upload="handleRemoveUpload"
            />
          </div>
        </div>
      </div>
    </div>

    <AssistantContextRail
      :plan="currentPlan"
      :approvals="pendingApprovals"
      :tool-calls="toolCalls"
    />
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { ElMessage } from 'element-plus'
import { MagicStick, VideoPause } from '@element-plus/icons-vue'
import { gsap } from 'gsap'
import { useAssistantStore } from '@/store/assistant'
import { getStoredAuth } from '@/utils/auth'
import { normalizePlanEvent } from '@/utils/aiAnalysisRuntime'
import {
  getToolApprovalPolicy,
  updateToolApprovalPolicy,
  type AssistantFileUploadPurpose,
  type AssistantToolApprovalMode,
} from '@/api/assistant'
import type { PlanEvent } from '@/api/aiAnalysis'
import AssistantSessionSidebar from './components/AssistantSessionSidebar.vue'
import AssistantConversation from './components/AssistantConversation.vue'
import AssistantComposer from './components/AssistantComposer.vue'
import AssistantContextRail from './components/AssistantContextRail.vue'
import ContextBudgetIndicator from '@/components/ContextBudgetIndicator.vue'

const route = useRoute()
const router = useRouter()
const store = useAssistantStore()
const workspaceRoot = ref<HTMLElement | null>(null)
let workspaceAnimation: ReturnType<typeof gsap.context> | null = null
let motionMedia: ReturnType<typeof gsap.matchMedia> | null = null
const approvalMode = ref<AssistantToolApprovalMode>('whitelist')
const approvalModeLoading = ref(false)
const uploadingFile = ref(false)
type UploadItemStatus = 'uploading' | 'success' | 'error'
type UploadItem = {
  id: string
  name: string
  purpose: AssistantFileUploadPurpose
  status: UploadItemStatus
  error?: string
}
const uploadItems = ref<UploadItem[]>([])

const {
  sessions,
  currentSession,
  messages,
  toolCalls,
  approvals,
  resultCards,
  contextBudget,
  compressionRecords,
  totalPromptTokens,
  totalCompletionTokens,
  streaming,
  loading,
} = storeToRefs(store)

const pendingApprovals = computed(() =>
  approvals.value.filter(a => a.status === 'pending')
)

const sessionMetadata = computed<Record<string, any>>(() => {
  const metadata = currentSession.value?.metadata
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
})

const maxTurnsLabel = computed(() => {
  const turns = Number(sessionMetadata.value.max_total_turns || 0)
  return turns > 0 ? String(turns) : '-'
})

const tokenUsageLabel = computed(() => {
  const metadataTotal = Number(sessionMetadata.value.total_tokens || 0)
  const total = totalPromptTokens.value + totalCompletionTokens.value || metadataTotal || contextBudget.value?.total_tokens || 0
  return total > 0 ? formatTokens(total) : '-'
})

// 从消息中提取最新的执行计划
const currentPlan = computed<PlanEvent | null>(() => {
  const candidates = messages.value.filter(msg => msg.role === 'assistant' && msg.plan)
  if (!candidates.length) return null

  const maxStepCount = Math.max(...candidates.map(msg => msg.plan?.steps?.length || 0))
  const bestCandidates = candidates.filter(msg => (msg.plan?.steps?.length || 0) === maxStepCount)
  const msg = bestCandidates[bestCandidates.length - 1]
  if (!msg.plan) return null

  return normalizePlanEvent({
    id: `plan-${msg.id || msg.message_id}`,
    plan_id: `plan-${msg.id || msg.message_id}`,
    ...msg.plan,
  })
})

// 选择会话
async function handleSessionSelected(sessionId: string) {
  await store.openSession(sessionId)
  router.replace({ query: { ...route.query, session: sessionId } })
}

// 新建会话
function handleNewSession() {
  store.setPendingTaskType('explanation')
  router.replace({ query: { ...route.query, session: undefined } })
}

// 发送消息
async function handleSend(content: string) {
  try {
    const refs = []
    if (route.query.context_type && route.query.context_id) {
      refs.push({
        object_type: route.query.context_type as string,
        object_id: route.query.context_id as string,
      })
    }
    await store.sendMessage(content, refs.length > 0 ? refs : undefined)
    // 创建会话后更新 URL
    if (currentSession.value?.session_id) {
      router.replace({ query: { ...route.query, session: currentSession.value.session_id } })
    }
  } catch (err) {
    console.error(translate('generatedScript.assistantAssistantWorkspace_failed_to_send_message_6340f8'), err)
  }
}

async function fetchApprovalMode() {
  try {
    const policy = await getToolApprovalPolicy()
    approvalMode.value = policy?.mode || 'whitelist'
  } catch {
    approvalMode.value = 'whitelist'
  }
}

async function handleApprovalModeChange(mode: AssistantToolApprovalMode) {
  const previous = approvalMode.value
  approvalMode.value = mode
  approvalModeLoading.value = true
  try {
    await updateToolApprovalPolicy({ mode })
    ElMessage.success(translate('generatedScript.assistantAssistantWorkspace_tool_permissions_mode_updated_564b52'))
  } catch (err: any) {
    approvalMode.value = previous
    ElMessage.error(err?.message || translate('generatedScript.assistantAssistantWorkspace_permission_mode_update_failed_52a5bc'))
  } finally {
    approvalModeLoading.value = false
  }
}

async function ensureSessionForUpload(file: File, purpose: AssistantFileUploadPurpose) {
  if (currentSession.value) return currentSession.value
  const titlePrefix: Record<AssistantFileUploadPurpose, string> = {
    analysis: translate('generatedScript.assistantAssistantWorkspace_file_analysis_1458ef'),
    baseline_template: translate('generatedScript.assistantAssistantWorkspace_baseline_template_169b50'),
    sigma_rule: translate('generatedScript.common_sigma_rules_80c495'),
  }
  const session = await store.createSession({
    title: `${titlePrefix[purpose]}：${file.name}`.slice(0, 48),
    task_type: purpose === 'analysis' ? 'explanation' : 'operations',
  })
  if (!session) {
    throw new Error(translate('generatedScript.assistantAssistantWorkspace_failed_to_create_session_4335b9'))
  }
  router.replace({ query: { ...route.query, session: session.session_id } })
  return session
}

async function handleUploadFile(file: File, purpose: AssistantFileUploadPurpose) {
  const item: UploadItem = {
    id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
    name: file.name,
    purpose,
    status: 'uploading',
  }
  uploadItems.value.push(item)
  uploadingFile.value = true
  try {
    await ensureSessionForUpload(file, purpose)
    const result = await store.uploadSessionFile(file, purpose)
    const title = result.context_ref?.title || file.name
    item.name = title
    item.status = 'success'
    ElMessage.success(translate('generatedScript.assistantAssistantWorkspace_uploaded_7132e9', { p0: title }))
  } catch (err: any) {
    item.status = 'error'
    item.error = err?.message || translate('generatedScript.assistantAssistantWorkspace_file_upload_failed_e398bb')
    ElMessage.error(item.error)
  } finally {
    uploadingFile.value = false
  }
}

function handleRemoveUpload(id: string) {
  uploadItems.value = uploadItems.value.filter(item => item.id !== id)
}

// 取消运行
async function cancelCurrentRun() {
  try {
    await store.cancelCurrentRun()
  } catch (err) {
    console.error(translate('generatedScript.assistantAssistantWorkspace_cancel_run_failed_b8f64b'), err)
  }
}

// 审批操作
async function handleApprove(approvalId: string, comment?: string) {
  try {
    await store.approveAction(approvalId, comment)
  } catch (err) {
    console.error(translate('generatedScript.assistantAssistantWorkspace_approval_failed_2e5d48'), err)
  }
}

async function handleReject(approvalId: string, comment?: string) {
  try {
    await store.rejectAction(approvalId, comment)
  } catch (err) {
    console.error(translate('generatedScript.assistantAssistantWorkspace_reject_failed_8acfe9'), err)
  }
}

// 搜索
function handleSearch(keyword: string) {
  currentPage.value = 1
  store.fetchSessions({ keyword })
}

// 加载更多会话
function handleLoadMore() {
  store.fetchSessions(undefined, true)
}

// 分页
const currentPage = ref(1)

async function handlePageChange(page: number) {
  currentPage.value = page
  await store.goToSessionPage(page)
}

// 删除会话
async function handleDeleteSession(sessionId: string) {
  const wasCurrentSession = currentSession.value?.session_id === sessionId
  await store.deleteSession(sessionId)

  if (wasCurrentSession) {
    if (sessions.value.length > 0) {
      const nextSession = sessions.value[0]
      await store.openSession(nextSession.session_id)
      router.replace({ query: { ...route.query, session: nextSession.session_id } })
    } else {
      router.replace({ query: {} })
    }
  }
}

// 状态标签
function getStatusType(status: string): string {
  const map: Record<string, string> = {
    active: 'info',
    running: 'warning',
    waiting_approval: 'warning',
    completed: 'success',
    cancelled: 'info',
    failed: 'danger',
  }
  return map[status] || 'info'
}

function getStatusLabel(status: string): string {
  const map: Record<string, string> = {
    active: translate('generatedScript.common_active_8c0daf'),
    running: translate('generatedScript.common_running_594249'),
    waiting_approval: translate('generatedScript.common_pending_approval_57fce0'),
    completed: translate('generatedScript.common_completed_e99b48'),
    cancelled: translate('generatedScript.common_canceled_a5ffdc'),
    failed: translate('generatedScript.common_fail_3e3c80'),
  }
  return map[status] || status
}

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

function setupWorkspaceMotion() {
  if (!workspaceRoot.value || typeof window === 'undefined') return

  workspaceAnimation = gsap.context(() => {
    const selectTargets = () => {
      const q = gsap.utils.selector(workspaceRoot.value)
      return {
        sidebar: q('.session-sidebar'),
        panel: q('.conversation-panel'),
        rail: q('.context-rail'),
        welcomeItems: q('.welcome-content > *'),
      }
    }

    const runEntranceMotion = () => {
      const targets = selectTargets()
      const timeline = gsap.timeline({
        defaults: { duration: 0.42, ease: 'power2.out' },
      })

      if (targets.sidebar.length) {
        timeline.from(targets.sidebar, { autoAlpha: 0, x: -18 })
      }
      if (targets.panel.length) {
        timeline.from(targets.panel, { autoAlpha: 0, y: 12 }, '<0.08')
      }
      if (targets.rail.length) {
        timeline.from(targets.rail, { autoAlpha: 0, x: 18 }, '<')
      }
      if (targets.welcomeItems.length) {
        timeline.from(targets.welcomeItems, {
          autoAlpha: 0,
          y: 14,
          stagger: 0.06,
          duration: 0.36,
        }, '<0.05')
      }
    }

    if (typeof window.matchMedia !== 'function') {
      runEntranceMotion()
      return
    }

    motionMedia = gsap.matchMedia()
    motionMedia.add('(prefers-reduced-motion: reduce)', () => {
      const targets = selectTargets()
      const elements = [
        ...targets.sidebar,
        ...targets.panel,
        ...targets.rail,
        ...targets.welcomeItems,
      ]
      if (elements.length) {
        gsap.set(elements, { clearProps: 'all' })
      }
    })
    motionMedia.add('(prefers-reduced-motion: no-preference)', () => {
      runEntranceMotion()
    })
  }, workspaceRoot.value)
}

onMounted(async () => {
  const auth = getStoredAuth()
  if (!auth) {
    ElMessage.warning(translate('generatedScript.assistantAssistantWorkspace_please_log_in_to_the_system_05ff36'))
    router.replace('/login')
    return
  }

  setupWorkspaceMotion()

  await fetchApprovalMode()

  try {
    await store.fetchSessions()
  } catch (err) {
    console.error(translate('generatedScript.assistantAssistantWorkspace_failed_to_load_session_list_d0bf46'), err)
  }

  const sessionId = route.query.session as string
  if (sessionId) {
    const exists = sessions.value.some(s => s.session_id === sessionId)
    if (exists) {
      try {
        await store.openSession(sessionId)
      } catch (err) {
        console.error(translate('generatedScript.assistantAssistantWorkspace_failed_to_restore_session_1ab5c0'), err)
        router.replace({ query: {} })
      }
    } else {
      router.replace({ query: {} })
    }
  } else if (sessions.value.length > 0) {
    const latestSession = sessions.value[0]
    if (latestSession?.session_id) {
      await store.openSession(latestSession.session_id)
      router.replace({ query: { session: latestSession.session_id } })
    }
  }
})

onUnmounted(() => {
  motionMedia?.revert()
  workspaceAnimation?.revert()
  motionMedia = null
  workspaceAnimation = null
})
</script>

<style scoped>
.assistant-workspace {
  display: flex;
  height: 100%;
  min-height: 0;
  background: #f5f7fa;
  overflow: hidden;
}

.conversation-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  background: #fff;
  overflow: hidden;
  will-change: transform, opacity;
}

.conversation-panel > :deep(.composer) {
  flex-shrink: 0;
  margin: 12px 20px 16px;
}

.conversation-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 20px;
  border-bottom: 1px solid #e4e7ed;
  background: #fff;
  flex-shrink: 0;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-left h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.runtime-metrics {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

/* 欢迎页面：输入框居中 */
.welcome-state {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #f5f7fa 0%, #e8eaed 100%);
}

.welcome-content {
  text-align: center;
  width: 100%;
  max-width: 680px;
  padding: 0 20px;
  will-change: transform, opacity;
}

.welcome-icon {
  font-size: 56px;
  color: #409eff;
  margin-bottom: 16px;
}

.welcome-content h2 {
  margin: 0 0 8px;
  font-size: 24px;
  font-weight: 600;
  color: #303133;
}

.welcome-content p {
  margin: 0 0 32px;
  color: #909399;
  font-size: 14px;
}

.center-composer {
  width: 100%;
}

.center-composer :deep(.composer) {
  margin: 0;
}
</style>
