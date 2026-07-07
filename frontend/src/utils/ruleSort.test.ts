import { describe, it, expect } from 'vitest'
import { ruleScriptStatusRank, compareRulesByScriptStatus } from './ruleSort'

describe('ruleScriptStatusRank', () => {
  it('ranks generated rules first (0)', () => {
    expect(ruleScriptStatusRank({ check_script_status: 'generated', fix_script_status: 'pending' })).toBe(0)
    expect(ruleScriptStatusRank({ check_script_status: 'pending', fix_script_status: 'generated' })).toBe(0)
    expect(ruleScriptStatusRank({ check_script_status: 'generated', fix_script_status: 'generated' })).toBe(0)
  })

  it('ranks generating rules in the middle (1)', () => {
    expect(ruleScriptStatusRank({ check_script_status: 'generating', fix_script_status: 'pending' })).toBe(1)
    expect(ruleScriptStatusRank({ check_script_status: 'pending', fix_script_status: 'generating' })).toBe(1)
  })

  it('ranks pending/failed rules last (2)', () => {
    expect(ruleScriptStatusRank({ check_script_status: 'pending', fix_script_status: 'pending' })).toBe(2)
    expect(ruleScriptStatusRank({ check_script_status: 'failed', fix_script_status: 'pending' })).toBe(2)
    expect(ruleScriptStatusRank({ check_script_status: 'failed', fix_script_status: 'failed' })).toBe(2)
  })

  it('does not treat a generated-fix + generating-check rule as generating', () => {
    // Either script generated => generated (rank 0), even if the other is generating
    expect(ruleScriptStatusRank({ check_script_status: 'generating', fix_script_status: 'generated' })).toBe(0)
  })
})

describe('compareRulesByScriptStatus', () => {
  it('orders generated before generating before pending', () => {
    const rules = [
      { id: 'a', check_script_status: 'pending', fix_script_status: 'pending' },
      { id: 'b', check_script_status: 'generating', fix_script_status: 'pending' },
      { id: 'c', check_script_status: 'generated', fix_script_status: 'pending' },
      { id: 'd', check_script_status: 'pending', fix_script_status: 'generating' }
    ]
    const sorted = [...rules].sort(compareRulesByScriptStatus).map(r => r.id)
    expect(sorted).toEqual(['c', 'b', 'd', 'a'])
  })
})
