// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AgentPanoramaExplorer from './AgentPanoramaExplorer.vue'

describe('AgentPanoramaExplorer', () => {
  it('keeps process and behavior evidence in the same hierarchy', async () => {
    const loadChildren = vi.fn().mockResolvedValue([{
      id: 'file-1',
      parent_id: 'process-1',
      node_type: 'file',
      label: 'write · payload.bin',
      severity: 'high',
      has_children: false,
      occurred_at: '2026-07-30T10:00:01Z',
    }])
    const wrapper = mount(AgentPanoramaExplorer, {
      props: {
        nodes: [{
          id: 'process-1',
          node_type: 'process',
          label: 'python',
          pid: 4120,
          ppid: 4110,
          process_start_ticks: '9200',
          process_status: 'running',
          has_children: true,
          occurred_at: '2026-07-30T10:00:00Z',
        }],
        loadChildren,
        mode: 'behavior',
      },
      global: {
        stubs: {
          'el-tag': { template: '<span><slot /></span>' },
          'el-button': { template: '<button @click="$emit(\'click\')"><slot /></button>' },
          'el-alert': true,
          'el-empty': true,
        },
      },
    })

    expect(wrapper.text()).toContain('python')
    expect(wrapper.text()).toContain('PID 4120 · PPID 4110 · 运行中')
    await wrapper.get('[data-testid="panorama-expand-process-1"]').trigger('click')
    await flushPromises()
    expect(loadChildren).toHaveBeenCalledWith('process-1')
    expect(wrapper.text()).toContain('write · payload.bin')
    await wrapper.get('[data-testid="panorama-node-file-1"]').trigger('click')
    expect(wrapper.emitted('select')?.at(-1)?.[0]).toMatchObject({ id: 'file-1' })
  })

  it('marks activity windows as inferred instead of Codex conversations', () => {
    const wrapper = mount(AgentPanoramaExplorer, {
      props: {
        nodes: [{
          id: 'session-1',
          node_type: 'session',
          label: 'inferred activity window',
          session_source: 'activity_window',
          session_confidence: 'inferred',
          has_children: false,
        }],
        loadChildren: vi.fn().mockResolvedValue([]),
        mode: 'behavior',
      },
      global: {
        stubs: {
          'el-tag': true,
          'el-alert': true,
          'el-empty': true,
        },
      },
    })

    expect(wrapper.text()).toContain('推断活动窗口（非 Codex 会话）')
    expect(wrapper.text()).toContain('未获得来源会话 ID')
  })

  it('shows tool semantics only with trusted provenance and keeps missing hooks explicit', async () => {
    const wrapper = mount(AgentPanoramaExplorer, {
      props: {
        nodes: [{
          id: 'tool-1',
          node_type: 'tool_call',
          label: 'shell',
          has_children: false,
          trust: {
            tool_semantics: 'trusted',
            source: 'adapter_hook',
            proof_verified: true,
            correlation: 'matched',
          },
        }, {
          id: 'remote-1',
          node_type: 'execution_unit',
          label: 'remote sandbox',
          has_children: false,
          collection: {
            visibility: 'unobservable',
            limitations: ['tool_semantics_unobservable', 'remote_unobservable'],
          },
          trust: {
            tool_semantics: 'tool_semantics_unobservable',
            remote_visibility: 'remote_unobservable',
          },
        }],
        loadChildren: vi.fn().mockResolvedValue([]),
        mode: 'behavior',
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
