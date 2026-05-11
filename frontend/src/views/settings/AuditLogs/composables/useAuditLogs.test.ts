// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuditLogs } from './useAuditLogs'

const { getLogsMock, getLogMock, getStatsMock, deleteLogsMock } = vi.hoisted(() => ({
  getLogsMock: vi.fn(),
  getLogMock: vi.fn(),
  getStatsMock: vi.fn(),
  deleteLogsMock: vi.fn()
}))

vi.mock('@/api/audit-logs', () => ({
  auditLogApi: {
    getLogs: getLogsMock,
    getLog: getLogMock,
    getStats: getStatsMock,
    deleteLogs: deleteLogsMock
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
      { id: '1', task_id: 't1', rule_id: 'r1', script_type: 'check', audit_source: 'generation', attempt: 1, passed: true, risk_level: 'safe', duration_ms: 100, script_content: '#!/bin/bash', blacklist_hits: [], ai_analysis: [], error_msg: '', created_at: '2026-05-07' }
    ]
    getLogsMock.mockResolvedValueOnce({ items: mockLogs, total: 1 })

    const { logs, total, loading, fetchLogs } = useAuditLogs()
    await fetchLogs()

    expect(logs.value).toEqual(mockLogs)
    expect(total.value).toBe(1)
    expect(loading.value).toBe(false)
  })

  it('fetches stats and updates state', async () => {
    const mockStats = { total: 100, passed: 90, failed: 10, pass_rate: 90, by_source: { generation: 80, dispatch: 15, agent: 5 }, by_type: { check: 60, fix: 30, poc_verify: 10 }, retry_distribution: { '1': 80, '2': 8, '3': 2, failed: 10 } }
    getStatsMock.mockResolvedValueOnce(mockStats)

    const { stats, fetchStats } = useAuditLogs()
    await fetchStats()

    expect(stats.value).toEqual(mockStats)
  })

  it('fetches a single log detail', async () => {
    const mockLog = { id: '1', task_id: 't1', rule_id: 'r1', script_type: 'check', audit_source: 'generation', attempt: 1, passed: true, risk_level: 'safe', duration_ms: 100, script_content: '#!/bin/bash', blacklist_hits: [], ai_analysis: [], error_msg: '', created_at: '2026-05-07' }
    getLogMock.mockResolvedValueOnce(mockLog)

    const { fetchLogDetail } = useAuditLogs()
    const result = await fetchLogDetail('1')

    expect(getLogMock).toHaveBeenCalledWith('1')
    expect(result).toEqual(mockLog)
  })

  it('deletes logs and refreshes list and stats', async () => {
    deleteLogsMock.mockResolvedValueOnce({ deleted: 2 })
    getLogsMock.mockResolvedValueOnce({ items: [], total: 0 })
    getStatsMock.mockResolvedValueOnce({ total: 0, passed: 0, failed: 0, pass_rate: 0, by_source: {}, by_type: {}, retry_distribution: { '1': 0, '2': 0, '3': 0, failed: 0 } })

    const { deleteLogs } = useAuditLogs()
    const result = await deleteLogs(['id-1', 'id-2'])

    expect(deleteLogsMock).toHaveBeenCalledWith(['id-1', 'id-2'])
    expect(getLogsMock).toHaveBeenCalled()
    expect(getStatsMock).toHaveBeenCalled()
    expect(result).toBe(2)
  })
})
