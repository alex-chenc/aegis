import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const componentSource = readFileSync(new URL('./MCPAggregationControl.vue', import.meta.url), 'utf8')
const englishLocale = readFileSync(new URL('../../i18n/locales/en-US/app.ts', import.meta.url), 'utf8')
const chineseLocale = readFileSync(new URL('../../i18n/locales/zh-CN/app.ts', import.meta.url), 'utf8')

describe('MCP aggregation OPA entry', () => {
  it('exposes OPA as a dedicated tab with a Rego editor', () => {
    expect(componentSource).toContain('name="opaPolicy"')
    expect(componentSource).toContain('v-model="regoSource"')
    expect(componentSource).toContain('@click="validateRegoPolicy"')
    expect(componentSource).toContain('@click="publishPolicy"')
    expect(componentSource).not.toContain('ref="opaPolicyCard"')
    expect(componentSource).not.toContain('securityRulesDrawer')
    expect(componentSource).not.toContain('toggleSecurityRule')
    expect(componentSource).toContain('regoOnlySecurityNotice')
    expect(componentSource).toContain("t('app.mcpAggregation.regoSecurityBoundaryNotice')")
    expect(componentSource).toContain("t('app.mcpAggregation.viewAuthorizationAudit')")
    expect(componentSource).toContain("activeTab = 'invocations'")
    expect(componentSource).toContain('newRegoRuleTemplate')
    expect(componentSource).toContain('custom_deny_rule_ids contains')
    expect(componentSource).toContain('not custom_deny_rule_ids')
    expect(componentSource).toContain('@click="insertNewRegoRule"')
    expect(componentSource).toContain('await loadAuthorizationPolicy()')
    expect(componentSource).toContain('policyPublishError')
    expect(componentSource).toContain('const status = await store.publishAuthorizationPolicy(policy)')
    expect(componentSource).toContain('hydratePolicyEditor(status?.policy)')
    expect(componentSource).toContain("t('app.mcpAggregation.newRegoRule')")
    expect(componentSource).toContain("t('app.mcpAggregation.regoNewRuleInserted')")
  })

  it('localizes the new rule and save feedback in both supported locales', () => {
    for (const locale of [englishLocale, chineseLocale]) {
      expect(locale).toContain('newRegoRule:')
      expect(locale).toContain('regoNewRuleInserted:')
      expect(locale).toContain('policyPublishFailed:')
    }
  })
})
