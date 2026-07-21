import type { Conclusion, ExecutionResult, StepResult } from '@/api/aiAnalysis'
import { translate } from '@/i18n'

type TagType = 'success' | 'danger' | 'warning' | 'info' | 'primary'
type TaskStatusCode = 'completed' | 'failed' | 'interrupted' | 'limited' | 'running' | 'pending' | 'cancelled' | 'unknown'
type StepStatusCode = 'completed' | 'failed' | 'running' | 'pending' | 'skipped' | 'retrying' | 'unknown'

const taskStatusAliases: Record<string, TaskStatusCode> = {
  completed: 'completed', '已完成': 'completed', failed: 'failed', '失败': 'failed', '执行失败': 'failed', interrupted: 'interrupted', '已中断': 'interrupted', limited: 'limited', '已限制': 'limited', '已受限': 'limited', running: 'running', '执行中': 'running', pending: 'pending', '等待中': 'pending', cancelled: 'cancelled', '已取消': 'cancelled',
}
const taskStatusTypes: Record<TaskStatusCode, TagType> = { completed: 'success', failed: 'danger', interrupted: 'warning', limited: 'warning', running: 'primary', pending: 'info', cancelled: 'info', unknown: 'info' }

const stepStatusAliases: Record<string, StepStatusCode> = {
  completed: 'completed', '已完成': 'completed', '完成': 'completed', failed: 'failed', '失败': 'failed', running: 'running', '执行中': 'running', pending: 'pending', '等待中': 'pending', skipped: 'skipped', '跳过': 'skipped', '已跳过': 'skipped', retrying: 'retrying', '正在重试': 'retrying',
}
const stepStatusTypes: Record<StepStatusCode, TagType> = { completed: 'success', failed: 'danger', running: 'primary', pending: 'info', skipped: 'info', retrying: 'warning', unknown: 'info' }

const exitReasonKeys: Record<string, string> = {
  normal_completed: 'normalCompleted', '正常完成': 'normalCompleted', max_iterations: 'maxIterations', '达到最大轮次': 'maxIterations', timeout: 'timeout', '执行超时': 'timeout', user_cancelled: 'userCancelled', '用户取消': 'userCancelled', cancelled: 'cancelled', '已取消': 'cancelled', error: 'error', '执行错误': 'error', audit_rejected: 'auditRejected', '审计拒绝': 'auditRejected', drift_detected: 'driftDetected', '检测到计划漂移': 'driftDetected', tool_failed: 'toolFailed', '工具执行失败': 'toolFailed', rate_limit: 'rateLimit', '速率限制': 'rateLimit',
}

const verdictTypes: Record<string, TagType> = { benign: 'success', malicious: 'danger', suspicious: 'warning', unknown: 'info' }
const executionResultMarkers = [/^Task status:/im, /^Exit reason:/im, /^(Completed|Failed|Running|Skipped)\s+step[_-]?\w*:/im, /^Errors?:/im]

function hasText(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0
}

function taskStatusCode(status: string | undefined): TaskStatusCode {
  if (!hasText(status)) return 'unknown'
  const value = status.trim()
  return taskStatusAliases[value] || taskStatusAliases[value.toLowerCase()] || 'unknown'
}

function stepStatusCode(status: string | undefined): StepStatusCode {
  if (!hasText(status)) return 'unknown'
  const value = status.trim()
  return stepStatusAliases[value] || stepStatusAliases[value.toLowerCase()] || 'unknown'
}

export function normalizeTaskStatus(status: string | undefined) {
  return translate(`execution.taskStatus.${taskStatusCode(status)}`)
}

export function normalizeStepStatus(status: string | undefined) {
  return translate(`execution.stepStatus.${stepStatusCode(status)}`)
}

export function normalizeExitReason(reason: string | undefined) {
  if (!hasText(reason)) return ''
  const value = reason.trim()
  const key = exitReasonKeys[value] || exitReasonKeys[value.toLowerCase()] || 'unknown'
  return translate(`execution.exitReason.${key}`)
}

export function executionStatusType(status: string | undefined): TagType {
  return taskStatusTypes[taskStatusCode(status)]
}

export function stepStatusType(status: string | undefined): TagType {
  return stepStatusTypes[stepStatusCode(status)]
}

export function normalizeVerdict(value: string | undefined): 'benign' | 'malicious' | 'suspicious' | 'unknown' {
  if (!hasText(value)) return 'unknown'
  const text = value.toLowerCase()
  if (text.includes('benign') || text.includes('false positive') || value.includes('良性') || value.includes('误报')) return 'benign'
  if (text.includes('malicious') || text.includes('confirmed threat') || text.includes('threat') || value.includes('恶意')) return 'malicious'
  if (text.includes('suspicious') || value.includes('可疑')) return 'suspicious'
  return 'unknown'
}

export function verdictType(verdict: string | undefined): TagType {
  return verdictTypes[normalizeVerdict(verdict)] || 'info'
}

export function verdictLabel(verdict: string | undefined) {
  return translate(`execution.verdict.${normalizeVerdict(verdict)}`)
}

function translateKnownVerdictText(value: string | undefined) {
  if (!hasText(value)) return ''
  const verdict = normalizeVerdict(value)
  const compact = value.trim().replace(/\s+/g, ' ')
  if (verdict !== 'unknown' && /^(benign|false positive|benign \/ false positive|malicious|suspicious)$/i.test(compact)) return verdictLabel(verdict)
  return value.trim()
}

function splitErrors(value: string | undefined) {
  if (!hasText(value)) return []
  return value.split(/;|\n/).map(item => item.trim()).filter(Boolean)
}

function lastMeaningfulStepResult(steps: StepResult[]) {
  for (let i = steps.length - 1; i >= 0; i -= 1) {
    const result = steps[i]?.result?.trim()
    if (result) return result
  }
  return ''
}

function buildFallbackConclusion(status: TaskStatusCode, errors: string[], steps: StepResult[]): Conclusion {
  const reasoning = lastMeaningfulStepResult(steps) || errors.slice(0, 3).join('; ')
  let key = 'unknown'
  if (status === 'completed') key = errors.length === 0 ? 'completed' : 'completedWithErrors'
  else if (status === 'failed') key = 'failed'
  else if (status === 'interrupted' || status === 'cancelled') key = 'interrupted'
  return { verdict: 'unknown', summary: translate(`execution.conclusion.${key}`), reasoning }
}

function inferConclusion(raw: Conclusion | undefined, status: TaskStatusCode, errors: string[], steps: StepResult[]): Conclusion {
  const stepVerdictSource = [...steps].reverse().map(step => step.result || '').find(result => normalizeVerdict(result) !== 'unknown')
  const rawVerdict = normalizeVerdict(raw?.verdict || raw?.summary || raw?.reasoning || stepVerdictSource)
  const fallback = buildFallbackConclusion(status, errors, steps)
  if (rawVerdict === 'unknown' && !hasText(raw?.summary) && !hasText(raw?.reasoning)) return fallback
  return {
    verdict: rawVerdict,
    summary: translateKnownVerdictText(raw?.summary) || (rawVerdict !== 'unknown' ? verdictLabel(rawVerdict) : fallback.summary),
    reasoning: translateKnownVerdictText(raw?.reasoning) || stepVerdictSource || fallback.reasoning,
  }
}

export function normalizeExecutionResult(raw: Partial<ExecutionResult>): ExecutionResult {
  const statusCode = taskStatusCode(raw.status)
  const steps = (raw.steps || []).map((step): StepResult => ({ step_id: step.step_id || '', status: normalizeStepStatus(step.status), result: translateKnownVerdictText(step.result), started_at: step.started_at || '', ended_at: step.ended_at || '', duration_ms: Number(step.duration_ms || 0) }))
  const errors = Array.isArray(raw.errors) ? raw.errors.filter(hasText).map(item => item.trim()) : []
  return { execution_id: raw.execution_id || '', task_id: raw.task_id || '', session_id: raw.session_id || '', status: translate(`execution.taskStatus.${statusCode}`), exit_reason: normalizeExitReason(raw.exit_reason), started_at: raw.started_at || '', ended_at: raw.ended_at || '', total_duration_ms: Number(raw.total_duration_ms || 0), steps, errors, conclusion: inferConclusion(raw.conclusion, statusCode, errors, steps) }
}

function stepStatusFromPrefix(prefix: string) {
  const normalized = prefix.toLowerCase()
  if (normalized === 'failed') return 'failed'
  if (normalized === 'running') return 'running'
  if (normalized === 'skipped') return 'skipped'
  return 'completed'
}

export function parseExecutionResultText(text: string): ExecutionResult | null {
  if (!hasText(text) || !executionResultMarkers.some(marker => marker.test(text))) return null
  const steps: StepResult[] = []
  const errors: string[] = []
  const result: Partial<ExecutionResult> = { status: '', exit_reason: '', total_duration_ms: 0, steps, errors }
  let activeStep: StepResult | null = null
  let collectingErrors = false
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim()
    if (!line) continue
    const statusMatch = line.match(/^Task status:\s*(.+)$/i)
    if (statusMatch) { result.status = statusMatch[1].trim(); activeStep = null; collectingErrors = false; continue }
    const exitReasonMatch = line.match(/^Exit reason:\s*(.+)$/i)
    if (exitReasonMatch) { result.exit_reason = exitReasonMatch[1].trim(); activeStep = null; collectingErrors = false; continue }
    const stepMatch = line.match(/^(Completed|Failed|Running|Skipped)\s+(step[_-]?\w*):\s*(.*)$/i)
    if (stepMatch) { activeStep = { step_id: stepMatch[2], status: stepStatusFromPrefix(stepMatch[1]), result: stepMatch[3].trim(), started_at: '', ended_at: '', duration_ms: 0 }; steps.push(activeStep); collectingErrors = false; continue }
    const errorsMatch = line.match(/^Errors?:\s*(.*)$/i)
    if (errorsMatch) { errors.push(...splitErrors(errorsMatch[1])); activeStep = null; collectingErrors = true; continue }
    if (collectingErrors) errors.push(...splitErrors(line))
    else if (activeStep) activeStep.result = `${activeStep.result}\n${line}`.trim()
  }
  return normalizeExecutionResult(result)
}
