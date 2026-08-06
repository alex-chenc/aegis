import request from './index'
import type {
  AgentBehaviorIndex,
  AgentGuardAction,
  AgentGuardActionAccepted,
  AgentGuardActionRequest,
  AgentGuardAgentQuery,
  AgentGuardAgentSummary,
  AgentGuardFindingQuery,
  AgentGuardExecutionUnitQuery,
  AgentGuardInstanceQuery,
  AgentGuardOverview,
  AgentGuardPanoramaQuery,
  AgentGuardRuleQuery,
  AgentGuardPolicy,
  AgentGuardPolicyDelivery,
  AgentGuardPolicyDraftRequest,
  AgentGuardPolicyMutationResult,
  AgentGuardPolicyPublishResult,
  AgentGuardPolicyValidation,
  AgentGuardRuntimeSettings,
  AgentConfigScanResult,
  AgentPanoramaResponse,
  AgentBehaviorSession,
  AgentExecutionUnit,
  AgentRuntimeInstance,
  AgentSecurityAnalysisRun,
  AgentSecurityFindingSummary,
  BuiltinAgentBehaviorRuleSummary,
  BuiltinAgentEscapeRuleSummary,
  PanoramaTreeNode,
  PageResult,
} from '@/types/agentGuard'

export function getAgentGuardRuntimeSettings(hostId: string): Promise<AgentGuardRuntimeSettings> {
  return request.get('/agent-guard/runtime-settings', { params: { host_id: hostId } })
}

export function updateAgentGuardRuntimeSettings(
  settings: Pick<AgentGuardRuntimeSettings, 'host_id' | 'tool_adapter_enabled' | 'session_hook_enabled' | 'behavior_policy_enabled' | 'escape_policy_enabled' | 'injections'>,
): Promise<AgentGuardRuntimeSettings> {
  return request.put('/agent-guard/runtime-settings', settings)
}

export function getAgentGuardOverview(params?: {
  host_ids?: string[]
  agent_types?: string[]
}): Promise<AgentGuardOverview> {
  return request.get('/agent-guard/overview', { params })
}

export function getAgentGuardCoverage(params?: {
  host_ids?: string[]
  agent_types?: string[]
  isolation_type?: string
}): Promise<AgentGuardOverview> {
  return request.get('/agent-guard/coverage', { params })
}

export function listAgentGuardAgents(
  params: AgentGuardAgentQuery,
): Promise<PageResult<AgentGuardAgentSummary>> {
  return request.get('/agent-guard/agents', { params })
}

export function scanAgentConfigurations(hostId: string): Promise<AgentConfigScanResult> {
  return request.get('/agent-guard/configurations', { params: { host_id: hostId } })
}

export function listAgentGuardInstances(
  params: AgentGuardInstanceQuery,
): Promise<PageResult<AgentRuntimeInstance>> {
  return request.get('/agent-guard/instances', { params })
}

export function listAgentGuardSessions(
  instanceId: string,
  params: { page?: number; page_size?: number } = {},
): Promise<PageResult<AgentBehaviorSession>> {
  return request.get(`/agent-guard/instances/${encodeURIComponent(instanceId)}/sessions`, {
    params: { page: params.page || 1, page_size: params.page_size || 100 },
  })
}

export function deleteAgentGuardSessions(sessionIds: string[]): Promise<{ deleted: number }> {
  return request.delete('/agent-guard/sessions', {
    data: { session_ids: sessionIds },
  })
}

export function getAgentPanorama(
  params: AgentGuardPanoramaQuery,
): Promise<AgentPanoramaResponse> {
  return request.get('/agent-guard/panorama', { params })
}

export function getAgentPanoramaChildren(
  nodeId: string,
  params: { page?: number; page_size?: number } = {},
): Promise<PageResult<PanoramaTreeNode>> {
  return request.get(
    `/agent-guard/panorama/nodes/${encodeURIComponent(nodeId)}/children`,
    { params: { page: params.page || 1, page_size: params.page_size || 100 } },
  )
}

export function getAgentExecutionUnit(id: string): Promise<AgentExecutionUnit> {
  return request.get(`/agent-guard/execution-units/${encodeURIComponent(id)}`)
}

export function listAgentExecutionUnits(
  params: AgentGuardExecutionUnitQuery,
): Promise<PageResult<AgentExecutionUnit>> {
  return request.get('/agent-guard/execution-units', { params })
}

export function listAgentExecutionUnitTimeline(
  id: string,
): Promise<PageResult<AgentGuardAction>> {
  return request.get(`/agent-guard/execution-units/${encodeURIComponent(id)}/timeline`, {
    params: { page: 1, page_size: 100 },
  })
}

export function getAgentGuardAction(id: string): Promise<AgentGuardAction> {
  return request.get(`/agent-guard/actions/${encodeURIComponent(id)}`)
}

export function freezeAgentExecutionUnit(
  id: string,
  payload: AgentGuardActionRequest,
): Promise<AgentGuardActionAccepted> {
  return request.post(`/agent-guard/execution-units/${encodeURIComponent(id)}/freeze`, payload)
}

export function resumeAgentExecutionUnit(
  id: string,
  payload: AgentGuardActionRequest,
): Promise<AgentGuardActionAccepted> {
  return request.post(`/agent-guard/execution-units/${encodeURIComponent(id)}/resume`, payload)
}

export function killAgentExecutionUnit(
  id: string,
  payload: AgentGuardActionRequest,
): Promise<AgentGuardActionAccepted> {
  return request.post(`/agent-guard/execution-units/${encodeURIComponent(id)}/kill`, payload)
}

export function listBuiltinAgentBehaviorRules(
  params: AgentGuardRuleQuery = {},
): Promise<PageResult<BuiltinAgentBehaviorRuleSummary>> {
  return request.get('/agent-guard/rules', {
    params: { page: 1, page_size: 100, ...params },
  })
}

export function listBuiltinAgentEscapeRules(
  params: AgentGuardRuleQuery = {},
): Promise<PageResult<BuiltinAgentEscapeRuleSummary>> {
  return request.get('/agent-guard/escape-rules', {
    params: { page: 1, page_size: 100, ...params },
  })
}

export function listAgentSecurityFindings(
  params: AgentGuardFindingQuery,
): Promise<PageResult<AgentSecurityFindingSummary>> {
  return request.get('/agent-guard/findings', { params })
}

export function getAgentSecurityFinding(
  id: string,
  params: { instance_id?: string; session_id?: string } = {},
): Promise<AgentSecurityFindingSummary> {
  const query = Object.fromEntries(Object.entries(params).filter(([, value]) => value))
  return Object.keys(query).length
    ? request.get(`/agent-guard/findings/${encodeURIComponent(id)}`, { params: query })
    : request.get(`/agent-guard/findings/${encodeURIComponent(id)}`)
}

export function listAgentSecurityFindingAnalyses(
  id: string,
  params: { page?: number; page_size?: number } = {},
): Promise<PageResult<AgentSecurityAnalysisRun>> {
  return request.get(`/agent-guard/findings/${encodeURIComponent(id)}/analyses`, {
    params: { page: params.page || 1, page_size: params.page_size || 10 },
  })
}

export function analyzeAgentSecurityFinding(
  id: string,
): Promise<AgentSecurityAnalysisRun> {
  return request.post(`/agent-guard/findings/${encodeURIComponent(id)}/analyze`)
}

export function getAgentBehavior(id: string): Promise<AgentBehaviorIndex> {
  return request.get(`/agent-guard/behaviors/${encodeURIComponent(id)}`)
}

export function listAgentGuardPolicies(params: {
  status?: string
  keyword?: string
  page: number
  page_size: number
}): Promise<PageResult<AgentGuardPolicy>> {
  return request.get('/agent-guard/policies', { params })
}

export function createAgentGuardPolicy(
  payload: AgentGuardPolicyDraftRequest,
): Promise<AgentGuardPolicyMutationResult> {
  return request.post('/agent-guard/policies', payload)
}

export function validateAgentGuardPolicy(
  id: string,
  payload: AgentGuardPolicyDraftRequest,
): Promise<AgentGuardPolicyValidation> {
  return request.post(`/agent-guard/policies/${encodeURIComponent(id)}/validate`, payload)
}

export function publishAgentGuardPolicy(
  id: string,
  reason: string,
): Promise<AgentGuardPolicyPublishResult> {
  return request.post(`/agent-guard/policies/${encodeURIComponent(id)}/publish`, { reason })
}

export function listAgentGuardPolicyDeliveries(
  id: string,
  params: { page: number; page_size: number; status?: string },
): Promise<PageResult<AgentGuardPolicyDelivery>> {
  return request.get(`/agent-guard/policies/${encodeURIComponent(id)}/deliveries`, { params })
}
