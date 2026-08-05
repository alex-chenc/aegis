// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import AgentPanoramaExplorer from './AgentPanoramaExplorer.vue'

describe('AgentPanoramaExplorer', () => {
  it('paginates behavior tool calls and emits the requested page', async () => {
    const PaginationStub = defineComponent({
      emits: ['current-change'],
      setup(_, { emit }) {
        return () => h('button', {
          class: 'panorama-pagination',
          onClick: () => emit('current-change', 2),
        }, 'next')
      },
    })
    const wrapper = mount(AgentPanoramaExplorer, {
      props: {
        nodes: [{
          id: 'tool-1', node_type: 'tool_call', label: 'Bash', tool_name: 'Bash', command: 'echo hello', has_children: false,
        }],
        total: 42,
        page: 1,
        pageSize: 20,
        loadChildren: vi.fn().mockResolvedValue([]),
        mode: 'behavior',
      },
      global: {
        stubs: {
          'el-pagination': PaginationStub,
          'el-tag': true,
          'el-alert': true,
          'el-empty': true,
        },
      },
    })

    await wrapper.get('.panorama-pagination').trigger('click')
    expect(wrapper.emitted('page-change')).toEqual([[2]])
  })

  it('renders tool execution content without an evidence panel or process tree', async () => {
    const loadChildren = vi.fn().mockResolvedValue([])
    const wrapper = mount(AgentPanoramaExplorer, {
      props: {
        nodes: [{
          id: 'tool-1',
          node_type: 'tool_call',
          label: 'Bash',
          tool_name: 'Bash',
          command: 'python worker --task payload.bin',
          tool_response: 'completed',
          pid: 4120,
          ppid: 4110,
          has_children: false,
          occurred_at: '2026-07-30T10:00:00Z',
        }],
        loadChildren,
        mode: 'behavior',
      },
      global: {
        stubs: {
          'el-tag': { template: '<span><slot /></span>' },
          'el-alert': true,
          'el-empty': true,
        },
      },
    })

    expect(wrapper.text()).toContain('Bash')
    expect(wrapper.text()).toContain('python worker --task payload.bin')
    expect(wrapper.text()).toContain('completed')
    expect(wrapper.classes()).toContain('tool-call-layout')
    expect(wrapper.find('.panorama-evidence').exists()).toBe(false)
    expect(loadChildren).not.toHaveBeenCalled()
  })

  it('does not render a Codex root or evidence details in behavior mode', () => {
    const wrapper = mount(AgentPanoramaExplorer, {
      props: {
        nodes: [{
          id: 'tool-1',
          node_type: 'tool_call',
          label: 'Bash',
          tool_name: 'Bash',
          command: 'echo session-1',
          has_children: false,
        }],
        loadChildren: vi.fn().mockResolvedValue([]),
        mode: 'behavior',
      },
      global: { stubs: { 'el-tag': true, 'el-alert': true, 'el-empty': true } },
    })

    expect(wrapper.text()).toContain('echo session-1')
    expect(wrapper.text()).not.toContain('推断活动窗口')
    expect(wrapper.find('.panorama-evidence').exists()).toBe(false)
  })

  it('shows PID and the complete command line', () => {
    const wrapper = mount(AgentPanoramaExplorer, {
      props: {
        nodes: [{
          id: 'tool-real',
          node_type: 'tool_call',
          label: 'Bash',
          tool_name: 'Bash',
          command: 'codex app-server --config /etc/codex/config.toml',
          pid: 4100,
          ppid: 1,
          has_children: false,
        }],
        loadChildren: vi.fn().mockResolvedValue([]),
        mode: 'behavior',
      },
      global: { stubs: { 'el-tag': true, 'el-alert': true, 'el-empty': true } },
    })

    expect(wrapper.text()).toContain('codex app-server --config /etc/codex/config.toml')
  })

  it('keeps trusted provenance details in the escape view', async () => {
    const wrapper = mount(AgentPanoramaExplorer, {
      props: {
        nodes: [{
          id: 'tool-1', node_type: 'tool_call', label: 'shell', has_children: false,
          trust: { tool_semantics: 'trusted', source: 'adapter_hook', proof_verified: true, correlation: 'matched' },
        }, {
          id: 'remote-1', node_type: 'execution_unit', label: 'remote sandbox', has_children: false,
          collection: { visibility: 'unobservable', limitations: ['tool_semantics_unobservable', 'remote_unobservable'] },
          trust: { tool_semantics: 'tool_semantics_unobservable', remote_visibility: 'remote_unobservable' },
        }],
        loadChildren: vi.fn().mockResolvedValue([]),
        mode: 'escape',
      },
      global: {
        stubs: {
          'el-tag': { template: '<span><slot /></span>' },
          'el-alert': { props: ['title'], template: '<div>{{ title }}</div>' },
          'el-empty': true,
        },
      },
    })

    expect(wrapper.text()).toContain('可信工具语义')
    await wrapper.get('[data-testid="panorama-node-tool-1"]').trigger('click')
    expect(wrapper.text()).toContain('adapter_hook')
    expect(wrapper.text()).toContain('tool call → process → resource')
    await wrapper.get('[data-testid="panorama-node-remote-1"]').trigger('click')
    expect(wrapper.text()).toContain('未获得可信 Agent 工具 Hook')
    expect(wrapper.text()).toContain('远端未关联可信传感器')
  })
})
