import type { SessionListItem } from '@/api/aiAnalysis'
import { translate } from '@/i18n'

/**
 * 根据conclusion字段判定会话显示状态
 * 只有conclusion不为空才算"已完成"，其他都是"未完成"
 * 支持conclusion为对象或JSON字符串的情况
 */
export function getDisplayStatus(session: Pick<SessionListItem, 'conclusion'>): 'completed' | 'active' {
  if (!session.conclusion) return 'active'

  // 如果是字符串，尝试解析
  if (typeof session.conclusion === 'string') {
    try {
      const parsed = JSON.parse(session.conclusion)
      return parsed && Object.keys(parsed).length > 0 ? 'completed' : 'active'
    } catch {
      return session.conclusion.trim().length > 0 ? 'completed' : 'active'
    }
  }

  // 如果是对象
  if (typeof session.conclusion === 'object') {
    return Object.keys(session.conclusion).length > 0 ? 'completed' : 'active'
  }

  return 'active'
}

/**
 * 判断是否为误报
 */
export function isFalsePositive(verdict: string): boolean {
  return verdict === 'benign' || verdict === 'false_positive'
}

/**
 * 获取处置建议
 */
export function getRemediationSuggestion(verdict: string): string {
  switch (verdict) {
    case 'malicious':
      return translate('analysis.remediation.malicious')
    case 'suspicious':
      return translate('analysis.remediation.suspicious')
    case 'unknown':
      return translate('analysis.remediation.unknown')
    default:
      return translate('analysis.remediation.default')
  }
}

/**
 * 获取结论显示类型
 */
export function getVerdictType(verdict: string): 'success' | 'danger' | 'warning' | 'info' {
  switch (verdict) {
    case 'benign':
    case 'false_positive':
      return 'success'
    case 'malicious':
      return 'danger'
    case 'suspicious':
      return 'warning'
    default:
      return 'info'
  }
}

/**
 * 获取结论显示文本
 */
export function getVerdictText(verdict: string): string {
  switch (verdict) {
    case 'benign':
    case 'false_positive':
      return translate('execution.verdict.benign')
    case 'malicious':
      return translate('execution.verdict.malicious')
    case 'suspicious':
      return translate('execution.verdict.suspicious')
    default:
      return translate('execution.verdict.unknown')
  }
}
