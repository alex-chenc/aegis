import type { SessionListItem } from '@/api/aiAnalysis'

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
      return '建议立即隔离受影响主机，进行深入取证分析，并检查横向移动迹象。'
    case 'suspicious':
      return '建议进一步监控相关进程和网络活动，收集更多证据以确认威胁。'
    case 'unknown':
      return '建议人工复核分析结果，结合上下文信息进行判断。'
    default:
      return '建议根据实际情况采取相应措施。'
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
      return '良性/误报'
    case 'malicious':
      return '恶意'
    case 'suspicious':
      return '可疑'
    default:
      return '未知'
  }
}
