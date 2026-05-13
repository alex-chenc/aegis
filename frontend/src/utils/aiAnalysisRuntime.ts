import type { PlanEvent, PlanStep } from '@/api/aiAnalysis'

type RuntimePlan = Record<string, any>

const terminalStatuses = new Set(['completed', 'failed', 'skipped', 'replaced', 'invalidated'])

function fallbackStepId(index: number) {
  return `step-${index + 1}`
}

function normalizeTools(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === 'string' && item.trim() !== '')
}

export function normalizePlanEvent(raw: RuntimePlan | null | undefined): PlanEvent | null {
  if (!raw) return null

  const steps = Array.isArray(raw.steps) ? raw.steps : []
  return {
    ...raw,
    id: raw.id || raw.plan_id || '',
    plan_id: raw.plan_id || raw.id || '',
    goal: raw.goal || '',
    total_steps: raw.total_steps || steps.length,
    steps: steps.map((step: RuntimePlan, index: number): PlanStep => {
      const stepId = step.step_id || step.id || fallbackStepId(index)
      const title = String(step.title || step.description || step.objective || `步骤 ${index + 1}`)
      const objective = String(step.objective || '')
      const tools = normalizeTools(step.suggested_tools || step.tool_names)

      return {
        ...step,
        id: stepId,
        step_id: stepId,
        title,
        description: title,
        objective,
        tool_names: tools,
        suggested_tools: tools,
        status: 'pending',
        result_summary: step.result_summary || ''
      }
    })
  } as PlanEvent
}

export function findPlanStep(plan: PlanEvent | null | undefined, stepId?: string): PlanStep | undefined {
  if (!plan || !stepId) return undefined
  return plan.steps.find(step => step.id === stepId || step.step_id === stepId)
}

export function applyPlanStepStatus(
  plan: PlanEvent | null | undefined,
  stepId: string | undefined,
  status: PlanStep['status'],
  resultSummary = ''
) {
  const step = findPlanStep(plan, stepId)
  if (!step) return false
  step.status = status
  if (resultSummary) {
    step.result_summary = resultSummary
  }
  return true
}

export function isPlanTerminal(plan: PlanEvent | null | undefined) {
  if (!plan || plan.steps.length === 0) return false
  return plan.steps.every(step => terminalStatuses.has(step.status))
}
