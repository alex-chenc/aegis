// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import CoverageBadge from './CoverageBadge.vue'

describe('CoverageBadge', () => {
  it.each([
    ['monitor_only', '仅监控'],
    ['no_isolation', '无隔离边界'],
    ['remote_unobservable', '远端不可观测'],
    ['unsupported', '能力不支持'],
    ['unsupported_profile', 'Profile 不支持'],
    ['degraded', '能力降级'],
  ])('renders an explicit %s label', (coverage, expected) => {
    const wrapper = mount(CoverageBadge, {
      props: {
        coverage: coverage as any,
        reasons: ['kernel capability unavailable'],
      },
      global: {
        stubs: {
          'el-tooltip': { template: '<span><slot /></span>' },
          'el-tag': { template: '<span class="coverage-tag"><slot /></span>' },
        },
      },
    })

    expect(wrapper.text()).toContain(expected)
  })
})
