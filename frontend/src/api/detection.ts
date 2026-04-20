import request from './index'
import type { Alert, BlockPolicy, SigmaRule, BlockRecord, ThreatStatistics, AlertTrendPoint, AttackMatrix, LLMAggregation } from '@/types'

export function getAlerts(params: any): Promise<{ data: Alert[]; total: number }> {
  return request.get('/detection/alerts', { params })
}

export function getAlertDetail(alertId: string): Promise<Alert> {
  return request.get(`/detection/alerts/${alertId}`)
}

export function resolveAlert(alertId: string): Promise<void> {
  return request.post(`/detection/alerts/${alertId}/resolve`)
}

export function blockAlert(alertId: string, action?: string): Promise<BlockRecord> {
  return request.post(`/detection/alerts/${alertId}/block`, { action: action || 'kill_process' })
}

export function getBlockPolicies(params?: { page?: number; page_size?: number }): Promise<{ data: any[]; total: number }> {
  return request.get('/detection/block-policies', { params })
}

export function updateBlockPolicy(mitreId: string, data: any): Promise<void> {
  return request.put(`/detection/block-policies/${mitreId}`, data)
}

export function deleteBlockPolicy(mitreId: string): Promise<void> {
  return request.delete(`/detection/block-policies/${mitreId}`)
}

export function getRules(params: any): Promise<{ data: SigmaRule[]; total: number }> {
  return request.get('/detection/rules', { params })
}

export function updateRuleStatus(ruleId: string, status: string): Promise<void> {
  return request.put(`/detection/rules/${ruleId}/status`, { status })
}

export function getBlockRecords(params: any): Promise<{ data: BlockRecord[]; total: number }> {
  return request.get('/detection/blocks', { params })
}

export function getThreatStatistics(): Promise<ThreatStatistics> {
  return request.get('/detection/statistics/threats')
}

export function getAlertTrend(hours: number = 24): Promise<AlertTrendPoint[]> {
  return request.get('/detection/statistics/alert-trend', { params: { hours } })
}

export function getAttackMatrix(): Promise<AttackMatrix> {
  return request.get('/detection/attack-matrix')
}

export function startLLMAggregation(startTime: string, endTime: string, hostIds?: string[], autoDispose?: boolean): Promise<LLMAggregation> {
  return request.post('/detection/llm/aggregate', {
    start_time: startTime,
    end_time: endTime,
    host_ids: hostIds || [],
    auto_dispose: autoDispose || false
  })
}

export function getLLMAggregationStatus(aggregationId: string): Promise<LLMAggregation> {
  return request.get(`/detection/llm/aggregate/${aggregationId}`)
}

export function deleteAlerts(alertIds: string[]): Promise<{ deleted_count: number }> {
  return request.delete('/detection/alerts', { data: { alert_ids: alertIds } })
}

export interface GenerateSigmaRuleRequest {
  event: string
  method?: string
  mitre_id?: string
  severity?: string
}

export interface GenerateSigmaRuleResponse {
  rule_id: string
  title: string
  mitre_id: string
  severity: string
  content: string
  duration: number
}

export function generateSigmaRule(data: GenerateSigmaRuleRequest): Promise<GenerateSigmaRuleResponse> {
  return request.post('/detection/rules/generate', data)
}

export function checkRulesBeforeDelete(ruleIds: string[]): Promise<{
  has_alerts: boolean
  rules_with_alerts: Array<{ rule_id: string; title: string; alert_count: number }>
  total_alerts: number
}> {
  return request.post('/detection/rules/check-delete', { rule_ids: ruleIds })
}

export function deleteRules(ruleIds: string[]): Promise<{
  deleted_rules: number
  deleted_alerts: number
  deleted_policies: number
}> {
  return request.delete('/detection/rules', { data: { rule_ids: ruleIds } })
}

// AI规则配置
export interface AIConfig {
  id: string
  name: string
  enabled: boolean
  mode: 'suggest' | 'auto'
  thresholds: {
    high_frequency_count: number
    high_frequency_hours: number
  }
  conservatism: number
  require_approval: boolean
  auto_activate_after_approval: boolean
  activation_delay_hours: number
  notify_on_generation: boolean
  notify_on_approval: boolean
  notification_targets: string[]
  rules_generated_count: number
  rules_approved_count: number
}

export interface UpdateAIConfigRequest {
  enabled?: boolean
  mode?: 'suggest' | 'auto'
  thresholds?: {
    high_frequency_count: number
    high_frequency_hours: number
  }
  conservatism?: number
  require_approval?: boolean
  auto_activate_after_approval?: boolean
  activation_delay_hours?: number
  notify_on_generation?: boolean
  notify_on_approval?: boolean
  notification_targets?: string[]
}

export function getAIConfig(): Promise<AIConfig> {
  return request.get('/detection/rules/ai-rule-config')
}

export function updateAIConfig(data: UpdateAIConfigRequest): Promise<AIConfig> {
  return request.put('/detection/rules/ai-rule-config', data)
}

export interface GenerateTestRuleRequest {
  mitre_id: string
  sample_alerts?: string[]
  conservatism?: number
}

export function generateTestRule(data: GenerateTestRuleRequest): Promise<{
  rule_id: string
  title: string
  mitre_id: string
  severity: string
  content: string
  status: string
}> {
  return request.post('/detection/rules/generate-test', data)
}

export interface UploadSigmaRulesResponse {
  success: boolean
  parsed_count: number
  failed_count: number
  skipped_count: number
  rules: Array<{
    rule_id: string
    title: string
    status: string
    mitre_id: string
    severity: string
  }>
  failed_files?: string[]
}

export function uploadSigmaRules(file: File): Promise<UploadSigmaRulesResponse> {
  const formData = new FormData()
  formData.append('file', file)
  return request.post('/detection/rules/upload', formData, {
    headers: {
      'Content-Type': 'multipart/form-data'
    }
  })
}
