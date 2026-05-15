// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import TaskExecutionResult from './TaskExecutionResult.vue'
import type { ExecutionResult } from '@/api/aiAnalysis'

const PassThroughStub = defineComponent({
  name: 'PassThroughStub',
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  }
})

function mountResult(result: Partial<ExecutionResult>) {
  return mount(TaskExecutionResult, {
    props: {
      result: result as ExecutionResult
    },
    global: {
      stubs: {
        'el-icon': PassThroughStub,
        'el-timeline': PassThroughStub,
        'el-timeline-item': PassThroughStub,
        'el-tag': PassThroughStub,
        'el-card': PassThroughStub
      }
    }
  })
}

describe('TaskExecutionResult', () => {
  it('renders raw API enum values as Chinese cards, steps, errors, and conclusion', () => {
    const wrapper = mountResult({
      execution_id: 'exec-1',
      task_id: 'task-1',
      session_id: 'session-1',
      status: 'completed',
      exit_reason: 'normal_completed',
      total_duration_ms: 330000,
      steps: [
        {
          step_id: 'step_1',
          status: 'completed',
          result: '经分析，目标进程已退出且未留下任何活跃文件句柄。',
          started_at: '',
          ended_at: '',
          duration_ms: 5000
        }
      ],
      errors: ['process 4181522 not found'],
      conclusion: {
        verdict: 'benign',
        summary: 'Benign / False Positive',
        reasoning: '未发现异常外联或持久化迹象。'
      }
    })

    expect(wrapper.text()).toContain('执行状态')
    expect(wrapper.text()).toContain('已完成')
    expect(wrapper.text()).toContain('退出原因')
    expect(wrapper.text()).toContain('正常完成')
    expect(wrapper.text()).toContain('5分30秒')
    expect(wrapper.text()).toContain('步骤执行详情')
    expect(wrapper.text()).toContain('经分析，目标进程已退出')
    expect(wrapper.text()).toContain('错误信息 (1)')
    expect(wrapper.text()).toContain('process 4181522 not found')
    expect(wrapper.text()).toContain('分析结论')
    expect(wrapper.text()).toContain('良性/误报')
    expect(wrapper.text()).not.toContain('normal_completed')
    expect(wrapper.text()).not.toContain('Benign / False Positive')
  })

  it('hides empty duration and error sections', () => {
    const wrapper = mountResult({
      status: 'running',
      exit_reason: '',
      steps: [],
      errors: [],
      conclusion: {
        verdict: 'unknown',
        summary: '',
        reasoning: ''
      }
    })

    expect(wrapper.text()).toContain('执行中')
    expect(wrapper.text()).not.toContain('总耗时')
    expect(wrapper.text()).not.toContain('错误信息')
    expect(wrapper.text()).toContain('分析结论')
  })
})
