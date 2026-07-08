// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, nextTick } from 'vue'
import { shallowMount } from '@vue/test-utils'
import Workbench from './Workbench.vue'

const { getTemplatesMock, getTemplateRulesMock } = vi.hoisted(() => ({
  getTemplatesMock: vi.fn(),
  getTemplateRulesMock: vi.fn()
}))

vi.mock('@/api/templates', () => ({
  batchGenerateScripts: vi.fn(),
  checkFileMD5: vi.fn(),
  deleteTemplate: vi.fn(),
  generateScript: vi.fn(),
  getTemplateRules: getTemplateRulesMock,
  getTemplates: getTemplatesMock,
  getTemplateStatus: vi.fn(),
  updateScript: vi.fn(),
  uploadTemplate: vi.fn()
}))

vi.mock('@/store/hosts', () => ({
  useHostStore: () => ({
    hosts: [{ id: 'host-1', hostname: 'web-01', ip_address: '10.0.0.1', os_type: 'linux', online: true }],
    fetchHosts: vi.fn().mockResolvedValue(undefined)
  })
}))

vi.mock('@/store/tasks', () => ({
  useTaskStore: () => ({
    setSelectedRules: vi.fn(),
    setSelectedHosts: vi.fn(),
    executeCheck: vi.fn(),
    executeFix: vi.fn()
  })
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() })
}))

vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), info: vi.fn(), warning: vi.fn(), error: vi.fn() },
  ElMessageBox: { confirm: vi.fn() }
}))

const PassThroughStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  }
})

const DialogStub = defineComponent({
  props: { modelValue: Boolean },
  setup(props, { slots }) {
    return () => props.modelValue ? h('section', [slots.default?.(), slots.footer?.()]) : null
  }
})

const ButtonStub = defineComponent({
  emits: ['click'],
  setup(_, { slots, emit }) {
    return () => h('button', { onClick: () => emit('click') }, slots.default?.())
  }
})

const CheckboxStub = defineComponent({
  inheritAttrs: false,
  setup(_, { attrs, slots }) {
    return () => h('label', attrs, slots.default?.())
  }
})

function flushPromises() {
  return new Promise(resolve => setTimeout(resolve, 0))
}

describe('Workbench task dispatch rule labels', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getTemplatesMock.mockResolvedValue([
      {
        id: 'template-1',
        name: 'CIS_Ubuntu_24.04_LTS_Benchmark.pdf',
        display_name: 'CIS_Ubuntu_24.04_LTS_Benchmark_very_long_template_filename.pdf',
        file_type: 'pdf',
        status: 'completed',
        rule_count: 1,
        created_at: '2026-07-08T00:00:00Z',
        updated_at: '2026-07-08T00:00:00Z'
      }
    ])
    getTemplateRulesMock.mockResolvedValue([
      {
        id: 'rule-1',
        template_id: 'template-1',
        title: '确保 SSH Root 登录已禁用',
        check_content: '检查 sshd_config',
        fix_content: '设置 PermitRootLogin no',
        check_script_version: 1,
        fix_script_version: 1,
        check_script_status: 'generated',
        fix_script_status: 'generated'
      }
    ])
  })

  it('shows the rule title without rendering the template filename in the dispatch rule column', async () => {
    const wrapper = shallowMount(Workbench, {
      global: {
        stubs: {
          'el-dialog': DialogStub,
          'el-checkbox-group': PassThroughStub,
          'el-checkbox': CheckboxStub,
          'el-button': ButtonStub,
          'el-icon': PassThroughStub,
          'el-tag': PassThroughStub,
          'el-form': PassThroughStub,
          'el-form-item': PassThroughStub,
          'el-alert': PassThroughStub,
          'el-input': PassThroughStub,
          'el-option': PassThroughStub,
          'el-select': PassThroughStub,
          'el-progress': PassThroughStub,
          'el-card': PassThroughStub,
          'el-segmented': PassThroughStub,
          'el-table-column': PassThroughStub,
          'el-tooltip': PassThroughStub,
          'el-table': PassThroughStub,
          'el-pagination': PassThroughStub,
          'el-empty': PassThroughStub,
          'el-collapse-item': PassThroughStub,
          'el-collapse': PassThroughStub,
          'el-upload': PassThroughStub,
          'el-input-number': PassThroughStub,
          'el-switch': PassThroughStub,
          'el-tab-pane': PassThroughStub,
          'el-tabs': PassThroughStub
        },
        directives: { loading: () => undefined }
      }
    })

    await flushPromises()
    await flushPromises()
    expect(wrapper.find('.dispatch-check').exists()).toBe(false)
    const dispatchButton = wrapper.findAll('button').find(button => button.text().includes('任务下发'))
    expect(dispatchButton).toBeDefined()
    await dispatchButton!.trigger('click')
    await nextTick()

    const dispatchRule = wrapper.find('.dispatch-check')
    expect(dispatchRule.exists()).toBe(true)
    expect(dispatchRule.text()).toContain('确保 SSH Root 登录已禁用')
    expect(dispatchRule.text()).not.toContain('CIS_Ubuntu_24.04_LTS_Benchmark_very_long_template_filename.pdf')
  })
})
