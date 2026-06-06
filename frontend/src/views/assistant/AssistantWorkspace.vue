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
      @select="handleSessionSelected"
      @create="handleNewSession"
      @search="handleSearch"
      @load-more="handleLoadMore"
    />

    <!-- 中间对话区 -->
    <div class="conversation-panel">
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

        <!-- 输入框 -->
        <AssistantComposer
          :disabled="streaming"
          @send="handleSend"
        />
      </template>

      <!-- 空状态 -->
      <div v-else class="empty-state">
        <div class="empty-content">
          <el-icon class="empty-icon"><MagicStick /></el-icon>
          <h2>智能安全助手</h2>
          <p>通过自然语言完成安全运营任务</p>
          <div class="quick-actions">
            <el-button @click="handleNewSession('investigation')">
              <el-icon><Search /></el-icon>
              安全研判
            </el-button>
            <el-button @click="handleNewSession('operations')">
              <el-icon><Operation /></el-icon>
              运维操作
            </el-button>
            <el-button @click="handleNewSession('explanation')">
              <el-icon><ChatDotRound /></el-icon>
              自由提问
            </el-button>
          </div>
        </div>
      </div>
    </div>

    <!-- 右侧上下文栏 -->
    <AssistantContextRail
      :context-refs="contextRefs"
      :approvals="pendingApprovals"
      :tool-calls="toolCalls"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { ElMessage } from 'element-plus'
import { MagicStick, Search, Operation, ChatDotRound, VideoPause } from '@element-plus/icons-vue'
import { useAssistantStore } from '@/store/assistant'
import { getStoredAuth } from '@/utils/auth'
import AssistantSessionSidebar from './components/AssistantSessionSidebar.vue'
import AssistantConversation from './components/AssistantConversation.vue'
import AssistantComposer from './components/AssistantComposer.vue'
import AssistantContextRail from './components/AssistantContextRail.vue'
import type { AssistantTaskType } from '@/api/assistant'

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

// 选择会话（同步更新 URL 参数）
async function handleSessionSelected(sessionId: string) {
  await store.openSession(sessionId)
  router.replace({ query: { ...route.query, session: sessionId } })
}

// 新建会话
async function handleNewSession(taskType?: AssistantTaskType) {
  try {
    const refs = []
    if (route.query.context_type && route.query.context_id) {
      refs.push({
        object_type: route.query.context_type as string,
        object_id: route.query.context_id as string,
      })
    }
    const session = await store.createSession({
      task_type: taskType || 'explanation',
      initial_message: route.query.prompt as string,
      context_refs: refs.length > 0 ? refs : undefined,
    })
    // 创建成功后更新 URL
    if (session?.session_id) {
      router.replace({ query: { ...route.query, session: session.session_id } })
    }
  } catch (err) {
    console.error('创建会话失败:', err)
  }
}

// 发送消息
async function handleSend(content: string) {
  if (!currentSession.value) return
  try {
    await store.sendMessage(currentSession.value.session_id, content)
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
  // 检查是否已登录
  const auth = getStoredAuth()
  if (!auth) {
    ElMessage.warning('请先登录系统')
    router.replace('/login')
    return
  }

  // 加载会话列表
  try {
    await store.fetchSessions()
  } catch (err) {
    console.error('加载会话列表失败:', err)
  }

  // 从 URL 参数恢复会话
  const sessionId = route.query.session as string
  if (sessionId) {
    try {
      await store.openSession(sessionId)
    } catch (err) {
      console.error('恢复会话失败:', err)
      // 清除无效的 session 参数，避免重复尝试
      router.replace({ query: {} })
    }
  } else if (sessions.value.length > 0) {
    // 如果没有指定会话，自动选择最近的会话
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

.empty-state {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #f5f7fa 0%, #e4e7ed 100%);
}

.empty-content {
  text-align: center;
}

.empty-icon {
  font-size: 64px;
  color: #409eff;
  margin-bottom: 16px;
}

.empty-content h2 {
  margin: 0 0 8px;
  font-size: 24px;
  color: #303133;
}

.empty-content p {
  margin: 0 0 24px;
  color: #909399;
}

.quick-actions {
  display: flex;
  gap: 12px;
  justify-content: center;
}
</style>
