import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAgentGuardStore } from './agentGuard'

const apiMocks = vi.hoisted(() => ({
  getAgentBehavior: vi.fn(),
  freezeAgentExecutionUnit: vi.fn(),
  getAgentGuardAction: vi.fn(),
  getAgentGuardOverview: vi.fn(),
  getAgentSecurityFinding: vi.fn(),
  deleteAgentGuardSessions: vi.fn(),
  listAgentSecurityFindingAnalyses: vi.fn(),
  analyzeAgentSecurityFinding: vi.fn(),
  listAgentGuardAgents: vi.fn(),
  listAgentGuardInstances: vi.fn(),
  listAgentGuardSessions: vi.fn(),
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
    apiMocks.listAgentGuardSessions.mockResolvedValue({ items: [], total: 0 })
    apiMocks.deleteAgentGuardSessions.mockResolvedValue({ deleted: 1 })
    apiMocks.getAgentPanorama.mockResolvedValue({ items: [], total: 0 })
    apiMocks.listAgentSecurityFindings.mockResolvedValue({ items: [], total: 0 })
    apiMocks.getAgentSecurityFinding.mockResolvedValue({
      id: 'finding-1',
      title: 'Finding',
      severity: 'high',
      status: 'open',
    })
    apiMocks.listAgentSecurityFindingAnalyses.mockResolvedValue({ items: [], total: 0 })
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

  it('uses server session pagination and removes selected sessions from local state', async () => {
    const store = useAgentGuardStore()
    apiMocks.listAgentGuardSessions.mockResolvedValueOnce({
      items: [{
        id: 'session-1', host_id: 'host-1', instance_id: 'instance-1', source: 'adapter_hook',
        confidence: 'confirmed', status: 'ended', started_at: '', last_seen_at: '',
        external_session_id: 'real-session-1',
      }],
      total: 41,
    })

    await store.fetchSessions('instance-1', 2)
    expect(apiMocks.listAgentGuardSessions).toHaveBeenCalledWith('instance-1', { page: 2, page_size: 20 })
    expect(store.sessionTotal).toBe(41)
    expect(store.sessionPage).toBe(2)

    await store.deleteSessions(['session-1'])
    expect(apiMocks.deleteAgentGuardSessions).toHaveBeenCalledWith(['session-1'])
    expect(store.sessions).toHaveLength(0)
    expect(store.sessionTotal).toBe(40)
  })

  it('keeps the paginated process-root response state', async () => {
    const store = useAgentGuardStore()
    apiMocks.getAgentPanorama.mockResolvedValueOnce({
      items: [{ id: 'process-root-21', node_type: 'process', label: 'PID 4200', pid: 4200 }],
      total: 42,
    })

    await store.fetchPanorama({
      instance_ids: ['instance-1'], session_id: 'session-1', page: 2, page_size: 20,
    })

    expect(store.panoramaNodes[0].pid).toBe(4200)
    expect(store.panoramaTotal).toBe(42)
    expect(store.panoramaPage).toBe(2)
    expect(store.panoramaPageSize).toBe(20)
  })

  it('loads one direct record for finding and event deep links', async () => {
    const store = useAgentGuardStore()

    await store.fetchFinding('finding-1')
    await store.fetchBehavior('event-1')

    expect(store.findings[0].id).toBe('finding-1')
    expect(store.selectedBehavior?.id).toBe('event-1')
  })

  it('keeps a deep-linked finding selected while restoring its scoped list', async () => {
    const store = useAgentGuardStore()
    apiMocks.getAgentSecurityFinding.mockResolvedValueOnce({
      id: 'finding-deep-link',
      instance_id: 'instance-1',
      title: 'Deep-linked finding',
      severity: 'high',
      status: 'open',
    })
    apiMocks.listAgentSecurityFindings.mockResolvedValueOnce({
      items: [{
        id: 'finding-latest',
        instance_id: 'instance-1',
        title: 'Latest finding',
        severity: 'medium',
        status: 'open',
      }],
      total: 42,
    })

    const selected = await store.fetchFinding('finding-deep-link')
    await store.fetchFindings({ instance_id: selected?.instance_id, page: 1, page_size: 20 })

    expect(store.selectedFinding?.id).toBe('finding-deep-link')
    expect(store.findings.map(item => item.id)).toEqual(['finding-latest'])
    expect(store.findingTotal).toBe(42)
  })

  it('keeps finding and analysis pagination totals and query state', async () => {
    const store = useAgentGuardStore()
    apiMocks.listAgentSecurityFindings.mockResolvedValueOnce({
      items: [{ id: 'finding-21', title: 'Finding 21', severity: 'medium', status: 'open' }],
      total: 42,
    })
    apiMocks.listAgentSecurityFindingAnalyses.mockResolvedValueOnce({
      items: [{
        id: 'analysis-11',
        finding_id: 'finding-21',
        attempt: 11,
        status: 'completed',
        prompt_version: 'v1',
        input_digest: 'digest-11',
        evidence_event_ids: [],
      }],
      total: 12,
    })

    await store.fetchFindings({ instance_id: 'instance-1', page: 2, page_size: 20 })
    await store.fetchFindingAnalyses('finding-21', { page: 2, page_size: 10 })

    expect(store.findingTotal).toBe(42)
    expect(store.findingPage).toBe(2)
    expect(store.findingPageSize).toBe(20)
    expect(store.analysisTotal).toBe(12)
    expect(store.analysisPage).toBe(2)
    expect(store.analysisPageSize).toBe(10)
    expect(apiMocks.listAgentSecurityFindingAnalyses).toHaveBeenCalledWith(
      'finding-21',
      { page: 2, page_size: 10 },
    )
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
