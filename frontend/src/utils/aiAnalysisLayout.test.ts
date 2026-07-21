import { describe, expect, it } from 'vitest'
import { getAIAnalysisFilterLabelWidth } from './aiAnalysisLayout'

describe('AI analysis layout', () => {
  it('keeps the compact filter labels for Chinese', () => {
    expect(getAIAnalysisFilterLabelWidth('zh-CN')).toBe(80)
  })

  it('reserves enough label width for English filter names', () => {
    expect(getAIAnalysisFilterLabelWidth('en-US')).toBe(190)
  })
})
