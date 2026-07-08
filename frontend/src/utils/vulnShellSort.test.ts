import { describe, expect, it } from 'vitest'
import {
  compareByShellGenerationStatus,
  compareHostShellRows,
  hostShellGenerationRank,
  shellGenerationStatusRank
} from './vulnShellSort'

describe('vulnShellSort', () => {
  it('ranks generated first (0)', () => {
    expect(shellGenerationStatusRank('generated')).toBe(0)
  })

  it('ranks generating in the middle (1)', () => {
    expect(shellGenerationStatusRank('generating')).toBe(1)
  })

  it('ranks none/pending/failed last (2)', () => {
    expect(shellGenerationStatusRank('none')).toBe(2)
    expect(shellGenerationStatusRank('pending')).toBe(2)
    expect(shellGenerationStatusRank('failed')).toBe(2)
    expect(shellGenerationStatusRank(undefined)).toBe(2)
  })

  it('orders generated before generating before none', () => {
    const items = ['none', 'generating', 'generated', 'pending']
    const sorted = [...items].sort(compareByShellGenerationStatus)
    expect(sorted[0]).toBe('generated')
    expect(sorted[1]).toBe('generating')
    expect(shellGenerationStatusRank(sorted[2])).toBe(2)
    expect(shellGenerationStatusRank(sorted[3])).toBe(2)
  })

  it('orders host rows generated > generating > failed > pending with host_id tie-break', () => {
    const rows = [
      { host_id: 'h3', generation_status: 'pending' },
      { host_id: 'h2', generation_status: 'generated' },
      { host_id: 'h1', generation_status: 'generating' },
      { host_id: 'h0', generation_status: 'failed' },
      { host_id: 'h4', generation_status: 'generated' }
    ]
    const sorted = [...rows].sort(compareHostShellRows)
    expect(sorted.map(r => r.host_id)).toEqual(['h2', 'h4', 'h1', 'h0', 'h3'])
    expect(hostShellGenerationRank('failed')).toBe(2)
  })
})
