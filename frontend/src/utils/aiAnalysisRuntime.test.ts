import { describe, expect, it } from 'vitest'
import { applyPlanStepStatus, isPlanTerminal, normalizePlanEvent } from './aiAnalysisRuntime'

describe('aiAnalysisRuntime helpers', () => {
  it('normalizes agent-runtime plan fields for the execution plan UI', () => {
    const plan = normalizePlanEvent({
      plan_id: 'plan-1',
      goal: '分析告警链路',
      steps: [
        {
          step_id: 'step-1',
          title: '查询进程树',
          objective: '确认可疑进程的父子关系',
          suggested_tools: ['GetProcessTree'],
          status: 'completed'
        },
        {
          step_id: 'step-2',
          title: '',
          objective: '查询网络连接',
          suggested_tools: ['GetNetworkConnections']
        }
      ]
    })

    expect(plan.id).toBe('plan-1')
    expect(plan.steps[0]).toMatchObject({
      id: 'step-1',
      step_id: 'step-1',
      title: '查询进程树',
      description: '查询进程树',
      objective: '确认可疑进程的父子关系',
      tool_names: ['GetProcessTree'],
      status: 'pending'
    })
    expect(plan.steps[1].description).toBe('查询网络连接')
  })

  it('updates steps by either normalized id or runtime step_id and detects terminal state', () => {
    const plan = normalizePlanEvent({
      plan_id: 'plan-2',
      goal: '分析告警链路',
      steps: [
        { step_id: 'step-1', title: '查询进程树' },
        { step_id: 'step-2', title: '查询网络连接' }
      ]
    })

    expect(isPlanTerminal(plan)).toBe(false)

    applyPlanStepStatus(plan, 'step-1', 'completed', '进程树确认完成')
    applyPlanStepStatus(plan, 'step-2', 'failed', '主机离线')

    expect(plan.steps[0].status).toBe('completed')
    expect(plan.steps[0].result_summary).toBe('进程树确认完成')
    expect(plan.steps[1].status).toBe('failed')
    expect(isPlanTerminal(plan)).toBe(true)
  })
})
