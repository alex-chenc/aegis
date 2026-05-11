// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, defineComponent, h } from 'vue'
import AuditLogTable from './AuditLogTable.vue'
import type { AuditLog } from '@/api/audit-logs'
import { auditSourceLabels, scriptTypeLabels } from '../constants'

const { confirmMock } = vi.hoisted(() => ({
  confirmMock: vi.fn().mockResolvedValue('confirm')
}))

vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), warning: vi.fn(), error: vi.fn() },
  ElMessageBox: { confirm: confirmMock }
}))

const mockLogs: AuditLog[] = [
  {
    id: 'aaaa-bbbb-cccc-dddd-0001',
    task_id: 'task-001',
    rule_id: 'rule-001',
    script_type: 'check',
    audit_source: 'generation',
    attempt: 1,
    passed: true,
    risk_level: 'safe',
    duration_ms: 120,
    script_content: '#!/bin/bash\necho hello',
    blacklist_hits: [],
    ai_analysis: [],
    error_msg: '',
    created_at: '2026-05-08T10:00:00Z'
  },
  {
    id: 'aaaa-bbbb-cccc-dddd-0002',
    task_id: 'task-002',
    rule_id: 'rule-002',
    script_type: 'fix',
    audit_source: 'dispatch',
    attempt: 2,
    passed: false,
    risk_level: 'high',
    duration_ms: 350,
    script_content: '#!/bin/bash\nrm -rf /',
    blacklist_hits: [{ rule_name: 'dangerous_rm', line_number: 2, matched_text: 'rm -rf /' }],
    ai_analysis: [{ type: 'privilege_escalation', description: 'Dangerous command', line_range: '2', suggestion: 'Remove rm -rf' }],
    error_msg: '',
    created_at: '2026-05-08T11:00:00Z'
  },
  {
    id: 'aaaa-bbbb-cccc-dddd-0003',
    task_id: 'task-003',
    rule_id: 'rule-003',
    script_type: 'poc_verify',
    audit_source: 'agent',
    attempt: 1,
    passed: true,
    risk_level: 'safe',
    duration_ms: 80,
    script_content: '#!/bin/bash\ncurl example.com',
    blacklist_hits: [],
    ai_analysis: [],
    error_msg: '',
    created_at: '2026-05-08T12:00:00Z'
  }
]

// Stub that renders a minimal version without slot scope issues
const ElTableStub = defineComponent({
  name: 'ElTable',
  props: ['data', 'loading'],
  setup(props, { slots, expose }) {
    const clearSelection = vi.fn()
    expose({ clearSelection })
    return () => h('div', { class: 'el-table' }, [
      h('div', { class: 'el-table-body' },
        (props.data as any[])?.map((row: any) =>
          h('div', { class: 'el-table-row', 'data-id': row.id }, row.id)
        )
      ),
      slots.default?.()
    ])
  }
})

const ElTableColumnStub = defineComponent({
  name: 'ElTableColumn',
  props: ['type', 'width', 'label', 'prop'],
  setup(props) {
    return () => h('div', {
      class: 'el-table-column',
      'data-type': props.type || 'default',
      'data-label': props.label || ''
    })
  }
})

function mountTable(props: Record<string, unknown> = {}) {
  return mount(AuditLogTable, {
    props: {
      logs: mockLogs,
      loading: false,
      total: 3,
      ...props
    },
    global: {
      stubs: {
        'el-card': { template: '<div><slot name="header" /><slot /></div>' },
        'el-table': ElTableStub,
        'el-table-column': ElTableColumnStub,
        'el-tag': { template: '<span class="el-tag"><slot /></span>', props: ['type', 'size'] },
        'el-button': { template: '<button class="el-button" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>', props: ['type', 'size', 'link', 'disabled'] },
        'el-select': { template: '<select><slot /></select>', props: ['modelValue', 'placeholder', 'size', 'clearable', 'style'] },
        'el-option': { template: '<option><slot /></option>', props: ['label', 'value'] },
        'el-pagination': { template: '<div class="el-pagination"></div>', props: ['currentPage', 'pageSize', 'total', 'layout'] }
      }
    }
  })
}

describe('AuditLogTable multi-select delete', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    confirmMock.mockResolvedValue('confirm')
  })

  it('renders table columns including selection', () => {
    const wrapper = mountTable()
    const columns = wrapper.findAll('.el-table-column')
    expect(columns.length).toBeGreaterThanOrEqual(8) // selection + 7 data columns
    const selectionCol = columns.find(c => c.attributes('data-type') === 'selection')
    expect(selectionCol).toBeTruthy()
  })

  it('shows delete button in header', () => {
    const wrapper = mountTable()
    const deleteBtn = wrapper.find('[data-testid="batch-delete-btn"]')
    expect(deleteBtn.exists()).toBe(true)
    expect(deleteBtn.text()).toContain('删除')
  })

  it('delete button is disabled when no rows selected', () => {
    const wrapper = mountTable()
    const deleteBtn = wrapper.find('[data-testid="batch-delete-btn"]')
    expect(deleteBtn.attributes('disabled')).toBeDefined()
  })

  it('emits delete event with selected ids when delete confirmed', async () => {
    const wrapper = mountTable()
    const vm = wrapper.vm as any

    // Simulate table selection
    vm.handleSelectionChange([mockLogs[0], mockLogs[1]])
    await nextTick()

    const deleteBtn = wrapper.find('[data-testid="batch-delete-btn"]')
    expect(deleteBtn.attributes('disabled')).toBeUndefined()
    expect(deleteBtn.text()).toContain('2')

    await deleteBtn.trigger('click')
    await nextTick()

    expect(confirmMock).toHaveBeenCalled()
    expect(wrapper.emitted('delete')).toBeTruthy()
    expect(wrapper.emitted('delete')![0]).toEqual([['aaaa-bbbb-cccc-dddd-0001', 'aaaa-bbbb-cccc-dddd-0002']])
  })

  it('does not emit delete event when confirmation cancelled', async () => {
    confirmMock.mockReset()
    confirmMock.mockImplementation(() => Promise.reject('cancel'))
    const wrapper = mountTable()
    const vm = wrapper.vm as any

    vm.handleSelectionChange([mockLogs[0]])
    await nextTick()

    const deleteBtn = wrapper.find('[data-testid="batch-delete-btn"]')
    await deleteBtn.trigger('click')
    await nextTick()
    await nextTick()

    expect(confirmMock).toHaveBeenCalled()
    expect(wrapper.emitted('delete')).toBeFalsy()
  })

  it('shows selected count in delete button text', async () => {
    const wrapper = mountTable()
    const vm = wrapper.vm as any

    vm.handleSelectionChange([mockLogs[0], mockLogs[1], mockLogs[2]])
    await nextTick()

    const deleteBtn = wrapper.find('[data-testid="batch-delete-btn"]')
    expect(deleteBtn.text()).toContain('3')
  })

  it('emits filter event with correct params', async () => {
    const wrapper = mountTable()
    const vm = wrapper.vm as any

    vm.filters.result = 'failed'
    vm.handleFilter()
    await nextTick()

    expect(wrapper.emitted('filter')).toBeTruthy()
    const emittedParams = wrapper.emitted('filter')![0][0] as any
    expect(emittedParams.passed).toBe('false')
    expect(emittedParams.page).toBe(1)
  })

  it('formats timestamps correctly', () => {
    const wrapper = mountTable()
    const vm = wrapper.vm as any
    expect(vm.formatTime('2026-05-08T10:00:00Z')).toBe('2026-05-08 10:00:00')
    expect(vm.formatTime('')).toBe('-')
  })

  it('returns correct risk tag types', () => {
    const wrapper = mountTable()
    const vm = wrapper.vm as any
    expect(vm.riskTagType('critical')).toBe('danger')
    expect(vm.riskTagType('high')).toBe('warning')
    expect(vm.riskTagType('safe')).toBe('success')
    expect(vm.riskTagType('medium')).toBe('info')
  })

  it('clears selection after successful delete', async () => {
    const wrapper = mountTable()
    const vm = wrapper.vm as any

    vm.handleSelectionChange([mockLogs[0]])
    await nextTick()
    expect(vm.selectedRows).toHaveLength(1)

    const deleteBtn = wrapper.find('[data-testid="batch-delete-btn"]')
    await deleteBtn.trigger('click')
    await nextTick()

    expect(vm.selectedRows).toHaveLength(0)
  })

  it('handles pagination correctly', async () => {
    const wrapper = mountTable()
    const vm = wrapper.vm as any

    vm.handlePageChange(3)
    await nextTick()

    expect(wrapper.emitted('filter')).toBeTruthy()
    const emittedParams = wrapper.emitted('filter')![0][0] as any
    expect(emittedParams.page).toBe(3)
  })
})

describe('AuditLogTable audit source display', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('maps generation audit source to Chinese label', () => {
    expect(auditSourceLabels['generation']).toBe('生成阶段')
  })

  it('maps dispatch audit source to Chinese label', () => {
    expect(auditSourceLabels['dispatch']).toBe('下发阶段')
  })

  it('maps agent audit source to Chinese label', () => {
    expect(auditSourceLabels['agent']).toBe('Agent侧')
  })

  it('does not contain obsolete blacklist/ai source labels', () => {
    expect(auditSourceLabels['blacklist']).toBeUndefined()
    expect(auditSourceLabels['ai']).toBeUndefined()
  })

  it('emits correct audit_source value in filter event', async () => {
    const wrapper = mountTable()
    const vm = wrapper.vm as any

    vm.filters.audit_source = 'generation'
    vm.handleFilter()
    await nextTick()

    expect(wrapper.emitted('filter')).toBeTruthy()
    const emittedParams = wrapper.emitted('filter')![0][0] as any
    expect(emittedParams.audit_source).toBe('generation')
  })
})
