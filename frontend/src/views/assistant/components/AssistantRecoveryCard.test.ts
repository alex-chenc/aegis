// @vitest-environment jsdom

import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AssistantRecoveryCard from './AssistantRecoveryCard.vue'
import type { AssistantRecoveryRequest } from '@/api/assistant'

const ElButtonStub = defineComponent({
  props: {
    disabled: Boolean,
    loading: Boolean,
  },
  emits: ['click'],
  setup(props, { emit, slots }) {
    return () => h('button', {
      disabled: props.disabled,
      onClick: () => emit('click'),
    }, slots.default?.())
  },
})

const recovery: AssistantRecoveryRequest = {
  id: 'id-1',
  recovery_id: 'recovery-1',
  session_id: 'session-1',
  run_id: 'run-1',
  tool_call_id: 'call-1',
  tool_name: 'Package.Draft.Generate',
  code: 'detection_package_hook_coverage_blocked',
  category: 'recoverable_business_blocker',
  risk_level: 'high',
  summary: '当前 Hook 白名单无法完整观测该漏洞利用链。',
  detail: 'AF_ALG 和 splice 尚不可观测。',
  context: {
    required_hooks: [
      { attach_type: 'tracepoint', attach: 'syscalls/sys_enter_socket' },
      { attach_type: 'tracepoint', attach: 'syscalls/sys_enter_splice' },
    ],
  },
  actions: [
    {
      id: 'pause',
      label: '暂停当前操作',
      risk_level: 'readonly',
    },
  ],
  status: 'pending',
  created_at: '2026-07-25T00:00:00Z',
  updated_at: '2026-07-25T00:00:00Z',
}

describe('AssistantRecoveryCard', () => {
  it('shows the exact hook diff and emits only a persisted action id', async () => {
    const wrapper = mount(AssistantRecoveryCard, {
      props: { recovery },
      global: {
        stubs: {
          ElButton: ElButtonStub,
          ElTag: defineComponent({
            setup(_, { slots }) {
              return () => h('span', slots.default?.())
            },
          }),
        },
      },
    })

    expect(wrapper.text()).toContain('syscalls/sys_enter_socket')
    expect(wrapper.text()).toContain('syscalls/sys_enter_splice')
    expect(wrapper.text()).toContain('未列出的动作不会执行')

    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('decide')).toEqual([[
      'recovery-1',
      'pause',
      undefined,
    ]])
  })
})
