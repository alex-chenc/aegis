interface AlertSnapshotLike {
  id: string
  hostname?: string
  rule_title?: string
  severity?: string
  description?: string
  last_seen_at?: string
}

const severityLabels: Record<string, string> = {
  critical: '严重',
  high: '高危',
  medium: '中危',
  low: '低危'
}

function formatTimestamp(timestamp?: string) {
  if (!timestamp) return '-'
  return new Date(timestamp).toLocaleString('zh-CN')
}

export function buildInitialAnalysisMessage(
  alerts: AlertSnapshotLike[],
  timeRange?: [string, string] | null
) {
  const lines = [`本次需要分析以下 ${alerts.length} 条真实告警：`]

  alerts.forEach((alert, index) => {
    lines.push(
      `${index + 1}. 告警ID：${alert.id}`,
      `   主机：${alert.hostname || '-'}`,
      `   规则：${alert.rule_title || '-'}`,
      `   级别：${severityLabels[alert.severity || ''] || alert.severity || '-'}`,
      `   最近时间：${formatTimestamp(alert.last_seen_at)}`,
      `   描述：${alert.description || '-'}`
    )
  })

  if (timeRange?.[0] && timeRange?.[1]) {
    lines.push(
      '',
      `分析时间范围：${formatTimestamp(timeRange[0])} 至 ${formatTimestamp(timeRange[1])}`
    )
  }

  lines.push('', '请结合以上真实告警内容判断是否为真实威胁，并给出攻击链路溯源与处置建议。')
  return lines.join('\n')
}

export function normalizeAIAnalysisErrorMessage(message: string) {
  if (message.includes('Maximum iterations reached without final answer')) {
    return 'AI 已达到最大推理轮数，但仍未生成最终结论。请缩小告警范围、补充问题，或提高最大轮数后重试。'
  }
  return message
}
