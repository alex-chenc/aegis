// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import zhCN from './locales/zh-CN'
import enUS from './locales/en-US'
import {
  DEFAULT_LOCALE,
  LOCALE_STORAGE_KEY,
  getCurrentLocale,
  setLocale,
  toggleLocale,
  translate,
} from './index'
import { formatDateTime, formatRelativeTime } from './formatters'

const sourceModules = import.meta.glob('../**/*.{ts,vue}', {
  eager: true,
  query: '?raw',
  import: 'default',
}) as Record<string, string>

function flattenKeys(value: unknown, prefix = ''): string[] {
  if (!value || typeof value !== 'object') return [prefix]
  return Object.entries(value as Record<string, unknown>)
    .flatMap(([key, child]) => flattenKeys(child, prefix ? `${prefix}.${key}` : key))
}

function flattenMessages(value: unknown, prefix = ''): Record<string, string> {
  if (typeof value === 'string') return { [prefix]: value }
  if (!value || typeof value !== 'object') return {}
  return Object.entries(value as Record<string, unknown>).reduce<Record<string, string>>(
    (messages, [key, child]) => Object.assign(messages, flattenMessages(child, prefix ? `${prefix}.${key}` : key)),
    {},
  )
}

function placeholders(message: string): string[] {
  return [...message.matchAll(/\{([\w.]+)\}/g)].map(match => match[1]).sort()
}

describe('i18n', () => {
  beforeEach(() => {
    localStorage.clear()
    setLocale(DEFAULT_LOCALE)
  })

  it('keeps Chinese and English resource keys aligned', () => {
    expect(flattenKeys(enUS).sort()).toEqual(flattenKeys(zhCN).sort())
  })

  it('keeps interpolation placeholders aligned across locales', () => {
    const zhMessages = flattenMessages(zhCN)
    const enMessages = flattenMessages(enUS)
    for (const key of Object.keys(zhMessages)) {
      expect(placeholders(enMessages[key]), key).toEqual(placeholders(zhMessages[key]))
    }
  })

  it('resolves literal translation keys used by source files', () => {
    const messages = flattenMessages(zhCN)
    const translationCall = /(?:\$t|\btranslate|\bt)\(\s*['"]([^'"]+)['"]/g

    for (const [file, source] of Object.entries(sourceModules)) {
      if (typeof source !== 'string') continue
      for (const match of source.matchAll(translationCall)) {
        expect(messages[match[1]], `${match[1]} referenced by ${file}`).toBeDefined()
      }
    }
  })

  it('switches locale, persists it and updates the document language', () => {
    expect(toggleLocale()).toBe('en-US')
    expect(getCurrentLocale()).toBe('en-US')
    expect(localStorage.getItem(LOCALE_STORAGE_KEY)).toBe('en-US')
    expect(document.documentElement.lang).toBe('en-US')
    expect(translate('app.mode.agentMode')).toBe('Agent Mode')
  })

  it('falls back to Chinese for unsupported locales', () => {
    expect(setLocale('fr-FR')).toBe('zh-CN')
    expect(translate('app.mode.agentMode')).toBe('智能体模式')
  })

  it('formats dates and relative time with the active locale', () => {
    setLocale('en-US')
    expect(formatDateTime('2026-07-10T10:30:00Z')).toMatch(/2026/)
    expect(formatRelativeTime(new Date(Date.now() - 5 * 60_000))).toBe('5 minutes ago')
  })
})
