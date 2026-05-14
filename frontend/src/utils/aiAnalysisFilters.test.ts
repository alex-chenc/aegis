import { describe, expect, it } from 'vitest'
import {
  buildAnalysisAlertQuery,
  buildAnalysisAlertSnapshot,
  filterAnalysisAlerts,
  filterOnlineHostnames,
  pruneSelectedAlertIds,
  shouldBypassClientFilter
} from './aiAnalysisFilters'

const alerts = [
  {
    id: 'a-1',
    alert_id: 'ALT-1',
    hostname: 'host-a',
    rule_title: '可疑进程',
    severity: 'high',
    status: 'pending',
    mitre_id: 'T1059',
    last_seen_at: '2026-04-28T01:05:00Z'
  },
  {
    id: 'a-2',
    alert_id: 'ALT-2',
    hostname: 'host-b',
    rule_title: '异常外联',
    severity: 'critical',
    status: 'pending',
    mitre_id: 'T1105',
    last_seen_at: '2026-04-28T01:30:00Z'
  },
  {
    id: 'a-3',
    alert_id: 'ALT-3',
    hostname: 'host-c',
    rule_title: '落地脚本',
    severity: 'medium',
    status: 'pending',
    mitre_id: 'T1027',
    last_seen_at: '2026-04-28T03:00:00Z'
  }
]

describe('AI analysis alert filtering', () => {
  it('builds selectable host options from online hosts only', () => {
    expect(filterOnlineHostnames([
      { hostname: 'host-a', online: true },
      { hostname: 'host-b', online: false },
      { hostname: 'host-a', online: true },
      { hostname: '', online: true }
    ])).toEqual(['host-a'])
  })

  it('does not show any alerts until time range or host filter is set', () => {
    expect(filterAnalysisAlerts(alerts, [], null)).toEqual([])
  })

  it('does not build an alert query until time range or host filter is set', () => {
    expect(buildAnalysisAlertQuery([], null)).toBeNull()
  })

  it('builds a backend-filtered alert query for time range and multiple hosts', () => {
    expect(buildAnalysisAlertQuery(['host-a', 'host-b'], [
      '2026-04-28T01:00:00Z',
      '2026-04-28T02:00:00Z'
    ])).toEqual({
      page: 1,
      pageSize: 10,
      hostnames: 'host-a,host-b',
      start_time: '2026-04-28T01:00:00.000Z',
      end_time: '2026-04-28T02:00:00.000Z'
    })
  })

  it('uses default pageSize of 10 when not specified', () => {
    const query = buildAnalysisAlertQuery(['host-a'], null)
    expect(query).toEqual({ page: 1, pageSize: 10, hostnames: 'host-a' })
  })

  it('supports custom pageSize of 20', () => {
    const query = buildAnalysisAlertQuery(['host-a'], null, 1, 20)
    expect(query).toEqual({ page: 1, pageSize: 20, hostnames: 'host-a' })
  })

  it('supports custom pageSize of 50', () => {
    const query = buildAnalysisAlertQuery(['host-a'], null, 1, 50)
    expect(query).toEqual({ page: 1, pageSize: 50, hostnames: 'host-a' })
  })

  it('supports custom page number', () => {
    const query = buildAnalysisAlertQuery(['host-a'], null, 3, 10)
    expect(query).toEqual({ page: 3, pageSize: 10, hostnames: 'host-a' })
  })

  it('shows alerts when only host filter is set', () => {
    const result = filterAnalysisAlerts(alerts, ['host-b'], null)

    expect(result.map(alert => alert.id)).toEqual(['a-2'])
  })

  it('shows alerts when only time range is set', () => {
    const result = filterAnalysisAlerts(alerts, [], [
      '2026-04-28T01:00:00Z',
      '2026-04-28T02:00:00Z'
    ])

    expect(result.map(alert => alert.id)).toEqual(['a-1', 'a-2'])
  })

  it('filters visible alerts by time range and multiple hosts', () => {
    const result = filterAnalysisAlerts(alerts, ['host-a', 'host-b'], [
      '2026-04-28T01:00:00Z',
      '2026-04-28T02:00:00Z'
    ])

    expect(result.map(alert => alert.id)).toEqual(['a-1', 'a-2'])
  })

  it('removes selected ids that are no longer visible after filters change', () => {
    const visible = filterAnalysisAlerts(alerts, ['host-a'], [
      '2026-04-28T01:00:00Z',
      '2026-04-28T02:00:00Z'
    ])

    expect(pruneSelectedAlertIds(['a-1', 'a-2'], visible)).toEqual(['a-1'])
  })

  it('builds a stable snapshot for the current analysis session', () => {
    const snapshot = buildAnalysisAlertSnapshot(alerts, ['a-2', 'a-1'])

    expect(snapshot.map(alert => alert.id)).toEqual(['a-2', 'a-1'])
    expect(snapshot[0]).toMatchObject({
      id: 'a-2',
      alert_id: 'ALT-2',
      hostname: 'host-b',
      rule_title: '异常外联'
    })
  })
})

describe('shouldBypassClientFilter', () => {
  it('returns false when no filters are set', () => {
    expect(shouldBypassClientFilter([], null)).toBe(false)
  })

  it('returns true when host filter is set', () => {
    expect(shouldBypassClientFilter(['host-a'], null)).toBe(true)
  })

  it('returns true when time range is set', () => {
    expect(shouldBypassClientFilter([], [
      '2026-04-28T01:00:00Z',
      '2026-04-28T02:00:00Z'
    ])).toBe(true)
  })

  it('returns true when both host and time range are set', () => {
    expect(shouldBypassClientFilter(['host-a'], [
      '2026-04-28T01:00:00Z',
      '2026-04-28T02:00:00Z'
    ])).toBe(true)
  })

  it('returns false when host filter has only empty strings', () => {
    expect(shouldBypassClientFilter(['', ''], null)).toBe(false)
  })
})
