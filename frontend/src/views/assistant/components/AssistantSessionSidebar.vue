<template>
  <aside class="session-sidebar">
    <!-- 头部 -->
    <div class="sidebar-header">
      <h3>{{ $t('generated.assistantAssistantSessionSidebar_smart_assistant_7c11c1') }}</h3>
      <el-button type="primary" size="small" @click="$emit('create')">
        <el-icon><Plus /></el-icon>
        {{ $t('generated.assistantAssistantSessionSidebar_new_session_db4436') }}
      </el-button>
    </div>

    <!-- 搜索 -->
    <div class="sidebar-search">
      <el-input
        v-model="searchKeyword"
        :placeholder="$t('generated.assistantAssistantSessionSidebar_search_conversation_4e3d01')"
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
      <div class="section-title">{{ $t('generated.common_history_session_60e77b') }}</div>
      <div v-if="loading" class="loading-state">
        <el-skeleton :rows="3" animated />
      </div>
      <div v-else-if="sessions.length === 0" class="empty-state">
        <el-empty :description="$t('generated.assistantAssistantSessionSidebar_no_session_yet_dc56f7')" :image-size="48" />
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
              <span>{{ session.message_count || 0 }} {{ $t('generated.assistantAssistantSessionSidebar_messages_3bee8e') }}</span>
              <span>{{ session.tool_call_count || 0 }} {{ $t('generated.assistantAssistantSessionSidebar_tool_calls_4624e9') }}</span>
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
import { translate } from '@/i18n'

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
    await ElMessageBox.confirm(translate('generatedScript.assistantAssistantSessionSidebar_are_you_sure_you_want_to_df23a6'), translate('generatedScript.assistantAssistantSessionSidebar_delete_session_19cf29'), {
      confirmButtonText: translate('generatedScript.common_delete_3755f5'),
      cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
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
    active: translate('generatedScript.common_active_8c0daf'),
    running: translate('generatedScript.common_running_594249'),
    waiting_approval: translate('generatedScript.common_pending_approval_57fce0'),
    completed: translate('generatedScript.common_completed_e99b48'),
    cancelled: translate('generatedScript.common_canceled_a5ffdc'),
    failed: translate('generatedScript.common_fail_3e3c80'),
  }
  return map[status] || status
}

function formatTime(time: string): string {
  if (!time) return ''
  const d = new Date(time)
  const now = new Date()
  const diff = now.getTime() - d.getTime()
  if (diff < 60000) return translate('generatedScript.assistantAssistantSessionSidebar_just_9e6366')
  if (diff < 3600000) return translate('generatedScript.assistantAssistantSessionSidebar_minutes_ago_cca145', { p0: Math.floor(diff / 60000) })
  if (diff < 86400000) return translate('generatedScript.assistantAssistantSessionSidebar_hours_ago_f43f26', { p0: Math.floor(diff / 3600000) })
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
