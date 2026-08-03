// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentSecurityAnalysis from './AgentSecurityAnalysis.vue'

describe('AgentSecurityAnalysis', () => {
  it('renders stable builtin rule IDs and marks AI-only evidence without an action', () => {
    const wrapper = mount(AgentSecurityAnalysis, {
      props: {
        rules: Array.from({ length: 5 }, (_, index) => ({
          rule_key: `AGB-BUILTIN-00${index + 1}`,
          rule_version: 1,
          name: `rule-${index + 1}`,
          enabled: true,
          severity: index === 4 ? 'high' : 'medium',
          action: 'alert',
        })),
        findings: [{
          id: 'finding-1',
          title: 'Suspicious generated command',
          severity: 'high',
          verdict: 'suspicious',
          confidence: 0.71,
          decision_sources: ['ai'],
          evidence_event_ids: ['event-1'],
          evidence_event_count: 1,
          evidence_completeness: {
            visibility: 'partial',
            reasons: ['stdin_not_captured'],
          },
          counter_evidence: ['command failed'],
          uncertainties: ['stdin_not_captured'],
          status: 'open',
        }],
        selectedFindingId: '',
      },
      global: {
        stubs: {
          'el-alert': { template: '<div class="alert"><slot /></div>', props: ['title'] },
          'el-tag': { template: '<span><slot /></span>' },
          'el-progress': true,
          'el-empty': true,
          'el-button': { template: '<button><slot /></button>' },
        },
      },
    })

    expect(wrapper.text()).toContain('AGB-BUILTIN-001')
    expect(wrapper.text()).toContain('AGB-BUILTIN-005')
    expect(wrapper.text()).toContain('Suspicious generated command')
    expect(wrapper.text()).toContain('command failed')
    expect(wrapper.find('[data-testid="automatic-action"]').exists()).toBe(false)
  })
})
