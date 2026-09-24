// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import { clearCapabilitySnapshot, getCapabilitySnapshot } from './capabilities'
import { clearStoredAuth, getStoredAuth, saveAuthSession } from './auth'

describe('auth capability persistence', () => {
  beforeEach(() => {
    localStorage.clear()
    clearCapabilitySnapshot()
  })

  it('rehydrates MCP policy capabilities after a page reload', () => {
    saveAuthSession({
      token: 'token-1',
      username: 'analyst',
      force_password_change: false,
      role: 'security_analyst',
      capabilities: ['mcp:server:read', 'mcp:policy:read'],
      capability_version: 'role-policy-v1',
      capability_expires_at: new Date(Date.now() + 60_000).toISOString(),
    })

    clearCapabilitySnapshot()
    expect(getCapabilitySnapshot()).toBeNull()

    expect(getStoredAuth()?.capabilities).toContain('mcp:policy:read')
    expect(getCapabilitySnapshot()).toContain('mcp:policy:read')
  })

  it('clears persisted policy capabilities on logout', () => {
    saveAuthSession({ token: 'token-2', username: 'analyst', force_password_change: false, capabilities: ['mcp:policy:read'] })
    clearStoredAuth()

    expect(getStoredAuth()).toBeNull()
    expect(getCapabilitySnapshot()).toBeNull()
  })
})
