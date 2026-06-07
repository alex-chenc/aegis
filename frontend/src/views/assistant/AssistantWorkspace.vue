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
            <div class="runtime-metrics" aria-label="智能体运行指标">
              <el-tag size="small" effect="plain">
                最大轮数 {{ maxTurnsLabel }}
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
              取消运行
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
          @send="handleSend"
        />
      </template>

      <!-- 无会话时：输入框居中 -->
      <div v-else class="welcome-state">
        <div class="welcome-content">
          <el-icon class="welcome-icon"><MagicStick /></el-icon>
          <h2>智能安全助手</h2>
          <p>通过自然语言完成安全运营任务</p>
          <div class="center-composer">
            <AssistantComposer
              :disabled="false"
              @send="handleSend"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- 右侧上下文栏 -->
    <AssistantContextRail
      :plan="currentPlan"
      :approvals="pendingApprovals"
      :tool-calls="toolCalls"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { ElMessage } from 'element-plus'
import { MagicStick, VideoPause } from '@element-plus/icons-vue'
import { gsap } from 'gsap'
import { useAssistantStore } from '@/store/assistant'
import { getStoredAuth } from '@/utils/auth'
import { normalizePlanEvent } from '@/utils/aiAnalysisRuntime'
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

const {
  sessions,
  currentSession,
  messages,
  contextRefs,
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
  // 从最新的助手消息中查找包含 plan 的消息
  for (let i = messages.value.length - 1; i >= 0; i--) {
    const msg = messages.value[i]
    if (msg.role === 'assistant' && msg.plan) {
      return normalizePlanEvent({
        id: `plan-${msg.id || msg.message_id}`,
        plan_id: `plan-${msg.id || msg.message_id}`,
        ...msg.plan,
      })
    }
  }
  return null
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
    console.error('发送消息失败:', err)
  }
}

// 取消运行
async function cancelCurrentRun() {
  try {
    await store.cancelCurrentRun()
  } catch (err) {
    console.error('取消运行失败:', err)
  }
}

// 审批操作
async function handleApprove(approvalId: string, comment?: string) {
  try {
    await store.approveAction(approvalId, comment)
  } catch (err) {
    console.error('审批失败:', err)
  }
}

async function handleReject(approvalId: string, comment?: string) {
  try {
    await store.rejectAction(approvalId, comment)
  } catch (err) {
    console.error('拒绝失败:', err)
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
    active: '活跃',
    running: '运行中',
    waiting_approval: '待审批',
    completed: '已完成',
    cancelled: '已取消',
    failed: '失败',
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
    ElMessage.warning('请先登录系统')
    router.replace('/login')
    return
  }

  setupWorkspaceMotion()

  try {
    await store.fetchSessions()
  } catch (err) {
    console.error('加载会话列表失败:', err)
  }

  const sessionId = route.query.session as string
  if (sessionId) {
    const exists = sessions.value.some(s => s.session_id === sessionId)
    if (exists) {
      try {
        await store.openSession(sessionId)
      } catch (err) {
        console.error('恢复会话失败:', err)
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
