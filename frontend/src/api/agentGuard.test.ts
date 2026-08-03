import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  getAgentBehavior,
  freezeAgentExecutionUnit,
  getAgentGuardAction,
  getAgentGuardOverview,
  getAgentPanorama,
  getAgentSecurityFinding,
  killAgentExecutionUnit,
  listAgentExecutionUnitTimeline,
  createAgentGuardPolicy,
  listAgentGuardAgents,
  listAgentGuardInstances,
  listAgentGuardPolicies,
  publishAgentGuardPolicy,
  resumeAgentExecutionUnit,
  listAgentSecurityFindings,
  validateAgentGuardPolicy,
} from './agentGuard'

const { getMock, postMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  postMock: vi.fn(),
}))

vi.mock('./index', () => ({
  default: {
    get: getMock,
    post: postMock,
  },
}))

describe('Agent Guard API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('uses the overview and paginated outer agent endpoints', async () => {
    getMock.mockResolvedValueOnce({ running_instances: 2 })
    getMock.mockResolvedValueOnce({ items: [], total: 0 })

    await getAgentGuardOverview({ host_ids: ['host-1'] })
    await listAgentGuardAgents({
      host_ids: ['host-1'],
      agent_types: ['codex', 'hermes'],
      page: 2,
      page_size: 20,
    })

    expect(getMock).toHaveBeenNthCalledWith(1, '/agent-guard/overview', {
      params: { host_ids: ['host-1'] },
    })
    expect(getMock).toHaveBeenNthCalledWith(2, '/agent-guard/agents', {
      params: {
        host_ids: ['host-1'],
        agent_types: ['codex', 'hermes'],
        page: 2,
        page_size: 20,
      },
    })
  })

  it('keeps detail endpoints separate so the page can load them lazily', async () => {
    getMock.mockResolvedValue({ items: [], total: 0 })

    await listAgentGuardInstances({ asset_ids: ['asset-1'] })
    await getAgentPanorama({ asset_id: 'asset-1', instance_ids: ['instance-1'] })
    await listAgentSecurityFindings({ asset_id: 'asset-1', page: 1, page_size: 20 })

    expect(getMock).toHaveBeenNthCalledWith(1, '/agent-guard/instances', {
      params: { asset_ids: ['asset-1'] },
    })
    expect(getMock).toHaveBeenNthCalledWith(2, '/agent-guard/panorama', {
      params: { asset_id: 'asset-1', instance_ids: ['instance-1'] },
    })
    expect(getMock).toHaveBeenNthCalledWith(3, '/agent-guard/findings', {
      params: { asset_id: 'asset-1', page: 1, page_size: 20 },
    })
  })

  it('uses direct detail endpoints for finding and event deep links', async () => {
    getMock.mockResolvedValue({})

    await getAgentSecurityFinding('finding/1')
    await getAgentBehavior('event/1')

    expect(getMock).toHaveBeenNthCalledWith(1, '/agent-guard/findings/finding%2F1')
    expect(getMock).toHaveBeenNthCalledWith(2, '/agent-guard/behaviors/event%2F1')
  })

  it('keeps policy validation and publishing on protected server endpoints', async () => {
    getMock.mockResolvedValue({ items: [], total: 0 })
    postMock.mockResolvedValue({})
    const payload = {
      policy_key: 'prod-agent-guard',
      name: 'Production Agent Guard',
      priority: 100,
      targets: { host_ids: ['host-1'], host_group_ids: [], agent_types: ['codex'] },
      collection: {
        categories: ['process'],
        command_argv: 'redacted' as const,
        file_content: 'disabled' as const,
        network_content: 'disabled' as const,
        aggregation: {},
      },
      builtin_rule_overrides: [],
      atomic_rules: [],
      correlation_rules: [],
      analysis: {
        enabled: false,
        trigger_severities: ['high'],
        ai_only_action_ceiling: 'alert' as const,
        evidence_window_seconds: 300,
      },
      escape_rules: [],
      freeze_timeout_seconds: 300,
    }

    await listAgentGuardPolicies({ status: 'draft', page: 1, page_size: 20 })
    await createAgentGuardPolicy(payload)
    await validateAgentGuardPolicy('policy/1', payload)
    await publishAgentGuardPolicy('policy/1', 'approved rollout')

    expect(getMock).toHaveBeenCalledWith('/agent-guard/policies', {
      params: { status: 'draft', page: 1, page_size: 20 },
    })
    expect(postMock).toHaveBeenNthCalledWith(1, '/agent-guard/policies', payload)
    expect(postMock).toHaveBeenNthCalledWith(
      2,
      '/agent-guard/policies/policy%2F1/validate',
      payload,
    )
    expect(postMock).toHaveBeenNthCalledWith(
      3,
      '/agent-guard/policies/policy%2F1/publish',
      { reason: 'approved rollout' },
    )
  })

  it('uses execution-unit-scoped action endpoints and keeps accepted state', async () => {
    getMock.mockResolvedValue({ items: [], total: 0 })
    postMock.mockResolvedValue({ action_id: 'action-1', command_id: 'AG-GUARD-1', status: 'pending' })

    await listAgentExecutionUnitTimeline('unit/1')
    await getAgentGuardAction('action/1')
    await freezeAgentExecutionUnit('unit/1', { reason: 'confirmed namespace escape', hold: false })
    await resumeAgentExecutionUnit('unit/1', { reason: 'review complete' })
    await killAgentExecutionUnit('unit/1', { reason: 'confirmed compromise' })

    expect(getMock).toHaveBeenNthCalledWith(
      1,
      '/agent-guard/execution-units/unit%2F1/timeline',
      { params: { page: 1, page_size: 100 } },
    )
    expect(getMock).toHaveBeenNthCalledWith(2, '/agent-guard/actions/action%2F1')
    expect(postMock).toHaveBeenNthCalledWith(
      1,
      '/agent-guard/execution-units/unit%2F1/freeze',
      { reason: 'confirmed namespace escape', hold: false },
    )
    expect(postMock).toHaveBeenNthCalledWith(
      2,
      '/agent-guard/execution-units/unit%2F1/resume',
      { reason: 'review complete' },
    )
    expect(postMock).toHaveBeenNthCalledWith(
      3,
      '/agent-guard/execution-units/unit%2F1/kill',
      { reason: 'confirmed compromise' },
    )
  })
})
