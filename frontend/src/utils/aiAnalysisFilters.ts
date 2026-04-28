export interface AnalysisAlertLike {
  id: string
  alert_id?: string
  hostname?: string
  rule_title?: string
  mitre_id?: string
  severity?: string
  status?: string
  description?: string
  last_seen_at?: string
}

export interface AnalysisHostLike {
  hostname?: string
  online?: boolean
}

export function filterOnlineHostnames(hosts: AnalysisHostLike[]) {
  return Array.from(
    new Set(
      hosts
        .filter(host => host.online && host.hostname)
        .map(host => host.hostname as string)
    )
  )
}

function parseTime(value?: string) {
  if (!value) return null
  const time = Date.parse(value)
  return Number.isNaN(time) ? null : time
}

function isInTimeRange(alert: AnalysisAlertLike, timeRange?: [string, string] | null) {
  if (!timeRange?.[0] || !timeRange?.[1]) return true

  const alertTime = parseTime(alert.last_seen_at)
  const startTime = parseTime(timeRange[0])
  const endTime = parseTime(timeRange[1])

  if (alertTime === null || startTime === null || endTime === null) return false
  return alertTime >= startTime && alertTime <= endTime
}

export function filterAnalysisAlerts<T extends AnalysisAlertLike>(
  alerts: T[],
  hostFilter: string[],
  timeRange?: [string, string] | null
) {
  const hasHostFilter = hostFilter.length > 0
  const hasTimeRange = Boolean(timeRange?.[0] && timeRange?.[1])
  if (!hasHostFilter && !hasTimeRange) return []

  return alerts.filter(alert => {
    const hostMatched = !hasHostFilter || Boolean(alert.hostname && hostFilter.includes(alert.hostname))
    return hostMatched && isInTimeRange(alert, timeRange)
  })
}

export function pruneSelectedAlertIds<T extends AnalysisAlertLike>(selectedIds: string[], visibleAlerts: T[]) {
  const visibleIds = new Set(visibleAlerts.map(alert => alert.id))
  return selectedIds.filter(id => visibleIds.has(id))
}

export function buildAnalysisAlertSnapshot<T extends AnalysisAlertLike>(alerts: T[], selectedIds: string[]) {
  const alertById = new Map(alerts.map(alert => [alert.id, alert]))
  return selectedIds
    .map(id => alertById.get(id))
    .filter((alert): alert is T => Boolean(alert))
    .map(alert => ({ ...alert }))
}
