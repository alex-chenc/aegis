import request from './index'

export interface CommandAuditRule {
  id: string
  name: string
  description: string
  rule_type: 'hard_block' | 'soft_warn'
  match_type: 'exact' | 'regex'
  pattern: string
  category: string
  severity: 'critical' | 'high' | 'medium'
  applies_to: string[]
  is_preset: boolean
  is_enabled: boolean
  created_at: string
  updated_at: string
}

export interface CommandAuditSettings {
  blacklist_enabled: boolean
  ai_enabled: boolean
  dispatch_check: boolean
  agent_check: boolean
  max_retry: number
}

export interface RuleListParams {
  page?: number
  page_size?: number
  category?: string
  severity?: string
  match_type?: string
  is_enabled?: boolean
  keyword?: string
}

export interface CreateRulePayload {
  name: string
  description?: string
  rule_type: 'hard_block' | 'soft_warn'
  match_type: 'exact' | 'regex'
  pattern: string
  category: string
  severity: 'critical' | 'high' | 'medium'
  applies_to: string[]
}

export interface TestPatternPayload {
  match_type: 'exact' | 'regex'
  pattern: string
  test_content: string
}

export interface TestPatternResult {
  matched: boolean
  matches: { line_number: number; matched_text: string }[]
}

export const commandAuditApi = {
  getRules: (params?: RuleListParams) =>
    request.get('/settings/command-audit/rules', { params }),

  createRule: (data: CreateRulePayload) =>
    request.post('/settings/command-audit/rules', data),

  updateRule: (id: string, data: Partial<CreateRulePayload & { is_enabled: boolean }>) =>
    request.put(`/settings/command-audit/rules/${id}`, data),

  deleteRule: (id: string) =>
    request.delete(`/settings/command-audit/rules/${id}`),

  toggleRule: (id: string) =>
    request.put(`/settings/command-audit/rules/${id}/toggle`),

  testPattern: (data: TestPatternPayload) =>
    request.post('/settings/command-audit/rules/test', data),

  getSettings: () =>
    request.get('/settings/command-audit/settings'),

  updateSettings: (data: Partial<CommandAuditSettings>) =>
    request.put('/settings/command-audit/settings', data),
}
