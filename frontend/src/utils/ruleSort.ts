/**
 * Rule script-readiness ranking used to sort baseline rules in the Workbench
 * rule list. Order: 已生成(generated) > 生成中(generating) > 未生成/失败(pending/failed).
 *
 * A rule carries both a check and a fix script status. The combined rank is:
 *  - 0 (top)    if either check or fix status is `generated`
 *  - 1 (middle) if neither is generated but either is `generating`
 *  - 2 (bottom) otherwise (pending / failed)
 */
export type ScriptStatus = 'pending' | 'generating' | 'generated' | 'failed' | string

export interface RuleScriptStatusLike {
  check_script_status?: ScriptStatus
  fix_script_status?: ScriptStatus
}

export function ruleScriptStatusRank(rule: RuleScriptStatusLike): number {
  const check = rule.check_script_status
  const fix = rule.fix_script_status
  if (check === 'generated' || fix === 'generated') return 0
  if (check === 'generating' || fix === 'generating') return 1
  return 2
}

export function compareRulesByScriptStatus(
  a: RuleScriptStatusLike,
  b: RuleScriptStatusLike
): number {
  return ruleScriptStatusRank(a) - ruleScriptStatusRank(b)
}
