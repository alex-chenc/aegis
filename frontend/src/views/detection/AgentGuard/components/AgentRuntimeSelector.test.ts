// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentRuntimeSelector from './AgentRuntimeSelector.vue'

describe('AgentRuntimeSelector', () => {
  it('uses the server total instead of presenting the first page length as the total', () => {
    const wrapper = mount(AgentRuntimeSelector, {
      props: {
        instances: [{
          id: 'instance-1', host_id: 'host-1', agent_type: 'codex',
          controller_pid: 4100, controller_start_ticks: '100', status: 'running',
          coverage_level: 'monitor_only', coverage_reasons: [],
        }],
        total: 208,
        selectedInstanceId: '',
      },
      global: {
        stubs: {
          'el-tooltip': { template: '<span><slot /></span>' },
        },
      },
    })

    expect(wrapper.text()).toContain('全部实例（208）')
    expect(wrapper.text()).toContain('PID 4100')
  })
})
