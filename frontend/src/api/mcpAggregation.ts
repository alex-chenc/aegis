import request from './index'
import type {
  MCPApprovalRequest,
  MCPClient,
  MCPClientEndpoint,
  MCPClientEndpointCreated,
  MCPCatalog,
  MCPInvocation,
  MCPOnboardingJob,
  MCPOnboardingPayload,
  MCPOverview,
  MCPPage,
  MCPSecurityVerdict,
  MCPServer,
  MCPToolRevision,
} from '@/types/mcpAggregation'

export function getMCPOverview(): Promise<MCPOverview> {
  return request.get('/mcp-platform/overview')
}

export function listMCPServers(params: Record<string, unknown> = {}): Promise<MCPPage<MCPServer>> {
  return request.get('/mcp-platform/servers', { params })
}

export function listMCPOnboardingJobs(params: Record<string, unknown> = {}): Promise<MCPPage<MCPOnboardingJob>> {
  return request.get('/mcp-platform/onboarding-jobs', { params })
}

export function getMCPOnboardingJob(id: string): Promise<MCPOnboardingJob> {
  return request.get(`/mcp-platform/onboarding-jobs/${id}`)
}

export function createMCPOnboardingJob(payload: MCPOnboardingPayload, idempotencyKey: string): Promise<MCPOnboardingJob> {
  return request.post('/mcp-platform/onboarding-jobs', payload, { headers: { 'Idempotency-Key': idempotencyKey } })
}

export function retryMCPOnboardingJob(id: string): Promise<MCPOnboardingJob> {
  return request.post(`/mcp-platform/onboarding-jobs/${id}/retry`)
}

export function cancelMCPOnboardingJob(id: string): Promise<MCPOnboardingJob> {
  return request.post(`/mcp-platform/onboarding-jobs/${id}/cancel`)
}

export function listMCPTools(params: Record<string, unknown> = {}): Promise<MCPPage<MCPToolRevision>> {
  return request.get('/mcp-platform/tools', { params })
}

export function listMCPCatalogs(params: Record<string, unknown> = {}): Promise<MCPPage<MCPCatalog>> {
  return request.get('/mcp-platform/catalogs', { params })
}

export function listMCPClients(params: Record<string, unknown> = {}): Promise<MCPPage<MCPClient>> {
  return request.get('/mcp-platform/clients', { params })
}

export function listMCPClientEndpoints(): Promise<MCPPage<MCPClientEndpoint>> {
  return request.get('/mcp-platform/client-endpoints')
}

export function createMCPClientEndpoint(payload: { client_key: string; display_name: string; client_type: string; server_id: string; tool_allowlist?: string[] }): Promise<MCPClientEndpointCreated> {
  return request.post('/mcp-platform/client-endpoints', payload)
}

export function updateMCPClientEndpointTools(grantId: string, toolAllowlist: string[]): Promise<MCPClientEndpoint> {
  return request.put(`/mcp-platform/client-endpoints/${grantId}/tools`, { tool_allowlist: toolAllowlist })
}

export function listMCPApprovals(params: Record<string, unknown> = {}): Promise<MCPPage<MCPApprovalRequest>> {
  return request.get('/mcp-platform/approvals', { params })
}

export type MCPApprovalDecisionStatus = 'approved' | 'rejected'

export function decideMCPApproval(
  id: string,
  status: MCPApprovalDecisionStatus,
  expectedRequestDigest: string,
  reason: string,
): Promise<MCPApprovalRequest> {
  const action = status === 'approved' ? 'approve' : 'reject'
  return request.post(`/mcp-platform/approvals/${id}/${action}`, {
    expected_request_digest: expectedRequestDigest,
    reason,
  })
}

export function listMCPInvocations(params: Record<string, unknown> = {}): Promise<MCPPage<MCPInvocation>> {
  return request.get('/mcp-platform/invocations', { params })
}

export function listMCPSecurityVerdicts(params: Record<string, unknown> = {}): Promise<MCPPage<MCPSecurityVerdict>> {
  return request.get('/mcp-platform/security-verdicts', { params })
}
