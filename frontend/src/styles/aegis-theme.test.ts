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

  describe('button readability (V5.7)', () => {
    it('uses solid background for primary buttons instead of gradient', () => {
      expect(theme).toContain('.el-button--primary')
      // Should use solid color, not gradient
      expect(theme).not.toMatch(/\.el-button--primary[^}]*background:\s*linear-gradient/)
    })

    it('applies text-shadow to primary buttons for legibility', () => {
      expect(theme).toMatch(/\.el-button--primary[^}]*text-shadow:\s*0\s+1px\s+2px/)
    })

    it('applies letter-spacing to primary buttons for character clarity', () => {
      expect(theme).toMatch(/\.el-button--primary[^}]*letter-spacing:\s*0\.02em/)
    })

    it('defines hover state with darker background', () => {
      expect(theme).toContain('.el-button--primary:hover')
    })

    it('defines active state with darkest background', () => {
      expect(theme).toContain('.el-button--primary:active')
    })
  })
})
