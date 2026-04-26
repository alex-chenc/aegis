import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

describe('aegis global visual system', () => {
  const theme = readFileSync(resolve(__dirname, 'aegis-theme.css'), 'utf-8')

  it('defines security-console color tokens', () => {
    expect(theme).toContain('--aegis-bg-deep')
    expect(theme).toContain('--aegis-accent-cyan')
    expect(theme).toContain('--aegis-action-blue')
    expect(theme).toContain('--aegis-risk-critical')
  })

  it('defines CJK-safe typography tokens for UI screenshots', () => {
    expect(theme).toContain('--aegis-font-sans')
    expect(theme).toContain('--aegis-font-mono')
    expect(theme).toContain('Noto Sans CJK SC')
    expect(theme).toContain('WenQuanYi Micro Hei')
    expect(theme).toContain('Noto Sans Mono CJK SC')
  })

  it('provides reusable page and component primitives', () => {
    expect(theme).toContain('.page-shell')
    expect(theme).toContain('.page-hero')
    expect(theme).toContain('.aegis-card')
    expect(theme).toContain('.metric-card')
    expect(theme).toContain('.status-pill')
  })

  it('overrides Element Plus defaults for cards, tables and forms', () => {
    expect(theme).toContain('.el-card')
    expect(theme).toContain('.el-table')
    expect(theme).toContain('.el-input__wrapper')
    expect(theme).toContain('.el-button--primary')
  })
})
