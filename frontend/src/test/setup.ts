import { beforeEach } from 'vitest'
import { config } from '@vue/test-utils'
import { DEFAULT_LOCALE, i18n, setLocale } from '@/i18n'

config.global.plugins = [...(config.global.plugins || []), i18n]

beforeEach(() => {
  setLocale(DEFAULT_LOCALE)
})
