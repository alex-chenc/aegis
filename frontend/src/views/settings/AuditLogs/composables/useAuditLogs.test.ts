// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuditLogs } from './useAuditLogs'

const { getLogsMock, getLogMock, getStatsMock } = vi.hoisted(() => ({
  getLogsMock: vi.fn(),
  getLogMock: vi.fn(),
  getStatsMock: vi.fn()
}))

vi.mock('@/api/audit-logs', () => ({
  auditLogApi: {
    getLogs: getLogsMock,
    getLog: getLogMock,
    getStats: getStatsMock
  }
}))

describe('useAuditLogs', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('initializes with empty state', () => {
    const { logs, loading, stats, total } = useAuditLogs()
    expect(logs.value).toEqual([])
    expect(loading.value).toBe(false)
    expect(stats.value).toBeNull()
    expect(total.value).toBe(0)
  })

  it('fetches logs and updates state', async () => {
    const mockLogs = [
      { id: '1', script_type: 'check', audit_source: 'blacklist', attempt_count: 1, result: 'passed' as const, risk_level: 'safe', duration_ms: 100, script_content: '#!/bin/bash', blacklist_hit_rules: [], ai_audit_issues: [], audit_timeline: [], created_at: '2026-05-07' }
    ]
    getLogsMock.mockResolvedValueOnce({ items: mockLogs, total: 1 })

    const { logs, total, loading, fetchLogs } = useAuditLogs()
    await fetchLogs()

    expect(logs.value).toEqual(mockLogs)
    expect(total.value).toBe(1)
    expect(loading.value).toBe(false)
  })

  it('fetches stats and updates state', async () => {
    const mockStats = { total: 100, passed: 90, failed: 10, pass_rate: 90, retry_distribution: { '1': 80, '2': 8, '3': 2, failed: 10 } }
    getStatsMock.mockResolvedValueOnce(mockStats)

    const { stats, fetchStats } = useAuditLogs()
    await fetchStats()

    expect(stats.value).toEqual(mockStats)
  })

  it('fetches a single log detail', async () => {
    const mockLog = { id: '1', script_type: 'check', audit_source: 'blacklist', attempt_count: 1, result: 'passed' as const, risk_level: 'safe', duration_ms: 100, script_content: '#!/bin/bash', blacklist_hit_rules: [], ai_audit_issues: [], audit_timeline: [], created_at: '2026-05-07' }
    getLogMock.mockResolvedValueOnce(mockLog)

    const { fetchLogDetail } = useAuditLogs()
    const result = await fetchLogDetail('1')

    expect(getLogMock).toHaveBeenCalledWith('1')
    expect(result).toEqual(mockLog)
  })
})
