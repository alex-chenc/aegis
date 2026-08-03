import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAgentGuardStore } from './agentGuard'

const apiMocks = vi.hoisted(() => ({
  getAgentBehavior: vi.fn(),
  freezeAgentExecutionUnit: vi.fn(),
  getAgentGuardAction: vi.fn(),
  getAgentGuardOverview: vi.fn(),
  getAgentSecurityFinding: vi.fn(),
  listAgentGuardAgents: vi.fn(),
  listAgentGuardInstances: vi.fn(),
  listAgentExecutionUnitTimeline: vi.fn(),
  getAgentPanorama: vi.fn(),
  listAgentSecurityFindings: vi.fn(),
  listAgentGuardPolicies: vi.fn(),
  createAgentGuardPolicy: vi.fn(),
  validateAgentGuardPolicy: vi.fn(),
  publishAgentGuardPolicy: vi.fn(),
  listAgentGuardPolicyDeliveries: vi.fn(),
  killAgentExecutionUnit: vi.fn(),
  resumeAgentExecutionUnit: vi.fn(),
}))

vi.mock('@/api/agentGuard', () => apiMocks)

const agent = {
  agent_scope_key: 'scope-1',
  asset_id: 'asset-1',
  host: { id: 'host-1', hostname: 'prod-ai-01', ip: '10.0.0.1' },
  agent_type: 'codex',
  display_name: 'Codex',
  running_instance_count: 2,
  controller_pids: [4100, 4400],
  runtime_status: 'running' as const,
  isolation_types: ['linux_namespace'] as const,
  coverage_level: 'monitor_only' as const,
  coverage_reasons: ['bpf_lsm_unavailable'],
  high_risk_finding_count: 1,
  escape_finding_count: 0,
  last_seen_at: '2026-07-30T10:00:00Z',
}

describe('Agent Guard store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    apiMocks.getAgentGuardOverview.mockResolvedValue({ running_instances: 2 })
    apiMocks.listAgentGuardAgents.mockResolvedValue({ items: [agent], total: 1 })
    apiMocks.listAgentGuardInstances.mockResolvedValue({ items: [], total: 0 })
    apiMocks.getAgentPanorama.mockResolvedValue({ items: [], total: 0 })
    apiMocks.listAgentSecurityFindings.mockResolvedValue({ items: [], total: 0 })
    apiMocks.getAgentSecurityFinding.mockResolvedValue({
      id: 'finding-1',
      title: 'Finding',
      severity: 'high',
      status: 'open',
    })
    apiMocks.getAgentBehavior.mockResolvedValue({ id: 'event-1' })
    apiMocks.listAgentExecutionUnitTimeline.mockResolvedValue({ items: [], total: 0 })
    apiMocks.freezeAgentExecutionUnit.mockResolvedValue({
      action_id: 'action-freeze', command_id: 'AG-GUARD-freeze', status: 'dispatching',
    })
    apiMocks.resumeAgentExecutionUnit.mockResolvedValue({
      action_id: 'action-resume', command_id: 'AG-GUARD-resume', status: 'pending',
    })
    apiMocks.killAgentExecutionUnit.mockResolvedValue({
      action_id: 'action-kill', command_id: 'AG-GUARD-kill', status: 'pending',
    })
    apiMocks.listAgentGuardPolicies.mockResolvedValue({ items: [], total: 0 })
    apiMocks.createAgentGuardPolicy.mockResolvedValue({
      policy: { id: 'policy-1', status: 'draft' },
      validation: { valid: true, errors: [], warnings: [] },
    })
    apiMocks.validateAgentGuardPolicy.mockResolvedValue({ valid: true, errors: [], warnings: [] })
    apiMocks.publishAgentGuardPolicy.mockResolvedValue({
      policy: { id: 'policy-1', status: 'published' },
      deliveries: [{ id: 'delivery-1', status: 'dispatching' }],
    })
    apiMocks.listAgentGuardPolicyDeliveries.mockResolvedValue({ items: [], total: 0 })
  })

  it('loads overview and outer summaries without prefetching drawer evidence', async () => {
    const store = useAgentGuardStore()

    await store.fetchPage({ page: 1, page_size: 20 })

    expect(store.agents).toEqual([agent])
    expect(store.agentTotal).toBe(1)
    expect(apiMocks.listAgentGuardInstances).not.toHaveBeenCalled()
    expect(apiMocks.getAgentPanorama).not.toHaveBeenCalled()
    expect(apiMocks.listAgentSecurityFindings).not.toHaveBeenCalled()
  })

  it('preserves previously loaded rows when a refresh fails', async () => {
    const store = useAgentGuardStore()
    await store.fetchPage({ page: 1, page_size: 20 })
    apiMocks.listAgentGuardAgents.mockRejectedValueOnce(new Error('temporarily unavailable'))

    await store.fetchPage({ page: 1, page_size: 20 })

    expect(store.agents).toEqual([agent])
    expect(store.errors.agents).toBe('temporarily unavailable')
  })

  it('loads panorama and analysis only after an explicit drawer request', async () => {
    const store = useAgentGuardStore()

    await store.fetchInstances({ asset_ids: ['asset-1'] })
    await store.fetchPanorama({ asset_id: 'asset-1' })
    await store.fetchFindings({ asset_id: 'asset-1', page: 1, page_size: 20 })

    expect(apiMocks.listAgentGuardInstances).toHaveBeenCalledOnce()
    expect(apiMocks.getAgentPanorama).toHaveBeenCalledOnce()
    expect(apiMocks.listAgentSecurityFindings).toHaveBeenCalledOnce()
  })

  it('loads one direct record for finding and event deep links', async () => {
    const store = useAgentGuardStore()

    await store.fetchFinding('finding-1')
    await store.fetchBehavior('event-1')

    expect(store.findings[0].id).toBe('finding-1')
    expect(store.selectedBehavior?.id).toBe('event-1')
  })

  it('publishes only through the policy workflow and keeps delivery truth', async () => {
    const store = useAgentGuardStore()
    const result = await store.publishPolicy('policy-1', 'approved rollout')

    expect(apiMocks.publishAgentGuardPolicy).toHaveBeenCalledWith('policy-1', 'approved rollout')
    expect(result.policy.status).toBe('published')
    expect(store.deliveries[0].status).toBe('dispatching')
    expect(store.deliveries[0].status).not.toBe('applied')
  })

  it('keeps accepted execution-unit actions pending until a terminal update arrives', async () => {
    const store = useAgentGuardStore()

    const accepted = await store.executeUnitAction(
      'unit-1',
      'freeze_execution_unit',
      { reason: 'confirmed namespace escape', hold: false },
    )

    expect(apiMocks.freezeAgentExecutionUnit).toHaveBeenCalledWith(
      'unit-1',
      { reason: 'confirmed namespace escape', hold: false },
    )
    expect(accepted.status).toBe('dispatching')
    expect(store.actions[0].status).toBe('dispatching')
    expect(store.actions[0].status).not.toBe('success')
  })
})
