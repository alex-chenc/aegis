import request from './index'

export interface AuditLog {
  id: string
  task_id: string
  rule_id: string
  script_type: string
  audit_source: string
  attempt: number
  passed: boolean
  risk_level: string
  duration_ms: number
  script_content: string
  blacklist_hits: { rule_name: string; line_number: number; matched_text: string }[]
  ai_analysis: { type: string; description: string; line_range: string; suggestion: string }[]
  error_msg: string
  created_at: string
}

export interface AuditStats {
  total: number
  passed: number
  failed: number
  pass_rate: number
  by_source: Record<string, number>
  by_type: Record<string, number>
  retry_distribution: Record<string, number>
}

export interface AuditLogParams {
  page?: number
  page_size?: number
  result?: string
  script_type?: string
  audit_source?: string
}

export const auditLogApi = {
  getLogs: (params?: AuditLogParams) =>
    request.get('/settings/audit-logs', { params }),

  getLog: (id: string) =>
    request.get(`/settings/audit-logs/${id}`),

  getStats: () =>
    request.get('/settings/audit-logs/stats'),

  deleteLogs: (ids: string[]) =>
    request.delete('/settings/audit-logs', { data: { ids } }),
}
