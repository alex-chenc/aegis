import { defineStore } from 'pinia'
import { getNotifications, markNotificationRead, markAllNotificationsRead } from '@/api/notification'
import type { Notification } from '@/types/notification'
import { ElMessage } from 'element-plus'

interface NotificationState {
  notifications: Notification[]
  unreadCount: number
  drawerVisible: boolean
  activeTab: 'unread' | 'read'
  loading: boolean
  pollingTimer: ReturnType<typeof setInterval> | null
}

export const useNotificationStore = defineStore('notification', {
  state: (): NotificationState => ({
    notifications: [],
    unreadCount: 0,
    drawerVisible: false,
    activeTab: 'unread',
    loading: false,
    pollingTimer: null
  }),

  getters: {
    unreadList(): Notification[] {
      return this.notifications.filter(n => !n.is_read)
    },
    readList(): Notification[] {
      return this.notifications.filter(n => n.is_read)
    },
    badgeValue(): string {
      if (this.unreadCount === 0) return ''
      return this.unreadCount > 99 ? '99+' : String(this.unreadCount)
    }
  },

  actions: {
    // 拉取通知列表
    async fetchNotifications() {
      this.loading = true
      try {
        const res = await getNotifications({ page: 1, pageSize: 50 })
        this.notifications = res.list
        this.unreadCount = res.unread_count
      } catch (err) {
        console.error('Failed to fetch notifications', err)
      } finally {
        this.loading = false
      }
    },

    // 标记单条已读（乐观更新）
    async markAsRead(id: string) {
      const target = this.notifications.find(n => n.id === id)
      if (!target || target.is_read) return

      // 乐观更新本地状态
      const prevRead = target.is_read
      target.is_read = true
      this.unreadCount -= 1

      try {
        await markNotificationRead(id)
      } catch {
        // 回滚
        target.is_read = prevRead
        this.unreadCount += 1
        ElMessage.error('操作失败，请重试')
      }
    },

    // 全部标为已读
    async markAllAsRead() {
      const prevNotifications = this.notifications.map(n => ({ ...n }))
      const prevCount = this.unreadCount

      // 乐观更新
      this.notifications.forEach(n => { n.is_read = true })
      this.unreadCount = 0

      try {
        await markAllNotificationsRead()
      } catch {
        // 回滚
        this.notifications = prevNotifications
        this.unreadCount = prevCount
        ElMessage.error('操作失败，请重试')
      }
    },

    // 切换抽屉
    toggleDrawer() {
      this.drawerVisible = !this.drawerVisible
    },

    // 启动轮询（每 60 秒）
    startPolling() {
      if (this.pollingTimer) return
      this.fetchNotifications()
      this.pollingTimer = setInterval(() => {
        this.fetchNotifications()
      }, 60_000)
    },

    // 停止轮询
    stopPolling() {
      if (this.pollingTimer) {
        clearInterval(this.pollingTimer)
        this.pollingTimer = null
      }
    }
  }
})