// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentRuntimeSelector from './AgentRuntimeSelector.vue'

describe('AgentRuntimeSelector', () => {
  it('uses the server total instead of presenting the first page length as the total', () => {
    const wrapper = mount(AgentRuntimeSelector, {
      props: {
        sessions: [{
          id: 'session-1', host_id: 'host-1', instance_id: 'instance-1', source: 'agent_official',
          confidence: 'confirmed', status: 'active', started_at: '', last_seen_at: '',
          external_session_id: '019fc96d-73e6-70b2-b25c-26e06f442a48',
        }],
        total: 208,
        selectedSessionId: '',
      },
      global: {
        stubs: {
          'el-tooltip': { template: '<span><slot /></span>' },
          'el-button': { template: '<button><slot /></button>' },
          'el-pagination': { template: '<div class="session-pagination" />' },
        },
      },
    })

    expect(wrapper.text()).toContain('019fc96d-73e6-70b2-b25c-26e06f442a48')
    expect(wrapper.text()).not.toContain('session-1')
  })

  it('renders the server-provided page without slicing it a second time', () => {
    const sessions = [{
      id: 'session-21',
      host_id: 'host-1',
      instance_id: 'instance-1',
      source: 'agent_official',
      confidence: 'confirmed',
      status: 'active',
      started_at: '',
      last_seen_at: '',
      external_session_id: 'external-session-21',
    }]
    const wrapper = mount(AgentRuntimeSelector, {
      props: { sessions, total: 21, page: 2, pageSize: 20, selectedSessionId: '' },
      global: {
        stubs: {
          'el-button': { template: '<button><slot /></button>' },
          'el-pagination': { template: '<div class="session-pagination" />' },
        },
      },
    })

    expect(wrapper.findAll('button.runtime-option')).toHaveLength(1)
    expect(wrapper.text()).toContain('external-session-21')
    expect(wrapper.text()).not.toContain('external-session-1')
  })
})
