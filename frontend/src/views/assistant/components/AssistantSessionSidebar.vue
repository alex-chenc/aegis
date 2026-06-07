<template>
  <aside class="session-sidebar">
    <!-- 头部 -->
    <div class="sidebar-header">
      <h3>智能助手</h3>
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

    <!-- 会话列表 -->
    <div class="session-list">
      <div class="section-title">历史会话</div>
      <div v-if="loading" class="loading-state">
        <el-skeleton :rows="3" animated />
      </div>
      <div v-else-if="sessions.length === 0" class="empty-state">
        <el-empty description="暂无会话" :image-size="48" />
      </div>
      <template v-else>
        <div class="session-items">
          <div
            v-for="session in sessions"
            :key="session.session_id"
            class="session-item"
            :class="{ active: activeSessionId === session.session_id }"
            @click="$emit('select', session.session_id)"
          >
            <div class="session-item-header">
              <div class="session-title">{{ session.title }}</div>
              <el-button
                class="delete-btn"
                type="danger"
                size="small"
                :icon="Delete"
                text
                @click.stop="handleDelete(session.session_id)"
              />
            </div>
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

        <!-- 分页（始终显示在会话列表底部） -->
        <div v-if="total > 10" class="session-pagination">
          <el-pagination
            small
            layout="prev, pager, next"
            :total="total"
            :page-size="10"
            :current-page="currentPage"
            @current-change="handlePageChange"
          />
        </div>
      </template>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Plus, Search, Delete } from '@element-plus/icons-vue'
import { ElMessageBox } from 'element-plus'
import type { AssistantSession } from '@/api/assistant'

defineProps<{
  sessions: AssistantSession[]
  activeSessionId?: string
  loading: boolean
  loadingMore?: boolean
  hasMore?: boolean
  total?: number
  currentPage?: number
}>()

const emit = defineEmits<{
  select: [sessionId: string]
  create: []
  search: [keyword: string]
  loadMore: []
  pageChange: [page: number]
  delete: [sessionId: string]
}>()

const searchKeyword = ref('')

function handleSearch() {
  emit('search', searchKeyword.value)
}

function handlePageChange(page: number) {
  emit('pageChange', page)
}

async function handleDelete(sessionId: string) {
  try {
    await ElMessageBox.confirm('确定删除该会话？删除后不可恢复。', '删除会话', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
    })
    emit('delete', sessionId)
  } catch {
    // 用户取消
  }
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
  will-change: transform, opacity;
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
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.section-title {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
  margin-bottom: 8px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
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

.session-item-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.session-title {
  flex: 1;
  font-size: 14px;
  font-weight: 500;
  margin-bottom: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.delete-btn {
  opacity: 0;
  transition: opacity 0.2s;
  flex-shrink: 0;
}

.session-item:hover .delete-btn {
  opacity: 1;
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

.session-pagination {
  display: flex;
  justify-content: center;
  padding: 12px 0 4px;
}

.session-pagination :deep(.el-pagination) {
  --el-pagination-bg-color: transparent;
  --el-pagination-text-color: rgba(255, 255, 255, 0.5);
  --el-pagination-button-color: rgba(255, 255, 255, 0.5);
  --el-pagination-hover-color: #409eff;
}

.session-pagination :deep(.el-pagination .btn-prev),
.session-pagination :deep(.el-pagination .btn-next),
.session-pagination :deep(.el-pager li) {
  background: transparent !important;
  color: rgba(255, 255, 255, 0.5);
  min-width: 24px;
  height: 24px;
  line-height: 24px;
}

.session-pagination :deep(.el-pager li.is-active) {
  color: #409eff;
}
</style>
