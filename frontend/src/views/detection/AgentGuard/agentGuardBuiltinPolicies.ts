import type { BuiltinAgentBehaviorRuleSummary } from '@/types/agentGuard'

export interface BuiltinAgentGuardPolicyView {
  policyKey: string
  nameKey: string
  descriptionKey: string
  modeKey: string
  categoryKeys: string[]
  ruleKeys: string[]
}

export const BUILTIN_AGENT_GUARD_POLICIES: BuiltinAgentGuardPolicyView[] = [
  {
    policyKey: 'builtin-agent-behavior-monitor',
    nameKey: 'agentGuard.policy.builtinBehaviorName',
    descriptionKey: 'agentGuard.policy.builtinBehaviorDescription',
    modeKey: 'agentGuard.policy.monitorOnly',
    categoryKeys: ['file', 'network', 'process', 'identity'],
    ruleKeys: [
      'AGB-BUILTIN-001',
      'AGB-BUILTIN-002',
      'AGB-BUILTIN-003',
      'AGB-BUILTIN-005',
    ],
  },
  {
    policyKey: 'builtin-agent-tool-command',
    nameKey: 'agentGuard.policy.builtinToolName',
    descriptionKey: 'agentGuard.policy.builtinToolDescription',
    modeKey: 'agentGuard.policy.monitorOnly',
    categoryKeys: ['tool', 'process'],
    ruleKeys: ['AGB-BUILTIN-004'],
  },
]

export function rulesForBuiltinPolicy(
  policy: BuiltinAgentGuardPolicyView,
  rules: BuiltinAgentBehaviorRuleSummary[],
): BuiltinAgentBehaviorRuleSummary[] {
  const byKey = new Map(rules.map(rule => [rule.rule_key, rule]))
  return policy.ruleKeys
    .map(ruleKey => byKey.get(ruleKey))
    .filter((rule): rule is BuiltinAgentBehaviorRuleSummary => Boolean(rule))
}

export function ruleExecutionOwner(rule: BuiltinAgentBehaviorRuleSummary): string {
  if (rule.rule_key === 'AGB-BUILTIN-004') {
    return 'api_server_tool_command_event'
  }
  return rule.engine || 'agent_and_api_server'
}
