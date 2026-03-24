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

export function getBlockPolicies(): Promise<BlockPolicy[]> {
  return request.get('/detection/block-policies')
}

export function updateBlockPolicy(mitreId: string, data: any): Promise<void> {
  return request.put(`/detection/block-policies/${mitreId}`, data)
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
