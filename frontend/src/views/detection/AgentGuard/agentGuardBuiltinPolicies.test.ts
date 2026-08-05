import { describe, expect, it } from 'vitest'
import type { BuiltinAgentBehaviorRuleSummary } from '@/types/agentGuard'
import {
  BUILTIN_AGENT_GUARD_POLICIES,
  ruleExecutionOwner,
  rulesForBuiltinPolicy,
} from './agentGuardBuiltinPolicies'

const rules: BuiltinAgentBehaviorRuleSummary[] = [
  { rule_key: 'AGB-BUILTIN-001', rule_version: 1, name: '敏感目录' },
  { rule_key: 'AGB-BUILTIN-004', rule_version: 1, name: '敏感命令' },
  { rule_key: 'AGB-BUILTIN-005', rule_version: 1, name: '提权行为' },
]

describe('Agent Guard built-in policy catalog', () => {
  it('groups the five built-in rules into behavior and tool policies', () => {
    expect(BUILTIN_AGENT_GUARD_POLICIES).toHaveLength(2)
    expect(rulesForBuiltinPolicy(BUILTIN_AGENT_GUARD_POLICIES[0], rules).map(rule => rule.rule_key))
      .toEqual(['AGB-BUILTIN-001', 'AGB-BUILTIN-005'])
    expect(rulesForBuiltinPolicy(BUILTIN_AGENT_GUARD_POLICIES[1], rules).map(rule => rule.rule_key))
      .toEqual(['AGB-BUILTIN-004'])
  })

  it('assigns tool command matching to api-server', () => {
    expect(ruleExecutionOwner(rules[1])).toBe('api_server_tool_command_event')
    expect(ruleExecutionOwner(rules[0])).toBe('agent_and_api_server')
  })
})
