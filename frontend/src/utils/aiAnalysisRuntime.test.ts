import { describe, expect, it } from 'vitest'
import { applyPlanStepStatus, getActionButtonType, isPlanTerminal, normalizePlanEvent } from './aiAnalysisRuntime'

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
      status: 'completed'
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

  it('preserves mixed step statuses from backend FinalPlan', () => {
    const plan = normalizePlanEvent({
      plan_id: 'plan-mixed',
      goal: 'test mixed statuses',
      steps: [
        { step_id: 's1', title: 'step 1', status: 'completed' },
        { step_id: 's2', title: 'step 2', status: 'completed' },
        { step_id: 's3', title: 'step 3', status: 'completed' },
        { step_id: 's4', title: 'step 4', status: 'running' },
        { step_id: 's5', title: 'step 5', status: 'pending' }
      ]
    })
    expect(plan.steps[0].status).toBe('completed')
    expect(plan.steps[1].status).toBe('completed')
    expect(plan.steps[2].status).toBe('completed')
    expect(plan.steps[3].status).toBe('running')
    expect(plan.steps[4].status).toBe('pending')
  })

  it('defaults unknown or missing step status to pending', () => {
    const plan = normalizePlanEvent({
      plan_id: 'plan-unknown',
      goal: 'test unknown statuses',
      steps: [
        { step_id: 's1', title: 'step 1' },
        { step_id: 's2', title: 'step 2', status: 'bogus' }
      ]
    })
    expect(plan.steps[0].status).toBe('pending')
    expect(plan.steps[1].status).toBe('pending')
  })

  it('isPlanTerminal returns false when steps have non-terminal statuses', () => {
    const plan = normalizePlanEvent({
      plan_id: 'plan-partial',
      goal: 'test partial',
      steps: [
        { step_id: 's1', title: 'step 1', status: 'completed' },
        { step_id: 's2', title: 'step 2', status: 'running' }
      ]
    })
    expect(isPlanTerminal(plan)).toBe(false)
  })

  it('isPlanTerminal returns true when all steps are in terminal statuses', () => {
    const plan = normalizePlanEvent({
      plan_id: 'plan-done',
      goal: 'test done',
      steps: [
        { step_id: 's1', title: 'step 1', status: 'completed' },
        { step_id: 's2', title: 'step 2', status: 'failed' },
        { step_id: 's3', title: 'step 3', status: 'skipped' }
      ]
    })
    expect(isPlanTerminal(plan)).toBe(true)
  })

  describe('getActionButtonType', () => {
    it('returns pause when loading and no input', () => {
      expect(getActionButtonType(true, false)).toBe('pause')
    })

    it('returns send when loading but user has input', () => {
      expect(getActionButtonType(true, true)).toBe('send')
    })

    it('returns send when not loading', () => {
      expect(getActionButtonType(false, false)).toBe('send')
      expect(getActionButtonType(false, true)).toBe('send')
    })
  })
})
