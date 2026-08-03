import { describe, expect, it } from 'vitest'
import { buildAgentGuardCollectionPolicy } from './agentGuardPolicy'

describe('Agent Guard tool adapter policy', () => {
  it('keeps tool semantics disabled and out of collection by default', () => {
    const collection = buildAgentGuardCollectionPolicy(false)
    expect(collection.tool_adapter_enabled).toBe(false)
    expect(collection.categories).not.toContain('tool')
  })

  it('requests the tool category only with the explicit policy gate', () => {
    const collection = buildAgentGuardCollectionPolicy(true)
    expect(collection.tool_adapter_enabled).toBe(true)
    expect(collection.categories).toContain('tool')
  })
})
