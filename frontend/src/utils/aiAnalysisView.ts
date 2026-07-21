import { translate } from '@/i18n'
import { formatDateTime } from '@/i18n/formatters'

interface AlertSnapshotLike {
  id: string
  hostname?: string
  rule_title?: string
  severity?: string
  description?: string
  last_seen_at?: string
}

const severityLabelKeys: Record<string, string> = {
  critical: 'analysis.severity.critical', high: 'analysis.severity.high', medium: 'analysis.severity.medium', low: 'analysis.severity.low',
}

function formatTimestamp(timestamp?: string) {
  if (!timestamp) return '-'
  return formatDateTime(timestamp)
}

export function buildInitialAnalysisMessage(
  alerts: AlertSnapshotLike[],
  timeRange?: [string, string] | null
) {
  const lines = [translate('analysis.prompt.intro', { count: alerts.length })]

  alerts.forEach((alert, index) => {
    lines.push(
      `${index + 1}. ${translate('analysis.prompt.alertId', { value: alert.id })}`,
      `   ${translate('analysis.prompt.host', { value: alert.hostname || '-' })}`,
      `   ${translate('analysis.prompt.rule', { value: alert.rule_title || '-' })}`,
      `   ${translate('analysis.prompt.severity', { value: severityLabelKeys[alert.severity || ''] ? translate(severityLabelKeys[alert.severity || '']) : alert.severity || '-' })}`,
      `   ${translate('analysis.prompt.latest', { value: formatTimestamp(alert.last_seen_at) })}`,
      `   ${translate('analysis.prompt.description', { value: alert.description || '-' })}`
    )
  })

  if (timeRange?.[0] && timeRange?.[1]) {
    lines.push(
      '',
      translate('analysis.prompt.timeRange', { start: formatTimestamp(timeRange[0]), end: formatTimestamp(timeRange[1]) })
    )
  }

  lines.push('', translate('analysis.prompt.request'))
  return lines.join('\n')
}

export function normalizeAIAnalysisErrorMessage(message: string) {
  if (message.includes('Maximum iterations reached without final answer')) {
    return translate('analysis.prompt.maxIterations')
  }
  return message
}
