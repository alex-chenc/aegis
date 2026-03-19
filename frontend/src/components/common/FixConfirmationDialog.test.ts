// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { defineComponent, h, nextTick, reactive } from 'vue'
import { mount } from '@vue/test-utils'
import FixConfirmationDialog from './FixConfirmationDialog.vue'

const hostStoreState = reactive({
  hosts: [
    { id: 'host-all-1', ip_address: '10.0.0.1', hostname: 'all-1', os_type: 'linux' },
    { id: 'host-all-2', ip_address: '10.0.0.2', hostname: 'all-2', os_type: 'linux' }
  ]
})

const fetchHostsMock = vi.fn().mockResolvedValue(undefined)

vi.mock('@/store/hosts', () => ({
  useHostStore: () => ({
    hosts: hostStoreState.hosts,
    fetchHosts: fetchHostsMock
  })
}))

vi.mock('@/api/vulnerability', () => ({
  getGenerationStatus: vi.fn().mockResolvedValue({ has_generation: false })
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

const ElSelectStub = defineComponent({
  name: 'ElSelectStub',
  props: {
    modelValue: {
      type: [Array, String],
      default: () => []
    }
  },
  setup(_, { slots }) {
    return () => h('div', { class: 'select' }, slots.default?.())
  }
})

const ElOptionStub = defineComponent({
  name: 'ElOptionStub',
  props: {
    value: {
      type: String,
      required: true
    },
    label: {
      type: String,
      required: true
    }
  },
  setup(props) {
    return () => h('div', { class: 'host-option', 'data-value': props.value }, props.label)
  }
})

const HostScriptStatusListStub = defineComponent({
  name: 'HostScriptStatusList',
  props: {
    selectedHosts: {
      type: Array,
      default: () => []
    }
  },
  setup(props) {
    return () => h('div', { class: 'host-script-status-list-stub' }, String(props.selectedHosts.length))
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
    source: 'llm_analysis'
  },
  affectedHosts: [
    { id: 'host-aff-1', ip_address: '192.168.1.1', hostname: 'aff-1', os_type: 'linux' }
  ]
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
        'el-select': ElSelectStub,
        'el-option': ElOptionStub,
        'el-alert': true,
        'el-empty': true,
        'el-button': true,
        SeverityTag: true,
        HostScriptStatusList: HostScriptStatusListStub
      }
    }
  })
}

describe('FixConfirmationDialog source hosts and multiselect', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('uses affected hosts for scanned cve source', async () => {
    const wrapper = mountDialog({
      cve: {
        ...baseProps.cve,
        source: 'llm_analysis'
      }
    })

    await nextTick()

    const options = wrapper.findAll('.host-option')
    expect(options).toHaveLength(1)
    expect(options[0].text()).toContain('192.168.1.1')
  })

  it('uses all hosts for custom cve source', async () => {
    const wrapper = mountDialog({
      cve: {
        ...baseProps.cve,
        source: 'custom_query'
      }
    })

    await nextTick()

    const options = wrapper.findAll('.host-option')
    expect(options).toHaveLength(2)
    expect(wrapper.text()).toContain('10.0.0.1')
    expect(wrapper.text()).toContain('10.0.0.2')
  })

  it('enables multiple selection for both poc and fix modes', async () => {
    const pocWrapper = mountDialog({ mode: 'poc' })
    const fixWrapper = mountDialog({ mode: 'fix' })

    await nextTick()

    const pocSelect = pocWrapper.findComponent(ElSelectStub)
    const fixSelect = fixWrapper.findComponent(ElSelectStub)

    expect(pocSelect.attributes('multiple')).toBeDefined()
    expect(fixSelect.attributes('multiple')).toBeDefined()
  })

  it('renders host script status list after selecting hosts', async () => {
    const wrapper = mountDialog({ mode: 'poc' })
    await nextTick()

    await (wrapper.vm as any).onHostSelectionChange(['host-aff-1'])
    await nextTick()

    expect(wrapper.find('.host-script-status-list-stub').exists()).toBe(true)
  })
})
