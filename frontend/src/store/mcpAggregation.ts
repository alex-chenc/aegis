import { defineStore } from 'pinia'
import {
  createMCPOnboardingJob,
  disableMCPInvocationTool,
  decideMCPApproval,
  getMCPOverview,
  listMCPOnboardingJobs,
  listMCPApprovals,
  listMCPCatalogs,
  listMCPClientEndpoints,
  createMCPClientEndpoint,
  deleteMCPClientEndpoint,
  deleteMCPServer,
  updateMCPClientEndpointTools,
  listMCPInvocations,
  listMCPServers,
  listMCPSecurityRules,
  listMCPSecurityVerdicts,
  listMCPTools,
  setMCPSecurityRuleEnabled,
} from '@/api/mcpAggregation'
import type { MCPApprovalRequest, MCPClientEndpoint, MCPClientEndpointCreated, MCPInvocation, MCPOnboardingJob, MCPOnboardingPayload, MCPOverview, MCPServer, MCPCatalog, MCPSecurityRule, MCPSecurityVerdict, MCPToolRevision } from '@/types/mcpAggregation'
import type { MCPApprovalDecisionStatus } from '@/api/mcpAggregation'

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message ? error.message : 'request_failed'
}

export const useMCPAggregationStore = defineStore('mcpAggregation', {
  state: () => ({
    overview: null as MCPOverview | null,
    servers: [] as MCPServer[],
    serverOptions: [] as MCPServer[],
    serverTotal: 0,
    jobs: [] as MCPOnboardingJob[],
    jobTotal: 0,
    tools: [] as MCPToolRevision[],
    toolTotal: 0,
    catalogs: [] as MCPCatalog[],
    catalogTotal: 0,
    clientEndpoints: [] as MCPClientEndpoint[],
    clientEndpointTotal: 0,
    approvals: [] as MCPApprovalRequest[],
    approvalTotal: 0,
    invocations: [] as MCPInvocation[],
    invocationTotal: 0,
    securityVerdicts: [] as MCPSecurityVerdict[],
    securityTotal: 0,
    securityRules: [] as MCPSecurityRule[],
    securityRuleTotal: 0,
    loading: false,
    error: '',
    lastUpdatedAt: '',
  }),
  actions: {
    async loadPrimary(serverParams: Record<string, unknown> = {}) {
      this.loading = true
      this.error = ''
      try {
        const [overview, servers, serverOptions, jobs, catalogs] = await Promise.all([
          getMCPOverview(),
          listMCPServers({ page: 1, page_size: 10, ...serverParams }),
          listMCPServers({ page: 1, page_size: 100, status: 'published' }),
          listMCPOnboardingJobs({ page: 1, page_size: 20 }),
          listMCPCatalogs({ page: 1, page_size: 100 }),
        ])
        this.overview = overview
        this.servers = servers.items
        this.serverTotal = servers.total
        this.serverOptions = serverOptions.items
        this.jobs = jobs.items
        this.jobTotal = jobs.total
        this.catalogs = catalogs.items
        this.catalogTotal = catalogs.total
        this.lastUpdatedAt = overview.updated_at
      } catch (error) {
        this.error = errorMessage(error)
        throw error
      } finally {
        this.loading = false
      }
    },
    async loadOverview() {
      try {
        const overview = await getMCPOverview()
        this.overview = overview
        this.lastUpdatedAt = overview.updated_at
      } catch (error) {
        this.error = errorMessage(error)
      }
    },
    async createOnboarding(payload: MCPOnboardingPayload) {
      const idempotencyKey = crypto.randomUUID()
      const job = await createMCPOnboardingJob(payload, idempotencyKey)
      const existed = this.jobs.some(item => item.id === job.id)
      this.jobs = [job, ...this.jobs.filter(item => item.id !== job.id)]
      if (!existed) this.jobTotal += 1
      return job
    },
    async retireServer(id: string) {
      const result = await deleteMCPServer(id)
      this.servers = this.servers.filter(item => item.id !== id)
      this.serverOptions = this.serverOptions.filter(item => item.id !== id)
      this.serverTotal = Math.max(0, this.serverTotal - 1)
      return result
    },
    async loadTab(tab: string, params: Record<string, unknown> = { page: 1, page_size: 10 }) {
      this.loading = true
      this.error = ''
      try {
        if (tab === 'tools') {
          const result = await listMCPTools(params)
          this.tools = result.items
          this.toolTotal = result.total
        } else if (tab === 'catalogs') {
          const result = await listMCPCatalogs(params)
          this.catalogs = result.items
          this.catalogTotal = result.total
        } else if (tab === 'clients') {
          const endpoints = await listMCPClientEndpoints(params)
          this.clientEndpoints = endpoints.items
          this.clientEndpointTotal = endpoints.total
        } else if (tab === 'approvals') {
          const result = await listMCPApprovals(params)
          this.approvals = result.items
          this.approvalTotal = result.total
        } else if (tab === 'invocations') {
          const result = await listMCPInvocations(params)
          this.invocations = result.items
          this.invocationTotal = result.total
        } else if (tab === 'security') {
          const result = await listMCPSecurityVerdicts(params)
          this.securityVerdicts = result.items
          this.securityTotal = result.total
        }
      } catch (error) {
        this.error = errorMessage(error)
        throw error
      } finally {
        this.loading = false
      }
    },
    async loadSecurityRules(params: Record<string, unknown> = { page: 1, page_size: 10 }) {
      this.loading = true
      this.error = ''
      try {
        const result = await listMCPSecurityRules(params)
        this.securityRules = result.items
        this.securityRuleTotal = result.total
      } catch (error) {
        this.error = errorMessage(error)
        throw error
      } finally {
        this.loading = false
      }
    },
    async setSecurityRuleEnabled(id: string, enabled: boolean) {
      const result = await setMCPSecurityRuleEnabled(id, enabled)
      this.securityRules = this.securityRules.map(item => item.id === id ? result : item)
      return result
    },
    async createClientEndpoint(payload: { client_key: string; display_name: string; client_type: string; server_id: string }): Promise<MCPClientEndpointCreated> {
      const result = await createMCPClientEndpoint(payload)
      this.clientEndpoints = [result, ...this.clientEndpoints.filter(item => item.client_id !== result.client_id)]
      this.clientEndpointTotal += 1
      return result
    },
    async revokeClientEndpoint(clientId: string) {
      const result = await deleteMCPClientEndpoint(clientId)
      this.clientEndpoints = this.clientEndpoints.filter(item => item.client_id !== clientId)
      this.clientEndpointTotal = Math.max(0, this.clientEndpointTotal - 1)
      return result
    },
    async updateClientEndpointTools(grantId: string, toolAllowlist: string[]) {
      const result = await updateMCPClientEndpointTools(grantId, toolAllowlist)
      this.clientEndpoints = this.clientEndpoints.map(item => item.grant_id === grantId ? result : item)
      return result
    },
    async disableInvocationTool(invocationId: string) {
      const result = await disableMCPInvocationTool(invocationId)
      this.invocations = this.invocations.map(item => (
        item.client_id === result.client_id
        && item.server_id === result.server_id
        && item.tool_alias === result.tool_alias
          ? { ...item, tool_enabled: false }
          : item
      ))
      this.clientEndpoints = this.clientEndpoints.map(endpoint => (
        endpoint.client_id === result.client_id
          ? { ...endpoint, tools: endpoint.tools.map(tool => tool.alias === result.tool_alias ? { ...tool, enabled: false } : tool) }
          : endpoint
      ))
      return result
    },
    async decideApproval(id: string, status: MCPApprovalDecisionStatus, expectedRequestDigest: string, reason: string) {
      this.loading = true
      this.error = ''
      try {
        const result = await decideMCPApproval(id, status, expectedRequestDigest, reason)
        this.approvals = this.approvals.map(item => item.id === id ? result : item)
        return result
      } catch (error) {
        this.error = errorMessage(error)
        throw error
      } finally {
        this.loading = false
      }
    },
  },
})
