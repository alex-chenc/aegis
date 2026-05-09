import request from './index'

export interface AuditLog {
  id: string
  script_type: string
  audit_source: string
  attempt_count: number
  result: 'passed' | 'failed'
  risk_level: string
  duration_ms: number
  script_content: string
  blacklist_hit_rules: { rule_name: string; line_number: number; matched_text: string }[]
  ai_audit_issues: { type: string; description: string; line_range: string; suggestion: string }[]
  audit_timeline: { attempt: number; result: string; timestamp: string }[]
  created_at: string
}

export interface AuditStats {
  total: number
  passed: number
  failed: number
  pass_rate: number
  retry_distribution: { '1': number; '2': number; '3': number; failed: number }
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
