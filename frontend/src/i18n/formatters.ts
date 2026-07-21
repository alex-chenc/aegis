import { getCurrentLocale, translate } from './index'

export function formatDateTime(value?: string | number | Date | null): string {
  if (value === undefined || value === null || value === '') return '-'
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return new Intl.DateTimeFormat(getCurrentLocale(), {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date)
}

export function formatDate(value?: string | number | Date | null): string {
  if (value === undefined || value === null || value === '') return '-'
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return new Intl.DateTimeFormat(getCurrentLocale(), {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(date)
}

export function formatTime(value?: string | number | Date | null): string {
  if (value === undefined || value === null || value === '') return '-'
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return new Intl.DateTimeFormat(getCurrentLocale(), {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date)
}

export function formatNumber(value: number): string {
  return new Intl.NumberFormat(getCurrentLocale()).format(value)
}

export function formatPercent(value: number, maximumFractionDigits = 1): string {
  return new Intl.NumberFormat(getCurrentLocale(), {
    style: 'percent',
    maximumFractionDigits,
  }).format(value)
}

export function formatRelativeTime(value?: string | number | Date | null): string {
  if (value === undefined || value === null || value === '') return '-'
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  const diff = Math.max(0, Math.floor((Date.now() - date.getTime()) / 1000))
  if (diff < 60) return translate('common.time.justNow')
  if (diff < 3600) return translate('common.time.minutesAgo', { count: Math.floor(diff / 60) })
  if (diff < 86400) return translate('common.time.hoursAgo', { count: Math.floor(diff / 3600) })
  if (diff < 604800) return translate('common.time.daysAgo', { count: Math.floor(diff / 86400) })
  return formatDate(date)
}
