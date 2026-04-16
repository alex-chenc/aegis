import request from './index'
import type { NotificationListResponse, MarkReadResponse, MarkAllReadResponse } from '@/types/notification'

// 获取通知列表
export function getNotifications(params: {
  page?: number
  pageSize?: number
  is_read?: boolean
  type?: string
}): Promise<NotificationListResponse> {
  return request({
    url: '/notifications',
    method: 'get',
    params
  })
}

// 标记单条通知为已读
export function markNotificationRead(id: string): Promise<MarkReadResponse> {
  return request({
    url: `/notifications/${id}/read`,
    method: 'put'
  })
}

// 标记所有通知为已读
export function markAllNotificationsRead(): Promise<MarkAllReadResponse> {
  return request({
    url: '/notifications/read-all',
    method: 'put'
  })
}