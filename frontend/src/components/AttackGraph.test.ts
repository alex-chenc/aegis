import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

describe('AttackGraph visual rendering', () => {
  const source = readFileSync(resolve(__dirname, 'AttackGraph.vue'), 'utf-8')

  it('uses CJK-safe text glyphs instead of emoji icons in SVG nodes', () => {
    expect(source).toContain('nodeGlyphs')
    expect(source).toContain("process: '进'")
    expect(source).not.toContain('👿')
    expect(source).not.toContain('💀')
    expect(source).not.toContain('⚙️')
    expect(source).not.toContain('📄')
    expect(source).not.toContain('🌐')
    expect(source).not.toContain('💻')
    expect(source).not.toContain('🦠')
  })
})
