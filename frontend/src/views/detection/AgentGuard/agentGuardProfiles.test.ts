import { describe, expect, it } from 'vitest'
import {
  AGENT_GUARD_AGENT_TYPES,
  AGENT_GUARD_AGENT_TYPE_FILTERS,
} from './agentGuardProfiles'

describe('Agent Guard P4 profiles', () => {
  it('keeps the stable P1 and P4 profile keys available to filters and policies', () => {
    expect(AGENT_GUARD_AGENT_TYPES).toEqual([
      'codex',
      'openclaw',
      'hermes',
      'claude-code',
      'zcode',
      'opencode',
      'gemini-cli',
    ])
    expect(AGENT_GUARD_AGENT_TYPE_FILTERS).toEqual([
      ...AGENT_GUARD_AGENT_TYPES,
      'other',
    ])
  })
})
