// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useCommandAudit } from './useCommandAudit'

const { getRulesMock, createRuleMock, updateRuleMock, deleteRuleMock, toggleRuleMock, testPatternMock, getSettingsMock, updateSettingsMock } = vi.hoisted(() => ({
  getRulesMock: vi.fn(),
  createRuleMock: vi.fn(),
  updateRuleMock: vi.fn(),
  deleteRuleMock: vi.fn(),
  toggleRuleMock: vi.fn(),
  testPatternMock: vi.fn(),
  getSettingsMock: vi.fn(),
  updateSettingsMock: vi.fn()
}))

vi.mock('@/api/command-audit', () => ({
  commandAuditApi: {
    getRules: getRulesMock,
    createRule: createRuleMock,
    updateRule: updateRuleMock,
    deleteRule: deleteRuleMock,
    toggleRule: toggleRuleMock,
    testPattern: testPatternMock,
    getSettings: getSettingsMock,
    updateSettings: updateSettingsMock
  }
}))

describe('useCommandAudit', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('initializes with empty state', () => {
    const { rules, loading, settings, total } = useCommandAudit()
    expect(rules.value).toEqual([])
    expect(loading.value).toBe(false)
    expect(settings.value).toBeNull()
    expect(total.value).toBe(0)
  })

  it('fetches rules and updates state', async () => {
    const mockRules = [
      { id: '1', name: 'rule1', category: 'network', severity: 'critical', match_type: 'regex', pattern: '.*', rule_type: 'hard_block', is_preset: true, is_enabled: true, applies_to: ['all'], description: '', created_at: '', updated_at: '' }
    ]
    getRulesMock.mockResolvedValueOnce({ rules: mockRules, total: 1 })

    const { rules, total, loading, fetchRules } = useCommandAudit()
    await fetchRules()

    expect(rules.value).toEqual(mockRules)
    expect(total.value).toBe(1)
    expect(loading.value).toBe(false)
  })

  it('fetches settings and updates state', async () => {
    const mockSettings = { blacklist_enabled: true, ai_enabled: false, dispatch_check: true, agent_check: true, max_retry: 3 }
    getSettingsMock.mockResolvedValueOnce(mockSettings)

    const { settings, fetchSettings } = useCommandAudit()
    await fetchSettings()

    expect(settings.value).toEqual(mockSettings)
  })

  it('creates a rule and refreshes', async () => {
    createRuleMock.mockResolvedValueOnce({ id: 'new-1' })
    getRulesMock.mockResolvedValueOnce({ rules: [], total: 0 })

    const { createRule } = useCommandAudit()
    await createRule({ name: 'test', rule_type: 'hard_block', match_type: 'regex', pattern: '.*', category: 'system', severity: 'high', applies_to: ['all'] })

    expect(createRuleMock).toHaveBeenCalled()
    expect(getRulesMock).toHaveBeenCalled()
  })

  it('toggles a rule and refreshes', async () => {
    toggleRuleMock.mockResolvedValueOnce({ id: '1', is_enabled: false })
    getRulesMock.mockResolvedValueOnce({ rules: [], total: 0 })

    const { toggleRule } = useCommandAudit()
    await toggleRule('1')

    expect(toggleRuleMock).toHaveBeenCalledWith('1')
    expect(getRulesMock).toHaveBeenCalled()
  })

  it('tests a pattern', async () => {
    const mockResult = { matched: true, matches: [{ line_number: 1, matched_text: 'rm -rf /' }] }
    testPatternMock.mockResolvedValueOnce(mockResult)

    const { testPattern } = useCommandAudit()
    const result = await testPattern({ match_type: 'regex', pattern: 'rm\\s+-rf', test_content: 'rm -rf /' })

    expect(result).toEqual(mockResult)
  })

  it('updates settings', async () => {
    updateSettingsMock.mockResolvedValueOnce({})
    getSettingsMock.mockResolvedValueOnce({})

    const { updateSettings } = useCommandAudit()
    await updateSettings({ max_retry: 5 })

    expect(updateSettingsMock).toHaveBeenCalledWith({ max_retry: 5 })
  })
})
