// @vitest-environment jsdom
import { defineComponent, h } from 'vue'
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentSummaryTable from './AgentSummaryTable.vue'

const agent = {
  agent_scope_key: 'scope-1',
  asset_id: 'asset-1',
  host: { id: 'host-1', hostname: 'prod-ai-01', ip: '10.0.0.1' },
  agent_type: 'codex',
  display_name: 'Codex',
  running_instance_count: 2,
  controller_pids: [4100, 4400],
  runtime_status: 'running',
  isolation_types: ['linux_namespace'],
  coverage_level: 'monitor_only',
  coverage_reasons: ['bpf_lsm_unavailable'],
  high_risk_finding_count: 1,
  escape_finding_count: 0,
  last_seen_at: '2026-07-30T10:00:00Z',
  cmdline: 'SECRET --token=must-not-render',
  resolved_path: '/sensitive/path',
  destination_ip: '203.0.113.10',
  analysis_summary: 'must-not-render',
}

const TableStub = defineComponent({
  props: ['data'],
  emits: ['row-click'],
  setup(props, { slots, emit }) {
    return () => h('div', { class: 'table-stub' }, [
      h('button', { class: 'row', onClick: () => emit('row-click', props.data[0]) }, 'row'),
      slots.default?.(),
    ])
  },
})

const ColumnStub = defineComponent({
  props: ['label'],
  setup(props, { slots }) {
    return () => h('div', { class: 'column', 'data-label': props.label }, slots.default?.({
      row: agent,
    }))
  },
})

describe('AgentSummaryTable', () => {
  it('renders only basic summary fields and never renders evidence fields', () => {
    const wrapper = mount(AgentSummaryTable, {
      props: {
        agents: [agent] as any,
        loading: false,
        total: 1,
        page: 1,
        pageSize: 20,
        mode: 'behavior',
      },
      global: {
        stubs: {
          'el-table': TableStub,
          'el-table-column': ColumnStub,
          'el-button': { template: '<button><slot /></button>' },
          'el-tag': { template: '<span><slot /></span>' },
          'el-pagination': { template: '<div class="pagination" />' },
          CoverageBadge: { template: '<span>仅监控</span>' },
        },
        directives: { loading: () => undefined },
      },
    })

    expect(wrapper.text()).toContain('Codex')
    expect(wrapper.text()).toContain('4100')
    expect(wrapper.text()).not.toContain('must-not-render')
    expect(wrapper.text()).not.toContain('/sensitive/path')
    expect(wrapper.text()).not.toContain('203.0.113.10')
  })

  it('emits the same open event from a row click', async () => {
    const wrapper = mount(AgentSummaryTable, {
      props: {
        agents: [agent] as any,
        loading: false,
        total: 1,
        page: 1,
        pageSize: 20,
        mode: 'behavior',
      },
      global: {
        stubs: {
          'el-table': TableStub,
          'el-table-column': ColumnStub,
          'el-button': { template: '<button><slot /></button>' },
          'el-tag': { template: '<span><slot /></span>' },
          'el-pagination': true,
          CoverageBadge: true,
        },
        directives: { loading: () => undefined },
      },
    })

    await wrapper.find('.row').trigger('click')
    expect(wrapper.emitted('open')?.[0]).toEqual([agent])
  })
})
