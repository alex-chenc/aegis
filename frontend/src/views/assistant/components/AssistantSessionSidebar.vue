<template>
  <aside class="session-sidebar">
    <!-- 头部 -->
    <div class="sidebar-header">
      <h3>智能模式</h3>
      <el-button type="primary" size="small" @click="$emit('create')">
        <el-icon><Plus /></el-icon>
        新会话
      </el-button>
    </div>

    <!-- 搜索 -->
    <div class="sidebar-search">
      <el-input
        v-model="searchKeyword"
        placeholder="搜索会话..."
        size="small"
        clearable
        @input="handleSearch"
      >
        <template #prefix>
          <el-icon><Search /></el-icon>
        </template>
      </el-input>
    </div>

    <!-- 快捷任务 -->
    <div class="quick-tasks">
      <div class="section-title">快捷任务</div>
      <div class="task-list">
        <div class="task-item" @click="$emit('create', 'investigation')">
          <el-icon><Search /></el-icon>
          <span>安全研判</span>
        </div>
        <div class="task-item" @click="$emit('create', 'operations')">
          <el-icon><Operation /></el-icon>
          <span>运维操作</span>
        </div>
        <div class="task-item" @click="$emit('create', 'explanation')">
          <el-icon><ChatDotRound /></el-icon>
          <span>自由提问</span>
        </div>
      </div>
    </div>

    <!-- 会话列表 -->
    <div class="session-list">
      <div class="section-title">历史会话</div>
      <div v-if="loading" class="loading-state">
        <el-skeleton :rows="3" animated />
      </div>
      <div v-else-if="sessions.length === 0" class="empty-state">
        <el-empty description="暂无会话" :image-size="48" />
      </div>
      <div v-else class="session-items">
        <div
          v-for="session in sessions"
          :key="session.session_id"
          class="session-item"
          :class="{ active: activeSessionId === session.session_id }"
          @click="$emit('select', session.session_id)"
        >
          <div class="session-title">{{ session.title }}</div>
          <div class="session-meta">
            <el-tag :type="getStatusType(session.status)" size="small">
              {{ getStatusLabel(session.status) }}
            </el-tag>
            <span class="session-time">{{ formatTime(session.created_at) }}</span>
          </div>
          <div class="session-stats">
            <span>{{ session.message_count || 0 }} 条消息</span>
            <span>{{ session.tool_call_count || 0 }} 次工具调用</span>
          </div>
        </div>
      </div>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Plus, Search, Operation, ChatDotRound } from '@element-plus/icons-vue'
import type { AssistantSession, AssistantTaskType } from '@/api/assistant'

defineProps<{
  sessions: AssistantSession[]
  activeSessionId?: string
  loading: boolean
}>()

const emit = defineEmits<{
  select: [sessionId: string]
  create: [taskType?: AssistantTaskType]
  search: [keyword: string]
}>()

const searchKeyword = ref('')

function handleSearch() {
  emit('search', searchKeyword.value)
}

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

function formatTime(time: string): string {
  if (!time) return ''
  const d = new Date(time)
  const now = new Date()
  const diff = now.getTime() - d.getTime()
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`
  return `${d.getMonth() + 1}/${d.getDate()} ${d.getHours()}:${String(d.getMinutes()).padStart(2, '0')}`
}
</script>

<style scoped>
.session-sidebar {
  width: 280px;
  background: #1e1e2d;
  color: #fff;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.sidebar-header h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}

.sidebar-search {
  padding: 12px 16px;
}

.quick-tasks {
  padding: 0 16px 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.section-title {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
  margin-bottom: 8px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.task-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.task-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.8);
  transition: background 0.2s;
}

.task-item:hover {
  background: rgba(255, 255, 255, 0.1);
}

.session-list {
  flex: 1;
  overflow-y: auto;
  padding: 12px 16px;
}

.session-items {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.session-item {
  padding: 12px;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.2s;
}

.session-item:hover {
  background: rgba(255, 255, 255, 0.05);
}

.session-item.active {
  background: rgba(64, 158, 255, 0.2);
}

.session-title {
  font-size: 14px;
  font-weight: 500;
  margin-bottom: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.session-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.session-time {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
}

.session-stats {
  display: flex;
  gap: 12px;
  font-size: 11px;
  color: rgba(255, 255, 255, 0.4);
}

.loading-state,
.empty-state {
  padding: 16px;
}
</style>
