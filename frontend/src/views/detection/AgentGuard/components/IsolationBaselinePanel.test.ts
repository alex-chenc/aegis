// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import IsolationBaselinePanel from './IsolationBaselinePanel.vue'

describe('IsolationBaselinePanel', () => {
  it('never presents no-isolation or unavailable evidence as healthy', () => {
    const wrapper = mount(IsolationBaselinePanel, {
      props: {
        unit: {
          id: 'unit-1',
          instance_id: 'instance-1',
          unit_type: 'local_process_tree',
          coverage_level: 'no_isolation',
          coverage_reasons: ['namespace_not_applicable', 'seccomp_unobservable'],
          status: 'observed',
          isolation_baseline: {
            namespace: { status: 'not_applicable' },
            cgroup: { status: 'unobservable', reason: 'cgroup_not_visible' },
          },
          isolation_actual: {},
          isolation_diff: {},
          first_seen_at: '2026-07-30T10:00:00Z',
          last_seen_at: '2026-07-30T10:00:00Z',
        },
      },
      global: {
        stubs: {
          'el-alert': { template: '<div class="alert"><slot /></div>', props: ['title'] },
          'el-tag': { template: '<span><slot /></span>' },
          'el-empty': true,
        },
      },
    })

    expect(wrapper.text()).toContain('no_isolation')
    expect(wrapper.text()).toContain('not_applicable')
    expect(wrapper.text()).toContain('unobservable')
    expect(wrapper.text()).not.toContain('健康')
    expect(wrapper.text()).not.toContain('Healthy')
  })
})
