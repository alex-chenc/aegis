// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentBehaviorEvidence from './AgentBehaviorEvidence.vue'

describe('AgentBehaviorEvidence', () => {
  it('renders whitelisted evidence fields but never arbitrary content fields', () => {
    const wrapper = mount(AgentBehaviorEvidence, {
      props: {
        behavior: {
          id: 'row-1',
          event_id: 'event-1',
          category: 'file',
          operation: 'write',
          outcome: 'success',
          actor: {
            pid: 4120,
            ppid: 4110,
            start_ticks: '9200',
            exe: '/usr/bin/python3',
            argv: ['python3', '[REDACTED]'],
            env: 'TEST_SECRET=must-not-render',
          },
          resource: {
            type: 'file',
            classification: 'credential',
            identity: '/etc/ssh/authorized_keys',
            file_content: 'must-not-render',
          },
          collection: {
            visibility: 'partial',
            truncated_fields: ['actor.argv'],
            lost_events_since_last: 2,
            raw_payload: 'must-not-render',
          },
        },
      },
      global: {
        stubs: {
          'el-alert': true,
          'el-tag': { template: '<span><slot /></span>' },
        },
      },
    })

    expect(wrapper.text()).toContain('4120')
    expect(wrapper.text()).toContain('/etc/ssh/authorized_keys')
    expect(wrapper.text()).toContain('[REDACTED]')
    expect(wrapper.text()).not.toContain('must-not-render')
  })
})
