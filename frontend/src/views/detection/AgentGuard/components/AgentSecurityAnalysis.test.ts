// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentSecurityAnalysis from './AgentSecurityAnalysis.vue'
import type { AgentSecurityFindingSummary } from '@/types/agentGuard'

const PaginationStub = {
  props: ['currentPage', 'pageSize', 'total'],
  emits: ['current-change'],
  template: '<button class="pagination" @click="$emit(\'current-change\', Number(currentPage) + 1)">{{ total }}</button>',
}

describe('AgentSecurityAnalysis', () => {
  it('renders matched rule names and the matching tool invocation', () => {
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
          title: '敏感命令',
          severity: 'high',
          verdict: 'suspicious',
          confidence: 0.71,
          decision_sources: ['rule'],
          rule_hits: [{
            rule_key: 'AGB-BUILTIN-004',
            rule_version: 1,
            rule_name: '敏感命令',
            severity: 'high',
            event_ids: ['event-1'],
          }],
          matched_rules: [{
            rule_key: 'AGB-BUILTIN-004',
            rule_version: 1,
            name: '敏感命令',
            severity: 'high',
            event_ids: ['event-1'],
            tool_calls: [{
              event_id: 'event-1',
              tool_name: 'Bash',
              command: 'touch /etc/passwd',
              tool_input: { command: 'touch /etc/passwd' },
              tool_response: { ok: true },
              outcome: 'success',
              pid: 200,
              ppid: 100,
              correlation_status: 'matched',
            }],
          }],
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
          'el-pagination': true,
        },
      },
    })

    expect(wrapper.text()).toContain('AGB-BUILTIN-004')
    expect(wrapper.text()).toContain('敏感命令')
    expect(wrapper.text()).toContain('touch /etc/passwd')
    expect(wrapper.text()).toContain('Bash')
    expect(wrapper.text()).not.toContain('进程树')
  })

  it('renders multiple matched rules and keeps finding pagination', async () => {
    const findings: AgentSecurityFindingSummary[] = [1, 2].map(index => ({
      id: `finding-${index}`,
      title: `Finding ${index}`,
      severity: index === 1 ? 'high' : 'medium',
      verdict: 'suspicious',
      confidence: 0.7,
      decision_sources: ['rule'],
      rule_hits: [{
        rule_key: `AGB-BUILTIN-00${index}`,
        rule_version: 1,
        rule_name: `Rule ${index}`,
        event_ids: [`event-${index}`],
      }],
      status: 'open',
    }))
    const wrapper = mount(AgentSecurityAnalysis, {
      props: {
        rules: [],
        findings,
        selectedFinding: findings[0],
        findingTotal: 42,
        findingPage: 1,
        findingPageSize: 20,
        selectedFindingId: 'finding-1',
      },
      global: {
        stubs: {
          'el-alert': true,
          'el-tag': { template: '<span><slot /></span>' },
          'el-empty': true,
          'el-button': { template: '<button><slot /></button>' },
          'el-pagination': PaginationStub,
        },
      },
    })

    expect(wrapper.findAll('.finding-row')).toHaveLength(2)
    expect(wrapper.text()).toContain('敏感目录访问')
    expect(wrapper.text()).toContain('外部网络访问')

    await wrapper.find('.finding-pagination').trigger('click')
    expect(wrapper.emitted('finding-page-change')).toEqual([[2]])
  })
})
