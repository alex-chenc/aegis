import { beforeEach, describe, expect, it, vi } from 'vitest'
import { commandAuditApi } from './command-audit'

const { getMock, postMock, putMock, deleteMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  postMock: vi.fn(),
  putMock: vi.fn(),
  deleteMock: vi.fn()
}))

vi.mock('./index', () => ({
  default: {
    get: getMock,
    post: postMock,
    put: putMock,
    delete: deleteMock
  }
}))

describe('command-audit APIs', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('getRules', () => {
    it('fetches rules with query params', async () => {
      const expected = { items: [], total: 0 }
      getMock.mockResolvedValueOnce(expected)
      const result = await commandAuditApi.getRules({ page: 1, page_size: 20, category: 'network' })
      expect(getMock).toHaveBeenCalledWith('/settings/command-audit/rules', { params: { page: 1, page_size: 20, category: 'network' } })
      expect(result).toEqual(expected)
    })

    it('fetches rules without params', async () => {
      const expected = { items: [], total: 0 }
      getMock.mockResolvedValueOnce(expected)
      const result = await commandAuditApi.getRules()
      expect(getMock).toHaveBeenCalledWith('/settings/command-audit/rules', { params: undefined })
      expect(result).toEqual(expected)
    })
  })

  describe('createRule', () => {
    it('creates a rule with correct payload', async () => {
      const payload = {
        name: '禁止curl管道执行',
        description: '禁止通过curl下载脚本并直接通过管道执行',
        rule_type: 'hard_block',
        match_type: 'regex',
        pattern: '(curl|wget).*\\|\\s*(bash|sh|zsh)',
        category: 'network',
        severity: 'critical',
        applies_to: ['all']
      }
      const expected = { id: 'rule-1', ...payload }
      postMock.mockResolvedValueOnce(expected)
      const result = await commandAuditApi.createRule(payload)
      expect(postMock).toHaveBeenCalledWith('/settings/command-audit/rules', payload)
      expect(result).toEqual(expected)
    })
  })

  describe('updateRule', () => {
    it('updates a rule by id', async () => {
      const payload = { name: 'updated name', is_enabled: false }
      const expected = { id: 'rule-1', ...payload }
      putMock.mockResolvedValueOnce(expected)
      const result = await commandAuditApi.updateRule('rule-1', payload)
      expect(putMock).toHaveBeenCalledWith('/settings/command-audit/rules/rule-1', payload)
      expect(result).toEqual(expected)
    })
  })

  describe('deleteRule', () => {
    it('deletes a rule by id', async () => {
      deleteMock.mockResolvedValueOnce(undefined)
      await commandAuditApi.deleteRule('rule-1')
      expect(deleteMock).toHaveBeenCalledWith('/settings/command-audit/rules/rule-1')
    })
  })

  describe('toggleRule', () => {
    it('toggles a rule by id', async () => {
      const expected = { id: 'rule-1', is_enabled: false }
      putMock.mockResolvedValueOnce(expected)
      const result = await commandAuditApi.toggleRule('rule-1')
      expect(putMock).toHaveBeenCalledWith('/settings/command-audit/rules/rule-1/toggle')
      expect(result).toEqual(expected)
    })
  })

  describe('testPattern', () => {
    it('tests a pattern with correct payload', async () => {
      const payload = {
        match_type: 'regex',
        pattern: 'rm\\s+-rf',
        test_content: 'rm -rf /tmp/test'
      }
      const expected = { matched: true, matches: [{ line_number: 1, matched_text: 'rm -rf /tmp/test' }] }
      postMock.mockResolvedValueOnce(expected)
      const result = await commandAuditApi.testPattern(payload)
      expect(postMock).toHaveBeenCalledWith('/settings/command-audit/rules/test', payload)
      expect(result).toEqual(expected)
    })
  })

  describe('getSettings', () => {
    it('fetches audit settings', async () => {
      const expected = { blacklist_enabled: true, ai_enabled: true, dispatch_check: true, agent_check: true, max_retry: 3 }
      getMock.mockResolvedValueOnce(expected)
      const result = await commandAuditApi.getSettings()
      expect(getMock).toHaveBeenCalledWith('/settings/command-audit/settings')
      expect(result).toEqual(expected)
    })
  })

  describe('updateSettings', () => {
    it('updates audit settings', async () => {
      const payload = { blacklist_enabled: false, ai_enabled: true, max_retry: 2 }
      const expected = { ...payload }
      putMock.mockResolvedValueOnce(expected)
      const result = await commandAuditApi.updateSettings(payload)
      expect(putMock).toHaveBeenCalledWith('/settings/command-audit/settings', payload)
      expect(result).toEqual(expected)
    })
  })
})
