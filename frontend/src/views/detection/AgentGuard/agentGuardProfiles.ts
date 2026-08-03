export const AGENT_GUARD_AGENT_TYPES = [
  'codex',
  'openclaw',
  'hermes',
  'claude-code',
  'opencode',
  'gemini-cli',
] as const

export const AGENT_GUARD_AGENT_TYPE_FILTERS = [
  ...AGENT_GUARD_AGENT_TYPES,
  'other',
] as const
