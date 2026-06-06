<template>
  <div class="assistant-workspace">
    <!-- 左侧会话栏 -->
    <AssistantSessionSidebar
      :sessions="sessions"
      :active-session-id="currentSession?.session_id"
      :loading="loading"
      :loading-more="store.loadingMore.value"
      :has-more="store.hasMoreSessions.value"
      :total="store.sessionTotal.value"
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
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { ElMessage } from 'element-plus'
import { MagicStick, VideoPause } from '@element-plus/icons-vue'
import { useAssistantStore } from '@/store/assistant'
import { getStoredAuth } from '@/utils/auth'
import type { AssistantPlan } from '@/api/assistant'
import AssistantSessionSidebar from './components/AssistantSessionSidebar.vue'
import AssistantConversation from './components/AssistantConversation.vue'
import AssistantComposer from './components/AssistantComposer.vue'
import AssistantContextRail from './components/AssistantContextRail.vue'

const route = useRoute()
const router = useRouter()
const store = useAssistantStore()

const {
  sessions,
  currentSession,
  messages,
  contextRefs,
  toolCalls,
  approvals,
  resultCards,
  streaming,
  loading,
} = storeToRefs(store)

const pendingApprovals = computed(() =>
  approvals.value.filter(a => a.status === 'pending')
)

// 从消息中提取最新的执行计划
const currentPlan = computed<AssistantPlan | null>(() => {
  // 从最新的助手消息中查找包含 plan 的消息
  for (let i = messages.value.length - 1; i >= 0; i--) {
    const msg = messages.value[i]
    if (msg.role === 'assistant' && msg.plan) {
      return {
        plan_id: `plan-${msg.id}`,
        goal: msg.plan.goal,
        status: (msg.plan.status as AssistantPlan['status']) || 'running',
        steps: msg.plan.steps.map(s => ({
          step_id: s.step_id,
          title: s.title,
          objective: s.title,
          suggested_tools: [],
          status: (s.status as AssistantPlan['steps'][0]['status']) || 'pending',
          result_summary: s.result_summary,
        })),
      }
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
    if (store.currentSession.value?.session_id) {
      router.replace({ query: { ...route.query, session: store.currentSession.value.session_id } })
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
  const wasCurrentSession = store.currentSession.value?.session_id === sessionId
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

onMounted(async () => {
  const auth = getStoredAuth()
  if (!auth) {
    ElMessage.warning('请先登录系统')
    router.replace('/login')
    return
  }

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
</style>
