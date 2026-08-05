import { describe, expect, it } from 'vitest'
import {
	buildAgentGuardDetailInstanceQuery,
	clearAgentGuardDetailQuery,
	parseAgentGuardQuery,
	selectPreferredAgentGuardInstance,
	serializeAgentGuardListQuery,
  withAgentGuardDetailQuery,
} from './agentGuardQuery'

describe('Agent Guard route query', () => {
  it('loads historical instances for detail instead of restricting to running ones', () => {
    expect(buildAgentGuardDetailInstanceQuery({
      assetId: 'asset-1',
      scopeKey: '',
      instanceId: '',
    })).toEqual({
      asset_ids: ['asset-1'],
      agent_scope_key: undefined,
      instance_ids: undefined,
      page: 1,
      page_size: 100,
    })
  })

  it('prefers an instance that already has a session over a newer empty instance', () => {
    const preferred = selectPreferredAgentGuardInstance([
      { id: 'new-empty', last_seen_at: '2026-08-05T10:00:00Z', high_risk_finding_count: 0 } as any,
      { id: 'historical-data', last_seen_at: '2026-08-04T10:00:00Z', high_risk_finding_count: 0 } as any,
    ], [{ id: 'session-1', instance_id: 'historical-data' } as any])

    expect(preferred?.id).toBe('historical-data')
  })

  it('restores filters, pagination, drawer selection and a whitelisted tab', () => {
    const parsed = parseAgentGuardQuery({
      host_id: 'host-1',
      agent_types: 'codex,hermes',
      status: 'running',
      coverage: 'monitor_only',
      keyword: 'prod',
      page: '3',
      page_size: '50',
      asset_id: 'asset-1',
      instance_id: 'instance-1',
      detail_tab: 'analysis',
    })

    expect(parsed.filters).toMatchObject({
      host_id: 'host-1',
      agent_types: ['codex', 'hermes'],
      runtime_status: 'running',
      coverage: 'monitor_only',
      keyword: 'prod',
    })
    expect(parsed.page).toBe(3)
    expect(parsed.pageSize).toBe(50)
    expect(parsed.detail).toEqual({
      assetId: 'asset-1',
      scopeKey: '',
      instanceId: 'instance-1',
      sessionId: '',
      findingId: '',
      eventId: '',
      tab: 'analysis',
    })
  })

  it('falls back to panorama for an invalid detail tab', () => {
    expect(parseAgentGuardQuery({ asset_id: 'asset-1', detail_tab: 'raw' }).detail.tab).toBe('panorama')
  })

  it('opens a finding or event deep link on analysis when no tab is supplied', () => {
    expect(parseAgentGuardQuery({ finding_id: 'finding-1' }).detail.tab).toBe('analysis')
    expect(parseAgentGuardQuery({ event_id: 'event-1' }).detail.tab).toBe('analysis')
  })

  it('merges detail state and removes only detail keys on close', () => {
    const current = {
      keyword: 'codex',
      page: '4',
      page_size: '20',
      unrelated: 'keep-me',
    }
    const opened = withAgentGuardDetailQuery(current, {
      assetId: 'asset-1',
      instanceId: 'instance-1',
      tab: 'analysis',
    })
    const closed = clearAgentGuardDetailQuery(opened)

    expect(opened).toMatchObject({
      keyword: 'codex',
      page: '4',
      asset_id: 'asset-1',
      instance_id: 'instance-1',
      detail_tab: 'analysis',
    })
    expect(closed).toEqual(current)
  })

  it('preserves an assetless stable agent scope key', () => {
    const parsed = parseAgentGuardQuery({
      agent_scope_key: 'signed-scope',
      detail_tab: 'panorama',
    })
    expect(parsed.detail.scopeKey).toBe('signed-scope')

    const closed = clearAgentGuardDetailQuery(withAgentGuardDetailQuery({}, {
      scopeKey: 'signed-scope',
      tab: 'panorama',
    }))
    expect(closed).toEqual({})
  })

  it('serializes only applied list filters and keeps pagination', () => {
    expect(serializeAgentGuardListQuery({
      host_id: 'host-1',
      agent_types: ['codex', 'openclaw'],
      runtime_status: '',
      coverage: 'degraded',
      isolation_type: '',
      keyword: '',
    }, 2, 50)).toEqual({
      host_id: 'host-1',
      agent_types: 'codex,openclaw',
      coverage: 'degraded',
      page: '2',
      page_size: '50',
    })
  })
})
