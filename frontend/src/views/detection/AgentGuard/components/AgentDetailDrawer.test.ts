// @vitest-environment jsdom
import { defineComponent, h } from 'vue'
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentDetailDrawer from './AgentDetailDrawer.vue'

const agent = {
  agent_scope_key: 'scope-1',
  asset_id: 'asset-1',
  host: { id: 'host-1', hostname: 'prod-ai-01', ip: '10.0.0.1' },
  agent_type: 'codex',
  display_name: 'Codex',
  running_instance_count: 1,
  controller_pids: [4100],
  runtime_status: 'running',
  isolation_types: ['linux_namespace'],
  coverage_level: 'monitor_only',
  coverage_reasons: [],
  high_risk_finding_count: 0,
  escape_finding_count: 0,
}

const DrawerStub = defineComponent({
  props: ['modelValue', 'size'],
  setup(props, { slots }) {
    return () => props.modelValue
      ? h('section', { class: 'drawer', 'data-size': props.size }, [slots.header?.(), slots.default?.()])
      : null
  },
})

const TabsStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', { class: 'tabs' }, slots.default?.())
  },
})

const TabPaneStub = defineComponent({
  props: ['label', 'name'],
  setup(props, { slots }) {
    return () => h('section', { class: 'tab-pane', 'data-name': props.name, 'data-label': props.label }, slots.default?.())
  },
})

function mountDrawer(mode: 'behavior' | 'escape') {
  return mount(AgentDetailDrawer, {
    props: {
      visible: true,
      mode,
      agent: agent as any,
      detailTab: 'panorama',
      instances: [],
      selectedInstanceId: '',
      panoramaNodes: [],
      findings: [],
      loading: { instances: false, panorama: false, analysis: false },
      errors: { instances: '', panorama: '', analysis: '' },
    },
    global: {
      stubs: {
        'el-drawer': DrawerStub,
        'el-tabs': TabsStub,
        'el-tab-pane': TabPaneStub,
        'el-empty': { template: '<div class="empty"><slot /></div>', props: ['description'] },
        'el-skeleton': true,
        'el-alert': true,
        'el-result': true,
        'el-button': true,
        'el-tag': true,
        'el-tooltip': true,
        'el-pagination': true,
        AgentRuntimeSelector: true,
        CoverageBadge: true,
      },
    },
  })
}

describe('AgentDetailDrawer', () => {
  it.each([
    ['behavior', ['panorama', 'analysis']],
    ['escape', ['analysis']],
  ] as const)('uses a 76%% drawer and the scoped detail tabs for %s', (mode, names) => {
    const wrapper = mountDrawer(mode)

    expect(wrapper.find('.drawer').attributes('data-size')).toBe('76%')
    expect(wrapper.find('.drawer').attributes('style')).toContain('min-width: 880px')
    expect(wrapper.findAll('.tab-pane')).toHaveLength(names.length)
    expect(wrapper.findAll('.tab-pane').map(tab => tab.attributes('data-name'))).toEqual(names)
    expect(wrapper.text()).toContain('Codex')
  })
})
