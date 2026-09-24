import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const componentSource = readFileSync(fileURLToPath(new URL('./MCPAggregationControl.vue', import.meta.url)), 'utf8')

describe('MCP aggregation OPA entry point', () => {
  it('keeps the OPA entry point on a native DOM anchor', () => {
    expect(componentSource).toContain("@click=\"focusPolicyEditor\"")
    expect(componentSource).toContain('id="mcp-opa-policy-pane"')
    expect(componentSource).toContain("document.getElementById('mcp-opa-policy-pane')?.scrollIntoView")
  })
})
