// 通知严重级别
export type NotificationSeverity = 'critical' | 'high' | 'medium' | 'low' | 'info'

// 通知类型
export type NotificationType =
  | 'rule_generated'       // AI 规则自动生成
  | 'alert_triggered'      // 告警触发
  | 'approval_required'    // 待审核规则
  | 'system'               // 系统消息

// 通知元数据（业务扩展字段）
export interface NotificationMetadata {
  i18n_key?:     string
  i18n_params?:  Record<string, string | number | boolean | null>
  rule_id?:       string    // 关联规则 ID（rule_generated 时携带）
  mitre_id?:      string    // 关联 MITRE ID
  trigger_count?: number    // 触发次数
  trigger_hours?: number    // 触发时间窗口（小时）
  alert_ids?:     string[]  // 关联告警 ID 列表
  host_ids?:      string[]  // 关联主机 ID 列表
}

// 通知主体
export interface Notification {
  id:        string
  title:     string
  content:   string
  is_read:   boolean
  timestamp: string                   // ISO 8601
  severity:  NotificationSeverity
  type:      NotificationType
  link?:     string
  metadata?: NotificationMetadata
}

// 列表响应
export interface NotificationListResponse {
  list:         Notification[]
  total:        number
  unread_count: number
  page:         number
  page_size:    number
}

// 标记已读响应
export interface MarkReadResponse {
  success: boolean
}

// 全部标记已读响应
export interface MarkAllReadResponse {
  success: boolean
  updated_count: number
}
