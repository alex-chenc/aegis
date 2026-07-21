import { computed } from 'vue'
import { createI18n } from 'vue-i18n'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import en from 'element-plus/es/locale/lang/en'
import zhCNMessages from './locales/zh-CN'
import enUSMessages from './locales/en-US'

export const SUPPORTED_LOCALES = ['zh-CN', 'en-US'] as const
export type SupportedLocale = typeof SUPPORTED_LOCALES[number]

export const DEFAULT_LOCALE: SupportedLocale = 'zh-CN'
export const LOCALE_STORAGE_KEY = 'aegis_locale'

export function isSupportedLocale(value: unknown): value is SupportedLocale {
  return typeof value === 'string' && SUPPORTED_LOCALES.includes(value as SupportedLocale)
}

function readStoredLocale(): SupportedLocale {
  if (typeof window === 'undefined') return DEFAULT_LOCALE
  try {
    const stored = window.localStorage.getItem(LOCALE_STORAGE_KEY)
    if (isSupportedLocale(stored)) return stored
    if (stored) window.localStorage.setItem(LOCALE_STORAGE_KEY, DEFAULT_LOCALE)
  } catch {
    // Storage may be disabled; the in-memory locale remains usable.
  }
  return DEFAULT_LOCALE
}

export const i18n = createI18n({
  legacy: false,
  locale: readStoredLocale(),
  fallbackLocale: DEFAULT_LOCALE,
  missingWarn: false,
  fallbackWarn: false,
  messages: {
    'zh-CN': zhCNMessages,
    'en-US': enUSMessages,
  },
})

export const currentLocale = i18n.global.locale

export function getCurrentLocale(): SupportedLocale {
  const value = currentLocale.value
  return isSupportedLocale(value) ? value : DEFAULT_LOCALE
}

export function setLocale(value: unknown, persist = true): SupportedLocale {
  const next = isSupportedLocale(value) ? value : DEFAULT_LOCALE
  currentLocale.value = next

  if (typeof document !== 'undefined') {
    document.documentElement.lang = next
  }
  if (persist && typeof window !== 'undefined') {
    try {
      window.localStorage.setItem(LOCALE_STORAGE_KEY, next)
    } catch {
      // Storage may be disabled; switching still works for this page session.
    }
  }
  return next
}

export function toggleLocale(): SupportedLocale {
  return setLocale(getCurrentLocale() === 'zh-CN' ? 'en-US' : 'zh-CN')
}

export const elementLocale = computed(() => getCurrentLocale() === 'en-US' ? en : zhCn)

export function translate(key: string, params?: Record<string, unknown>): string {
  return String(params ? i18n.global.t(key, params) : i18n.global.t(key))
}

export function installLocaleSync(): () => void {
  setLocale(getCurrentLocale(), false)
  if (typeof window === 'undefined') return () => undefined

  const handleStorage = (event: StorageEvent) => {
    if (event.key === LOCALE_STORAGE_KEY) setLocale(event.newValue, false)
  }
  window.addEventListener('storage', handleStorage)
  return () => window.removeEventListener('storage', handleStorage)
}
