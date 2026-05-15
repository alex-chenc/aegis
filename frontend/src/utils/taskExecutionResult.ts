import type { Conclusion, ExecutionResult, StepResult } from '@/api/aiAnalysis'

type TagType = 'success' | 'danger' | 'warning' | 'info' | 'primary'

const taskStatusLabels: Record<string, string> = {
  completed: '已完成',
  failed: '执行失败',
  interrupted: '已中断',
  limited: '已受限',
  running: '执行中',
  pending: '等待中',
  cancelled: '已取消',
  已完成: '已完成',
  失败: '执行失败',
  执行失败: '执行失败',
  已中断: '已中断',
  已限制: '已受限',
  已受限: '已受限',
  执行中: '执行中',
  等待中: '等待中',
  已取消: '已取消'
}

const taskStatusTypes: Record<string, TagType> = {
  已完成: 'success',
  执行失败: 'danger',
  已中断: 'warning',
  已受限: 'warning',
  执行中: 'primary',
  等待中: 'info',
  已取消: 'info',
  未知状态: 'info'
}

const stepStatusLabels: Record<string, string> = {
  completed: '已完成',
  failed: '失败',
  running: '执行中',
  pending: '等待中',
  skipped: '已跳过',
  retrying: '正在重试',
  已完成: '已完成',
  完成: '已完成',
  失败: '失败',
  执行中: '执行中',
  等待中: '等待中',
  跳过: '已跳过',
  已跳过: '已跳过',
  正在重试: '正在重试'
}

const stepStatusTypes: Record<string, TagType> = {
  已完成: 'success',
  失败: 'danger',
  执行中: 'primary',
  等待中: 'info',
  已跳过: 'info',
  正在重试: 'warning',
  未知: 'info'
}

const exitReasonLabels: Record<string, string> = {
  normal_completed: '正常完成',
  max_iterations: '达到最大轮次',
  timeout: '执行超时',
  user_cancelled: '用户取消',
  cancelled: '已取消',
  error: '执行错误',
  audit_rejected: '审计拒绝',
  drift_detected: '检测到计划漂移',
  tool_failed: '工具执行失败',
  rate_limit: '速率限制',
  正常完成: '正常完成',
  达到最大轮次: '达到最大轮次',
  执行超时: '执行超时',
  用户取消: '用户取消',
  已取消: '已取消',
  执行错误: '执行错误',
  审计拒绝: '审计拒绝',
  检测到计划漂移: '检测到计划漂移',
  工具执行失败: '工具执行失败',
  速率限制: '速率限制'
}

const verdictLabels: Record<string, string> = {
  benign: '良性/误报',
  malicious: '恶意',
  suspicious: '可疑',
  unknown: '未知'
}

const verdictTypes: Record<string, TagType> = {
  benign: 'success',
  malicious: 'danger',
  suspicious: 'warning',
  unknown: 'info'
}

const executionResultMarkers = [
  /^Task status:/im,
  /^Exit reason:/im,
  /^(Completed|Failed|Running|Skipped)\s+step[_-]?\w*:/im,
  /^Errors?:/im
]

function hasText(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0
}

function normalizeLookup(value: string | undefined, labels: Record<string, string>, fallback: string) {
  if (!hasText(value)) return ''
  const trimmed = value.trim()
  return labels[trimmed] || labels[trimmed.toLowerCase()] || fallback
}

export function normalizeTaskStatus(status: string | undefined) {
  return normalizeLookup(status, taskStatusLabels, '未知状态') || '未知状态'
}

export function normalizeStepStatus(status: string | undefined) {
  return normalizeLookup(status, stepStatusLabels, '未知') || '未知'
}

export function normalizeExitReason(reason: string | undefined) {
  return normalizeLookup(reason, exitReasonLabels, '未知原因')
}

export function executionStatusType(status: string | undefined): TagType {
  return taskStatusTypes[normalizeTaskStatus(status)] || 'info'
}

export function stepStatusType(status: string | undefined): TagType {
  return stepStatusTypes[normalizeStepStatus(status)] || 'info'
}

export function normalizeVerdict(value: string | undefined): 'benign' | 'malicious' | 'suspicious' | 'unknown' {
  if (!hasText(value)) return 'unknown'

  const text = value.toLowerCase()
  if (text.includes('benign') || text.includes('false positive') || value.includes('良性') || value.includes('误报')) {
    return 'benign'
  }
  if (text.includes('suspicious') || text.includes('potentially malicious') || value.includes('可疑')) {
    return 'suspicious'
  }
  if (text.includes('malicious') || text.includes('confirmed threat') || text.includes('threat') || value.includes('恶意')) {
    return 'malicious'
  }
  return 'unknown'
}

export function verdictType(verdict: string | undefined): TagType {
  return verdictTypes[normalizeVerdict(verdict)] || 'info'
}

export function verdictLabel(verdict: string | undefined) {
  return verdictLabels[normalizeVerdict(verdict)]
}

function translateKnownVerdictText(value: string | undefined) {
  if (!hasText(value)) return ''
  const verdict = normalizeVerdict(value)
  const compact = value.trim().replace(/\s+/g, ' ')

  if (verdict !== 'unknown' && /^(benign|false positive|benign \/ false positive|malicious|suspicious)$/i.test(compact)) {
    return verdictLabels[verdict]
  }
  return value.trim()
}

function splitErrors(value: string | undefined) {
  if (!hasText(value)) return []
  return value
    .split(/;|\n/)
    .map(item => item.trim())
    .filter(Boolean)
}

function lastMeaningfulStepResult(steps: StepResult[]) {
  for (let i = steps.length - 1; i >= 0; i -= 1) {
    const result = steps[i]?.result?.trim()
    if (result) return result
  }
  return ''
}

function buildFallbackConclusion(status: string, errors: string[], steps: StepResult[]): Conclusion {
  const reasoning = lastMeaningfulStepResult(steps) || errors.slice(0, 3).join('; ')

  if (status === '已完成' && errors.length === 0) {
    return {
      verdict: 'unknown',
      summary: '执行完成，未发现明确异常结论',
      reasoning
    }
  }

  if (status === '已完成' && errors.length > 0) {
    return {
      verdict: 'unknown',
      summary: '执行完成，但存在采集或检查错误，需要结合错误信息复核',
      reasoning
    }
  }

  if (status === '执行失败') {
    return {
      verdict: 'unknown',
      summary: '执行失败，无法形成可靠安全结论',
      reasoning
    }
  }

  if (status === '已中断' || status === '已取消') {
    return {
      verdict: 'unknown',
      summary: '任务未完整执行，当前结果仅供参考',
      reasoning
    }
  }

  return {
    verdict: 'unknown',
    summary: '暂无明确安全结论',
    reasoning
  }
}

function inferConclusion(raw: Conclusion | undefined, status: string, errors: string[], steps: StepResult[]): Conclusion {
  const stepVerdictSource = [...steps]
    .reverse()
    .map(step => step.result || '')
    .find(result => normalizeVerdict(result) !== 'unknown')
  const rawVerdict = normalizeVerdict(raw?.verdict || raw?.summary || raw?.reasoning || stepVerdictSource)
  const fallback = buildFallbackConclusion(status, errors, steps)

  if (rawVerdict === 'unknown' && !hasText(raw?.summary) && !hasText(raw?.reasoning)) {
    return fallback
  }

  const summary = translateKnownVerdictText(raw?.summary) || (rawVerdict !== 'unknown' ? verdictLabels[rawVerdict] : fallback.summary)
  const reasoning = translateKnownVerdictText(raw?.reasoning) || stepVerdictSource || fallback.reasoning

  return {
    verdict: rawVerdict,
    summary,
    reasoning
  }
}

export function normalizeExecutionResult(raw: Partial<ExecutionResult>): ExecutionResult {
  const status = normalizeTaskStatus(raw.status)
  const exitReason = normalizeExitReason(raw.exit_reason)
  const steps = (raw.steps || []).map((step): StepResult => ({
    step_id: step.step_id || '',
    status: normalizeStepStatus(step.status),
    result: translateKnownVerdictText(step.result),
    started_at: step.started_at || '',
    ended_at: step.ended_at || '',
    duration_ms: Number(step.duration_ms || 0)
  }))
  const errors = Array.isArray(raw.errors) ? raw.errors.filter(hasText).map(item => item.trim()) : []

  return {
    execution_id: raw.execution_id || '',
    task_id: raw.task_id || '',
    session_id: raw.session_id || '',
    status,
    exit_reason: exitReason,
    started_at: raw.started_at || '',
    ended_at: raw.ended_at || '',
    total_duration_ms: Number(raw.total_duration_ms || 0),
    steps,
    errors,
    conclusion: inferConclusion(raw.conclusion, status, errors, steps)
  }
}

function stepStatusFromPrefix(prefix: string) {
  const normalized = prefix.toLowerCase()
  if (normalized === 'failed') return 'failed'
  if (normalized === 'running') return 'running'
  if (normalized === 'skipped') return 'skipped'
  return 'completed'
}

export function parseExecutionResultText(text: string): ExecutionResult | null {
  if (!hasText(text) || !executionResultMarkers.some(marker => marker.test(text))) {
    return null
  }

  const steps: StepResult[] = []
  const errors: string[] = []
  const result: Partial<ExecutionResult> = {
    status: '',
    exit_reason: '',
    total_duration_ms: 0,
    steps,
    errors
  }
  let activeStep: StepResult | null = null
  let collectingErrors = false

  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim()
    if (!line) continue

    const statusMatch = line.match(/^Task status:\s*(.+)$/i)
    if (statusMatch) {
      result.status = statusMatch[1].trim()
      activeStep = null
      collectingErrors = false
      continue
    }

    const exitReasonMatch = line.match(/^Exit reason:\s*(.+)$/i)
    if (exitReasonMatch) {
      result.exit_reason = exitReasonMatch[1].trim()
      activeStep = null
      collectingErrors = false
      continue
    }

    const stepMatch = line.match(/^(Completed|Failed|Running|Skipped)\s+(step[_-]?\w*):\s*(.*)$/i)
    if (stepMatch) {
      activeStep = {
        step_id: stepMatch[2],
        status: stepStatusFromPrefix(stepMatch[1]),
        result: stepMatch[3].trim(),
        started_at: '',
        ended_at: '',
        duration_ms: 0
      }
      steps.push(activeStep)
      collectingErrors = false
      continue
    }

    const errorsMatch = line.match(/^Errors?:\s*(.*)$/i)
    if (errorsMatch) {
      errors.push(...splitErrors(errorsMatch[1]))
      activeStep = null
      collectingErrors = true
      continue
    }

    if (collectingErrors) {
      errors.push(...splitErrors(line))
    } else if (activeStep) {
      activeStep.result = `${activeStep.result}\n${line}`.trim()
    }
  }

  return normalizeExecutionResult(result)
}
