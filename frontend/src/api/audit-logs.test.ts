import { beforeEach, describe, expect, it, vi } from 'vitest'
import { auditLogApi } from './audit-logs'

const { getMock, deleteMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  deleteMock: vi.fn()
}))

vi.mock('./index', () => ({
  default: {
    get: getMock,
    delete: deleteMock
  }
}))

describe('audit-logs APIs', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('getLogs', () => {
    it('fetches logs with query params', async () => {
      const expected = { items: [], total: 0 }
      getMock.mockResolvedValueOnce(expected)
      const result = await auditLogApi.getLogs({ page: 1, page_size: 20, result: 'failed' })
      expect(getMock).toHaveBeenCalledWith('/settings/audit-logs', { params: { page: 1, page_size: 20, result: 'failed' } })
      expect(result).toEqual(expected)
    })

    it('fetches logs without params', async () => {
      const expected = { items: [], total: 0 }
      getMock.mockResolvedValueOnce(expected)
      const result = await auditLogApi.getLogs()
      expect(getMock).toHaveBeenCalledWith('/settings/audit-logs', { params: undefined })
      expect(result).toEqual(expected)
    })
  })

  describe('getLog', () => {
    it('fetches a single log by id', async () => {
      const expected = { id: 'log-1', script_content: '#!/bin/bash', result: 'passed' }
      getMock.mockResolvedValueOnce(expected)
      const result = await auditLogApi.getLog('log-1')
      expect(getMock).toHaveBeenCalledWith('/settings/audit-logs/log-1')
      expect(result).toEqual(expected)
    })
  })

  describe('getStats', () => {
    it('fetches audit stats', async () => {
      const expected = { total: 100, passed: 90, failed: 10, pass_rate: 90, retry_distribution: { '1': 80, '2': 8, '3': 2, failed: 10 } }
      getMock.mockResolvedValueOnce(expected)
      const result = await auditLogApi.getStats()
      expect(getMock).toHaveBeenCalledWith('/settings/audit-logs/stats')
      expect(result).toEqual(expected)
    })
  })

  describe('deleteLogs', () => {
    it('sends DELETE with ids in request body', async () => {
      const expected = { deleted: 3 }
      deleteMock.mockResolvedValueOnce(expected)
      const result = await auditLogApi.deleteLogs(['id-1', 'id-2', 'id-3'])
      expect(deleteMock).toHaveBeenCalledWith('/settings/audit-logs', { data: { ids: ['id-1', 'id-2', 'id-3'] } })
      expect(result).toEqual(expected)
    })
  })
})
