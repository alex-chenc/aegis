// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ExecutionPlan from './ExecutionPlan.vue'
import type { PlanEvent } from '@/api/aiAnalysis'

const plan: PlanEvent = {
  id: 'plan-1',
  plan_id: 'plan-1',
  goal: '分析在线主机整体安全状态',
  total_steps: 2,
  steps: [
    {
      id: 'step-1',
      step_id: 'step-1',
      title: '定位全部目标主机',
      description: '定位全部目标主机',
      objective: '获取在线主机范围',
      tool_names: ['Host.List', 'Host.AgentStatus.Get'],
      suggested_tools: ['Host.List', 'Host.AgentStatus.Get'],
      status: 'completed',
      result_summary: '已获取 2 台在线主机',
    },
    {
      id: 'step-2',
      step_id: 'step-2',
      title: '逐台研判主机风险',
      description: '逐台研判主机风险',
      objective: '输出每台主机风险',
      tool_names: ['Detection.Alert.List'],
      suggested_tools: ['Detection.Alert.List'],
      status: 'running',
      result_summary: '',
    },
  ],
}

const globalStubs = {
  'el-icon': { template: '<span><slot /></span>' },
  'el-tag': { template: '<span class="el-tag"><slot /></span>' },
  'el-timeline': { template: '<div><slot /></div>' },
  'el-timeline-item': { template: '<div><slot /></div>' },
  List: true,
  ArrowDown: true,
  Aim: true,
  Clock: true,
}

describe('ExecutionPlan', () => {
  it('renders only step titles in title-only mode', () => {
    const wrapper = mount(ExecutionPlan, {
      props: {
        plan,
        titleOnly: true,
        audits: [{ decision: 'pass', risk_level: 'low', findings: ['审计详情'] }],
      },
      global: {
        stubs: globalStubs,
      },
    })

    expect(wrapper.text()).toContain('定位全部目标主机')
    expect(wrapper.text()).toContain('逐台研判主机风险')
    expect(wrapper.text()).toContain('1/2')
    expect(wrapper.text()).toContain('完成')
    expect(wrapper.text()).toContain('执行中')
    expect(wrapper.text()).not.toContain('分析在线主机整体安全状态')
    expect(wrapper.text()).not.toContain('Host.List')
    expect(wrapper.text()).not.toContain('已获取 2 台在线主机')
    expect(wrapper.text()).not.toContain('审计详情')
  })
})
