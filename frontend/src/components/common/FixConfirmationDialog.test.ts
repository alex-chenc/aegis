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

vi.mock('@/api/vulnerability', () => ({
  generateFixScript: vi.fn(),
  generatePocScript: vi.fn(),
  getScriptStatus: vi.fn()
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
  emits: ['update:modelValue', 'closed'],
  setup(props, { slots }) {
    return () => (props.modelValue ? h('div', { class: 'el-dialog-stub' }, slots.default?.()) : null)
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
  emits: ['update:modelValue'],
  setup(_, { slots }) {
    return () => h('div', { class: 'el-select-stub' }, slots.default?.())
  }
})

const ElButtonStub = defineComponent({
  name: 'ElButtonStub',
  props: {
    disabled: {
      type: Boolean,
      default: false
    }
  },
  setup(props, { slots }) {
    return () => h('button', { disabled: props.disabled }, slots.default?.())
  }
})

const baseProps = {
  visible: false,
  mode: 'fix' as const,
  cve: {
    id: 'vul-1',
    cve_id: 'CVE-2024-0001',
    severity: 'High' as const,
    cvss_score: 8.8,
    description: 'test'
  },
  affectedHosts: [
    {
      id: 'host-1',
      ip_address: '127.0.0.1',
      hostname: 'test-host'
    }
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
        'el-option': true,
        'el-alert': true,
        'el-button': ElButtonStub,
        SeverityTag: true,
        ScriptPreview: true
      }
    }
  })
}

describe('FixConfirmationDialog button visibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows Generate Script button when script is empty and hosts are selected', async () => {
    const wrapper = mountDialog({
      restoreStatus: {
        scriptId: null,
        status: 'idle',
        hostIds: ['host-1']
      }
    })

    await wrapper.setProps({ visible: true })
    await nextTick()

    expect(wrapper.text()).toContain('生成修复脚本')
  })

  it('does not show Generate Script button when script is empty and no hosts are selected', async () => {
    const wrapper = mountDialog()

    await wrapper.setProps({ visible: true })
    await nextTick()

    expect(wrapper.text()).not.toContain('生成修复脚本')
    expect(wrapper.text()).not.toContain('确认执行修复')
  })

  it('shows Execute button when script exists and hosts are selected', async () => {
    const wrapper = mountDialog({
      restoreStatus: {
        scriptId: 'script-1',
        status: 'generated',
        script: 'echo test',
        hostIds: ['host-1']
      }
    })

    await wrapper.setProps({ visible: true })
    await nextTick()

    expect(wrapper.text()).toContain('确认执行修复')
  })
})
