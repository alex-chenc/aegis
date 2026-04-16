<template>
  <div class="notification-item" :class="{ 'is-unread': !notification.is_read }">
    <div class="notification-header">
      <el-tag :type="severityType" size="small" class="severity-tag">
        {{ severityLabel }}
      </el-tag>
      <span class="notification-time">{{ timeAgo }}</span>
    </div>
    <div class="notification-title" :title="notification.title">
      {{ notification.title }}
    </div>
    <el-tooltip :content="notification.content" placement="top" :disabled="!isContentTruncated">
      <div class="notification-content" :class="{ 'is-truncated': isContentTruncated }">
        {{ notification.content }}
      </div>
    </el-tooltip>
    <div v-if="notification.link" class="notification-actions">
      <el-button link type="primary" @click="handleLink">
        前往查看
        <el-icon class="el-icon--right"><ArrowRight /></el-icon>
      </el-button>
    </div>
    <div v-if="!notification.is_read" class="notification-mark-read">
      <el-button link type="primary" size="small" @click="handleMarkRead">
        标为已读
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ArrowRight } from '@element-plus/icons-vue'
import type { Notification } from '@/types/notification'

const props = defineProps<{
  notification: Notification
}>()

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
    critical: '严重',
    high: '高危',
    medium: '中危',
    low: '低危',
    info: '通知'
  }
  return map[props.notification.severity] || '通知'
})

// 相对时间
const timeAgo = computed(() => {
  const now = new Date()
  const timestamp = new Date(props.notification.timestamp)
  const diff = Math.floor((now.getTime() - timestamp.getTime()) / 1000)

  if (diff < 60) return '刚刚'
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`
  if (diff < 604800) return `${Math.floor(diff / 86400)} 天前`
  return timestamp.toLocaleDateString('zh-CN')
})

// 内容是否被截断
const isContentTruncated = computed(() => {
  return props.notification.content.length > 100
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
}

.notification-mark-read {
  position: absolute;
  bottom: 12px;
  right: 12px;
}
</style>