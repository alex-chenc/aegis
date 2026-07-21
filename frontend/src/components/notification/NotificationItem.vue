<template>
  <div class="notification-item" :class="{ 'is-unread': !notification.is_read }">
    <div class="notification-header">
      <el-tag :type="severityType" size="small" class="severity-tag">
        {{ severityLabel }}
      </el-tag>
      <span class="notification-time">{{ timeAgo }}</span>
    </div>
    <div class="notification-title" :title="localizedTitle">
      {{ localizedTitle }}
    </div>
    <el-tooltip :content="localizedContent" placement="top" :disabled="!isContentTruncated">
      <div class="notification-content" :class="{ 'is-truncated': isContentTruncated }">
        {{ localizedContent }}
      </div>
    </el-tooltip>
    <div class="notification-actions">
      <el-button v-if="notification.link" link type="primary" @click="handleLink">
        {{ t('notifications.view') }}
        <el-icon class="el-icon--right"><ArrowRight /></el-icon>
      </el-button>
      <el-button v-if="!notification.is_read" link type="primary" @click="handleMarkRead">
        {{ t('notifications.markRead') }}
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ArrowRight } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import type { Notification } from '@/types/notification'
import { formatRelativeTime } from '@/i18n/formatters'

const props = defineProps<{
  notification: Notification
}>()
const { t, te } = useI18n()

const emit = defineEmits<{
  (e: 'mark-read', id: string): void
}>()

// severity 标签颜色映射
const severityType = computed(() => {
  const map: Record<string, string> = {
    critical: 'danger',
    high: 'warning',
    medium: 'info',
    low: 'info',
    info: 'info'
  }
  return map[props.notification.severity] || 'info'
})

// severity 标签文字
const severityLabel = computed(() => {
  const map: Record<string, string> = {
    critical: 'common.severity.critical',
    high: 'common.severity.high',
    medium: 'common.severity.medium',
    low: 'common.severity.low',
    info: 'common.severity.info'
  }
  return t(map[props.notification.severity] || 'common.severity.info')
})

const timeAgo = computed(() => formatRelativeTime(props.notification.timestamp))

const localizedTitle = computed(() => localizeNotificationField('title', props.notification.title))
const localizedContent = computed(() => localizeNotificationField('content', props.notification.content))

function localizeNotificationField(field: 'title' | 'content', fallback: string): string {
  const baseKey = props.notification.metadata?.i18n_key
  if (!baseKey) return fallback
  const key = `${baseKey}.${field}`
  if (!te(key)) return fallback
  return t(key, props.notification.metadata?.i18n_params || {})
}

// 内容是否被截断
const isContentTruncated = computed(() => {
  return localizedContent.value.length > 100
})

function handleMarkRead() {
  emit('mark-read', props.notification.id)
}

function handleLink() {
  if (props.notification.link) {
    window.location.href = props.notification.link
  }
}
</script>

<style scoped>
.notification-item {
  padding: 12px;
  border-bottom: 1px solid #f0f0f0;
  position: relative;
}

.notification-item.is-unread {
  border-left: 3px solid #409EFF;
  padding-left: 10px;
}

.notification-item:hover {
  background-color: #fafafa;
}

.notification-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}

.severity-tag {
  margin-right: 8px;
}

.notification-time {
  font-size: 12px;
  color: #909399;
}

.notification-title {
  font-size: 14px;
  font-weight: 500;
  color: #303133;
  margin-bottom: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.notification-content {
  font-size: 13px;
  color: #606266;
  line-height: 1.5;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  word-break: break-all;
}

.notification-content.is-truncated {
  cursor: pointer;
}

.notification-actions {
  margin-top: 8px;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
