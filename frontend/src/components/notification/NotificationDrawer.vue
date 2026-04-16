<template>
  <el-drawer
    v-model="store.drawerVisible"
    title="消息通知"
    direction="rtl"
    size="400px"
    @close="handleClose"
  >
    <template #header>
      <div class="drawer-header">
        <span class="drawer-title">消息通知</span>
        <el-button
          link
          type="primary"
          :disabled="store.unreadCount === 0"
          @click="store.markAllAsRead"
        >
          全部标为已读
        </el-button>
      </div>
    </template>

    <!-- 未读/已读 Tab -->
    <el-tabs v-model="store.activeTab">
      <el-tab-pane label="未读" name="unread">
        <!-- 空状态 -->
        <el-empty v-if="store.unreadList.length === 0" description="暂无未读通知" />
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

      <el-tab-pane label="已读" name="read">
        <el-empty v-if="store.readList.length === 0" description="暂无已读通知" />
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
import NotificationItem from './NotificationItem.vue'

const store = useNotificationStore()

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