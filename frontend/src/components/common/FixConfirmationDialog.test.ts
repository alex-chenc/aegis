// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { defineComponent, h, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import FixConfirmationDialog from './FixConfirmationDialog.vue'

const pushMock = vi.fn()

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: pushMock
  })
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    success: vi.fn(),
    warning: vi.fn(),
    error: vi.fn()
  }
}))

const ElDialogStub = defineComponent({
  name: 'ElDialogStub',
  props: {
    modelValue: {
      type: Boolean,
      default: false
    }
  },
  setup(props, { slots }) {
    return () => (props.modelValue ? h('div', { class: 'dialog' }, slots.default?.()) : null)
  }
})

const HostScriptStatusListStub = defineComponent({
  name: 'HostScriptStatusList',
  props: {
    cveId: String,
    scriptType: String,
    cveSource: String,
    affectedHostsCount: Number
  },
  emits: ['execute'],
  setup(props, { emit }) {
    return () => h('button', {
      class: 'host-script-status-list-stub',
      onClick: () => emit('execute', { taskGroupId: 'tg-1', hosts: ['host-aff-1'] })
    }, `${props.cveId}:${props.scriptType}:${props.cveSource}:${props.affectedHostsCount}`)
  }
})

const baseProps = {
  visible: true,
  mode: 'poc' as const,
  cve: {
    id: 'vul-1',
    cve_id: 'CVE-2024-0001',
    severity: 'High' as const,
    cvss_score: 8.8,
    description: 'test',
    affected_hosts_count: 1,
    source: 'llm_analysis'
  }
}

function mountDialog(extraProps?: Record<string, unknown>) {
  return mount(FixConfirmationDialog, {
    props: {
      ...baseProps,
      ...extraProps
    },
    global: {
      stubs: {
        'el-dialog': ElDialogStub,
        'el-descriptions': true,
        'el-descriptions-item': true,
        'el-link': true,
        'el-alert': true,
        'el-empty': true,
        'el-button': true,
        SeverityTag: true,
        HostScriptStatusList: HostScriptStatusListStub
      }
    }
  })
}

describe('FixConfirmationDialog host script dispatch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('passes cve context to host script status list', async () => {
    const wrapper = mountDialog({
      cve: {
        ...baseProps.cve,
        source: 'llm_analysis'
      }
    })

    await nextTick()

    const list = wrapper.find('.host-script-status-list-stub')
    expect(list.exists()).toBe(true)
    expect(list.text()).toContain('CVE-2024-0001:poc:llm_analysis:1')
  })

  it('uses fix script type for fix mode', async () => {
    const wrapper = mountDialog({
      mode: 'fix',
      cve: {
        ...baseProps.cve,
        source: 'custom_query'
      }
    })

    await nextTick()

    expect(wrapper.find('.host-script-status-list-stub').text()).toContain('CVE-2024-0001:fix:custom_query:1')
  })

  it('emits task after host script execution', async () => {
    const wrapper = mountDialog({ mode: 'poc' })
    await nextTick()

    await wrapper.find('.host-script-status-list-stub').trigger('click')
    await nextTick()

    expect(wrapper.emitted('execute')?.[0]).toEqual([{ taskId: 'tg-1', hosts: ['host-aff-1'] }])
    expect(pushMock).toHaveBeenCalledWith('/vulnerability/tasks')
  })
})
