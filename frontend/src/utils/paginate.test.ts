import { describe, it, expect } from 'vitest'
import { paginate } from './paginate'

describe('paginate', () => {
  const items = Array.from({ length: 25 }, (_, i) => i + 1) // [1..25]

  it('returns the first page of pageSize items', () => {
    expect(paginate(items, 1, 10)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10])
  })

  it('returns the correct middle page', () => {
    expect(paginate(items, 2, 10)).toEqual([11, 12, 13, 14, 15, 16, 17, 18, 19, 20])
  })

  it('returns the remaining items on a partial last page', () => {
    expect(paginate(items, 3, 10)).toEqual([21, 22, 23, 24, 25])
  })

  it('treats page < 1 as page 1', () => {
    expect(paginate(items, 0, 10)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10])
    expect(paginate(items, -3, 10)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10])
  })

  it('clamps an out-of-range page to the last page', () => {
    expect(paginate(items, 99, 10)).toEqual([21, 22, 23, 24, 25])
  })

  it('returns an empty array for empty input', () => {
    expect(paginate([], 1, 10)).toEqual([])
  })

  it('returns the whole list when pageSize <= 0', () => {
    expect(paginate(items, 1, 0)).toEqual(items)
    expect(paginate(items, 2, -5)).toEqual(items)
  })

  it('handles a page size larger than the list', () => {
    expect(paginate(items, 1, 100)).toEqual(items)
  })
})
