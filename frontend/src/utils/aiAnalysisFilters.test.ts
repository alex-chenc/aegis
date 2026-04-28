import { describe, expect, it } from 'vitest'
import {
  buildAnalysisAlertSnapshot,
  filterAnalysisAlerts,
  pruneSelectedAlertIds
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
