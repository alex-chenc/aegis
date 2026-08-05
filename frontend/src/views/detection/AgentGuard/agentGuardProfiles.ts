export const AGENT_GUARD_AGENT_TYPES = [
  'codex',
  'openclaw',
  'hermes',
  'claude-code',
  'zcode',
  'opencode',
  'gemini-cli',
] as const

export const AGENT_GUARD_AGENT_TYPE_FILTERS = [
  ...AGENT_GUARD_AGENT_TYPES,
  'other',
] as const

export const AGENT_GUARD_NATIVE_HOOK_AGENT_TYPES = [
  'codex',
  'claude-code',
  'openclaw',
  'hermes',
  'zcode',
] as const
