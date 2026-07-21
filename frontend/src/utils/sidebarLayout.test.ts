import { describe, expect, it } from 'vitest'
import { getSidebarLayout } from './sidebarLayout'

describe('sidebar layout', () => {
  it('keeps the compact sidebar for Chinese labels', () => {
    expect(getSidebarLayout('zh-CN')).toEqual({
      expandedWidth: 220,
      wrapMenuLabels: false,
    })
  })

  it('allocates more width and wrapping for long English labels', () => {
    expect(getSidebarLayout('en-US')).toEqual({
      expandedWidth: 320,
      wrapMenuLabels: true,
    })
  })
})
