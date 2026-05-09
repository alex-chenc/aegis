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
      expect(theme).toContain('.el-button--primary:not(.is-link)')
      // Should use solid color, not gradient
      expect(theme).not.toMatch(/\.el-button--primary:not\(\.is-link\)[^}]*background:\s*linear-gradient/)
    })

    it('does not blur Chinese text with shadow on primary buttons', () => {
      expect(theme).toMatch(/\.el-button--primary:not\(\.is-link\)[^}]*text-shadow:\s*none/)
      expect(theme).not.toMatch(/\.el-button--primary:not\(\.is-link\)[^}]*text-shadow:\s*0\s+1px\s+2px/)
    })

    it('keeps default CJK letter spacing on primary buttons', () => {
      expect(theme).toMatch(/\.el-button--primary:not\(\.is-link\)[^}]*letter-spacing:\s*0/)
      expect(theme).not.toMatch(/\.el-button--primary:not\(\.is-link\)[^}]*letter-spacing:\s*0\.02em/)
    })

    it('sets explicit white text and stable line height for filled primary buttons', () => {
      expect(theme).toMatch(/\.el-button--primary:not\(\.is-link\)[^}]*color:\s*#ffffff/)
      expect(theme).toMatch(/\.el-button--primary:not\(\.is-link\)[^}]*line-height:\s*1\.2/)
    })

    it('keeps link primary buttons as text links instead of filled pills', () => {
      expect(theme).toContain('.el-button.is-link.el-button--primary')
      expect(theme).toMatch(/\.el-button\.is-link\.el-button--primary[^}]*background:\s*transparent/)
      expect(theme).toMatch(/\.el-button\.is-link\.el-button--primary[^}]*box-shadow:\s*none/)
      expect(theme).toMatch(/\.el-button\.is-link\.el-button--primary[^}]*color:\s*var\(--aegis-action-blue-dark\)/)
    })

    it('defines hover state with darker background', () => {
      expect(theme).toContain('.el-button--primary:not(.is-link):hover')
    })

    it('defines active state with darkest background', () => {
      expect(theme).toContain('.el-button--primary:not(.is-link):active')
    })
  })
})
