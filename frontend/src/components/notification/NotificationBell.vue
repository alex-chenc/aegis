<template>
  <!-- 铃铛按钮 + 未读 Badge -->
  <el-badge :value="store.badgeValue" :hidden="store.unreadCount === 0" class="notification-badge">
    <el-tooltip :content="t('notifications.title')" placement="bottom">
      <el-button
        :icon="Bell"
        circle
        size="small"
        @click="store.toggleDrawer"
      />
    </el-tooltip>
  </el-badge>

  <!-- 通知抽屉 -->
  <NotificationDrawer />
</template>

<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { Bell } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import { useNotificationStore } from '@/store/notification'
import NotificationDrawer from './NotificationDrawer.vue'

const store = useNotificationStore()
const { t } = useI18n()

// 组件挂载时启动轮询
onMounted(() => store.startPolling())
// 组件卸载时停止轮询
onUnmounted(() => store.stopPolling())
</script>

<style scoped>
.notification-badge :deep(.el-badge__content) {
  transform: translate(50%, -50%) translateY(-2px);
}
</style>
