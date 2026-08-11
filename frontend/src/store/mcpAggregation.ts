import { defineStore } from 'pinia'
import {
  createMCPOnboardingJob,
  decideMCPApproval,
  getMCPOverview,
  listMCPOnboardingJobs,
  listMCPApprovals,
  listMCPCatalogs,
  listMCPClients,
  listMCPClientEndpoints,
  createMCPClientEndpoint,
  updateMCPClientEndpointTools,
  listMCPInvocations,
  listMCPServers,
  listMCPSecurityVerdicts,
  listMCPTools,
} from '@/api/mcpAggregation'
import type { MCPApprovalRequest, MCPClient, MCPClientEndpoint, MCPClientEndpointCreated, MCPInvocation, MCPOnboardingJob, MCPOnboardingPayload, MCPOverview, MCPServer, MCPCatalog, MCPSecurityVerdict, MCPToolRevision } from '@/types/mcpAggregation'
import type { MCPApprovalDecisionStatus } from '@/api/mcpAggregation'

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message ? error.message : 'request_failed'
}

export const useMCPAggregationStore = defineStore('mcpAggregation', {
  state: () => ({
    overview: null as MCPOverview | null,
    servers: [] as MCPServer[],
    serverTotal: 0,
    jobs: [] as MCPOnboardingJob[],
    jobTotal: 0,
    tools: [] as MCPToolRevision[],
    toolTotal: 0,
    catalogs: [] as MCPCatalog[],
    catalogTotal: 0,
    clients: [] as MCPClient[],
    clientTotal: 0,
    clientEndpoints: [] as MCPClientEndpoint[],
    approvals: [] as MCPApprovalRequest[],
    approvalTotal: 0,
    invocations: [] as MCPInvocation[],
    invocationTotal: 0,
    securityVerdicts: [] as MCPSecurityVerdict[],
    securityTotal: 0,
    loading: false,
    error: '',
    lastUpdatedAt: '',
  }),
  actions: {
    async loadPrimary(serverParams: Record<string, unknown> = {}) {
      this.loading = true
      this.error = ''
      try {
        const [overview, servers, jobs, tools, catalogs] = await Promise.all([
          getMCPOverview(),
          listMCPServers({ page: 1, page_size: 20, ...serverParams }),
          listMCPOnboardingJobs({ page: 1, page_size: 20 }),
          listMCPTools({ page: 1, page_size: 100 }),
          listMCPCatalogs({ page: 1, page_size: 100 }),
        ])
        this.overview = overview
        this.servers = servers.items
        this.serverTotal = servers.total
        this.jobs = jobs.items
        this.jobTotal = jobs.total
        this.tools = tools.items
        this.toolTotal = tools.total
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
    async createOnboarding(payload: MCPOnboardingPayload) {
      const idempotencyKey = crypto.randomUUID()
      const job = await createMCPOnboardingJob(payload, idempotencyKey)
      const existed = this.jobs.some(item => item.id === job.id)
      this.jobs = [job, ...this.jobs.filter(item => item.id !== job.id)]
      if (!existed) this.jobTotal += 1
      return job
    },
    async loadTab(tab: string) {
      const params = { page: 1, page_size: 100 }
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
          const [result, endpoints] = await Promise.all([listMCPClients(params), listMCPClientEndpoints()])
          this.clients = result.items
          this.clientTotal = result.total
          this.clientEndpoints = endpoints.items
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
    async createClientEndpoint(payload: { client_key: string; display_name: string; client_type: string; server_id: string; tool_allowlist?: string[] }): Promise<MCPClientEndpointCreated> {
      const result = await createMCPClientEndpoint(payload)
      this.clientEndpoints = [result, ...this.clientEndpoints.filter(item => item.client_id !== result.client_id)]
      this.clients = [{ id: result.client_id, client_key: result.client_key, display_name: result.display_name, client_type: result.client_type, status: result.status, created_by: '', created_at: new Date().toISOString() }, ...this.clients.filter(item => item.id !== result.client_id)]
      this.clientTotal = this.clients.length
      return result
    },
    async updateClientEndpointTools(grantId: string, toolAllowlist: string[]) {
      const result = await updateMCPClientEndpointTools(grantId, toolAllowlist)
      this.clientEndpoints = this.clientEndpoints.map(item => item.grant_id === grantId ? result : item)
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
