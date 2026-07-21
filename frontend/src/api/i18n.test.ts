// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import { setLocale } from '@/i18n'
import { localizeAPIError } from './index'

describe('API localization', () => {
  beforeEach(() => {
    setLocale('zh-CN')
  })

  it('uses a known error code in the active language', () => {
    setLocale('en-US')
    expect(localizeAPIError('TASK_DELETE_RUNNING')).toBe('Running tasks cannot be deleted')
  })

  it('does not leak a Chinese fallback while English is active', () => {
    setLocale('en-US')
    expect(localizeAPIError(undefined, undefined, '后端中文错误')).toBe('Request failed')
  })

  it('keeps compatible backend detail in Chinese mode', () => {
    expect(localizeAPIError(undefined, undefined, '后端中文错误')).toBe('后端中文错误')
  })
})
