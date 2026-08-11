import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMCPClientEndpoint, createMCPOnboardingJob, decideMCPApproval, getMCPOverview, listMCPClientEndpoints, listMCPServers, listMCPTools, updateMCPClientEndpointTools } from './mcpAggregation'

const { getMock, postMock, putMock } = vi.hoisted(() => ({ getMock: vi.fn(), postMock: vi.fn(), putMock: vi.fn() }))

vi.mock('./index', () => ({ default: { get: getMock, post: postMock, put: putMock } }))

describe('MCP aggregation API', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('loads the overview from the platform control plane', async () => {
    getMock.mockResolvedValueOnce({ remote_servers: 1 })
    await getMCPOverview()
    expect(getMock).toHaveBeenCalledWith('/mcp-platform/overview')
  })

  it('keeps server filters in query parameters', async () => {
    getMock.mockResolvedValueOnce({ items: [], total: 0 })
    await listMCPServers({ page: 1, status: 'published' })
    expect(getMock).toHaveBeenCalledWith('/mcp-platform/servers', { params: { page: 1, status: 'published' } })
  })

  it('loads tool revisions for the tool list', async () => {
    getMock.mockResolvedValueOnce({ items: [], total: 0 })
    await listMCPTools({ page: 1, page_size: 100 })
    expect(getMock).toHaveBeenCalledWith('/mcp-platform/tools', { params: { page: 1, page_size: 100 } })
  })

  it('submits an approval decision with the digest and reason', async () => {
    postMock.mockResolvedValueOnce({ id: 'approval-1', status: 'approved' })
    await decideMCPApproval('approval-1', 'approved', 'digest-1', 'reviewed')
    expect(postMock).toHaveBeenCalledWith(
      '/mcp-platform/approvals/approval-1/approve',
      { expected_request_digest: 'digest-1', reason: 'reviewed' },
    )
  })

  it('submits onboarding with an idempotency key', async () => {
    postMock.mockResolvedValueOnce({ id: 'job-1', status: 'created' })
    await createMCPOnboardingJob({
      display_name: 'remote-test',
      endpoint_url: 'https://mcp.example.test/mcp',
      auth_type: 'oauth2',
      environment: 'test',
      publish_policy: 'approval_required',
    }, 'idem-1')
    expect(postMock).toHaveBeenCalledWith(
      '/mcp-platform/onboarding-jobs',
      expect.objectContaining({ endpoint_url: 'https://mcp.example.test/mcp' }),
      { headers: { 'Idempotency-Key': 'idem-1' } },
    )
  })

  it('creates a client-specific endpoint with a tool allowlist', async () => {
    postMock.mockResolvedValueOnce({ client_id: 'client-1', token: 'one-time-token' })
    await createMCPClientEndpoint({ client_key: 'codex-aegis', display_name: 'Codex', client_type: 'service', server_id: 'server-1', tool_allowlist: ['list_hosts'] })
    expect(postMock).toHaveBeenCalledWith('/mcp-platform/client-endpoints', expect.objectContaining({ server_id: 'server-1', tool_allowlist: ['list_hosts'] }))
  })

  it('updates a client endpoint tool switch', async () => {
    putMock.mockResolvedValueOnce({ grant_id: 'grant-1', tools: [] })
    await updateMCPClientEndpointTools('grant-1', [])
    expect(putMock).toHaveBeenCalledWith('/mcp-platform/client-endpoints/grant-1/tools', { tool_allowlist: [] })
  })

  it('loads endpoint records separately from legacy client records', async () => {
    getMock.mockResolvedValueOnce({ items: [], total: 0 })
    await listMCPClientEndpoints()
    expect(getMock).toHaveBeenCalledWith('/mcp-platform/client-endpoints')
  })
})
