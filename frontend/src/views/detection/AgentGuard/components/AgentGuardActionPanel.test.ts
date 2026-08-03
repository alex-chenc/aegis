// @vitest-environment jsdom
import { defineComponent, h } from 'vue'
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentGuardActionPanel from './AgentGuardActionPanel.vue'

const unit = {
  id: 'unit-12345678',
  instance_id: 'instance-1',
  unit_type: 'linux_namespace',
  root_pid: 4200,
  process_count: 3,
  coverage_level: 'full_enforcement',
  coverage_reasons: [],
  status: 'running',
  isolation_baseline: {},
  isolation_actual: {},
  isolation_diff: {},
  first_seen_at: '2026-07-30T10:00:00Z',
  last_seen_at: '2026-07-30T10:00:00Z',
}

const ButtonStub = defineComponent({
  inheritAttrs: false,
  props: ['disabled', 'loading', 'type'],
  emits: ['click'],
  setup(props, { attrs, emit, slots }) {
    return () => h('button', {
      ...attrs,
      disabled: props.disabled,
      onClick: () => emit('click'),
    }, slots.default?.())
  },
})

const DialogStub = defineComponent({
  props: ['modelValue', 'title'],
  emits: ['update:modelValue'],
  setup(props, { slots }) {
    return () => props.modelValue
      ? h('section', { class: 'dialog', 'data-title': props.title }, [slots.default?.(), slots.footer?.()])
      : null
  },
})

const AlertStub = defineComponent({
  props: ['title'],
  setup(props) {
    return () => h('div', { class: 'alert' }, props.title)
  },
})

function mountPanel(overrides: Record<string, unknown> = {}) {
  return mount(AgentGuardActionPanel, {
    props: {
      unit: unit as any,
      hostLabel: 'prod-ai-01 / 10.0.0.1',
      agentLabel: 'Codex',
      instanceLabel: 'PID 4100',
      actions: [],
      canOperate: true,
      ...overrides,
    },
    global: {
      stubs: {
        'el-button': ButtonStub,
        'el-dialog': DialogStub,
        'el-input': defineComponent({
          props: ['modelValue'], emits: ['update:modelValue'],
          setup(props, { emit }) {
            return () => h('textarea', {
              value: props.modelValue,
              onInput: (event: Event) => emit('update:modelValue', (event.target as HTMLTextAreaElement).value),
            })
          },
        }),
        'el-alert': AlertStub,
        'el-empty': true,
        'el-tag': true,
        'el-timeline': { template: '<div><slot /></div>' },
        'el-timeline-item': { template: '<div><slot /></div>' },
        'el-checkbox': true,
      },
    },
  })
}

describe('AgentGuardActionPanel', () => {
  it('confirms a single-unit freeze and does not claim success for accepted state', async () => {
    const wrapper = mountPanel()
    await wrapper.get('[data-action="freeze"]').trigger('click')

    expect(wrapper.text()).toContain('prod-ai-01')
    expect(wrapper.text()).toContain('unit-12345678')
    expect(wrapper.text()).toContain('3')
    expect(wrapper.text()).toContain('仅影响当前执行单元')

    await wrapper.get('textarea').setValue('confirmed namespace escape')
    await wrapper.get('[data-action="confirm"]').trigger('click')

    expect(wrapper.emitted('execute')?.[0]).toEqual([
      'freeze_execution_unit',
      { reason: 'confirmed namespace escape', hold: false },
    ])
  })

  it('never offers enforcement for remote-unobservable units', () => {
    const wrapper = mountPanel({
      unit: { ...unit, coverage_level: 'remote_unobservable' } as any,
    })

    expect(wrapper.find('[data-action="freeze"]').exists()).toBe(false)
    expect(wrapper.find('[data-action="kill"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('远端')
  })

  it('requires an explicit phrase before emitting kill', async () => {
    const wrapper = mountPanel()
    await wrapper.get('[data-action="kill"]').trigger('click')
    const fields = wrapper.findAll('textarea')
    await fields[0].setValue('confirmed compromise')
    await fields[1].setValue('wrong phrase')
    expect(wrapper.get('[data-action="confirm"]').attributes()).toHaveProperty('disabled')

    await fields[1].setValue('KILL 12345678')
    await wrapper.get('[data-action="confirm"]').trigger('click')
    expect(wrapper.emitted('execute')?.[0]?.[0]).toBe('kill_execution_unit')
  })
})
