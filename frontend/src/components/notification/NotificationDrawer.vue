<template>
  <el-drawer
    v-model="store.drawerVisible"
    :title="t('notifications.title')"
    direction="rtl"
    size="480px"
    :append-to-body="true"
    @close="handleClose"
  >
    <template #header>
      <div class="drawer-header">
        <span class="drawer-title">{{ t('notifications.title') }}</span>
        <el-button
          link
          type="primary"
          :disabled="store.unreadCount === 0"
          @click="store.markAllAsRead"
        >
          {{ t('notifications.markAllRead') }}
        </el-button>
      </div>
    </template>

    <!-- 未读/已读 Tab -->
    <el-tabs v-model="store.activeTab">
      <el-tab-pane :label="t('notifications.unread')" name="unread">
        <!-- 空状态 -->
        <el-empty v-if="store.unreadList.length === 0" :description="t('notifications.noUnread')" />
        <!-- 通知列表 -->
        <div v-else class="notification-list">
          <NotificationItem
            v-for="item in store.unreadList"
            :key="item.id"
            :notification="item"
            @mark-read="store.markAsRead"
          />
        </div>
      </el-tab-pane>

      <el-tab-pane :label="t('notifications.read')" name="read">
        <el-empty v-if="store.readList.length === 0" :description="t('notifications.noRead')" />
        <div v-else class="notification-list">
          <NotificationItem
            v-for="item in store.readList"
            :key="item.id"
            :notification="item"
          />
        </div>
      </el-tab-pane>
    </el-tabs>
  </el-drawer>
</template>

<script setup lang="ts">
import { useNotificationStore } from '@/store/notification'
import { useI18n } from 'vue-i18n'
import NotificationItem from './NotificationItem.vue'

const store = useNotificationStore()
const { t } = useI18n()

function handleClose() {
  // 抽屉关闭时不需要额外处理
}
</script>

<style scoped>
.drawer-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
}

.drawer-title {
  font-size: 16px;
  font-weight: 600;
}

.notification-list {
  max-height: calc(100vh - 200px);
  overflow-y: auto;
}
</style>
